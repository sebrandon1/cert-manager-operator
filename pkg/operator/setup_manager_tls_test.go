package operator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/openshift/cert-manager-operator/pkg/controller/istiocsr"
	"github.com/openshift/cert-manager-operator/pkg/tlsprofile"
)

// stubReader is a minimal client.Reader that serves a single APIServer object
// or returns a fixed error from Get. Errors in failFirst are returned, in
// order, by the first Get calls before falling back to getErr/apiServer.
type stubReader struct {
	apiServer *configv1.APIServer
	getErr    error
	failFirst []error
	calls     int
	onGet     func() // runs at the start of every Get, if set
}

func (s *stubReader) Get(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	s.calls++
	if s.onGet != nil {
		s.onGet()
	}
	if s.calls <= len(s.failFirst) {
		return s.failFirst[s.calls-1]
	}
	if s.getErr != nil {
		return s.getErr
	}
	out, ok := obj.(*configv1.APIServer)
	if !ok {
		return fmt.Errorf("stubReader: unexpected object type %T", obj)
	}
	s.apiServer.DeepCopyInto(out)
	return nil
}

func (s *stubReader) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return errors.New("stubReader: List not implemented")
}

func apiServerWith(adherence configv1.TLSAdherencePolicy, profile *configv1.TLSSecurityProfile) *configv1.APIServer {
	return &configv1.APIServer{
		Spec: configv1.APIServerSpec{
			TLSAdherence:       adherence,
			TLSSecurityProfile: profile,
		},
	}
}

func mustEffectiveSpec(t *testing.T, profile *configv1.TLSSecurityProfile) *configv1.TLSProfileSpec {
	t.Helper()
	spec, err := tlsprofile.EffectiveSpec(profile)
	if err != nil {
		t.Fatalf("EffectiveSpec(%v): %v", profile, err)
	}
	return spec
}

// apiServersGR is the GroupResource of apiserver.config.openshift.io.
var apiServersGR = schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}

// fastAPIServerReadBackoff shrinks apiServerReadBackoff for the duration of
// the test so retry cases don't sleep for seconds.
func fastAPIServerReadBackoff(t *testing.T) {
	t.Helper()
	orig := apiServerReadBackoff
	apiServerReadBackoff.Duration = time.Millisecond
	t.Cleanup(func() { apiServerReadBackoff = orig })
}

func TestResolveIstioCSRTLSConfig(t *testing.T) {
	fastAPIServerReadBackoff(t)
	modern := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}
	unavailable := apierrors.NewServiceUnavailable("apiserver is shutting down")
	strictModern := apiServerWith(configv1.TLSAdherencePolicyStrictAllComponents, modern)

	tests := []struct {
		name      string
		reader    *stubReader
		want      istiocsr.TLSConfig
		wantErr   string
		wantCalls int // 0 means "don't check"
	}{
		{
			name:   "apiserver/cluster not found uses default TLS config",
			reader: &stubReader{getErr: apierrors.NewNotFound(apiServersGR, "cluster")},
			want:   istiocsr.TLSConfig{},
		},
		{
			name:      "apiserver/cluster read error is returned without retry",
			reader:    &stubReader{getErr: errors.New("connection refused")},
			wantErr:   "failed to get apiserver.config.openshift.io/cluster",
			wantCalls: 1,
		},
		{
			name:      "Forbidden is returned without retry",
			reader:    &stubReader{getErr: apierrors.NewForbidden(apiServersGR, "cluster", errors.New("rbac"))},
			wantErr:   "forbidden",
			wantCalls: 1,
		},
		{
			name:   "transient 503s are retried until the read succeeds",
			reader: &stubReader{failFirst: []error{unavailable, unavailable}, apiServer: strictModern},
			want: istiocsr.TLSConfig{
				Adherence: configv1.TLSAdherencePolicyStrictAllComponents,
				Spec:      mustEffectiveSpec(t, modern),
				Honor:     true,
			},
			wantCalls: 3,
		},
		{
			name:      "transient error then NotFound uses default TLS config",
			reader:    &stubReader{failFirst: []error{apierrors.NewTimeoutError("timeout", 1)}, getErr: apierrors.NewNotFound(apiServersGR, "cluster")},
			want:      istiocsr.TLSConfig{},
			wantCalls: 2,
		},
		{
			name:      "persistent 503 fails after the backoff is exhausted",
			reader:    &stubReader{getErr: unavailable},
			wantErr:   "apiserver is shutting down",
			wantCalls: apiServerReadBackoff.Steps,
		},
		{
			name:   "empty tlsAdherence does not honor the cluster profile",
			reader: &stubReader{apiServer: apiServerWith(configv1.TLSAdherencePolicyNoOpinion, modern)},
			want:   istiocsr.TLSConfig{Adherence: configv1.TLSAdherencePolicyNoOpinion, Honor: false},
		},
		{
			name:   "LegacyAdheringComponentsOnly does not honor the cluster profile",
			reader: &stubReader{apiServer: apiServerWith(configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly, modern)},
			want:   istiocsr.TLSConfig{Adherence: configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly, Honor: false},
		},
		{
			name:   "StrictAllComponents with no profile resolves to Intermediate",
			reader: &stubReader{apiServer: apiServerWith(configv1.TLSAdherencePolicyStrictAllComponents, nil)},
			want: istiocsr.TLSConfig{
				Adherence: configv1.TLSAdherencePolicyStrictAllComponents,
				Spec:      mustEffectiveSpec(t, &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType}),
				Honor:     true,
			},
		},
		{
			name:   "StrictAllComponents with Modern profile resolves to Modern",
			reader: &stubReader{apiServer: apiServerWith(configv1.TLSAdherencePolicyStrictAllComponents, modern)},
			want: istiocsr.TLSConfig{
				Adherence: configv1.TLSAdherencePolicyStrictAllComponents,
				Spec:      mustEffectiveSpec(t, modern),
				Honor:     true,
			},
		},
		{
			name:   "unknown tlsAdherence is treated as StrictAllComponents",
			reader: &stubReader{apiServer: apiServerWith("FutureAdherence", modern)},
			want: istiocsr.TLSConfig{
				Adherence: "FutureAdherence",
				Spec:      mustEffectiveSpec(t, modern),
				Honor:     true,
			},
		},
		{
			name: "StrictAllComponents with Custom profile missing settings fails",
			reader: &stubReader{apiServer: apiServerWith(configv1.TLSAdherencePolicyStrictAllComponents,
				&configv1.TLSSecurityProfile{Type: configv1.TLSProfileCustomType})},
			wantErr: "failed to resolve effective TLS profile",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveIstioCSRTLSConfig(context.Background(), tt.reader)
			if tt.wantCalls != 0 && tt.reader.calls != tt.wantCalls {
				t.Errorf("Get calls = %d, want %d", tt.reader.calls, tt.wantCalls)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolveIstioCSRTLSConfig() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestResolveIstioCSRTLSConfigCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := &stubReader{apiServer: apiServerWith(configv1.TLSAdherencePolicyNoOpinion, nil)}

	_, err := resolveIstioCSRTLSConfig(ctx, reader)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if reader.calls != 0 {
		t.Errorf("Get calls = %d, want 0 with a cancelled context", reader.calls)
	}
}

func TestResolveIstioCSRTLSConfigCancelledDuringBackoff(t *testing.T) {
	// A full-length backoff: the wait between attempts must be cut short by
	// the cancellation, not by the timer.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := &stubReader{getErr: apierrors.NewServiceUnavailable("apiserver is shutting down"), onGet: cancel}

	start := time.Now()
	_, err := resolveIstioCSRTLSConfig(ctx, reader)
	if err == nil || !strings.Contains(err.Error(), "apiserver is shutting down") {
		t.Fatalf("error = %v, want the last transient error", err)
	}
	if reader.calls != 1 {
		t.Errorf("Get calls = %d, want 1 after cancellation", reader.calls)
	}
	if elapsed := time.Since(start); elapsed >= apiServerReadBackoff.Duration {
		t.Errorf("returned after %v, want before the first backoff step (%v)", elapsed, apiServerReadBackoff.Duration)
	}
}

func TestIsTransientAPIError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "503 ServiceUnavailable", err: apierrors.NewServiceUnavailable("unavailable"), want: true},
		{name: "504 Timeout", err: apierrors.NewTimeoutError("timeout", 1), want: true},
		{name: "ServerTimeout", err: apierrors.NewServerTimeout(apiServersGR, "get", 1), want: true},
		{name: "429 TooManyRequests", err: apierrors.NewTooManyRequests("slow down", 1), want: true},
		{name: "500 InternalError", err: apierrors.NewInternalError(errors.New("etcdserver: leader changed")), want: true},
		{name: "connection refused", err: &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, want: true},
		{name: "connection reset", err: &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}, want: true},
		{name: "unexpected EOF", err: io.ErrUnexpectedEOF, want: true},
		{name: "net timeout", err: &net.DNSError{Err: "i/o timeout", IsTimeout: true}, want: true},
		{name: "wrapped 503", err: fmt.Errorf("get: %w", apierrors.NewServiceUnavailable("unavailable")), want: true},
		{name: "NotFound", err: apierrors.NewNotFound(apiServersGR, "cluster"), want: false},
		{name: "Forbidden", err: apierrors.NewForbidden(apiServersGR, "cluster", errors.New("rbac")), want: false},
		{name: "plain error", err: errors.New("boom"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientAPIError(tt.err); got != tt.want {
				t.Errorf("isTransientAPIError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

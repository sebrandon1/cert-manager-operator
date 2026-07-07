package http01proxy

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/cert-manager-operator/api/operator/v1alpha1"
)

func TestGetInternalPort(t *testing.T) {
	tests := []struct {
		name string
		proxy *v1alpha1.HTTP01Proxy
		want  int32
	}{
		{
			name: "DefaultDeployment mode returns default port",
			proxy: &v1alpha1.HTTP01Proxy{
				Spec: v1alpha1.HTTP01ProxySpec{Mode: v1alpha1.HTTP01ProxyModeDefault},
			},
			want: defaultInternalPort,
		},
		{
			name: "CustomDeployment with port set",
			proxy: &v1alpha1.HTTP01Proxy{
				Spec: v1alpha1.HTTP01ProxySpec{
					Mode:             v1alpha1.HTTP01ProxyModeCustom,
					CustomDeployment: &v1alpha1.HTTP01ProxyCustomDeploymentSpec{InternalPort: 9999},
				},
			},
			want: 9999,
		},
		{
			name: "CustomDeployment with nil CustomDeployment pointer",
			proxy: &v1alpha1.HTTP01Proxy{
				Spec: v1alpha1.HTTP01ProxySpec{
					Mode:             v1alpha1.HTTP01ProxyModeCustom,
					CustomDeployment: nil,
				},
			},
			want: defaultInternalPort,
		},
		{
			name: "CustomDeployment with port zero",
			proxy: &v1alpha1.HTTP01Proxy{
				Spec: v1alpha1.HTTP01ProxySpec{
					Mode:             v1alpha1.HTTP01ProxyModeCustom,
					CustomDeployment: &v1alpha1.HTTP01ProxyCustomDeploymentSpec{InternalPort: 0},
				},
			},
			want: defaultInternalPort,
		},
	}

	r := &Reconciler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.getInternalPort(tt.proxy)
			if got != tt.want {
				t.Errorf("getInternalPort() = %d, want %d", got, tt.want)
			}
		})
	}
}

func baseDaemonSet(image string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "proxy"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"app": "proxy"},
					Annotations: map[string]string{"version": "1"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "proxy",
						Image: image,
						Ports: []corev1.ContainerPort{{Name: proxyPortName, ContainerPort: 8888, HostPort: 8888}},
					}},
					HostNetwork: true,
				},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
			},
		},
	}
}

func TestDaemonSetSpecModified(t *testing.T) {
	tests := []struct {
		name    string
		desired *appsv1.DaemonSet
		fetched *appsv1.DaemonSet
		want    bool
	}{
		{
			name:    "identical specs",
			desired: baseDaemonSet("img:v1"),
			fetched: baseDaemonSet("img:v1"),
			want:    false,
		},
		{
			name:    "different image",
			desired: baseDaemonSet("img:v1"),
			fetched: baseDaemonSet("img:v2"),
			want:    true,
		},
		{
			name: "different template labels",
			desired: baseDaemonSet("img:v1"),
			fetched: func() *appsv1.DaemonSet {
				ds := baseDaemonSet("img:v1")
				ds.Spec.Template.Labels = map[string]string{"app": "other"}
				return ds
			}(),
			want: true,
		},
		{
			name: "different template annotations",
			desired: baseDaemonSet("img:v1"),
			fetched: func() *appsv1.DaemonSet {
				ds := baseDaemonSet("img:v1")
				ds.Spec.Template.Annotations = map[string]string{"version": "2"}
				return ds
			}(),
			want: true,
		},
		{
			name: "different selector",
			desired: baseDaemonSet("img:v1"),
			fetched: func() *appsv1.DaemonSet {
				ds := baseDaemonSet("img:v1")
				ds.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "other"}}
				return ds
			}(),
			want: true,
		},
		{
			name: "different update strategy",
			desired: baseDaemonSet("img:v1"),
			fetched: func() *appsv1.DaemonSet {
				ds := baseDaemonSet("img:v1")
				ds.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{Type: appsv1.OnDeleteDaemonSetStrategyType}
				return ds
			}(),
			want: true,
		},
		{
			name: "different hostNetwork",
			desired: baseDaemonSet("img:v1"),
			fetched: func() *appsv1.DaemonSet {
				ds := baseDaemonSet("img:v1")
				ds.Spec.Template.Spec.HostNetwork = false
				return ds
			}(),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := daemonSetSpecModified(tt.desired, tt.fetched)
			if got != tt.want {
				t.Errorf("daemonSetSpecModified() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasObjectChanged(t *testing.T) {
	tests := []struct {
		name        string
		desired     client.Object
		fetched     client.Object
		wantChanged bool
		wantErr     bool
	}{
		{
			name:    "type mismatch returns error",
			desired: &rbacv1.ClusterRole{},
			fetched: &rbacv1.ClusterRoleBinding{},
			wantErr: true,
		},
		{
			name: "ClusterRole identical",
			desired: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}}},
			},
			fetched: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}}},
			},
			wantChanged: false,
		},
		{
			name: "ClusterRole rules different",
			desired: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}}},
			},
			fetched: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				Rules:      []rbacv1.PolicyRule{{Verbs: []string{"list"}}},
			},
			wantChanged: true,
		},
		{
			name: "ClusterRole metadata different",
			desired: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}}},
			},
			fetched: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "2"}},
				Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}}},
			},
			wantChanged: true,
		},
		{
			name: "ClusterRoleBinding identical",
			desired: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				RoleRef:    rbacv1.RoleRef{APIGroup: "rbac", Kind: "ClusterRole", Name: "x"},
				Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "sa", Namespace: "ns"}},
			},
			fetched: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				RoleRef:    rbacv1.RoleRef{APIGroup: "rbac", Kind: "ClusterRole", Name: "x"},
				Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "sa", Namespace: "ns"}},
			},
			wantChanged: false,
		},
		{
			name: "ClusterRoleBinding RoleRef different",
			desired: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				RoleRef:    rbacv1.RoleRef{APIGroup: "rbac", Kind: "ClusterRole", Name: "role-a"},
				Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "sa", Namespace: "ns"}},
			},
			fetched: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				RoleRef:    rbacv1.RoleRef{APIGroup: "rbac", Kind: "ClusterRole", Name: "role-b"},
				Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "sa", Namespace: "ns"}},
			},
			wantChanged: true,
		},
		{
			name: "ClusterRoleBinding Subjects different",
			desired: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				RoleRef:    rbacv1.RoleRef{APIGroup: "rbac", Kind: "ClusterRole", Name: "x"},
				Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "sa1", Namespace: "ns"}},
			},
			fetched: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				RoleRef:    rbacv1.RoleRef{APIGroup: "rbac", Kind: "ClusterRole", Name: "x"},
				Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "sa2", Namespace: "ns"}},
			},
			wantChanged: true,
		},
		{
			name: "DaemonSet identical",
			desired: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				Spec:       baseDaemonSet("img:v1").Spec,
			},
			fetched: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				Spec:       baseDaemonSet("img:v1").Spec,
			},
			wantChanged: false,
		},
		{
			name: "DaemonSet spec different",
			desired: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				Spec:       baseDaemonSet("img:v1").Spec,
			},
			fetched: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				Spec:       baseDaemonSet("img:v2").Spec,
			},
			wantChanged: true,
		},
		{
			name: "NetworkPolicy identical",
			desired: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				Spec:       networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}},
			},
			fetched: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				Spec:       networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}},
			},
			wantChanged: false,
		},
		{
			name: "NetworkPolicy spec different",
			desired: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				Spec:       networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}},
			},
			fetched: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
				Spec:       networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}},
			},
			wantChanged: true,
		},
		{
			name: "ServiceAccount metadata identical",
			desired: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
			},
			fetched: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
			},
			wantChanged: false,
		},
		{
			name: "ServiceAccount metadata different",
			desired: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}},
			},
			fetched: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "2"}},
			},
			wantChanged: true,
		},
		{
			name:    "unsupported type returns error",
			desired: &corev1.Pod{},
			fetched: &corev1.Pod{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hasObjectChanged(tt.desired, tt.fetched)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantChanged {
				t.Errorf("hasObjectChanged() = %v, want %v", got, tt.wantChanged)
			}
		})
	}
}

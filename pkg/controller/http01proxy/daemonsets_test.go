package http01proxy

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/go-logr/logr"

	"github.com/openshift/cert-manager-operator/api/operator/v1alpha1"
	"github.com/openshift/cert-manager-operator/pkg/controller/common"
)

func newDefaultProxy() *v1alpha1.HTTP01Proxy {
	proxy := &v1alpha1.HTTP01Proxy{
		Spec: v1alpha1.HTTP01ProxySpec{Mode: v1alpha1.HTTP01ProxyModeDefault},
	}
	proxy.SetNamespace(common.OperatorNamespace)
	return proxy
}

func TestGetDaemonSetObject(t *testing.T) {
	r := &Reconciler{
		log:        logr.Discard(),
		proxyImage: "quay.io/test/proxy:v1",
	}

	t.Run("empty proxy image returns error", func(t *testing.T) {
		noImage := &Reconciler{log: logr.Discard(), proxyImage: ""}
		_, err := noImage.getDaemonSetObject(newDefaultProxy(), map[string]string{"app": "test"})
		if err == nil {
			t.Fatal("expected error for empty proxyImage")
		}
		if !strings.Contains(err.Error(), http01proxyImageNameEnvVarName) {
			t.Errorf("error = %q, want mention of env var %q", err.Error(), http01proxyImageNameEnvVarName)
		}
	})

	t.Run("valid build with default mode", func(t *testing.T) {
		ds, err := r.getDaemonSetObject(newDefaultProxy(), map[string]string{"app": "test"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ds.GetNamespace() != common.OperatorNamespace {
			t.Errorf("namespace = %q, want %q", ds.GetNamespace(), common.OperatorNamespace)
		}
		if len(ds.Spec.Template.Spec.Containers) == 0 {
			t.Fatal("expected at least one container")
		}
		if ds.Spec.Template.Spec.Containers[0].Image != "quay.io/test/proxy:v1" {
			t.Errorf("image = %q, want %q", ds.Spec.Template.Spec.Containers[0].Image, "quay.io/test/proxy:v1")
		}
	})

	t.Run("custom mode with custom port", func(t *testing.T) {
		proxy := &v1alpha1.HTTP01Proxy{
			Spec: v1alpha1.HTTP01ProxySpec{
				Mode:             v1alpha1.HTTP01ProxyModeCustom,
				CustomDeployment: &v1alpha1.HTTP01ProxyCustomDeploymentSpec{InternalPort: 9999},
			},
		}
		proxy.SetNamespace(common.OperatorNamespace)

		ds, err := r.getDaemonSetObject(proxy, map[string]string{"app": "test"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		found := false
		for _, port := range ds.Spec.Template.Spec.Containers[0].Ports {
			if port.Name == proxyPortName {
				if port.ContainerPort != 9999 {
					t.Errorf("container port = %d, want %d", port.ContainerPort, 9999)
				}
				if port.HostPort != 9999 {
					t.Errorf("host port = %d, want %d", port.HostPort, 9999)
				}
				found = true
			}
		}
		if !found {
			t.Error("expected port with name 'proxy' in container ports")
		}
	})

	t.Run("labels propagated to template", func(t *testing.T) {
		labels := map[string]string{"custom-label": "value"}
		ds, err := r.getDaemonSetObject(newDefaultProxy(), labels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ds.Spec.Template.Labels["custom-label"] != "value" {
			t.Error("expected custom label to be propagated to template labels")
		}
	})
}

func TestUpdateDaemonSetPort(t *testing.T) {
	r := &Reconciler{}

	t.Run("updates existing proxy port", func(t *testing.T) {
		ds := &appsv1.DaemonSet{
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "proxy",
							Ports: []corev1.ContainerPort{{Name: proxyPortName, ContainerPort: 8888, HostPort: 8888}},
							Env:   []corev1.EnvVar{{Name: proxyPortEnvVar, Value: "8888"}},
						}},
					},
				},
			},
		}

		r.updateDaemonSetPort(ds, 9999)

		container := ds.Spec.Template.Spec.Containers[0]
		portFound := false
		for _, port := range container.Ports {
			if port.Name == proxyPortName {
				if port.ContainerPort != 9999 {
					t.Errorf("container port = %d, want 9999", port.ContainerPort)
				}
				if port.HostPort != 9999 {
					t.Errorf("host port = %d, want 9999", port.HostPort)
				}
				portFound = true
			}
		}
		if !portFound {
			t.Error("expected port with name 'proxy' in container ports")
		}
		envFound := false
		for _, env := range container.Env {
			if env.Name == proxyPortEnvVar {
				if env.Value != "9999" {
					t.Errorf("env value = %q, want %q", env.Value, "9999")
				}
				envFound = true
			}
		}
		if !envFound {
			t.Error("expected PROXY_PORT env var")
		}
	})

	t.Run("appends env var when missing", func(t *testing.T) {
		ds := &appsv1.DaemonSet{
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "proxy",
							Ports: []corev1.ContainerPort{{Name: proxyPortName, ContainerPort: 8888, HostPort: 8888}},
							Env:   []corev1.EnvVar{},
						}},
					},
				},
			},
		}

		r.updateDaemonSetPort(ds, 7777)

		container := ds.Spec.Template.Spec.Containers[0]
		found := false
		for _, env := range container.Env {
			if env.Name == proxyPortEnvVar {
				if env.Value != "7777" {
					t.Errorf("env value = %q, want %q", env.Value, "7777")
				}
				found = true
			}
		}
		if !found {
			t.Error("expected PROXY_PORT env var to be appended")
		}
	})

	t.Run("does not modify non-proxy ports", func(t *testing.T) {
		ds := &appsv1.DaemonSet{
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "proxy",
							Ports: []corev1.ContainerPort{
								{Name: "metrics", ContainerPort: 9090, HostPort: 9090},
								{Name: proxyPortName, ContainerPort: 8888, HostPort: 8888},
							},
							Env: []corev1.EnvVar{{Name: proxyPortEnvVar, Value: "8888"}},
						}},
					},
				},
			},
		}

		r.updateDaemonSetPort(ds, 5555)

		container := ds.Spec.Template.Spec.Containers[0]
		for _, port := range container.Ports {
			if port.Name == "metrics" {
				if port.ContainerPort != 9090 {
					t.Errorf("metrics port should be unchanged, got %d", port.ContainerPort)
				}
			}
		}
	})
}

func TestUpdateImageInStatus(t *testing.T) {
	r := &Reconciler{}

	t.Run("sets proxy image from daemonset container", func(t *testing.T) {
		proxy := &v1alpha1.HTTP01Proxy{}
		ds := &appsv1.DaemonSet{
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Image: "quay.io/proxy:v2"}},
					},
				},
			},
		}

		r.updateImageInStatus(proxy, ds)

		if proxy.Status.ProxyImage != "quay.io/proxy:v2" {
			t.Errorf("ProxyImage = %q, want %q", proxy.Status.ProxyImage, "quay.io/proxy:v2")
		}
	})

	t.Run("no-op when no containers", func(t *testing.T) {
		proxy := &v1alpha1.HTTP01Proxy{}
		proxy.Status.ProxyImage = "original"
		ds := &appsv1.DaemonSet{
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{},
					},
				},
			},
		}

		r.updateImageInStatus(proxy, ds)

		if proxy.Status.ProxyImage != "original" {
			t.Errorf("ProxyImage should be unchanged, got %q", proxy.Status.ProxyImage)
		}
	})
}

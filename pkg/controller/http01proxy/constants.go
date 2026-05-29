package http01proxy

import (
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openshift/cert-manager-operator/pkg/controller/common"
)

var (
	// infrastructureGVK is the GVK for the OpenShift Infrastructure resource.
	infrastructureGVK = schema.GroupVersionKind{
		Group:   "config.openshift.io",
		Version: "v1",
		Kind:    "Infrastructure",
	}
)

const (
	// http01proxyCommonName is the name commonly used for naming resources.
	http01proxyCommonName = "cert-manager-http01-proxy"

	// ControllerName is the name of the controller used in logs and events.
	ControllerName = http01proxyCommonName + "-controller"

	// controllerProcessedAnnotation is the annotation added to http01proxy resource once after
	// successful reconciliation by the controller.
	controllerProcessedAnnotation = "operator.openshift.io/http01-proxy-processed"

	// finalizer name for http01proxies.operator.openshift.io resource.
	finalizer = "http01proxy.openshift.operator.io/" + ControllerName

	// defaultRequeueTime is the default reconcile requeue time.
	defaultRequeueTime = time.Second * 30

	// http01proxyObjectName is the name of the http01proxy resource created by user.
	// The CRD enforces name to be `default`.
	http01proxyObjectName = "default"

	// http01proxyImageNameEnvVarName is the environment variable key name
	// containing the image name of the http01proxy as value.
	http01proxyImageNameEnvVarName = "RELATED_IMAGE_CERT_MANAGER_HTTP01PROXY"

	// http01proxyImageVersionEnvVarName is the environment variable key name
	// containing the image version of the http01proxy as value.
	http01proxyImageVersionEnvVarName = "HTTP01PROXY_OPERAND_IMAGE_VERSION"

	// defaultInternalPort is the default port the proxy listens on.
	defaultInternalPort int32 = 8888

	// proxyPortName is the name of the proxy container port in the DaemonSet spec.
	proxyPortName = "proxy"

	// proxyPortEnvVar is the environment variable name for the proxy port.
	proxyPortEnvVar = "PROXY_PORT"
)

var (
	controllerDefaultResourceLabels = map[string]string{
		common.ManagedResourceLabelKey: http01proxyCommonName,
		"app.kubernetes.io/name":       http01proxyCommonName,
		"app.kubernetes.io/instance":   http01proxyCommonName,
		"app.kubernetes.io/version":    os.Getenv(http01proxyImageVersionEnvVarName),
		"app.kubernetes.io/managed-by": "cert-manager-operator",
		"app.kubernetes.io/part-of":    "cert-manager-operator",
	}
)

// asset names are the files present in the root bindata/ dir.
const (
	daemonsetAssetName          = "http01-proxy/cert-manager-http01-proxy-daemonset.yaml"
	serviceAccountAssetName     = "http01-proxy/cert-manager-http01-proxy-serviceaccount.yaml"
	clusterRoleAssetName        = "http01-proxy/cert-manager-http01-proxy-clusterrole.yaml"
	clusterRoleBindingAssetName = "http01-proxy/cert-manager-http01-proxy-clusterrolebinding.yaml"
	sccRoleBindingAssetName     = "http01-proxy/cert-manager-http01-proxy-scc-rolebinding.yaml"
)

var http01ProxyNetworkPolicyAssets = []string{
	"networkpolicies/http01-proxy-deny-all-networkpolicy.yaml",
	"networkpolicies/http01-proxy-allow-egress-networkpolicy.yaml",
}

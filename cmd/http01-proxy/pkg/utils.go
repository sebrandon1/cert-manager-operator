package utils

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"text/template"

	"github.com/openshift/cert-manager-operator/cmd/http01-proxy/templates"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	yamlserializer "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var (
	infrastructureGVR       = schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "infrastructures"}
	ingressGVR              = schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "ingresses"}
	clusterVersionGVR       = schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "clusterversions"}
	machineConfigGVR        = schema.GroupVersionResource{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "machineconfigs"}
	machineConfigurationGVR = schema.GroupVersionResource{Group: "operator.openshift.io", Version: "v1", Resource: "machineconfigurations"}
)

// OCPEnvironment holds the discovered OpenShift cluster details needed by the proxy.
type OCPEnvironment struct {
	APIHostname    string
	AppsVIP        string
	APIVIP         string
	PlatformType   string
	ClusterVersion string
	Client         *dynamic.DynamicClient
}

func NewReverseProxy(targetURL string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target URL %q: %w", targetURL, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Header.Set("X-Proxy-Server", "cert-manager-http01-proxy")
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		log.Printf("Error handling request: %v", err)
		http.Error(w, "Backend unavailable", http.StatusBadGateway)
	}

	return proxy, nil
}

func newKubeClient() (*dynamic.DynamicClient, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return client, nil
}

func GetOCPEnvDetails(ctx context.Context) (*OCPEnvironment, error) {
	client, err := newKubeClient()
	if err != nil {
		return nil, err
	}

	infrastructureData, err := client.Resource(infrastructureGVR).Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get infrastructure: %w", err)
	}
	platformType, found, err := unstructured.NestedString(infrastructureData.Object, "status", "platform")
	if err != nil {
		return nil, fmt.Errorf("failed to read platform type: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("platform type not found in infrastructure status")
	}

	ingressData, err := client.Resource(ingressGVR).Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ingress: %w", err)
	}
	ingressDomain, found, err := unstructured.NestedString(ingressData.Object, "spec", "domain")
	if err != nil {
		return nil, fmt.Errorf("failed to read ingress domain: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("ingress domain not found in ingress spec")
	}

	clusterVersionData, err := client.Resource(clusterVersionGVR).Get(ctx, "version", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get clusterversion: %w", err)
	}
	clusterVersion, found, err := unstructured.NestedString(clusterVersionData.Object, "status", "desired", "version")
	if err != nil {
		return nil, fmt.Errorf("failed to read cluster version: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("desired version not found in clusterversion status")
	}

	suffix, ok := strings.CutPrefix(ingressDomain, "apps.")
	if !ok {
		return nil, fmt.Errorf("ingress domain %q does not start with \"apps.\"", ingressDomain)
	}
	apiHostname := "api." + suffix

	// Resolve a subdomain under the wildcard *.apps record to get the Apps VIP
	appsIPs, err := resolveDNSRecord(ctx, "test."+ingressDomain)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve apps domain: %w", err)
	}

	apiIPs, err := resolveDNSRecord(ctx, apiHostname)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve api hostname: %w", err)
	}

	return &OCPEnvironment{
		APIHostname:    apiHostname,
		AppsVIP:        appsIPs[0].String(),
		APIVIP:         apiIPs[0].String(),
		PlatformType:   platformType,
		ClusterVersion: clusterVersion,
		Client:         client,
	}, nil
}

func CreateNFTablesRuleMachineConfig(ctx context.Context, client *dynamic.DynamicClient, apiVIP, port string) error {
	data := templates.TemplateData{
		APIVIP:    apiVIP,
		ProxyPort: port,
	}

	tmpl, err := template.New("nftables").Parse(templates.NFTRuleTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse nftables template: %w", err)
	}
	var nftablesResult bytes.Buffer
	if err := tmpl.Execute(&nftablesResult, data); err != nil {
		return fmt.Errorf("failed to execute nftables template: %w", err)
	}
	data.NFTRules = base64.StdEncoding.EncodeToString(nftablesResult.Bytes())

	tmpl, err = template.New("machineconfig").Parse(templates.MachineConfigTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse machineconfig template: %w", err)
	}
	var machineConfigResult bytes.Buffer
	if err := tmpl.Execute(&machineConfigResult, data); err != nil {
		return fmt.Errorf("failed to execute machineconfig template: %w", err)
	}

	fieldManager := "cert-manager-http01-proxy"

	if err := applyManifest(ctx, client, []byte(templates.MachineConfigurationManifest), machineConfigurationGVR, fieldManager); err != nil {
		return err
	}

	if err := applyManifest(ctx, client, machineConfigResult.Bytes(), machineConfigGVR, fieldManager); err != nil {
		return err
	}

	return nil
}

func applyManifest(ctx context.Context, client *dynamic.DynamicClient, manifest []byte, gvr schema.GroupVersionResource, fieldManager string) error {
	decoder := yamlserializer.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	if _, _, err := decoder.Decode(manifest, nil, obj); err != nil {
		return fmt.Errorf("failed to decode %s: %w", gvr.Resource, err)
	}

	objData, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", gvr.Resource, err)
	}

	if _, err := client.Resource(gvr).Patch(
		ctx,
		obj.GetName(),
		types.ApplyPatchType,
		objData,
		metav1.PatchOptions{FieldManager: fieldManager},
	); err != nil {
		return fmt.Errorf("failed to apply %s: %w", gvr.Resource, err)
	}

	return nil
}

func SupportedOCPVersion(runningVersion string) error {
	version := strings.Split(runningVersion, ".")
	if len(version) < 3 {
		return fmt.Errorf("invalid OCP version %q (expecting X.Y.Z format)", runningVersion)
	}
	major, err := strconv.Atoi(version[0])
	if err != nil {
		return fmt.Errorf("invalid OCP major version in %q: %w", runningVersion, err)
	}
	minor, err := strconv.Atoi(version[1])
	if err != nil {
		return fmt.Errorf("invalid OCP minor version in %q: %w", runningVersion, err)
	}
	if major < 4 || (major == 4 && minor < 17) {
		return fmt.Errorf("unsupported OCP version %q (minimum supported is 4.17+)", runningVersion)
	}
	return nil
}

func resolveDNSRecord(ctx context.Context, hostname string) ([]net.IP, error) {
	var r net.Resolver
	ips, err := r.LookupIP(ctx, "ip4", hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %q: %w", hostname, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no IPs found for %q", hostname)
	}
	return ips, nil
}

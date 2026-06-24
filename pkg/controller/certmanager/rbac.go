package certmanager

// RBAC markers for the cert-manager operand components (controller, webhook,
// cainjector). These permissions are consumed by controller-gen to produce
// config/rbac/role.yaml and are not tied to a specific reconciler because the
// library-go controllers that need them do not use controller-runtime markers.

//+kubebuilder:rbac:groups=operator.openshift.io,resources=certmanagers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.openshift.io,resources=certmanagers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.openshift.io,resources=certmanagers/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=pods;secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events;services;namespaces;serviceaccounts;configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=serviceaccounts/token,verbs=create
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles;rolebindings;clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=deployments;replicasets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="config.openshift.io",resources=certmanagers;clusteroperators;clusteroperators/status;infrastructures,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="config.openshift.io",resources=apiservers,verbs=get;list;watch

//+kubebuilder:rbac:groups="cert-manager.io",resources=certificaterequests;certificaterequests/finalizers;certificaterequests/status;certificates;certificates/finalizers;certificates/status;clusterissuers;clusterissuers/finalizers;clusterissuers/status;issuers;issuers/finalizers;issuers/status,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups="certificates.k8s.io",resources=certificatesigningrequests;certificatesigningrequests/status,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="certificates.k8s.io",resources=signers,verbs=get;list;watch;create;update;patch;delete;sign
//+kubebuilder:rbac:groups="cert-manager.io",resources=signers,resourceNames=clusterissuers.cert-manager.io/*;issuers.cert-manager.io/*,verbs=approve
//+kubebuilder:rbac:groups="gateway.networking.k8s.io",resources=gateways;gateways/finalizers;httproutes;httproutes/finalizers;listenersets;listenersets/finalizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses;ingresses/finalizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apiregistration.k8s.io",resources=apiservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="acme.cert-manager.io",resources=challenges;challenges/finalizers;challenges/status;orders;orders/finalizers;orders/status,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups="route.openshift.io",resources=routes;routes/custom-host,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;list;watch;create;update

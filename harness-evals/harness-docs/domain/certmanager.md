# CertManager

**API Group**: `operator.openshift.io/v1alpha1`  
**Kind**: `CertManager`  
**Scope**: Cluster (singleton name **`cluster`**)

**API Definition**: [api/operator/v1alpha1/certmanager_types.go](https://github.com/openshift/cert-manager-operator/blob/master/api/operator/v1alpha1/certmanager_types.go)

## Purpose

Primary operator CR that configures the three cert-manager operand Deployments (`cert-manager`, `cert-manager-webhook`, `cert-manager-cainjector`) and optional NetworkPolicies. Missing `cluster` CR is auto-created by `DefaultCertManager` (`pkg/controller/certmanager/default_cert_manager_controller.go`). Controllers hardcode `Lister().Get("cluster")` — a misnamed CertManager CR is **silently ignored** (unlike TrustManager/IstioCSR, the `cluster`/`default` name is **not** CEL-enforced on CertManager).

**Key Principle**: Embeds OpenShift `OperatorSpec` / `OperatorStatus` (library-go status model), unlike IstioCSR/TrustManager which use custom `ConditionalStatus`.

## Spec Structure

Verified in `certmanager_types.go`:

```go
type CertManagerSpec struct {
    apiv1.OperatorSpec `json:",inline"` // ManagementState, LogLevel, OperatorLogLevel,
                                        // UnsupportedConfigOverrides, ObservedConfig
    ControllerConfig       *DeploymentConfig `json:"controllerConfig,omitempty"`
    WebhookConfig          *DeploymentConfig `json:"webhookConfig,omitempty"`
    CAInjectorConfig       *DeploymentConfig `json:"cainjectorConfig,omitempty"`
    DefaultNetworkPolicy   string            `json:"defaultNetworkPolicy,omitempty"` // "true"|"false"|""
    NetworkPolicies        []NetworkPolicy   `json:"networkPolicies,omitempty"`
}

type DeploymentConfig struct {
    OverrideArgs       []string
    OverrideEnv        []corev1.EnvVar
    OverrideLabels     map[string]string
    OverrideResources  CertManagerResourceRequirements // Limits, Requests
    OverrideReplicas   *int32
    OverrideScheduling CertManagerScheduling           // NodeSelector, Tolerations
}
```

**NetworkPolicy** (`Name`, `ComponentName`, `Egress`):
- `defaultNetworkPolicy`: CEL ratchet — once `"true"`, cannot go to `"false"`. Enabling applies deny-all + allow bindata; static defaults are **never deleted** on disable.
- User `networkPolicies`: `name`+`componentName` immutable; egress appendable; ingress is operator-derived (not user-configurable).
- CRD Enum allows only **`CoreController`** today, even though `getPodSelectorForComponent` already has `CAInjector`/`Webhook` cases — exposing those needs extending the Enum, not the selector.

**UnsupportedConfigOverrides** helper types (`Controller`/`Webhook`/`CAInjector` with `Args []string`) parse `OperatorSpec.UnsupportedConfigOverrides` RawExtension — not first-class Spec fields.

## Status

```go
type CertManagerStatus struct {
    apiv1.OperatorStatus `json:",inline"` // Conditions, ObservedGeneration, Versions, Generations, ...
}
```

Condition types from OpenShift operator API: `Available`, `Progressing`, `Degraded`, `PrereqsSatisfied`, `Upgradeable`.

## Lifecycle

1. **Creation**: Operator creates `certmanagers.operator.openshift.io/cluster` if absent.
2. **Update**: library-go staticresource + deployment controllers reconcile bindata assets with overrides (args/env/labels/resources/replicas/scheduling), proxy, trusted-CA, TLS profile, optional cloud-credentials mount.
3. **Deletion / uninstall**: CSV documents that operands need **manual** cleanup (`console.openshift.io/disable-operand-delete: "true"`).

## Component-Specific Behavior

| Concern | Behavior | Code |
|---------|----------|------|
| Images | Env `RELATED_IMAGE_CERT_MANAGER_{CONTROLLER,WEBHOOK,CA_INJECTOR,ACMESOLVER}` | `pkg/controller/certmanager/related_images.go` |
| Apply | library-go `resourceapply` / DeploymentController — **not** ctrl-runtime SSA | `cert_manager_*_deployment.go` |
| NetworkPolicy | Static + user-defined via `resourceapply.ApplyNetworkPolicy`; defaults never deleted once applied | `cert_manager_networkpolicy.go` |
| Placeholder | `CertManagerReconciler` in `certmanager_controller.go` is **not** started — RBAC scaffold only | `starter.go` |

**Operand CRDs** (Certificate, Issuer, ClusterIssuer, Order, Challenge) are reconciled by the **cert-manager controller** Deployment, not by this operator’s primary loop. Types live under `vendor/github.com/cert-manager/...` (replace → `openshift/jetstack-cert-manager`).

## Common Mistakes

1. Editing `config.openshift.io` CertManager CRD stub under `config/crd/bases/` — not wired in kustomization; no Go types; not the operator API.
2. Expecting SSA field ownership on CertManager-managed Deployments.
3. Setting `defaultNetworkPolicy: "false"` after `"true"` — API validation rejects it.
4. Setting user `networkPolicies[].componentName` to `CAInjector`/`Webhook` — CRD Enum rejects; only `CoreController` is allowed today.

## Related Concepts

- [IstioCSR](./istiocsr.md) | [TrustManager](./trustmanager.md) | [Architecture](../architecture/components.md)

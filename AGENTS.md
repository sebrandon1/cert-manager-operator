# Cert Manager Operator - Agentic Documentation

**Component**: Cert Manager Operator for OpenShift  
**Repository**: [openshift/cert-manager-operator](https://github.com/openshift/cert-manager-operator)  
**Default branch**: `master` | **Go**: 1.26.0

> **Retrieval-first**: Prefer `ai-docs/` for architecture/API; prefer `docs/*-guidelines.md` for deep playbooks. Platform hub: [openshift/enhancements/ai-docs](https://github.com/openshift/enhancements/tree/master/ai-docs).

## What is Cert Manager Operator?

Deploys/configures upstream **cert-manager** (and optionally **istio-csr** / **trust-manager**) on OpenShift. Operator NS `cert-manager-operator`; operand NS `cert-manager` (hardcoded).

**Key Principle**: Dual-stack — library-go for CertManager; controller-runtime for IstioCSR/TrustManager. **Apply methods differ** (do not assume SSA everywhere).

## Core Components

| CR | Scope / name | Stack | Gate |
|----|--------------|-------|------|
| [CertManager](ai-docs/domain/certmanager.md) | Cluster / `cluster` | library-go `resourceapply` | Always on |
| [IstioCSR](ai-docs/domain/istiocsr.md) | Namespaced / `default` | Create+Update | `IstioCSR` GA default true |
| [TrustManager](ai-docs/domain/trustmanager.md) | Cluster / `cluster` | **SSA** | `TrustManager` TP default false |

**Operands**: Certificate/Issuer/… → cert-manager; Bundle → trust-manager.  
**Quick Start**: `oc get certmanager cluster -o yaml` | `make local-run`

## Critical Patterns

1. **Never assume uniform SSA** — CertManager library-go; IstioCSR Create+`UpdateWithRetry`; TrustManager only SSA + field owner `trust-manager-controller`. [components.md](ai-docs/architecture/components.md)
2. **Never hand-edit generated assets** — bindata.go, clients, regenerated `bindata/` / `bundle/` (`make generate`, `update-manifests`, `bundle`, `verify-bindata`).
3. **Greenfield = TrustManager** — copy SSA + `HandleReconcileResult`/`FromClientError` (30s requeue); not IstioCSR. [ADR-0002](ai-docs/decisions/adr-0002-apply-strategies.md)
4. **Feature gates** — `--unsupported-addon-features`; five touchpoints; no cluster FeatureSet for TrustManager (CM-1141). [ADR-0003](ai-docs/decisions/adr-0003-feature-gates.md)
5. **Ignore** `certmanager_controller.go` placeholder — RBAC markers only, never started.
6. **RELATED_IMAGE triple-sync** — manager.yaml env ↔ controller constants ↔ CSV `relatedImages` + `make bundle`.
7. **Cache** — no label-filtered ConfigMap/Issuer caches; use predicates when unlabeled watches are required.
8. **TLS 1.3** — strip cipher args via `StripArgsByKeys`; nil profile = Intermediate; Infrastructure discovery gates TLS + cloud-cred hooks (APIServer shares that factory, not discovered separately).

## Key Paths

| Area | Path |
|------|------|
| Startup | `pkg/operator/starter.go` → CertManager set → optional ctrl-runtime manager |
| Shared | `pkg/controller/common` (`CtrlClient`, errors, TLS/validation) |
| Features | `api/operator/v1alpha1/features.go`, `pkg/features` |
| Images | `RELATED_IMAGE_CERT_MANAGER_{CONTROLLER,WEBHOOK,CA_INJECTOR,ACMESOLVER,ISTIOCSR,TRUST_MANAGER}` |
| ADRs | [0001](ai-docs/decisions/adr-0001-dual-controller-frameworks.md) · [0002](ai-docs/decisions/adr-0002-apply-strategies.md) · [0003](ai-docs/decisions/adr-0003-feature-gates.md) |

## Documentation Map

```text
ai-docs/     # architecture, domain CRDs, ADRs, DEVELOPMENT, TESTING
docs/*-guidelines.md  # deep agent playbooks (controllers, errors, OLM, FIPS, …)
docs/{proxy,cloud_credentials,operand_metrics}.md
```

| Need | Start here |
|------|------------|
| Spec / CEL / NetworkPolicy enum | `ai-docs/domain/*.md` |
| Errors, cache, TLS, cloud, FIPS | `ai-docs/architecture/components.md` |
| Controllers / apply / gates | `docs/operator-controllers-guidelines.md` |
| OLM / relatedImages | `docs/olm-packaging-guidelines.md` + DEVELOPMENT |
| Unit assert Patch vs Update | `ai-docs/CERT_MANAGER_OPERATOR_TESTING.md` |
| FIPS build rules | `docs/fips-guidelines.md` |

**AI Agent Path**: domain → components.md → matching `docs/*-guidelines.md` → DEVELOPMENT/TESTING

**Also**: [DEVELOPMENT](ai-docs/CERT_MANAGER_OPERATOR_DEVELOPMENT.md) · [TESTING](ai-docs/CERT_MANAGER_OPERATOR_TESTING.md) · [enhancements](ai-docs/references/enhancements.md) · [ecosystem](ai-docs/references/ecosystem.md)

**Guideline index**: `docs/{operator-controllers,error-handling,api-contracts,olm-packaging,integration,security,testing,fips}-guidelines.md`

**Platform**: [hub](https://github.com/openshift/enhancements/tree/master/ai-docs) · [operator-patterns](https://github.com/openshift/enhancements/tree/master/ai-docs/platform/operator-patterns) · [status-conditions](https://github.com/openshift/enhancements/blob/master/ai-docs/platform/operator-patterns/status-conditions.md)

## External References

- [Product docs](https://docs.openshift.com/container-platform/latest/security/cert_manager_operator/index.html)
- [Upstream cert-manager](https://cert-manager.io/docs/)
- [README](README.md) · [docs/proxy.md](docs/proxy.md) · [docs/cloud_credentials.md](docs/cloud_credentials.md) · [docs/operand_metrics.md](docs/operand_metrics.md)

---

**Platform Documentation**: [openshift/enhancements/ai-docs](https://github.com/openshift/enhancements/tree/master/ai-docs)

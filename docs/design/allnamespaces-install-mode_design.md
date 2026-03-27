# AllNamespaces Install Mode for Conversion Webhooks

Date: 2026-03-27
Related: [OADP-3379](https://redhat.atlassian.net/browse/OADP-3379)

## Abstract

OLM requires `AllNamespaces` as the only supported install mode when a CSV declares conversion webhooks (`conversionCRDs`).
OADP currently only supports `OwnNamespace`.
This document lays out why the change is needed, what it affects, and what it doesn't affect.

## Background

OADP-3379 introduces a `v1alpha2` API version for the DPA CRD to change duration fields from `*time.Duration` (nanosecond integers) to `*metav1.Duration` (human-readable strings like `"2h"`).
Serving two versions of the same CRD with different field types requires a conversion webhook.

When `operator-sdk generate bundle` detects a CRD with `spec.conversion.strategy: Webhook`, it automatically adds `webhookdefinitions` with `conversionCRDs` to the CSV.
OLM then enforces that `AllNamespaces` is the **only** supported install mode, because CRDs and their conversion webhooks are cluster-scoped resources.

This is not an OADP-specific constraint — any OLM-managed operator that adds a CRD conversion webhook must support AllNamespaces.

## What Changes

### CSV Install Modes

```yaml
# Before
installModes:
- supported: true    # ← currently the only mode
  type: OwnNamespace
- supported: false
  type: AllNamespaces

# After
installModes:
- supported: false
  type: OwnNamespace
- supported: false
  type: SingleNamespace
- supported: false
  type: MultiNamespace
- supported: true    # ← now the only mode
  type: AllNamespaces
```

### OperatorGroup

OLM creates an OperatorGroup to scope the operator.
With AllNamespaces, the OperatorGroup must not have `spec.targetNamespaces` (empty = all namespaces).

The `make deploy-olm` target in the Makefile creates an OperatorGroup and needs updating to remove `targetNamespaces`.

### CI Configuration (openshift/release repo)

The Prow CI test infrastructure in `openshift/release` creates an OperatorGroup with `targetNamespaces: ["openshift-adp"]` (OwnNamespace scoping) before running E2E tests.
This must be changed to a global OperatorGroup (no `targetNamespaces`) to match AllNamespaces mode.

This is a **cross-repo change** that must be coordinated with the OADP CSV change.

### WATCH_NAMESPACE Handling (cmd/main.go)

In AllNamespaces mode, OLM sets `WATCH_NAMESPACE` from the pod annotation `olm.targetNamespaces`, which is empty for AllNamespaces.
The operator currently requires `WATCH_NAMESPACE` to be non-empty for:

1. **PSA label patching** — patches the operator namespace with `pod-security.kubernetes.io/enforce: privileged`
2. **Cache scoping** — `cache.Options.DefaultNamespaces` is set to `watchNamespace`
3. **CLI download setup** — creates resources in `watchNamespace`

A fallback is needed: when `WATCH_NAMESPACE` is empty, the operator reads its own namespace from `/var/run/secrets/kubernetes.io/serviceaccount/namespace` (standard Kubernetes downward API, always available in a pod).

The operator still only watches and creates resources in its own namespace — the AllNamespaces install mode changes what OLM *allows*, not what the operator *does*.

## What Does NOT Change

### Runtime Behavior

The operator continues to:
- Watch only its own namespace (via `operatorNamespace` fallback)
- Create Velero, node-agent, and all resources in that namespace
- Scope its cache to that namespace
- Reconcile DPAs only in that namespace

### RBAC

OADP already uses `ClusterRole` and `ClusterRoleBinding` in OwnNamespace mode.
AllNamespaces mode does not change the RBAC configuration.

### Console Install Experience

The OpenShift console shows "All namespaces on the cluster" as the installation mode.
However, the namespace picker still appears under "Installed Namespace" and defaults to `openshift-adp` because the CSV has:

```yaml
annotations:
  operatorframework.io/suggested-namespace: openshift-adp
```

Users still install OADP into `openshift-adp` — the experience is functionally the same.

### Security Posture

OADP already manages cluster-scoped resources (SCCs, CRDs, ClusterRoleBindings, routes).
The AllNamespaces install mode does not grant additional permissions beyond what already exists.

## Pros

- **Enables CRD API evolution** — the operator can introduce new API versions with type changes, which is standard Kubernetes practice.
- **OLM-managed webhook certs** — OLM handles TLS cert generation, rotation, and injection. No custom cert management code needed.
- **Architecturally correct** — follows the kubebuilder/controller-runtime multi-version CRD pattern without workarounds.
- **Future-proof** — any future API version changes (v1beta1, v1) will also require conversion webhooks.

## Cons

- **Install mode is a visible change** — even though behavior is unchanged, the CSV declares a different mode. This may require documentation updates and customer communication.
- **Cross-repo CI coordination** — the `openshift/release` repo must update the OperatorGroup in test configs, coordinated with the OADP CSV change.
- **Multiple OADP instances per cluster no longer possible** — OwnNamespace allows (in theory) separate OADP installations in different namespaces. AllNamespaces restricts to one global OperatorGroup, so only one OADP installation per cluster. In practice, this is already the expected deployment model.
- **New code path for WATCH_NAMESPACE** — the `getOperatorNamespace()` fallback is new code that needs testing, though it uses a standard Kubernetes mechanism.
- **Upgrade is not seamless** — existing installations have an OperatorGroup with `spec.targetNamespaces: ["openshift-adp"]` (OwnNamespace scoping). The new CSV requires AllNamespaces, which is incompatible with this OperatorGroup. On upgrade, OLM will fail the new CSV with `UnsupportedOperatorGroup`. Users must manually remove `spec.targetNamespaces` from their OperatorGroup before the upgrade can succeed. This requires clear release notes and documentation, and means the upgrade cannot be fully automatic.

## Alternatives Considered

### Self-managed webhook certs (hide from OLM)

The operator generates its own TLS certs, writes them to disk, stores them in a Secret, and patches the CRD conversion config at runtime — all without declaring `webhookdefinitions` in the CSV.

**Rejected because**: This is fighting the framework. It adds operational complexity (custom cert generation, CRD patching at runtime, no cert rotation), and OLM is unaware of the webhook, which breaks the declarative model.

### Custom type accepting both integers and strings (no new API version)

A custom `DurationString` type with `+kubebuilder:validation:Type=""` in v1alpha1 that accepts both nanosecond integers and duration strings.

**Rejected because**: Disabling schema type enforcement (`Type=""`) means the API server accepts any value for these fields — strings, objects, arrays, booleans. Validation moves entirely to the Go code, which runs after admission. This weakens the API contract and is not standard practice for Kubernetes CRDs.

### Document the unsupported-args workaround

Users can already pass string durations via the `oadp.openshift.io/unsupported-velero-server-args` ConfigMap annotation.

**Rejected as a long-term solution because**: It replaces all server args (not just the targeted field), bypasses CRD validation, is not discoverable, and is explicitly labeled "unsupported."

## Implementation Steps

1. **OADP repo** — CSV install mode change, main.go WATCH_NAMESPACE fallback, Makefile OperatorGroup update
2. **OADP repo** — v1alpha2 API types, conversion webhook, webhook infrastructure
3. **openshift/release repo** — Update OperatorGroup in CI test configs (coordinated PR)
4. **Documentation** — Update install guides to reflect AllNamespaces mode

## Open Questions

- Is there precedent for other Red Hat operators switching from OwnNamespace to AllNamespaces? If so, what was the customer communication approach?
- Should the AllNamespaces change be made in a separate release from the v1alpha2 API addition to reduce risk?
- Does the Red Hat certification process have specific requirements around install mode changes?

# Enable AllNamespaces Install Mode for OADP Operator

## Abstract

- Enable `AllNamespaces` install mode alongside the existing `OwnNamespace` mode in a single CSV.
- The operator's runtime behavior is unchanged — it watches only the namespace it is deployed in.
- Two CSV metadata changes and one minimal Go code change are required.

## Background

### OLMv0 — Current model

OLMv0 uses a five-resource model:

- **`CatalogSource`** — where to find operators (catalog image)
- **`OperatorGroup`** — which namespaces the operator watches + RBAC generation
- **`Subscription`** — install request
- **`InstallPlan`** — auto-generated execution plan
- **`ClusterServiceVersion` (CSV)** — the operator's metadata, permissions, and deployment spec

OLMv0 supports four install modes: `AllNamespaces`, `OwnNamespace`, `SingleNamespace`, and `MultiNamespace`. This multi-tenancy model allowed multiple independent operator instances in different namespaces, each watching its own scope. OADP currently uses `OwnNamespace`, meaning the operator only watches CRs in the namespace where it is installed.

### OLMv1 — The new model

OLMv1 is not a version bump — it is a ground-up redesign that consolidates the five OLMv0 resources into two:

- **`ClusterCatalog`** — replaces `CatalogSource`
- **`ClusterExtension`** — replaces `OperatorGroup` + `Subscription` + `InstallPlan` + `CSV`

It is declarative, GitOps-friendly, and uses a cluster-admin security model instead of namespace-scoped ServiceAccount RBAC.

**OLMv1 GA only supports AllNamespaces mode.** The rationale, as documented in the [OLMv1 single/own namespace enhancement proposal](https://github.com/openshift/enhancements/pull/1849):

1. **CRDs are cluster-scoped singletons.** Only one definition of a CRD can exist per cluster. OLMv0's multi-tenancy promise — that multiple operator instances in different namespaces could each own their own CRDs — was architecturally flawed.
2. **Dependency resolution requires a global view.** OLMv1's explicit dependency model cannot work at namespace scope.
3. **PSA and security are simpler at cluster scope.** Managed platforms (ROSA, OSD) operate cluster-admin anyway.

`OwnNamespace` and `SingleNamespace` were added as Tech Preview in OCP 4.19 ([OCPSTRAT-1711](https://issues.redhat.com/browse/OCPSTRAT-1711)) behind the `TechPreviewNoUpgrade` feature gate — a cluster-wide toggle that, while enabled, blocks upgrading to the next OCP minor version:

- OCP 4.21 release notes explicitly state these modes "continued as a Technology Preview feature" and are "not recommended for production use."
- A planned GA promotion was subsequently reversed. [OPRUN-4514](https://issues.redhat.com/browse/OPRUN-4514) ("Revert Single/Own Namespace promotion to GA") explicitly states the goal was to return the feature to `TPNU` status; the upstream revert is [operator-controller#2568](https://github.com/operator-framework/operator-controller/pull/2568).

`OwnNamespace` and `SingleNamespace` are not a viable option for production operators.

### OADP Operator

Against this backdrop, OADP today is installed via OLM with only `OwnNamespace` install mode supported. At runtime, the `WATCH_NAMESPACE` environment variable controls which namespace the controller-runtime cache monitors.

Two distinct pieces of information are at play, set by different actors:

- **`olm.targetNamespaces`** — a pod annotation set by OLM at install time, based on the OperatorGroup. In OwnNamespace mode this is `openshift-adp`; in AllNamespaces mode this is `""` (empty). Per the [OLM OperatorGroup design](https://github.com/operator-framework/operator-lifecycle-manager/blob/master/doc/design/operatorgroups.md), the empty string is the intentional signal for "watch all namespaces" — *"the consuming operator must know to treat `""` as an all namespace configuration."*
- **`metadata.namespace`** — a standard Kubernetes pod field set by Kubernetes itself. Always the pod's actual namespace (`openshift-adp`), regardless of install mode or OperatorGroup.

In OwnNamespace mode these happen to be the same value, which is why the distinction hasn't mattered until now.

`WATCH_NAMESPACE` is currently sourced from `olm.targetNamespaces`. In AllNamespaces mode it becomes `""`, which breaks PSA labeling, STS credential flow, CLI/VMDP setup, and the cache configuration — all of which need to know *where the operator lives*, not just *what it watches*.

To understand why this can't be fixed purely at the CSV level, it helps to trace what happens at each phase:

- **Bundle time**: `operator-sdk generate bundle` generates the CSV from the manager deployment spec. The SDK's `setNamespacedFields` function ([source](https://github.com/operator-framework/operator-sdk/blob/299157a814e674f8f40d1b91ee77d64a90e850de/internal/generate/clusterserviceversion/clusterserviceversion_updaters.go)) unconditionally rewrites any env var named `WATCH_NAMESPACE` to source from `olm.targetNamespaces`. This is hardcoded and cannot be disabled.
- **Install time**: OLM deploys the operator pod and sets `olm.targetNamespaces` based on the OperatorGroup — `openshift-adp` in OwnNamespace mode, `""` in AllNamespaces mode.
- **Runtime**: The kubelet resolves the fieldRef. `WATCH_NAMESPACE` is `""` under AllNamespaces.

The fix has two parts: a new `OPERATOR_NAMESPACE` env var (sourced from `metadata.namespace`, which the SDK does not rewrite) gives the operator its own namespace. A minimal Go code change reads `WATCH_NAMESPACE` and, if empty, falls back to `OPERATOR_NAMESPACE` for the watch scope — aligning with OLM's stated contract that operators must handle `""` themselves.

Additionally, OLM requires every ServiceAccount declared in `clusterPermissions` to have a corresponding `permissions` (namespace-scoped Role) entry when running in AllNamespaces mode. Two of the three OADP ServiceAccounts currently lack this entry.

### OLMv1 Alignment

AllNamespaces is the strategically correct direction:

- OLMv1 GA (OCP 4.18) shipped with AllNamespaces-only support.
- OwnNamespace was added as Tech Preview in OCP 4.19 ([OCPSTRAT-1711](https://issues.redhat.com/browse/OCPSTRAT-1711)) and remains TP.
- Enabling AllNamespaces now positions OADP for OLMv1 readiness without requiring disruptive changes to runtime behavior — the operator continues watching only its own namespace.

> **Note:** AllNamespaces here refers to the install mode (how OLM deploys the operator), not the operator's watch scope.

## Goals

- Enable `AllNamespaces` install mode alongside `OwnNamespace` in the same CSV.
- Keep runtime behavior identical: the operator watches only the namespace it is deployed in.
- Maintain full backward compatibility for existing `OwnNamespace` installations.

## Non Goals

- Actually watching all namespaces or supporting multi-tenant Velero (one DPA per namespace).
- `SingleNamespace` or `MultiNamespace` install mode support.

## High-Level Design

Two CSV metadata changes and one minimal Go code change:

| # | Change | Why |
|---|---|---|
| 1 | Enable `AllNamespaces: true` in `installModes` | Allow installation with a global OperatorGroup |
| 2 | Add `OPERATOR_NAMESPACE` env var (from `metadata.namespace`) + Go fallback | `WATCH_NAMESPACE` is `""` in AllNamespaces mode by OLM design; operator needs its own namespace separately |
| 3 | Add `permissions` entries for `non-admin-controller` and `velero` SAs | OLM requires these to create the ServiceAccounts in AllNamespaces mode |

The RBAC `clusterPermissions` and all runtime behavior remain identical.

**How the operator resolves its namespace in each mode:**

| OperatorGroup | `WATCH_NAMESPACE` (`olm.targetNamespaces`) | `OPERATOR_NAMESPACE` (`metadata.namespace`) | Operator watches |
|---|---|---|---|
| Namespaced (OwnNamespace) | `openshift-adp` | `openshift-adp` | `openshift-adp` (from `WATCH_NAMESPACE`) |
| Global (AllNamespaces) | `""` (empty — OLM's all-namespaces signal) | `openshift-adp` | `openshift-adp` (fallback to `OPERATOR_NAMESPACE`) |

## Detailed Design

### Change 1: Enable AllNamespaces Install Mode

```yaml
installModes:
- supported: true
  type: OwnNamespace
- supported: false
  type: SingleNamespace
- supported: false
  type: MultiNamespace
- supported: true      # ← changed from false
  type: AllNamespaces
```

Adding installMode support is a safe superset change in OLM — it never blocks upgrades. Existing customers with a namespaced OperatorGroup are unaffected; their install mode stays OwnNamespace.

### Change 2: OPERATOR_NAMESPACE env var + Go fallback

Add a new `OPERATOR_NAMESPACE` env var to the manager deployment, sourced from `metadata.namespace`:

```yaml
- name: OPERATOR_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.namespace
- name: WATCH_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.annotations['olm.targetNamespaces']  # unchanged; SDK manages this
```

`WATCH_NAMESPACE` is left sourcing from `olm.targetNamespaces` — the SDK's `setNamespacedFields` function rewrites any env var named `WATCH_NAMESPACE` to this unconditionally ([source](https://github.com/operator-framework/operator-sdk/blob/299157a814e674f8f40d1b91ee77d64a90e850de/internal/generate/clusterserviceversion/clusterserviceversion_updaters.go)), so fighting it with a post-gen patch is fragile. Instead, `OPERATOR_NAMESPACE` uses a different name and is not touched by the SDK.

The Go code change reads both vars and falls back when `WATCH_NAMESPACE` is empty:

```go
watchNamespace := os.Getenv("WATCH_NAMESPACE")
if watchNamespace == "" {
    watchNamespace = os.Getenv("OPERATOR_NAMESPACE")
}
```

This is the approach the [OLM OperatorGroup design](https://github.com/operator-framework/operator-lifecycle-manager/blob/master/doc/design/operatorgroups.md) expects: `""` is OLM's intentional all-namespaces signal, and *"the consuming operator must know to treat `""` as an all namespace configuration"* — or in OADP's case, fall back to its own namespace rather than watching everything.

Under OwnNamespace mode, `WATCH_NAMESPACE` is `openshift-adp` and the fallback is never reached — fully backward-compatible.

### Change 3: Add Permissions for Missing ServiceAccounts

The CSV declares `clusterPermissions` for three SAs but only `openshift-adp-controller-manager` has a `permissions` entry. Add `permissions` entries for `non-admin-controller` and `velero` with leader-election rules (configmaps, leases, events).

- The `velero` SA does not actually perform leader election — these are placeholder rules required solely to satisfy OLM's SA creation requirement.
- In AllNamespaces mode, OLM promotes these namespace-scoped Role/RoleBinding entries to ClusterRoles/ClusterRoleBindings (via its internal `ensureSingletonRBAC` reconciler, which merges all operator permissions into a single cluster-scoped set).
- In OwnNamespace mode, they remain namespace-scoped Roles/RoleBindings — no change to existing behavior.

### OLM Behavior in AllNamespaces Mode

These OLM behaviors do not affect operator functionality but should be understood:

- **CSV copies**: OLM copies the CSV to every namespace. On large clusters, `OLMConfig.spec.features.disableCopiedCSVs: true` disables this.
- **CRD ownership**: The operator globally owns its CRDs. This is a pre-existing constraint of OADP's cluster-scoped CRD ownership — customers running standalone upstream Velero alongside OADP would hit `InterOperatorGroupOwnerConflict` regardless of install mode.
- **Dual installation prevention**: OLM prevents installing the operator in two namespaces with overlapping OperatorGroups.

### OperatorHub User Experience

When both install modes are enabled, OperatorHub (OpenShift 4.14+) presents install mode radio buttons and a namespace dropdown:

- The existing `suggested-namespace: openshift-adp` annotation defaults the namespace to `openshift-adp` in both modes.
- For fresh AllNamespaces installs, the Console automatically creates the namespace, a global OperatorGroup, and a Subscription.

This follows the pattern used by the Loki Operator (`openshift-operators-redhat`) and OpenShift Serverless (`openshift-serverless`).

## Implementation

| Phase | Scope | Risk | Depends on |
|---|---|---|---|
| 1. CSV changes | installModes, WATCH_NAMESPACE source, permissions | Low | None |
| 2. Deploy + CI | Makefile target + Prow job with existing e2e suite | Low | Phase 1 |
| 3. AllNamespaces e2e tests | AllNamespaces-specific test scenarios | Medium | Phase 2 |
| 4. Migration documentation | OperatorGroup swap procedure | Low | Phase 3 |

### Phase 1: CSV + Go Changes

- Add `OPERATOR_NAMESPACE` env var to `config/manager/manager.yaml`.
- Add the `WATCH_NAMESPACE` empty-string fallback to the Go startup code.
- Enable `AllNamespaces: true` in `installModes`.
- Add `permissions` entries for `non-admin-controller` and `velero` SAs.
- Run `make bundle` to regenerate — no post-gen patch needed.

### Phase 2: Deploy + CI for AllNamespaces

- Add a `deploy-olm-allnamespaces` Makefile target using `operator-sdk run bundle --install-mode AllNamespaces`.
- Add a Prow presubmit job that deploys with this target and runs the existing e2e suite unchanged.
- This gives immediate signal before writing any new test code.

### Phase 3: AllNamespaces Install E2E Tests

Test scenarios specific to AllNamespaces behavior:

- **AllNamespaces fresh install**: verify CSV Succeeded, `WATCH_NAMESPACE` = `""`, `OPERATOR_NAMESPACE` = pod namespace, DPA reconciles, Velero deploys.
- **OwnNamespace to AllNamespaces migration**: install OwnNamespace, swap OperatorGroup to global, verify operator re-deploys and existing DPA continues functioning.
- **AllNamespaces to OwnNamespace rollback**: reverse the migration, verify operator recovers.
- **Upgrade to dual-mode CSV**: upgrade from a prior version (OwnNamespace-only) to the new version, verify the existing namespaced OperatorGroup continues to work.
- **Upgrade test parameterization**: the existing upgrade test hardcodes a namespaced OperatorGroup — parameterize to also test with a global OperatorGroup.

### Phase 4: Migration Documentation

Since both modes coexist in the same CSV, migration is a single step: swap the OperatorGroup.

1. Delete the existing namespaced OperatorGroup.
2. Create a global OperatorGroup in `openshift-adp` (`spec: {}`).
3. OLM re-deploys the operator. Behavior is identical.

Notes:
- Rollback is the reverse. No CSV downgrade is needed.
- Brief operator unavailability occurs during the swap (seconds to ~1 minute).
- In-flight backups or restores should be completed before migration.

## Upgrade Path

- Upgrading from a prior OADP version (OwnNamespace-only CSV) to the version with this change is seamless.
- Adding `AllNamespaces: true` is a safe superset change — OLM does not reject the upgrade.
- The customer's existing namespaced OperatorGroup continues to work.
- No customer action is required unless they want to switch to AllNamespaces mode.

## Alternatives Considered

### Two separate channels (one per install mode)

Rejected:
- A single CSV supports both install modes — OLM uses the OperatorGroup to determine the active mode.
- Two channels would require separate bundles, kustomize overlays, catalog changes, and cross-channel update graphs.
- It would also create an OLM upgrade deadlock if a future release dropped OwnNamespace support.

### Two separate OLM packages

Rejected:
- Requires customers to uninstall and reinstall to migrate.
- The single-CSV approach requires only an OperatorGroup swap.

### Rewrite WATCH_NAMESPACE source in the CSV via post-gen patch

Rejected:
- `operator-sdk generate bundle` unconditionally rewrites any `WATCH_NAMESPACE` env var to source from `olm.targetNamespaces` ([source](https://github.com/operator-framework/operator-sdk/blob/299157a814e674f8f40d1b91ee77d64a90e850de/internal/generate/clusterserviceversion/clusterserviceversion_updaters.go)). A post-gen `sed` patch to restore `metadata.namespace` would be silently clobbered on every `make bundle` run unless the patch is also part of the target — and would need to survive `bundle-isupdated` CI checks.
- The OLM OperatorGroup design explicitly defines `""` as the all-namespaces signal and places the handling responsibility on the operator. Fighting the SDK to avoid a three-line Go change is the wrong trade-off.

## Security Considerations

- The `velero` SA's ClusterRole grants near-cluster-admin permissions (`apiGroups: ['*'], resources: ['*']`). These are bound via `ClusterRoleBinding`, which is cluster-scoped regardless of install mode. The `OwnNamespace` OperatorGroup does not restrict these permissions — the RBAC posture is identical in both modes.
- The new `permissions` entries add minimal RBAC (configmaps, leases, events) and are strictly less permissive than the existing `clusterPermissions` for those SAs. In AllNamespaces mode, OLM promotes these to ClusterRoles.
- A security review of the velero SA permissions is recommended as a general hygiene item, independent of this change.

## Compatibility

- Existing OwnNamespace installations are fully backward-compatible. No OperatorGroup change required.
- Upgrade from prior releases does not alter the install mode.
- Migration to AllNamespaces is optional and requires an explicit OperatorGroup swap.
- Under OwnNamespace, `metadata.namespace` and `olm.targetNamespaces` resolve to the same value.
- Minimum OpenShift version for AllNamespaces namespace selection in OperatorHub: 4.14.

## Open Issues

1. **Target OADP release**: Which version will include this change?

## Validation

Tested on OpenShift 4.22.0-ec.3 (2026-08-24) with the final implementation (`OPERATOR_NAMESPACE` + Go fallback, no post-gen patch).

| Test | Mode | Result |
|---|---|---|
| CSV install | AllNamespaces | PASS |
| CSV install | OwnNamespace | PASS |
| `WATCH_NAMESPACE` value at runtime | AllNamespaces | `""` (correct — OLM's signal) |
| `OPERATOR_NAMESPACE` value at runtime | AllNamespaces | `openshift-adp` (correct) |
| PSA labeling (`patching operator namespace`) | AllNamespaces | PASS |
| Manager started, leader election acquired | AllNamespaces | PASS |
| `make bundle` idempotent (no post-gen patch) | — | PASS |

## Future Enhancements

### Cluster-Wide Watching

If a future requirement is for the operator to watch all namespaces, `WATCH_NAMESPACE` would need to be empty. This requires:

- A separate `OPERATOR_NAMESPACE` env var so the operator knows its own namespace for PSA labeling, STS, CLI/VMDP setup.
- Changes to the cache configuration.
- A design decision on DPA singleton scope (one globally or one per namespace).

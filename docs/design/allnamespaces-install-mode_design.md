# AllNamespaces Install Mode for OADP Operator

## Abstract

OADP operator currently supports only `OwnNamespace` install mode via OLM.

This proposal enables `AllNamespaces` install mode in the same CSV by enabling both install modes simultaneously and changing the `WATCH_NAMESPACE` source from `olm.targetNamespaces` to `metadata.namespace`.

The operator's runtime behavior remains identical: it watches only the namespace it is deployed in, regardless of install mode.
No Go code changes are required.

## Background

The OADP operator is installed via OLM with a strict `OwnNamespace` install mode.
The CSV declares only `OwnNamespace: true` and all other modes are `supported: false`.

At runtime, the `WATCH_NAMESPACE` environment variable controls which namespace the controller-runtime cache monitors.
In the OwnNamespace CSV, `WATCH_NAMESPACE` is sourced from the `olm.targetNamespaces` annotation, which OLM sets to the operator's own namespace.
In the non-OLM deployment (`config/manager/manager.yaml`), `WATCH_NAMESPACE` is sourced from `metadata.namespace` (the pod's own namespace via downward API).

Two things blocked AllNamespaces support:

1. **`WATCH_NAMESPACE` sourced from `olm.targetNamespaces`** — in AllNamespaces mode, `olm.targetNamespaces` is empty, which would break PSA labeling, STS credential flow, CLI/VMDP setup, and the controller-runtime cache configuration.
2. **Missing namespace-scoped `permissions` entries** — OLM requires every ServiceAccount declared in `clusterPermissions` to also have a `permissions` (namespace-scoped Role) entry when running in AllNamespaces mode. Without this, OLM refuses to create the ServiceAccounts and the CSV stays `Pending` with `"no owned roles found"`.

Both blockers are CSV metadata issues, not Go code issues.

### Validated on Cluster

The single-CSV approach was tested on OpenShift 4.22.0-ec.3 (2026-08-13).
Both OwnNamespace and AllNamespaces install modes were validated with the same bundle.
See [full test log](https://hackmd.io/MVRrs4zTTHiwGcxfRUwhBA) for the chronological record.

Key results:
- CSV reached `Succeeded` in both install modes
- `WATCH_NAMESPACE` resolved to `openshift-adp` in both modes
- DPA reconciliation, Velero deployment, and BSL creation worked identically in both modes
- All controllers started cleanly (DPA, CloudStorage, DataProtectionTest, CLI/VMDP downloads)
- The `operatorframework.io/suggested-namespace: openshift-adp` annotation (already in the CSV) causes OperatorHub to default to `openshift-adp` even in AllNamespaces mode (OpenShift 4.14+)

RBAC is already cluster-scoped (ClusterRoles and ClusterRoleBindings), so no fundamental RBAC changes are required.
No webhooks are currently enabled, but this opens the door to having conversion webhooks, allowing us to have new CRD versions without breaking changes.
A `ClusterWideClient` (uncached) already exists for cross-namespace DPA validation.

## Goals

- Enable `AllNamespaces` install mode alongside `OwnNamespace` in the same CSV.
- Keep runtime behavior identical to today: the operator watches only the namespace it is deployed in.
- Maintain full backward compatibility for existing `OwnNamespace` installations (no OperatorGroup change required for existing customers).

## Non Goals

- Actually watching all namespaces. The operator continues to watch only its own namespace. Cluster-wide watching can be enabled in the future by setting `WATCH_NAMESPACE` to empty, but that is a separate enhancement requiring additional work (see Future Enhancements).
- Multi-tenant Velero (one DPA per namespace). Global DPA singleton enforcement will be maintained.
- `SingleNamespace` or `MultiNamespace` install mode support.

## High-Level Design

Three changes to the CSV:

1. Enable `AllNamespaces: true` in `installModes` (alongside the existing `OwnNamespace: true`).
2. Change `WATCH_NAMESPACE` source from `metadata.annotations['olm.targetNamespaces']` to `metadata.namespace`.
3. Add `permissions` (namespace-scoped Roles) entries for `non-admin-controller` and `velero` ServiceAccounts.

No Go code changes, no new channels, no kustomize overlays, no catalog restructuring.
Existing OwnNamespace installations are unaffected — under a namespaced OperatorGroup, `metadata.namespace` and `olm.targetNamespaces` resolve to the same value.

## Detailed Design

### Change 1: Enable AllNamespaces Install Mode

In `config/manifests/bases/oadp-operator.clusterserviceversion.yaml`:

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

This allows the CSV to be installed with either a namespaced OperatorGroup (OwnNamespace) or a global OperatorGroup (AllNamespaces).
Existing customers with a namespaced OperatorGroup are unaffected — their install mode stays OwnNamespace.

Adding installMode support is a safe superset change in OLM.
The reverse (removing a previously-supported installMode) would block upgrades, but adding support never does.

### Change 2: WATCH_NAMESPACE Source

The `WATCH_NAMESPACE` env var in the CSV deployment spec must be changed from `olm.targetNamespaces` to `metadata.namespace`.

**Before:**
```yaml
- name: WATCH_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.annotations['olm.targetNamespaces']
```

**After:**
```yaml
- name: WATCH_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.namespace
```

Under a namespaced OperatorGroup (OwnNamespace), both sources resolve to the same value — the pod's namespace.
Under a global OperatorGroup (AllNamespaces), `olm.targetNamespaces` would be empty, but `metadata.namespace` correctly resolves to the pod's namespace.

This change is backward-compatible and has no behavioral impact on existing installations.

Note: `operator-sdk generate bundle` automatically rewrites `metadata.namespace` to `metadata.annotations['olm.targetNamespaces']` during bundle generation.
This substitution is hardcoded in the SDK (`setNamespacedFields` in `clusterserviceversion_updaters.go`) and cannot be disabled.
A post-generation patch step is required to restore `metadata.namespace`.

Note: OLM still sets the `olm.targetNamespaces` annotation on the pod template regardless of whether the operator reads it.
This annotation continues to function for OLM's internal RBAC management — it is simply unused by the operator's env var.

### Change 3: Add Permissions for Missing ServiceAccounts

In AllNamespaces mode, OLM requires every ServiceAccount declared in `clusterPermissions` to also have a corresponding `permissions` (namespace-scoped Role) entry.
Without this, OLM cannot create the ServiceAccounts and the CSV stays `Pending` with `"no owned roles found"`.

The CSV currently declares `clusterPermissions` for three SAs but only has a `permissions` entry for `openshift-adp-controller-manager` (leader-election Role).

Add `permissions` entries for `non-admin-controller` and `velero` SAs with leader-election rules (configmaps, leases, events).
These are the same rules already used by the existing `openshift-adp-controller-manager` permissions entry.
The `velero` SA does not actually perform leader election — these are placeholder rules required solely to satisfy OLM's SA creation requirement.

In AllNamespaces mode, OLM promotes these namespace-scoped `permissions` to ClusterRoles/ClusterRoleBindings via `ensureSingletonRBAC`.
In OwnNamespace mode, they remain namespace-scoped Roles/RoleBindings.
This promotion is handled entirely by OLM and is transparent to the operator.

### What does NOT change

- No Go code changes. `cmd/main.go`, controllers, and all runtime behavior are untouched.
- `WATCH_NAMESPACE` always resolves to a non-empty namespace name (the pod's own namespace).
- Cache scoping, PSA labeling, STS flow, CLI/VMDP downloads, sub-controller propagation all continue to work exactly as today.
- RBAC `clusterPermissions` (ClusterRoles and ClusterRoleBindings) are unchanged.
- The operator binary is identical — the same image is used regardless of install mode.
- The `operatorframework.io/suggested-namespace: openshift-adp` annotation remains, ensuring OperatorHub defaults to `openshift-adp` for both install modes.

### OLM Behavioral Differences in AllNamespaces Mode

In AllNamespaces mode, OLM behaves differently in several ways that do not affect operator functionality but should be understood:

- **CSV copies**: OLM creates a copy of the CSV resource in every namespace on the cluster.
  On large clusters this has performance implications (etcd storage, API server load).
  Mitigation: `OLMConfig` provides `spec.features.disableCopiedCSVs: true` to disable copies for AllNamespaces operators.
  Copied CSVs are informational only and do not affect operator behavior.

- **CRD ownership**: In AllNamespaces mode, the operator globally owns its CRDs.
  No other operator can declare ownership of the same CRDs (e.g., Velero CRDs).
  This is not an issue for OADP since it is the sole owner of both `oadp.openshift.io` and `velero.io` CRDs on the cluster.
  However, customers running a standalone upstream Velero alongside OADP would hit an `InterOperatorGroupOwnerConflict`.

- **Dual installation prevention**: OLM prevents installing the same operator in two different namespaces with overlapping OperatorGroups.
  A customer cannot have OADP in both `openshift-adp` (global) and `openshift-operators` (global) simultaneously.

### OperatorHub User Experience

When both install modes are enabled, the OperatorHub UI (OpenShift 4.14+) presents:

1. **Install mode selection** — radio buttons: "All namespaces on the cluster" and "A specific namespace on the cluster"
2. **Namespace selection** — dropdown defaulting to `openshift-adp` (driven by the `suggested-namespace` annotation) in both modes

For fresh AllNamespaces installations via OperatorHub, the Console automatically creates:
- The `openshift-adp` namespace (if it doesn't exist)
- A global OperatorGroup in that namespace
- A Subscription pointing to the operator

This follows the established pattern used by the Loki Operator (`openshift-operators-redhat`) and OpenShift Serverless (`openshift-serverless`).

For CLI installations, users must manually create the namespace, OperatorGroup, and Subscription.

### OLMv1 Considerations

AllNamespaces is the strategically correct direction for OLMv1.
At OLMv1 GA (OCP 4.18), only AllNamespaces operators were installable.
OwnNamespace support was added later as a backward-compatibility feature (Tech Preview in OCP 4.19, GA in OCP 4.22).

OLMv1 does not use `installModes` or `OperatorGroups`.
Namespace scoping is handled via `ClusterExtension.spec.config.inline.watchNamespace`.
No operator code changes are needed for OLMv1 compatibility — only the installation mechanism changes.

OLM Classic coexists with OLMv1 throughout the OpenShift 4 lifecycle.

## Implementation

| Phase | Scope | Risk | Depends on |
|---|---|---|---|
| 1. CSV changes | CSV metadata (installModes, WATCH_NAMESPACE source, permissions) | Low | None |
| 2. Deploy + CI for AllNamespaces | Makefile target + Prow job running existing e2e suite | Low | Phase 1 |
| 3. AllNamespaces install e2e tests | Test scenarios for AllNamespaces-specific behavior | Medium | Phase 2 |
| 4. Migration documentation | Docs for OperatorGroup swap | Low | Phase 2 |

### Phase 3: AllNamespaces Install E2E Tests

With Phase 2 providing baseline signal from the existing e2e suite, this phase adds test scenarios that specifically validate AllNamespaces install behavior.

Test scenarios:

- **AllNamespaces fresh install**: install with `--install-mode AllNamespaces`, verify CSV Succeeded, WATCH_NAMESPACE = pod namespace, DPA reconciles, Velero deploys.
- **OwnNamespace to AllNamespaces migration**: install OwnNamespace, swap OperatorGroup to global, verify operator re-deploys and DPA continues functioning without re-creation.
- **AllNamespaces to OwnNamespace rollback**: reverse the migration, verify operator recovers.
- **Upgrade with AllNamespaces**: upgrade from a prior OADP version (OwnNamespace-only CSV) to the new version (dual-mode CSV), verify the upgrade succeeds and the existing namespaced OperatorGroup continues to work.
- **Upgrade test parameterization**: the existing upgrade test (`upgrade_suite_test.go`) hardcodes a namespaced OperatorGroup. Parameterize to also test with a global OperatorGroup.

### Upgrade Path

Upgrading from a prior OADP version (OwnNamespace-only) to the version with this change is seamless.
Adding `AllNamespaces: true` to the installModes is a safe superset change — OLM does not reject the upgrade.
The customer's existing namespaced OperatorGroup continues to work.
No customer action is required unless they want to switch to AllNamespaces mode.

### Migration (OwnNamespace to AllNamespaces)

Since both install modes are supported in the same CSV, migration is a single step: swap the OperatorGroup.
No channel switch or CSV change is needed.

1. Delete the existing namespaced OperatorGroup.
2. Create a global OperatorGroup in `openshift-adp`.
3. OLM re-deploys the operator. Behavior is identical.

Rollback is the reverse: delete the global OperatorGroup, create a namespaced one.
No CSV downgrade is needed — the same CSV supports both modes.

Brief operator unavailability occurs during the OperatorGroup swap (seconds to approximately one minute).
In-flight backups or restores should be completed before migration.

## Alternatives Considered

### Two separate channels (one per install mode)

A separate AllNamespaces channel with its own CSV was considered.
This was rejected because a single CSV can support both install modes simultaneously — OLM uses the OperatorGroup to determine the active mode.
The single-CSV approach eliminates the need for two channels, two bundles, kustomize overlays, and catalog changes.
It also avoids the OLM upgrade deadlock that would occur if a future release dropped OwnNamespace support.

### Two separate OLM packages

A separate package (e.g., `oadp-operator-allnamespaces`) would provide clean separation but requires customers to uninstall and reinstall to migrate.
The single-CSV approach avoids this entirely — customers only swap their OperatorGroup.

### Handle empty WATCH_NAMESPACE in Go code

Instead of switching the env var source to `metadata.namespace`, the operator could detect an empty `WATCH_NAMESPACE` and fall back to the pod's own namespace at runtime.
This was rejected because it would require Go code changes, defeating the CSV-only design goal.
It would also introduce a runtime behavior difference between the two install modes.

## Security Considerations

The `velero` ServiceAccount's ClusterRole grants near-cluster-admin permissions (`apiGroups: ['*'], resources: ['*']`).
These permissions are declared as `clusterPermissions` in the CSV and bound via `ClusterRoleBinding`, which is cluster-scoped regardless of the OperatorGroup's install mode.
The `OwnNamespace` OperatorGroup does not restrict or contain these permissions — the RBAC posture is identical in both `OwnNamespace` and `AllNamespaces` modes.

The new namespace-scoped `permissions` entries for `non-admin-controller` and `velero` add minimal RBAC: configmaps, leases, and events (standard leader-election permissions).
In AllNamespaces mode, OLM promotes these to ClusterRoles/ClusterRoleBindings.
These are strictly less permissive than the existing `clusterPermissions` for those SAs.

Since the operator's runtime behavior is unchanged (still watches only its own namespace), the actual security posture does not change.
A security review of the velero SA permissions is recommended as a general hygiene item, independent of this install mode change.

## Compatibility

- Existing `OwnNamespace` installations are fully backward-compatible. The CSV supports both modes, so no OperatorGroup change is required to continue running as OwnNamespace.
- Upgrading from a prior release to the version with this change does not alter the customer's install mode — their existing namespaced OperatorGroup stays in place.
- Migration to AllNamespaces is optional and requires an explicit OperatorGroup swap.
- No Go code changes are required. The operator binary is identical.
- Runtime behavior is identical in both modes: `WATCH_NAMESPACE` always resolves to the operator's own namespace.
- Under OwnNamespace, `metadata.namespace` and `olm.targetNamespaces` resolve to the same value — the change is transparent.
- Minimum OpenShift version: the operator functions in both modes on any supported OpenShift version. The `suggested-namespace` annotation in OperatorHub requires OpenShift 4.14+ for AllNamespaces namespace selection.

## Open Issues

1. **`operator-sdk generate bundle` override**: The SDK automatically replaces `metadata.namespace` with `metadata.annotations['olm.targetNamespaces']` in the generated CSV.
This substitution is hardcoded and cannot be disabled.
A post-generation patch step (e.g., `sed` in the Makefile `bundle` target) is required.
This must survive the `bundle-isupdated` CI check, which regenerates the bundle and diffs the result.

2. **Minimum version**: Which OADP release will include this change?

## Future Enhancements

These are out of scope for this proposal but are natural follow-on work:

### Decouple OPERATOR_NAMESPACE from WATCH_NAMESPACE

If a future requirement is for the operator to actually watch all namespaces (or a different namespace), `WATCH_NAMESPACE` would need to be set to empty or to a different value than the operator's own namespace.
In that case, a separate `OPERATOR_NAMESPACE` env var (sourced from `metadata.namespace`) would be needed so the operator knows its own namespace for PSA labeling, STS credential setup, CLI/VMDP downloads, and sub-controller configuration.

### Handle Empty WATCH_NAMESPACE for Cluster-Wide Watching

If `WATCH_NAMESPACE` is set to empty (to watch all namespaces), the controller-runtime manager's cache config must be updated to omit `DefaultNamespaces` instead of inserting an empty-string key.
The `getWatchNamespace()` function in `cmd/main.go` would also need to treat empty/unset as valid rather than logging an error.
DPA singleton enforcement scope would need a design decision: one DPA globally or one per namespace (multi-tenant Velero).

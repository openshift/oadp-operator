# AllNamespaces Install Mode for OADP Operator

## Abstract

Enable `AllNamespaces` install mode alongside the existing `OwnNamespace` mode in a single CSV.
The operator's runtime behavior is unchanged — it watches only the namespace it is deployed in.
No Go code changes are required.

## Background

OADP is installed via OLM with only `OwnNamespace` install mode supported.
At runtime, the `WATCH_NAMESPACE` environment variable controls which namespace the controller-runtime cache monitors.

The OLM-deployed CSV sources `WATCH_NAMESPACE` from the `olm.targetNamespaces` annotation, which OLM sets based on the OperatorGroup.
In OwnNamespace mode this resolves to the operator's namespace.
In AllNamespaces mode this would be empty — breaking PSA labeling, STS credential flow, CLI/VMDP setup, and the cache configuration.

The fix is to source `WATCH_NAMESPACE` from `metadata.namespace` (the pod's own namespace via Kubernetes downward API) instead.
This resolves to the same value under OwnNamespace (backward-compatible) and correctly resolves to the pod's namespace under AllNamespaces.

Additionally, OLM requires every ServiceAccount declared in `clusterPermissions` to have a corresponding `permissions` (namespace-scoped Role) entry when running in AllNamespaces mode.
Two of the three OADP ServiceAccounts currently lack this entry.

Both issues are CSV metadata problems, not Go code problems.

### OLMv1 Alignment

AllNamespaces is the strategically correct direction.
At OLMv1 GA (OCP 4.18), only AllNamespaces operators were installable.
OwnNamespace support was added later as backward-compatibility (Tech Preview in OCP 4.19, GA in OCP 4.22).
Enabling AllNamespaces now positions OADP for OLMv1 readiness.

## Goals

- Enable `AllNamespaces` install mode alongside `OwnNamespace` in the same CSV.
- Keep runtime behavior identical: the operator watches only the namespace it is deployed in.
- Maintain full backward compatibility for existing `OwnNamespace` installations.

## Non Goals

- Actually watching all namespaces or supporting multi-tenant Velero (one DPA per namespace).
- `SingleNamespace` or `MultiNamespace` install mode support.

## High-Level Design

Three CSV metadata changes, no Go code changes:

| # | Change | Why |
|---|---|---|
| 1 | Enable `AllNamespaces: true` in `installModes` | Allow installation with a global OperatorGroup |
| 2 | Source `WATCH_NAMESPACE` from `metadata.namespace` | Avoid empty value under AllNamespaces; backward-compatible under OwnNamespace |
| 3 | Add `permissions` entries for `non-admin-controller` and `velero` SAs | OLM requires these to create the ServiceAccounts in AllNamespaces mode |

The operator binary, RBAC `clusterPermissions`, and all runtime behavior remain identical.

**How `WATCH_NAMESPACE` resolves in each mode:**

| OperatorGroup | `olm.targetNamespaces` | `metadata.namespace` | Operator watches |
|---|---|---|---|
| Namespaced (OwnNamespace) | `openshift-adp` | `openshift-adp` | `openshift-adp` |
| Global (AllNamespaces) | `""` (empty) | `openshift-adp` | `openshift-adp` |

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

Adding installMode support is a safe superset change in OLM — it never blocks upgrades.
Existing customers with a namespaced OperatorGroup are unaffected; their install mode stays OwnNamespace.

### Change 2: WATCH_NAMESPACE Source

```yaml
# Before
- name: WATCH_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.annotations['olm.targetNamespaces']

# After
- name: WATCH_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.namespace
```

`operator-sdk generate bundle` automatically rewrites `metadata.namespace` to `olm.targetNamespaces` during bundle generation.
This substitution is hardcoded in the SDK and cannot be disabled.
A post-generation patch step is required to restore `metadata.namespace`.

OLM still sets the `olm.targetNamespaces` annotation on the pod template regardless — it is simply unused by the operator's env var.

### Change 3: Add Permissions for Missing ServiceAccounts

The CSV declares `clusterPermissions` for three SAs but only `openshift-adp-controller-manager` has a `permissions` entry.
Add `permissions` entries for `non-admin-controller` and `velero` with leader-election rules (configmaps, leases, events).

The `velero` SA does not actually perform leader election — these are placeholder rules required solely to satisfy OLM's SA creation requirement.
In AllNamespaces mode, OLM promotes these to ClusterRoles/ClusterRoleBindings via `ensureSingletonRBAC`.
In OwnNamespace mode, they remain namespace-scoped Roles/RoleBindings.

### OLM Behavior in AllNamespaces Mode

These OLM behaviors do not affect operator functionality but should be understood:

- **CSV copies**: OLM copies the CSV to every namespace. On large clusters, `OLMConfig.spec.features.disableCopiedCSVs: true` disables this.
- **CRD ownership**: The operator globally owns its CRDs. Customers running standalone upstream Velero alongside OADP would hit `InterOperatorGroupOwnerConflict`.
- **Dual installation prevention**: OLM prevents installing the operator in two namespaces with overlapping OperatorGroups.

### OperatorHub User Experience

When both install modes are enabled, OperatorHub (OpenShift 4.14+) presents install mode radio buttons and a namespace dropdown.
The existing `suggested-namespace: openshift-adp` annotation defaults the namespace to `openshift-adp` in both modes.
For fresh AllNamespaces installs, the Console automatically creates the namespace, a global OperatorGroup, and a Subscription.

This follows the pattern used by the Loki Operator (`openshift-operators-redhat`) and OpenShift Serverless (`openshift-serverless`).

## Implementation

| Phase | Scope | Risk | Depends on |
|---|---|---|---|
| 1. CSV changes | installModes, WATCH_NAMESPACE source, permissions | Low | None |
| 2. Deploy + CI | Makefile target + Prow job with existing e2e suite | Low | Phase 1 |
| 3. AllNamespaces e2e tests | AllNamespaces-specific test scenarios | Medium | Phase 2 |
| 4. Migration documentation | OperatorGroup swap procedure | Low | Phase 3 |

### Phase 1: CSV Changes

Apply the three changes described in Detailed Design.
The `bundle` Makefile target needs a post-generation step to replace `olm.targetNamespaces` with `metadata.namespace`.
This must survive the `bundle-isupdated` CI check.

### Phase 2: Deploy + CI for AllNamespaces

Add a `deploy-olm-allnamespaces` Makefile target using `operator-sdk run bundle --install-mode AllNamespaces`.
Add a Prow presubmit job that deploys with this target and runs the existing e2e suite unchanged.
This gives immediate signal before writing any new test code.

### Phase 3: AllNamespaces Install E2E Tests

Test scenarios specific to AllNamespaces behavior:

- **AllNamespaces fresh install**: verify CSV Succeeded, `WATCH_NAMESPACE` = pod namespace, DPA reconciles, Velero deploys.
- **OwnNamespace to AllNamespaces migration**: install OwnNamespace, swap OperatorGroup to global, verify operator re-deploys and existing DPA continues functioning.
- **AllNamespaces to OwnNamespace rollback**: reverse the migration, verify operator recovers.
- **Upgrade to dual-mode CSV**: upgrade from a prior version (OwnNamespace-only) to the new version, verify the existing namespaced OperatorGroup continues to work.
- **Upgrade test parameterization**: the existing upgrade test hardcodes a namespaced OperatorGroup — parameterize to also test with a global OperatorGroup.

### Phase 4: Migration Documentation

Since both modes coexist in the same CSV, migration is a single step: swap the OperatorGroup.

1. Delete the existing namespaced OperatorGroup.
2. Create a global OperatorGroup in `openshift-adp` (`spec: {}`).
3. OLM re-deploys the operator. Behavior is identical.

Rollback is the reverse. No CSV downgrade is needed.
Brief operator unavailability occurs during the swap (seconds to ~1 minute).
In-flight backups or restores should be completed before migration.

## Upgrade Path

Upgrading from a prior OADP version (OwnNamespace-only CSV) to the version with this change is seamless.
Adding `AllNamespaces: true` is a safe superset change — OLM does not reject the upgrade.
The customer's existing namespaced OperatorGroup continues to work.
No customer action is required unless they want to switch to AllNamespaces mode.

## Alternatives Considered

### Two separate channels (one per install mode)

Rejected. A single CSV supports both install modes — OLM uses the OperatorGroup to determine the active mode.
Two channels would require separate bundles, kustomize overlays, catalog changes, and cross-channel update graphs.
It would also create an OLM upgrade deadlock if a future release dropped OwnNamespace support.

### Two separate OLM packages

Rejected. Requires customers to uninstall and reinstall to migrate.
The single-CSV approach requires only an OperatorGroup swap.

### Handle empty WATCH_NAMESPACE in Go code

Rejected. Would require Go code changes and introduce a runtime behavior difference between install modes.
Sourcing from `metadata.namespace` achieves the same result at the CSV level.

## Security Considerations

The `velero` SA's ClusterRole grants near-cluster-admin permissions (`apiGroups: ['*'], resources: ['*']`).
These are bound via `ClusterRoleBinding`, which is cluster-scoped regardless of install mode.
The `OwnNamespace` OperatorGroup does not restrict these permissions — the RBAC posture is identical in both modes.

The new `permissions` entries add minimal RBAC (configmaps, leases, events) and are strictly less permissive than the existing `clusterPermissions` for those SAs.
In AllNamespaces mode, OLM promotes these to ClusterRoles.

A security review of the velero SA permissions is recommended as a general hygiene item, independent of this change.

## Compatibility

- Existing OwnNamespace installations are fully backward-compatible. No OperatorGroup change required.
- Upgrade from prior releases does not alter the install mode.
- Migration to AllNamespaces is optional and requires an explicit OperatorGroup swap.
- Under OwnNamespace, `metadata.namespace` and `olm.targetNamespaces` resolve to the same value.
- Minimum OpenShift version for AllNamespaces namespace selection in OperatorHub: 4.14.

## Open Issues

1. **`operator-sdk generate bundle` override**: The SDK hardcodes the `olm.targetNamespaces` substitution. A post-generation patch is required and must survive the `bundle-isupdated` CI check.

2. **Target OADP release**: Which version will include this change?

## Validation

Tested on OpenShift 4.22.0-ec.3 (2026-08-13). Both install modes validated with the same bundle.
See [full test log](https://hackmd.io/MVRrs4zTTHiwGcxfRUwhBA).

| Test | Mode | Result |
|---|---|---|
| CSV install | AllNamespaces | PASS |
| CSV install | OwnNamespace | PASS |
| WATCH_NAMESPACE value | Both | `openshift-adp` |
| DPA reconciliation + Velero deploy | Both | PASS |
| All controllers started | Both | PASS |

## Future Enhancements

### Cluster-Wide Watching

If a future requirement is for the operator to watch all namespaces, `WATCH_NAMESPACE` would need to be empty.
This requires a separate `OPERATOR_NAMESPACE` env var so the operator knows its own namespace for PSA labeling, STS, CLI/VMDP setup.
It also requires changes to the cache configuration and a design decision on DPA singleton scope (one globally or one per namespace).

# AllNamespaces Install Mode for OADP Operator

## Abstract

OADP operator currently supports only `OwnNamespace` install mode via OLM.
This proposal describes a phased plan to add `AllNamespaces` install mode support, delivered as a separate channel within the existing OLM package.
The operator's runtime behavior remains identical: it watches only the namespace it is deployed in, regardless of install mode.
Both channels must coexist for at least one release cycle to provide a safe upgrade path for existing customers.

## Background

The OADP operator is installed via OLM with a strict `OwnNamespace` install mode.
The CSV declares only `OwnNamespace: true` and all other modes are `supported: false`.

At runtime, the `WATCH_NAMESPACE` environment variable controls which namespace the controller-runtime cache monitors.
In the OwnNamespace CSV, `WATCH_NAMESPACE` is sourced from the `olm.targetNamespaces` annotation, which OLM sets to the operator's own namespace.
In the non-OLM deployment (`config/manager/manager.yaml`), `WATCH_NAMESPACE` is sourced from `metadata.namespace` (the pod's own namespace via downward API).

The key insight is that `AllNamespaces` is an OLM install mode — it controls what OperatorGroup the CSV is compatible with, not what the operator actually watches at runtime.
The AllNamespaces CSV can source `WATCH_NAMESPACE` from `metadata.namespace` (just like the non-OLM deployment does today) instead of from `olm.targetNamespaces`.
This means the operator keeps watching only its own namespace by default, even when installed via a global OperatorGroup.

RBAC is already cluster-scoped (ClusterRoles and ClusterRoleBindings), so no fundamental RBAC changes are required.
No webhooks are currently enabled.
A `ClusterWideClient` (uncached) already exists for cross-namespace DPA validation.

### Why Two Channels Are Required

A single CSV cannot have both `OwnNamespace` and `AllNamespaces` enabled simultaneously.
More critically, OLM validates that a CSV supports the OperatorGroup's install mode before allowing an install or upgrade.

If a future release shipped only an AllNamespaces CSV, existing customers with a namespaced OperatorGroup (`targetNamespaces: [openshift-adp]`) would be **blocked from upgrading** — OLM would reject the new CSV because it doesn't support `OwnNamespace`.
The customer cannot change the OperatorGroup first, because their current CSV also doesn't support `AllNamespaces`, creating a **deadlock**.

Both channels must coexist for at least one release cycle so customers can:
1. Upgrade to the version that offers both channels (staying on OwnNamespace).
2. Migrate to AllNamespaces at their own pace using the documented migration steps.
3. A future release can then deprecate the OwnNamespace channel after the migration window closes.

## Goals

- Enable `AllNamespaces` install mode via a second channel in the existing OLM package.
- Keep runtime behavior identical to today: the operator watches only the namespace it is deployed in.
- Provide a documented migration path for customers moving from `OwnNamespace` to `AllNamespaces`.
- Maintain full backward compatibility for existing `OwnNamespace` installations.

## Non Goals

- Actually watching all namespaces. The operator continues to watch only its own namespace. Cluster-wide watching can be enabled in the future by setting `WATCH_NAMESPACE` to empty, but that is a separate enhancement requiring additional work (see Future Enhancements).
- Multi-tenant Velero (one DPA per namespace). Global DPA singleton enforcement will be maintained.
- Removing `OwnNamespace` support. Both channels will coexist for at least one release cycle. Deprecation of OwnNamespace is a separate future decision.
- `SingleNamespace` or `MultiNamespace` install mode support.

## High-Level Design

The work is split into four phases, each independently mergeable:

1. **Build infrastructure for two bundle variants and enable AllNamespaces** (kustomize overlays that produce two CSVs in separate channels within the same OLM package). The existing OwnNamespace CSV is not modified.
2. **E2E test infrastructure** for both install modes.
3. **CI/Prow integration** with AllNamespaces-specific jobs.
4. **Migration documentation** for customers.

## Detailed Design

### Current State

| Area | Current Behavior | Key File(s) |
|---|---|---|
| CSV installModes | Only `OwnNamespace: true` | `config/manifests/bases/oadp-operator.clusterserviceversion.yaml` (lines 467-475) |
| CSV WATCH_NAMESPACE source | `olm.targetNamespaces` annotation | `bundle/manifests/oadp-operator.clusterserviceversion.yaml` (lines 1449-1452) |
| Manager WATCH_NAMESPACE source | `metadata.namespace` (downward API) | `config/manager/manager.yaml` (lines 63-66) |
| Cache scoping | `DefaultNamespaces` map with single entry | `cmd/main.go` (lines 204-208) |
| PSA labeling | Patches `watchNamespace`, errors if empty | `cmd/main.go` (lines 362-389) |
| CLI/VMDP downloads | Skipped if `watchNamespace` is empty | `cmd/main.go` (lines 305-306) |
| STS flow | Reads `WATCH_NAMESPACE` as install namespace | `pkg/credentials/stsflow/stsflow.go` (line 115) |
| Sub-controllers | All receive `WATCH_NAMESPACE` = own namespace | `nonadmin_controller.go` (line 176), `kubevirt_datamover_controller.go` (line 152), `vmfilerestore_controller.go` (line 184) |
| E2E OperatorGroup | Always `targetNamespaces: [namespace]` | `Makefile` (line 639), `upgrade_suite_test.go` (lines 31-50) |
| Cross-NS validation | Uses uncached `ClusterWideClient` | `cmd/main.go` (lines 260-270), `validator.go` (line 144) |
| RBAC | Already cluster-scoped (ClusterRoles) | `config/rbac/role.yaml`, CSV `clusterPermissions` |
| OLM channels | Single channel: `dev` (release branches use e.g. `oadp-1.5`) | `Makefile` (lines 23, 33), `bundle/metadata/annotations.yaml` |

### Phase 1: Build Infrastructure for Two Bundle Variants and Enable AllNamespaces

Produce two distinct OLM bundles from one codebase, shipped as separate channels in the same `oadp-operator` package.
The CSV changes (installModes flip, `WATCH_NAMESPACE` source change) are applied only to the AllNamespaces variant via a kustomize overlay — the existing OwnNamespace CSV is never modified.

The AllNamespaces CSV differs from the OwnNamespace CSV in exactly two ways:

1. `installModes` — only `AllNamespaces: true` (instead of only `OwnNamespace: true`).
2. `WATCH_NAMESPACE` source — `metadata.namespace` via downward API (instead of `olm.targetNamespaces` annotation).

With `WATCH_NAMESPACE` sourced from `metadata.namespace`, the operator always resolves to the pod's own namespace regardless of what `olm.targetNamespaces` contains (which would be empty under a global OperatorGroup).
No Go code changes are needed. The operator binary is identical for both CSVs.

#### Catalog Structure

```
Catalog: oadp-operator-catalog
└── Package: oadp-operator
    ├── Channel: dev                    ← OwnNamespace CSV
    │   └── oadp-operator.v99.0.0
    └── Channel: dev-allnamespaces      ← AllNamespaces CSV
        └── oadp-operator.v99.0.0-allns
```

Release branches follow the same pattern: `stable-1.7` (OwnNamespace) and `stable-1.7-allnamespaces` (AllNamespaces).

#### What does NOT change

- No Go code changes. `cmd/main.go`, controllers, and all runtime behavior are untouched.
- The existing OwnNamespace CSV, bundle, and channel are not modified.
- `WATCH_NAMESPACE` always resolves to a non-empty namespace name (the pod's own namespace).
- Cache scoping, PSA labeling, STS flow, CLI/VMDP downloads, sub-controller propagation all continue to work exactly as today.
- RBAC is already cluster-scoped and does not need modification.

#### Changes

| Area | Change |
|---|---|
| **CSV base template** | Keep `config/manifests/bases/oadp-operator.clusterserviceversion.yaml` as the shared base, unchanged |
| **Kustomize overlays** | Create `config/manifests/overlays/ownnamespace/` and `config/manifests/overlays/allnamespaces/` with patches for `installModes` and `WATCH_NAMESPACE` env var source |
| **AllNamespaces overlay** | Patches: (1) `installModes` set to only `AllNamespaces: true`, (2) `WATCH_NAMESPACE` source changed from `olm.targetNamespaces` to `metadata.namespace` |
| **OwnNamespace overlay** | Patches: keeps current behavior (only `OwnNamespace: true`, `WATCH_NAMESPACE` from `olm.targetNamespaces`). Identity transform initially. |
| **Makefile** | Add `INSTALL_MODE ?= OwnNamespace`. New targets: `bundle-allnamespaces`, `bundle-build-allnamespaces` |
| **Catalog build** | Parameterize `Dockerfile.catalog` and `catalog-build` target to produce a single catalog with two channels: existing channel (OwnNamespace bundle) and new `-allnamespaces` channel (AllNamespaces bundle) |
| **CSV naming** | AllNamespaces CSV uses a distinct version suffix: `oadp-operator.v99.0.0-allns` vs `oadp-operator.v99.0.0` |
| **Channel naming** | Convention: `<existing-channel>-allnamespaces` (e.g., `dev-allnamespaces`, `stable-1.7-allnamespaces`) |

#### Validation

- `make bundle` and `make bundle-allnamespaces` both produce valid bundles.
- `opm validate` passes on both bundles.
- Catalog with two channels builds and serves correctly.

#### Risk: Medium

Build plumbing only, no runtime impact.

### Phase 2: E2E Test Infrastructure

Tests need to validate both install modes.

#### Changes

| Area | Change |
|---|---|
| `Makefile` `deploy-olm` (line 634-642) | Parameterize OperatorGroup creation: when `INSTALL_MODE=AllNamespaces`, create OperatorGroup with empty `spec` (no `targetNamespaces`). Default (`OwnNamespace`) keeps current behavior. |
| `Makefile` | New target: `deploy-olm-allnamespaces` that sets `INSTALL_MODE=AllNamespaces` and `DEFAULT_CHANNEL=dev-allnamespaces` |
| `tests/e2e/upgrade_suite_test.go` (lines 31-50) | Parameterize OperatorGroup creation to support both modes based on a test flag |
| E2E test scenarios | Add: install AllNamespaces, create DPA in operator namespace, verify Velero deploys. Verify `WATCH_NAMESPACE` resolves to operator namespace. Verify singleton enforcement. Verify sub-controller namespace config. |
| Migration test | Test switching channel on an existing Subscription and swapping the OperatorGroup to verify the documented migration path works end-to-end. |

#### Validation

Full e2e suite passes with both `make deploy-olm` (OwnNamespace) and `make deploy-olm-allnamespaces`.

#### Risk: Medium

Test infrastructure changes are additive.

### Phase 3: CI/Prow Integration

AllNamespaces mode needs CI coverage.

#### Changes

| Area | Change |
|---|---|
| `openshift/release` config | Add new presubmit job(s) running e2e tests with `INSTALL_MODE=AllNamespaces` |
| Job naming | e.g., `e2e-aws-allnamespaces` alongside existing `e2e-aws` |
| Periodic jobs | Add AllNamespaces variants for nightly runs |

#### Validation

CI jobs pass on a test PR.

#### Risk: Low

Additive CI config in a separate repo.

### Phase 4: Migration Documentation

Customers migrating from OwnNamespace to AllNamespaces need clear, tested manual steps.
OLM does not automate the OperatorGroup swap.

Note: after migration, the operator's runtime behavior is identical.
`WATCH_NAMESPACE` is sourced from `metadata.namespace` in the AllNamespaces CSV, so it always resolves to the operator's own namespace.
The operator does not start watching all namespaces.

#### Prerequisites

- Cluster admin access.
- No active backups or restores in progress.
- Current OADP version supports the AllNamespaces channel (minimum version TBD).

#### Migration Steps

**Step 1: Verify current state**

```bash
oc get subscription oadp-operator -n openshift-adp -o yaml
oc get operatorgroup -n openshift-adp -o yaml
oc get dpa -n openshift-adp
```

**Step 2: Switch subscription channel**

```bash
oc patch subscription oadp-operator -n openshift-adp \
  --type merge -p '{"spec":{"channel":"stable-1.x-allnamespaces"}}'
```

OLM installs the new CSV.
The operator pod restarts, but the OperatorGroup still scopes it to OwnNamespace.
No behavioral change yet.

**Step 3: Delete the existing namespaced OperatorGroup**

```bash
oc delete operatorgroup oadp-operator-group -n openshift-adp
```

The operator pod stops (OLM removes the deployment when no valid OperatorGroup exists).

**Step 4: Create a global OperatorGroup**

```bash
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: oadp-operator-group
  namespace: openshift-adp
spec: {}
EOF
```

OLM re-deploys the operator.
`WATCH_NAMESPACE` is sourced from `metadata.namespace`, so the operator continues to watch only the `openshift-adp` namespace.
Behavior is identical to before the migration.

**Step 5: Verify the migration**

```bash
# Operator pod is running
oc get pods -n openshift-adp -l control-plane=controller-manager

# CSV is Succeeded
oc get csv -n openshift-adp

# DPA is reconciled
oc get dpa -n openshift-adp -o jsonpath='{.items[0].status.conditions}'

# Velero is running
oc get deployment -n openshift-adp -l app.kubernetes.io/name=velero
```

#### Rollback

1. Delete the global OperatorGroup.
2. Create the namespaced OperatorGroup with `targetNamespaces: [openshift-adp]`.
3. Switch the subscription channel back to the OwnNamespace channel.

#### Expected Downtime

Brief operator unavailability during the OperatorGroup swap (seconds to approximately one minute).
No impact on existing backups at rest.
In-flight backups or restores should be completed before migration.

## Alternatives Considered

### Single CSV with both install modes enabled

OLM allows a CSV to declare multiple `installModes` as `supported: true`, with the OperatorGroup determining which is active.
This was rejected because the two install modes cannot be enabled simultaneously, and having a single CSV that advertises both creates ambiguity about which mode is in use.
Two separate CSVs make the install mode explicit and independently releasable.

### Two separate OLM packages

A separate package (e.g., `oadp-operator-allnamespaces`) would provide clean separation but requires customers to uninstall and reinstall to migrate.
The channel-based approach within a single package allows customers to switch by editing the Subscription, avoiding uninstall/reinstall.
Both approaches still require manual OperatorGroup changes, so the channel approach provides a smoother migration UX with no additional risk.

## Security Considerations

The `velero` ServiceAccount's ClusterRole grants near-cluster-admin permissions (`apiGroups: ['*'], resources: ['*']`).
In `OwnNamespace` mode this is contained by the OperatorGroup scope.
In `AllNamespaces` mode the RBAC is identical (already cluster-scoped), but the perception of blast radius changes.
Since the operator's runtime behavior is unchanged (still watches only its own namespace), the actual security posture does not change.
A security review of the velero SA permissions is still recommended as a general hygiene item.

## Compatibility

- Existing `OwnNamespace` installations are unaffected. The OwnNamespace channel continues to exist and receive updates.
- Upgrading from a prior release to the version that introduces the AllNamespaces channel does not change the customer's install mode. They stay on the OwnNamespace channel and must explicitly migrate.
- Migration from `OwnNamespace` to `AllNamespaces` is a manual process documented in Phase 4.
- No Go code changes are required. The operator binary is identical for both CSVs.
- Runtime behavior is identical in both modes: `WATCH_NAMESPACE` always resolves to the operator's own namespace.
- Both channels must coexist for at least one release cycle before the OwnNamespace channel can be deprecated.

## Implementation

| Phase | Scope | Risk | Depends on | Parallelizable |
|---|---|---|---|---|
| 1. Bundle variants + AllNamespaces channel | Build infrastructure + CSV metadata | Medium | None | No (foundation) |
| 2. E2E test infrastructure | Test infrastructure | Medium | Phase 1 | No |
| 3. CI/Prow integration | CI config | Low | Phase 2 | Yes (with Phase 4) |
| 4. Migration documentation | Docs | Low | Phase 1 | Yes (with Phase 3) |

## Open Issues

1. **Channel naming convention**: Proposed convention is `<existing-channel>-allnamespaces` (e.g., `dev-allnamespaces`, `stable-1.7-allnamespaces`).
Open to shorter alternatives if the convention is too verbose.

2. **Minimum version for migration**: Which OADP release will be the first to ship the AllNamespaces channel?
This determines the migration documentation's version requirements and the minimum coexistence window before OwnNamespace can be deprecated.

3. **OwnNamespace deprecation timeline**: How many release cycles should both channels coexist before the OwnNamespace channel is removed?
At minimum one cycle is required for a safe upgrade path.

## Future Enhancements

These are out of scope for this proposal but are natural follow-on work:

### Decouple OPERATOR_NAMESPACE from WATCH_NAMESPACE

If a future requirement is for the operator to actually watch all namespaces (or a different namespace), `WATCH_NAMESPACE` would need to be set to empty or to a different value than the operator's own namespace.
In that case, a separate `OPERATOR_NAMESPACE` env var (sourced from `metadata.namespace`) would be needed so the operator knows its own namespace for PSA labeling, STS credential setup, CLI/VMDP downloads, and sub-controller configuration.

Key files that would need changes:
- `cmd/main.go`: `addPodSecurityPrivilegedLabels()`, `CLIDownloadSetup`, `VMDPDownloadSetup` would use `OPERATOR_NAMESPACE` instead of `WATCH_NAMESPACE`.
- `pkg/credentials/stsflow/stsflow.go`: Read `OPERATOR_NAMESPACE` for install namespace.
- `internal/controller/nonadmin_controller.go`, `kubevirt_datamover_controller.go`, `vmfilerestore_controller.go`: Propagate `OPERATOR_NAMESPACE` to sub-controllers.

### Handle Empty WATCH_NAMESPACE for Cluster-Wide Watching

If `WATCH_NAMESPACE` is set to empty (to watch all namespaces), the controller-runtime manager's cache config must be updated to omit `DefaultNamespaces` instead of inserting an empty-string key.
The `getWatchNamespace()` function in `cmd/main.go` would also need to treat empty/unset as valid rather than logging an error.
DPA singleton enforcement scope would need a design decision: one DPA globally or one per namespace (multi-tenant Velero).

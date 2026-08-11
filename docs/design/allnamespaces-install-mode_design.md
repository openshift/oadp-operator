# AllNamespaces Install Mode for OADP Operator

## Abstract

OADP operator currently supports only `OwnNamespace` install mode via OLM.
This proposal describes a phased plan to add `AllNamespaces` install mode support, delivered via a second CSV to allow customers to migrate at their own pace.
The operator's runtime behavior remains identical: it watches only the namespace it is deployed in, regardless of install mode.
Two delivery options are presented for the second CSV: a separate channel within the existing OLM package, or a separate OLM package with its own catalog entry.

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

## Goals

- Enable `AllNamespaces` install mode via a second CSV, delivered alongside the existing `OwnNamespace` CSV.
- Keep runtime behavior identical to today: the operator watches only the namespace it is deployed in.
- Provide a documented migration path for customers moving from `OwnNamespace` to `AllNamespaces`.
- Maintain full backward compatibility for existing `OwnNamespace` installations.

## Non Goals

- Actually watching all namespaces. The operator continues to watch only its own namespace. Cluster-wide watching can be enabled in the future by setting `WATCH_NAMESPACE` to empty, but that is a separate enhancement requiring additional work (see Future Enhancements).
- Multi-tenant Velero (one DPA per namespace). Global DPA singleton enforcement will be maintained.
- Removing `OwnNamespace` support. Both modes will coexist until a future deprecation decision.
- `SingleNamespace` or `MultiNamespace` install mode support.

## High-Level Design

The work is split into five phases, each independently mergeable:

1. **Enable AllNamespaces install mode in the CSV** (the core change: flip installModes, source `WATCH_NAMESPACE` from `metadata.namespace`).
2. **Build infrastructure for two bundle variants** (kustomize overlays, Makefile targets, two CSVs). Delivery mechanism (channels vs packages) is decided in this phase.
3. **E2E test infrastructure** for both install modes.
4. **CI/Prow integration** with AllNamespaces-specific jobs.
5. **Migration documentation** for customers.

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

### Phase 1: Enable AllNamespaces Install Mode in the CSV

The core change.
The AllNamespaces CSV differs from the OwnNamespace CSV in exactly two ways:

1. `installModes` — only `AllNamespaces: true` (instead of only `OwnNamespace: true`).
2. `WATCH_NAMESPACE` source — `metadata.namespace` via downward API (instead of `olm.targetNamespaces` annotation).

With `WATCH_NAMESPACE` sourced from `metadata.namespace`, the operator always resolves to the pod's own namespace regardless of what `olm.targetNamespaces` contains (which would be empty under a global OperatorGroup).
No Go code changes are needed. The operator binary is identical for both CSVs.

#### Changes

| File | Change |
|---|---|
| `config/manifests/bases/oadp-operator.clusterserviceversion.yaml` (lines 467-475) | Create an AllNamespaces variant with only `AllNamespaces: true` in `installModes` |
| CSV deploymentSpec `WATCH_NAMESPACE` env var | Change source from `metadata.annotations['olm.targetNamespaces']` to `metadata.namespace` (downward API) in the AllNamespaces variant |
| `make bundle` | Regenerate bundle to verify the OwnNamespace CSV is unchanged |

#### What does NOT change

- No Go code changes. `cmd/main.go`, controllers, and all runtime behavior are untouched.
- `WATCH_NAMESPACE` always resolves to a non-empty namespace name (the pod's own namespace).
- Cache scoping, PSA labeling, STS flow, CLI/VMDP downloads, sub-controller propagation all continue to work exactly as today.
- RBAC is already cluster-scoped and does not need modification.

#### Validation

- `make test` passes (no code changes).
- Manual OLM install with AllNamespaces OperatorGroup: operator starts, `WATCH_NAMESPACE` = pod namespace, DPA reconciles normally.

#### Risk: Low

No runtime behavioral change. The only change is in the CSV metadata and env var source.

### Phase 2: Build Infrastructure for Two Bundle Variants

Produce two distinct OLM bundles from one codebase.
A single CSV cannot have both `OwnNamespace` and `AllNamespaces` enabled simultaneously, so two CSVs are required.
The delivery mechanism for the second CSV is decided in this phase.

#### Common Build Changes (both options)

Regardless of delivery option, the following build changes are needed to produce two bundle variants from one codebase:

| Area | Change |
|---|---|
| **CSV base template** | Keep `config/manifests/bases/oadp-operator.clusterserviceversion.yaml` as the shared base |
| **Kustomize overlays** | Create `config/manifests/overlays/ownnamespace/` and `config/manifests/overlays/allnamespaces/` with patches for `installModes` and `WATCH_NAMESPACE` env var source |
| **AllNamespaces overlay** | Patches: (1) `installModes` set to only `AllNamespaces: true`, (2) `WATCH_NAMESPACE` source changed from `olm.targetNamespaces` to `metadata.namespace` |
| **OwnNamespace overlay** | Patches: keeps current behavior (only `OwnNamespace: true`, `WATCH_NAMESPACE` from `olm.targetNamespaces`). Identity transform initially. |
| **Makefile** | Add `INSTALL_MODE ?= OwnNamespace`. New targets: `bundle-allnamespaces`, `bundle-build-allnamespaces` |

#### Option A: Two Channels in the Same Package

The AllNamespaces CSV is shipped as a separate channel within the existing `oadp-operator` package.
Both channels live in the same catalog and share the same package identity.

```
Catalog: oadp-operator-catalog
└── Package: oadp-operator
    ├── Channel: dev                    ← OwnNamespace CSV
    │   └── oadp-operator.v99.0.0
    └── Channel: dev-allnamespaces      ← AllNamespaces CSV
        └── oadp-operator.v99.0.0-allns
```

Release branches follow the same pattern: `stable-1.6` (OwnNamespace) and `stable-1.6-allnamespaces` (AllNamespaces).

**Additional changes for Option A:**

| Area | Change |
|---|---|
| **Catalog build** | Parameterize `Dockerfile.catalog` and `catalog-build` target to produce a single catalog with two channels: existing channel (OwnNamespace bundle) and new `-allnamespaces` channel (AllNamespaces bundle) |
| **CSV naming** | AllNamespaces CSV uses a distinct version suffix: `oadp-operator.v99.0.0-allns` vs `oadp-operator.v99.0.0` |
| **Channel naming** | Convention: `<existing-channel>-allnamespaces` (e.g., `dev-allnamespaces`, `stable-1.6-allnamespaces`) |

**Trade-offs:**

| Pro | Con |
|---|---|
| Single package in OperatorHub; cleaner customer experience | Every release branch must produce two bundles and two channel entries |
| Migration via Subscription channel change (no uninstall/reinstall of the package) | Channel names are overloaded (channels typically mean release stability, not install topology) |
| Single catalog image to build and publish | Customer must still manually swap the OperatorGroup after changing channel |

#### Option B: Two Separate OLM Packages

The AllNamespaces CSV is shipped as a separate OLM package with its own catalog entry.
Each package has its own identity and upgrade graph.

```
Catalog: oadp-operator-catalog
├── Package: oadp-operator                  ← OwnNamespace CSV
│   └── Channel: dev
│       └── oadp-operator.v99.0.0
└── Package: oadp-operator-allnamespaces    ← AllNamespaces CSV
    └── Channel: dev
        └── oadp-operator-allnamespaces.v99.0.0
```

**Additional changes for Option B:**

| Area | Change |
|---|---|
| **Catalog build** | Produce a single catalog containing two packages, each with its own channel(s). Or produce two separate catalogs (one per package). |
| **CSV naming** | AllNamespaces CSV uses a distinct package name: `oadp-operator-allnamespaces` |
| **Bundle metadata** | AllNamespaces bundle has its own `annotations.yaml` with `operators.operatorframework.io.bundle.package.v1: oadp-operator-allnamespaces` |
| **Makefile** | Additional target: `catalog-build-allnamespaces` if producing separate catalogs |

**Trade-offs:**

| Pro | Con |
|---|---|
| Clean separation; no channel naming overload | Two tiles in OperatorHub; customers must know which to pick |
| Each package has its own independent upgrade graph | Migration requires uninstalling the old package and installing the new one |
| Channel names stay semantic (stable, dev, etc.) | Doubles catalog/release artifacts per version |
| Simpler catalog structure per package | Customer loses the Subscription during migration (must re-create) |

#### Decision Criteria

The choice between Option A and Option B should consider:

1. **Downstream release pipeline**: Does Konflux/ART handle two channels in one package easily, or is a second package simpler to wire up?
2. **Migration UX priority**: Is avoiding uninstall/reinstall (Option A) worth the channel naming complexity?
3. **Long-term intent**: If OwnNamespace will eventually be deprecated, Option A lets you sunset a channel; Option B requires deprecating an entire package.
4. **OperatorHub presentation**: One tile with a channel picker (Option A) vs two tiles (Option B).

#### Validation

- `make bundle` and `make bundle-allnamespaces` both produce valid bundles.
- `opm validate` passes on both bundles.
- Catalog builds and serves correctly with the chosen delivery structure.

#### Risk: Medium

Build plumbing only, no runtime impact.

### Phase 3: E2E Test Infrastructure

Tests need to validate both install modes.
The test infrastructure changes depend on the delivery option chosen in Phase 2.

#### Changes

| Area | Change |
|---|---|
| `Makefile` `deploy-olm` (line 634-642) | Parameterize OperatorGroup creation: when `INSTALL_MODE=AllNamespaces`, create OperatorGroup with empty `spec` (no `targetNamespaces`). Default keeps current behavior. |
| `Makefile` | New target: `deploy-olm-allnamespaces`. For Option A (channels): sets `INSTALL_MODE=AllNamespaces` and `DEFAULT_CHANNEL=dev-allnamespaces`. For Option B (packages): sets `INSTALL_MODE=AllNamespaces` and overrides `CATALOG_SOURCE_NAME` and subscription package name. |
| `tests/e2e/upgrade_suite_test.go` (lines 31-50) | Parameterize OperatorGroup creation to support both modes based on a test flag |
| E2E test scenarios | Add: install AllNamespaces, create DPA in operator namespace, verify Velero deploys. Verify `WATCH_NAMESPACE` resolves to operator namespace. Verify singleton enforcement. Verify sub-controller namespace config. |
| Migration test (Option A only) | Test switching channel on an existing Subscription and swapping the OperatorGroup to verify the documented migration path works end-to-end. |

#### Validation

Full e2e suite passes with both `make deploy-olm` (OwnNamespace) and `make deploy-olm-allnamespaces`.

#### Risk: Medium

Test infrastructure changes are additive.

### Phase 4: CI/Prow Integration

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

### Phase 5: Migration Documentation

Customers migrating from OwnNamespace to AllNamespaces need clear, tested manual steps.
OLM does not automate the OperatorGroup swap.
The migration path differs depending on the delivery option chosen in Phase 2.

Note: after migration, the operator's runtime behavior is identical.
`WATCH_NAMESPACE` is sourced from `metadata.namespace` in the AllNamespaces CSV, so it always resolves to the operator's own namespace.
The operator does not start watching all namespaces.

#### Prerequisites (both options)

- Cluster admin access.
- No active backups or restores in progress.
- Current OADP version supports the AllNamespaces CSV (minimum version TBD).

#### Migration Path for Option A (Channels)

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

**Rollback (Option A):**

1. Delete the global OperatorGroup.
2. Create the namespaced OperatorGroup with `targetNamespaces: [openshift-adp]`.
3. Switch the subscription channel back to the OwnNamespace channel.

#### Migration Path for Option B (Packages)

**Step 1: Verify current state**

```bash
oc get subscription oadp-operator -n openshift-adp -o yaml
oc get operatorgroup -n openshift-adp -o yaml
oc get dpa -n openshift-adp
```

**Step 2: Delete the existing subscription (keeps the DPA and Velero resources)**

```bash
oc delete subscription oadp-operator -n openshift-adp
```

**Step 3: Delete the OwnNamespace CSV**

```bash
CSV_NAME=$(oc get csv -n openshift-adp -o name | grep oadp-operator)
oc delete $CSV_NAME -n openshift-adp
```

The operator pod is removed. DPA and Velero resources remain.

**Step 4: Delete the existing namespaced OperatorGroup**

```bash
oc delete operatorgroup oadp-operator-group -n openshift-adp
```

**Step 5: Create a global OperatorGroup**

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

**Step 6: Create a new subscription to the AllNamespaces package**

```bash
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: oadp-operator-allnamespaces
  namespace: openshift-adp
spec:
  channel: stable-1.x
  name: oadp-operator-allnamespaces
  source: oadp-operator-catalog
  sourceNamespace: openshift-marketplace
  installPlanApproval: Automatic
EOF
```

OLM installs the AllNamespaces CSV.
`WATCH_NAMESPACE` is sourced from `metadata.namespace`, so the operator watches only the `openshift-adp` namespace.
The operator reconciles the existing DPA. Behavior is identical to before the migration.

**Step 7: Verify the migration**

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

**Rollback (Option B):**

1. Delete the AllNamespaces subscription and CSV.
2. Delete the global OperatorGroup.
3. Create the namespaced OperatorGroup with `targetNamespaces: [openshift-adp]`.
4. Create a new Subscription to the original `oadp-operator` package.

#### Expected Downtime (both options)

Brief operator unavailability during the OperatorGroup swap (seconds to approximately one minute).
No impact on existing backups at rest.
In-flight backups or restores should be completed before migration.
Option B has a slightly longer window because the subscription must also be re-created.

## Alternatives Considered

### Single CSV with both install modes enabled

OLM allows a CSV to declare multiple `installModes` as `supported: true`, with the OperatorGroup determining which is active.
This was rejected because the two install modes cannot be enabled simultaneously, and having a single CSV that advertises both creates ambiguity about which mode is in use.
Two separate CSVs make the install mode explicit and independently releasable.

## Security Considerations

The `velero` ServiceAccount's ClusterRole grants near-cluster-admin permissions (`apiGroups: ['*'], resources: ['*']`).
In `OwnNamespace` mode this is contained by the OperatorGroup scope.
In `AllNamespaces` mode the RBAC is identical (already cluster-scoped), but the perception of blast radius changes.
Since the operator's runtime behavior is unchanged (still watches only its own namespace), the actual security posture does not change.
A security review of the velero SA permissions is still recommended as a general hygiene item.

## Compatibility

- Existing `OwnNamespace` installations are unaffected. The OwnNamespace CSV continues to exist and receive updates regardless of delivery option.
- Migration from `OwnNamespace` to `AllNamespaces` is a manual process documented in Phase 5. The exact steps depend on the delivery option chosen in Phase 2.
- No Go code changes are required. The operator binary is identical for both CSVs.
- Runtime behavior is identical in both modes: `WATCH_NAMESPACE` always resolves to the operator's own namespace.

## Implementation

| Phase | Scope | Risk | Depends on | Parallelizable |
|---|---|---|---|---|
| 1. Enable AllNamespaces in CSV | CSV metadata | Low | None | No (foundation) |
| 2. Two bundle variants | Build infrastructure | Medium | Phase 1 | No |
| 3. E2E test infrastructure | Test infrastructure | Medium | Phase 2 | No |
| 4. CI/Prow integration | CI config | Low | Phase 3 | Yes (with Phase 5) |
| 5. Migration documentation | Docs | Low | Phase 2 | Yes (with Phase 4) |

## Open Issues

1. **Delivery option**: Two channels in the same package (Option A) or two separate OLM packages (Option B)?
This decision must be made in Phase 2 and affects Phases 3, 4, and 5.
Key factors: downstream release pipeline compatibility (Konflux/ART), migration UX priority, and long-term deprecation strategy for OwnNamespace.
See the Decision Criteria section in Phase 2 for details.

2. **Channel naming convention (Option A only)**: If channels are chosen, proposed convention is `<existing-channel>-allnamespaces` (e.g., `dev-allnamespaces`, `stable-1.6-allnamespaces`).
Open to shorter alternatives if the convention is too verbose.

3. **Minimum version for migration**: Which OADP release will be the first to ship the AllNamespaces CSV?
This determines the migration documentation's version requirements.

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

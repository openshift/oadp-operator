# AllNamespaces Install Mode for OADP Operator

## Abstract

OADP operator currently supports only `OwnNamespace` install mode via OLM.
This proposal describes a phased plan to add `AllNamespaces` install mode support, delivered via a second CSV to allow customers to migrate at their own pace.
Two delivery options are presented for the second CSV: a separate channel within the existing OLM package, or a separate OLM package with its own catalog entry.

## Background

The OADP operator is installed via OLM with a strict `OwnNamespace` install mode.
The CSV declares only `OwnNamespace: true` and all other modes are `supported: false`.
At runtime, the `WATCH_NAMESPACE` environment variable serves double duty: it identifies both the namespace where the operator lives and the namespace scope for the controller-runtime cache.

In `OwnNamespace` mode, `WATCH_NAMESPACE` is sourced from the `olm.targetNamespaces` CSV annotation (set by the OperatorGroup's `targetNamespaces`), which always resolves to the operator's own namespace.
This conflation works today but breaks in `AllNamespaces` mode, where `olm.targetNamespaces` is empty (meaning "watch everything") but the operator still needs to know its own namespace for PSA labeling, STS credential setup, CLI/VMDP downloads, and sub-controller configuration.

RBAC is already cluster-scoped (ClusterRoles and ClusterRoleBindings), so no fundamental RBAC changes are required.
No webhooks are currently enabled.
A `ClusterWideClient` (uncached) already exists for cross-namespace DPA validation.

## Goals

- Enable `AllNamespaces` install mode via a second CSV, delivered alongside the existing `OwnNamespace` CSV.
- Provide a documented migration path for customers moving from `OwnNamespace` to `AllNamespaces`.
- Maintain full backward compatibility for existing `OwnNamespace` installations.

## Non Goals

- Multi-tenant Velero (one DPA per namespace) is out of scope for the initial implementation. Global DPA singleton enforcement will be maintained.
- Removing `OwnNamespace` support. Both modes will coexist until a future deprecation decision.
- `SingleNamespace` or `MultiNamespace` install mode support.

## High-Level Design

The work is split into six phases, each independently mergeable:

1. **Decouple `OPERATOR_NAMESPACE` from `WATCH_NAMESPACE`** (refactoring, no behavioral change).
2. **Handle empty `WATCH_NAMESPACE`** in the controller-runtime manager (enables AllNamespaces code path).
3. **Build infrastructure for two bundle variants** (kustomize overlays, Makefile targets, two CSVs). Delivery mechanism (channels vs packages) is decided in this phase.
4. **E2E test infrastructure** for both install modes.
5. **CI/Prow integration** with AllNamespaces-specific jobs.
6. **Migration documentation** for customers.

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

### Phase 1: Decouple OPERATOR_NAMESPACE from WATCH_NAMESPACE

Introduce `OPERATOR_NAMESPACE` as a distinct concept from `WATCH_NAMESPACE`.
Today `WATCH_NAMESPACE` is used for both "where the operator lives" and "what to watch."
In `AllNamespaces` mode these diverge: `WATCH_NAMESPACE` is empty (watch all) but the operator still needs to know its home namespace.

#### Changes

| File | Change | Lines |
|---|---|---|
| `config/manager/manager.yaml` | Add `OPERATOR_NAMESPACE` env var via downward API (`metadata.namespace`) | near 63 |
| `cmd/main.go` | Add `getOperatorNamespace()` helper, modeled on `getWatchNamespace()` | near 348 |
| `cmd/main.go` | `addPodSecurityPrivilegedLabels()` uses `operatorNamespace` instead of `watchNamespace` | 140 |
| `cmd/main.go` | `CLIDownloadSetup` / `VMDPDownloadSetup` `Namespace` and `OperatorNamespace` use `operatorNamespace` | 312-329 |
| `pkg/credentials/stsflow/stsflow.go` | Read `OPERATOR_NAMESPACE` instead of `WATCH_NAMESPACE` | 115 |
| `internal/controller/nonadmin_controller.go` | Propagate `OPERATOR_NAMESPACE` (resolves existing TODO at line 176) | 176 |
| `internal/controller/kubevirt_datamover_controller.go` | Propagate `OPERATOR_NAMESPACE` | 152 |
| `internal/controller/vmfilerestore_controller.go` | Propagate `OPERATOR_NAMESPACE` | 184 |

#### Validation

- `make test` passes.
- Existing e2e tests pass unchanged (both vars resolve to the same value under OwnNamespace).

#### Risk: Low

Pure refactoring. No behavioral change.

### Phase 2: Handle Empty WATCH_NAMESPACE in the Manager

Make the controller-runtime manager work correctly when `WATCH_NAMESPACE` is empty, which signals AllNamespaces mode.

#### Changes

| File | Change | Lines |
|---|---|---|
| `cmd/main.go` | Conditional cache config: skip `DefaultNamespaces` when `watchNamespace` is empty (cache watches all namespaces) | 204-208 |
| `cmd/main.go` | `getWatchNamespace()`: empty or unset is now valid; log info instead of error | 127-131, 348-360 |
| `cmd/main.go` | Remove the `watchNamespace == ""` skip for CLI/VMDP setup (these use `operatorNamespace` from Phase 1) | 305-306 |
| `cmd/main.go` | `addPodSecurityPrivilegedLabels`: already fixed in Phase 1 to use `operatorNamespace`; verify empty-string guard is removed | 362-368 |

#### DPA Singleton Enforcement

`validator.go` (line 144) uses `ClusterWideClient` to list all DPAs cluster-wide and enforce singleton constraints (NonAdminController, VolumeSnapshotMover).
In AllNamespaces mode, global DPA singleton enforcement is maintained (one DPA across the entire cluster).
Multi-tenant (one DPA per namespace) is a future enhancement.

#### Validation

- Unit tests for the empty `WATCH_NAMESPACE` code path.
- Manual testing with `WATCH_NAMESPACE=""` and `OPERATOR_NAMESPACE=openshift-adp`.

#### Risk: Medium

Behavioral change for the empty-namespace path, but that path is not reachable until Phase 3 ships an AllNamespaces CSV.

### Phase 3: Build Infrastructure for Two Bundle Variants

Produce two distinct OLM bundles from one codebase.
A single CSV cannot have both `OwnNamespace` and `AllNamespaces` enabled simultaneously, so two CSVs are required.
The delivery mechanism for the second CSV is decided in this phase.

#### Common Build Changes (both options)

Regardless of delivery option, the following build changes are needed to produce two bundle variants from one codebase:

| Area | Change |
|---|---|
| **CSV base template** | Keep `config/manifests/bases/oadp-operator.clusterserviceversion.yaml` as the shared base |
| **Kustomize overlays** | Create `config/manifests/overlays/ownnamespace/` and `config/manifests/overlays/allnamespaces/` with patches for `installModes` and deployment env vars |
| **AllNamespaces overlay** | Patches: (1) `installModes` set to only `AllNamespaces: true`, (2) add `OPERATOR_NAMESPACE` env var to deploymentSpec, (3) `WATCH_NAMESPACE` source remains `olm.targetNamespaces` (will be empty with global OperatorGroup) |
| **OwnNamespace overlay** | Patches: keeps current behavior (only `OwnNamespace: true`). Identity transform initially. |
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

### Phase 4: E2E Test Infrastructure

Tests need to validate both install modes.
The test infrastructure changes depend on the delivery option chosen in Phase 3.

#### Changes

| Area | Change |
|---|---|
| `Makefile` `deploy-olm` (line 634-642) | Parameterize OperatorGroup creation: when `INSTALL_MODE=AllNamespaces`, create OperatorGroup with empty `spec` (no `targetNamespaces`). Default keeps current behavior. |
| `Makefile` | New target: `deploy-olm-allnamespaces`. For Option A (channels): sets `INSTALL_MODE=AllNamespaces` and `DEFAULT_CHANNEL=dev-allnamespaces`. For Option B (packages): sets `INSTALL_MODE=AllNamespaces` and overrides `CATALOG_SOURCE_NAME` and subscription package name. |
| `tests/e2e/upgrade_suite_test.go` (lines 31-50) | Parameterize OperatorGroup creation to support both modes based on a test flag |
| E2E test scenarios | Add: install AllNamespaces, create DPA in operator namespace, verify Velero deploys. Verify singleton enforcement. Verify sub-controller namespace config. |
| Migration test (Option A only) | Test switching channel on an existing Subscription and swapping the OperatorGroup to verify the documented migration path works end-to-end. |

#### Validation

Full e2e suite passes with both `make deploy-olm` (OwnNamespace) and `make deploy-olm-allnamespaces`.

#### Risk: Medium

Test infrastructure changes are additive.

### Phase 5: CI/Prow Integration

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

### Phase 6: Migration Documentation

Customers migrating from OwnNamespace to AllNamespaces need clear, tested manual steps.
OLM does not automate the OperatorGroup swap.
The migration path differs depending on the delivery option chosen in Phase 3.

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

OLM re-deploys the operator with `olm.targetNamespaces` empty.
`WATCH_NAMESPACE` becomes empty.
AllNamespaces mode is now active.

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

OLM installs the AllNamespaces CSV with `olm.targetNamespaces` empty.
The operator starts in AllNamespaces mode and reconciles the existing DPA.

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
A security review of the velero SA permissions should be conducted as part of Phase 3.

## Compatibility

- Existing `OwnNamespace` installations are unaffected. The OwnNamespace CSV continues to exist and receive updates regardless of delivery option.
- Migration from `OwnNamespace` to `AllNamespaces` is a manual process documented in Phase 6. The exact steps depend on the delivery option chosen in Phase 3.
- The `OPERATOR_NAMESPACE` env var (Phase 1) is additive and backward-compatible.
- The `WATCH_NAMESPACE` empty-string handling (Phase 2) does not affect existing deployments where the var is always set to a non-empty value.

## Implementation

| Phase | Scope | Risk | Depends on | Parallelizable |
|---|---|---|---|---|
| 1. Decouple OPERATOR_NAMESPACE | Refactoring | Low | None | No (foundation) |
| 2. Handle empty WATCH_NAMESPACE | Runtime behavior | Medium | Phase 1 | No |
| 3. Two bundle variants | Build infrastructure | Medium | Phase 2 | No |
| 4. E2E test infrastructure | Test infrastructure | Medium | Phase 3 | No |
| 5. CI/Prow integration | CI config | Low | Phase 4 | Yes (with Phase 6) |
| 6. Migration documentation | Docs | Low | Phase 3 | Yes (with Phase 5) |

## Open Issues

1. **Delivery option**: Two channels in the same package (Option A) or two separate OLM packages (Option B)?
This decision must be made in Phase 3 and affects Phases 4, 5, and 6.
Key factors: downstream release pipeline compatibility (Konflux/ART), migration UX priority, and long-term deprecation strategy for OwnNamespace.
See the Decision Criteria section in Phase 3 for details.

2. **DPA singleton scope**: In AllNamespaces mode, should we allow one DPA per namespace (multi-tenant Velero) or enforce a single DPA globally?
Initial recommendation is global singleton to minimize blast radius; multi-tenant support can follow as a separate enhancement.

3. **Channel naming convention (Option A only)**: If channels are chosen, proposed convention is `<existing-channel>-allnamespaces` (e.g., `dev-allnamespaces`, `stable-1.6-allnamespaces`).
Open to shorter alternatives if the convention is too verbose.

4. **Velero operational scope**: Even when the operator watches all namespaces, Velero's deployment lives in one namespace and backs up resources across namespaces (this is existing behavior).
No change expected, but worth validating in e2e tests.

5. **Minimum version for migration**: Which OADP release will be the first to ship the AllNamespaces CSV?
This determines the migration documentation's version requirements.

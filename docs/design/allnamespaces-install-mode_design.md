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
See [HackMD test log](https://hackmd.io/ZAjwOe39SjWv2yCWIlGdzg) for the full chronological record.

Key results:
- CSV reached `Succeeded` with a global OperatorGroup in `openshift-adp`
- `WATCH_NAMESPACE` resolved to `openshift-adp` inside the pod
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

The change is a CSV metadata patch — no Go code changes, no new channels, no kustomize overlays, no catalog restructuring.

Three changes to the CSV:

1. Enable `AllNamespaces: true` in `installModes` (alongside the existing `OwnNamespace: true`).
2. Change `WATCH_NAMESPACE` source from `metadata.annotations['olm.targetNamespaces']` to `metadata.namespace`.
3. Add `permissions` (namespace-scoped Roles) entries for `non-admin-controller` and `velero` ServiceAccounts.

Existing OwnNamespace installations are unaffected — under a namespaced OperatorGroup, `metadata.namespace` and `olm.targetNamespaces` resolve to the same value.

## Detailed Design

### Current State

| Area | Current Behavior | Key File(s) |
|---|---|---|
| CSV installModes | Only `OwnNamespace: true` | `config/manifests/bases/oadp-operator.clusterserviceversion.yaml` (lines 463-471) |
| CSV WATCH_NAMESPACE source | `olm.targetNamespaces` annotation | `bundle/manifests/oadp-operator.clusterserviceversion.yaml` (line 1105) |
| Manager WATCH_NAMESPACE source | `metadata.namespace` (downward API) | `config/manager/manager.yaml` (lines 57-60) |
| CSV permissions | Only `openshift-adp-controller-manager` has a `permissions` entry | `bundle/manifests/oadp-operator.clusterserviceversion.yaml` (line 1183) |
| CSV clusterPermissions | Three SAs: `non-admin-controller`, `openshift-adp-controller-manager`, `velero` | `bundle/manifests/oadp-operator.clusterserviceversion.yaml` (line 727) |
| Cache scoping | `DefaultNamespaces` map with single entry | `cmd/main.go` (lines 204-208) |
| PSA labeling | Patches `watchNamespace`, errors if empty | `cmd/main.go` (lines 362-389) |
| CLI/VMDP downloads | Skipped if `watchNamespace` is empty | `cmd/main.go` (lines 305-306) |
| STS flow | Reads `WATCH_NAMESPACE` as install namespace | `pkg/credentials/stsflow/stsflow.go` (line 115) |
| Sub-controllers | All receive `WATCH_NAMESPACE` = own namespace | `nonadmin_controller.go` (line 176), `kubevirt_datamover_controller.go` (line 152), `vmfilerestore_controller.go` (line 184) |
| E2E deploy | `operator-sdk run bundle` (implicitly OwnNamespace) | `Makefile` (line 459) |
| Suggested namespace | `operatorframework.io/suggested-namespace: openshift-adp` | `config/manifests/bases/oadp-operator.clusterserviceversion.yaml` (line 24) |
| RBAC | Already cluster-scoped (ClusterRoles) | `config/rbac/role.yaml`, CSV `clusterPermissions` |

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

### Change 2: WATCH_NAMESPACE Source

The `WATCH_NAMESPACE` env var in the CSV deployment spec must be changed from `olm.targetNamespaces` to `metadata.namespace`.

Currently, `operator-sdk generate bundle` automatically rewrites `metadata.namespace` to `metadata.annotations['olm.targetNamespaces']` during bundle generation.
This must be patched after generation, or the bundle generation process must be modified to preserve `metadata.namespace`.

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

### Change 3: Add Permissions for Missing ServiceAccounts

In AllNamespaces mode, OLM requires every ServiceAccount declared in `clusterPermissions` to also have a corresponding `permissions` (namespace-scoped Role) entry.
Without this, OLM cannot create the ServiceAccounts and the CSV stays `Pending` with `"no owned roles found"`.

The CSV currently declares `clusterPermissions` for three SAs but only has a `permissions` entry for `openshift-adp-controller-manager` (leader-election Role).

Add `permissions` entries for `non-admin-controller` and `velero` SAs with leader-election rules (configmaps, leases, events).
These are the same rules already used by the existing `openshift-adp-controller-manager` permissions entry.

The corresponding RBAC config files also need updating:
- `config/non-admin-controller_rbac/` — add a `leader_election_role.yaml` and binding
- `config/velero/` — add a `leader_election_role.yaml` and binding

### What does NOT change

- No Go code changes. `cmd/main.go`, controllers, and all runtime behavior are untouched.
- `WATCH_NAMESPACE` always resolves to a non-empty namespace name (the pod's own namespace).
- Cache scoping, PSA labeling, STS flow, CLI/VMDP downloads, sub-controller propagation all continue to work exactly as today.
- RBAC `clusterPermissions` (ClusterRoles and ClusterRoleBindings) are unchanged.
- The operator binary is identical — the same image is used regardless of install mode.
- The `operatorframework.io/suggested-namespace: openshift-adp` annotation remains, ensuring OperatorHub defaults to `openshift-adp` for both install modes.

### OperatorHub User Experience

When both install modes are enabled, the OperatorHub UI (OpenShift 4.14+) presents:

1. **Install mode selection** — radio buttons: "All namespaces on the cluster" and "A specific namespace on the cluster"
2. **Namespace selection** — dropdown defaulting to `openshift-adp` (driven by the `suggested-namespace` annotation) in both modes

The `suggested-namespace` annotation ensures the operator installs in `openshift-adp` regardless of which install mode the user selects.
This follows the established pattern used by the Loki Operator (`openshift-operators-redhat`) and OpenShift Serverless (`openshift-serverless`).

## Implementation

The work is split into three phases:

| Phase | Scope | Risk | Depends on |
|---|---|---|---|
| 1. CSV changes | CSV metadata (installModes, WATCH_NAMESPACE source, permissions) | Low | None |
| 2. Deploy + CI for AllNamespaces | Makefile target + Prow job | Low | Phase 1 |
| 3. Migration documentation | Docs for OperatorGroup swap | Low | Phase 1 |

### Phase 1: CSV Changes

Apply the three changes described above.

| File | Change |
|---|---|
| `config/manifests/bases/oadp-operator.clusterserviceversion.yaml` | Set `AllNamespaces: supported: true` |
| `bundle/manifests/oadp-operator.clusterserviceversion.yaml` | Change `WATCH_NAMESPACE` source to `metadata.namespace` (post-generation patch or Makefile sed command) |
| `bundle/manifests/oadp-operator.clusterserviceversion.yaml` | Add `permissions` entries for `non-admin-controller` and `velero` SAs |
| `config/non-admin-controller_rbac/` | Add `leader_election_role.yaml` and `leader_election_role_binding.yaml` |
| `config/velero/` | Add `leader_election_role.yaml` and `leader_election_role_binding.yaml` |
| `Makefile` `bundle` target | Add post-generation step to replace `olm.targetNamespaces` with `metadata.namespace` in the generated CSV |

#### Validation

- `make bundle` produces a valid bundle with both install modes enabled.
- `opm validate` passes.
- `operator-sdk run bundle --install-mode AllNamespaces` succeeds on a cluster (verified 2026-08-13).
- `operator-sdk run bundle --install-mode OwnNamespace` still works (backward compatibility).
- `WATCH_NAMESPACE` resolves to the pod's namespace in both modes.

#### Risk: Low

CSV metadata changes only. No runtime behavioral change. Backward-compatible with existing OwnNamespace installations.

### Phase 2: Deploy and CI for AllNamespaces

Add the ability to deploy and test with AllNamespaces mode, then wire up CI.

`operator-sdk run bundle` supports `--install-mode AllNamespaces` as a flag.

| Area | Change |
|---|---|
| **Makefile** | New target `deploy-olm-allnamespaces` that runs `operator-sdk run bundle --install-mode AllNamespaces --security-context-config restricted $(THIS_BUNDLE_IMAGE) --namespace $(OADP_TEST_NAMESPACE)` |
| **`openshift/release` config** | Add new presubmit job (e.g., `e2e-aws-allnamespaces`) that runs `make deploy-olm-allnamespaces` then `make test-e2e` |
| `tests/e2e/upgrade_suite_test.go` (lines 31-50) | Parameterize OperatorGroup creation to support both modes based on a test flag or env var |

#### What this validates

- The full e2e suite passes with AllNamespaces mode: DPA creation, Velero deployment, backup/restore operations, sub-controllers, credential management.
- Any failures reveal real incompatibilities.

#### Risk: Low

One new Makefile target and one new CI job. The existing e2e suite is the test.

### Phase 3: Migration Documentation

Document how existing OwnNamespace customers can switch to AllNamespaces mode.

Since both install modes are supported in the same CSV, migration is a single step: swap the OperatorGroup.
No channel switch or CSV change is needed.

#### Prerequisites

- Cluster admin access.
- No active backups or restores in progress.
- OADP version that supports AllNamespaces (the version containing Phase 1 changes).

#### Migration Steps

**Step 1: Verify current state**

```bash
oc get operatorgroup -n openshift-adp -o yaml
oc get csv -n openshift-adp
oc get dpa -n openshift-adp
```

**Step 2: Delete the existing namespaced OperatorGroup**

```bash
oc delete operatorgroup <operatorgroup-name> -n openshift-adp
```

The operator pod stops (OLM removes the deployment when no valid OperatorGroup exists).

**Step 3: Create a global OperatorGroup**

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

**Step 4: Verify the migration**

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
2. Create a namespaced OperatorGroup with `targetNamespaces: [openshift-adp]`.

No channel switch or CSV downgrade is needed — the same CSV supports both modes.

#### Expected Downtime

Brief operator unavailability during the OperatorGroup swap (seconds to approximately one minute).
No impact on existing backups at rest.
In-flight backups or restores should be completed before migration.

## Alternatives Considered

### Two separate channels (one per install mode)

A separate AllNamespaces channel with its own CSV was considered.
This was rejected because a single CSV can support both install modes simultaneously — OLM uses the OperatorGroup to determine the active mode.
The single-CSV approach eliminates the need for two channels, two bundles, kustomize overlays, and catalog changes.
It also avoids the OLM upgrade deadlock that would occur if a future release dropped OwnNamespace support (the two-channel approach would have required both channels to coexist for at least one release cycle).

### Two separate OLM packages

A separate package (e.g., `oadp-operator-allnamespaces`) would provide clean separation but requires customers to uninstall and reinstall to migrate.
The single-CSV approach avoids this entirely — customers only swap their OperatorGroup.

## Security Considerations

The `velero` ServiceAccount's ClusterRole grants near-cluster-admin permissions (`apiGroups: ['*'], resources: ['*']`).
These permissions are declared as `clusterPermissions` in the CSV and bound via `ClusterRoleBinding`, which is cluster-scoped regardless of the OperatorGroup's install mode.
The `OwnNamespace` OperatorGroup does not restrict or contain these permissions — the RBAC posture is identical in both `OwnNamespace` and `AllNamespaces` modes.
Since the operator's runtime behavior is also unchanged (still watches only its own namespace), the actual security posture does not change.
A security review of the velero SA permissions is recommended as a general hygiene item, independent of this install mode change.

## Compatibility

- Existing `OwnNamespace` installations are fully backward-compatible. The CSV supports both modes, so no OperatorGroup change is required to continue running as OwnNamespace.
- Upgrading from a prior release to the version with this change does not alter the customer's install mode — their existing namespaced OperatorGroup stays in place.
- Migration to AllNamespaces is optional and requires an explicit OperatorGroup swap (documented in Phase 3).
- No Go code changes are required. The operator binary is identical.
- Runtime behavior is identical in both modes: `WATCH_NAMESPACE` always resolves to the operator's own namespace.
- Under OwnNamespace, `metadata.namespace` and `olm.targetNamespaces` resolve to the same value — the change is transparent.

## Open Issues

1. **`operator-sdk generate bundle` override**: `operator-sdk generate bundle` automatically replaces `metadata.namespace` with `metadata.annotations['olm.targetNamespaces']` in the generated CSV. A post-generation patch (e.g., `sed` in the Makefile) or upstream SDK configuration is needed to preserve `metadata.namespace`.

2. **Minimum version**: Which OADP release will include this change?

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

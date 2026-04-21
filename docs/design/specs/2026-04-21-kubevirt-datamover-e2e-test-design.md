# Kubevirt Datamover E2E Test Design

## Summary

Add a basic E2E test to `virt_backup_restore_suite_test.go` that validates the kubevirt-datamover backup path: enabling CBT on a CirrOS VM via HCO configuration and VM labels, deploying OADP with the `kubevirt-datamover` plugin, triggering a Velero backup with `SnapshotMoveData=true`, and verifying that a `VirtualMachineBackupTracker` is created (proving the kubevirt-datamover controller processed the backup).

## Approach

Extend the existing HCO/OLM-based virt test infrastructure. No new KubeVirt install paths.

## Prerequisites

- HCO version with KubeVirt >= v1.7 (CBT support, released Nov 2025)
- kubevirt-datamover-controller deployed (handled by OADP operator when `kubevirt-datamover` plugin is enabled)
- kubevirt-datamover-plugin image available (Velero init container, configured via DPA `DefaultPluginKubeVirtDataMover`)

---

## Existing KubeVirt Installation Flow (No Changes)

The existing virt test suite installs KubeVirt via the HyperConverged Cluster Operator (HCO) through OLM. This flow is in `tests/e2e/lib/virt_helpers.go` and is driven by the `BeforeAll` in `virt_backup_restore_suite_test.go`. The test does NOT install raw upstream KubeVirt; it always uses HCO.

### Step-by-step existing flow

1. **`GetVirtOperator(client, clientset, dynamicClient, useUpstreamHco)`** (line 65)
   - Selects namespace and OLM package based on `upstream` flag:
     - OpenShift (default): namespace `openshift-cnv`, PackageManifest `kubevirt-hyperconverged`, catalog `redhat-operators`
     - Upstream (`HCO_UPSTREAM=true`): namespace `kubevirt-hyperconverged`, PackageManifest `community-kubevirt-hyperconverged`, catalog `community-operators`
   - Reads the `stable` channel from the PackageManifest to get the current CSV name and version

2. **`EnsureVirtInstallation()`** (line 732) — only runs if HCO is not already present
   - `EnsureNamespace(v.Namespace)` — creates `openshift-cnv` or `kubevirt-hyperconverged`
   - `ensureOperatorGroup()` — creates OperatorGroup (upstream uses empty `TargetNamespaces`)
   - `ensureSubscription()` — creates OLM Subscription pointing to the catalog/channel/CSV
   - `ensureCsv(5min)` — waits for the ClusterServiceVersion to be ready
   - `ensureHco(5min)` — creates the `HyperConverged` CR and waits for health

3. **`installHco()`** (line 339) — creates the HyperConverged CR with **empty spec**:
   ```yaml
   apiVersion: hco.kubevirt.io/v1beta1
   kind: HyperConverged
   metadata:
     name: kubevirt-hyperconverged
     namespace: <v.Namespace>
   spec: {}
   ```
   HCO then creates and manages the KubeVirt CR, CDI, and other operands.

4. **Optional: KVM emulation** (`EnsureEmulation()`, line 686)
   - Only when `kvmEmulation=true` (cloud clusters without nested virt)
   - Patches the HCO CR's **annotation** `kubevirt.kubevirt.io/jsonpatch` with:
     ```json
     [{"op": "add", "path": "/spec/configuration/developerConfiguration", "value": {"useEmulation": true}}]
     ```
   - HCO applies this JSON patch to the KubeVirt CR it manages

5. **CirrOS boot image setup**
   - Downloads latest CirrOS image URL
   - Creates DataVolume `cirros` in `openshift-virtualization-os-images` namespace
   - Creates DataSource from the PVC

6. **DPA plugin configuration**
   - Appends `DefaultPluginKubeVirt` to `dpaCR.VeleroDefaultPlugins`

7. **Storage classes and RBAC**
   - Creates `test-sc-immediate` and `test-sc-wffc` StorageClasses
   - Installs `cirros-rbac.yaml`

### Key point: HCO annotations propagate to KubeVirt CR

HCO manages the KubeVirt CR. Direct edits to the KubeVirt CR are overwritten by HCO. To inject KubeVirt-level configuration that HCO doesn't directly expose, the pattern is to use the `kubevirt.kubevirt.io/jsonpatch` annotation on the HCO CR. This is already used for KVM emulation (step 4 above).

---

## CBT Enablement: Two Separate Configurations Required

Enabling ChangedBlockTracking on a VM requires two distinct cluster-level configurations plus a per-VM label.

**Note:** An older setup procedure used three `kubevirt.kubevirt.io/jsonpatch` operations to inject `IncrementalBackup` and `UtilityVolumes` feature gates plus the label selector. That is **no longer necessary** — HCO now exposes `incrementalBackup` as a first-class feature gate, and enabling it automatically enables `UtilityVolumes`. Only the label selector still requires the jsonpatch annotation.

### Configuration 1: HCO Feature Gate (`incrementalBackup`)

The HCO CR has a feature gate `spec.featureGates.incrementalBackup` (default: `false`). Setting this to `true`:
- Enables the `IncrementalBackup` feature gate in the KubeVirt CR
- Automatically enables the `UtilityVolumes` feature gate (required for backup output storage)
- This is a Tech Preview feature (Alpha graduation)

**What to set on HCO:**
```yaml
apiVersion: hco.kubevirt.io/v1beta1
kind: HyperConverged
metadata:
  name: kubevirt-hyperconverged
spec:
  featureGates:
    incrementalBackup: true
```

This is a direct field on the HCO spec, so it can be set via a standard merge patch on the HCO resource (no annotation needed).

### Configuration 2: KubeVirt Label Selector (`changedBlockTrackingLabelSelectors`)

The KubeVirt CR has a configuration field `spec.configuration.changedBlockTrackingLabelSelectors` that tells KubeVirt which VMs should have CBT enabled, using label selectors. This field is on the **KubeVirt CR**, not directly exposed by HCO.

**What to set on KubeVirt CR:**
```yaml
apiVersion: kubevirt.io/v1
kind: KubeVirt
spec:
  configuration:
    changedBlockTrackingLabelSelectors:
      virtualMachineLabelSelector:
        matchLabels:
          changedBlockTracking: "true"
```

Since HCO manages the KubeVirt CR and overwrites direct edits, this must be injected via the `kubevirt.kubevirt.io/jsonpatch` annotation on the HCO CR (same mechanism as KVM emulation):

```json
[{"op": "add", "path": "/spec/configuration/changedBlockTrackingLabelSelectors", "value": {"virtualMachineLabelSelector": {"matchLabels": {"changedBlockTracking": "true"}}}}]
```

**Important:** The `kubevirt.kubevirt.io/jsonpatch` annotation is a single annotation holding a JSON array of patch operations. If KVM emulation is also enabled, both patches must be combined into one annotation value:
```json
[
  {"op": "add", "path": "/spec/configuration/developerConfiguration", "value": {"useEmulation": true}},
  {"op": "add", "path": "/spec/configuration/changedBlockTrackingLabelSelectors", "value": {"virtualMachineLabelSelector": {"matchLabels": {"changedBlockTracking": "true"}}}}
]
```

### Configuration 3: VM Label

The VM itself must carry the matching label. This is baked into the VM manifest:
```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  labels:
    changedBlockTracking: "true"
```

### VM Restart Required

Even with the label present from VM creation, a restart cycle is required for KubeVirt to:
1. Create a backend storage PVC
2. Create a qcow2 overlay on top of the raw disk
3. Update the VM's domain XML

After restart, the VM's `status.ChangedBlockTracking.State` transitions to `Enabled`.

### Full CBT activation sequence in the test

```
1. EnsureVirtInstallation()          — existing flow, installs HCO with empty spec
2. EnableCBTFeatureGate()            — patch HCO: spec.featureGates.incrementalBackup = true
3. EnableCBTLabelSelector()          — patch HCO annotation: jsonpatch to set changedBlockTrackingLabelSelectors on KubeVirt CR
4. Deploy CirrOS VM with label       — template has changedBlockTracking: "true"
5. Wait for VM Running
6. Restart VM (stop + start)         — required for qcow2 overlay creation
7. Wait for VM Running again
8. Wait for status.ChangedBlockTracking.State == "Enabled"
9. Proceed with backup
```

Steps 2 and 3 are idempotent and can be placed in `BeforeAll` so they run once for the entire suite.

---

## Changes

### 1. New CirrOS VM template with CBT label

**File:** `tests/e2e/sample-applications/virtual-machines/cirros-test/cirros-test-cbt.yaml`

Based on existing `cirros-test.yaml`, with the addition of:
- `metadata.labels.changedBlockTracking: "true"` on the VirtualMachine
- Same CirrOS boot image, same storage class, same resource requests

### 2. New VirtOperator methods in `tests/e2e/lib/virt_helpers.go`

#### `EnableCBTFeatureGate() error`

Patches the HCO CR to set `spec.featureGates.incrementalBackup: true`. Uses the dynamic client to get the HCO, set the nested field, and update. Follows the same retry-on-conflict pattern as `EnsureEmulation`.

#### `EnableCBTLabelSelector() error`

Patches the HCO CR's `kubevirt.kubevirt.io/jsonpatch` annotation to inject `changedBlockTrackingLabelSelectors` into the KubeVirt CR. Must handle the case where:
- The annotation doesn't exist yet (create it with just the CBT patch)
- The annotation already has patches (e.g. emulation) — parse the existing JSON array, append the CBT patch if not already present, write back

#### `StartVm(namespace, name string) error`

REST call to the KubeVirt subresource API:
```
PUT /apis/subresources.kubevirt.io/v1/namespaces/{namespace}/virtualmachines/{name}/start
```
Mirrors the existing `StopVm` method.

#### `RestartVmAndWaitRunning(namespace, name string, timeout time.Duration) error`

Stops the VM, waits for Stopped status, starts it, and waits for Running status.

#### `WaitForCBTEnabled(namespace, name string, timeout time.Duration) error`

Polls the VM's `status.changedBlockTracking.state` via the dynamic client until it equals `"Enabled"` or times out.

### 3. DPA configuration in `virt_backup_restore_suite_test.go`

In `BeforeAll`, add `DefaultPluginKubeVirtDataMover` to `dpaCR.VeleroDefaultPlugins` alongside the existing `DefaultPluginKubeVirt`. This causes the OADP operator to:
- Add the kubevirt-datamover-plugin as a Velero init container
- Deploy the kubevirt-datamover-controller Deployment

### 4. New test entry in `virt_backup_restore_suite_test.go`

A new `ginkgo.Entry` in the existing `DescribeTable` with label `"virt"`:

**"no-application kubevirt-datamover backup, CirrOS VM with CBT"**

Uses a modified `runVmBackupAndRestore` flow or a dedicated run function that:

1. Creates DPA (via `prepareBackupAndRestore`)
2. Creates namespace, installs the CBT CirrOS VM template
3. Waits for VM Running
4. Calls `v.EnableCBTFeatureGate()` and `v.EnableCBTLabelSelector()` (idempotent, can also be in BeforeAll)
5. Restarts the VM (`v.RestartVmAndWaitRunning`)
6. Waits for `status.ChangedBlockTracking.State == Enabled` (`v.WaitForCBTEnabled`)
7. Triggers Velero backup (via existing `runBackup` with `CSIDataMover` type for `SnapshotMoveData=true`)
8. **Post-backup verification**: Checks that a `VirtualMachineBackupTracker` CR (`backup.kubevirt.io/v1alpha1`) was created in the VM's namespace. This is the definitive signal that the kubevirt-datamover-controller received and started processing the DataUpload — it creates the VMBT during the Accepted phase before creating the VMB for the actual qcow2 backup.
9. Deletes VM and namespace
10. Runs restore
11. **Post-restore verification**: VM comes back running (restore path is best-effort since the kubevirt-datamover-controller doesn't implement DataDownload reconciliation yet)

### 5. Verification helper

**`verifyVMBackupTrackerExists(dynamicClient, vmNamespace string)`**

Uses the dynamic client to list `VirtualMachineBackupTracker` resources (`backup.kubevirt.io/v1alpha1`) in the VM namespace and asserts at least one exists. This proves the full chain worked: BIA plugin created a DataUpload with `dataMover: kubevirt`, and the kubevirt-datamover-controller reconciled it and created the VMBT.

## Files Changed

| File | Type | Description |
|------|------|-------------|
| `tests/e2e/sample-applications/virtual-machines/cirros-test/cirros-test-cbt.yaml` | New | CirrOS VM template with `changedBlockTracking: "true"` label |
| `tests/e2e/lib/virt_helpers.go` | Modified | Add `EnableCBTFeatureGate`, `EnableCBTLabelSelector`, `StartVm`, `RestartVmAndWaitRunning`, `WaitForCBTEnabled` |
| `tests/e2e/virt_backup_restore_suite_test.go` | Modified | Add `DefaultPluginKubeVirtDataMover` to plugins, add CBT test entry, add VMBT verification |

## Test Labels and Execution

The test entry uses the `"virt"` label (same as existing VM tests), gated by `TEST_VIRT=true`. If the HCO version doesn't support the `incrementalBackup` feature gate or CBT, the enablement steps will fail with a clear error.

## Volume Policy: `custom` Action Type

As of Velero 1.18.1, the upstream `custom` action type for volume policies has been merged ([velero-io/velero#9678](https://github.com/velero-io/velero/pull/9678), fixing [#9505](https://github.com/velero-io/velero/issues/9505)). This allows volume policies to specify a `custom` action with an action parameters map, which the kubevirt-datamover-plugin can inspect to determine if a PVC should use the kubevirt datamover path.

The plugin currently approximates this by checking for PVCs with "skip" policy (no snapshot), but the `custom` action type is the intended long-term mechanism. The E2E test should use a volume policy ConfigMap with the `custom` action type if the Velero version supports it. If the test environment uses Velero < 1.18.1, the existing skip-based fallback in the plugin still works.

Example volume policy with custom action (for future use):
```yaml
version: v1
volumePolicies:
  - conditions:
      pvcLabels:
        changedBlockTracking: "true"
    action:
      type: custom
      parameters:
        provider: kubevirt
```

For the initial E2E test, this is noted as a future enhancement — the test will rely on the plugin's current skip-based volume policy detection.

## Out of Scope

- Restore via kubevirt-datamover (DataDownload controller not implemented)
- Raw upstream KubeVirt daily build installation
- New BackupRestoreType for kubevirt-datamover (reuses `CSIDataMover` for `SnapshotMoveData=true`)
- Implementing `custom` volume policy in the E2E test (plugin's skip-based fallback is sufficient for initial test; `custom` action integration tracked in [kubevirt-datamover-plugin#4](https://github.com/migtools/kubevirt-datamover-plugin/issues/4))

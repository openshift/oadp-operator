# Kubevirt incremental datamover

In a kubevirt environment, one possibility for improving volume backup performance to allow more frequent backups would be to take incremental qcow2 backups using libvirt tools. Integrating this into OADP/Velero backups will require some new components to OADP (plugins, controller), as well as some modifications to the velero codebase.

## Background

Taking a VolumeSnapshot and then using kopia to process the entire volume and copy the required incremental changes into the Backup Storage Location (BSL) is a heavyweight process. Creating an incremental qcow2 backup for the same volume is generally a much more lightweight action. We want to make use of the existing Velero backup/restore process, with actual kubevirt incremental backup/restore happening via a new controller. For the moment, this will be referred to as the Kubevirt Datamover Controller. This action will be coordinated with Velero via existing infrastructure -- BackupItemActions (BIAs), RestoreItemActions (RIAs) and the DataUpload/DataDownload CRs. Initial implementation should require minimal changes to Velero, since Velero currently ignores DataUploads with `Spec.DataMover` set to something other than `velero`.

## Goals

- Back up and restore VM volumes using kubevirt incremental backup instead of Velero's built-in CSI datamover
- Use existing Velero infrastructure to integrate this feature into regular velero backup and restore
- Implementation based on kubevirt enhancement defined at <https://github.com/kubevirt/enhancements/blob/main/veps/sig-storage/incremental-backup.md>
  - There is a follow-on design PR at <https://github.com/kubevirt/enhancements/pull/126/changes> although this mainly discusses pull-mode, which is out of scope for the current design.

## Non goals
- Deep integration with velero data mover pluggability (this could be considered in the long-term though, which would minimize duplication of effort and enhance maintainability)
- Using pull mode with kubevirt.

## Use cases
- As a user I want to use OADP to trigger backups that will back up volume data using kubevirt tooling rather than CSI snapshots
  - Volume backups will be incremental when possible (first backup for a given volume will be full, subsequent backups for that same volume will be incremental)
  - Assuming that kubevirt incremental volume backups should be much faster than CSI snapshots followed by incremental kopia snapshot copy, the expectation is that users might run kubevirt datamover backups more frequently than they would for CSI-based backups.
  - Users should set the volume policy action to `"custom"` with `"datamover: kubevirt"` in the action parameters for VM PVCs in the backup's resource/volume policy configuration. This prevents Velero from also taking CSI snapshots or fs-backups of the same volumes (the BIA plugin skips any PVC whose action is not `"kubevirt"` or `"skip"`).

```
  Key Differences between CSI and KubeVirt incremental snapshots
  ┌────────────────────┬──────────────────────────────────────────┬────────────────────────────────────────────┐
  │       Aspect       │               CSI Approach               │          KubeVirt qcow2 Approach           │
  ├────────────────────┼──────────────────────────────────────────┼────────────────────────────────────────────┤
  │ Layer              │ Storage (CSI driver)                     │ Hypervisor (QEMU/libvirt)                  │
  ├────────────────────┼──────────────────────────────────────────┼────────────────────────────────────────────┤
  │ Snapshot mechanism │ CSI VolumeSnapshot                       │ VirtualMachineBackup CR                    │
  ├────────────────────┼──────────────────────────────────────────┼────────────────────────────────────────────┤
  │ Incremental        │ Kopia deduplication (scans whole volume) │ True block-level CBT (only changed blocks) │
  ├────────────────────┼──────────────────────────────────────────┼────────────────────────────────────────────┤
  │ Data mover         │ Velero's node-agent + kopia              │ New OADP controller + qemu-img             │
  ├────────────────────┼──────────────────────────────────────────┼────────────────────────────────────────────┤
  │ VM awareness       │ None (just sees PVC)                     │ Full (knows it's a VM disk)                │
  └────────────────────┴──────────────────────────────────────────┴────────────────────────────────────────────┘
```

## High-Level design

### Upstream velero changes
- Update velero volume policy model to allow custom volume policy actions. Velero would treat custom actions as "skip" (i.e. no snapshot or fs-backup), but the kubevirt datamover could only act if the policy action is "custom" with "datamover: kubevirt" in "parameters".
- In `pkg/restore/actions/dataupload_retrieve_action.go` and in `DataDownload` we need to add SnapshotType.

### BackupItemAction/RestoreItemAction plugins
All 6 registered in `main.go` (kubevirt-datamover-plugin repo).
- VirtualMachine BIA plugin
  - The plugin will check whether the VirtualMachine's `status.ChangedBlockTracking` is `Enabled`
  - The plugin must also determine whether the VM is running, since offline backup is not supported in the initial release.
    - If QEMU backup is enabled, the next question is whether the user wants to use the kubevirt datamover for this VM's volumes. We will use volume policies for this, although it's a bit more complicated since a VM could have multiple PVCs. If at least one PVC for the VM has the "custom" policy action with "datamover=kubevirt" parameter specified, and no PVCs in this VM have other non-skip policies (i.e. "snapshot", etc.) then we'll use the kubevirt datamover
    - In addition, SnapshotMoveData must be true
    - Iterate over all PVCs for the VM
    - If any PVC has an action other than "custom" or "skip", exit without action
    - If at least one PVC has an action of "custom" with parameter "datamover=kubevirt", then use the kubevirt datamover
  - This plugin will create a DataUpload with `Spec.SnapshotType` set to "kubevirt" and `Spec.DataMover` set to "kubevirt"
    - Note that unlike the built-in datamover, this DataUpload is tied to a VM (which could include multiple PVCs) rather than to a single PVC.
  - An annotation will be added to the DataUpload identifying the VirtualMachine we're backing up.
  - Add `velerov1api.DataUploadNameAnnotation` to VirtualMachine
  - Add `velerov1api.PVCNamespaceNameLabel` annotation to VirtualMachine (doesn't need to be a label, since we're just using it to figure out what label selector to use for the ConfigMap on restore).
  - OperationID will be created and returned similar to what's done with the CSI PVC plugin, and the async operation Progress method will report on progress based on the DU status (similar to CSI PVC plugin)
  - **As implemented** (prio 01): CBT check via `controllercommon.ValidateCBTEnabled`, then `checkVolumePolicies` detects custom-kubevirt vs. conflicting volume policies per-PVC (`hasKubevirtPolicy`/`hasConflictingPolicy`), matching the design above.
- PVC BIA plugin
  - Add `kubevirt-datamover-vm` annotation to PVC with the `VirtualMachine` name to signal to RIA that we need to remove `VolumeName` and set `Selector.MatchLabels` on PVC.
  - **As implemented** (prio 02): the actual annotation is `controllercommon.AnnotationVMName`, stamped on the raw unstructured PVC (not a typed round-trip) to preserve unknown fields.
- VM DeleteItemAction (prio 01, separate action type) — implemented; not originally scoped in this doc.
- VirtualMachine RIA plugin
  - Similar in functionality to csi PVC restore action
  - Create DD based on DU annotation and DU ConfigMap
  - Need to confirm that VM resource has the PVC name annotation added by the BIA plugin
  - VM run-state restore: if the backed-up VM was auto-starting (`spec.runStrategy` or the deprecated `spec.running` bool indicates running), the RIA overrides it to `RunStrategyHalted` on restore and stashes the original run state in an annotation. The VM is not flipped back to its original run state until the Kubevirt Datamover Controller confirms every sibling DataDownload for this VM has completed (see DataDownload reconciler below) — this prevents the VM from booting against partially-restored disks.
  - **As implemented** (prio 05, plugin#44, open/not yet merged): each `Execute()` call unconditionally overwrites the stash annotations (`AnnotationOriginalRunStrategy`/`-Source`) computed fresh from that call's own `input.Item` (the backup data being restored) — it never reads back a previous value, so a stale annotation from an earlier failed restore attempt cannot leak into a new restore's halt decision. **Known gap — terminal-failure handling** (verified by kdm-controller/kdm-plugin, not the original design): if a sibling DataDownload for this VM ends `Failed` or `Canceled` instead of `Completed`, the VM stays `Halted` permanently, with no visible failure signal beyond that DataDownload's own status — `allSiblingDataDownloadsCompleted` does a blanket `!= Completed` check with no special-case for terminal-failed siblings, and since Failed/Canceled DataDownloads are never reconciled again, the wait can never resolve on its own (this is a permanent hang by construction, not a timing race). The stash annotations are only ever cleared by the controller atomically together with a successful flip-back; on failure they're left in place unchanged. A **manual retry** (new DataDownloads created against the same still-halted VM) is **not an officially supported workaround today**, even though it can work mechanically: deleting the old Failed/Canceled DataDownload objects first *does* unblock `allSiblingDataDownloadsCompleted` (confirmed by kdm-controller — the completeness check simply stops finding the stale terminal object), but this is a manual, undocumented operator step with no product-level guardrail, no visible prompt to do it, and no test coverage — not a designed recovery path. A **full second Velero restore** (VM object deleted and recreated) is not subject to this, since the plugin recomputes the stash annotations from that restore's own backup data rather than reading the old ones. **Required fix (not yet implemented — tracked as [kubevirt-datamover-controller#169](https://github.com/migtools/kubevirt-datamover-controller/issues/169), which proposes correlating by a restore-attempt id coordinated with kdm-plugin on what it can stamp at DataDownload-creation time)**: correlate DataDownloads to a specific restore attempt — e.g. stamp the owning Velero `Restore`'s UID/name into the correlation annotations at creation time — and scope `allSiblingDataDownloadsCompleted`'s query to that attempt only, so a retry's new DataDownloads are evaluated independently of any superseded Failed/Canceled ones from a prior attempt, removing the need for operators to manually delete anything. **Until #169 lands, treat manual cleanup-then-retry as an unsupported, undocumented workaround, not a designed recovery path**; `Progress()`'s grace-period-expired error message is the only operator-facing signal that something is stuck. Note on that grace period: it's anchored to when the operation first observed an *empty* DataDownload list, not to the restore's start time (plugin commit 8b05d38) — this is the correct implemented design for this PR, not a pre-existing bug that was found and fixed.
- PVC RIA plugin
  - If PVC has `kubevirt-datamover-vm` annotation, need to do the following:
    - set spec.VolumeName to ""
    - set selector with MatchLabels to match PV that will be created by restore controller
  - **As implemented** (prio 03): `clearPVCBinding` clears `spec.volumeName`, `status`, and the PV-controller bind annotations. **Deviation, RESOLVED (2026-08-06)**: does *not* reset `spec.selector` as specified above — confirmed safe, not just assumed. `spec.selector` is a PVC field used only for the static/pre-provisioned binding pattern (a user manually creates a labeled PV and the PVC selects it) — Kubernetes never auto-populates it, and dynamically-provisioned PVCs (the default for KubeVirt VM disks via DataVolumes/CDI) never set it. `TestClearPVCBinding_LeavesSelectorUntouched` (`pvc/restore_test.go`, commit 65147c4) pins that `clearPVCBinding` leaves `spec.selector` completely untouched whether it's set or absent going in. oadp-e2e's PR #2350 (commit 30a3352f) closes the remaining live-cluster question: the actual source PVC (`cirros-test-disk`) has `spec.selector == nil` before backup, verified on a real cluster — confirming kubevirt-datamover-backed PVCs never carry a selector in practice, not just in theory.
- VirtualMachineBackup/VirtualMachineBackupTracker RIA plugin
  - Simple RIA that discards VMB/VMBT resources on restore
  - We don't want to restore these because they would kick off another VMBackup action.
  - **As implemented** (prio 04): discards both `virtualmachinebackups.backup.kubevirt.io` and `virtualmachinebackuptrackers.backup.kubevirt.io` via `WithoutRestore()`, matching the design above.

### Kubevirt Datamover Controller
- Responsible for reconciling DataUploads/DataDownloads where `Spec.DataMover` is "kubevirt"
- Configurable concurrency limits: concurrent-vm-backups and concurrent-vm-datauploads. **As implemented**: `MaxConcurrentReconciles` per controller (default 3 if unset); DataUpload additionally serializes per-VM (`hasOlderActiveDUForVM` requeues a new DU if an older active one targets the same VM) so incremental checkpoint chains stay ordered even under concurrency.
- We need the `qemu-img` binary built into the controller image.
- Both reconcilers implement the same phase state machine: `New -> Accepted -> Prepared -> InProgress -> Completed/Failed/Canceling`, with `Spec.Cancel` handled at any non-terminal phase.
- DataUpload reconciler (backup):
  - create the (temporary) PVC.
  - identify the VirtualMachine from the PVC metadata.
  - From the BSL, identify the latest checkpoint for this VM and confirm that all of the related qcow2 files are available.
  - create/update the VirtualMachineBackupTracker CR with `source` set to the VirtualMachine, and `status.latestCheckpoint.name` set to match the latest checkpoint we have for this vm. Note that it might be better if we could avoid hacking the VMBT status and instead have a field on the VM -- something like `spec.overrideCheckpoint` to use a specific checkpoint if the one in the VMBT doesn't match the latest we have in the BSL.
  - create the VirtualMachineBackup CR with `source` set to the VirtualMachineBackupTracker, `pvcName` set to the temporary PVC, and (optionally)`forceFullBackup`, set to `true` to force a full backup
  - Wait for VMBackup to complete (monitoring status)
  - Launch kubevirt datamover pod mounting the temporary PVC with the qcow2 file(s) from the backup.
    - This pod needs to be running a command that will do the datamover operation from pvc to object storage
    - The datamover pod functionality should be built into the same image as the kubevirt-datamover-controller pod image.
  - Copy the new file to object storage (see [Where to store qcow2 files](#wherehow-to-store-qcow2-files-and-metadata) below)
  - Save any required metadata to identify the stored data (collection of qcow2 pathnames/checkpoints, etc.), along with identifying the backup and VirtualMachine they're associated with. Save this metadata file as well (see [Where to store qcow2 files](#wherehow-to-store-qcow2-files-and-metadata) below)
    - We need to properly handle cases where we attempt an incremental backup but a full backup is taken instead (checkpoint lost, CSI snapshot restore since last checkpoint, VM restart, etc.)
    - Aborted backups also need to be handled (resulting in a failed PVC backup on the Velero side)
  - **VMB/VMBT lifecycle, as implemented**: the uploader pod deletes the VMB itself on success (after the S3 upload completes); the controller (`cleanupVMBackupResources`) deletes the VMB on cancel. VMBT is *never* deleted by either path — intentionally kept so KubeVirt can reuse it across VM restarts/migrations to redefine libvirt checkpoints (issue #32). **Gap**: on a genuine `Failed` (not `Canceled`) DataUpload, nothing deletes the VMB — `cleanupVMBackupResources` is only called from `handleCanceling`, and none of the ~20 `DataUploadPhaseFailed` transition sites in `kubevirt_dataupload_controller.go` delete it, so it is left orphaned unconditionally. Issue #12 ("Phase 5: Complete cleanup handling and VMB/VMBT S3 archival") is closed as completed and covers the success-path half (S3 archival, pod self-deletes VMB, controller reads archived `vmbt.json`), but the failure-path VMB deletion it also proposed was never implemented — #12 should be treated as partially delivered, not as having resolved this. **Required fix (not yet implemented — tracked as [kubevirt-datamover-controller#168](https://github.com/migtools/kubevirt-datamover-controller/issues/168), which explicitly notes #12 as partially-delivered rather than resolving)**: every `DataUploadPhaseFailed` transition site must also invoke `cleanupVMBackupResources` (or an equivalent idempotent VMB deletion), exactly as `handleCanceling` already does, so a Failed DataUpload no longer orphans its VMB. VMBT retention (never deleted) is intentional and must not change.
- DataDownload reconciler (restore)
  - Identify the VM from the DD.
  - Pull BSL metadata for the VM and backup
  - Once this DD reaches Completed, check whether every other DataDownload matching this VM's correlation annotations has also completed; if so, restore the VM's original run state (stashed by the VM RIA — see above). **Current scope boundary**: this check only considers DataDownloads it currently knows about, not an independently-verified expected-volume-count for the VM — race-free for single-disk VMs (the only case validated so far), but not yet safe for multi-disk VMs if their DataDownloads could be created in a staggered fashion. **Design requirement for multi-disk support** ([kubevirt-datamover-controller#73](https://github.com/migtools/kubevirt-datamover-controller/issues/73) phase 4): before multi-disk restore ships, the controller must gate on an explicit expected-volume-count signal (from the VM spec or the plugin) and *reject or hold* automatic run-state restoration until that count is satisfied — it must not resume opportunistically just because every *currently discovered* DataDownload is Completed. Single-disk VMs are unaffected by this requirement (expected=discovered=1 trivially) and keep today's completion-gated behavior.
  - Create the temporary PVC to download the qcow2 files onto.
    - PV here is also temporary
    - PVC size based on the size of the qcow2 files in BSL needed for restore as well as the PVC sizes
    - For each PVC, calculate the sum of all qcow2 files added to the PVC size, and then add 10% as a buffer. If there are multiple PVCs, take the max value, as we can process one PVC at a time, so we don't need to hold files for all PVCs on the temp disk at the same time. **In-flight, not yet on oadp-dev HEAD** (part of PR #124, unmerged): scratch/work/output PVC sizes will derive from the backup index's recorded *bound-PV actual capacity*, not requested size, to avoid undersizing from storage-backend rounding (e.g. AWS EBS 1GiB minimum). **Currently shipping behavior** (the DataUpload/backup-only precursor is already merged into `oadp-dev`): every backup manifest produced today records *requested* size, not bound-PV capacity. **Compat gap**: the restore-side floor (`maxDiskSizeFromIndex`) floors the manifest's recorded size against the restore target's own requested size — the same value — so it does not protect against the exact backend-bump-above-request scenario the fix was designed for. Any backup taken before #124 merges could produce an undersized scratch PVC on restore, and #124 does not retroactively correct already-stored manifests; a migration/compat note (and likely a fallback for pre-fix manifests) is needed before this ships.
  - Create temporary PVCs for each PVC in the VM (identified from BSL metadata).
    - These need to be mounted as block mode volumes.
    - PV will be bound to workload PVCs after restore, similar to velero datamover.
    - To facilitate PV reattachment, we need a similar approach to the upstream velero exposer logic:
      - The `DynamicPVRestoreLabel` needs to be set on the restore PV
      - Generic restore exposer reads the selector back from the target PVC: In <https://github.com/vmware-tanzu/velero/blob/main/pkg/exposer/generic_restore.go#L443-L449>, `RebindVolume()` extracts `targetPVC.Spec.Selector.MatchLabels` and passes it to `ResetPVBinding()`.
      - `ResetPVBinding()` copies the labels onto the PV: In <https://github.com/vmware-tanzu/velero/blob/main/pkg/util/kube/pvc_pv.go#L226-L270>, the labels (including `DynamicPVRestoreLabel`) are copied from the PVC selector to the PV's labels, and ClaimRef is reset so Kubernetes can bind them.
    - Size based on the `pvcSizes` metadata in the BSL.
  - We'll need to create another datamover pod here which will do the following:
    - The pod permissions will need to be the same as we have for velero datamover (run as root, selinux config etc.)
    - The pod will have temp PVC mounted, as well as PVCs mounted for each vm disk we're creating.
    - The pod running command/image will first get the list of qcow2 files to pull from object storage
    - Process one PVC at a time:
      - Download all required qcow2 files for this PVC from object storage.
      - Validate the checkpoint chain from the per-VM manifest (`checkpointChain`)
        before rebasing: verify every intermediate file is present on disk and each
        is a readable qcow2 image (`qemu-img info` succeeds). The `-u` (unsafe)
        rebase flag skips all validation, so missing or corrupt intermediates cause
        silent data loss.
      - Build the backing chain by rebasing each incremental onto its parent (not
        all onto the full backup — that loses intermediate changes):
        - `qemu-img rebase -b full.qcow2 -F qcow2 -f qcow2 -u inc1.qcow2`
        - `qemu-img rebase -b inc1.qcow2 -F qcow2 -f qcow2 -u inc2.qcow2`
        - (continue for each incremental in chain order)
      - Convert the top-of-chain directly to the target block device (no
        intermediate raw file or `dd` needed):
        - `qemu-img convert -f qcow2 -O raw incN.qcow2 /dev/target_pvc_block_device`, passing `-S 0` for block-mode targets (sparse-write skip is unsafe on a reused block device).
      - Delete all qcow2 files from scratch space.
      - Chain resolution (`pkg/uploader` index → `resolveTargetDiskName`) prefers the newest checkpoint's disk-name mapping, falling back through older entries if the newest is malformed.
      - References:
        - Chained rebase approach: [KubeVirt VEP — Restore from Backup](https://github.com/kubevirt/enhancements/blob/main/veps/sig-storage/incremental-backup.md?plain=1#L443-L466)
        - `-F` backing format flag required since [QEMU 6.1](https://wiki.qemu.org/ChangeLog/6.1#Block_layer); see [qemu-img rebase docs](https://www.qemu.org/docs/master/tools/qemu-img.html#cmdoption-qemu-img-commands-arg-F)
        - Workflow validated with synthetic qcow2 files: [validation gist](https://gist.github.com/kaovilai/5ac5d2563d4d3c60be090475f2ac2c06)
    - Note that the various `qemu-img` actions might eventually be combined into a single kubevirt API call, but for the moment this would need to be done manually.
  - Once datamover pod has restored the VM disks, it will exit, the temporary PVCs will be deleted, leaving the restored PVs but deleting the qcow2 staging PV.

### Kubevirt datamover backup data/metadata

The kubevirt datamover data will be stored in the BSL using a prefix derived from `BSLPrefix+"-kubevirt-datamover"`.

The directory structure will be as follows:
```
  <bsl-prefix>-kubevirt-datamover/
  ├── manifests/
  │   └── <velero-backup-name>/
  │       ├── <vm-name>.json                       # Per-backup-per-vm manifest
  │       └── index.json                           # Per-backup manifest
  └── checkpoints/
      └── <namespace>/
          └── <vm-name>/
              ├── <checkpoint-id>/                 # checkpoint dir
              │   └── <vmb-name>-<disk-name>.qcow2 # Actual qcow2 files
              └── index.json                       # Per-VM index file
```
Example of a Per-VM Index file:

`pvcSizes` semantics have changed across implementations and are **not yet consistent on `oadp-dev`**: the currently-shipping (merged) uploader records each PVC's *requested* size here. An in-flight fix (PR #124, unmerged) changes this to record the *bound-PV actual capacity* instead, to avoid undersizing restores when the storage backend rounds up (e.g. AWS EBS 1GiB minimum) — see DataDownload reconciler above for the full compat gap this creates for manifests written before #124 merges. Readers of this file (and any migration tooling) must not assume a fixed meaning for `pvcSizes` without checking which uploader version wrote it.
```
Per-VM Index (checkpoints/<ns>/<vm-name>/index.json):

{
  "vmName": "my-vm",
  "namespace": "default",
  "checkpoints": [
    {
      "id": "cp-001",
      "type": "full",
      "timestamp": "2025-01-10T10:00:00Z",
      "vmBackup": "vmb-001",
      "files": ["vmb-001-disk0.qcow2", "vmb-001-disk1.qcow2"],
      "pvcs": ["my-vm-pvc-0", "my-vm-pvc-1"],
      "pvcSizes": ["10Gi", "20Gi"],
      "referencedBy": ["backup-2025-01-10", "backup-2025-01-11", "backup-2025-01-12"]
    },
    {
      "id": "cp-002",
      "type": "incremental",
      "parent": "cp-001",
      "timestamp": "2025-01-11T10:00:00Z",
      "vmBackup": "vmb-002",
      "files": ["vmb-002-disk0.qcow2", "vmb-002-disk1.qcow2"],
      "pvcs": ["my-vm-pvc-0", "my-vm-pvc-1"],
      "pvcSizes": ["10Gi", "20Gi"],
      "referencedBy": ["backup-2025-01-11", "backup-2025-01-12"]
    },
    {
      "id": "cp-003",
      "type": "incremental",
      "parent": "cp-002",
      "timestamp": "2025-01-12T10:00:00Z",
      "vmBackup": "vmb-003",
      "files": ["vmb-003-disk0.qcow2", "vmb-003-disk1.qcow2"],
      "pvcs": ["my-vm-pvc-0", "my-vm-pvc-1"],
      "pvcSizes": ["10Gi", "20Gi"],
      "referencedBy": ["backup-2025-01-12"]
    }
  ]
}
```
Example of a per-backup manifest:
```
Per-Backup Manifest (manifests/<backup-name>/index.json):

{
  "backupName": "backup-2025-01-12",
  "timestamp": "2025-01-12T10:00:00Z",
}
```
Example of a per-backup manifest:
```
Per-Backup-oer-vm Manifest (manifests/<backup-name>/<vm-name>.json):

{
  "namespace": "default",
  "name": "my-vm",
  "checkpointChain": ["cp-001", "cp-002", "cp-003"]
}
```


### Where/how to store qcow2 files and metadata
- Current approach:
  - Use the Velero object store plugin API but not the velero-specific logic in `persistence/object-store.go`
  - Create a top-level dir in the BSL (under the BSL prefix, parallel to backups/restores/kopia) for kubevirt datamover.
    - Actually, this may have to be outside the prefix (i.e. if prefix is "data" then we may need to create a parallel dir "data-kubevirt" or something similar, since I think Velero allows only its own files under the prefix)
  - Copy individual qcow2 files and metadata files identifying the required qcow2 checkpoints. We may want a subdir per VirtualMachine for qcow2 files. For metadata files, these should probably be organized by velero backup.
  - We need to manage storage usage on backup deletion -- when deleting a backup, we should delete any qcow2 files no longer referenced by any still-existing backups.
- Other approaches:
  - On volume in cluster
    - Likely the simplest approach
    - Volume must be mounted by the controller pod
    - Will require its own periodic velero backups (less frequently than the VM incremental backups) for disaster recovery purposes
  - Using kopia
    - We could use kopia on top of the object storage API, but it is not clear that this will provide any real benefits, since we're already working with files that represent just the data diff we need. We can just manage them as individual objects.
    - This will also require additional overhead around kopia maintenance, and we still may need to manage qcow2 file deletion manually.

### E2E coverage (as of 2026-08-06)

`tests/e2e/virt_backup_restore_suite_test.go`, verified on AWS + community HCO/KubeVirt.
- PASS: multi-PVC VM backup/restore via generic CSI-datamover (Velero built-in, not the kubevirt-datamover-specific path).
- PASS: full → incremental → VM-restart-preserves-checkpoint-chain → the per-VM `kubevirt-datamover.io/max-incremental-backups` limit forces the *next* backup to fall back to full (validated prior session, referenced as green in the current PR description; not independently rerun this session). **This is the automatic threshold-triggered path only** — it is a distinct mechanism from the manual `kubevirt-datamover.io/force-full-backup` DataUpload annotation (see Open questions below); verified by grepping the whole e2e suite, there is zero test coverage for the manual annotation today.
- PASS: restore from a full kubevirt-datamover CBT backup (verified twice this session, including after the RBAC fix).
- Known gaps (scaffolded `ginkgo.PIt`, blocked upstream — not flakes): multi-PVC restore from a CBT backup, and restore from an incremental CBT backup (both blocked on [kubevirt-datamover-controller#73](https://github.com/migtools/kubevirt-datamover-controller/issues/73) phases 4/5); the `maxIncrementalBackups=0` checkpoint-delete sub-case is blocked on CNV-85377 (virt-controller never falls back to full, VMB hangs `Initializing`).
- No flakes observed across 4 runs this session (small sample — not a long-term flake-free claim).

### Open questions
- How to determine PVC size?
  - user-configurable? configmap or annotation?
  - From the kubevirt enhancement: "Before the process begins, an estimation of the required backup size will be performed. If the provided PVC size is insufficient, an error will be returned"
  - If the PVC is too small, we need a clear error on the backup indicating that it failed due to insufficient PVC space.
  - Since controller is responsible for PVC creation rather than plugin, the controller may be able to respond to PVC too small errors by retrying with a larger PVC.
  - [alitke] The safest approach is to create a PVC that is 5% larger than the combined size of all disks to be backed up.
  - **IN-FLIGHT, NOT YET ON oadp-dev (2026-08-06)**: PR #124 (unmerged) derives PVC sizing from recorded bound-PV actual capacity instead of requested size — see DataDownload reconciler above. Currently-shipping backups record requested size, and the fix doesn't retroactively correct their manifests, so a compat/migration note is needed before merge.
- The kubevirt datamover controller will be responsible for deleting the `VirtualMachineBackup` resource once it's no longer needed. When should this happen? Upon velero backup deletion? This would enable debugging in the case of failed operations. If we delete it immediately, that will make troubleshooting more difficult. If on backup deletion, we'll need to write a `DeleteItemAction` plugin.  [alitke] The VirtualMachineBackup resource should be deleted after the data mover has completed.  It no longer has any use and accumulating these on-cluster will harm usability.  Perhaps completed ones could be garbage collected by the KubeVirt DataMover Controller.
  - **PARTIALLY RESOLVED (2026-08-06)**: uploader pod deletes VMB on success, controller deletes it on cancel; VMBT is never deleted (kept for KubeVirt to reuse across VM lifecycle events). **Still open**: on genuine `Failed` (not canceled), the VMB is orphaned — no code path deletes it. See DataUpload reconciler above.
- Do we need an option to force full backups? If we're always doing incremental, eventually the incremental backup list becomes really long, requiring applying possibly hundreds of incremental files for a single restore.
  - For initial release, we can add a force-full-virt-backup annotation on the velero backup. Longer-term, we can push for a general datamover feature in velero which could force full backups for both fs-backup and velero datamover if backup.Spec.ForceFullVolumeBackup is true, and once implemented, the qcow2 datamover can use this as well.
  - **SUPERSEDED**: the annotation-on-the-Velero-Backup proposal above was not what shipped. What's actually implemented is a `kubevirt-datamover.io/force-full-backup` annotation on the **DataUpload** (a different object), honored as `VMB.Spec.ForceFullBackup`. There is no Velero-Backup-level annotation — operators must annotate the DataUpload (or whatever object the BIA plugin exposes this through), not the Backup, to force a full backup. **Provenance**: introduced in [kubevirt-datamover-controller#13](https://github.com/migtools/kubevirt-datamover-controller/pull/13) (mpryc) as one of several Phase 4 features in a squashed commit — no PR discussion, commit message, or linked issue documents *why* DataUpload-level was chosen over Backup-level; this deviation's rationale is not recoverable from git/GitHub history and should be attributed as undocumented rather than inferred.
  - **PARTIALLY RESOLVED (2026-08-06)**: implemented via the `kubevirt-datamover.io/force-full-backup` DataUpload annotation honored as `VMB.Spec.ForceFullBackup` (per kdm-controller `pkg/common/constants.go`). **Gap**: zero e2e coverage for this specific manual annotation — the only e2e-tested force-full path is the *automatic* `max-incremental-backups` threshold trigger (a different, unrelated annotation), see E2E coverage above.
- **NEW (2026-08-06): how should `pvcSizes` manifest semantics be versioned across the requested-size (currently shipping) and bound-PV-capacity (PR #124, unmerged) writers?** No plan exists yet — kdm-controller offered this as an unreviewed sketch, explicitly not a commitment: add a `schemaVersion` (or a narrower `sizeSemantics: "requested"|"boundPV"`) field to the per-VM backup index; on restore, `maxDiskSizeFromIndex` treats its *absence* as "legacy, requested-size" and does not trust the recorded number as a bound-PV-capacity floor for the backend-bump scenario — i.e. fail open to the more conservative interpretation rather than silently trusting an old number as if it were the larger bound-PV value. This needs real design review, not a unilateral decision — see the manifest schema note above.

### General notes
- SnapshotMoveData must be true on the backup or DU/DD processing won't work properly
- Longer-term, we can probably eliminate some of the custom code in the new controller by refactoring the velero datamover pluggability features, allowing the node agent to orchestrate this (with a custom image and configuration for datamover pods, etc.)
- The kubevirt enhancement references both "push mode" and "pull mode" -- initial implementation on the kubevirt side will be push mode only. This OADP proposal is also push mode only for the initial implementation

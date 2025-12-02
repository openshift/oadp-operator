# Kubevirt incremental datamover

In a kubevirt environment, one possibility for improving volume backup performance to allow more frequent backups would be to take incremental qcow2 backups using libvirt tools. Integrating this into OADP/Velero backups will require some new components to OADP (plugins, controller), as well as some modifications to the velero codebase.

## Background

Taking a VolumeSnapshot and then using kopia to process the entire volume and copy the required incremental changes into the Backup Storage Location (BSL) is a heavyweight process. Creating an incremental qcow2 backup for the same volume is generally a much more lightweight action. We want to make use of the existing Velero backup/restore process, with actual kubevirt incremental backup/restore happening via a new controller. For the moment, this will be referred to as the Kubevirt Datamover Controller. This action will be coordinated with Velero via existing infrastructure -- BackupItemActions (BIAs), RestoreItemActions (RIAs) and the DataUpload/DataDownload CRs. Initial implementation should require minimal changes to Velero, since Velero currently ignores DataUploads with `Spec.DataMover` set to something other than `velero`.

## Goals

- Back up and restore VM volumes using kubevirt incremental backup instead of Velero's built-in CSI datamover
- Use existing Velero infrastructure to integrate this feature into regular velero backup and restore
- Implementation based on kubevirt enhancement defined at <https://github.com/kubevirt/enhancements/blob/main/veps/sig-storage/incremental-backup.md>
  - There is a follow-on design PR at <https://github.com/kubevirt/enhancements/pull/126/changes> although this mainly discusses push-mode, which is out of scope for the current design.

## Non goals
- Deep integration with velero data mover pluggability (this could be considered in the long-term though, which would minimize duplication of effort and enhance maintainability)
- Using push mode with kubevirt.

## Use cases
- As a user I want to use OADP to trigger backups that will back up volume data using kubevirt tooling rather than CSI snapshots
  - Volume backups will be incremental when possible (first backup for a given volume will be full, subsequent backups for that same volume will be incremental)
  - Assuming that kubevirt incremental volume backups should be much faster than CSI snapshots followed by incremental kopia snapshot copy, the expectation is that users might run kubevirt datamover backups more frequently than they would for CSI-based backups.
  - If users are backing up more frequently with this method, they should ensure that they are not using CSI snapshots or fs-backup via the resource/volume policy configuration in the backup.

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
- Update velero volume policy model to allow unrecognized volume policy actions. Velero would treat unrecognized actions as "skip" (i.e. no snapshot or fs-backup), but the kubevirt datamover could only act if the policy action is "kubevirt".
- In `pkg/restore/actions/dataupload_retrieve_action.go` and in `DataDownload` we need to add SnapshotType.

### BackupItemAction/RestoreItemAction plugins
- VirtualMachine BIA plugin
  - The plugin will check whether the VirtualMachine's `status.ChangedBlockTracking` is `Enabled`
  - The plugin must also determine whether the VM is running, since offline backup is not supported in the initial release.
    - If QEMU backup is enabled, the next question is whether the user wants to use the kubevirt datamover for this VM's volumes. We will use volume policies for this, although it's a bit more complicated since a VM could have multiple PVCs. If at least one PVC for the VM has the "kubevirt" policy action specified, and no PVCs in this VM have other non-skip policies (i.e. "snapshot", etc.) then we'll use the kubevirt datamover
	- In addition, SnapshotMoveData must be true
    - Iterate over all PVCs for the VM
    - If any PVC has an action other than "kubevirt" or "skip", exit without action
    - If at least one PVC has an action of "kubevirt", then use the kubevirt datamover
  - This plugin will create a DataUpload with `Spec.SnapshotType` set to "kubevirt" and `Spec.DataMover` set to "kubevirt"
    - Note that unlike the built-in datamover, this DataUpload is tied to a VM (which could include multiple PVCs) rather than to a single PVC.
  - An annotation will be added to the DataUpload identifying the VirtualMachine we're backing up.
  - Add `velerov1api.DataUploadNameAnnotation` to VirtualMachine
  - OperationID will be created and returned similar to what's done with the CSI PVC plugin, and the async operation Progress method will report on progress based on the DU status (similar to CSI PVC plugin)
- VirtualMachine RIA plugin
  - (further investigation still needed on exactly what's needed here)
  - Similar in functionality to csi PVC restore action
  - Create temporary PVC (needed, or is this handled by controller?)
  - Create DD based on DU annotation and DU ConfigMap (this may be handled already by existing DU RIA)
- VirtualMachineBackup/VirtualMachineBackupTracker RIA plugin
  - Simple RIA that discards VMB/VMBT resources on restore
  - We don't want to restore these because they would kick off another VMBackup action.

### Kubevirt Datamover Controller
- Responsible for reconciling DataUploads/DataDownloads where `Spec.DataMover` is "kubevirt"
- Configurable concurrency limits: concurrent-vm-backups and concurrent-vm-datauploads
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
- DataDownload reconciler (restore)
  - (this area is less well defined so far, since the kubevirt enhancement doesn't go into as much detail on restore)
  - We will need a temporary PVC for pulling qcow2 images from object store (if we're restoring the empty temp PVC from backup, that might work here)
  - We also need PVCs created for each VM disk we're restoring from qcow2 images.
  - We'll need to create another datamover pod here which will do the following:
    - pod will have temp PVC mounted, as well as PVCs mounted for each vm disk we're creating.
    - pod running command/image will first get the list of qcow2 files to pull from object storage
    - once we have the qcow2 full backup and incremental files from object store, repeatedly call `qemu-img rebase -b fullbackup.qcow2 -f qcow2 -u incrementalN.qcow2` for each incremental backup, in order
    - Then, convert qcow2 image to raw disk image: `qemu-img convert -f qcow2 -O raw fullbackup.qcow2 restored-raw.img`
    - Finally, write this image to the PVC which contains the VM disk
    - (repeat process for each disk if VM has multiple disks to restore)
    - Note that the various `qemu-img` actions might eventually be combined into a single kubevirt API call, but for the moment this would need to be done manually.
  - Once datamover pod has restored the VM disks, it will exit and the VirtualMachine can launch with these disks (following the velero datamover model where the temporary PVC is deleted, retaining the PV, which then binds to the VM's PVCs may work here). The temporary PVC (containing the qcow2 images, not the restored VM disk image) should be completely removed at this point, including PV content.

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

### Open questions
- How to determine PVC size?
  - user-configurable? configmap or annotation?
  - From the kubevirt enhancement: "Before the process begins, an estimation of the required backup size will be performed. If the provided PVC size is insufficient, an error will be returned"
  - If the PVC is too small, we need a clear error on the backup indicating that it failed due to insufficient PVC space.
  - Since controller is responsible for PVC creation rather than plugin, the controller may be able to respond to PVC too small errors by retrying with a larger PVC.
- The kubevirt datamover controller will be responsible for deleting the `VirtualMachineBackup` resource once it's no longer needed. When should this happen? Upon velero backup deletion? This would enable debugging in the case of failed operations. If we delete it immediately, that will make troubleshooting more difficult. If on backup deletion, we'll need to write a `DeleteItemAction` plugin.
- Do we need an option to force full backups? If we're always doing incremental, eventually the incremental backup list becomes really long, requiring applying possibly hundreds of incremental files for a single restore.
  - For initial release, we can add a force-full-virt-backup annotation on the velero backup. Longer-term, we can push for a general datamover feature in velero which could force full backups for both fs-backup and velero datamover if backup.Spec.ForceFullVolumeBackup is true, and once implemented, the qcow2 datamover can use this as well.

### General notes
- SnapshotMoveData must be true on the backup or DU/DD processing won't work properly
- Longer-term, we can probably eliminate some of the custom code in the new controller by refactoring the velero datamover pluggability features, allowing the node agent to orchestrate this (with a custom image and configuration for datamover pods, etc.)
- The kubevirt enhancement references both "push mode" and "pull mode" -- initial implementation on the kubevirt side will be push mode only. This OADP proposal is also push mode only for the initial implementation

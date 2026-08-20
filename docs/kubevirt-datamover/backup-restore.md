# Backing Up and Restoring VirtualMachines with KubeVirt DataMover

This guide walks through a complete backup and restore cycle for a KubeVirt VirtualMachine using the KubeVirt DataMover feature. It assumes you have already enabled and configured KubeVirt DataMover as described in [configuration.md](./configuration.md).

## Overview

KubeVirt DataMover backs up VM disks by taking QEMU-level snapshots and tracking changed blocks between backups, instead of relying on CSI volume snapshots. From your point of view as a Velero user, the workflow looks the same as any other Velero backup and restore: you create a `Backup` object, Velero backs up the namespace, and later you create a `Restore` object to bring it back. The difference happens behind the scenes, where the kubevirt-datamover-plugin and kubevirt-datamover-controller take over the disk data movement for you.

## Step 1: Label the VM for Changed Block Tracking

CBT has to be turned on per VM, in addition to being enabled at the HCO level. Add the `changedBlockTracking: "true"` label to the VirtualMachine:

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: my-vm
  namespace: my-vm-namespace
  labels:
    changedBlockTracking: "true"
spec:
  dataVolumeTemplates:
  - metadata:
      name: my-vm-disk
    spec:
      pvc:
        accessModes:
        - ReadWriteOnce
        resources:
          requests:
            storage: 10Gi
        volumeMode: Block
      source:
        registry:
          pullMethod: node
          url: docker://your-image
  running: true
  template:
    spec:
      domain:
        devices:
          disks:
          - disk:
              bus: virtio
            name: rootdisk
      volumes:
      - dataVolume:
          name: my-vm-disk
        name: rootdisk
```

Two things matter here for KubeVirt DataMover to work correctly:

- The disk's `volumeMode` must be `Block`. CBT tracking relies on being able to read raw changed blocks off the underlying volume, which is not available in filesystem-mode PVCs.
- The label goes on the `VirtualMachine`, not on the `DataVolume` or `PersistentVolumeClaim`.

If you apply the label to an existing VM that is already running, you may need to restart the VM (stop and start it again) for CBT to actually start tracking, depending on your KubeVirt version. Check that CBT is active by describing the VM's VirtualMachineInstance and looking for CBT status in KubeVirt's own status fields, or simply proceed to the backup step below and confirm the first backup succeeds as a full backup.

## Step 2: Create a volume policy that routes VM disks through KubeVirt DataMover

Create a ConfigMap containing the volume policy, if you have not already done so as part of your DPA configuration:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubevirt-volume-policy
  namespace: openshift-adp
data:
  volume-policy.yaml: |
    version: v1
    volumePolicies:
      - conditions:
          csi:
            driver: "*"
        action:
          type: custom
          parameters:
            datamover: kubevirt
```

## Step 3: Run a backup

Create a Velero `Backup` that references the namespace containing your VM and the volume policy ConfigMap:

```yaml
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: my-vm-backup
  namespace: openshift-adp
spec:
  includedNamespaces:
  - my-vm-namespace
  defaultVolumesToFsBackup: false
  snapshotMoveData: true
  resourcePolicy:
    kind: ConfigMap
    name: kubevirt-volume-policy
```

`snapshotMoveData: true` is required. KubeVirt DataMover always moves the backed up data to your object storage location, it does not leave data sitting in an in-cluster snapshot the way a CSI-only backup might.

Watch the backup progress:

```bash
oc get backup my-vm-backup -n openshift-adp -w
```

Behind the scenes, when Velero gets to the VM's disks, the kubevirt-datamover-plugin creates a `DataUpload` custom resource with `spec.datamover: kubevirt`. The kubevirt-datamover-controller picks that up and works through a series of phases: `New`, `Accepted`, `Prepared`, `InProgress`, and finally `Completed` (or `Failed` if something goes wrong). You can watch this directly if you want more granular visibility than the Backup object gives you:

```bash
oc get datauploads.velero.io -n openshift-adp -w
```

Along the way, the controller creates a KubeVirt `VirtualMachineBackup` for your VM to trigger the actual CBT snapshot, and a `VirtualMachineBackupTracker` to record the checkpoint chain for that VM. These objects are temporary. Once a backup finishes, the controller archives their state into your object storage bucket and removes the CR from the cluster, so do not be surprised if you cannot find them after the backup completes.

The first backup you take for a given VM is always a full backup, since there is no previous checkpoint to diff against. Subsequent backups are incremental by default: only the blocks that changed since the last checkpoint get uploaded, which makes them much faster and cheaper on storage than a full backup.

### Forcing a full backup

If you need to force a fresh full backup instead of an incremental one, for example after a maintenance operation that you suspect broke the checkpoint chain, add this annotation to the VirtualMachine before running the backup:

```bash
oc annotate vm my-vm -n my-vm-namespace kubevirt-datamover.io/force-full-backup=true
```

## Step 4: Confirm the backup completed successfully

```bash
oc get backup my-vm-backup -n openshift-adp -o jsonpath='{.status.phase}'
```

You should see `Completed`. Check the `DataUpload` object's phase too, since Velero can sometimes report a backup as complete while individual DataUploads are still finishing up in edge cases:

```bash
oc get datauploads.velero.io -n openshift-adp -l velero.io/backup-name=my-vm-backup
```

## Step 5: Restore the VM

Delete or otherwise lose your VM (or restore into a different namespace/cluster), then create a Velero `Restore`:

```yaml
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: my-vm-restore
  namespace: openshift-adp
spec:
  backupName: my-vm-backup
```

```bash
oc apply -f restore.yaml
oc get restore my-vm-restore -n openshift-adp -w
```

On the restore path, the kubevirt-datamover-plugin's Restore Item Action creates a `DataDownload` resource, which the controller processes through its own phase sequence. The controller downloads the checkpoint chain from object storage and reconstructs the disk image using `qemu-img rebase`, chaining each incremental checkpoint onto its parent rather than flattening everything onto the full backup in one step. This preserves the same layered structure the backup had, and lets the controller avoid downloading the full backup data again for every incremental restore.

Once the `DataDownload` reaches `Completed`, the restored PVC is bound and rebound to the new VM created by Velero's standard VM restore path (handled by kubevirt-velero-plugin), and the VM should come up with its data intact.

Check that the VM started correctly:

```bash
oc get vm my-vm -n my-vm-namespace
oc get vmi my-vm -n my-vm-namespace
```

## Verifying data integrity

For a meaningful test, write something identifiable to the VM's disk before backing it up (a file, a database record, whatever suits your workload), take the backup, delete the VM, restore it, and confirm the same data is present. This is exactly the pattern OADP's own end-to-end tests use, and it is the best way to build confidence in your specific storage backend and VM configuration before relying on this for production backups.

## Incremental backup chains and full backups over time

Left running long enough, a VM will accumulate a chain of incremental backups, each depending on the one before it. There are two mechanisms that eventually force a new full backup, so the chain does not grow forever:

- **maxIncrementalBackups**: configured DPA-wide or per VM (see [configuration.md](./configuration.md)), this caps how many incrementals can chain together before the controller starts a new full backup automatically.
- **Broken chain detection**: if the controller cannot validate the existing checkpoint chain against what is in the BackupStorageLocation (for example, if an earlier backup or checkpoint was deleted out from under it), it falls back to a full backup rather than failing outright.

You do not need to manage this yourself under normal operation. It is worth knowing about if you notice a backup taking noticeably longer than expected, since that is usually a sign a full backup happened instead of an incremental one.

For troubleshooting failed or stuck backups and restores, see [troubleshooting.md](./troubleshooting.md).

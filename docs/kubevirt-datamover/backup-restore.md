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

If you apply the label to an existing VM that is already running, you may need to restart the VM (stop and start it again) for CBT to actually start tracking, depending on your KubeVirt version. Check that CBT is active on the VirtualMachine itself:

```bash
oc get vm my-vm -n my-vm-namespace -o jsonpath='{.status.changedBlockTracking.state}'
```

You should see `Enabled`. If it isn't, try restarting the VM (`virtctl stop` then `virtctl start`), or simply proceed to the backup step below and confirm the first backup succeeds as a full backup.

## Step 2: Create a volume policy that routes VM disks through KubeVirt DataMover

Create a ConfigMap containing the volume policy, if you have not already done so as part of your DPA configuration. The ConfigMap must have exactly one entry under `data`, but the key name does not matter, Velero reads whatever single value is there. `policy.yaml` is just the conventional name used in most examples:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubevirt-volume-policy
  namespace: openshift-adp
data:
  policy.yaml: |
    version: v1
    volumePolicies:
      - conditions: {}
        action:
          type: custom
          parameters:
            datamover: kubevirt
```

If you would rather keep your policy in a separate file and create the ConfigMap from it, that works the same way:

```bash
oc create cm kubevirt-volume-policy -n openshift-adp --from-file policy.yaml
```

See [configuration.md](./configuration.md#volume-policy-configuration) for more on how volume policy matching works and what to watch out for with catch-all entries.

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

Or create the same backup with the `oc oadp` CLI plugin instead of writing the YAML by hand:

```bash
oc oadp backup create my-vm-backup --include-namespaces my-vm-namespace --resource-policies-configmap kubevirt-volume-policy --snapshot-move-data
```

`snapshotMoveData: true` is required. KubeVirt DataMover always moves the backed up data to your object storage location, it does not leave data sitting in an in-cluster snapshot the way a CSI-only backup might.

Watch the backup progress:

```bash
oc get backups.velero.io my-vm-backup -n openshift-adp -w
```

Behind the scenes, when Velero gets to the VM's disks, the kubevirt-datamover-plugin creates a `DataUpload` custom resource with `spec.datamover: kubevirt`. The kubevirt-datamover-controller picks that up and works through a series of phases: `New`, `Accepted`, `Prepared`, `InProgress`, and finally `Completed` (or `Failed` if something goes wrong). You can watch this directly if you want more granular visibility than the Backup object gives you:

```bash
oc get datauploads.velero.io -n openshift-adp -w
```

Along the way, the controller creates a KubeVirt `VirtualMachineBackup` for your VM to trigger the actual CBT snapshot, and a `VirtualMachineBackupTracker` to record the checkpoint chain for that VM. The `VirtualMachineBackup` is temporary: once a backup finishes, the controller archives its state into your object storage bucket and removes it from the cluster, so do not be surprised if you cannot find it afterward. The `VirtualMachineBackupTracker` behaves differently and is left on the cluster between backups on purpose, so KubeVirt can use it to redefine the VM's libvirt checkpoint across restarts and live migrations. You will see it stick around in the VM's namespace even after a backup completes, that is expected.

### When a full backup happens automatically

You don't need to manage full-versus-incremental yourself. The controller decides this on its own, and falls back to a full backup automatically in a few situations: when it can't find or validate a previous checkpoint chain in your BackupStorageLocation (for example, if something in the bucket was deleted or changed outside of normal operation), or when the `maxIncrementalBackups` limit configured on the DPA has been reached for that VM (see [configuration.md](./configuration.md)). Restarting the VM does not force a full backup and does not invalidate its checkpoint chain, a backup taken after a restart stays incremental as normal, because the controller deliberately keeps the VM's `VirtualMachineBackupTracker` on the cluster across restarts rather than deleting it. If that tracker object is ever missing when a new backup starts, either because it was deleted manually or the VM's namespace was recreated, the controller tries to rebuild it from the archived state in object storage first, and only falls back to a full backup if that archive can't be found either. There is currently no supported way to request a one-off full backup directly from the Backup or VirtualMachine object, and manually editing or deleting anything in object storage is not a supported way to reset the chain either. If you need a full backup on demand, lower `maxIncrementalBackups` (either on the DPA or with the per-VM `kubevirt-datamover.io/max-incremental-backups` annotation) so the next backup crosses the limit and falls back to full.

## Step 4: Confirm the backup completed successfully

```bash
oc get backups.velero.io my-vm-backup -n openshift-adp -o jsonpath='{.status.phase}'
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

### Expect the restored VM to start out halted

Before the restore, the plugin stops the VM and remembers whether it was running or stopped at backup time. This is expected and not a sign of a failed restore. If the VM had more than one disk, each disk gets its own `DataDownload`, and the controller only puts the VM back into its original run state once every one of those `DataDownload`s for that VM has reached `Completed`. In practice this means a freshly restored multi-disk VM can sit in a Halted or Stopped state for a little while, then start on its own once all of its disks are done. Don't start the VM manually while restores are still in progress, just wait for it to come up by itself.

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

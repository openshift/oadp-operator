# Troubleshooting KubeVirt DataMover

This guide covers the failure modes you are most likely to run into with KubeVirt DataMover, how to tell them apart, and where to look for more detail. It assumes you have gone through [configuration.md](./configuration.md) and are following the workflow in [backup-restore.md](./backup-restore.md).

## Where to find logs

The kubevirt-datamover-controller does the vast majority of the work, so start there:

```bash
oc logs -n openshift-adp deployment/oadp-kubevirt-datamover-controller-manager
```

An important detail here: the datamover and downloader pods that actually move VM disk data are short-lived, and the controller deletes them once they finish. You do not need to catch them running to see what happened inside them. Before deleting a pod, the controller streams that pod's own logs into its own log output, tagged with the pod name and source, so everything you need is in the controller's log stream even after the pod itself is gone. Filter for lines mentioning "Datamover pod log" or the specific DataUpload/DataDownload name if you need to isolate one operation.

Also check events on the relevant objects, since the controller emits Kubernetes events for major phase transitions and failures:

```bash
oc get events -n openshift-adp --field-selector involvedObject.kind=DataUpload
oc get events -n my-vm-namespace --field-selector involvedObject.kind=VirtualMachine
```

## Common issues

### Backup stuck in Velero's InProgress phase

Check the DataUpload phase directly:

```bash
oc get datauploads.velero.io -n openshift-adp -o wide
oc describe datauploads.velero.io <name> -n openshift-adp
```

Compare the DataUpload phase against the reconciliation flow: `New`, `Accepted`, `Prepared`, `InProgress`, then `Completed` or `Failed`. If it is stuck at `New` or `Accepted` for a long time, the controller may be waiting on another DataUpload for the same VM to finish first. Only one active DataUpload per VM is allowed at a time, which prevents two backups from stepping on the same checkpoint chain. Check whether an older DataUpload for the same VM is still active:

```bash
oc get datauploads.velero.io -n openshift-adp -o custom-columns=NAME:.metadata.name,PHASE:.status.phase,VM:.metadata.annotations.kubevirt\\.io/vm-name
```

If an older DataUpload appears to be stuck rather than genuinely still running, the controller has a built in safety valve for this: after the `staleDataUploadThreshold` (2 hours by default), a DataUpload that is still sitting in an active phase is treated as stale and no longer blocks newer DataUploads for the same VM. If you are seeing this often, it usually means something upstream (KubeVirt, storage, or the object storage endpoint) is failing silently rather than erroring out cleanly. Check the controller logs for the specific VM and look for repeated retries.

### CBT not enabled or not working

If the very first backup on a VM you expect to be incremental turns out to be a full backup every single time, or backups are much slower than expected, CBT is probably not actually active for that VM even if you added the label. Confirm:

1. The VM has the `changedBlockTracking: "true"` label (this goes on the `VirtualMachine`, not the `DataVolume` or PVC).
2. The VM's disk volumes use `volumeMode: Block`. CBT does not work with filesystem-mode volumes.
3. HCO has the CBT feature gate enabled cluster-wide, and the HCO/KubeVirt version meets the minimum (HCO 1.18+, KubeVirt 1.8.2+).
4. The VM was restarted after the label was added, if you added it to an already-running VM. Some KubeVirt versions only pick up CBT at VM start.

### Restore fails partway through

Check the DataDownload's phase and events the same way you would for a DataUpload:

```bash
oc get datadownloads.velero.io -n openshift-adp -o wide
oc describe datadownloads.velero.io <name> -n openshift-adp
```

A restore failure partway through the checkpoint chain usually means one of the incremental checkpoints referenced in the chain is missing or corrupted in object storage. This can happen if someone manually deleted objects out of the bucket, or if a lifecycle policy on the bucket expired objects that were still referenced by a VM's checkpoint chain. If you use bucket lifecycle rules, make sure they exclude the datamover checkpoint prefix, or align expiration with your actual backup retention policy so you never expire a checkpoint that a stored backup still depends on.

### Credential or authentication errors

These show up as errors in the controller log mentioning the object storage provider (AWS, Azure, or GCP), typically at the start of a DataUpload or DataDownload, before any actual data movement happens. Since the controller reads the same BackupStorageLocation and credentials Velero uses, the first thing to check is whether ordinary non-VM Velero backups to the same BackupStorageLocation work. If they do not, the problem is in your BSL/credential configuration generally, not specific to KubeVirt DataMover.

If ordinary Velero backups work fine but VM backups with KubeVirt DataMover fail on credentials, check that the datamover controller pod actually has the environment variables or projected token it expects. For AWS STS setups and Azure Workload Identity setups, the operator propagates the same environment variables it configures for Velero itself, so compare the Velero deployment's environment against the datamover controller deployment's environment if you suspect a mismatch.

### DPA validation error when enabling the plugin

If applying your DPA fails with a message like "only a single instance of KubeVirt DataMover Controller can be installed across the entire cluster," another DPA on the cluster already has `kubevirt-datamover` enabled and its controller deployment already exists. KubeVirt DataMover's controller is a cluster-scoped singleton, not a per-namespace deployment, so only one DPA across the whole cluster can have it enabled at a time. Check for other DPAs:

```bash
oc get dpa -A
```

### Warning about missing kubevirt plugin

If you see a warning that VM restore requires the `kubevirt` plugin, add `kubevirt` alongside `kubevirt-datamover` in `spec.configuration.velero.defaultPlugins`. `kubevirt-datamover` handles disk data movement, but VM metadata and file-level restore actions are handled by the separate kubevirt-velero-plugin (the `kubevirt` plugin). Both need to be present for a complete VM backup and restore workflow.

## Known limitations

- **Single active backup per VM**: only one DataUpload can be active for a given VM at a time. Concurrent backups of the same VM are not supported.
- **Block volume mode required**: CBT-based backup only works with `volumeMode: Block` PVCs. Filesystem-mode disks fall back to whatever your volume policy routes them to (typically a CSI snapshot or File System Backup), not KubeVirt DataMover.
- **Object storage required**: KubeVirt DataMover always requires `snapshotMoveData: true` and a working BackupStorageLocation. There is no in-cluster-snapshot-only mode.
- **Cluster-wide singleton controller**: only one DPA per cluster can have KubeVirt DataMover enabled.
- **VirtualMachineBackup/VirtualMachineBackupTracker are transient**: the controller creates and deletes these CRs as part of normal operation, archiving their state to object storage. If you are scripting around these objects directly, expect them to disappear once a backup or restore completes; treat the archived JSON in object storage as the durable record, not the live cluster objects.
- **Restore chains rebuild incrementally**: a restore from a VM with a long incremental chain replays each checkpoint in sequence via `qemu-img rebase`, rather than restoring straight from the full backup. A very long incremental chain can make individual restores slower than you might expect, even though it keeps backups themselves fast. This is a reasonable trade to be aware of when deciding on your `maxIncrementalBackups` setting.

If you run into an issue that is not covered here, the most useful thing to collect before opening a bug report is the full controller log around the time of the failure, plus `oc describe` output for the affected DataUpload or DataDownload and the corresponding Velero Backup or Restore object.

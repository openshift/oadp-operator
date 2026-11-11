# Troubleshooting virtualization backup and restore

1. [Create a backup for a single VM in a namespace with multiple VMs.]( #create-a-backup-for-a-single-vm-in-a-namespace-with-multiple-vms)
1. [Create a restore for a single VM in a backup with multiple VMs.](#create-a-restore-for-a-single-vm-in-a-backup-with-multiple-vms)
1. [Backups fail or are not application-consistent when the descheduler is enabled.](#backups-fail-or-are-not-application-consistent-when-the-descheduler-is-enabled)

## Create a backup for a single VM in a namespace with multiple VMs.

If you have a namespace with many VMs, and only want to back up one of them, you can use a label selector with the `app: vmname` label to indicate which VM should be included in the backup. For example:
```yaml
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: vmbackupsingle
  namespace: openshift-adp
spec:
  snapshotMoveData: true
  includedNamespaces:
  - bkp1
  labelSelector:
    matchLabels:
      app: vm1
  storageLocation: aws-dpa-1
```
Creating a restore pointing to `vmbkpsingle` will then create only one VM, `vm1`:
```yaml
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: vmrestoresingle
  namespace: openshift-adp
spec:
  backupName: vmbackupsingle
  restorePVs: true
```

## Create a restore for a single VM in a backup with multiple VMs.

If you have a backup containing multiple VMs, and you only want to restore one VM, you can use label selectors to choose the VM to restore. VMs usually have an `app: vmname` label that can be used for this purpose. However, this label may be occupied by the containerized-data-importer for DataVolume-created PVCs, so a Data Mover restore will result in a VM stuck in provisioning due to the mismatched `app` label. It is possible to apply a new label prior to the backup with `oc label pvc vm-name=vm-name ...` and later filter the restore with this new label, but for existing backups it may be preferrable to include a filter on the label `kubevirt.io/created-by`. For standard VM creation workflows, this label is set to the UID of the VM that the PVC is attached to, so the PVC should be correctly restored. For example, if the VM named `vm2` has UID `b683b53a-ddd7-4d9d-9407-a0c2b77ce0e5`:

```yaml
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: singlevmrestore
  namespace: openshift-adp
spec:
  backupName: multiplevmbackup
  restorePVs: true
  orLabelSelectors:
    - matchLabels:
        kubevirt.io/created-by: b683b53a-ddd7-4d9d-9407-a0c2b77ce0e5
    - matchLabels:
        app: vm2
```
If you deleted the VM without recording the UID, it is possible to retrieve it by attempting an intentionally failing restore without the PVC, and then copying the UID from the YAML of the failing VM.

## Backups fail or are not application-consistent when the descheduler is enabled.

The [Kube Descheduler Operator](https://github.com/openshift/cluster-kube-descheduler-operator) can evict pods while a backup is running. This is most visible with the `KubeVirtRelieveAndMigrate` profile, which evicts pods from busy nodes and live migrates VMs, but any profile that evicts pods can cause the same problems.

Problems seen during backups:

- **Data Mover pod evicted.** Data Mover pods have an owner reference to their `DataUpload` or `DataDownload`, so the descheduler treats them as evictable. When a Data Mover pod is evicted, the `DataUpload` can stay `InProgress` until the item operation timeout (4h by default) expires. See [velero-io/velero#10303](https://github.com/velero-io/velero/issues/10303).
- **VM live migrated during the backup.** After a migration, the freeze and unfreeze hooks can run against the old, already completed `virt-launcher` pod. The VM is not frozen when its PVC is snapshotted, so the backup is only crash-consistent, and the backup reports hook errors.

Fixes are in progress. Until they are available, keep the descheduler away from OADP and from the VMs being backed up:

- Exclude the OADP namespace and the VM namespaces in the `KubeDescheduler` custom resource with `spec.profileCustomizations.namespaces.excluded`:

  ```yaml
  apiVersion: operator.openshift.io/v1
  kind: KubeDescheduler
  metadata:
    name: cluster
    namespace: openshift-kube-descheduler-operator
  spec:
    mode: Automatic
    profiles:
    - KubeVirtRelieveAndMigrate
    profileCustomizations:
      namespaces:
        excluded:
        - openshift-adp
        - <vm_namespace>
  ```

  The descheduler operator already excludes `kube-system`, `hypershift`, and `openshift-*` namespaces, but it builds that list only when the operator starts. A namespace such as `openshift-adp` that is created after the descheduler operator starts is not excluded until the operator restarts, so list it explicitly. `namespaces.excluded` cannot be combined with `namespaces.included`. See the [`Namespaces` API](https://github.com/openshift/cluster-kube-descheduler-operator/blob/e953834d07d69aabc73e3a0726d35f20163ed71b/pkg/apis/descheduler/v1/types_descheduler.go#L230-L235) and how the [excluded namespaces are built](https://github.com/openshift/cluster-kube-descheduler-operator/blob/e953834d07d69aabc73e3a0726d35f20163ed71b/pkg/operator/target_config_reconciler.go#L108-L119).

- Or set `spec.mode: Predictive` during backup windows. In `Predictive` mode the descheduler only simulates evictions. Set it back to `Automatic` after the backups finish. See the [`Mode` API](https://github.com/openshift/cluster-kube-descheduler-operator/blob/e953834d07d69aabc73e3a0726d35f20163ed71b/pkg/apis/descheduler/v1/types_descheduler.go#L284-L293).

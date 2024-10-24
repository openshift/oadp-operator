# Troubleshooting virtualization backup and restore

1. [Create a backup for a single VM in a namespace with multiple VMs.]( #create-a-backup-for-a-single-vm-in-a-namespace-with-multiple-vms)
1. [Create a restore for a single VM in a backup with multiple VMs.](#create-a-restore-for-a-single-vm-in-a-backup-with-multiple-vms)

## Create a backup for a single VM in a namespace with multiple VMs.

If you have a namespace with many VMs, and only want to back up one of them, you can use a label selector with the `app: vmname` label to indicate which VM should be included in the backup. For example:
```
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
```
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

```
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

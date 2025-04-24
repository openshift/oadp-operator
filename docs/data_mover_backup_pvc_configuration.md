# Advanced Configuration for the Data Movement

This guide provides instructions for configuring intermediate PVC behavior used during backup and restore workflows in OADP via the Data Protection Application (DPA) Custom Resource Definition (CRD).

The settings introduced here provide control over how NodeAgent handles intermediate persistent volumes used in data movement operations.

The following sections describe how to configure:

- `backupPVC`
- `restorePVC`

These settings are defined under the `.spec.configuration.nodeAgent` section of the DPA CRD.

---

## 1. Configuring backupPVC for Snapshot Data Movement

The `backupPVC` field allows you to define behavior for intermediate PVCs used during snapshot-to-object-store data movement. You can configure multiple storage classes, each with specific parameters.

Each storage class block within backupPVC supports the following fields:

- `storageClass` – the name of the storage class to use for the intermediate PVC.
- `readOnly` – indicates if the PVC should be mounted as read-only. In some storage providers this can significantly reduce the performance when creating the backup from the snapshot. Setting this option to true requires `spcNoRelabeling` to be set to true as well.
- `spcNoRelabeling` – disables automatic relabeling of the SecurityContextConstraints if set to true. It must be set to true when `readOnly` is set to true.

Please refer to the Velero documentation for more details on the `backupPVC` feature. In the OADP project, this feature is controlled via the `nodeAgent.backupPVC` field in the DPA CRD, not through a ConfigMap. The latter is automatically created and managed by the OADP controller for you: [BackupPVC Configuration for Data Movement Backup](https://velero.io/docs/main/data-movement-backup-pvc-configuration/).


Example Configuration

The following example configures two storage classes: `gp3-csi` and `another-storage-class`. The gp3-csi PVC is mounted read-write, while `another-storage-class` is mounted read-only.

```yaml
spec:
  configuration:
    nodeAgent:
      enable: true
      backupPVC:
        another-storage-class:
          readOnly: true
          spcNoRelabeling: true # required when readOnly is true
          storageClass: gp3-csi
        gp3-csi:
          readOnly: false
          spcNoRelabeling: false
          storageClass: gp3-csi
```



## 2. Configuring restorePVC for Snapshot Data Movement

The `restorePVC` field sets the behavior of intermediate PVCs used during restore operations. Currently, it supports a single option:

- `ignoreDelayBinding` – when set to true, the data movement restore will bypass waiting for storage with WaitForFirstConsumer binding mode and proceed with restore PVC scheduling.

This can help avoid delays in cluster environments with custom scheduler configurations or in cases where prompt restore is preferred.

Example Configuration

```yaml
spec:
  configuration:
    nodeAgent:
      enable: true
      restorePVC:
        ignoreDelayBinding: true
```

Please refer to the Velero documentation for more details on the `restorePVC` feature. In the OADP project, this feature is controlled via the `nodeAgent.restorePVC` field in the DPA CRD, not through a ConfigMap. The latter is automatically created and managed by the OADP controller for you: [RestorePVC Configuration for Data Movement Restore](https://velero.io/docs/v1.16/data-movement-restore-pvc-configuration/).
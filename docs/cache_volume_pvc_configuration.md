# Cache Volume PVC Configuration for Data Movement Restore

This guide describes how to configure dedicated cache volumes for Kopia repository cache storage during restore operations in OADP via the Data Protection Application (DPA) Custom Resource Definition (CRD).

## Problem

During data movement restore operations (DataDownload and PodVolumeRestore), Kopia stores repository cache data on the restore pod's root filesystem at `/root/.cache/kopia/`. The default cache limit is 5 GB per concurrent operation. In environments with small system disks or high restore concurrency, this can exhaust the node's ephemeral storage, causing pod evictions and restore failures.

## Solution

The `cachePVC` configuration enables OADP to provision dedicated PersistentVolumeClaims for Kopia cache storage during restore operations. Instead of consuming ephemeral storage on the node's root filesystem, cache data is written to dynamically provisioned PVCs that are automatically created per restore operation and cleaned up upon completion.

This setting is defined under the `.spec.configuration.nodeAgent` section of the DPA CRD.

## Configuration

The `cachePVC` field supports the following parameters:

- `storageClass` (optional) - The name of the Kubernetes StorageClass to use for provisioning cache PVCs. The StorageClass must:
  - Support `ReadWriteOnce` access mode
  - Support `Filesystem` volume mode
  - Have a `Delete` reclaim policy (to ensure cache PVCs are properly cleaned up)

  If this field is omitted or empty, OADP will automatically attempt to use the cluster's default StorageClass (the one annotated with `storageclass.kubernetes.io/is-default-class: "true"`). If no default StorageClass is found, cache volume creation is disabled and Kopia falls back to using the pod's root filesystem for cache storage.

- `residentThresholdInMB` (optional) - The minimum size (in MB) of the backup data that triggers cache PVC creation. If the backup being restored is smaller than this threshold, no cache PVC is created and the cache remains on the root filesystem. This avoids the overhead of provisioning a PVC for small restores. If set to 0 or omitted, a cache PVC is created for all restores.

## Cache PVC Size Calculation

The size of each cache PVC is calculated as:

```
cacheVolumeSize = cacheLimitMB * 1.2 (rounded up to the nearest GB)
```

Where `cacheLimitMB` defaults to 5 GB (Kopia's default) but can be overridden via the `cacheLimitMB` field in the DPA. For example:

- Default (no `cacheLimitMB` set): 5 GB x 1.2 = 6 GB cache PVC
- With `cacheLimitMB: 10240`: 10 GB x 1.2 = 12 GB cache PVC

## Lifecycle

1. When a restore operation starts, a cache PVC is created with the calculated size
2. The cache PVC is mounted to the restore pod
3. Kopia uses the mounted path for local repository caching
4. When the restore completes, the cache PVC is automatically deleted
5. The `Delete` reclaim policy on the StorageClass ensures the underlying PersistentVolume is also cleaned up

## Example Configuration

### Basic configuration with cache volume (explicit StorageClass)

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
  namespace: openshift-adp
spec:
  configuration:
    nodeAgent:
      enable: true
      uploaderType: kopia
      cachePVC:
        storageClass: gp3-csi
    velero:
      defaultPlugins:
        - openshift
        - aws
```

### Basic configuration using the cluster's default StorageClass

When `storageClass` is omitted, OADP resolves the cluster's default StorageClass automatically:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
  namespace: openshift-adp
spec:
  configuration:
    nodeAgent:
      enable: true
      uploaderType: kopia
      cachePVC: {}
    velero:
      defaultPlugins:
        - openshift
        - aws
```

### Configuration with resident threshold and custom cache limit

This example only creates cache PVCs for restores larger than 1 GB and sets the Kopia cache limit to 10 GB:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
  namespace: openshift-adp
spec:
  configuration:
    nodeAgent:
      enable: true
      uploaderType: kopia
      cachePVC:
        storageClass: gp3-csi
        residentThresholdInMB: 1024
      cacheLimitMB: 10240
    velero:
      defaultPlugins:
        - openshift
        - aws
```

## Backward Compatibility

If no `cachePVC` configuration is provided, OADP and Velero fall back to the original behavior of storing cache data on the pod's root filesystem. Existing DPA configurations continue to work without modification.

## Related Settings

- `cacheLimitMB` - Controls the Kopia repository cache size limit. Defined in `spec.configuration.nodeAgent.cacheLimitMB`. This affects the size of cache PVCs when `cachePVC` is configured.
- `backupPVC` - Controls intermediate PVCs for snapshot data movement (backup direction). See [Data Mover Backup PVC Configuration](data_mover_backup_pvc_configuration.md).
- `restorePVC` - Controls intermediate PVCs for restore data movement. See [Data Mover Backup PVC Configuration](data_mover_backup_pvc_configuration.md).

Please refer to the Velero documentation for more details on the cache volume feature. In the OADP project, this feature is controlled via the `nodeAgent.cachePVC` field in the DPA CRD, not through a ConfigMap. The latter is automatically created and managed by the OADP controller for you: [Cache Volume for Data Movement](https://velero.io/docs/main/data-movement-cache-volume/).

<h1>API References</h1>

### DataProtectionApplicationSpec

| Property             | Type                                                                        | Description                                                                                                     |
|----------------------|-----------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------|
| backupLocations      | [] [BackupLocation](https://pkg.go.dev/github.com/openshift/oadp-operator@v1.0.4/api/v1alpha1#BackupLocation)                                                           | BackupLocations defines the list of desired configuration to use for BackupStorageLocations                     |
| snapshotLocations    | [] [SnapshotLocation](https://pkg.go.dev/github.com/openshift/oadp-operator@v1.0.4/api/v1alpha1#SnapshotLocation)                                                         | SnapshotLocations defines the list of desired configuration to use for VolumeSnapshotLocations                  |
| unsupportedOverrides | map [ [UnsupportedImageKey](https://pkg.go.dev/github.com/openshift/oadp-operator@v1.0.4/api/v1alpha1#UnsupportedImageKey) ] [string](https://pkg.go.dev/builtin#string)                                          | UnsupportedOverrides can be used to override the deployed dependent images for development. Options are `veleroImageFqin`, `awsPluginImageFqin`, `openshiftPluginImageFqin`, `azurePluginImageFqin`, `gcpPluginImageFqin`, `csiPluginImageFqin`, `dataMoverImageFqin`, `resticRestoreImageFqin`, `kubevirtPluginImageFqin`, and `operator-type`                     |
| podAnnotations       | map [ [string](https://pkg.go.dev/builtin#string) ] [string](https://pkg.go.dev/builtin#string)                                                       | Used to add annotations to pods deployed by operator                                                            |
| podDnsPolicy         | [corev1.DNSPolicy] ( https://pkg.go.dev/k8s.io/api/core/v1#DNSPolicy)       | DNSPolicy defines how a pod's DNS will be configured.                                                           |
| podDnsConfig         | [corev1.PodDNSConfig] ( https://pkg.go.dev/k8s.io/api/core/v1#PodDNSConfig) | PodDNSConfig defines the DNS parameters of a pod in addition to those generated from DNSPolicy.                 |
| backupImages         | *[bool](https://pkg.go.dev/builtin#bool)                                                                       | BackupImages is used to specify whether you want to deploy a registry for enabling backup and restore of images |
| configuration        | *[ApplicationConfig](https://pkg.go.dev/github.com/openshift/oadp-operator@v1.0.4/api/v1alpha1#ApplicationConfig)                                                          | Configuration is used to configure the data protection application's server config.                             |

### BackupLocation

| Property | Type                                                                                              | Description                                                                                    |
|----------|---------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| name     | metav1. ObjectMeta                                                                                |                                                                                                |
| velero   | [*velero.BackupStorageLocationSpec](https://velero.io/docs/v1.6/api-types/backupstoragelocation/) | Location to store volume snapshots. For further details, see  [here] ( config/bsl_and_vsl.md). |

### VolumeSnapshot

| Property | Type                                                                                                | Description                                                                                    |
|----------|-----------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| name     | metav1. ObjectMeta                                                                                  |                                                                                                |
| velero   | [*velero.VolumeSnapshotLocationSpec](https://velero.io/docs/v1.6/api-types/volumesnapshotlocation/) | Location to store volume snapshots. For further details, see  [here] ( config/bsl_and_vsl.md). |

### ApplicationConfig (DataProtectionApplicationSpec.Configuration)

| Property | Type          | Description                                          |
|----------|---------------|------------------------------------------------------|
| velero   | *VeleroConfig | This defines the configuration for the Velero server |
| restic   | *resticConfig | This defines the configuration for the Restic server |

### VeleroConfig

| Property                        | Type                    | Description                                                                                                                                                                                                                                              |
|---------------------------------|-------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| featureFlags                    | [] string               | FeatureFlags defines the list of features to enable for Velero instance                                                                                                                                                                                  |
| defaultPlugins                  | [] string               | Five types of default Velero plugins can be installed:  `AWS` ,  `GCP` ,  `Azure`  and  `OpenShift` , and  `CSI` . See  [here] ( config/plugins.md) for further information.                                                                             |
| customPlugins                   | map [string]interface{} | Used for installation of custom Velero plugins. See  [here] ( config/plugins.md) for further information.                                                                                                                                                |
| restoreResourcesVersionPriority | string                  | RestoreResourceVersionPriority represents a configmap that will be created if defined for use in conjunction with `EnableAPIGroupVersions` feature flag. Defining this field automatically add EnableAPIGroupVersions to the velero server feature flag  |
| noDefaultBackupLocation         | bool                    | If you need to install Velero without a default backup storage location NoDefaultBackupLocation flag is required for confirmation                                                                                                                        |
| podConfig                       | *PodConfig              | Velero Pod specific configuration                                                                                                                                                                                                                        |
| logLevel                       | string              | Velero server’s log level (default info, use debug for the most logging). Valid options are error, warn, warning, info, debug, trace                                                                                                                                                                                                                        |

### ResticConfig

| Property           | Type       | Description                                                                 |
|--------------------|------------|-----------------------------------------------------------------------------|
| enable             | *bool      | Enables backup/restore using Restic. If set to false, snapshots are needed. |
| supplementalGroups | []int64    | SupplementalGroups defines the linux groups to be applied to the Restic Pod |
| timeout            | string     | Timeout defines the Restic timeout, default value is 1h                     |
| PodConfig          | *PodConfig | Restic Pod specific configuration                                           |

### PodConfig

| Property            | Type                                                                                      | Description                                                                                                                               |
|---------------------|-------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------|
| nodeSelector        | map [ string ] string                                                                     | NodeSelector defines the nodeSelector to be supplied to Velero/Restic podSpec                                                             |
| tolerations         | [[]corev1. Toleration](https://pkg.go.dev/k8s.io/api/core/v1#Toleration)                  | Tolerations defines the list of tolerations to be applied to Velero Deployment/Restic daemonset                                                             |
| resourceAllocations | [corev1.ResourceRequirements](https://pkg.go.dev/k8s.io/api/core/v1#ResourceRequirements) | Set specific resource  `limits`  and  `requests`  for the Velero/Restic pods. For more information, go  [here] ( config/resource_req_limits.md). |


See also [![Go Reference](https://pkg.go.dev/badge/github.com/openshift/oadp-operator.svg)](https://pkg.go.dev/github.com/openshift/oadp-operator) for a deeper dive.

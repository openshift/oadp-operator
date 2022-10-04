<h1>API References</h1>

### [DataProtectionApplicationSpec](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#DataProtectionApplicationSpec)

| Property             | Type                                                                        | Description                                                                                                     |
|----------------------|-----------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------|
| backupLocations      | [] [BackupLocation](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#BackupLocation)                                                           | BackupLocations defines the list of desired configuration to use for BackupStorageLocations                     |
| snapshotLocations    | [] [SnapshotLocation](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#SnapshotLocation)                                                         | SnapshotLocations defines the list of desired configuration to use for VolumeSnapshotLocations                  |
| unsupportedOverrides | map [ [UnsupportedImageKey](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#UnsupportedImageKey) ] [string](https://pkg.go.dev/builtin#string)                                          | UnsupportedOverrides can be used to override the deployed dependent images for development. Options are `veleroImageFqin`, `awsPluginImageFqin`, `openshiftPluginImageFqin`, `azurePluginImageFqin`, `gcpPluginImageFqin`, `csiPluginImageFqin`, `dataMoverImageFqin`, `resticRestoreImageFqin`, `kubevirtPluginImageFqin`, and `operator-type`                     |
| podAnnotations       | map [ [string](https://pkg.go.dev/builtin#string) ] [string](https://pkg.go.dev/builtin#string)                                                       | Used to add annotations to pods deployed by operator                                                            |
| podDnsPolicy         | [corev1.DNSPolicy] ( https://pkg.go.dev/k8s.io/api/core/v1#DNSPolicy)       | DNSPolicy defines how a pod's DNS will be configured.                                                           |
| podDnsConfig         | [corev1.PodDNSConfig] ( https://pkg.go.dev/k8s.io/api/core/v1#PodDNSConfig) | PodDNSConfig defines the DNS parameters of a pod in addition to those generated from DNSPolicy.                 |
| backupImages         | *[bool](https://pkg.go.dev/builtin#bool)                                                                       | BackupImages is used to specify whether you want to deploy a registry for enabling backup and restore of images |
| configuration        | *[ApplicationConfig](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#ApplicationConfig)                                                          | Configuration is used to configure the data protection application's server config.                             |
| features             | *[Features](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#Features)                                                                   | Features defines the configuration for the DPA to enable the tech preview features                           |

### [BackupLocation](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#BackupLocation)

| Property | Type                                                                                              | Description                                                                                    |
|----------|---------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| velero   | [*velero.BackupStorageLocationSpec](https://velero.io/docs/v1.9/api-types/backupstoragelocation/) | Location to store backup objects. For further details, see  [here](config/bsl_and_vsl.md). |
| bucket   | [*CloudStorageLocation](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#CloudStorageLocation) | [Tech Preview] Automates creation of bucket at some cloud storage providers for use as a backup storage location |


### [SnapshotLocation](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#SnapshotLocation)

| Property | Type                                                                                                | Description                                                                                    |
|----------|-----------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| velero   | [*velero.VolumeSnapshotLocationSpec](https://velero.io/docs/v1.9/api-types/volumesnapshotlocation/) | Location to store volume snapshots. For further details, see  [here] ( config/bsl_and_vsl.md). |

### [ApplicationConfig](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#ApplicationConfig)

| Property | Type          | Description                                          |
|----------|---------------|------------------------------------------------------|
| velero   | *[VeleroConfig](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#VeleroConfig) | This defines the configuration for the Velero server |
| restic   | *[ResticConfig](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#ResticConfig) | This defines the configuration for the Restic server |

### [VeleroConfig](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#VeleroConfig)

| Property                        | Type                    | Description                                                                                                                                                                                                                                              |
|---------------------------------|-------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| featureFlags                    | [] [string](https://pkg.go.dev/builtin#string)               | FeatureFlags defines the list of features to enable for Velero instance                                                                                                                                                                                  |
| defaultPlugins                  | [] [string](https://pkg.go.dev/builtin#string)               | Five types of default Velero plugins can be installed:  `AWS` ,  `GCP` ,  `Azure`  and  `OpenShift` , and  `CSI` . See  [here] ( config/plugins.md) for further information.                                                                             |
| customPlugins                   | [][CustomPlugin](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#CustomPlugin) | Used for installation of custom Velero plugins. See  [here] ( config/plugins.md) for further information.                                                                                                                                                |
| restoreResourcesVersionPriority | [string](https://pkg.go.dev/builtin#string)                  | RestoreResourceVersionPriority represents a configmap that will be created if defined for use in conjunction with `EnableAPIGroupVersions` feature flag. Defining this field automatically add EnableAPIGroupVersions to the velero server feature flag  |
| noDefaultBackupLocation         | [bool](https://pkg.go.dev/builtin#bool)                    | If you need to install Velero without a default backup storage location NoDefaultBackupLocation flag is required for confirmation                                                                                                                        |
| podConfig                       | *[PodConfig](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#PodConfig)              | Velero Pod specific configuration                                                                                                                                                                                                                        |
| logLevel                       | [string](https://pkg.go.dev/builtin#string)              | Velero server’s log level (default info, use debug for the most logging). Valid options are trace, debug, info, warning, error, fatal, or panic                                                                                                                                                                                                                        |

### [CustomPlugin](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#CustomPlugin)

| Property           | Type       | Description                                                                 |
|--------------------|------------|-----------------------------------------------------------------------------|
| name             |  [string](https://pkg.go.dev/builtin#string)     | Name of custom plugin |
| image             |  [string](https://pkg.go.dev/builtin#string)     | Image of custom plugin |

### [ResticConfig](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#ResticConfig)

| Property           | Type       | Description                                                                 |
|--------------------|------------|-----------------------------------------------------------------------------|
| enable             | *[bool](https://pkg.go.dev/builtin#bool)      | Enables backup/restore using Restic. If set to false, snapshots are needed. |
| supplementalGroups | [][int64](https://pkg.go.dev/builtin#int64)    | SupplementalGroups defines the linux groups to be applied to the Restic Pod |
| timeout            | [string](https://pkg.go.dev/builtin#string)     | Timeout defines the Restic timeout, default value is 1h                     |
| PodConfig          | *[PodConfig](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#PodConfig) | Restic Pod specific configuration                                           |

### [PodConfig](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#PodConfig)

| Property            | Type                                                                                      | Description                                                                                                                               |
|---------------------|-------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------|
| labels        | map [ [string](https://pkg.go.dev/builtin#string) ] [string](https://pkg.go.dev/builtin#string)                                                                     | Labels to add to pods                                                             |
| nodeSelector        | map [ [string](https://pkg.go.dev/builtin#string) ] [string](https://pkg.go.dev/builtin#string)                                                                     | NodeSelector defines the nodeSelector to be supplied to Velero/Restic podSpec                                                             |
| tolerations         | [][corev1.Toleration](https://pkg.go.dev/k8s.io/api/core/v1#Toleration)                  | Tolerations defines the list of tolerations to be applied to Velero Deployment/Restic daemonset                                                             |
| resourceAllocations | [corev1.ResourceRequirements](https://pkg.go.dev/k8s.io/api/core/v1#ResourceRequirements) | Set specific resource  `limits`  and  `requests`  for the Velero/Restic pods. For more information, go  [here] ( config/resource_req_limits.md). |

### [Features](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#Features)
| Property  | Type      | Description                                                                                                                               |
|-----------|-----------|-------------------------------------------------------------------------------------------------------------------------------------------|
| dataMover | [DataMover](https://pkg.go.dev/github.com/openshift/oadp-operator@master/api/v1alpha1#DataMover) | DataMover defines the various config for DPA data mover                                                             |

### DataMover
| Property       | Type | Description                                                                                                                   |
|----------------|------|-------------------------------------------------------------------------------------------------------------------------------|
| enable         | [bool](https://pkg.go.dev/builtin#bool) | Enable is used to specify whether you want to deploy the volume snapshot mover controller and a modified csi datamover plugin |
| credentialName | [string](https://pkg.go.dev/builtin#string) | User supplied Restic Secret name for DataMover |                                                                               |
| timeout        | [string](https://pkg.go.dev/builtin#string) | User supplied timeout to be used for VolumeSnapshotBackup and VolumeSnapshotRestore to complete, default value is 10m |                                                                               |

See also [![Go Reference](https://pkg.go.dev/badge/github.com/openshift/oadp-operator.svg)](https://pkg.go.dev/github.com/openshift/oadp-operator) for a deeper dive.

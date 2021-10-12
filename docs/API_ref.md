<h1>API References</h1>

| Property   |      Type     |  Description |
|:-----------|:--------------|:-------------|
| backupImages | bool |  Determine whether the Velero install will backup internal images when an imagestream is backed up.  |
| backupStorageLocations | [[]velero.BackupStorageLocationSpec](https://velero.io/docs/v1.6/api-types/backupstoragelocation/) |  Location(s) to store backups. For more details, see [here](config/bsl_and_vsl.md).  |
| customVeleroPlugins | map[string]interface{} |  Used for installation of custom Velero plugins. See [here](config/plugins.md) for further information.  |
| defaultVeleroPlugins |  []string |  Five types of default Velero plugins can be installed: `AWS`, `GCP`, `Azure` and `OpenShift`, and `CSI`. See [here](config/plugins.md) for further information. |
| enableRestic |   bool  |   Enables backup/restore using Restic. If set to false, snapshots are needed.  |
| Noobaa | bool |  An optional backup storage locaion. For more information, go [here](config/noobaa/install_oadp_noobaa.md). |
| podAnnotations |  map[string]string |   Add metadata to your pods to select and find certain pods. |
| podDnsConfig |    [corev1.PodDNSConfig](https://pkg.go.dev/k8s.io/api/core/v1#PodDNSConfig)   |        |
| podDndPolicy | [corev1.DNSPolicy](https://pkg.go.dev/k8s.io/api/core/v1#DNSPolicy) |         |
| resticNodeSelector | map[string]string |   Assign Restic pods to only certain nodes. |
| resticResourceAllocations | [corev1.ResourceRequirements](https://pkg.go.dev/k8s.io/api/core/v1#ResourceRequirements) |  Set specific resource `limits` and `requests` for the Restic pods. For more information, go [here](config/resource_req_limits.md). |
| resticSupplementalGroups | []int64  |        |
| resticTimeout | string | Used when a Restic backup/restore sits in progress for X amount of time. Defaults to 1 hour. Usage: `--restic-timeout` |
| resticTolerations | [[]corev1.Toleration](https://pkg.go.dev/k8s.io/api/core/v1#Toleration) |       |
| restoreResourcesVersionPriority |  string  |        |
| veleroFeatureFlags | []string{} |  Enables additional Velero features. For more details and usage, see [here](config/features_flag.md). |
| veleroResourceAllocations | [corev1.ResourceRequirements](https://pkg.go.dev/k8s.io/api/core/v1#ResourceRequirements) |  Set specific resource `limits` and `requests` for the Velero pod. For more information, go [here](config/resource_req_limits.md). |
| veleroTolerations | [[]corev1.Toleration](https://pkg.go.dev/k8s.io/api/core/v1#Toleration) |        |
| volumeSnapshotLocations | [[]velero.VolumeSnapshotLocationSpec](https://velero.io/docs/v1.6/api-types/volumesnapshotlocation/) |  Location to store volume snapshots. For further deatils, see [here](config/bsl_and_vsl.md). |

See also [![Go Reference](https://pkg.go.dev/badge/github.com/openshift/oadp-operator.svg)](https://pkg.go.dev/github.com/openshift/oadp-operator) for a deeper dive.
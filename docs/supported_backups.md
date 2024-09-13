# OADP Supported Backups

OADP supports backup and restore disaster recovery operations for both containerized and OpenShift Virtualization workloads. It is important to distinguish the nuances and support for each workload type.

## Containerized Workloads

OADP supports backup and restore operations for containerized workloads. 

| VolumeMode | FSB - restic | FSB - kopia | CSI | CSI DataMover ** |
|------------|--------------|-------------|-----|------------------|
| Filesystem | S, I         | S, I        | S   | S, I             |
| Block      | N            | N           | S   | S, I             |


## OpenShift Virtualization Workloads

OADP supports backup and restore operations for OpenShift Virtualization workloads.

| VolumeMode | FSB - restic | FSB - kopia | CSI | CSI DataMover ** |
|------------|--------------|-------------|-----|------------------|
| Filesystem | Supported    | Supported   | Supported | Supported  |
| Block      | Not Supported| Not Supported| Supported| Supported  |

* Legend:
  * S - Supported
  * I - Incremental Backup supported
  * N - Not Supported
  * FSB - File System Backup
  * **Note: DataMover upload and download operations use kopia regardless of uploader type.
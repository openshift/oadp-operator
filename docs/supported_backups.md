# OADP Supported Backups

OADP supports backup and restore disaster recovery operations for both containerized and OpenShift Virtualization workloads. It is important to distinguish the nuances and support for each workload type.

## Containerized Workloads

OADP supports backup and restore operations for containerized workloads. 

| VolumeMode | FSB - restic | FSB - kopia | CSI | CSI DataMover (ii) | Volume Snapshot |
|------------|--------------|-------------|-----|------------------| ----------------|
| Filesystem | S, I         | S, I        | S   | S, I             | S               |
| Block      | N            | N           | S   | S, I             | S               |


## OpenShift Virtualization Workloads

OADP supports backup and restore operations for OpenShift Virtualization workloads.

| VolumeMode | FSB - restic | FSB - kopia | CSI | CSI DataMover (ii) | Volume Snapshot |
|------------|--------------|-------------|-----|------------------| ----------------|
| Filesystem | N            | N           | S   | S, I             | N               |
| Block      | N            | N           | S   | S, I             | N               |

* Legend:
  * S - Supported
  * I - Incremental Backup supported
  * N - Not Supported
  * FSB - File System Backup
  * Volume Snapshot - Non CSI Volume support for AWS, GCP, Azure via plugins.
  * ii -  DataMover upload and download operations use kopia regardless of uploader type.
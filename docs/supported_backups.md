# OADP Supported Backups

OADP supports backup and restore disaster recovery operations for both containerized and OpenShift Virtualization workloads. It is important to distinguish the nuances and support for each workload type.

## Containerized Workloads

OADP supports backup and restore operations for containerized workloads. 

| VolumeMode | FSB - restic (i) | FSB - kopia | CSI | CSI DataMover (ii) | Cloud Provider Native Snapshots |
|------------|--------------|-------------|-----|------------------| ----------------|
| Filesystem | S, I         | S, I        | S   | S, I             | S               |
| Block      | N            | N           | S   | S, I             | S               |


## OpenShift Virtualization Workloads

OADP supports backup and restore operations for OpenShift Virtualization workloads.

| VolumeMode | FSB - restic (i) | FSB - kopia | CSI | CSI DataMover (ii) | Cloud Provider Native Snapshots |
|------------|--------------|-------------|-----|------------------| ----------------|
| Filesystem | N            | N           | S   | S, I             | N               |
| Block      | N            | N           | S   | S, I             | N               |

* Legend:
  * **S:** Supported
  * **I:** Incremental Backup supported
  * **N:** Not Supported
  * **FSB:** File System Backup
  * **Cloud Provider Native Snapshots:** Cloud Provider Native Volume support for [AWS](https://github.com/openshift/velero-plugin-for-aws), [GCP](https://github.com/openshift/velero-plugin-for-gcp), [Azure](https://github.com/openshift/velero-plugin-for-microsoft-azure) via plugins.
  * **i:** FSB - restic will be deprecated in 1.5.0, using kopia is recommended. Restoring restic backups will continue to be supported.
  * **ii:** DataMover upload and download operations use kopia regardless of uploader type.
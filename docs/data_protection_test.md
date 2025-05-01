# DataProtectionTest (DPT) CR usage documentation

This document explains the usage of the `DataProtectionTest` (DPT) custom resource introduced by the OADP Operator.  
It covers configuration, available fields, behavior, and example workflows.

---

## Overview

The `DataProtectionTest` (`dpt`) Custom Resource (CR) provides a framework to **validate** and **measure**:

- **Upload performance** to the object storage backend.
- **CSI snapshot readiness** for PersistentVolumeClaims.
- **Storage bucket configuration** (encryption/versioning for S3 providers).

This enables users to ensure their data protection environment is properly configured and performant.

---

## Spec Fields

| Field | Type | Description |
|:------|:-----|:------------|
| `backupLocationName` | string | Name of the existing BackupStorageLocation to use. |
| `backupLocationSpec` | object | Inline specification of the BackupStorageLocation (mutually exclusive with `backupLocationName`). |
| `uploadSpeedTestConfig` | object | Configuration to run an upload speed test to object storage. |
| `csiVolumeSnapshotTestConfigs` | list | List of PVCs to snapshot and verify snapshot readiness. |
| `forceRun` | boolean | Re-run the DPT even if status is already `Complete` or `Failed`. |

---

## Status Fields

| Field | Type | Description |
|:------|:-----|:------------|
| `phase` | string | Current phase: `InProgress`, `Complete`, or `Failed`. |
| `lastTested` | timestamp | Last time the tests were run. |
| `uploadTest` | object | Results of the upload speed test. |
| `bucketMetadata` | object | Information about the storage bucket encryption and versioning. |
| `snapshotTests` | list | Per-PVC snapshot test results. |
| `snapshotSummary` | string | Aggregated pass/fail summary for snapshots (e.g., `2/2 passed`). |
| `s3Vendor` | string | Detected S3-compatible vendor (e.g., `AWS`, `MinIO`, `Ceph`). |
| `errorMessage` | string | Top-level error message if the DPT fails. |

---

Notes:

- If DPT `status.phase` is `Complete` or `Failed` **and** `forceRun` is `false`, the controller **skips** re-running tests.
- If `forceRun: true`, the tests will re-execute, and `forceRun` is reset to `false` after execution.
- During a test run, the phase transitions:
    - `InProgress` -> `Complete` (on success)
    - `InProgress` -> `Failed` (on error)
- Upload test and snapshot tests are optional based on the spec fields populated.

---

## Printer Columns

When running:

```bash
oc get dpt dpt-sample-1 -n openshift-adp
```

You will see:

```bash
NAME           PHASE      LASTTESTED   UPLOADSPEED(MBPS)   ENCRYPTION   VERSIONING   SNAPSHOTS    AGE
dpt-sample-1   Complete   72s          660                 AES256       None         2/2 passed   72s
```

| Column | Description |
|:-------|:------------|
| Phase | Current phase of the DPT (`InProgress`, `Complete`, `Failed`). |
| LastTested | Timestamp of the last test run. |
| UploadSpeed(Mbps) | Upload speed result to the object storage. |
| Encryption | Storage bucket encryption algorithm (e.g., `AES256`). |
| Versioning | Storage bucket versioning state (e.g., `Enabled`, `Suspended`). |
| Snapshots | Pass/fail summary of snapshot tests (e.g., `2/2 passed`). |
| Age | Time since the DPT resource was created. |

---

## Example DataProtectionTest (DPT) CR
- Example 1
```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionTest
metadata:
  name: dpt-sample
  namespace: openshift-adp
spec:
  backupLocationName: sample-bsl
  uploadSpeedTestConfig:
    fileSize: 5MB
    timeout: 60s
  csiVolumeSnapshotTestConfigs:
    - volumeSnapshotSource:
        persistentVolumeClaimName: mysql
        persistentVolumeClaimNamespace: mysql-persistent
      snapshotClassName: csi-snapclass
      timeout: 2m
  forceRun: true
```

- Example 2
```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionTest
metadata:
  name: dpt-sample
  namespace: openshift-adp
spec:
  backupLocationSpec:
    provider: aws
    default: true
    objectStorage:
      bucket: sample-bucket
      prefix: velero
    config:
      region: us-east-1
      profile: "default"
      insecureSkipTLSVerify: "true"
      s3Url: "https://s3.amazonaws.com/sample-bucket"
    credential:
      name: cloud-credentials
      key: cloud
  uploadSpeedTestConfig:
    fileSize: 50MB
    timeout: 120s
  csiVolumeSnapshotTestConfigs:
    - volumeSnapshotSource:
        persistentVolumeClaimName: mongo
        persistentVolumeClaimNamespace: mongo-persistent
      snapshotClassName: csi-snapclass
      timeout: 2m
  forceRun: true
```
---

## Key Notes

- `uploadSpeedTestConfig` is optional. If not provided, upload tests are skipped.
- `csiVolumeSnapshotTestConfigs` is optional. If not provided, snapshot tests are skipped.
- Upload tests require appropriate cloud provider secrets.
- Snapshot tests require VolumeSnapshotClass and CSI snapshot support in the cluster.
- Set `forceRun: true` manually if you want to rerun tests without recreating the CR.

---

## Common Troubleshooting

| Symptom | Possible Cause | Resolution |
|:--------|:---------------|:-----------|
| DPT stuck in `InProgress` | Credentials or bucket access failure | Check Secret, bucket permissions, and logs. |
| Upload test failed | Incorrect secret or S3 endpoint | Validate BackupStorageLocation config and access keys. |
| Snapshot tests fail | CSI snapshot controller misconfiguration | Check VolumeSnapshotClass availability and CSI driver logs. |
| Bucket encryption/versioning not populated | Cloud provider limitations | Not all object stores expose these fields consistently. |

---


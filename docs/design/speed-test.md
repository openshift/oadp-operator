# Upload Speed Test for Backup Storage Location (Object Storage)

## Abstract

This document presents the design of a Custom Resource Definition (CRD) and its controller to test the upload speed from an OpenShift cluster to cloud object storage.
The design leverages the BackupStorageLocation configuration from the OADP operator’s DPA CRD to specify object storage locations for testing. The controller will use
these configurations to authenticate and perform uploads, measuring speed and reporting results through the CRD status.

## Background

We have observed several instances in customer environments where backup or restore operations became stalled or significantly delayed, but the root cause was not immediately apparent.
These issues could stem from poor network connectivity, cloud storage throttling, or misconfigurations in BackupStorageLocation (BSL) settings. Since backup performance plays a critical
role in disaster recovery strategies, it is essential to have a reliable way to test upload speeds.

This Upload Speed Test CRD will help in such scenarios by proactively measuring the upload speed from the OpenShift cluster to the cloud storage configured in OADP’s BSL. By identifying
performance bottlenecks ahead of backup or restore operations, administrators can take corrective actions to avoid failures during critical operations.

## Goals

- 	Utilize BackupStorageLocation (BSL) settings from the OADP operator’s DPA CRD for storage configurations and credentials.
-   Measure upload speeds from the OpenShift cluster to the configured object storage.
-   Update the test results in the status of the UploadSpeedTest CRD for visibility and tracking.
-   Ensure smooth integration with OADP operator-managed backup storage location configurations.


## Non-Goals

- This design will not create or modify BackupStorageLocation entries in OADP.
- It will not implement download or latency tests, focusing solely on upload speed.
- Scheduling of recurring tests is not supported in the initial version.

## High-Level Design
Components involved and their responsibilities:

- UploadSpeedTest (UST) CRD:
  - Captures the BSL configuration used for testing directly in the CRD.
  - Holds the test results, including success/failure and upload speed.
- UploadSpeedTest (UST) Controller:
  - Watches for changes to the UploadSpeedTest CRs and executes uploads based on the provided BSL configuration. 
  - Uses appropriate cloud SDKs (AWS, Azure, or GCP) to perform uploads.
  - Updates the status of UST CR with upload speed, upload success/failure and error message if any.
- OADP/DPA-UST Integration:
  - Fetches BackupStorageLocation (BSL) details from the OADP operator’s DPA CR. 
  - Populates the UploadSpeedTest CR with relevant configurations from the BSL.
  - Creates the UST CR for the BSL configured in DPA instance.
## Detailed Design

Proposed Upload Speed Test CRD:

```yaml
apiVersion: uploadspeedtest.io/v1alpha1
kind: UploadSpeedTest
metadata:
  name: upload-speed-test-sample
spec:
  dpaNamespace: oadp-operator    # Namespace of the DPA CR
  bslConfig:                     # Configuration from BackupStorageLocation
    name: aws-backup-location
    provider: aws
    region: us-east-1
    bucket: my-backup-bucket
    uploadSpeedTest:
      fileSize: 10MB             # Test file size
      testTimeout: 60s           # Timeout for upload test
  cloudProviderSecretRef: aws-credentials   # Secret for authentication
status:
  lastTested: "2024-10-08T10:00:00Z"
  uploadSpeedMbps: 55.3
  uploadSuccess: true
  errorMessage: ""
```

Changes to DPA CRD BSL Configuration spec:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: sample-dpa
spec:
  backupStorageLocations:
    - name: aws-backup-location
      provider: aws
      config:
        region: us-east-1
        bucket: my-backup-bucket
        uploadSpeedTest:
          enabled: true              # Flag to enable upload speed test
          fileSize: 10MB             # Size of the file to be uploaded
          testTimeout: 60s           # Timeout for the upload test
```

UST controller workflow:

Reconciliation loop:
- Watches for new or updated UploadSpeedTest CRD instances.
- Fetches the DPA CRD and retrieves the BSL configuration based on the provided name and namespace.
- Checks if the uploadSpeedTest.enabled flag is set. If enabled, it copies the BSL configuration into the UploadSpeedTest CRD and initiates the test.


Upload  execution:
- Uses the appropriate cloud SDK (AWS, Azure, GCP) based on the BSL configuration.
- Performs the upload and measures the speed in Mbps.
- Records whether the upload succeeded or failed.

Status Update:
- Updates the UploadSpeedTest CRD status with:
  - Upload speed (in Mbps)
  - Test outcome (success/failure)
  - Error messages (if any)

## Implementation


## Future Scope


## Open Questions

# DataProtectionTest CRD and Controller Design

## Abstract

This design introduces the **DataProtectionTest** (DPT) CRD and its controller to evaluate data protection performance—including backup upload and volume snapshot readiness—in an OpenShift cluster. It supports two tests:

1. **Upload Speed Test:** Measures the speed at which test data is uploaded to cloud object storage using configuration from a BackupStorageLocation.
2. **CSI VolumeSnapshot Test:** Creates a VolumeSnapshot(s) from a specified PVC source(s) and measures the time taken (as a duration) for the snapshot(s) to become ready.

Additionally, the design includes a mechanism to determine the S3-compatible vendor. DPT will also report object storage bucket metadata, such as encryption algorithm and versioning status

## Background

In several customer environments, backup/restore operations have experienced delays or stalls due to poor network connectivity, throttling at the cloud provider, or misconfigurations in BackupStorageLocation settings. Given the critical role of backup performance in disaster recovery, administrators need real-time, accurate metrics on how fast data uploads occur and how promptly snapshots are created.

This design addresses that need by measuring:

- **Upload speed:** How quickly data is transferred to object storage.
- **Snapshot performance:** How long a CSI VolumeSnapshot(s) take to become ReadyToUse.
- **S3 compatibility:** Identifying the specific S3 vendor by inspecting HTTP response headers from the storage endpoint.

## Goals

- Upload data and compute the speed in Mbps.
- Create a CSI VolumeSnapshot(s) and measure the time taken for it to become ready (reported as a duration).
- Identify the S3-compatible vendor using an HTTP call that inspects response headers.
- Gather information on object storage bucket metadata like versioning and encryption.
- Expose all results—including upload speed, snapshot ready duration, bucket metadata and S3 vendor—in the CRD status.
- Must gather will be updated to gather the DPT CR(s) for insights.


## Non-Goals

- This design will not create or modify BackupStorageLocation entries in OADP.
- It will not implement download or latency tests, focusing solely on upload speed.
- Scheduling of recurring tests is not supported in the initial version.
- Integration via DPA CR

## High-Level Design
Components involved and their responsibilities:

- **DataProtectionTest (DPT) CRD:**
    - **Spec:**
        - **backupLocationSpec:** Contains Velero-based backup storage configuration.
        - **backupLocationRef:** Contains the Name and Namespace reference for Velero BSL
        - **uploadSpeedTestConfig:** Test parameters for the upload (file size, test timeout).
        - **CSIVolumeSnapshotTestConfig:** Test parameters for the CSI VolumeSnapshot test (snapshot class, source PVC name and namespace for each PVC, plus snapshot timeout).
    - **Status:**
        - **UploadTestStatus:** Groups upload speed (in Mbps), duration of time taken to upload data,  success flag, and error messages.
        - **SnapshotTestStatus:** Groups the snapshot test results, reporting status and the duration taken for the snapshot(s) to be ready.
        - **S3Vendor:** Reports the detected S3 vendor string from vendor determination. (This is only applicable for S3-compatible object storage providers)
        - **BucketMetadata:** Reports the encryptionAlgorithm used for the bucket as well as the versioningStatus.

**Note:** Either `backupLocationSpec` or `backupLocationRef` will be processed for a particular DPT instance, if both are specified DPT would error out. 
- **DataProtectionTest Controller:**
    - Monitors DataProtectionTest CRs.
    - Extracts configuration from the backup location spec.
    - Determines the S3 vendor via an HTTP HEAD call.
    - Initializes the appropriate cloud provider using the CloudProvider interface.
    - Executes the upload test and, if enabled, the CSI VolumeSnapshot test.
    - Updates the CRD status with grouped results.
- **CloudProvider Interface:**
    - Defines an `UploadTest(ctx, config, bucket, fileSizeMB) (int64, error)` function.
    - Defines a `GetBucketMetadata(ctx context.Context, bucket string) (*v1alpha1.BucketMetadata, error)` function.
    - AWS-specific implementation (S3Provider) is provided using the AWS SDK.
- **Vendor Determination Logic:**
    - A helper function performs an HTTP HEAD call to the **s3Url** and inspects headers (especially the `Server` header) to determine the vendor (e.g., "AWS", "MinIO", etc.).

## Detailed Design

#### Proposed DataProtectionTest CRD:

DataProtectionTest (DPT) CRD would look like:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionTest
metadata:
  name: my-data-protection-test
spec:
  backupLocationRef:  # optional, either this or backupLocationSpec
    name: aws-bsl
    namespace: openshift-adp
  backupLocationSpec:
    provider: aws                # Cloud provider type (aws, azure, gcp)
    default: true
    objectStorage:
      bucket: sample-bucket
      prefix: velero
    config:
      region: us-east-1
      profile: "default"
      insecureSkipTLSVerify: "true"
      s3Url: "https://s3.amazonaws.com" # indicates s3 compatibility
    credential:
      name: cloud-credentials     # Secret for cloud credentials
      key: cloud
  uploadSpeedTestConfig:
    fileSize: "100MB"                # Size of file to upload for testing
    testTimeout: "60s"               # Maximum duration for upload test
  CSIVolumeSnapshotTestConfigs:
    - snapshotClassName: "csi-snapclass"
      volumeSnapshotSource:
        persistentVolumeClaimName: "db-pvc-1"
        persistentVolumeClaimNamespace: "app1"
      timeout: "60s"
    - snapshotClassName: "csi-snapclass"
      volumeSnapshotSource:
        persistentVolumeClaimName: "db-pvc-2"
        persistentVolumeClaimNamespace: "app1"
      timeout: "60s"
status:
  lastTested: "2024-10-08T10:00:00Z"
  bucketMetadata:
    encryptionAlgorithm: AES256
    versioningStatus: Enabled
  uploadTest:
    speedMbps: 55.3
    duration: 4.1s
    success: true
    errorMessage: ""
  snapshotTests:
  - persistentVolumeClaimName: "db-pvc-1"
    persistentVolumeClaimNamespace: "app1"
    status: "Ready"
    readyDuration: "45s"
  - persistentVolumeClaimName: "db-pvc-2"
    persistentVolumeClaimNamespace: "app1"
    status: "Failed"
    errorMessage: "timeout waiting for snapshot readiness"
  s3Vendor: "AWS"

```

#### CloudProvider interface

```go
package cloudprovider

// CloudProvider defines the interface for cloud-based upload tests.
type CloudProvider interface {
	// UploadTest performs a test upload and returns the speed in Mbps or error.
	UploadTest(ctx context.Context, config v1alpha1.UploadSpeedTestConfig, bucket string, fileSizeMB int) (int64, error)

	// GetBucketMetadata Fetches the object storage bucket metadata like encryptionAlgorithm and versioning status
	GetBucketMetadata(ctx context.Context, bucket string) (*v1alpha1.BucketMetadata, error)
}

```

#### DPT controller workflow General workflow for DPT CR processing - user created or via DPA CR):

1. Retrieve the DPT CR: At the start of the reconcile loop the controller fetches the DPT CR from the API server
2. Determine the s3 compatible vendor if applicable: Identify the s3 vendor by performing an HTTP request to the configured s3 storage and inspecting the response headers.
```go
// determineVendor inspects the s3Url via an HTTP HEAD request
// and extracts the S3-compatible vendor name from the Server header.
func (r *DataProtectionTestReconciler) determineVendor(ctx context.Context, dpt *oadpv1alpha1.DataProtectionTest) error {
    // check if s3Url is specified 
    // Send an HTTP HEAD request to the storage URL
    // Parse response 'Server' header to detect vendor
}
```
3. Initialize the Cloud Provider for the Upload Test: Instantiate a cloud provider based on the BSL config.
```go
// initializeProvider constructs a CloudProvider (currently S3) based on config and credentials from the DPT CR.
func (r *DataProtectionTestReconciler) initializeProvider(dpt *oadpv1alpha1.DataProtectionTest) (cloudprovider.CloudProvider, error) {
	
    // Get region (default to us-east-1 if not specified)
    // s3Url is required for custom endpoints (e.g., MinIO, Ceph)
    // Load credentials from Kubernetes secret
    // Parse access/secret key pair from the secret data 
	// Return a CloudProvider instance (S3Provider)
}

```
4. Execute the Upload Speed test: Upload test data of specified size to the object storage to measure the data transfer speed.
```go
// runUploadTest uploads test data to object storage and calculates the speed in Mbps.
func (r *DataProtectionTestReconciler) runUploadTest(ctx context.Context, dpt *oadpv1alpha1.DataProtectionTest, cp cloudprovider.CloudProvider) error {
    // Parse the data size (e.g., "100MB" -> 100)
    // Upload to the target bucket defined in the BackupLocation
    // Success: update speed and duration on status
}

```
5. Fetch the object storage bucket metadata.
```go
// getBucketMetadata fetches the object storage bucket metadata
func (r *DataProtectionTestReconciler) getBucketMetadata(ctx context.Context, dpt *oadpv1alpha1.DataProtectionTest, cp cloudprovider.CloudProvider) error {
    // Get the bucket metadata
	// Success: update status to add encryptionAlgorithm and versioning status
}

```

6.Execute the CSI VolumeSnapshot Test: Create a CSI VolumeSnapshot(s) for a specified PVC(s) and measure the time taken for it to be ready.
```go
// runSnapshotTest creates a CSI VolumeSnapshot(s) from a PVC(s) and measures the time until it's ReadyToUse.
func (r *DataProtectionTestReconciler) runSnapshotTest(ctx context.Context, dpt *oadpv1alpha1.DataProtectionTest) error {
    // Get PVC(s) and snapshot class info
    // Parse snapshot(s) readiness timeout duration
    // Create the VolumeSnapshot(s)
    // Poll for ReadyToUse status within timeout
    // Check if snapshot is ReadyToUse
    // Success: capture duration
}

```
6. Update the Status of the DPT CR: Consolidate results from the upload test, snapshot test, and vendor detection, and update the CR status.


```mermaid
flowchart TD
  A[Start Reconciliation] --> B[Fetch DPT CR<br/>If not found, exit]
  B --> C[Determine S3 Vendor via inspecting HTTP request<br/>If s3Compatible is true]
  C --> D[Initialize Cloud Provider from BSL config]
  D --> E[Check if uploadSpeedTestConfig is present]
  E -->|Yes| F[Run Upload Speed Test<br/>Upload test data, measure speed]
  F --> FF[Get Bucket Metadata like versioning status and encryption algorithm used] 
  E -->|No| G[Skip Upload Test]
  FF --> H
  G --> H
  H[Check for CSI VolumeSnapshot Test config] -->|Yes| I[Create VolumeSnapshot<br/>Poll until ReadyToUse]
  H -->|No| J[Skip Snapshot Test]
  I --> K[Calculate readyDuration]
  J --> K
  K --> L[Update DPT Status<br/>Set uploadSpeed, snapshot ready duration, vendor, bucket metadata, lastTested]
  L --> M[End Reconciliation]

```


## Implementation

- We are targeting this feature for OADP 1.5
- The implementation would be done in small phases:
    1. First phase would independent introduction of DPT CRD and controller (only for AWS provider)
    1. Followed by remaining cloud provider Azure and GCP

## Future Scope

- Recurring Tests: Support for recurring tests could be added by integrating with a scheduling system.
- Enhanced Metrics: Consider additional metrics like latency or download speed.

## Open Questions

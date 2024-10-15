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

-   Measure upload speeds for a given BackupStorageLocation (BSL) config settings and credentials
-   Measure upload speeds from the OpenShift cluster to the configured object storage.
-   Update the test results in the status of the UploadSpeedTest CRD for visibility and tracking.
-   Enable smooth integration with OADP operator-managed backup storage location configurations.


## Non-Goals

- This design will not create or modify BackupStorageLocation entries in OADP.
- It will not implement download or latency tests, focusing solely on upload speed.
- Scheduling of recurring tests is not supported in the initial version.

## High-Level Design
Components involved and their responsibilities:

- UploadSpeedTest (UST) CRD:
  - Captures the BSL configuration, test file size and test timeout used for testing directly in the CRD.
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

The UploadSpeedTest CRD structure includes fields to capture backup storage location configurations, upload test parameters, and the location of cloud provider secrets.

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: UploadSpeedTest
metadata:
  name: my-upload-speed-test
spec:
  backupLocation:
    velero:
      provider: aws # Cloud provider type (aws, azure, gcp)
      default: true
      objectStorage:
        bucket: sample-bucket
        prefix: velero
      config:
        region: us-east-1
        profile: "default"
        insecureSkipTLSVerify: "true"
      credential:
        name: cloud-credentials  # Secret containing cloud credentials
        key: cloud
  uploadSpeedTestConfig:
    fileSize: "100MB"    # File size for the test upload
    testTimeout: "60s"   # Maximum duration allowed for the test
  cloudProviderSecretRef:
    name: cloud-credentials
    namespace: openshift-adp
```

Changes to DPA CRD BSL Configuration spec:

The DPA CRD configuration includes fields to enable and configure the Upload Speed Test (UST) within the BSL configuration.

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: sample-dpa
spec:
  backupStorageLocations:
    - name: aws-backup-location
      BSLSpec: ...
      uploadSpeedTest:
        enabled: true              # Flag to enable upload speed test
        fileSize: "10MB"           # Size of the file to be uploaded
        testTimeout: "60s"         # Timeout for the upload test
```

UST controller workflow:

Reconciliation loop:
- Watches for UploadSpeedTest CRD instances.
- Fetches Cloud Provider configurations and credentials
- Performs upload speed test
- Calculates upload speed and update UST status

Upload  execution:
- Uses the appropriate cloud SDK (AWS, Azure, GCP) based on the BSL configuration.
- Initializes the cloud provider client dynamically using a CloudProvider interface, allowing extension to new providers without changes to the controller logic.
- Performs the upload and measures speed in Mbps, calculating speed using the formula:
```
Speed (Mbps) = (FileSize (bytes) * 8)/(uploadDuration(ms) * 1000)
```
- Records whether the upload succeeded or failed.

Status Update:
- Updates the UploadSpeedTest CR status with:
  - LastTested: The timestamp of the last test.
  - Status: Result of the test operation (e.g., “Complete” or “Failed”).
  - SpeedMbps: Calculated upload speed.
  - ErrorMessage: Details of any error encountered during the test.

## Implementation

#### Dynamic Provider Initialization:
  - The `initializeProvider` function in UST reconcile dynamically selects the correct cloud provider (AWS, Azure, GCP) based on the configuration in the UploadSpeedTest CR.
  - Credentials are fetched from the specified secret in the CR and used to initialize the cloud provider client.

#### CloudProvider Interface:
  - Each provider (AWS, Azure, GCP) implements the CloudProvider interface with its own logic for the UploadTest method.
  - This allows the controller to remain agnostic to provider-specific implementation details.
```go
package cloudprovider

import (
	"context"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"time"
)

type CloudProvider interface {
	UploadTest(ctx context.Context, ust *oadpv1alpha1.UploadSpeedTest, fileSize int64, testTimeout time.Duration) (int64, error)
}
```

#### Calculation and Status Update:
  -  The upload speed is calculated and recorded in the CR’s status for user visibility.
  - Error handling is centralized in the controller, which records error messages and sets the status to “Failed” in case of errors.

### UST CRD Spec

```go
// UploadSpeedTestSpec defines the desired state of UploadSpeedTest
type UploadSpeedTestSpec struct {
	// BackupLocation defines relevant configuration of the object storage/backup storage location to be tested
	BackupLocation BackupLocation `json:"backupLocation"`

	// UploadSpeedTestConfig defines the parameters for testing upload speed
	UploadSpeedTestConfig UploadSpeedTestConfig `json:"uploadSpeedTestConfig"`

	// CloudProviderSecretRef is the reference to the secret to be used for authentication with object storage
	CloudProviderSecretRef CloudProviderSecretRef `json:"cloudProviderSecretRef"`
}

type UploadSpeedTestConfig struct {
	// size of the file to be used for test
	FileSize string `json:"fileSize"`

	// timeout value for the upload test operation
	TestTimeout string `json:"testTimeout"`
}

type CloudProviderSecretRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}
```

### UST CRD Status

```go
// UploadSpeedTestStatus defines the observed state of UploadSpeedTest
type UploadSpeedTestStatus struct {
	// Timestamp of the last upload speed test
	LastTested metav1.Time `json:"lastTested"`

	// Status of the test operation - Complete, Failed
	Status string `json:"phase"`

	// Upload Speed of the operation
	SpeedMbps int64 `json:"speed"`

	// Details of any error encountered
	ErrorMessage string `json:"errorMessage"`
}
```

**Note:**
- We are targeting this feature for OADP 1.5
- The implementation would be done in small phases:
  1. First phase would independent introduction of UST CRD and controller (only for AWS provider)
  1. Then next would enabling integration with OADP/DPA
  1. Followed by remaining cloud provider Azure and GCP

## Future Scope

- Recurring Tests: Support for recurring tests could be added by integrating with a scheduling system.
- Enhanced Metrics: Consider additional metrics like latency or download speed.

## Open Questions

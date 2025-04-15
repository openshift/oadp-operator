/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DataProtectionTestSpec defines the desired tests to perform.
type DataProtectionTestSpec struct {
	// backupLocationName specifies the name the Velero BackupStorageLocation (BSL) to test against.
	// +optional
	BackupLocationName string `json:"backupLocationRef,omitempty"`

	// backupLocationSpec is an inline copy of the BSL spec to use during testing.
	// +optional
	BackupLocationSpec *velerov1.BackupStorageLocationSpec `json:"backupLocationSpec,omitempty"`

	// uploadSpeedTestConfig specifies parameters for an object storage upload speed test.
	// +optional
	UploadSpeedTestConfig *UploadSpeedTestConfig `json:"uploadSpeedTestConfig,omitempty"`

	// csiVolumeSnapshotTestConfigs defines one or more CSI VolumeSnapshot tests to perform.
	// +optional
	CSIVolumeSnapshotTestConfigs []CSIVolumeSnapshotTestConfig `json:"csiVolumeSnapshotTestConfigs,omitempty"`
}

// UploadSpeedTestConfig contains configuration for testing object storage upload performance.
type UploadSpeedTestConfig struct {
	// fileSize is the size of data to upload, e.g., "100MB".
	// +optional
	FileSize string `json:"fileSize,omitempty"`

	// testTimeout defines the maximum duration for the upload test, e.g., "60s".
	// +optional
	TestTimeout string `json:"testTimeout,omitempty"`
}

// CSIVolumeSnapshotTestConfig contains config for performing a CSI VolumeSnapshot test.
type CSIVolumeSnapshotTestConfig struct {
	// snapshotClassName specifies the CSI snapshot class to use.
	// +optional
	SnapshotClassName string `json:"snapshotClassName,omitempty"`

	// timeout specifies how long to wait for the snapshot to become ready.
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// volumeSnapshotSource defines the PVC to snapshot.
	// +optional
	VolumeSnapshotSource VolumeSnapshotSource `json:"volumeSnapshotSource,omitempty"`
}

// VolumeSnapshotSource points to the PVC that should be snapshotted.
type VolumeSnapshotSource struct {
	// persistentVolumeClaimName is the name of the PVC to snapshot.
	// +optional
	PersistentVolumeClaimName string `json:"persistentVolumeClaimName,omitempty"`

	// persistentVolumeClaimNamespace is the namespace of the PVC.
	// +optional
	PersistentVolumeClaimNamespace string `json:"persistentVolumeClaimNamespace,omitempty"`
}

// DataProtectionTestStatus represents the observed results of the tests.
type DataProtectionTestStatus struct {
	// lastTested is the timestamp when the test was last run.
	// +optional
	LastTested metav1.Time `json:"lastTested,omitempty"`

	// s3Vendor indicates the detected s3 vendor name from the storage endpoint if applicable (e.g., AWS, MinIO).
	// +optional
	S3Vendor string `json:"s3Vendor,omitempty"`

	// bucketMetadata reports the encryption and versioning status of the target bucket.
	// +optional
	BucketMetadata *BucketMetadata `json:"bucketMetadata,omitempty"`

	// uploadTest contains results of the object storage upload test.
	// +optional
	UploadTest UploadTestStatus `json:"uploadTest,omitempty"`

	// snapshotTests contains results for each snapshot tested PVC.
	// +optional
	SnapshotTests []SnapshotTestStatus `json:"snapshotTests,omitempty"`
}

// UploadTestStatus holds the results of the upload test.
type UploadTestStatus struct {
	// speedMbps is the calculated upload speed.
	// +optional
	SpeedMbps int64 `json:"speedMbps,omitempty"`

	// duration is the time taken to upload the test file.
	// +optional
	Duration string `json:"duration,omitempty"`

	// success indicates if the upload succeeded.
	// +optional
	Success bool `json:"success,omitempty"`

	// errorMessage contains details of any upload failure.
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// SnapshotTestStatus holds the result for an individual PVC snapshot test.
type SnapshotTestStatus struct {
	// persistentVolumeClaimName of the tested PVC.
	// +optional
	PersistentVolumeClaimName string `json:"persistentVolumeClaimName,omitempty"`

	// persistentVolumeClaimNamespace of the tested PVC.
	// +optional
	PersistentVolumeClaimNamespace string `json:"persistentVolumeClaimNamespace,omitempty"`

	// status indicates snapshot readiness ("Ready", "Failed").
	// +optional
	Status string `json:"status,omitempty"`

	// readyDuration is the time it took for the snapshot to become ReadyToUse.
	// +optional
	ReadyDuration string `json:"readyDuration,omitempty"`

	// errorMessage contains details of any snapshot failure.
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// BucketMetadata contains encryption and versioning info for the target bucket.
type BucketMetadata struct {
	// encryptionAlgorithm reports the encryption method (AES256, aws:kms, or "None").
	// +optional
	EncryptionAlgorithm string `json:"encryptionAlgorithm,omitempty"`

	// versioningStatus indicates whether bucket versioning is Enabled, Suspended, or None.
	// +optional
	VersioningStatus string `json:"versioningStatus,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=dataprotectiontests,shortName=dpt

// DataProtectionTest is the Schema for the dataprotectiontests API
type DataProtectionTest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataProtectionTestSpec   `json:"spec,omitempty"`
	Status DataProtectionTestStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DataProtectionTestList contains a list of DataProtectionTest
type DataProtectionTestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataProtectionTest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataProtectionTest{}, &DataProtectionTestList{})
}

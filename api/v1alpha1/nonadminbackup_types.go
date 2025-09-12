/*
Copyright 2024.

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

	"github.com/openshift/oadp-operator/internal/common/constant"
)

// NonAdminBackupSpec defines the desired state of NonAdminBackup
type NonAdminBackupSpec struct {
	// BackupSpec defines the specification for a Velero backup.
	BackupSpec *velerov1.BackupSpec `json:"backupSpec"`

	// DeleteBackup removes the NonAdminBackup and its associated NonAdminRestores and VeleroBackup from the cluster,
	// as well as the corresponding data in object storage
	// +optional
	DeleteBackup bool `json:"deleteBackup,omitempty"`
}

// VeleroBackup contains information of the related Velero backup object.
type VeleroBackup struct {
	// spec captures the current spec of the Velero backup.
	// +optional
	Spec *velerov1.BackupSpec `json:"spec,omitempty"`

	// status captures the current status of the Velero backup.
	// +optional
	Status *velerov1.BackupStatus `json:"status,omitempty"`

	// nacuuid references the Velero Backup object by it's label containing same NACUUID.
	// +optional
	NACUUID string `json:"nacuuid,omitempty"`

	// references the Velero Backup object by it's name.
	// +optional
	Name string `json:"name,omitempty"`

	// namespace references the Namespace in which Velero backup exists.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// VeleroDeleteBackupRequest contains information of the related Velero delete backup request object.
type VeleroDeleteBackupRequest struct {
	// status captures the current status of the Velero delete backup request.
	// +optional
	Status *velerov1.DeleteBackupRequestStatus `json:"status,omitempty"`

	// nacuuid references the Velero delete backup request object by it's label containing same NACUUID.
	// +optional
	NACUUID string `json:"nacuuid,omitempty"`

	// name references the Velero delete backup request object by it's name.
	// +optional
	Name string `json:"name,omitempty"`

	// namespace references the Namespace in which Velero delete backup request exists.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// DataMoverDataUploads contains information of the related Velero DataUpload objects.
type DataMoverDataUploads struct {
	// number of DataUploads related to this NonAdminBackup's Backup
	// +optional
	Total int `json:"total,omitempty"`

	// number of DataUploads related to this NonAdminBackup's Backup in phase New
	// +optional
	New int `json:"new,omitempty"`

	// number of DataUploads related to this NonAdminBackup's Backup in phase Accepted
	// +optional
	Accepted int `json:"accepted,omitempty"`

	// number of DataUploads related to this NonAdminBackup's Backup in phase Prepared
	// +optional
	Prepared int `json:"prepared,omitempty"`

	// number of DataUploads related to this NonAdminBackup's Backup in phase InProgress
	// +optional
	InProgress int `json:"inProgress,omitempty"`

	// number of DataUploads related to this NonAdminBackup's Backup in phase Canceling
	// +optional
	Canceling int `json:"canceling,omitempty"`

	// number of DataUploads related to this NonAdminBackup's Backup in phase Canceled
	// +optional
	Canceled int `json:"canceled,omitempty"`

	// number of DataUploads related to this NonAdminBackup's Backup in phase Failed
	// +optional
	Failed int `json:"failed,omitempty"`

	// number of DataUploads related to this NonAdminBackup's Backup in phase Completed
	// +optional
	Completed int `json:"completed,omitempty"`
}

// FileSystemPodVolumeBackups contains information of the related Velero PodVolumeBackup objects.
type FileSystemPodVolumeBackups struct {
	// number of PodVolumeBackups related to this NonAdminBackup's Backup
	// +optional
	Total int `json:"total,omitempty"`

	// number of PodVolumeBackups related to this NonAdminBackup's Backup in phase New
	// +optional
	New int `json:"new,omitempty"`

	// number of PodVolumeBackups related to this NonAdminBackup's Backup in phase InProgress
	// +optional
	InProgress int `json:"inProgress,omitempty"`

	// number of PodVolumeBackups related to this NonAdminBackup's Backup in phase Failed
	// +optional
	Failed int `json:"failed,omitempty"`

	// number of PodVolumeBackups related to this NonAdminBackup's Backup in phase Completed
	// +optional
	Completed int `json:"completed,omitempty"`
}

// NonAdminBackupStatus defines the observed state of NonAdminBackup
type NonAdminBackupStatus struct {
	// +optional
	VeleroBackup *VeleroBackup `json:"veleroBackup,omitempty"`

	// +optional
	VeleroDeleteBackupRequest *VeleroDeleteBackupRequest `json:"veleroDeleteBackupRequest,omitempty"`

	// +optional
	DataMoverDataUploads *DataMoverDataUploads `json:"dataMoverDataUploads,omitempty"`

	// +optional
	FileSystemPodVolumeBackups *FileSystemPodVolumeBackups `json:"fileSystemPodVolumeBackups,omitempty"`

	// queueInfo is used to estimate how many backups are scheduled before the given VeleroBackup in the OADP namespace.
	// This number is not guaranteed to be accurate, but it should be close. It's inaccurate for cases when
	// Velero pod is not running or being restarted after Backup object were created.
	// It counts only VeleroBackups that are still subject to be handled by OADP/Velero.
	// +optional
	QueueInfo *QueueInfo `json:"queueInfo,omitempty"`

	// phase is a simple one high-level summary of the lifecycle of an NonAdminBackup.
	Phase NonAdminPhase `json:"phase,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nonadminbackups,shortName=nab
// +kubebuilder:printcolumn:name="Request-Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Velero-Phase",type="string",JSONPath=".status.veleroBackup.status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// NonAdminBackup is the Schema for the nonadminbackups API
type NonAdminBackup struct {
	Spec   NonAdminBackupSpec   `json:"spec,omitempty"`
	Status NonAdminBackupStatus `json:"status,omitempty"`

	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

// +kubebuilder:object:root=true

// NonAdminBackupList contains a list of NonAdminBackup
type NonAdminBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NonAdminBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NonAdminBackup{}, &NonAdminBackupList{})
}

// Helper Functions to avoid digging into NAB controller to understand how to get desired values

// VeleroBackupName returns the name of the VeleroBackup object.
func (nab *NonAdminBackup) VeleroBackupName() string {
	if nab.Status.VeleroBackup == nil {
		return constant.EmptyString
	}
	return nab.Status.VeleroBackup.Name
}

// UsesNaBSL returns true if backup is using NonAdminBackupStorageLocation
func (nab *NonAdminBackup) UsesNaBSL() bool {
	return nab.Spec.BackupSpec != nil && nab.Spec.BackupSpec.StorageLocation != constant.EmptyString
}

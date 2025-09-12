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

// NonAdminRestoreSpec defines the desired state of NonAdminRestore
type NonAdminRestoreSpec struct {
	// restoreSpec defines the specification for a Velero restore.
	RestoreSpec *velerov1.RestoreSpec `json:"restoreSpec"`
}

// VeleroRestore contains information of the related Velero restore object.
type VeleroRestore struct {
	// status captures the current status of the Velero restore.
	// +optional
	Status *velerov1.RestoreStatus `json:"status,omitempty"`

	// references the Velero Restore object by it's name.
	// +optional
	Name string `json:"name,omitempty"`

	// nacuuid references the Velero Restore object by it's label containing same NACUUID.
	// +optional
	NACUUID string `json:"nacuuid,omitempty"`

	// namespace references the Namespace in which Velero Restore exists.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// DataMoverDataDownloads contains information of the related Velero DataDownload objects.
type DataMoverDataDownloads struct {
	// number of DataDownloads related to this NonAdminRestore's Restore
	// +optional
	Total int `json:"total,omitempty"`

	// number of DataDownloads related to this NonAdminRestore's Restore in phase New
	// +optional
	New int `json:"new,omitempty"`

	// number of DataDownloads related to this NonAdminRestore's Restore in phase Accepted
	// +optional
	Accepted int `json:"accepted,omitempty"`

	// number of DataDownloads related to this NonAdminRestore's Restore in phase Prepared
	// +optional
	Prepared int `json:"prepared,omitempty"`

	// number of DataDownloads related to this NonAdminRestore's Restore in phase InProgress
	// +optional
	InProgress int `json:"inProgress,omitempty"`

	// number of DataDownloads related to this NonAdminRestore's Restore in phase Canceling
	// +optional
	Canceling int `json:"canceling,omitempty"`

	// number of DataDownloads related to this NonAdminRestore's Restore in phase Canceled
	// +optional
	Canceled int `json:"canceled,omitempty"`

	// number of DataDownloads related to this NonAdminRestore's Restore in phase Failed
	// +optional
	Failed int `json:"failed,omitempty"`

	// number of DataDownloads related to this NonAdminRestore's Restore in phase Completed
	// +optional
	Completed int `json:"completed,omitempty"`
}

// FileSystemPodVolumeRestores contains information of the related Velero PodVolumeRestore objects.
type FileSystemPodVolumeRestores struct {
	// number of PodVolumeRestores related to this NonAdminRestore's Restore
	// +optional
	Total int `json:"total,omitempty"`

	// number of PodVolumeRestores related to this NonAdminRestore's Restore in phase New
	// +optional
	New int `json:"new,omitempty"`

	// number of PodVolumeRestores related to this NonAdminRestore's Restore in phase InProgress
	// +optional
	InProgress int `json:"inProgress,omitempty"`

	// number of PodVolumeRestores related to this NonAdminRestore's Restore in phase Failed
	// +optional
	Failed int `json:"failed,omitempty"`

	// number of PodVolumeRestores related to this NonAdminRestore's Restore in phase Completed
	// +optional
	Completed int `json:"completed,omitempty"`
}

// NonAdminRestoreStatus defines the observed state of NonAdminRestore
type NonAdminRestoreStatus struct {
	// +optional
	VeleroRestore *VeleroRestore `json:"veleroRestore,omitempty"`

	// +optional
	DataMoverDataDownloads *DataMoverDataDownloads `json:"dataMoverDataDownloads,omitempty"`

	// +optional
	FileSystemPodVolumeRestores *FileSystemPodVolumeRestores `json:"fileSystemPodVolumeRestores,omitempty"`

	// queueInfo is used to estimate how many restores are scheduled before the given VeleroRestore in the OADP namespace.
	// This number is not guaranteed to be accurate, but it should be close. It's inaccurate for cases when
	// Velero pod is not running or being restarted after Restore object were created.
	// It counts only VeleroRestores that are still subject to be handled by OADP/Velero.
	// +optional
	QueueInfo *QueueInfo `json:"queueInfo,omitempty"`

	// phase is a simple one high-level summary of the lifecycle of an NonAdminRestore.
	Phase NonAdminPhase `json:"phase,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nonadminrestores,shortName=nar
// +kubebuilder:printcolumn:name="Request-Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Velero-Phase",type="string",JSONPath=".status.veleroRestore.status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// NonAdminRestore is the Schema for the nonadminrestores API
type NonAdminRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NonAdminRestoreSpec   `json:"spec,omitempty"`
	Status NonAdminRestoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NonAdminRestoreList contains a list of NonAdminRestore
type NonAdminRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NonAdminRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NonAdminRestore{}, &NonAdminRestoreList{})
}

// Helper Functions to avoid digging into NAR controller to understand how to get desired values

// VeleroRestoreName returns the name of the VeleroRestore object.
func (nar *NonAdminRestore) VeleroRestoreName() string {
	if nar.Status.VeleroRestore == nil {
		return constant.EmptyString
	}
	return nar.Status.VeleroRestore.Name
}

// NonAdminBackupName returns NonAdminBackup name of this NAR
func (nar *NonAdminRestore) NonAdminBackupName() string {
	return nar.Spec.RestoreSpec.BackupName
}

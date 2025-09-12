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
)

// NonAdminBSLCondition contains additional conditions to the
// generic ones defined as NonAdminCondition
type NonAdminBSLCondition string

// Predefined NonAdminBSLConditions
const (
	NonAdminBSLConditionSecretSynced       NonAdminBSLCondition = "SecretSynced"
	NonAdminBSLConditionBSLSynced          NonAdminBSLCondition = "BackupStorageLocationSynced"
	NonAdminBSLConditionApproved           NonAdminBSLCondition = "ClusterAdminApproved"
	NonAdminBSLConditionSpecUpdateApproved NonAdminBSLCondition = "SpecUpdateApproved"
)

// NonAdminBackupStorageLocationSpec defines the desired state of NonAdminBackupStorageLocation
type NonAdminBackupStorageLocationSpec struct {
	BackupStorageLocationSpec *velerov1.BackupStorageLocationSpec `json:"backupStorageLocationSpec"`
}

// VeleroBackupStorageLocation contains information of the related Velero backup object.
type VeleroBackupStorageLocation struct {
	// status captures the current status of the Velero backup storage location.
	// +optional
	Status *velerov1.BackupStorageLocationStatus `json:"status,omitempty"`

	// nacuuid references the Velero BackupStorageLocation object by it's label containing same NACUUID.
	// +optional
	NACUUID string `json:"nacuuid,omitempty"`

	// references the Velero BackupStorageLocation object by it's name.
	// +optional
	Name string `json:"name,omitempty"`

	// namespace references the Namespace in which Velero backup storage location exists.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// NonAdminBackupStorageLocationStatus defines the observed state of NonAdminBackupStorageLocation
type NonAdminBackupStorageLocationStatus struct {
	// +optional
	VeleroBackupStorageLocation *VeleroBackupStorageLocation `json:"veleroBackupStorageLocation,omitempty"`

	// phase is a simple one high-level summary of the lifecycle of an NonAdminBackupStorageLocation.
	Phase NonAdminPhase `json:"phase,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nonadminbackupstoragelocations,shortName=nabsl
// +kubebuilder:printcolumn:name="Request-Approved",type="string",JSONPath=".status.conditions[?(@.type=='ClusterAdminApproved')].status"
// +kubebuilder:printcolumn:name="Request-Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Velero-Phase",type="string",JSONPath=".status.veleroBackupStorageLocation.status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// NonAdminBackupStorageLocation is the Schema for the nonadminbackupstoragelocations API
type NonAdminBackupStorageLocation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NonAdminBackupStorageLocationSpec   `json:"spec,omitempty"`
	Status NonAdminBackupStorageLocationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NonAdminBackupStorageLocationList contains a list of NonAdminBackupStorageLocation
type NonAdminBackupStorageLocationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NonAdminBackupStorageLocation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NonAdminBackupStorageLocation{}, &NonAdminBackupStorageLocationList{})
}

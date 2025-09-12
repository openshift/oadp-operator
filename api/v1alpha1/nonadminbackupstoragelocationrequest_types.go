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

// NonAdminBSLRequest controls the approval of the NonAdminBackupStorageLocation
// +kubebuilder:validation:Enum=approve;reject;pending
type NonAdminBSLRequest string

// NonAdminBSLRequestPhase is the phase of the NonAdminBackupStorageLocationRequest
// +kubebuilder:validation:Enum=Pending;Approved;Rejected
type NonAdminBSLRequestPhase string

// Predefined NonAdminBSLRequestConditions
const (
	NonAdminBSLRequestApproved NonAdminBSLRequest = "approve"
	NonAdminBSLRequestRejected NonAdminBSLRequest = "reject"
	NonAdminBSLRequestPending  NonAdminBSLRequest = "pending"
)

// Predefined NonAdminBSLRequestPhases
const (
	NonAdminBSLRequestPhasePending  NonAdminBSLRequestPhase = "Pending"
	NonAdminBSLRequestPhaseApproved NonAdminBSLRequestPhase = "Approved"
	NonAdminBSLRequestPhaseRejected NonAdminBSLRequestPhase = "Rejected"
)

// NonAdminBackupStorageLocationRequestSpec defines the desired state of NonAdminBackupStorageLocationRequest
type NonAdminBackupStorageLocationRequestSpec struct {
	// approvalDecision is the decision of the cluster admin on the Requested NonAdminBackupStorageLocation creation.
	// The value may be set to either approve or reject.
	// +optional
	ApprovalDecision NonAdminBSLRequest `json:"approvalDecision,omitempty"`
}

// SourceNonAdminBSL contains information of the NonAdminBackupStorageLocation object that triggered NonAdminBSLRequest
type SourceNonAdminBSL struct {
	// requestedSpec contains the requested Velero BackupStorageLocation spec from the NonAdminBackupStorageLocation
	// +optionl
	RequestedSpec *velerov1.BackupStorageLocationSpec `json:"requestedSpec"`

	// nacuuid references the NonAdminBackupStorageLocation object by it's label containing same NACUUID.
	// +optional
	NACUUID string `json:"nacuuid,omitempty"`

	// name references the NonAdminBackupStorageLocation object by it's name.
	// +optional
	Name string `json:"name,omitempty"`

	// namespace references the Namespace in which NonAdminBackupStorageLocation exists.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// NonAdminBackupStorageLocationRequestStatus defines the observed state of NonAdminBackupStorageLocationRequest
type NonAdminBackupStorageLocationRequestStatus struct {
	// nonAdminBackupStorageLocation contains information of the NonAdminBackupStorageLocation object that triggered NonAdminBSLRequest
	// +optional
	SourceNonAdminBSL *SourceNonAdminBSL `json:"nonAdminBackupStorageLocation,omitempty"`

	// phase represents the current state of the NonAdminBSLRequest. It can be either Pending, Approved or Rejected.
	// +optional
	Phase NonAdminBSLRequestPhase `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nonadminbackupstoragelocationrequests,shortName=nabslrequest
// +kubebuilder:printcolumn:name="Request-Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Request-Namespace",type="string",JSONPath=".status.nonAdminBackupStorageLocation.namespace"
// +kubebuilder:printcolumn:name="Request-Name",type="string",JSONPath=".status.nonAdminBackupStorageLocation.name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// NonAdminBackupStorageLocationRequest is the Schema for the nonadminbackupstoragelocationrequests API
type NonAdminBackupStorageLocationRequest struct {
	Status            NonAdminBackupStorageLocationRequestStatus `json:"status,omitempty"`
	Spec              NonAdminBackupStorageLocationRequestSpec   `json:"spec,omitempty"`
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

// +kubebuilder:object:root=true

// NonAdminBackupStorageLocationRequestList contains a list of NonAdminBackupStorageLocationRequest
type NonAdminBackupStorageLocationRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NonAdminBackupStorageLocationRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NonAdminBackupStorageLocationRequest{}, &NonAdminBackupStorageLocationRequestList{})
}

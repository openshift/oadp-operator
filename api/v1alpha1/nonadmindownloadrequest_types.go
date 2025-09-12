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
	"fmt"

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NonAdminDownloadRequestSpec defines the desired state of NonAdminDownloadRequest.
// Mirrors velero DownloadRequestSpec to allow non admins to download information for a non admin backup/restore
type NonAdminDownloadRequestSpec struct {
	// Target is what to download (e.g. logs for a backup).
	Target velerov1.DownloadTarget `json:"target"`
}

// VeleroDownloadRequest represents VeleroDownloadRequest
type VeleroDownloadRequest struct {
	// VeleroDownloadRequestStatus represents VeleroDownloadRequestStatus
	// +optional
	Status *velerov1.DownloadRequestStatus `json:"status,omitempty"`
}

// NonAdminDownloadRequestStatus defines the observed state of NonAdminDownloadRequest.
type NonAdminDownloadRequestStatus struct {
	// +optional
	VeleroDownloadRequest VeleroDownloadRequest `json:"velero,omitempty"`
	// phase is a simple one high-level summary of the lifecycle of an NonAdminDownloadRequest
	Phase NonAdminPhase `json:"phase,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nonadmindownloadrequests,shortName=nadr
// +kubebuilder:printcolumn:name="Request-Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// NonAdminDownloadRequest is the Schema for the nonadmindownloadrequests API.
type NonAdminDownloadRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NonAdminDownloadRequestSpec   `json:"spec,omitempty"`
	Status NonAdminDownloadRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NonAdminDownloadRequestList contains a list of NonAdminDownloadRequest.
type NonAdminDownloadRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NonAdminDownloadRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NonAdminDownloadRequest{}, &NonAdminDownloadRequestList{})
}

// NonAdminDownloadRequestConditionType prevents untyped strings for NADR conditions functions
type NonAdminDownloadRequestConditionType string

const (
	// ConditionNonAdminBackupStorageLocationNotUsed block download requests processing if NaBSL is not used
	ConditionNonAdminBackupStorageLocationNotUsed NonAdminDownloadRequestConditionType = "NonAdminBackupStorageLocationNotUsed"
	// ConditionNonAdminBackupNotAvailable indicates backup is not available, and will backoff download request
	ConditionNonAdminBackupNotAvailable NonAdminDownloadRequestConditionType = "NonAdminBackupNotAvailable"
	// ConditionNonAdminRestoreNotAvailable indicates restore is not available, and will backoff download request
	ConditionNonAdminRestoreNotAvailable NonAdminDownloadRequestConditionType = "NonAdminRestoreNotAvailable"
	// ConditionNonAdminProcessed indicates that the NADR is in a terminal state
	ConditionNonAdminProcessed NonAdminDownloadRequestConditionType = "Processed"
)

// ReadyForProcessing returns if this NonAdminDownloadRequests is in a state ready for processing
//
// Terminal conditions include
// - NonAdminBackupStorageLocationNotUsed: we currently require NaBSL usage on the NAB/NAR to process this download request
// returns true if ready for processing, false otherwise
func (nadr *NonAdminDownloadRequest) ReadyForProcessing() bool {
	// if nadr has ConditionNonAdminBackupStorageLocationUsed return false
	if nadr.Status.Conditions != nil {
		for _, condition := range nadr.Status.Conditions {
			if condition.Type == string(ConditionNonAdminBackupStorageLocationNotUsed) &&
				condition.Status == metav1.ConditionTrue {
				return false
			}
		}
	}
	return true // required fields are set via velero validation markers
}

// VeleroDownloadRequestName defines velero download request name for this NonAdminDownloadRequest
func (nadr *NonAdminDownloadRequest) VeleroDownloadRequestName() string {
	return fmt.Sprintf("nadr-%s", string(nadr.GetUID()))
}

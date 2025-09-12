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

// NonAdminPhase is a simple one high-level summary of the lifecycle of a NonAdminBackup, NonAdminRestore, NonAdminBackupStorageLocation, or NonAdminDownloadRequest
// +kubebuilder:validation:Enum=New;BackingOff;Created;Deleting
type NonAdminPhase string

const (
	// NonAdminPhaseNew - NonAdmin object was accepted by the OpenShift cluster, but it has not yet been processed by the NonAdminController
	NonAdminPhaseNew NonAdminPhase = "New"
	// NonAdminPhaseBackingOff - Velero object was not created due to NonAdmin object error (configuration or similar)
	NonAdminPhaseBackingOff NonAdminPhase = "BackingOff"
	// NonAdminPhaseCreated - Velero object was created. The Phase will not have additional information about it.
	NonAdminPhaseCreated NonAdminPhase = "Created"
	// NonAdminPhaseDeleting - Velero object is pending deletion. The Phase will not have additional information about it.
	NonAdminPhaseDeleting NonAdminPhase = "Deleting"
)

// NonAdminCondition are used for more detailed information supporing NonAdminBackupPhase state.
// +kubebuilder:validation:Enum=Accepted;Queued;Deleting
type NonAdminCondition string

// Predefined conditions for NonAdminController objects.
// One NonAdminController object may have multiple conditions.
// It is more granular knowledge of the NonAdminController object and represents the
// array of the conditions through which the NonAdminController has or has not passed
const (
	NonAdminConditionAccepted NonAdminCondition = "Accepted"
	NonAdminConditionQueued   NonAdminCondition = "Queued"
	NonAdminConditionDeleting NonAdminCondition = "Deleting"
)

// QueueInfo holds the queue position for a specific operation.
type QueueInfo struct {
	// estimatedQueuePosition is the number of operations ahead in the queue (0 if not queued)
	EstimatedQueuePosition int `json:"estimatedQueuePosition"`
}

// Constants representing resource names for non-admin objects
// These are used to identify custom resources managed for non-admin users.
const (
	// NonAdminBackups represents the resource name for non-admin backups.
	NonAdminBackups = "nonadminbackups"

	// NonAdminRestores represents the resource name for non-admin restores.
	NonAdminRestores = "nonadminrestores"

	// NonAdminBackupStorageLocations represents the resource name for non-admin backup storage locations.
	NonAdminBackupStorageLocations = "nonadminbackupstoragelocations"
)

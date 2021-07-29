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
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Conditions
const ConditionReconciled = "Reconciled"
const ReconciledReasonComplete = "Complete"
const ReconciledReasonError = "Error"
const ReconcileCompleteMessage = "Reconcile complete"

const OadpOperatorLabel = "openshift.io/oadp"

type DefaultPlugin string

const DefaultPluginAWS DefaultPlugin = "aws"
const DefaultPluginGCP DefaultPlugin = "gcp"
const DefaultPluginMicrosoftAzure DefaultPlugin = "azure"
const DefaultPluginCSI DefaultPlugin = "csi"
const DefaultPluginOpenShift DefaultPlugin = "openshift"

type CustomPlugin struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

// VeleroSpec defines the desired state of Velero
type VeleroSpec struct {
	// Important: Run "make" to regenerate code after modifying this file
	// Determine whether this was installed via OLM
	OlmManaged *bool `json:"olmManaged,omitempty"`

	// Velero configuration
	BackupStorageLocations  []velero.BackupStorageLocationSpec  `json:"backupStorageLocations"`
	VolumeSnapshotLocations []velero.VolumeSnapshotLocationSpec `json:"volumeSnapshotLocations"`
	VeleroFeatureFlags      []string                            `json:"veleroFeatureFlags,omitempty"`
	// We do not currently support setting tolerations for Velero
	VeleroTolerations         []corev1.Toleration         `json:"veleroTolerations,omitempty"`
	VeleroResourceAllocations corev1.ResourceRequirements `json:"veleroResourceAllocations,omitempty"`

	// Plugin configuration
	DefaultVeleroPlugins []DefaultPlugin `json:"defaultVeleroPlugins,omitempty"`
	// +optional
	CustomVeleroPlugins []CustomPlugin `json:"customVeleroPlugins,omitempty"`

	// Noobaa is a boolean to specify if we should install backup storage from OCS operator with Noobaa
	// +optional
	Noobaa bool `json:"noobaa,omitempty"`

	// Restic options
	EnableRestic              *bool                       `json:"enableRestic,omitempty"`
	ResticSupplementalGroups  []string                    `json:"resticSupplementalGroups,omitempty"`
	ResticNodeSelector        map[string]string           `json:"resticNodeSelector,omitempty"`
	ResticTolerations         []corev1.Toleration         `json:"resticTolerations,omitempty"`
	ResticResourceAllocations corev1.ResourceRequirements `json:"resticResourceAllocations,omitempty"`
}

// VeleroStatus defines the observed state of Velero
type VeleroStatus struct {
	Conditions []metav1.Condition
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Velero is the Schema for the veleroes API
type Velero struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VeleroSpec   `json:"spec,omitempty"`
	Status VeleroStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VeleroList contains a list of Velero
type VeleroList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Velero `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Velero{}, &VeleroList{})
}

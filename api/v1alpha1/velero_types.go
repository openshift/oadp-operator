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

type UnsupportedImageKey string

const VeleroImageKey UnsupportedImageKey = "veleroPluginImageFqin"
const AWSPluginImageKey UnsupportedImageKey = "awsPluginImageFqin"
const OpenShiftPluginImageKey UnsupportedImageKey = "openshiftPluginImageFqin"
const AzurePluginImageKey UnsupportedImageKey = "azurePluginImageFqin"
const GCPPluginImageKey UnsupportedImageKey = "gcpPluginImageFqin"
const CSIPluginImageKey UnsupportedImageKey = "csiPluginImageFqin"
const ResticRestoreImageKey UnsupportedImageKey = "resticRestoreImageFqin"
const RegistryImageKey UnsupportedImageKey = "registryImageFqin"

// VeleroSpec defines the desired state of Velero
type VeleroSpec struct {
	// BackupStorageLocations defines the list of desired configuration to use for BackupStorageLocations
	// +optional
	BackupStorageLocations []velero.BackupStorageLocationSpec `json:"backupStorageLocations"`
	// VolumeSnapshotLocations defines the list of desired configuration to use for VolumeSnapshotLocations
	// +optional
	VolumeSnapshotLocations []velero.VolumeSnapshotLocationSpec `json:"volumeSnapshotLocations"`
	// VeleroFeatureFlags defines the list of features to enable for Velero instance
	// +optional
	VeleroFeatureFlags []string `json:"veleroFeatureFlags,omitempty"`
	// VeleroTolerations defines the list of tolerations to be applied to Velero deployment
	// +optional
	VeleroTolerations []corev1.Toleration `json:"veleroTolerations,omitempty"`
	// VeleroResourceAllocations defines the CPU and Memory resource allocations for the Velero Pod
	// +optional
	VeleroResourceAllocations corev1.ResourceRequirements `json:"veleroResourceAllocations,omitempty"`
	// DefaultVeleroPlugins defines the list of default plugins (aws, gcp, azure, openshift, csi) to be installed with Velero
	DefaultVeleroPlugins []DefaultPlugin `json:"defaultVeleroPlugins,omitempty"`
	// CustomVeleroPlugins defines the custom plugin to be installed with Velero
	// +optional
	CustomVeleroPlugins []CustomPlugin `json:"customVeleroPlugins,omitempty"`

	// EnableRestic is a boolean to specify if restic daemonset instance should be created or not
	// +optional
	EnableRestic *bool `json:"enableRestic,omitempty"`
	// ResticSupplementalGroups defines the linux groups to be applied to the Restic Pod
	// +optional
	ResticSupplementalGroups []int64 `json:"resticSupplementalGroups,omitempty"`
	// ResticNodeSelector defines the nodeSelector to be supplied to Restic podSpec
	// +optional
	ResticNodeSelector map[string]string `json:"resticNodeSelector,omitempty"`
	// ResticTolerations defines the list of tolerations to be applied to Restic daemonset
	// +optional
	ResticTolerations []corev1.Toleration `json:"resticTolerations,omitempty"`
	// ResticResourceAllocations defines the CPU and Memory resource allocations for the restic Pod
	// +optional
	ResticResourceAllocations corev1.ResourceRequirements `json:"resticResourceAllocations,omitempty"`
	// ResticTimeout defines the Restic timeout, default value is 1h
	// +optional
	ResticTimeout string `json:"resticTimeout,omitempty"`
	// +optional
	UnsupportedOverrides map[UnsupportedImageKey]string `json:"unsupportedOverrides,omitempty"`
	// add annotations to pods deployed by operator
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`
	// DNSPolicy defines how a pod's DNS will be configured.
	// https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-s-dns-policy
	// +optional
	PodDnsPolicy corev1.DNSPolicy `json:"podDnsPolicy,omitempty"`
	// PodDNSConfig defines the DNS parameters of a pod in addition to
	// those generated from DNSPolicy.
	// https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-dns-config
	// +optional
	PodDnsConfig corev1.PodDNSConfig `json:"podDnsConfig,omitempty"`
	// RestoreResourceVersionPriority represents a configmap that will be created if defined for use in conjunction with `EnableAPIGroupVersions` feature flag
	// Defining this field automatically add EnableAPIGroupVersions to the velero server feature flag
	// +optional
	RestoreResourcesVersionPriority string `json:"restoreResourcesVersionPriority,omitempty"`
	// BackupImages is used to specify whether you want to deploy a registry for enabling backup and restore of images
	// +optional
	BackupImages *bool `json:"backupImages,omitempty"`
	// If you need to install Velero without a default backup storage location NoDefaultBackupLocation flag is required for confirmation
	// +optional
	NoDefaultBackupLocation bool `json:"noDefaultBackupLocation,omitempty"`
}

// VeleroStatus defines the observed state of Velero
type VeleroStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
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

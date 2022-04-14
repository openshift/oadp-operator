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
const RegistryDeploymentLabel = "openshift.io/oadp-registry"

type DefaultPlugin string

const DefaultPluginAWS DefaultPlugin = "aws"
const DefaultPluginGCP DefaultPlugin = "gcp"
const DefaultPluginMicrosoftAzure DefaultPlugin = "azure"
const DefaultPluginCSI DefaultPlugin = "csi"
const DefaultPluginOpenShift DefaultPlugin = "openshift"
const DefaultPluginKubeVirt DefaultPlugin = "kubevirt"

type CustomPlugin struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

type UnsupportedImageKey string

const VeleroImageKey UnsupportedImageKey = "veleroImageFqin"
const AWSPluginImageKey UnsupportedImageKey = "awsPluginImageFqin"
const OpenShiftPluginImageKey UnsupportedImageKey = "openshiftPluginImageFqin"
const AzurePluginImageKey UnsupportedImageKey = "azurePluginImageFqin"
const GCPPluginImageKey UnsupportedImageKey = "gcpPluginImageFqin"
const CSIPluginImageKey UnsupportedImageKey = "csiPluginImageFqin"
const ResticRestoreImageKey UnsupportedImageKey = "resticRestoreImageFqin"
const RegistryImageKey UnsupportedImageKey = "registryImageFqin"
const KubeVirtPluginImageKey UnsupportedImageKey = "kubevirtPluginImageFqin"

type VeleroConfig struct {
	// FeatureFlags defines the list of features to enable for Velero instance
	// +optional
	FeatureFlags   []string        `json:"featureFlags,omitempty"`
	DefaultPlugins []DefaultPlugin `json:"defaultPlugins,omitempty"`
	// CustomVeleroPlugins defines the custom plugin to be installed with Velero
	// +optional
	CustomPlugins []CustomPlugin `json:"customPlugins,omitempty"`
	// RestoreResourceVersionPriority represents a configmap that will be created if defined for use in conjunction with `EnableAPIGroupVersions` feature flag
	// Defining this field automatically add EnableAPIGroupVersions to the velero server feature flag
	// +optional
	RestoreResourcesVersionPriority string `json:"restoreResourcesVersionPriority,omitempty"`
	// If you need to install Velero without a default backup storage location NoDefaultBackupLocation flag is required for confirmation
	// +optional
	NoDefaultBackupLocation bool `json:"noDefaultBackupLocation,omitempty"`
	// Pod specific configuration
	PodConfig *PodConfig `json:"podConfig,omitempty"`
	// Velero server’s log level (use debug for the most logging, leave unset for velero default)
	// +optional
	// +kubebuilder:validation:Enum=error;warn;warning;info;debug;trace
	LogLevel string `json:"logLevel,omitempty"`
}

// PodConfig defines the pod configuration options
type PodConfig struct {
	// NodeSelector defines the nodeSelector to be supplied to Restic podSpec
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations defines the list of tolerations to be applied to Restic daemonset
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// ResourceAllocations defines the CPU and Memory resource allocations for the restic Pod
	// +optional
	ResourceAllocations corev1.ResourceRequirements `json:"resourceAllocations,omitempty"`
}

// ResticConfig is the configuration for restic server
type ResticConfig struct {
	// Enable defines a boolean pointer whether we want the daemonset to
	// exist or not
	// +optional
	Enable *bool `json:"enable,omitempty"`
	// SupplementalGroups defines the linux groups to be applied to the Restic Pod
	// +optional
	SupplementalGroups []int64 `json:"supplementalGroups,omitempty"`
	// Timeout defines the Restic timeout, default value is 1h
	// +optional
	Timeout string `json:"timeout,omitempty"`
	// Pod specific configuration
	PodConfig *PodConfig `json:"podConfig,omitempty"`
}

// ApplicationConfig defines the configuration for the Data Protection Application
type ApplicationConfig struct {
	Velero *VeleroConfig `json:"velero,omitempty"`
	Restic *ResticConfig `json:"restic,omitempty"`
}

type CloudStorageLocation struct {
	CloudStorageRef corev1.LocalObjectReference `json:"cloudStorageRef"`

	// Config is for provider-specific configuration fields.
	// +optional
	Config map[string]string `json:"config,omitempty"`

	// Credential contains the credential information intended to be used with this location
	// +optional
	Credential *corev1.SecretKeySelector `json:"credential,omitempty"`

	// Default indicates this location is the default backup storage location.
	// +optional
	Default bool `json:"default,omitempty"`

	// BackupSyncPeriod defines how frequently to sync backup API objects from object storage. A value of 0 disables sync.
	// +optional
	// +nullable
	BackupSyncPeriod *metav1.Duration `json:"backupSyncPeriod,omitempty"`
}

// BackupLocation defines the configuration for the DPA backup storage
type BackupLocation struct {
	// TODO: Add name/annotations/labels support
	// +optional
	Velero *velero.BackupStorageLocationSpec `json:"velero,omitempty"`
	// +optional
	CloudStorage *CloudStorageLocation `json:"bucket,omitempty"`
}

// SnapshotLocation defines the configuration for the DPA snapshot store
type SnapshotLocation struct {
	// TODO: Add name/annotations/labels support
	Velero *velero.VolumeSnapshotLocationSpec `json:"velero"`
}

// DataProtectionApplicationSpec defines the desired state of Velero
type DataProtectionApplicationSpec struct {
	// BackupLocations defines the list of desired configuration to use for BackupStorageLocations
	// +optional
	BackupLocations []BackupLocation `json:"backupLocations"`
	// VolumeSnapshotLocations defines the list of desired configuration to use for VolumeSnapshotLocations
	// +optional
	SnapshotLocations []SnapshotLocation `json:"snapshotLocations"`
	// UnsupportedOverrides can be used to override the deployed dependent images for development
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
	// BackupImages is used to specify whether you want to deploy a registry for enabling backup and restore of images
	// +optional
	BackupImages *bool `json:"backupImages,omitempty"`

	// Configuration is used to configure the data protection application's server config
	Configuration *ApplicationConfig `json:"configuration"`
}

// DataProtectionApplicationStatus defines the observed state of DataProtectionApplication
type DataProtectionApplicationStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:categories=all,path=dataprotectionapplications,shortName=dpa

// DataProtectionApplication is the Schema for the dpa API
type DataProtectionApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataProtectionApplicationSpec   `json:"spec,omitempty"`
	Status DataProtectionApplicationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DataProtectionApplicationList contains a list of Velero
type DataProtectionApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataProtectionApplication `json:"items"`
}

// Default BackupImages behavior when nil to true
func (dpa *DataProtectionApplication) BackupImages() bool {
	return dpa.Spec.BackupImages == nil || *dpa.Spec.BackupImages
}

func init() {
	SchemeBuilder.Register(&DataProtectionApplication{}, &DataProtectionApplicationList{}, &CloudStorage{}, &CloudStorageList{})
}

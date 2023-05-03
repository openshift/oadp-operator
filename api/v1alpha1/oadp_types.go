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
	"time"

	"github.com/openshift/oadp-operator/pkg/common"
	"github.com/openshift/oadp-operator/pkg/velero/server"
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
const DataMoverDeploymentLabel = "openshift.io/oadp-data-mover"
const OadpBSLnameLabel = "openshift.io/oadp-bsl-name"
const OadpBSLProviderLabel = "openshift.io/oadp-bsl-provider"

type DefaultPlugin string

const DefaultPluginAWS DefaultPlugin = "aws"
const DefaultPluginGCP DefaultPlugin = "gcp"
const DefaultPluginMicrosoftAzure DefaultPlugin = "azure"
const DefaultPluginCSI DefaultPlugin = "csi"
const DefaultPluginVSM DefaultPlugin = "vsm"
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
const VSMPluginImageKey UnsupportedImageKey = "vsmPluginImageFqin"
const DataMoverImageKey UnsupportedImageKey = "dataMoverImageFqin"
const ResticRestoreImageKey UnsupportedImageKey = "resticRestoreImageFqin"
const KubeVirtPluginImageKey UnsupportedImageKey = "kubevirtPluginImageFqin"
const OperatorTypeKey UnsupportedImageKey = "operator-type"

const OperatorTypeMTC = "mtc"

type VeleroConfig struct {
	// featureFlags defines the list of features to enable for Velero instance
	// +optional
	FeatureFlags   []string        `json:"featureFlags,omitempty"`
	DefaultPlugins []DefaultPlugin `json:"defaultPlugins,omitempty"`
	// customPlugins defines the custom plugin to be installed with Velero
	// +optional
	CustomPlugins []CustomPlugin `json:"customPlugins,omitempty"`
	// restoreResourceVersionPriority represents a configmap that will be created if defined for use in conjunction with EnableAPIGroupVersions feature flag
	// Defining this field automatically add EnableAPIGroupVersions to the velero server feature flag
	// +optional
	RestoreResourcesVersionPriority string `json:"restoreResourcesVersionPriority,omitempty"`
	// If you need to install Velero without a default backup storage location noDefaultBackupLocation flag is required for confirmation
	// +optional
	NoDefaultBackupLocation bool `json:"noDefaultBackupLocation,omitempty"`
	// Pod specific configuration
	PodConfig *PodConfig `json:"podConfig,omitempty"`
	// Velero server’s log level (use debug for the most logging, leave unset for velero default)
	// +optional
	// +kubebuilder:validation:Enum=trace;debug;info;warning;error;fatal;panic
	LogLevel string `json:"logLevel,omitempty"`
	// How often to check status on async backup/restore operations after backup processing. Default value is 2m.
	// +optional
	ItemOperationSyncFrequency string `json:"itemOperationSyncFrequency,omitempty"`
	// How long to wait on asynchronous BackupItemActions and RestoreItemActions to complete before timing out. Default value is 1h.
	// +optional
	DefaultItemOperationTimeout string `json:"defaultItemOperationTimeout,omitempty"`
	// resourceTimeout defines how long to wait for several Velero resources before timeout occurs,
	// such as Velero CRD availability, volumeSnapshot deletion, and repo availability.
	// Default is 10m
	// +optional
	ResourceTimeout string `json:"resourceTimeout,omitempty"`
	// Velero args are settings to customize velero server arguments. Overrides values in other fields.
	// +optional
	Args *server.Args `json:"args,omitempty"`
}

// PodConfig defines the pod configuration options
type PodConfig struct {
	// labels to add to pods
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// nodeSelector defines the nodeSelector to be supplied to Restic podSpec
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// tolerations defines the list of tolerations to be applied to Restic daemonset
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// resourceAllocations defines the CPU and Memory resource allocations for the restic Pod
	// +optional
	// +nullable
	ResourceAllocations corev1.ResourceRequirements `json:"resourceAllocations,omitempty"`
	// env defines the list of environment variables to be supplied to podSpec
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
}

// ResticConfig is the configuration for restic server
type ResticConfig struct {
	// enable defines a boolean pointer whether we want the daemonset to
	// exist or not
	// +optional
	Enable *bool `json:"enable,omitempty"`
	// supplementalGroups defines the linux groups to be applied to the Restic Pod
	// +optional
	SupplementalGroups []int64 `json:"supplementalGroups,omitempty"`
	// timeout defines the Restic timeout, default value is 1h
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

	// config is for provider-specific configuration fields.
	// +optional
	Config map[string]string `json:"config,omitempty"`

	// credential contains the credential information intended to be used with this location
	// +optional
	Credential *corev1.SecretKeySelector `json:"credential,omitempty"`

	// default indicates this location is the default backup storage location.
	// +optional
	Default bool `json:"default,omitempty"`

	// backupSyncPeriod defines how frequently to sync backup API objects from object storage. A value of 0 disables sync.
	// +optional
	// +nullable
	BackupSyncPeriod *metav1.Duration `json:"backupSyncPeriod,omitempty"`
}

// BackupLocation defines the configuration for the DPA backup storage
type BackupLocation struct {
	// TODO: Add name/annotations/labels support

	// +optional
	Name string `json:"name,omitempty"`
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

// DataMover defines the various config for DPA data mover
type DataMover struct {
	// enable flag is used to specify whether you want to deploy the volume snapshot mover controller
	// +optional
	Enable bool `json:"enable,omitempty"`
	// User supplied Restic Secret name
	// +optional
	CredentialName string `json:"credentialName,omitempty"`
	// User supplied timeout to be used for VolumeSnapshotBackup and VolumeSnapshotRestore to complete, default value is 10m
	// +optional
	Timeout string `json:"timeout,omitempty"`
	// the number of batched volumeSnapshotBackups that can be inProgress at once, default value is 10
	// +optional
	MaxConcurrentBackupVolumes string `json:"maxConcurrentBackupVolumes,omitempty"`
	// the number of batched volumeSnapshotRestores that can be inProgress at once, default value is 10
	// +optional
	MaxConcurrentRestoreVolumes string `json:"maxConcurrentRestoreVolumes,omitempty"`
	// defines how often (in days) to prune the datamover snapshots from the repository
	// +optional
	PruneInterval string `json:"pruneInterval,omitempty"`
	// defines configurations for data mover volume options
	// +optional
	DataMoverVolumeOptions *DataMoverVolumeOptions `json:"volumeOptions,omitempty"`
	// defines the parameters that can be specified for retention of datamover snapshots
	// +optional
	SnapshotRetainPolicy *RetainPolicy `json:"snapshotRetainPolicy,omitempty"`
}

// RetainPolicy defines the fields for retention of datamover snapshots
type RetainPolicy struct {
	// Hourly defines the number of snapshots to be kept hourly
	//+optional
	Hourly string `json:"hourly,omitempty"`
	// Daily defines the number of snapshots to be kept daily
	//+optional
	Daily string `json:"daily,omitempty"`
	// Weekly defines the number of snapshots to be kept weekly
	//+optional
	Weekly string `json:"weekly,omitempty"`
	// Monthly defines the number of snapshots to be kept monthly
	//+optional
	Monthly string `json:"monthly,omitempty"`
	// Yearly defines the number of snapshots to be kept yearly
	//+optional
	Yearly string `json:"yearly,omitempty"`
	// Within defines the number of snapshots to be kept Within the given time period
	//+optional
	Within string `json:"within,omitempty"`
}

type DataMoverVolumeOptions struct {
	SourceVolumeOptions      *VolumeOptions `json:"sourceVolumeOptions,omitempty"`
	DestinationVolumeOptions *VolumeOptions `json:"destinationVolumeOptions,omitempty"`
}

// VolumeOptions defines configurations for VolSync options
type VolumeOptions struct {
	// storageClassName can be used to override the StorageClass of the source
	// or destination PVC
	//+optional
	StorageClassName string `json:"storageClassName,omitempty"`
	// accessMode can be used to override the accessMode of the source or
	// destination PVC
	//+optional
	AccessMode string `json:"accessMode,omitempty"`
	// cacheStorageClassName is the storageClass that should be used when provisioning
	// the data mover cache volume
	//+optional
	CacheStorageClassName string `json:"cacheStorageClassName,omitempty"`
	// cacheCapacity determines the size of the restic metadata cache volume
	//+optional
	CacheCapacity string `json:"cacheCapacity,omitempty"`
	// cacheAccessMode is the access mode to be used to provision the cache volume
	//+optional
	CacheAccessMode string `json:"cacheAccessMode,omitempty"`
	// moverSecurityContext allows enabling the PodSecurityContext used in
	// the application pod(s)
	//+optional
	MoverSecurityContext *bool `json:"moverSecurityContext,omitempty"`
}

// Features defines the configuration for the DPA to enable the tech preview features
type Features struct {
	// Contains data mover specific configurations
	// +optional
	DataMover *DataMover `json:"dataMover,omitempty"`
}

// DataProtectionApplicationSpec defines the desired state of Velero
type DataProtectionApplicationSpec struct {
	// backupLocations defines the list of desired configuration to use for BackupStorageLocations
	// +optional
	BackupLocations []BackupLocation `json:"backupLocations"`
	// snapshotLocations defines the list of desired configuration to use for VolumeSnapshotLocations
	// +optional
	SnapshotLocations []SnapshotLocation `json:"snapshotLocations"`
	// unsupportedOverrides can be used to override images used in deployments.
	// Available keys are:
	//   - veleroImageFqin
	//   - awsPluginImageFqin
	//   - openshiftPluginImageFqin
	//   - azurePluginImageFqin
	//   - gcpPluginImageFqin
	//   - csiPluginImageFqin
	//   - dataMoverImageFqin
	//   - resticRestoreImageFqin
	//   - kubevirtPluginImageFqin
	// +optional
	UnsupportedOverrides map[UnsupportedImageKey]string `json:"unsupportedOverrides,omitempty"`
	// add annotations to pods deployed by operator
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`
	// podDnsPolicy defines how a pod's DNS will be configured.
	// https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-s-dns-policy
	// +optional
	PodDnsPolicy corev1.DNSPolicy `json:"podDnsPolicy,omitempty"`
	// podDnsConfig defines the DNS parameters of a pod in addition to
	// those generated from DNSPolicy.
	// https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-dns-config
	// +optional
	PodDnsConfig corev1.PodDNSConfig `json:"podDnsConfig,omitempty"`
	// backupImages is used to specify whether you want to deploy a registry for enabling backup and restore of images
	// +optional
	BackupImages *bool `json:"backupImages,omitempty"`
	// configuration is used to configure the data protection application's server config
	Configuration *ApplicationConfig `json:"configuration"`
	// features defines the configuration for the DPA to enable the OADP tech preview features
	// +optional
	Features *Features `json:"features"`
}

// DataProtectionApplicationStatus defines the observed state of DataProtectionApplication
type DataProtectionApplicationStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=dataprotectionapplications,shortName=dpa

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

func (veleroConfig *VeleroConfig) HasFeatureFlag(flag string) bool {
	for _, featureFlag := range veleroConfig.FeatureFlags {
		if featureFlag == flag {
			return true
		}
	}
	return false
}

func init() {
	SchemeBuilder.Register(&DataProtectionApplication{}, &DataProtectionApplicationList{}, &CloudStorage{}, &CloudStorageList{})
}

// AutoCorrect is a collection of auto-correction functions for the DPA CR
// These auto corrects are in-memory only and do not persist to the CR
// There should not be another place where these auto-corrects are done
func (dpa *DataProtectionApplication) AutoCorrect() {
	//check if CSI plugin is added in spec
	if hasCSIPlugin(dpa.Spec.Configuration.Velero.DefaultPlugins) {
		dpa.Spec.Configuration.Velero.FeatureFlags = append(dpa.Spec.Configuration.Velero.FeatureFlags, velero.CSIFeatureFlag)
	}
	if dpa.Spec.Configuration.Velero.RestoreResourcesVersionPriority != "" {
		// if the RestoreResourcesVersionPriority is specified then ensure feature flag is enabled for enableApiGroupVersions
		// duplicate feature flag checks are done in ReconcileVeleroDeployment
		dpa.Spec.Configuration.Velero.FeatureFlags = append(dpa.Spec.Configuration.Velero.FeatureFlags, velero.APIGroupVersionsFeatureFlag)
	}
	if dpa.Spec.Configuration.Velero.Args != nil {
		// if args is not nil, we take care of some fields that will be overridden from dpa if not specified in args
		// Enable user to specify --restic-timeout (defaults to 1h)
		resticTimeout := "1h"
		if dpa.Spec.Configuration != nil && dpa.Spec.Configuration.Restic != nil && len(dpa.Spec.Configuration.Restic.Timeout) > 0 {
			resticTimeout = dpa.Spec.Configuration.Restic.Timeout
		}
		if pvOperationTimeout, err := time.ParseDuration(resticTimeout); err == nil && dpa.Spec.Configuration.Velero.Args.PodVolumeOperationTimeout == nil {
			dpa.Spec.Configuration.Velero.Args.PodVolumeOperationTimeout = &pvOperationTimeout
		}
	}
	dpa.Spec.Configuration.Velero.DefaultPlugins = common.RemoveDuplicateValues(dpa.Spec.Configuration.Velero.DefaultPlugins)
	dpa.Spec.Configuration.Velero.FeatureFlags = common.RemoveDuplicateValues(dpa.Spec.Configuration.Velero.FeatureFlags)
}

func hasCSIPlugin(plugins []DefaultPlugin) bool {
	for _, plugin := range plugins {
		if plugin == DefaultPluginCSI {
			// CSI plugin is added so ensure that CSI feature flags is set
			return true
		}
	}
	return false
}

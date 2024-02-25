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
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=aws;gcp;azure;csi;openshift;kubevirt
type DefaultPlugin string

const (
	DefaultPluginAWS            DefaultPlugin = "aws"
	DefaultPluginGCP            DefaultPlugin = "gcp"
	DefaultPluginMicrosoftAzure DefaultPlugin = "azure"
	DefaultPluginCSI            DefaultPlugin = "csi"
	DefaultPluginOpenShift      DefaultPlugin = "openshift"
	DefaultPluginKubeVirt       DefaultPlugin = "kubevirt"
)

type CustomPlugin struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

// Field does not have enum validation for development flexibility
type UnsupportedImageKey string

const (
	VeleroImageKey          UnsupportedImageKey = "veleroImageFqin"
	AWSPluginImageKey       UnsupportedImageKey = "awsPluginImageFqin"
	OpenShiftPluginImageKey UnsupportedImageKey = "openshiftPluginImageFqin"
	AzurePluginImageKey     UnsupportedImageKey = "azurePluginImageFqin"
	GCPPluginImageKey       UnsupportedImageKey = "gcpPluginImageFqin"
	CSIPluginImageKey       UnsupportedImageKey = "csiPluginImageFqin"
	ResticRestoreImageKey   UnsupportedImageKey = "resticRestoreImageFqin"
	KubeVirtPluginImageKey  UnsupportedImageKey = "kubevirtPluginImageFqin"
	OperatorTypeKey         UnsupportedImageKey = "operator-type"
)

// Args are the arguments that are passed to the Velero server
type Args struct {
	ServerFlags `json:",inline"`
	GlobalFlags `json:",inline"`
}

// To get these flags, run
// go run github.com/openshift/velero@SPECIFIC-VERSION-FROM-GO-MOD help server

// Flags defined under "Flags:"

// ServerFlags are flags that are defined for Velero server CLI command
type ServerFlags struct {
	// TODO check description of each flag

	// How often (in nanoseconds) to ensure all Velero backups in object storage exist as Backup API objects in the cluster. This is the default sync period if none is explicitly specified for a backup storage location.
	// +optional
	BackupSyncPeriod *time.Duration `json:"backup-sync-period,omitempty"`

	// Maximum number of requests by the server to the Kubernetes API in a short period of time.
	// +optional
	ClientBurst *int `json:"client-burst,omitempty"`

	// Page size of requests by the server to the Kubernetes API when listing objects during a backup. Set to 0 to disable paging.
	// +optional
	ClientPageSize *int `json:"client-page-size,omitempty"`

	// Maximum number of requests per second by the server to the Kubernetes API once the burst limit has been reached.
	// this will be validated as a valid float32
	// +optional
	ClientQPS *string `json:"client-qps,omitempty"`

	// --default-backup-storage-location string              Name of the default backup storage location. DEPRECATED: this flag will be removed in v2.0. Use "velero backup-location set --default" instead. (default "default")

	// How long (in nanoseconds) to wait by default before backups can be garbage collected. (default is 720 hours)
	// +optional
	DefaultBackupTTL *time.Duration `json:"default-backup-ttl,omitempty"`

	// How long (in nanoseconds) to wait on asynchronous BackupItemActions and RestoreItemActions to complete before timing out. (default is 1 hour)
	DefaultItemOperationTimeout *time.Duration `json:"default-item-operation-timeout,omitempty"`
	// DUPLICATE OF VeleroConfig.DefaultItemOperationTimeout

	// How often (in nanoseconds) 'maintain' is run for backup repositories by default.
	// +optional
	RepoMaintenanceFrequency *time.Duration `json:"default-repo-maintain-frequency,omitempty"`

	// --default-snapshot-move-data                          Move data by default for all snapshots supporting data movement.
	// VeleroConfig.DefaultSnapshotMoveData

	// TODO --default-volume-snapshot-locations mapStringString   List of unique volume providers and default volume snapshot location (provider1:location-01,provider2:location-02,...)

	// Backup all volumes with pod volume file system backup by default.
	// +optional
	DefaultVolumesToFsBackup *bool `json:"default-volumes-to-fs-backup,omitempty"`
	// DUPLICATE of VeleroConfig.DefaultVolumesToFSBackup

	// --disable-controllers strings                         List of controllers to disable on startup. Valid values are backup,backup-operations,backup-deletion,backup-finalizer,backup-sync,download-request,gc,backup-repo,restore,restore-operations,schedule,server-status-request
	// TYPO? List of controllers to disable on startup. Valid values are backup,backup-operations,backup-deletion,backup-finalizer,backup-sync,download-request,gc,backup-repo,restore,restore-operations,schedule,server-status-request
	// +kubebuilder:validation:Enum=backup;backup-operations;backup-deletion;backup-finalizer;backup-sync;download-request;gc;backup-repo;restore;restore-operations;schedule;server-status-request
	// +optional
	DisabledControllers []string `json:"disabled-controllers,omitempty"`

	// --disable-informer-cache                              Disable informer cache for Get calls on restore. With this enabled, it will speed up restore in cases where there are backup resources which already exist in the cluster, but for very large clusters this will increase velero memory usage. Default is false (don't disable).
	// VeleroConfig.DisableInformerCache

	// How long (in nanoseconds) pod volume file system backups/restores should be allowed to run before timing out. (default is 4 hours)
	// +optional
	PodVolumeOperationTimeout *time.Duration `json:"fs-backup-timeout,omitempty"`
	// DUPLICATE of NodeAgentCommonFields.Timeout

	// How often (in nanoseconds) garbage collection checks for expired backups. (default is 1 hour)
	// +optional
	GarbageCollectionFrequency *time.Duration `json:"garbage-collection-frequency,omitempty"`

	// How often (in nanoseconds) to check status on backup/restore operations after backup/restore processing.
	// +optional
	ItemOperationSyncFrequency *time.Duration `json:"item-operation-sync-frequency,omitempty"`
	// DUPLICATE of VeleroConfig.ItemOperationSyncFrequency

	// The format for log output. Valid values are text, json. (default text)
	// +kubebuilder:validation:Enum=text;json
	// +optional
	FormatFlag string `json:"log-format,omitempty"`

	// --log-level                                           The level at which to log. Valid values are trace, debug, info, warning, error, fatal, panic. (default info)
	// VeleroConfig.LogLevel

	// Max concurrent connections number that Velero can create with kube-apiserver. Default is 30. (default 30)
	MaxConcurrentK8SConnections *int `json:"max-concurrent-k8s-connections,omitempty"`

	// The address to expose prometheus metrics
	// +optional
	MetricsAddress string `json:"metrics-address,omitempty"`

	// TODO --plugin-dir string                                   Directory containing Velero plugins (default "/plugins")

	// The address to expose the pprof profiler.
	// +optional
	ProfilerAddress string `json:"profiler-address,omitempty"`

	// How long (in nanoseconds) to wait for resource processes which are not covered by other specific timeout parameters. (default is 10 minutes)
	ResourceTimeout *time.Duration `json:"resource-timeout,omitempty"`
	// DUPLICATE of VeleroConfig.ResourceTimeout

	// TODO DEPRECATED: this flag will be removed in v2.0. Use read-only backup storage locations instead.
	// +optional
	// RestoreOnly *bool `json:"restore-only,omitempty"`

	// Desired order of resource restores, the priority list contains two parts which are split by "-" element. The resources before "-" element are restored first as high priorities, the resources after "-" element are restored last as low priorities, and any resource not in the list will be restored alphabetically between the high and low priorities. (default securitycontextconstraints,customresourcedefinitions,namespaces,roles,rolebindings,clusterrolebindings,managedcluster.cluster.open-cluster-management.io,managedcluster.clusterview.open-cluster-management.io,klusterletaddonconfig.agent.open-cluster-management.io,managedclusteraddon.addon.open-cluster-management.io,storageclasses,volumesnapshotclass.snapshot.storage.k8s.io,volumesnapshotcontents.snapshot.storage.k8s.io,volumesnapshots.snapshot.storage.k8s.io,datauploads.velero.io,persistentvolumes,persistentvolumeclaims,serviceaccounts,secrets,configmaps,limitranges,pods,replicasets.apps,clusterclasses.cluster.x-k8s.io,endpoints,services,-,clusterbootstraps.run.tanzu.vmware.com,clusters.cluster.x-k8s.io,clusterresourcesets.addons.cluster.x-k8s.io)
	// +optional
	RestoreResourcePriorities string `json:"restore-resource-priorities,omitempty"`

	// TODO --schedule-skip-immediately                           Skip the first scheduled backup immediately after creating a schedule. Default is false (don't skip).

	// How often (in nanoseconds) to verify if the storage is valid. Optional. Set this to `0` to disable sync. (default is 1 minute)
	// +optional
	StoreValidationFrequency *time.Duration `json:"store-validation-frequency,omitempty"`

	// How long (in nanoseconds) to wait on persistent volumes and namespaces to terminate during a restore before timing out.
	// +optional
	ResourceTerminatingTimeout *time.Duration `json:"terminating-resource-timeout,omitempty"`

	// --uploader-type string                                Type of uploader to handle the transfer of data of pod volumes (default "restic")
	// NodeAgentConfig.UploaderType
}

// Flags defined under "Global Flags:"

// GlobalFlags are flags that are defined across Velero CLI commands
type GlobalFlags struct {
	// If true, adds the file directory to the header of the log messages
	// +optional
	AddDirHeader *bool `json:"add_dir_header,omitempty"`

	// log to standard error as well as files (no effect when -logtostderr=true)
	// +optional
	AlsoToStderr *bool `json:"alsologtostderr,omitempty"`

	// Show colored output in TTY
	// +optional
	Colorized *bool `json:"colorized,omitempty"`

	// --features stringArray             Comma-separated list of features to enable for this Velero process. Combines with values from $HOME/.config/velero/config.json if present
	// VeleroConfig.FeatureFlags

	// TODO --kubeconfig string                Path to the kubeconfig file to use to talk to the Kubernetes apiserver. If unset, try the environment variable KUBECONFIG, as well as in-cluster configuration

	// TODO --kubecontext string               The context to use to talk to the Kubernetes apiserver. If unset defaults to whatever your current-context is (kubectl config current-context)

	// when logging hits line file:N, emit a stack trace (default :0)
	// +optional
	TraceLocation string `json:"log_backtrace_at,omitempty"`

	// If non-empty, write log files in this directory (no effect when -logtostderr=true)
	// +optional
	LogDir string `json:"log_dir,omitempty"`

	// If non-empty, use this log file (no effect when -logtostderr=true)
	// +optional
	LogFile string `json:"log_file,omitempty"`

	// Defines the maximum size a log file can grow to (no effect when -logtostderr=true). Unit is megabytes. If the value is 0, the maximum file size is unlimited. (default 1800)
	// +kubebuilder:validation:Minimum=0
	// +optional
	LogFileMaxSizeMB *int64 `json:"log_file_max_size,omitempty"`

	// log to standard error instead of files (default true)
	// +optional
	ToStderr *bool `json:"logtostderr,omitempty"`

	// TODO -n, --namespace string                 The namespace in which Velero should operate (default "velero")

	// If true, only write logs to their native severity level (vs also writing to each lower severity level; no effect when -logtostderr=true)
	// +optional
	OneOutput *bool `json:"one_output,omitempty"`

	// If true, avoid header prefixes in the log messages
	// +optional
	SkipHeaders *bool `json:"skip_headers,omitempty"`

	// If true, avoid headers when opening log files (no effect when -logtostderr=true)
	// +optional
	SkipLogHeaders *bool `json:"skip_log_headers,omitempty"`

	// logs at or above this threshold go to stderr when writing to files and stderr (no effect when -logtostderr=true or -alsologtostderr=false) (default 2)
	// +optional
	StderrThreshold *int `json:"stderrthreshold,omitempty"`

	// number for the log level verbosity
	// +optional
	Verbosity *int `json:"v,omitempty"`

	// comma-separated list of pattern=N settings for file-filtered logging
	// +optional
	Vmodule string `json:"vmodule,omitempty"`
}

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
	// Use pod volume file system backup by default for volumes
	// +optional
	DefaultVolumesToFSBackup *bool `json:"defaultVolumesToFSBackup,omitempty"`
	// Specify whether CSI snapshot data should be moved to backup storage by default
	// +optional
	DefaultSnapshotMoveData *bool `json:"defaultSnapshotMoveData,omitempty"`
	// Disable informer cache for Get calls on restore. With this enabled, it will speed up restore in cases where there are backup resources which already exist in the cluster, but for very large clusters this will increase velero memory usage. Default is false.
	// +optional
	DisableInformerCache *bool `json:"disableInformerCache,omitempty"`
	// resourceTimeout defines how long to wait for several Velero resources before timeout occurs,
	// such as Velero CRD availability, volumeSnapshot deletion, and repo availability.
	// Default is 10m
	// +optional
	ResourceTimeout string `json:"resourceTimeout,omitempty"`
	// Velero args are settings to customize velero server arguments. Overrides values in other fields.
	// +optional
	Args *Args `json:"args,omitempty"`
}

// PodConfig defines the pod configuration options
type PodConfig struct {
	// labels to add to pods
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// nodeSelector defines the nodeSelector to be supplied to podSpec
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// tolerations defines the list of tolerations to be applied to daemonset
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// resourceAllocations defines the CPU and Memory resource allocations for the Pod
	// +optional
	// +nullable
	ResourceAllocations corev1.ResourceRequirements `json:"resourceAllocations,omitempty"`
	// env defines the list of environment variables to be supplied to podSpec
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
}

type NodeAgentCommonFields struct {
	// enable defines a boolean pointer whether we want the daemonset to
	// exist or not
	// +optional
	Enable *bool `json:"enable,omitempty"`
	// supplementalGroups defines the linux groups to be applied to the NodeAgent Pod
	// +optional
	SupplementalGroups []int64 `json:"supplementalGroups,omitempty"`
	// timeout defines the NodeAgent timeout, default value is 1h
	// +optional
	Timeout string `json:"timeout,omitempty"`
	// Pod specific configuration
	PodConfig *PodConfig `json:"podConfig,omitempty"`
}

// NodeAgentConfig is the configuration for node server
type NodeAgentConfig struct {
	// Embedding NodeAgentCommonFields
	// +optional
	NodeAgentCommonFields `json:",inline"`

	// The type of uploader to transfer the data of pod volumes, the supported values are 'restic' or 'kopia'
	// +kubebuilder:validation:Enum=restic;kopia
	// +kubebuilder:validation:Required
	UploaderType string `json:"uploaderType"`
}

// ResticConfig is the configuration for restic server
type ResticConfig struct {
	// Embedding NodeAgentCommonFields
	// +optional
	NodeAgentCommonFields `json:",inline"`
}

// ApplicationConfig defines the configuration for the Data Protection Application
type ApplicationConfig struct {
	// TODO missing description for velero

	Velero *VeleroConfig `json:"velero,omitempty"`

	// (deprecation warning) ResticConfig is the configuration for restic DaemonSet.
	// restic is for backwards compatibility and is replaced by the nodeAgent
	// restic will be removed with the OADP 1.4
	// +kubebuilder:deprecatedversion:warning=1.3
	// +optional
	Restic *ResticConfig `json:"restic,omitempty"`

	// NodeAgent is needed to allow selection between kopia or restic
	// +optional
	NodeAgent *NodeAgentConfig `json:"nodeAgent,omitempty"`
}

// CloudStorageLocation defines BackupStorageLocation using bucket referenced by CloudStorage CR.
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

	// Prefix and CACert are copied from velero/pkg/apis/v1/backupstoragelocation_types.go under ObjectStorageLocation

	// Prefix is the path inside a bucket to use for Velero storage. Optional.
	// +optional
	Prefix string `json:"prefix,omitempty"`

	// CACert defines a CA bundle to use when verifying TLS connections to the provider.
	// +optional
	CACert []byte `json:"caCert,omitempty"`
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

// Features defines the configuration for the DPA to enable the tech preview features
type Features struct{}

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
	//   - resticRestoreImageFqin
	//   - kubevirtPluginImageFqin
	//   - operator-type
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

func init() {
	SchemeBuilder.Register(&DataProtectionApplication{}, &DataProtectionApplicationList{}, &CloudStorage{}, &CloudStorageList{})
}

// TODO write tests for these functions

// Default BackupImages behavior when nil to true
func (dpa *DataProtectionApplication) BackupImages() bool {
	return dpa.Spec.BackupImages == nil || *dpa.Spec.BackupImages
}

// Default DisableInformerCache behavior when nil to false
func (dpa *DataProtectionApplication) GetDisableInformerCache() bool {
	if dpa.Spec.Configuration.Velero.DisableInformerCache == nil {
		return false
	}
	return *dpa.Spec.Configuration.Velero.DisableInformerCache
}

func (veleroConfig *VeleroConfig) HasFeatureFlag(flag string) bool {
	for _, featureFlag := range veleroConfig.FeatureFlags {
		if featureFlag == flag {
			return true
		}
	}
	return false
}

// AutoCorrect is a collection of auto-correction functions for the DPA CR
// These auto corrects are in-memory only and do not persist to the CR
// There should not be another place where these auto-corrects are done
func (dpa *DataProtectionApplication) AutoCorrect() {
	// TODO error instead of changing user object?

	//check if CSI plugin is added in spec
	if hasCSIPlugin(dpa.Spec.Configuration.Velero.DefaultPlugins) {
		// CSI plugin is added, so ensure that CSI feature flags is set
		dpa.Spec.Configuration.Velero.FeatureFlags = append(dpa.Spec.Configuration.Velero.FeatureFlags, velero.CSIFeatureFlag)
	}
	if dpa.Spec.Configuration.Velero.RestoreResourcesVersionPriority != "" {
		// if the RestoreResourcesVersionPriority is specified then ensure feature flag is enabled for enableApiGroupVersions
		// duplicate feature flag checks are done in ReconcileVeleroDeployment
		dpa.Spec.Configuration.Velero.FeatureFlags = append(dpa.Spec.Configuration.Velero.FeatureFlags, velero.APIGroupVersionsFeatureFlag)
	}

	dpa.Spec.Configuration.Velero.DefaultPlugins = common.RemoveDuplicateValues(dpa.Spec.Configuration.Velero.DefaultPlugins)
	dpa.Spec.Configuration.Velero.FeatureFlags = common.RemoveDuplicateValues(dpa.Spec.Configuration.Velero.FeatureFlags)
}

func hasCSIPlugin(plugins []DefaultPlugin) bool {
	for _, plugin := range plugins {
		if plugin == DefaultPluginCSI {
			return true
		}
	}
	return false
}

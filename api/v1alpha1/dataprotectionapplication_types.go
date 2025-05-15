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

	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/nodeagent"
	"github.com/vmware-tanzu/velero/pkg/util/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/oadp-operator/pkg/common"
)

// Conditions
const ConditionReconciled = "Reconciled"
const ReconciledReasonComplete = "Complete"
const ReconciledReasonError = "Error"
const ReconcileCompleteMessage = "Reconcile complete"

const OadpOperatorLabel = "openshift.io/oadp"

// +kubebuilder:validation:Enum=aws;legacy-aws;gcp;azure;csi;vsm;openshift;kubevirt;hypershift
type DefaultPlugin string

const DefaultPluginAWS DefaultPlugin = "aws"
const DefaultPluginLegacyAWS DefaultPlugin = "legacy-aws"
const DefaultPluginGCP DefaultPlugin = "gcp"
const DefaultPluginMicrosoftAzure DefaultPlugin = "azure"
const DefaultPluginCSI DefaultPlugin = "csi"
const DefaultPluginVSM DefaultPlugin = "vsm"
const DefaultPluginOpenShift DefaultPlugin = "openshift"
const DefaultPluginKubeVirt DefaultPlugin = "kubevirt"
const DefaultPluginHypershift DefaultPlugin = "hypershift"

type CustomPlugin struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

// Field does not have enum validation for development flexibility
type UnsupportedImageKey string

const VeleroImageKey UnsupportedImageKey = "veleroImageFqin"
const AWSPluginImageKey UnsupportedImageKey = "awsPluginImageFqin"
const LegacyAWSPluginImageKey UnsupportedImageKey = "legacyAWSPluginImageFqin"
const OpenShiftPluginImageKey UnsupportedImageKey = "openshiftPluginImageFqin"
const AzurePluginImageKey UnsupportedImageKey = "azurePluginImageFqin"
const GCPPluginImageKey UnsupportedImageKey = "gcpPluginImageFqin"
const ResticRestoreImageKey UnsupportedImageKey = "resticRestoreImageFqin"
const KubeVirtPluginImageKey UnsupportedImageKey = "kubevirtPluginImageFqin"
const HypershiftPluginImageKey UnsupportedImageKey = "hypershiftPluginImageFqin"
const NonAdminControllerImageKey UnsupportedImageKey = "nonAdminControllerImageFqin"
const OperatorTypeKey UnsupportedImageKey = "operator-type"

const OperatorTypeMTC = "mtc"

// NAC defaults
const (
	DefaultGarbageCollectionPeriod = 24 * time.Hour
	DefaultBackupSyncPeriod        = 2 * time.Minute
)

// VeleroServerArgs are the arguments that are passed to the Velero server
type VeleroServerArgs struct {
	ServerFlags `json:",inline"`
	GlobalFlags `json:",inline"`
}

// This package is used to store ServerConfig struct and note information about flags for velero server and how they are set.
// The options you can set in `velero server` is a combination of ServerConfig, featureFlagSet

// ServerConfig holds almost all the configuration for the Velero server.
// https://github.com/openshift/velero/blob/dd02df5cd5751263fce6d1ebd48ea11423b0cd16/pkg/cmd/server/server.go#L112-L129
// +kubebuilder:object:generate=true
type ServerFlags struct {
	// pluginDir will be fixed to /plugins
	// pluginDir

	// The address to expose prometheus metrics
	// +optional
	MetricsAddress string `json:"metrics-address,omitempty"`
	// defaultBackupLocation will be defined outside of server config in DataProtectionApplication
	// defaultBackupLocation                        string

	// How often (in nanoseconds) to ensure all Velero backups in object storage exist as Backup API objects in the cluster. This is the default sync period if none is explicitly specified for a backup storage location.
	// +optional
	BackupSyncPeriod *time.Duration `json:"backup-sync-period,omitempty"`
	// How long (in nanoseconds) pod volume file system backups/restores should be allowed to run before timing out. (default is 4 hours)
	// +optional
	PodVolumeOperationTimeout *time.Duration `json:"fs-backup-timeout,omitempty"`
	// How long (in nanoseconds) to wait on persistent volumes and namespaces to terminate during a restore before timing out.
	// +optional
	ResourceTerminatingTimeout *time.Duration `json:"terminating-resource-timeout,omitempty"`
	// How long (in nanoseconds) to wait by default before backups can be garbage collected. (default is 720 hours)
	// +optional
	DefaultBackupTTL *time.Duration `json:"default-backup-ttl,omitempty"`
	// How often (in nanoseconds) to verify if the storage is valid. Optional. Set this to `0` to disable sync. (default is 1 minute)
	// +optional
	StoreValidationFrequency *time.Duration `json:"store-validation-frequency,omitempty"`
	// Desired order of resource restores, the priority list contains two parts which are split by "-" element. The resources before "-" element are restored first as high priorities, the resources after "-" element are restored last as low priorities, and any resource not in the list will be restored alphabetically between the high and low priorities. (default securitycontextconstraints,customresourcedefinitions,klusterletconfigs.config.open-cluster-management.io,managedcluster.cluster.open-cluster-management.io,namespaces,roles,rolebindings,clusterrolebindings,klusterletaddonconfig.agent.open-cluster-management.io,managedclusteraddon.addon.open-cluster-management.io,storageclasses,volumesnapshotclass.snapshot.storage.k8s.io,volumesnapshotcontents.snapshot.storage.k8s.io,volumesnapshots.snapshot.storage.k8s.io,datauploads.velero.io,persistentvolumes,persistentvolumeclaims,serviceaccounts,secrets,configmaps,limitranges,pods,replicasets.apps,clusterclasses.cluster.x-k8s.io,endpoints,services,-,clusterbootstraps.run.tanzu.vmware.com,clusters.cluster.x-k8s.io,clusterresourcesets.addons.cluster.x-k8s.io)
	// +optional
	RestoreResourcePriorities string `json:"restore-resource-priorities,omitempty"`
	// defaultVolumeSnapshotLocations will be defined outside of server config in DataProtectionApplication
	// defaultVolumeSnapshotLocations                                          map[string]string

	// DEPRECATED: this flag will be removed in v2.0. Use read-only backup storage locations instead.
	// +optional
	// RestoreOnly *bool `json:"restore-only,omitempty"`

	// List of controllers to disable on startup. Valid values are backup,backup-operations,backup-deletion,backup-finalizer,backup-sync,download-request,gc,backup-repo,restore,restore-operations,schedule,server-status-request
	// +kubebuilder:validation:Enum=backup;backup-operations;backup-deletion;backup-finalizer;backup-sync;download-request;gc;backup-repo;restore;restore-operations;schedule;server-status-request
	// +optional
	DisabledControllers []string `json:"disabled-controllers,omitempty"`
	// Maximum number of requests per second by the server to the Kubernetes API once the burst limit has been reached.
	// this will be validated as a valid float32
	// +optional
	ClientQPS *string `json:"client-qps,omitempty"`
	// Maximum number of requests by the server to the Kubernetes API in a short period of time.
	// +optional
	ClientBurst *int `json:"client-burst,omitempty"`
	// Page size of requests by the server to the Kubernetes API when listing objects during a backup. Set to 0 to disable paging.
	// +optional
	ClientPageSize *int `json:"client-page-size,omitempty"`
	// The address to expose the pprof profiler.
	// +optional
	ProfilerAddress string `json:"profiler-address,omitempty"`
	// How often (in nanoseconds) to check status on backup/restore operations after backup/restore processing.
	// +optional
	ItemOperationSyncFrequency *time.Duration `json:"item-operation-sync-frequency,omitempty"`
	// The format for log output. Valid values are text, json. (default text)
	// +kubebuilder:validation:Enum=text;json
	// +optional
	FormatFlag string `json:"log-format,omitempty"`
	// How often (in nanoseconds) 'maintain' is run for backup repositories by default.
	// +optional
	RepoMaintenanceFrequency *time.Duration `json:"default-repo-maintain-frequency,omitempty"`
	// How often (in nanoseconds) garbage collection checks for expired backups. (default is 1 hour)
	// +optional
	GarbageCollectionFrequency *time.Duration `json:"garbage-collection-frequency,omitempty"`
	// Backup all volumes with pod volume file system backup by default.
	// +optional
	DefaultVolumesToFsBackup *bool `json:"default-volumes-to-fs-backup,omitempty"`
	// How long (in nanoseconds) to wait on asynchronous BackupItemActions and RestoreItemActions to complete before timing out. (default is 1 hour)
	DefaultItemOperationTimeout *time.Duration `json:"default-item-operation-timeout,omitempty"`
	// How long (in nanoseconds) to wait for resource processes which are not covered by other specific timeout parameters. (default is 10 minutes)
	ResourceTimeout *time.Duration `json:"resource-timeout,omitempty"`
	// Max concurrent connections number that Velero can create with kube-apiserver. Default is 30. (default 30)
	MaxConcurrentK8SConnections *int `json:"max-concurrent-k8s-connections,omitempty"`
}

// GlobalFlags are flags that are defined across Velero CLI commands
type GlobalFlags struct {
	// We use same namespace as DataProtectionApplication
	// Namespace string `json:"namespace,omitempty"`

	// Features is an existing field in DataProtectionApplication
	// --kubebuilder:validation:Enum=EnableCSI;EnableAPIGroupVersions;EnableUploadProgress
	// Features  []VeleroFeatureFlag `json:"features,omitempty"`

	// Show colored output in TTY
	// +optional
	Colorized *bool `json:"colorized,omitempty"`
	// CACert is not a flag in velero server
	LoggingFlags `json:",inline"`
}

// klog init flags from https://github.com/openshift/velero/blob/240b4e666fe15ef98defa2b51483fe87ac9996fb/pkg/cmd/velero/velero.go#L125
// LoggingFlags collects all the global state of the logging setup.
type LoggingFlags struct {
	// Boolean flags. Not handled atomically because the flag.Value interface
	// does not let us avoid the =true, and that shorthand is necessary for
	// compatibility. TODO: does this matter enough to fix? Seems unlikely.
	// +optional
	ToStderr *bool `json:"logtostderr,omitempty"` // The -logtostderr flag.
	// log to standard error as well as files (no effect when -logtostderr=true)
	// +optional
	AlsoToStderr *bool `json:"alsologtostderr,omitempty"` // The -alsologtostderr flag.

	// logs at or above this threshold go to stderr when writing to files and stderr (no effect when -logtostderr=true or -alsologtostderr=false) (default 2)
	// +optional
	StderrThreshold *int `json:"stderrthreshold,omitempty"` // The -stderrthreshold flag.

	// bufferCache maintains the free list. It uses its own mutex
	// so buffers can be grabbed and printed to without holding the main lock,
	// for better parallelization.
	// bufferCache buffer.Buffers

	// mu protects the remaining elements of this structure and is
	// used to synchronize logging.
	// mu sync.Mutex
	// file holds writer for each of the log types.
	// file [severity.NumSeverity]flushSyncWriter
	// flushD holds a flushDaemon that frequently flushes log file buffers.
	// flushD *flushDaemon
	// flushInterval is the interval for periodic flushing. If zero,
	// the global default will be used.
	// flushInterval time.Duration
	// pcs is used in V to avoid an allocation when computing the caller's PC.
	// pcs [1]uintptr
	// vmap is a cache of the V Level for each V() call site, identified by PC.
	// It is wiped whenever the vmodule flag changes state.
	// vmap map[uintptr]Level
	// filterLength stores the length of the vmodule filter chain. If greater
	// than zero, it means vmodule is enabled. It may be read safely
	// using sync.LoadInt32, but is only modified under mu.
	// filterLength int32
	// traceLocation is the state of the -log_backtrace_at flag.

	// when logging hits line file:N, emit a stack trace
	// +optional
	TraceLocation string `json:"log_backtrace_at,omitempty"`
	// These flags are modified only under lock, although verbosity may be fetched
	// safely using atomic.LoadInt32.

	// comma-separated list of pattern=N settings for file-filtered logging
	// +optional
	Vmodule string `json:"vmodule,omitempty"` // The state of the -vmodule flag.
	// number for the log level verbosity
	// +optional
	Verbosity *int `json:"v,omitempty"` // V logging level, the value of the -v flag/

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

	// If true, avoid header prefixes in the log messages
	// +optional
	SkipHeaders *bool `json:"skip_headers,omitempty"`

	// If true, avoid headers when opening log files (no effect when -logtostderr=true)
	// +optional
	SkipLogHeaders *bool `json:"skip_log_headers,omitempty"`

	// If true, adds the file directory to the header of the log messages
	// +optional
	AddDirHeader *bool `json:"add_dir_header,omitempty"`

	// If true, only write logs to their native severity level (vs also writing to each lower severity level; no effect when -logtostderr=true)
	// +optional
	OneOutput *bool `json:"one_output,omitempty"`

	// If set, all output will be filtered through the filter.
	// filter LogFilter
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
	// Velero server's log level (use debug for the most logging, leave unset for velero default)
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
	// DisableFsBackup determines whether the NodeAgent should disable file system backup.
	// When set to true, the NodeAgent runs in non-privileged mode.
	// Defaults to false.
	// +optional
	// +kubebuilder:default=false
	DisableFsBackup *bool `json:"disableFsBackup,omitempty"`
	// Specify whether CSI snapshot data should be moved to backup storage by default
	// +optional
	DefaultSnapshotMoveData *bool `json:"defaultSnapshotMoveData,omitempty"`
	// Disable informer cache for Get calls on restore. With this enabled, it will speed up restore in cases where there are backup resources which already exist in the cluster, but for very large clusters this will increase velero memory usage. Default is false.
	// +optional
	DisableInformerCache *bool `json:"disableInformerCache,omitempty"`
	// Number of workers in worker pool for processing item backup. This will allow multiple items within
	// a Velero backup to be backed up at the same time which may improve performance for backups with
	// a large number of items. Default is 1.
	// +optional
	ItemBlockWorkerCount int `json:"itemBlockWorkerCount,omitempty"`
	// resourceTimeout defines how long to wait for several Velero resources before timeout occurs,
	// such as Velero CRD availability, volumeSnapshot deletion, and repo availability.
	// Default is 10m
	// +optional
	ResourceTimeout string `json:"resourceTimeout,omitempty"`
	// maximum number of requests by the server to the Kubernetes API in a short period of time. (default 100)
	// +optional
	ClientBurst *int `json:"client-burst,omitempty"`
	// maximum number of requests per second by the server to the Kubernetes API once the burst limit has been reached. (default 100)
	// +optional
	ClientQPS *int `json:"client-qps,omitempty"`
	// Velero args are settings to customize velero server arguments. Overrides values in other fields.
	// +optional
	Args *VeleroServerArgs `json:"args,omitempty"`
	// LoadAffinityConfig is the config for data path load affinity.
	// +optional
	LoadAffinityConfig []*LoadAffinity `json:"loadAffinity,omitempty"`
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
	// resourceAllocations defines the CPU, Memory and ephemeral-storage resource allocations for the Pod
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

// Below struct should be same as:
// https://github.com/openshift/velero/blob/584cf1148a746838ee67aa27e3e4e0ded1f5c069/pkg/nodeagent/node_agent.go#L52-L58

// LoadConcurrency is the config for data path load concurrency per node.
type LoadConcurrency struct {
	// GlobalConfig specifies the concurrency number to all nodes for which per-node config is not specified
	GlobalConfig int `json:"globalConfig,omitempty"`

	// PerNodeConfig specifies the concurrency number to nodes matched by rules
	PerNodeConfig []RuledConfigs `json:"perNodeConfig,omitempty"`
}

// Below struct should be same as:
// https://github.com/openshift/velero/blob/584cf1148a746838ee67aa27e3e4e0ded1f5c069/pkg/nodeagent/node_agent.go#L60-L63

// LoadAffinity is the config for data path load affinity.
// Used by the Node-Agent, that needs to match the DataMover and the RepositoryMaintenance pods.
type LoadAffinity struct {
	// NodeSelector specifies the label selector to match nodes
	// +optional
	NodeSelector metav1.LabelSelector `json:"nodeSelector,omitempty"`
}

// Below struct should be same as:
// https://github.com/openshift/velero/blob/584cf1148a746838ee67aa27e3e4e0ded1f5c069/pkg/nodeagent/node_agent.go#L65-L71

// RuledConfigs is the config for data path load concurrency per node.
type RuledConfigs struct {
	// NodeSelector specifies the label selector to match nodes
	NodeSelector metav1.LabelSelector `json:"nodeSelector"`

	// Number specifies the number value associated to the matched nodes
	Number int `json:"number"`
}

// Below struct should be same as:
// https://github.com/openshift/velero/blob/584cf1148a746838ee67aa27e3e4e0ded1f5c069/pkg/nodeagent/node_agent.go#L90-L105

// NodeAgentConfigMapSettings is the config for node-agent
type NodeAgentConfigMapSettings struct {
	// LoadConcurrency is the config for data path load concurrency per node.
	// +optional
	LoadConcurrency *LoadConcurrency `json:"loadConcurrency,omitempty"`
	// LoadAffinity is the config for data path load affinity.
	// +optional
	LoadAffinityConfig []*LoadAffinity `json:"loadAffinity,omitempty"`
	// BackupPVCConfig is the config for backupPVC (intermediate PVC) of snapshot data movement
	// +optional
	BackupPVCConfig map[string]nodeagent.BackupPVC `json:"backupPVC,omitempty"`
	// RestoreVCConfig is the config for restorePVC (intermediate PVC) of generic restore
	// +optional
	RestorePVCConfig *nodeagent.RestorePVC `json:"restorePVC,omitempty"`
	// PodResources is the resource config for various types of pods launched by node-agent, i.e., data mover pods.
	// +optional
	PodResources *kube.PodResources `json:"podResources,omitempty"`
}

// Velero nodeAgentServerConfig struct used in below struct:
// https://github.com/openshift/velero/blob/8c8a6cccd78b78bd797e40189b0b9bee46a97f9e/pkg/cmd/cli/nodeagent/server.go#L87-L92

// NodeAgentConfig is the configuration for node server
// Holds the configuration for the Node Agent Server.
type NodeAgentConfig struct {
	// Embedding NodeAgentCommonFields
	// +optional
	NodeAgentCommonFields `json:",inline"`
	// How long to wait for preparing a DataUpload/DataDownload. Default is 30 minutes.
	// +optional
	DataMoverPrepareTimeout *metav1.Duration `json:"dataMoverPrepareTimeout,omitempty"`
	// How long to wait for resource processes which are not covered by other specific timeout parameters. Default is 10 minutes.
	// +optional
	ResourceTimeout *metav1.Duration `json:"resourceTimeout,omitempty"`
	// The type of uploader to transfer the data of pod volumes, the supported values are 'restic' or 'kopia'
	// +kubebuilder:validation:Enum=restic;kopia
	// +kubebuilder:validation:Required
	UploaderType string `json:"uploaderType"`
	// Embedding NodeAgentConfigMapSettings
	// +optional
	NodeAgentConfigMapSettings `json:",inline"`
	// Embedding KopiaRepoOptions
	// +optional
	KopiaRepoOptions `json:",inline"`
}

type KopiaRepoOptions struct {
	// CacheLimitMB specifies the size limit(in MB) for the local data cache
	// +kubebuilder:validation:Minimum=0
	// +optional
	CacheLimitMB *int64 `json:"cacheLimitMB,omitempty"`
	// fullMaintenanceInterval determines the time between kopia full maintenance operations.
	// normalGC: 24 hours
	// fastGC: 12 hours
	// eagerGC: 6 hours
	// +kubebuilder:validation:Enum=normalGC;fastGC;eagerGC
	// +optional
	FullMaintenanceInterval FullMaintenanceInterval `json:"fullMaintenanceInterval,omitempty"`
}

type FullMaintenanceInterval string

const (
	FullMaintenanceIntervalNormalGC FullMaintenanceInterval = "normalGC"
	FullMaintenanceIntervalFastGC   FullMaintenanceInterval = "fastGC"
	FullMaintenanceIntervalEagerGC  FullMaintenanceInterval = "eagerGC"
)

// ResticConfig is the configuration for restic server
type ResticConfig struct {
	// Embedding NodeAgentCommonFields
	// +optional
	NodeAgentCommonFields `json:",inline"`
}

type RepositoryMaintenanceConfig struct {
	// LoadAffinity is the config for data path load affinity.
	// +optional
	LoadAffinityConfig []*LoadAffinity `json:"loadAffinity,omitempty"`

	// PodResources is the config for the CPU and memory resources setting.
	// +optional
	PodResources *kube.PodResources `json:"podResources,omitempty"`
}

// ApplicationConfig defines the configuration for the Data Protection Application
type ApplicationConfig struct {
	Velero *VeleroConfig `json:"velero,omitempty"`
	// (do not use warning) restic field is for backwards compatibility and
	// will be removed in the future. Use nodeAgent field instead
	// +optional
	Restic *ResticConfig `json:"restic,omitempty"`

	// NodeAgent is needed to allow selection between kopia or restic
	// +optional
	NodeAgent *NodeAgentConfig `json:"nodeAgent,omitempty"`

	// RepositoryMaintenance maps a BackupRepository identifier to its configuration.
	// Keys can be:
	//  - "global" : Applies to all repositories without specific config.
	//  - "<namespace>" : The namespace of the BackupRepository.
	//  - "<repository name>" : The specific BackupRepository name referencing the BSL.
	//  - "<repository type>" : Either "kopia" or "restic".
	// +optional
	RepositoryMaintenance map[string]RepositoryMaintenanceConfig `json:"repositoryMaintenance,omitempty"`
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

	// +optional
	Name   string                             `json:"name,omitempty"`
	Velero *velero.VolumeSnapshotLocationSpec `json:"velero"`
}

// We need to create enforcement structures for the BSL spec fields, because the Velero BSL spec
// is requiring fields like bucket, provider which are allowed to be empty for the enforcement in the DPA.

// ObjectStorageLocation defines the enforced values for the Velero ObjectStorageLocation
type ObjectStorageLocation struct {
	// Bucket is the bucket to use for object storage.
	// +optional
	Bucket string `json:"bucket,omitempty"`

	// Prefix is the path inside a bucket to use for Velero storage. Optional.
	// +optional
	Prefix string `json:"prefix,omitempty"`

	// CACert defines a CA bundle to use when verifying TLS connections to the provider.
	// +optional
	CACert []byte `json:"caCert,omitempty"`
}

// StorageType defines the enforced values for the Velero StorageType
type StorageType struct {
	// +optional
	// +nullable
	ObjectStorage *ObjectStorageLocation `json:"objectStorage,omitempty"`
}

// EnforceBackupStorageLocationSpec defines the enforced values for the Velero BackupStorageLocationSpec
type EnforceBackupStorageLocationSpec struct {
	// Provider is the provider of the backup storage.
	// +optional
	Provider string `json:"provider,omitempty"`

	// Config is for provider-specific configuration fields.
	// +optional
	Config map[string]string `json:"config,omitempty"`

	// Credential contains the credential information intended to be used with this location
	// +optional
	Credential *corev1.SecretKeySelector `json:"credential,omitempty"`

	StorageType `json:",inline"`

	// AccessMode defines the permissions for the backup storage location.
	// +optional
	AccessMode velero.BackupStorageLocationAccessMode `json:"accessMode,omitempty"`

	// BackupSyncPeriod defines how frequently to sync backup API objects from object storage. A value of 0 disables sync.
	// +optional
	// +nullable
	BackupSyncPeriod *metav1.Duration `json:"backupSyncPeriod,omitempty"`

	// ValidationFrequency defines how frequently to validate the corresponding object storage. A value of 0 disables validation.
	// +optional
	// +nullable
	ValidationFrequency *metav1.Duration `json:"validationFrequency,omitempty"`
}

type NonAdmin struct {
	// Enables non admin feature, by default is disabled
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// which bakup spec field values to enforce
	// +optional
	EnforceBackupSpec *velero.BackupSpec `json:"enforceBackupSpec,omitempty"`

	// which restore spec field values to enforce
	// +optional
	EnforceRestoreSpec *velero.RestoreSpec `json:"enforceRestoreSpec,omitempty"`

	// which backupstoragelocation spec field values to enforce
	// +optional
	EnforceBSLSpec *EnforceBackupStorageLocationSpec `json:"enforceBSLSpec,omitempty"`

	// RequireApprovalForBSL specifies whether cluster administrator approval is required
	// for creating Velero BackupStorageLocation (BSL) resources.
	// - If set to false, all NonAdminBackupStorageLocationApproval CRDs will be automatically approved,
	//   including those that were previously pending or rejected.
	// - If set to true, any existing BackupStorageLocation CRDs that lack the necessary approvals may be deleted,
	//   leaving the associated NonAdminBackup objects non-restorable until approval is granted.
	// Defaults to false
	// +optional
	RequireApprovalForBSL *bool `json:"requireApprovalForBSL,omitempty"`

	// GarbageCollectionPeriod defines how frequently to look for possible leftover non admin related objects in OADP namespace.
	// A value of 0 disables garbage collection.
	// By default 24h
	// +optional
	GarbageCollectionPeriod *metav1.Duration `json:"garbageCollectionPeriod,omitempty"`

	// BackupSyncPeriod specifies the interval at which backups from the OADP namespace are synchronized with non-admin namespaces.
	// A value of 0 disables sync.
	// By default 2m
	// +optional
	BackupSyncPeriod *metav1.Duration `json:"backupSyncPeriod,omitempty"`
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
	// defines configurations for data mover volume options for a storageClass
	// +optional
	VolumeOptionsForStorageClasses map[string]DataMoverVolumeOptions `json:"volumeOptionsForStorageClasses,omitempty"`
	// defines the parameters that can be specified for retention of datamover snapshots
	// +optional
	SnapshotRetainPolicy *RetainPolicy `json:"snapshotRetainPolicy,omitempty"`
	// schedule is a cronspec (https://en.wikipedia.org/wiki/Cron#Overview) that
	// can be used to schedule datamover(volsync) synchronization to occur at regular, time-based
	// intervals. For example, in order to enforce datamover SnapshotRetainPolicy at a regular interval you need to
	// specify this Schedule trigger as a cron expression, by default the trigger is a manual trigger. For more details
	// on Volsync triggers, refer: https://volsync.readthedocs.io/en/stable/usage/triggers.html
	//+kubebuilder:validation:Pattern=`^(\d+|\*)(/\d+)?(\s+(\d+|\*)(/\d+)?){4}$`
	//+optional
	Schedule string `json:"schedule,omitempty"`
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
	AccessMode corev1.PersistentVolumeAccessMode `json:"accessMode,omitempty"`
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
}

// Features defines the configuration for the DPA to enable the tech preview features
type Features struct {
	// (do not use warning) dataMover is for backwards compatibility and
	// will be removed in the future. Use Velero Built-in Data Mover instead
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
	//   - legacyAWSPluginImageFqin
	//   - openshiftPluginImageFqin
	//   - azurePluginImageFqin
	//   - gcpPluginImageFqin
	//   - resticRestoreImageFqin
	//   - kubevirtPluginImageFqin
	//   - hypershiftPluginImageFqin
	//   - nonAdminControllerImageFqin
	//   - operator-type
	//   - tech-preview-ack
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
	// which imagePullPolicy to use in all container images used by OADP.
	// By default, for images with sha256 or sha512 digest, OADP uses IfNotPresent and uses Always for all other images.
	// +optional
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	ImagePullPolicy *corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// nonAdmin defines the configuration for the DPA to enable backup and restore operations for non-admin users
	// +optional
	NonAdmin *NonAdmin `json:"nonAdmin,omitempty"`
	// The format for log output. Valid values are text, json. (default text)
	// +kubebuilder:validation:Enum=text;json
	// +kubebuilder:default=text
	// +optional
	LogFormat LogFormat `json:"logFormat,omitempty"`
}

// DataProtectionApplicationStatus defines the observed state of DataProtectionApplication
type DataProtectionApplicationStatus struct {
	// Conditions defines the observed state of DataProtectionApplication
	//+operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=dataprotectionapplications,shortName=dpa
// +kubebuilder:printcolumn:name="Reconciled",type="string",JSONPath=".status.conditions[?(@.type=='Reconciled')].status",description="DataProtectionApplication Reconciled Status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="DataProtectionApplication creation timestamp"

// DataProtectionApplication represents configuration to install a data protection
// application to safely backup and restore, perform disaster recovery and migrate
// Kubernetes cluster resources and persistent volumes.
type DataProtectionApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataProtectionApplicationSpec   `json:"spec,omitempty"`
	Status DataProtectionApplicationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DataProtectionApplicationList contains a list of DataProtectionApplication
type DataProtectionApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataProtectionApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataProtectionApplication{}, &DataProtectionApplicationList{})
}

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
		// Enable user to specify --fs-backup-timeout duration (OADP default 4h0m0s)
		fsBackupTimeout := "4h"
		if dpa.Spec.Configuration != nil {
			if dpa.Spec.Configuration.NodeAgent != nil && len(dpa.Spec.Configuration.NodeAgent.Timeout) > 0 {
				fsBackupTimeout = dpa.Spec.Configuration.NodeAgent.Timeout
			}
		}
		if pvOperationTimeout, err := time.ParseDuration(fsBackupTimeout); err == nil && dpa.Spec.Configuration.Velero.Args.PodVolumeOperationTimeout == nil {
			dpa.Spec.Configuration.Velero.Args.PodVolumeOperationTimeout = &pvOperationTimeout
		}
		if dpa.Spec.Configuration.Velero.Args.RestoreResourcePriorities == "" {
			dpa.Spec.Configuration.Velero.Args.RestoreResourcePriorities = common.DefaultRestoreResourcePriorities.String()
		}
	}

	dpa.Spec.Configuration.Velero.DefaultPlugins = common.RemoveDuplicateValues(dpa.Spec.Configuration.Velero.DefaultPlugins)
	dpa.Spec.Configuration.Velero.FeatureFlags = common.RemoveDuplicateValues(dpa.Spec.Configuration.Velero.FeatureFlags)

	// Auto correct nodeAffinity for the node agent, but only if the new schema is not used
	// The new schema will be used instead of the dpa.Spec.Configuration.NodeAgent.PodConfig
	// There is need to translate map of labels to a LabelSelector with MatchLabels
	if dpa.Spec.Configuration.NodeAgent != nil && dpa.Spec.Configuration.NodeAgent.PodConfig != nil && dpa.Spec.Configuration.NodeAgent.PodConfig.NodeSelector != nil {
		// Only modify if LoadAffinityConfig is not already set
		if dpa.Spec.Configuration.NodeAgent.LoadAffinityConfig == nil {
			// Convert the NodeSelector map to a LabelSelector with MatchLabels
			dpa.Spec.Configuration.NodeAgent.LoadAffinityConfig = []*LoadAffinity{
				{
					NodeSelector: metav1.LabelSelector{
						MatchLabels: dpa.Spec.Configuration.NodeAgent.PodConfig.NodeSelector,
					},
				},
			}
		}
	}
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

package server

import (
	"time"

	"github.com/vmware-tanzu/velero/pkg/client"
	vServer "github.com/vmware-tanzu/velero/pkg/cmd/server"
)

// This package is used to store ServerConfig struct and note information about flags for velero server and how they are set.
// The options you can set in `velero server` is a combination of ServerConfig, featureFlagSet

// ServerConfig holds almost all the configuration for the Velero server.
// https://github.com/openshift/velero/blob/dd02df5cd5751263fce6d1ebd48ea11423b0cd16/pkg/cmd/server/server.go#L112-L129
// +kubebuilder:object:generate=true
type ServerConfig struct {
	// pluginDir will be fixed to /plugins
	// pluginDir

	// The address to expose prometheus metrics
	// +optional
	MetricsAddress string `json:"metrics-address,omitempty"`
	// defaultBackupLocation will be defined outside of server config in DataProtectionApplication
	// defaultBackupLocation                        string

	// How often to ensure all Velero backups in object storage exist as Backup API objects in the cluster. This is the default sync period if none is explicitly specified for a backup storage location.
	// +optional
	BackupSyncPeriod *time.Duration `json:"backup-sync-period,omitempty"`
	// How long backups/restores of pod volumes should be allowed to run before timing out.
	// +optional
	PodVolumeOperationTimeout *time.Duration `json:"restic-timeout,omitempty"`
	// How long to wait on persistent volumes and namespaces to terminate during a restore before timing out.
	// +optional
	ResourceTerminatingTimeout *time.Duration `json:"terminating-resource-timeout,omitempty"`
	// default 720h0m0s
	// +optional
	DefaultBackupTTL *time.Duration `json:"default-backup-ttl,omitempty"`
	// How often to verify if the storage is valid. Optional. Set this to `0s` to disable sync. Default 1 minute.
	// +optional
	StoreValidationFrequency *time.Duration `json:"store-validation-frequency,omitempty"`
	// Desired order of resource restores, the priority list contains two parts which are split by "-" element. The resources before "-" element are restored first as high priorities, the resources after "-" element are restored last as low priorities, and any resource not in the list will be restored alphabetically between the high and low priorities. (default securitycontextconstraints,customresourcedefinitions,namespaces,storageclasses,volumesnapshotbackups.datamover.oadp.openshift.io,volumesnapshotclass.snapshot.storage.k8s.io,volumesnapshotcontents.snapshot.storage.k8s.io,volumesnapshots.snapshot.storage.k8s.io,persistentvolumes,persistentvolumeclaims,serviceaccounts,secrets,configmaps,limitranges,pods,replicasets.apps,clusterclasses.cluster.x-k8s.io,endpoints,services,-,clusterbootstraps.run.tanzu.vmware.com,clusters.cluster.x-k8s.io,clusterresourcesets.addons.cluster.x-k8s.io)
	// +optional
	RestoreResourcePriorities string `json:"restore-resource-priorities,omitempty"`
	// defaultVolumeSnapshotLocations will be defined outside of server config in DataProtectionApplication
	// defaultVolumeSnapshotLocations                                          map[string]string

	// DEPRECATED: this flag will be removed in v2.0. Use read-only backup storage locations instead.
	// +optional
	// RestoreOnly *bool `json:"restore-only,omitempty"`

	// List of controllers to disable on startup. Valid values are backup,backup-deletion,backup-sync,download-request,gc,restic-repo,restore,schedule,server-status-request
	// +kubebuilder:validation:Enum=backup;backup-deletion;backup-storage-location;backup-sync;download-request;gc;pod-volume-backup;pod-volume-restore;restic-repo;restore;schedule;server-status-request
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
	// The format for log output. Valid values are text, json. (default text)
	// +kubebuilder:validation:Enum=text;json
	// +optional
	FormatFlag string `json:"log-format,omitempty"`
	// default 168h0m0s
	// +optional
	DefaultResticMaintenanceFrequency *time.Duration `json:"default-restic-prune-frequency,omitempty"`
	// How long to wait by default before backups can be garbage collected. (default 720h0m0s)
	// +optional
	GarbageCollectionFrequency *time.Duration `json:"garbage-collection-frequency,omitempty"`
	// Backup all volumes with restic by default.
	// +optional
	DefaultVolumesToRestic *bool `json:"default-volumes-to-restic,omitempty"`
}

var VeleroServerCommand = vServer.NewCommand(client.NewFactory("velero-server", client.VeleroConfig{})).Flags()

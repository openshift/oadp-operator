package server

import (
	"time"

	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/restore"
	"github.com/vmware-tanzu/velero/pkg/util/logging"
)

// This package is used to store ServerConfig struct and note information about flags for velero server and how they are set.
// The options you can set in `velero server` is a combination of ServerConfig, featureFlagSet

// ServerConfig holds all the configuration for the Velero server.
// https://github.com/openshift/velero/blob/dd02df5cd5751263fce6d1ebd48ea11423b0cd16/pkg/cmd/server/server.go#L112-L129
type ServerConfig struct {
	// TODO(2.0) Deprecate defaultBackupLocation
	pluginDir, metricsAddress, defaultBackupLocation                        string
	backupSyncPeriod, podVolumeOperationTimeout, resourceTerminatingTimeout time.Duration
	defaultBackupTTL, storeValidationFrequency, defaultCSISnapshotTimeout   time.Duration
	restoreResourcePriorities                                               restore.Priorities
	defaultVolumeSnapshotLocations                                          map[string]string
	restoreOnly                                                             bool
	disabledControllers                                                     []string
	clientQPS                                                               float32
	clientBurst                                                             int
	clientPageSize                                                          int
	profilerAddress                                                         string
	formatFlag                                                              *logging.FormatFlag
	defaultResticMaintenanceFrequency                                       time.Duration
	garbageCollectionFrequency                                              time.Duration
	defaultVolumesToRestic                                                  bool
}

// featureFlags values are located in https://github.com/openshift/velero/blob/8e4f88db682f322c761e1f6c381cedeab51e5760/pkg/apis/velero/v1/constants.go#L40-L52

type VeleroFeatureFlag string

const (
	EnableCSI              VeleroFeatureFlag = v1.CSIFeatureFlag
	EnableAPIGroupVersions VeleroFeatureFlag = v1.APIGroupVersionsFeatureFlag
	EnableUploadProgress   VeleroFeatureFlag = v1.UploadProgressFeatureFlag
)

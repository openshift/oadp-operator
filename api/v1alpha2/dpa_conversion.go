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

package v1alpha2

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/openshift/oadp-operator/api/v1alpha1"
)

// ConvertTo converts this v1alpha2 DataProtectionApplication to the hub version (v1alpha1).
//
//nolint:unparam // error return is required by the conversion.Convertible interface
func (src *DataProtectionApplication) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha1.DataProtectionApplication)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	convertSpecToHub(&src.Spec, &dst.Spec)

	// Status
	dst.Status.Conditions = src.Status.Conditions

	return nil
}

// ConvertFrom converts the hub version (v1alpha1) to this v1alpha2 DataProtectionApplication.
//
//nolint:unparam // error return is required by the conversion.Convertible interface
func (dst *DataProtectionApplication) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha1.DataProtectionApplication)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	convertSpecFromHub(&src.Spec, &dst.Spec)

	// Status
	dst.Status.Conditions = src.Status.Conditions

	return nil
}

// convertSpecToHub converts v1alpha2 spec to v1alpha1 spec (spoke -> hub).
func convertSpecToHub(src *DataProtectionApplicationSpec, dst *v1alpha1.DataProtectionApplicationSpec) {
	// BackupLocations
	if src.BackupLocations != nil {
		dst.BackupLocations = make([]v1alpha1.BackupLocation, len(src.BackupLocations))
		for i, bl := range src.BackupLocations {
			dst.BackupLocations[i].Name = bl.Name
			dst.BackupLocations[i].Velero = bl.Velero
			if bl.CloudStorage != nil {
				dst.BackupLocations[i].CloudStorage = &v1alpha1.CloudStorageLocation{
					CloudStorageRef:  bl.CloudStorage.CloudStorageRef,
					Config:           bl.CloudStorage.Config,
					Credential:       bl.CloudStorage.Credential,
					Default:          bl.CloudStorage.Default,
					BackupSyncPeriod: bl.CloudStorage.BackupSyncPeriod,
					Prefix:           bl.CloudStorage.Prefix,
					CACert:           bl.CloudStorage.CACert,
				}
			}
		}
	}

	// SnapshotLocations
	if src.SnapshotLocations != nil {
		dst.SnapshotLocations = make([]v1alpha1.SnapshotLocation, len(src.SnapshotLocations))
		for i, sl := range src.SnapshotLocations {
			dst.SnapshotLocations[i].Name = sl.Name
			dst.SnapshotLocations[i].Velero = sl.Velero
		}
	}

	// Simple fields
	dst.UnsupportedOverrides = convertUnsupportedOverridesToHub(src.UnsupportedOverrides)
	dst.PodAnnotations = src.PodAnnotations
	dst.ResourceLabels = src.ResourceLabels
	dst.ResourceAnnotations = src.ResourceAnnotations
	dst.PodDnsPolicy = src.PodDnsPolicy
	dst.PodDnsConfig = src.PodDnsConfig
	dst.BackupImages = src.BackupImages
	dst.ImagePullPolicy = src.ImagePullPolicy
	dst.LogFormat = v1alpha1.LogFormat(src.LogFormat)

	// Features
	if src.Features != nil {
		dst.Features = &v1alpha1.Features{}
		if src.Features.DataMover != nil {
			dst.Features.DataMover = convertDataMoverToHub(src.Features.DataMover)
		}
	}

	// NonAdmin
	if src.NonAdmin != nil {
		dst.NonAdmin = convertNonAdminToHub(src.NonAdmin)
	}

	// VMFileRestore
	if src.VMFileRestore != nil {
		dst.VMFileRestore = &v1alpha1.VMFileRestore{
			Enable:    src.VMFileRestore.Enable,
			Resources: src.VMFileRestore.Resources,
		}
	}

	// Configuration
	if src.Configuration != nil {
		dst.Configuration = &v1alpha1.ApplicationConfig{}
		convertAppConfigToHub(src.Configuration, dst.Configuration)
	}
}

// convertSpecFromHub converts v1alpha1 spec to v1alpha2 spec (hub -> spoke).
func convertSpecFromHub(src *v1alpha1.DataProtectionApplicationSpec, dst *DataProtectionApplicationSpec) {
	// BackupLocations
	if src.BackupLocations != nil {
		dst.BackupLocations = make([]BackupLocation, len(src.BackupLocations))
		for i, bl := range src.BackupLocations {
			dst.BackupLocations[i].Name = bl.Name
			dst.BackupLocations[i].Velero = bl.Velero
			if bl.CloudStorage != nil {
				dst.BackupLocations[i].CloudStorage = &CloudStorageLocation{
					CloudStorageRef:  bl.CloudStorage.CloudStorageRef,
					Config:           bl.CloudStorage.Config,
					Credential:       bl.CloudStorage.Credential,
					Default:          bl.CloudStorage.Default,
					BackupSyncPeriod: bl.CloudStorage.BackupSyncPeriod,
					Prefix:           bl.CloudStorage.Prefix,
					CACert:           bl.CloudStorage.CACert,
				}
			}
		}
	}

	// SnapshotLocations
	if src.SnapshotLocations != nil {
		dst.SnapshotLocations = make([]SnapshotLocation, len(src.SnapshotLocations))
		for i, sl := range src.SnapshotLocations {
			dst.SnapshotLocations[i].Name = sl.Name
			dst.SnapshotLocations[i].Velero = sl.Velero
		}
	}

	// Simple fields
	dst.UnsupportedOverrides = convertUnsupportedOverridesFromHub(src.UnsupportedOverrides)
	dst.PodAnnotations = src.PodAnnotations
	dst.ResourceLabels = src.ResourceLabels
	dst.ResourceAnnotations = src.ResourceAnnotations
	dst.PodDnsPolicy = src.PodDnsPolicy
	dst.PodDnsConfig = src.PodDnsConfig
	dst.BackupImages = src.BackupImages
	dst.ImagePullPolicy = src.ImagePullPolicy
	dst.LogFormat = LogFormat(src.LogFormat)

	// Features
	if src.Features != nil {
		dst.Features = &Features{}
		if src.Features.DataMover != nil {
			dst.Features.DataMover = convertDataMoverFromHub(src.Features.DataMover)
		}
	}

	// NonAdmin
	if src.NonAdmin != nil {
		dst.NonAdmin = convertNonAdminFromHub(src.NonAdmin)
	}

	// VMFileRestore
	if src.VMFileRestore != nil {
		dst.VMFileRestore = &VMFileRestore{
			Enable:    src.VMFileRestore.Enable,
			Resources: src.VMFileRestore.Resources,
		}
	}

	// Configuration
	if src.Configuration != nil {
		dst.Configuration = &ApplicationConfig{}
		convertAppConfigFromHub(src.Configuration, dst.Configuration)
	}
}

// convertAppConfigToHub converts v1alpha2 ApplicationConfig to v1alpha1.
func convertAppConfigToHub(src *ApplicationConfig, dst *v1alpha1.ApplicationConfig) {
	if src.Velero != nil {
		dst.Velero = &v1alpha1.VeleroConfig{
			FeatureFlags:                    src.Velero.FeatureFlags,
			DefaultPlugins:                  convertDefaultPluginsToHub(src.Velero.DefaultPlugins),
			CustomPlugins:                   convertCustomPluginsToHub(src.Velero.CustomPlugins),
			RestoreResourcesVersionPriority: src.Velero.RestoreResourcesVersionPriority,
			NoDefaultBackupLocation:         src.Velero.NoDefaultBackupLocation,
			LogLevel:                        src.Velero.LogLevel,
			ItemOperationSyncFrequency:      src.Velero.ItemOperationSyncFrequency,
			DefaultItemOperationTimeout:     src.Velero.DefaultItemOperationTimeout,
			DefaultVolumesToFSBackup:        src.Velero.DefaultVolumesToFSBackup,
			DisableFsBackup:                 src.Velero.DisableFsBackup,
			DefaultSnapshotMoveData:         src.Velero.DefaultSnapshotMoveData,
			DisableInformerCache:            src.Velero.DisableInformerCache,
			ItemBlockWorkerCount:            src.Velero.ItemBlockWorkerCount,
			ConcurrentBackups:               src.Velero.ConcurrentBackups,
			ResourceTimeout:                 src.Velero.ResourceTimeout,
			ClientBurst:                     src.Velero.ClientBurst,
			ClientQPS:                       src.Velero.ClientQPS,
		}
		if src.Velero.PodConfig != nil {
			dst.Velero.PodConfig = convertPodConfigToHub(src.Velero.PodConfig)
		}
		if src.Velero.Args != nil {
			dst.Velero.Args = convertServerArgsToHub(src.Velero.Args)
		}
		if src.Velero.LoadAffinityConfig != nil {
			dst.Velero.LoadAffinityConfig = convertLoadAffinityToHub(src.Velero.LoadAffinityConfig)
		}
	}

	if src.Restic != nil {
		dst.Restic = &v1alpha1.ResticConfig{
			NodeAgentCommonFields: convertNodeAgentCommonFieldsToHub(src.Restic.NodeAgentCommonFields),
		}
	}

	if src.NodeAgent != nil {
		dst.NodeAgent = convertNodeAgentConfigToHub(src.NodeAgent)
	}

	if src.RepositoryMaintenance != nil {
		dst.RepositoryMaintenance = make(map[string]v1alpha1.RepositoryMaintenanceConfig, len(src.RepositoryMaintenance))
		for k, v := range src.RepositoryMaintenance {
			dst.RepositoryMaintenance[k] = v1alpha1.RepositoryMaintenanceConfig{
				LoadAffinityConfig: convertLoadAffinityToHub(v.LoadAffinityConfig),
				PodResources:       v.PodResources,
			}
		}
	}
}

// convertAppConfigFromHub converts v1alpha1 ApplicationConfig to v1alpha2.
func convertAppConfigFromHub(src *v1alpha1.ApplicationConfig, dst *ApplicationConfig) {
	if src.Velero != nil {
		dst.Velero = &VeleroConfig{
			FeatureFlags:                    src.Velero.FeatureFlags,
			DefaultPlugins:                  convertDefaultPluginsFromHub(src.Velero.DefaultPlugins),
			CustomPlugins:                   convertCustomPluginsFromHub(src.Velero.CustomPlugins),
			RestoreResourcesVersionPriority: src.Velero.RestoreResourcesVersionPriority,
			NoDefaultBackupLocation:         src.Velero.NoDefaultBackupLocation,
			LogLevel:                        src.Velero.LogLevel,
			ItemOperationSyncFrequency:      src.Velero.ItemOperationSyncFrequency,
			DefaultItemOperationTimeout:     src.Velero.DefaultItemOperationTimeout,
			DefaultVolumesToFSBackup:        src.Velero.DefaultVolumesToFSBackup,
			DisableFsBackup:                 src.Velero.DisableFsBackup,
			DefaultSnapshotMoveData:         src.Velero.DefaultSnapshotMoveData,
			DisableInformerCache:            src.Velero.DisableInformerCache,
			ItemBlockWorkerCount:            src.Velero.ItemBlockWorkerCount,
			ConcurrentBackups:               src.Velero.ConcurrentBackups,
			ResourceTimeout:                 src.Velero.ResourceTimeout,
			ClientBurst:                     src.Velero.ClientBurst,
			ClientQPS:                       src.Velero.ClientQPS,
		}
		if src.Velero.PodConfig != nil {
			dst.Velero.PodConfig = convertPodConfigFromHub(src.Velero.PodConfig)
		}
		if src.Velero.Args != nil {
			dst.Velero.Args = convertServerArgsFromHub(src.Velero.Args)
		}
		if src.Velero.LoadAffinityConfig != nil {
			dst.Velero.LoadAffinityConfig = convertLoadAffinityFromHub(src.Velero.LoadAffinityConfig)
		}
	}

	if src.Restic != nil {
		dst.Restic = &ResticConfig{
			NodeAgentCommonFields: convertNodeAgentCommonFieldsFromHub(src.Restic.NodeAgentCommonFields),
		}
	}

	if src.NodeAgent != nil {
		dst.NodeAgent = convertNodeAgentConfigFromHub(src.NodeAgent)
	}

	if src.RepositoryMaintenance != nil {
		dst.RepositoryMaintenance = make(map[string]RepositoryMaintenanceConfig, len(src.RepositoryMaintenance))
		for k, v := range src.RepositoryMaintenance {
			dst.RepositoryMaintenance[k] = RepositoryMaintenanceConfig{
				LoadAffinityConfig: convertLoadAffinityFromHub(v.LoadAffinityConfig),
				PodResources:       v.PodResources,
			}
		}
	}
}

// convertServerArgsToHub converts v1alpha2 VeleroServerArgs to v1alpha1.
// This is where the key *metav1.Duration -> *time.Duration conversion happens.
func convertServerArgsToHub(src *VeleroServerArgs) *v1alpha1.VeleroServerArgs {
	dst := &v1alpha1.VeleroServerArgs{}

	// ServerFlags — non-duration fields
	dst.MetricsAddress = src.MetricsAddress
	dst.RestoreResourcePriorities = src.RestoreResourcePriorities
	dst.DisabledControllers = src.DisabledControllers
	dst.ClientQPS = src.ClientQPS
	dst.ClientBurst = src.ClientBurst
	dst.ClientPageSize = src.ClientPageSize
	dst.ProfilerAddress = src.ProfilerAddress
	dst.FormatFlag = src.FormatFlag
	dst.DefaultVolumesToFsBackup = src.DefaultVolumesToFsBackup
	dst.MaxConcurrentK8SConnections = src.MaxConcurrentK8SConnections

	// ServerFlags — duration fields: *metav1.Duration -> *time.Duration
	dst.BackupSyncPeriod = metav1DurationToTimeDuration(src.BackupSyncPeriod)
	dst.PodVolumeOperationTimeout = metav1DurationToTimeDuration(src.PodVolumeOperationTimeout)
	dst.ResourceTerminatingTimeout = metav1DurationToTimeDuration(src.ResourceTerminatingTimeout)
	dst.DefaultBackupTTL = metav1DurationToTimeDuration(src.DefaultBackupTTL)
	dst.StoreValidationFrequency = metav1DurationToTimeDuration(src.StoreValidationFrequency)
	dst.ItemOperationSyncFrequency = metav1DurationToTimeDuration(src.ItemOperationSyncFrequency)
	dst.RepoMaintenanceFrequency = metav1DurationToTimeDuration(src.RepoMaintenanceFrequency)
	dst.GarbageCollectionFrequency = metav1DurationToTimeDuration(src.GarbageCollectionFrequency)
	dst.DefaultItemOperationTimeout = metav1DurationToTimeDuration(src.DefaultItemOperationTimeout)
	dst.ResourceTimeout = metav1DurationToTimeDuration(src.ResourceTimeout)

	// GlobalFlags
	dst.Colorized = src.Colorized

	// LoggingFlags
	dst.ToStderr = src.ToStderr
	dst.AlsoToStderr = src.AlsoToStderr
	dst.StderrThreshold = src.StderrThreshold
	dst.TraceLocation = src.TraceLocation
	dst.Vmodule = src.Vmodule
	dst.Verbosity = src.Verbosity
	dst.LogDir = src.LogDir
	dst.LogFile = src.LogFile
	dst.LogFileMaxSizeMB = src.LogFileMaxSizeMB
	dst.SkipHeaders = src.SkipHeaders
	dst.SkipLogHeaders = src.SkipLogHeaders
	dst.AddDirHeader = src.AddDirHeader
	dst.OneOutput = src.OneOutput

	return dst
}

// convertServerArgsFromHub converts v1alpha1 VeleroServerArgs to v1alpha2.
// This is where the key *time.Duration -> *metav1.Duration conversion happens.
func convertServerArgsFromHub(src *v1alpha1.VeleroServerArgs) *VeleroServerArgs {
	dst := &VeleroServerArgs{}

	// ServerFlags — non-duration fields
	dst.MetricsAddress = src.MetricsAddress
	dst.RestoreResourcePriorities = src.RestoreResourcePriorities
	dst.DisabledControllers = src.DisabledControllers
	dst.ClientQPS = src.ClientQPS
	dst.ClientBurst = src.ClientBurst
	dst.ClientPageSize = src.ClientPageSize
	dst.ProfilerAddress = src.ProfilerAddress
	dst.FormatFlag = src.FormatFlag
	dst.DefaultVolumesToFsBackup = src.DefaultVolumesToFsBackup
	dst.MaxConcurrentK8SConnections = src.MaxConcurrentK8SConnections

	// ServerFlags — duration fields: *time.Duration -> *metav1.Duration
	dst.BackupSyncPeriod = timeDurationToMetav1Duration(src.BackupSyncPeriod)
	dst.PodVolumeOperationTimeout = timeDurationToMetav1Duration(src.PodVolumeOperationTimeout)
	dst.ResourceTerminatingTimeout = timeDurationToMetav1Duration(src.ResourceTerminatingTimeout)
	dst.DefaultBackupTTL = timeDurationToMetav1Duration(src.DefaultBackupTTL)
	dst.StoreValidationFrequency = timeDurationToMetav1Duration(src.StoreValidationFrequency)
	dst.ItemOperationSyncFrequency = timeDurationToMetav1Duration(src.ItemOperationSyncFrequency)
	dst.RepoMaintenanceFrequency = timeDurationToMetav1Duration(src.RepoMaintenanceFrequency)
	dst.GarbageCollectionFrequency = timeDurationToMetav1Duration(src.GarbageCollectionFrequency)
	dst.DefaultItemOperationTimeout = timeDurationToMetav1Duration(src.DefaultItemOperationTimeout)
	dst.ResourceTimeout = timeDurationToMetav1Duration(src.ResourceTimeout)

	// GlobalFlags
	dst.Colorized = src.Colorized

	// LoggingFlags
	dst.ToStderr = src.ToStderr
	dst.AlsoToStderr = src.AlsoToStderr
	dst.StderrThreshold = src.StderrThreshold
	dst.TraceLocation = src.TraceLocation
	dst.Vmodule = src.Vmodule
	dst.Verbosity = src.Verbosity
	dst.LogDir = src.LogDir
	dst.LogFile = src.LogFile
	dst.LogFileMaxSizeMB = src.LogFileMaxSizeMB
	dst.SkipHeaders = src.SkipHeaders
	dst.SkipLogHeaders = src.SkipLogHeaders
	dst.AddDirHeader = src.AddDirHeader
	dst.OneOutput = src.OneOutput

	return dst
}

// Duration conversion helpers

func metav1DurationToTimeDuration(d *metav1.Duration) *time.Duration {
	if d == nil {
		return nil
	}
	dur := d.Duration
	return &dur
}

func timeDurationToMetav1Duration(d *time.Duration) *metav1.Duration {
	if d == nil {
		return nil
	}
	return &metav1.Duration{Duration: *d}
}

// Type conversion helpers for types that are structurally identical but in different packages.

func convertDefaultPluginsToHub(src []DefaultPlugin) []v1alpha1.DefaultPlugin {
	if src == nil {
		return nil
	}
	dst := make([]v1alpha1.DefaultPlugin, len(src))
	for i, p := range src {
		dst[i] = v1alpha1.DefaultPlugin(p)
	}
	return dst
}

func convertDefaultPluginsFromHub(src []v1alpha1.DefaultPlugin) []DefaultPlugin {
	if src == nil {
		return nil
	}
	dst := make([]DefaultPlugin, len(src))
	for i, p := range src {
		dst[i] = DefaultPlugin(p)
	}
	return dst
}

func convertCustomPluginsToHub(src []CustomPlugin) []v1alpha1.CustomPlugin {
	if src == nil {
		return nil
	}
	dst := make([]v1alpha1.CustomPlugin, len(src))
	for i, p := range src {
		dst[i] = v1alpha1.CustomPlugin{Name: p.Name, Image: p.Image}
	}
	return dst
}

func convertCustomPluginsFromHub(src []v1alpha1.CustomPlugin) []CustomPlugin {
	if src == nil {
		return nil
	}
	dst := make([]CustomPlugin, len(src))
	for i, p := range src {
		dst[i] = CustomPlugin{Name: p.Name, Image: p.Image}
	}
	return dst
}

func convertUnsupportedOverridesToHub(src map[UnsupportedImageKey]string) map[v1alpha1.UnsupportedImageKey]string {
	if src == nil {
		return nil
	}
	dst := make(map[v1alpha1.UnsupportedImageKey]string, len(src))
	for k, v := range src {
		dst[v1alpha1.UnsupportedImageKey(k)] = v
	}
	return dst
}

func convertUnsupportedOverridesFromHub(src map[v1alpha1.UnsupportedImageKey]string) map[UnsupportedImageKey]string {
	if src == nil {
		return nil
	}
	dst := make(map[UnsupportedImageKey]string, len(src))
	for k, v := range src {
		dst[UnsupportedImageKey(k)] = v
	}
	return dst
}

func convertPodConfigToHub(src *PodConfig) *v1alpha1.PodConfig {
	if src == nil {
		return nil
	}
	return &v1alpha1.PodConfig{
		Labels:              src.Labels,
		Annotations:         src.Annotations,
		NodeSelector:        src.NodeSelector,
		Tolerations:         src.Tolerations,
		ResourceAllocations: src.ResourceAllocations,
		Env:                 src.Env,
	}
}

func convertPodConfigFromHub(src *v1alpha1.PodConfig) *PodConfig {
	if src == nil {
		return nil
	}
	return &PodConfig{
		Labels:              src.Labels,
		Annotations:         src.Annotations,
		NodeSelector:        src.NodeSelector,
		Tolerations:         src.Tolerations,
		ResourceAllocations: src.ResourceAllocations,
		Env:                 src.Env,
	}
}

func convertLoadAffinityToHub(src []*LoadAffinity) []*v1alpha1.LoadAffinity {
	if src == nil {
		return nil
	}
	dst := make([]*v1alpha1.LoadAffinity, len(src))
	for i, la := range src {
		if la != nil {
			dst[i] = &v1alpha1.LoadAffinity{NodeSelector: la.NodeSelector}
		}
	}
	return dst
}

func convertLoadAffinityFromHub(src []*v1alpha1.LoadAffinity) []*LoadAffinity {
	if src == nil {
		return nil
	}
	dst := make([]*LoadAffinity, len(src))
	for i, la := range src {
		if la != nil {
			dst[i] = &LoadAffinity{NodeSelector: la.NodeSelector}
		}
	}
	return dst
}

func convertNodeAgentCommonFieldsToHub(src NodeAgentCommonFields) v1alpha1.NodeAgentCommonFields {
	return v1alpha1.NodeAgentCommonFields{
		Enable:             src.Enable,
		SupplementalGroups: src.SupplementalGroups,
		Timeout:            src.Timeout,
		PodConfig:          convertPodConfigToHub(src.PodConfig),
	}
}

func convertNodeAgentCommonFieldsFromHub(src v1alpha1.NodeAgentCommonFields) NodeAgentCommonFields {
	return NodeAgentCommonFields{
		Enable:             src.Enable,
		SupplementalGroups: src.SupplementalGroups,
		Timeout:            src.Timeout,
		PodConfig:          convertPodConfigFromHub(src.PodConfig),
	}
}

func convertNodeAgentConfigToHub(src *NodeAgentConfig) *v1alpha1.NodeAgentConfig {
	if src == nil {
		return nil
	}
	dst := &v1alpha1.NodeAgentConfig{
		NodeAgentCommonFields:   convertNodeAgentCommonFieldsToHub(src.NodeAgentCommonFields),
		DataMoverPrepareTimeout: src.DataMoverPrepareTimeout,
		ResourceTimeout:         src.ResourceTimeout,
		UploaderType:            src.UploaderType,
		KopiaRepoOptions: v1alpha1.KopiaRepoOptions{
			CacheLimitMB:            src.CacheLimitMB,
			FullMaintenanceInterval: v1alpha1.FullMaintenanceInterval(src.FullMaintenanceInterval),
		},
	}
	convertNodeAgentConfigMapSettingsToHub(&src.NodeAgentConfigMapSettings, &dst.NodeAgentConfigMapSettings)
	return dst
}

func convertNodeAgentConfigFromHub(src *v1alpha1.NodeAgentConfig) *NodeAgentConfig {
	if src == nil {
		return nil
	}
	dst := &NodeAgentConfig{
		NodeAgentCommonFields:   convertNodeAgentCommonFieldsFromHub(src.NodeAgentCommonFields),
		DataMoverPrepareTimeout: src.DataMoverPrepareTimeout,
		ResourceTimeout:         src.ResourceTimeout,
		UploaderType:            src.UploaderType,
		KopiaRepoOptions: KopiaRepoOptions{
			CacheLimitMB:            src.CacheLimitMB,
			FullMaintenanceInterval: FullMaintenanceInterval(src.FullMaintenanceInterval),
		},
	}
	convertNodeAgentConfigMapSettingsFromHub(&src.NodeAgentConfigMapSettings, &dst.NodeAgentConfigMapSettings)
	return dst
}

func convertNodeAgentConfigMapSettingsToHub(src *NodeAgentConfigMapSettings, dst *v1alpha1.NodeAgentConfigMapSettings) {
	if src.LoadConcurrency != nil {
		dst.LoadConcurrency = &v1alpha1.LoadConcurrency{
			GlobalConfig:       src.LoadConcurrency.GlobalConfig,
			PrepareQueueLength: src.LoadConcurrency.PrepareQueueLength,
		}
		if src.LoadConcurrency.PerNodeConfig != nil {
			dst.LoadConcurrency.PerNodeConfig = make([]v1alpha1.RuledConfigs, len(src.LoadConcurrency.PerNodeConfig))
			for i, rc := range src.LoadConcurrency.PerNodeConfig {
				dst.LoadConcurrency.PerNodeConfig[i] = v1alpha1.RuledConfigs{
					NodeSelector: rc.NodeSelector,
					Number:       rc.Number,
				}
			}
		}
	}
	dst.LoadAffinityConfig = convertLoadAffinityToHub(src.LoadAffinityConfig)
	dst.BackupPVCConfig = src.BackupPVCConfig
	dst.RestorePVCConfig = src.RestorePVCConfig
	dst.PodResources = src.PodResources
	dst.CachePVCConfig = src.CachePVCConfig
}

func convertNodeAgentConfigMapSettingsFromHub(src *v1alpha1.NodeAgentConfigMapSettings, dst *NodeAgentConfigMapSettings) {
	if src.LoadConcurrency != nil {
		dst.LoadConcurrency = &LoadConcurrency{
			GlobalConfig:       src.LoadConcurrency.GlobalConfig,
			PrepareQueueLength: src.LoadConcurrency.PrepareQueueLength,
		}
		if src.LoadConcurrency.PerNodeConfig != nil {
			dst.LoadConcurrency.PerNodeConfig = make([]RuledConfigs, len(src.LoadConcurrency.PerNodeConfig))
			for i, rc := range src.LoadConcurrency.PerNodeConfig {
				dst.LoadConcurrency.PerNodeConfig[i] = RuledConfigs{
					NodeSelector: rc.NodeSelector,
					Number:       rc.Number,
				}
			}
		}
	}
	dst.LoadAffinityConfig = convertLoadAffinityFromHub(src.LoadAffinityConfig)
	dst.BackupPVCConfig = src.BackupPVCConfig
	dst.RestorePVCConfig = src.RestorePVCConfig
	dst.PodResources = src.PodResources
	dst.CachePVCConfig = src.CachePVCConfig
}

func convertNonAdminToHub(src *NonAdmin) *v1alpha1.NonAdmin {
	dst := &v1alpha1.NonAdmin{
		Enable:                  src.Enable,
		EnforceBackupSpec:       src.EnforceBackupSpec,
		EnforceRestoreSpec:      src.EnforceRestoreSpec,
		RequireApprovalForBSL:   src.RequireApprovalForBSL,
		GarbageCollectionPeriod: src.GarbageCollectionPeriod,
		BackupSyncPeriod:        src.BackupSyncPeriod,
	}
	if src.EnforceBSLSpec != nil {
		dst.EnforceBSLSpec = &v1alpha1.EnforceBackupStorageLocationSpec{
			Provider:            src.EnforceBSLSpec.Provider,
			Config:              src.EnforceBSLSpec.Config,
			Credential:          src.EnforceBSLSpec.Credential,
			AccessMode:          src.EnforceBSLSpec.AccessMode,
			BackupSyncPeriod:    src.EnforceBSLSpec.BackupSyncPeriod,
			ValidationFrequency: src.EnforceBSLSpec.ValidationFrequency,
		}
		if src.EnforceBSLSpec.ObjectStorage != nil {
			dst.EnforceBSLSpec.ObjectStorage = &v1alpha1.ObjectStorageLocation{
				Bucket: src.EnforceBSLSpec.ObjectStorage.Bucket,
				Prefix: src.EnforceBSLSpec.ObjectStorage.Prefix,
				CACert: src.EnforceBSLSpec.ObjectStorage.CACert,
			}
		}
	}
	return dst
}

func convertNonAdminFromHub(src *v1alpha1.NonAdmin) *NonAdmin {
	dst := &NonAdmin{
		Enable:                  src.Enable,
		EnforceBackupSpec:       src.EnforceBackupSpec,
		EnforceRestoreSpec:      src.EnforceRestoreSpec,
		RequireApprovalForBSL:   src.RequireApprovalForBSL,
		GarbageCollectionPeriod: src.GarbageCollectionPeriod,
		BackupSyncPeriod:        src.BackupSyncPeriod,
	}
	if src.EnforceBSLSpec != nil {
		dst.EnforceBSLSpec = &EnforceBackupStorageLocationSpec{
			Provider:            src.EnforceBSLSpec.Provider,
			Config:              src.EnforceBSLSpec.Config,
			Credential:          src.EnforceBSLSpec.Credential,
			AccessMode:          src.EnforceBSLSpec.AccessMode,
			BackupSyncPeriod:    src.EnforceBSLSpec.BackupSyncPeriod,
			ValidationFrequency: src.EnforceBSLSpec.ValidationFrequency,
		}
		if src.EnforceBSLSpec.ObjectStorage != nil {
			dst.EnforceBSLSpec.ObjectStorage = &ObjectStorageLocation{
				Bucket: src.EnforceBSLSpec.ObjectStorage.Bucket,
				Prefix: src.EnforceBSLSpec.ObjectStorage.Prefix,
				CACert: src.EnforceBSLSpec.ObjectStorage.CACert,
			}
		}
	}
	return dst
}

func convertDataMoverToHub(src *DataMover) *v1alpha1.DataMover {
	dst := &v1alpha1.DataMover{
		Enable:                      src.Enable,
		CredentialName:              src.CredentialName,
		Timeout:                     src.Timeout,
		MaxConcurrentBackupVolumes:  src.MaxConcurrentBackupVolumes,
		MaxConcurrentRestoreVolumes: src.MaxConcurrentRestoreVolumes,
		PruneInterval:               src.PruneInterval,
		Schedule:                    src.Schedule,
	}
	if src.SnapshotRetainPolicy != nil {
		dst.SnapshotRetainPolicy = &v1alpha1.RetainPolicy{
			Hourly:  src.SnapshotRetainPolicy.Hourly,
			Daily:   src.SnapshotRetainPolicy.Daily,
			Weekly:  src.SnapshotRetainPolicy.Weekly,
			Monthly: src.SnapshotRetainPolicy.Monthly,
			Yearly:  src.SnapshotRetainPolicy.Yearly,
			Within:  src.SnapshotRetainPolicy.Within,
		}
	}
	if src.VolumeOptionsForStorageClasses != nil {
		dst.VolumeOptionsForStorageClasses = make(map[string]v1alpha1.DataMoverVolumeOptions, len(src.VolumeOptionsForStorageClasses))
		for k, v := range src.VolumeOptionsForStorageClasses {
			dvo := v1alpha1.DataMoverVolumeOptions{}
			if v.SourceVolumeOptions != nil {
				dvo.SourceVolumeOptions = &v1alpha1.VolumeOptions{
					StorageClassName:      v.SourceVolumeOptions.StorageClassName,
					AccessMode:            v.SourceVolumeOptions.AccessMode,
					CacheStorageClassName: v.SourceVolumeOptions.CacheStorageClassName,
					CacheCapacity:         v.SourceVolumeOptions.CacheCapacity,
					CacheAccessMode:       v.SourceVolumeOptions.CacheAccessMode,
				}
			}
			if v.DestinationVolumeOptions != nil {
				dvo.DestinationVolumeOptions = &v1alpha1.VolumeOptions{
					StorageClassName:      v.DestinationVolumeOptions.StorageClassName,
					AccessMode:            v.DestinationVolumeOptions.AccessMode,
					CacheStorageClassName: v.DestinationVolumeOptions.CacheStorageClassName,
					CacheCapacity:         v.DestinationVolumeOptions.CacheCapacity,
					CacheAccessMode:       v.DestinationVolumeOptions.CacheAccessMode,
				}
			}
			dst.VolumeOptionsForStorageClasses[k] = dvo
		}
	}
	return dst
}

func convertDataMoverFromHub(src *v1alpha1.DataMover) *DataMover {
	dst := &DataMover{
		Enable:                      src.Enable,
		CredentialName:              src.CredentialName,
		Timeout:                     src.Timeout,
		MaxConcurrentBackupVolumes:  src.MaxConcurrentBackupVolumes,
		MaxConcurrentRestoreVolumes: src.MaxConcurrentRestoreVolumes,
		PruneInterval:               src.PruneInterval,
		Schedule:                    src.Schedule,
	}
	if src.SnapshotRetainPolicy != nil {
		dst.SnapshotRetainPolicy = &RetainPolicy{
			Hourly:  src.SnapshotRetainPolicy.Hourly,
			Daily:   src.SnapshotRetainPolicy.Daily,
			Weekly:  src.SnapshotRetainPolicy.Weekly,
			Monthly: src.SnapshotRetainPolicy.Monthly,
			Yearly:  src.SnapshotRetainPolicy.Yearly,
			Within:  src.SnapshotRetainPolicy.Within,
		}
	}
	if src.VolumeOptionsForStorageClasses != nil {
		dst.VolumeOptionsForStorageClasses = make(map[string]DataMoverVolumeOptions, len(src.VolumeOptionsForStorageClasses))
		for k, v := range src.VolumeOptionsForStorageClasses {
			dvo := DataMoverVolumeOptions{}
			if v.SourceVolumeOptions != nil {
				dvo.SourceVolumeOptions = &VolumeOptions{
					StorageClassName:      v.SourceVolumeOptions.StorageClassName,
					AccessMode:            v.SourceVolumeOptions.AccessMode,
					CacheStorageClassName: v.SourceVolumeOptions.CacheStorageClassName,
					CacheCapacity:         v.SourceVolumeOptions.CacheCapacity,
					CacheAccessMode:       v.SourceVolumeOptions.CacheAccessMode,
				}
			}
			if v.DestinationVolumeOptions != nil {
				dvo.DestinationVolumeOptions = &VolumeOptions{
					StorageClassName:      v.DestinationVolumeOptions.StorageClassName,
					AccessMode:            v.DestinationVolumeOptions.AccessMode,
					CacheStorageClassName: v.DestinationVolumeOptions.CacheStorageClassName,
					CacheCapacity:         v.DestinationVolumeOptions.CacheCapacity,
					CacheAccessMode:       v.DestinationVolumeOptions.CacheAccessMode,
				}
			}
			dst.VolumeOptionsForStorageClasses[k] = dvo
		}
	}
	return dst
}

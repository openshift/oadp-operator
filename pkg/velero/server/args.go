package server

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/vmware-tanzu/velero/pkg/restore"
	"github.com/vmware-tanzu/velero/pkg/util/boolptr"
)

const (
	Server = "server"

	defaultFsBackupTimeout = 4 * time.Hour

	TrueVal  = "true"
	FalseVal = "false"
)

var defaultRestoreResourcePriorities = restore.Priorities{
	HighPriorities: []string{
		"securitycontextconstraints",
		"customresourcedefinitions",
		"namespaces",
		"roles",
		"rolebindings",
		"clusterrolebindings",
		"managedcluster.cluster.open-cluster-management.io",
		"managedcluster.clusterview.open-cluster-management.io",
		"klusterletaddonconfig.agent.open-cluster-management.io",
		"managedclusteraddon.addon.open-cluster-management.io",
		"storageclasses",
		"volumesnapshotclass.snapshot.storage.k8s.io",
		"volumesnapshotcontents.snapshot.storage.k8s.io",
		"volumesnapshots.snapshot.storage.k8s.io",
		"datauploads.velero.io",
		"persistentvolumes",
		"persistentvolumeclaims",
		"serviceaccounts",
		"secrets",
		"configmaps",
		"limitranges",
		"pods",
		"replicasets.apps",
		"clusterclasses.cluster.x-k8s.io",
		"endpoints",
		"services",
	},
	LowPriorities: []string{
		"clusterbootstraps.run.tanzu.vmware.com",
		"clusters.cluster.x-k8s.io",
		"clusterresourcesets.addons.cluster.x-k8s.io",
	},
}

func getFsBackupTimeout(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.Configuration.Restic != nil && len(dpa.Spec.Configuration.Restic.Timeout) > 0 {
		fsBackupTimeout, err := time.ParseDuration(dpa.Spec.Configuration.Restic.Timeout)
		if err == nil {
			return fsBackupTimeout.String()
		}
		// TODO should not error out?
	}
	if dpa.Spec.Configuration.NodeAgent != nil && len(dpa.Spec.Configuration.NodeAgent.Timeout) > 0 {
		fsBackupTimeout, err := time.ParseDuration(dpa.Spec.Configuration.NodeAgent.Timeout)
		if err == nil {
			return fsBackupTimeout.String()
		}
		// TODO should not error out?
	}
	return defaultFsBackupTimeout.String()
}

func getDefaultSnapshotMoveDataValue(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.Configuration.Velero != nil && boolptr.IsSetToTrue(dpa.Spec.Configuration.Velero.DefaultSnapshotMoveData) {
		return TrueVal
	}

	if dpa.Spec.Configuration.Velero != nil && boolptr.IsSetToFalse(dpa.Spec.Configuration.Velero.DefaultSnapshotMoveData) {
		return FalseVal
	}

	return ""
}

func getDefaultVolumesToFSBackup(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.Configuration.Velero != nil && boolptr.IsSetToTrue(dpa.Spec.Configuration.Velero.DefaultVolumesToFSBackup) {
		return TrueVal
	}

	if dpa.Spec.Configuration.Velero != nil && boolptr.IsSetToFalse(dpa.Spec.Configuration.Velero.DefaultVolumesToFSBackup) {
		return FalseVal
	}

	return ""
}

func disableInformerCacheValue(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.GetDisableInformerCache() {
		return TrueVal
	}
	return FalseVal
}

func GetVeleroServerFlags(dpa *oadpv1alpha1.DataProtectionApplication) ([]string, error) {
	args := []string{Server}
	userArgs := dpa.Spec.Configuration.Velero.Args
	hasUserArgs := userArgs != nil

	// Velero server command Flags

	if hasUserArgs && userArgs.BackupSyncPeriod != nil {
		args = append(args, fmt.Sprintf("--backup-sync-period=%s", userArgs.BackupSyncPeriod.String()))
	}
	if hasUserArgs && userArgs.ClientBurst != nil {
		args = append(args, fmt.Sprintf("--client-burst=%s", strconv.Itoa(*userArgs.ClientBurst)))
	}
	if hasUserArgs && userArgs.ClientPageSize != nil {
		args = append(args, fmt.Sprintf("--client-page-size=%s", strconv.Itoa(*userArgs.ClientPageSize)))
	}
	if hasUserArgs && userArgs.ClientQPS != nil {
		// TODO add to api update task
		if _, err := strconv.ParseFloat(*userArgs.ClientQPS, 32); err != nil {
			return nil, err
		}
		args = append(args, fmt.Sprintf("--client-qps=%s", *userArgs.ClientQPS))
	}
	if hasUserArgs && userArgs.DefaultBackupTTL != nil {
		args = append(args, fmt.Sprintf("--default-backup-ttl=%s", userArgs.DefaultBackupTTL.String()))
	}
	if hasUserArgs && userArgs.DefaultItemOperationTimeout != nil {
		args = append(args, fmt.Sprintf("--default-item-operation-timeout=%s", userArgs.DefaultItemOperationTimeout.String()))
	} else {
		// TODO DUPLICATION
		// Setting async operations server parameter DefaultItemOperationTimeout
		if dpa.Spec.Configuration.Velero.DefaultItemOperationTimeout != "" {
			args = append(args, fmt.Sprintf("--default-item-operation-timeout=%v", dpa.Spec.Configuration.Velero.DefaultItemOperationTimeout))
		}
	}
	if hasUserArgs && userArgs.RepoMaintenanceFrequency != nil {
		args = append(args, fmt.Sprintf("--default-repo-maintain-frequency=%s", userArgs.RepoMaintenanceFrequency.String()))
	}
	defaultSnapshotMoveData := getDefaultSnapshotMoveDataValue(dpa)
	if len(defaultSnapshotMoveData) > 0 {
		args = append(args, fmt.Sprintf("--default-snapshot-move-data=%s", defaultSnapshotMoveData))
	}
	// TODO --default-volume-snapshot-locations set outside Args
	if hasUserArgs && userArgs.DefaultVolumesToFsBackup != nil {
		args = append(args, fmt.Sprintf("--default-volumes-to-fs-backup=%s", strconv.FormatBool(*userArgs.DefaultVolumesToFsBackup)))
	} else {
		// TODO DUPLICATION
		defaultVolumesToFSBackup := getDefaultVolumesToFSBackup(dpa)
		if len(defaultVolumesToFSBackup) > 0 {
			args = append(args, fmt.Sprintf("--default-volumes-to-fs-backup=%s", defaultVolumesToFSBackup))
		}
	}
	if hasUserArgs && userArgs.DisabledControllers != nil {
		args = append(args, fmt.Sprintf("--disable-controllers=%s", strings.Join(userArgs.DisabledControllers, ",")))
	}
	disableInformerCache := disableInformerCacheValue(dpa)
	args = append(args, fmt.Sprintf("--disable-informer-cache=%s", disableInformerCache))

	if hasUserArgs && userArgs.PodVolumeOperationTimeout != nil {
		args = append(args, fmt.Sprintf("--fs-backup-timeout=%s", userArgs.PodVolumeOperationTimeout.String()))
	} else {
		// TODO DUPLICATION
		// Enable user to specify --fs-backup-timeout (defaults to 4h)
		// Append FS timeout option manually. Not configurable via install package, missing from podTemplateConfig struct. See: https://github.com/vmware-tanzu/velero/blob/8d57215ded1aa91cdea2cf091d60e072ce3f340f/pkg/install/deployment.go#L34-L45
		args = append(args, fmt.Sprintf("--fs-backup-timeout=%s", getFsBackupTimeout(dpa)))
	}
	if hasUserArgs && userArgs.GarbageCollectionFrequency != nil {
		args = append(args, fmt.Sprintf("--garbage-collection-frequency=%s", userArgs.GarbageCollectionFrequency.String()))
	}
	if hasUserArgs && userArgs.ItemOperationSyncFrequency != nil {
		args = append(args, fmt.Sprintf("--item-operation-sync-frequency=%s", userArgs.ItemOperationSyncFrequency.String()))
	} else {
		// TODO DUPLICATION
		// Setting async operations server parameter ItemOperationSyncFrequency
		if dpa.Spec.Configuration.Velero.ItemOperationSyncFrequency != "" {
			args = append(args, fmt.Sprintf("--item-operation-sync-frequency=%v", dpa.Spec.Configuration.Velero.ItemOperationSyncFrequency))
		}
	}
	if hasUserArgs && userArgs.FormatFlag != "" {
		args = append(args, fmt.Sprintf("--log-format=%s", userArgs.FormatFlag))
	}
	if dpa.Spec.Configuration.Velero.LogLevel != "" {
		args = append(args, fmt.Sprintf("--log-level=%s", dpa.Spec.Configuration.Velero.LogLevel))
	}
	if hasUserArgs && userArgs.MaxConcurrentK8SConnections != nil {
		args = append(args, fmt.Sprintf("--max-concurrent-k8s-connections=%d", *userArgs.MaxConcurrentK8SConnections))
	}
	if hasUserArgs && userArgs.MetricsAddress != "" {
		args = append(args, fmt.Sprintf("--metrics-address=%s", userArgs.MetricsAddress))
	}
	// TODO --plugin-dir plugin-dir is fixed to /plugins
	if hasUserArgs && userArgs.ProfilerAddress != "" {
		args = append(args, fmt.Sprintf("--profiler-address=%s", userArgs.ProfilerAddress))
	}
	if hasUserArgs && userArgs.ResourceTimeout != nil {
		args = append(args, fmt.Sprintf("--resource-timeout=%s", userArgs.ResourceTimeout.String()))
	} else {
		// TODO DUPLICATION
		if dpa.Spec.Configuration.Velero.ResourceTimeout != "" {
			args = append(args, fmt.Sprintf("--resource-timeout=%v", dpa.Spec.Configuration.Velero.ResourceTimeout))
		}
	}
	// TODO --restore-only is being deprecated, set in bsl

	// RestoreResourcePriorities are set in DPA which creates a ConfigMap
	// However, server args is also an option.
	if hasUserArgs && userArgs.RestoreResourcePriorities != "" {
		args = append(args, fmt.Sprintf("--restore-resource-priorities=%s", userArgs.RestoreResourcePriorities))
	} else {
		// Overriding velero restore resource priorities to OpenShift default (ie. SecurityContextConstraints needs to be restored before pod/SA)
		args = append(args, fmt.Sprintf("--restore-resource-priorities=%s", defaultRestoreResourcePriorities.String()))
	}
	// TODO --schedule-skip-immediately
	if hasUserArgs && userArgs.StoreValidationFrequency != nil {
		args = append(args, fmt.Sprintf("--store-validation-frequency=%s", userArgs.StoreValidationFrequency.String()))
	}
	if hasUserArgs && userArgs.ResourceTerminatingTimeout != nil {
		args = append(args, fmt.Sprintf("--terminating-resource-timeout=%s", userArgs.ResourceTerminatingTimeout.String()))
	}
	// TODO --uploader-type string

	// Global Flags

	if hasUserArgs && userArgs.AddDirHeader != nil {
		args = append(args, fmt.Sprintf("--add_dir_header=%s", strconv.FormatBool(*userArgs.AddDirHeader)))
	}
	if hasUserArgs && userArgs.AlsoToStderr != nil {
		args = append(args, fmt.Sprintf("--alsologtostderr=%s", strconv.FormatBool(*userArgs.AlsoToStderr)))
	}
	if hasUserArgs && userArgs.Colorized != nil {
		args = append(args, fmt.Sprintf("--colorized=%s", strconv.FormatBool(*userArgs.Colorized)))
	}
	if len(dpa.Spec.Configuration.Velero.FeatureFlags) > 0 {
		args = append(args, fmt.Sprintf("--features=%s", strings.Join(dpa.Spec.Configuration.Velero.FeatureFlags, ",")))
	}
	// TODO --kubeconfig
	// TODO --kubecontext
	if hasUserArgs && userArgs.TraceLocation != "" {
		args = append(args, fmt.Sprintf("--log_backtrace_at=%s", userArgs.TraceLocation))
	}
	if hasUserArgs && userArgs.LogDir != "" {
		args = append(args, fmt.Sprintf("--log_dir=%s", userArgs.LogDir))
	}
	if hasUserArgs && userArgs.LogFile != "" {
		args = append(args, fmt.Sprintf("--log_file=%s", userArgs.LogFile))
	}
	if hasUserArgs && userArgs.LogFileMaxSizeMB != nil {
		args = append(args, fmt.Sprintf("--log_file_max_size=%d", *userArgs.LogFileMaxSizeMB))
	}
	if hasUserArgs && userArgs.ToStderr != nil {
		args = append(args, fmt.Sprintf("--logtostderr=%s", strconv.FormatBool(*userArgs.ToStderr)))
	}
	// TODO --namespace
	if hasUserArgs && userArgs.OneOutput != nil {
		args = append(args, fmt.Sprintf("--one_output=%s", strconv.FormatBool(*userArgs.ToStderr)))
	}
	if hasUserArgs && userArgs.SkipHeaders != nil {
		args = append(args, fmt.Sprintf("--skip_headers=%s", strconv.FormatBool(*userArgs.SkipHeaders)))
	}
	if hasUserArgs && userArgs.SkipLogHeaders != nil {
		args = append(args, fmt.Sprintf("--skip_log_headers=%s", strconv.FormatBool(*userArgs.SkipLogHeaders)))
	}
	if hasUserArgs && userArgs.StderrThreshold != nil {
		args = append(args, fmt.Sprintf("--stderrthreshold=%d", *userArgs.StderrThreshold))
	}
	if hasUserArgs && userArgs.Verbosity != nil {
		args = append(args, fmt.Sprintf("--v=%d", *userArgs.Verbosity))
	}
	if hasUserArgs && userArgs.Vmodule != "" {
		args = append(args, fmt.Sprintf("--vmodule=%s", userArgs.Vmodule))
	}

	return args, nil
}

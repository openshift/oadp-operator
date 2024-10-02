package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/vmware-tanzu/velero/pkg/util/boolptr"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

// GetArgs returns the Velero server arguments as a string array
// Most validations are done in the DPA CRD except float32 validation
func GetArgs(dpa *oadpv1alpha1.DataProtectionApplication) ([]string, error) {
	serverArgs := dpa.Spec.Configuration.Velero.Args
	args := []string{"server"}
	// we are overriding args, so recreate args from scratch
	if len(dpa.Spec.Configuration.Velero.FeatureFlags) > 0 {
		args = append(args, fmt.Sprintf("--features=%s", strings.Join(dpa.Spec.Configuration.Velero.FeatureFlags, ",")))
	}
	if boolptr.IsSetToTrue(serverArgs.DefaultVolumesToFsBackup) {
		args = append(args, "--default-volumes-to-fs-backup=true")
	} else if boolptr.IsSetToFalse(serverArgs.DefaultVolumesToFsBackup) {
		args = append(args, "--default-volumes-to-fs-backup=false")
	}
	if dpa.Spec.Configuration.Velero.LogLevel != "" {
		args = append(args, fmt.Sprintf("--log-level=%s", dpa.Spec.Configuration.Velero.LogLevel))
	}
	if serverArgs.BackupSyncPeriod != nil {
		args = append(args, fmt.Sprintf("--backup-sync-period=%s", serverArgs.BackupSyncPeriod.String())) // duration
	}
	if serverArgs.ClientBurst != nil {
		args = append(args, fmt.Sprintf("--client-burst=%s", strconv.Itoa(*serverArgs.ClientBurst))) // int
	}
	if serverArgs.ClientPageSize != nil {
		args = append(args, fmt.Sprintf("--client-page-size=%s", strconv.Itoa(*serverArgs.ClientPageSize))) // int
	}
	if serverArgs.ClientQPS != nil {
		if _, err := strconv.ParseFloat(*serverArgs.ClientQPS, 32); err != nil {
			return nil, err
		}
		args = append(args, fmt.Sprintf("--client-qps=%s", *serverArgs.ClientQPS)) // float32
	}
	// default-backup-storage-location set outside Args
	if serverArgs.DefaultBackupTTL != nil {
		args = append(args, fmt.Sprintf("--default-backup-ttl=%s", serverArgs.DefaultBackupTTL.String())) // duration
	}
	if serverArgs.DefaultItemOperationTimeout != nil {
		args = append(args, fmt.Sprintf("--default-item-operation-timeout=%s", serverArgs.DefaultItemOperationTimeout.String())) // duration
	}
	if serverArgs.ResourceTimeout != nil {
		args = append(args, fmt.Sprintf("--resource-timeout=%s", serverArgs.ResourceTimeout.String())) // duration
	}
	if serverArgs.RepoMaintenanceFrequency != nil {
		args = append(args, fmt.Sprintf("--default-repo-maintain-frequency=%s", serverArgs.RepoMaintenanceFrequency.String())) // duration
	}
	// default-volume-snapshot-locations set outside Args
	if serverArgs.DisabledControllers != nil {
		args = append(args, fmt.Sprintf("--disable-controllers=%s", strings.Join(serverArgs.DisabledControllers, ","))) // strings
	}
	if serverArgs.GarbageCollectionFrequency != nil {
		args = append(args, fmt.Sprintf("--garbage-collection-frequency=%s", serverArgs.GarbageCollectionFrequency.String())) // duration
	}
	if serverArgs.FormatFlag != "" {
		args = append(args, fmt.Sprintf("--log-format=%s", serverArgs.FormatFlag)) // format
	}
	if serverArgs.MetricsAddress != "" {
		args = append(args, fmt.Sprintf("--metrics-address=%s", serverArgs.MetricsAddress)) // string
	}
	// plugin-dir is fixed to /plugins
	if serverArgs.ProfilerAddress != "" {
		args = append(args, fmt.Sprintf("--profiler-address=%s", serverArgs.ProfilerAddress)) // string
	}
	if serverArgs.PodVolumeOperationTimeout != nil {
		args = append(args, fmt.Sprintf("--fs-backup-timeout=%s", serverArgs.PodVolumeOperationTimeout.String())) // duration
	}
	if serverArgs.ItemOperationSyncFrequency != nil {
		args = append(args, fmt.Sprintf("--item-operation-sync-frequency=%s", serverArgs.ItemOperationSyncFrequency.String())) // duration
	}
	if serverArgs.MaxConcurrentK8SConnections != nil {
		args = append(args, fmt.Sprintf("--max-concurrent-k8s-connections=%d", *serverArgs.MaxConcurrentK8SConnections)) // uint
	}
	// restore-only is being deprecated, set in bsl
	// RestoreResourcePriorities are set in DPA which creates a configmap
	// However, server args is also an option.
	if serverArgs.RestoreResourcePriorities != "" {
		args = append(args, fmt.Sprintf("--restore-resource-priorities=%s", serverArgs.RestoreResourcePriorities)) // stringArray
	}
	if serverArgs.StoreValidationFrequency != nil {
		args = append(args, fmt.Sprintf("--store-validation-frequency=%s", serverArgs.StoreValidationFrequency.String())) // duration
	}
	if serverArgs.ResourceTerminatingTimeout != nil {
		args = append(args, fmt.Sprintf("--terminating-resource-timeout=%s", serverArgs.ResourceTerminatingTimeout.String())) // duration
	}
	if serverArgs.AddDirHeader != nil {
		args = append(args, fmt.Sprintf("--add_dir_header=%s", strconv.FormatBool(*serverArgs.AddDirHeader))) // optionalBool
	}
	if serverArgs.AlsoToStderr != nil {
		args = append(args, fmt.Sprintf("--alsologtostderr=%s", strconv.FormatBool(*serverArgs.AlsoToStderr))) // alsologtostderr
	}
	if serverArgs.Colorized != nil {
		args = append(args, fmt.Sprintf("--colorized=%s", strconv.FormatBool(*serverArgs.Colorized))) // optionalBool
	}
	// features set outside Args
	// args = append(args, "--kubeconfig")        // string
	// args = append(args, "--kubecontext")       // string
	if serverArgs.TraceLocation != "" {
		args = append(args, fmt.Sprintf("--log_backtrace_at=%s", serverArgs.TraceLocation)) // traceLocation
	}
	if serverArgs.LogDir != "" {
		args = append(args, fmt.Sprintf("--log_dir=%s", serverArgs.LogDir)) // string
	}
	if serverArgs.LogFile != "" {
		args = append(args, fmt.Sprintf("--log_file=%s", serverArgs.LogFile)) // string
	}
	if serverArgs.LogFileMaxSizeMB != nil {
		args = append(args, fmt.Sprintf("--log_file_max_size=%d", *serverArgs.LogFileMaxSizeMB)) // uint
	}
	if serverArgs.ToStderr != nil {
		args = append(args, fmt.Sprintf("--logtostderr=%s", strconv.FormatBool(*serverArgs.ToStderr))) // optionalBool
	}
	// args = append(args, "--namespace")         // string
	if serverArgs.SkipHeaders != nil {
		args = append(args, fmt.Sprintf("--skip_headers=%s", strconv.FormatBool(*serverArgs.SkipHeaders))) // optionalBool
	}
	if serverArgs.SkipLogHeaders != nil {
		args = append(args, fmt.Sprintf("--skip_log_headers=%s", strconv.FormatBool(*serverArgs.SkipLogHeaders))) // optionalBool
	}
	if serverArgs.StderrThreshold != nil {
		args = append(args, fmt.Sprintf("--stderrthreshold=%d", *serverArgs.StderrThreshold)) // severity
	}
	if serverArgs.Verbosity != nil {
		args = append(args, fmt.Sprintf("--v=%d", *serverArgs.Verbosity)) // count
	}
	if serverArgs.Vmodule != "" {
		args = append(args, fmt.Sprintf("--vmodule=%s", serverArgs.Vmodule)) // string
	}
	return args, nil
}

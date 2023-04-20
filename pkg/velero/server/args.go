package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/openshift/oadp-operator/pkg/klog"
	"github.com/openshift/oadp-operator/pkg/velero/client"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero/pkg/util/boolptr"
)

// VeleroServerArgs are the arguments that are passed to the Velero server
// +kubebuilder:object:generate=true
type Args struct {
	ServerConfig `json:",inline"`
	GlobalFlags  `json:",inline"`
}

// GlobalFlags are flags that are defined across Velero CLI commands
// +kubebuilder:object:generate=true
type GlobalFlags struct {
	client.VeleroConfig `json:",inline"`
	klog.LoggingT       `json:",inline"`
}

// StringArr returns the Velero server arguments as a string array
// dpaFeatureFlags are the feature flags that are defined in the DPA CR which will be merged with Args
// Most validations are done in the DPA CRD except float32 validation
func (a Args) StringArr(dpaFeatureFlags []string, logLevel string) ([]string, error) {
	args := []string{"server"}
	// we are overriding args, so recreate args from scratch
	if len(dpaFeatureFlags) > 0 {
		args = append(args, fmt.Sprintf("--features=%s", strings.Join(dpaFeatureFlags, ",")))
	}
	if boolptr.IsSetToTrue(a.DefaultVolumesToRestic) {
		args = append(args, "--default-volumes-to-restic=true")
	} else if boolptr.IsSetToFalse(a.DefaultVolumesToRestic) {
		args = append(args, "--default-volumes-to-restic=false")
	}
	if logLevel != "" {
		logrusLevel, err := logrus.ParseLevel(logLevel)
		if err != nil {
			return []string{}, fmt.Errorf("invalid log level %s, use: %s", logLevel, "trace, debug, info, warning, error, fatal, or panic")
		}
		args = append(args, fmt.Sprintf("--log-level=%s", logrusLevel.String()))
	}
	if a.BackupSyncPeriod != nil {
		args = append(args, fmt.Sprintf("--backup-sync-period=%s", a.BackupSyncPeriod.String())) // duration
	}
	if a.ClientBurst != nil {
		args = append(args, fmt.Sprintf("--client-burst=%s", strconv.Itoa(*a.ClientBurst))) // int
	}
	if a.ClientPageSize != nil {
		args = append(args, fmt.Sprintf("--client-page-size=%s", strconv.Itoa(*a.ClientPageSize))) // int
	}
	if a.ClientQPS != nil {
		if _, err := strconv.ParseFloat(*a.ClientQPS, 32); err != nil {
			return nil, err
		}
		args = append(args, fmt.Sprintf("--client-qps=%s", *a.ClientQPS)) // float32
	}
	// default-backup-storage-location set outside Args
	if a.DefaultBackupTTL != nil {
		args = append(args, fmt.Sprintf("--default-backup-ttl=%s", a.DefaultBackupTTL.String())) // duration
	}
	if a.DefaultResticMaintenanceFrequency != nil {
		args = append(args, fmt.Sprintf("--default-restic-prune-frequency=%s", a.DefaultResticMaintenanceFrequency.String())) // duration
	}
	// default-volume-snapshot-locations set outside Args
	if a.DisabledControllers != nil {
		args = append(args, fmt.Sprintf("--disable-controllers=%s", strings.Join(a.DisabledControllers, ","))) // strings
	}
	if a.GarbageCollectionFrequency != nil {
		args = append(args, fmt.Sprintf("--garbage-collection-frequency=%s", a.GarbageCollectionFrequency.String())) // duration
	}
	if a.FormatFlag != "" {
		args = append(args, fmt.Sprintf("--log-format=%s", a.FormatFlag)) // format
	}
	if a.MetricsAddress != "" {
		args = append(args, fmt.Sprintf("--metrics-address=%s", a.MetricsAddress)) // string
	}
	// plugin-dir is fixed to /plugins
	if a.ProfilerAddress != "" {
		args = append(args, fmt.Sprintf("--profiler-address=%s", a.ProfilerAddress)) // string
	}
	if a.PodVolumeOperationTimeout != nil {
		args = append(args, fmt.Sprintf("--restic-timeout=%s", a.PodVolumeOperationTimeout.String())) // duration
	}
	// restore-only is being deprecated, set in bsl
	// RestoreResourcePriorities are set in DPA which creates a configmap
	// However, server args is also an option.
	if a.RestoreResourcePriorities != "" {
		args = append(args, fmt.Sprintf("--restore-resource-priorities=%s", a.RestoreResourcePriorities)) // stringArray
	}
	if a.StoreValidationFrequency != nil {
		args = append(args, fmt.Sprintf("--store-validation-frequency=%s", a.StoreValidationFrequency.String())) // duration
	}
	if a.ResourceTerminatingTimeout != nil {
		args = append(args, fmt.Sprintf("--terminating-resource-timeout=%s", a.ResourceTerminatingTimeout.String())) // duration
	}
	if a.AddDirHeader != nil {
		args = append(args, fmt.Sprintf("--add_dir_header=%s", strconv.FormatBool(*a.AddDirHeader))) // optionalBool
	}
	if a.AlsoToStderr != nil {
		args = append(args, fmt.Sprintf("--alsologtostderr=%s", strconv.FormatBool(*a.AlsoToStderr))) // alsologtostderr
	}
	if a.Colorized != nil {
		args = append(args, fmt.Sprintf("--colorized=%s", strconv.FormatBool(*a.Colorized))) // optionalBool
	}
	// features set outside Args
	// args = append(args, "--kubeconfig")        // string
	// args = append(args, "--kubecontext")       // string
	if a.TraceLocation != "" {
		args = append(args, fmt.Sprintf("--log_backtrace_at=%s", a.TraceLocation)) // traceLocation
	}
	if a.LogDir != "" {
		args = append(args, fmt.Sprintf("--log_dir=%s", a.LogDir)) // string
	}
	if a.LogFile != "" {
		args = append(args, fmt.Sprintf("--log_file=%s", a.LogFile)) // string
	}
	if a.LogFileMaxSizeMB != nil {
		args = append(args, fmt.Sprintf("--log_file_max_size=%d", *a.LogFileMaxSizeMB)) // uint
	}
	if a.ToStderr != nil {
		args = append(args, fmt.Sprintf("--logtostderr=%s", strconv.FormatBool(*a.ToStderr))) // optionalBool
	}
	// args = append(args, "--namespace")         // string
	if a.SkipHeaders != nil {
		args = append(args, fmt.Sprintf("--skip_headers=%s", strconv.FormatBool(*a.SkipHeaders))) // optionalBool
	}
	if a.SkipLogHeaders != nil {
		args = append(args, fmt.Sprintf("--skip_log_headers=%s", strconv.FormatBool(*a.SkipLogHeaders))) // optionalBool
	}
	if a.StderrThreshold != nil {
		args = append(args, fmt.Sprintf("--stderrthreshold=%d", *a.StderrThreshold)) // severity
	}
	if a.Verbosity != nil {
		args = append(args, fmt.Sprintf("--v=%d", *a.Verbosity)) // count
	}
	if a.Vmodule != "" {
		args = append(args, fmt.Sprintf("--vmodule=%s", a.Vmodule)) // string
	}
	return args, nil
}

package klog

// klog init flags from https://github.com/openshift/velero/blob/240b4e666fe15ef98defa2b51483fe87ac9996fb/pkg/cmd/velero/velero.go#L125
// loggingT collects all the global state of the logging setup.
// +kubebuilder:object:generate=true
type LoggingT struct {
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

package common

const (
	Velero             = "velero"
	Restic             = "restic"
	VeleroNamespace    = "oadp-operator"
	OADPOperator       = "oadp-operator"
	OADPOperatorVelero = "oadp-operator-velero"
)

// Plugin names
const (
	VeleroPluginForAWS       = "velero-plugin-for-aws"
	VeleroPluginForAzure     = "velero-plugin-for-microsoft-azure"
	VeleroPluginForGCP       = "velero-plugin-for-gcp"
	VeleroPluginForCSI       = "velero-plugin-for-csi"
	VeleroPluginForOpenshift = "openshift-velero-plugin"
)

// Environment Vars keys
const (
	LDLibraryPathEnvKey            = "LD_LIBRARY_PATH"
	VeleroNamespaceEnvKey          = "VELERO_NAMESPACE"
	VeleroScratchDirEnvKey         = "VELERO_SCRATCH_DIR"
	AWSSharedCredentialsFileEnvKey = "AWS_SHARED_CREDENTIALS_FILE"
	AzureCredentialsFileEnvKey     = "AZURE_CREDENTIALS_FILE"
	GCPCredentialsEnvKey           = "GOOGLE_APPLICATION_CREDENTIALS"
	HTTPProxyEnvVar                = "HTTP_PROXY"
	HTTPSProxyEnvVar               = "HTTPS_PROXY"
	NoProxyEnvVar                  = "NO_PROXY"
)

package common

const (
	Velero             = "velero"
	Restic             = "restic"
	VeleroNamespace    = "oadp-operator"
	OADPOperator       = "oadp-operator"
	OADPOperatorVelero = "oadp-operator-velero"
)

// Images
const (
	VeleroImage          = "quay.io/konveyor/velero:latest"
	OpenshiftPluginImage = "quay.io/konveyor/openshift-velero-plugin:latest"
	AWSPluginImage       = "quay.io/konveyor/velero-plugin-for-aws:latest"
	AzurePluginImage     = "quay.io/konveyor/velero-plugin-for-microsoft-azure:latest"
	GCPPluginImage       = "quay.io/konveyor/velero-plugin-for-gcp:latest"
	CSIPluginImage       = "quay.io/konveyor/velero-plugin-for-csi:latest"
	RegistryImage        = "quay.io/konveyor/registry:latest"
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

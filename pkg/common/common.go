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
	VeleroImage          = "quay.io/konveyor/velero:konveyor-1.7.0"
	OpenshiftPluginImage = "quay.io/konveyor/openshift-velero-plugin:oadp-0.3.0"
	AWSPluginImage       = "quay.io/konveyor/velero-plugin-for-aws:konveyor-1.3.0"
	AzurePluginImage     = "quay.io/konveyor/velero-plugin-for-microsoft-azure:konveyor-1.3.0"
	GCPPluginImage       = "quay.io/konveyor/velero-plugin-for-gcp:konveyor-1.3.0"
	CSIPluginImage       = "quay.io/konveyor/velero-plugin-for-csi:konveyor-0.2.0"
	RegistryImage        = "quay.io/konveyor/registry:oadp-0.3.0"
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

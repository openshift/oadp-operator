package common

import "fmt"

const (
	Velero             = "velero"
	Restic             = "restic"
	VeleroNamespace    = "oadp-operator"
	OADPOperator       = "oadp-operator"
	OADPOperatorVelero = "oadp-operator-velero"
)

// Images
const (
	VeleroImage          = "quay.io/konveyor/velero:oadp-1.0"
	OpenshiftPluginImage = "quay.io/konveyor/openshift-velero-plugin:oadp-1.0"
	AWSPluginImage       = "quay.io/konveyor/velero-plugin-for-aws:oadp-1.0"
	AzurePluginImage     = "quay.io/konveyor/velero-plugin-for-microsoft-azure:oadp-1.0"
	GCPPluginImage       = "quay.io/konveyor/velero-plugin-for-gcp:oadp-1.0"
	CSIPluginImage       = "quay.io/konveyor/velero-plugin-for-csi:oadp-1.0"
	RegistryImage        = "quay.io/konveyor/registry:oadp-1.0"
	KubeVirtPluginImage  = "quay.io/konveyor/kubevirt-velero-plugin:v0.2.0"
)

// Plugin names
const (
	VeleroPluginForAWS       = "velero-plugin-for-aws"
	VeleroPluginForAzure     = "velero-plugin-for-microsoft-azure"
	VeleroPluginForGCP       = "velero-plugin-for-gcp"
	VeleroPluginForCSI       = "velero-plugin-for-csi"
	VeleroPluginForOpenshift = "openshift-velero-plugin"
	KubeVirtPlugin           = "kubevirt-velero-plugin"
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

const defaultMode = int32(420)

func DefaultModePtr() *int32 {
	var mode int32 = defaultMode
	return &mode
}

// append labels together
func AppendUniqueLabels(userLabels ...map[string]string) (map[string]string, error) {
	return AppendUniqueKeyStringOfStringMaps(userLabels...)
}

func AppendUniqueKeyStringOfStringMaps(userLabels ...map[string]string) (map[string]string, error) {
	base := map[string]string{}
	for _, labels := range userLabels {
		if labels == nil {
			continue
		}
		for k, v := range labels {
			if base[k] == "" {
				base[k] = v
			} else if base[k] != v {
				return nil, fmt.Errorf("conflicting key %s with value %s may not override %s", k, v, base[k])
			}
		}
	}
	return base, nil
}

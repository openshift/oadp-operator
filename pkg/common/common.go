package common

import "fmt"

const (
	Velero                       = "velero"
	Restic                       = "restic"
	VeleroNamespace              = "oadp-operator"
	OADPOperator                 = "oadp-operator"
	OADPOperatorVelero           = "oadp-operator-velero"
	DataMover                    = "volume-snapshot-mover"
	DataMoverController          = "data-mover-controller"
	DataMoverControllerContainer = "data-mover-controller-container"
	OADPOperatorServiceAccount   = "openshift-adp-controller-manager"
	VolSyncDeploymentName        = "volsync-controller-manager"
	VolSyncDeploymentNamespace   = "openshift-operators"
)

// Images
const (
	VeleroImage          = "quay.io/konveyor/velero:konveyor-1.9"
	OpenshiftPluginImage = "quay.io/konveyor/openshift-velero-plugin:latest"
	AWSPluginImage       = "quay.io/konveyor/velero-plugin-for-aws:konveyor-1.5"
	AzurePluginImage     = "quay.io/konveyor/velero-plugin-for-microsoft-azure:konveyor-1.5"
	GCPPluginImage       = "quay.io/konveyor/velero-plugin-for-gcp:konveyor-1.5"
	CSIPluginImage       = "quay.io/konveyor/velero-plugin-for-csi:konveyor-0.3"
	// CSIDataMoverPluginImage is the modified version of the velero plugin for csi which facilitates movement of csi snapshots
	CSIDataMoverPluginImage = "quay.io/konveyor/velero-plugin-for-csi:data-mover"
	// DataMoverImage is the data mover controller for data mover CRs - VolumeSnapshotBackup and VolumeSnapshotRestore
	DataMoverImage      = "quay.io/konveyor/volume-snapshot-mover:latest"
	RegistryImage       = "quay.io/konveyor/registry:latest"
	KubeVirtPluginImage = "quay.io/konveyor/kubevirt-velero-plugin:v0.2.0"
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

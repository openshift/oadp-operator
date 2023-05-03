package common

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

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
	VeleroImage          = "quay.io/konveyor/velero:oadp-1.1"
	OpenshiftPluginImage = "quay.io/konveyor/openshift-velero-plugin:oadp-1.1"
	AWSPluginImage       = "quay.io/konveyor/velero-plugin-for-aws:oadp-1.1"
	AzurePluginImage     = "quay.io/konveyor/velero-plugin-for-microsoft-azure:oadp-1.1"
	GCPPluginImage       = "quay.io/konveyor/velero-plugin-for-gcp:oadp-1.1"
	CSIPluginImage       = "quay.io/konveyor/velero-plugin-for-csi:oadp-1.1"
	// DataMoverImage is the data mover controller for data mover CRs - VolumeSnapshotBackup and VolumeSnapshotRestore
	DataMoverImage      = "quay.io/konveyor/volume-snapshot-mover:oadp-1.1"
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

const defaultMode = int32(420)

func DefaultModePtr() *int32 {
	var mode int32 = defaultMode
	return &mode
}

func AppendUniqueKeyTOfTMaps[T comparable](userLabels ...map[T]T) (map[T]T, error) {
	var base map[T]T
	for _, labels := range userLabels {
		if labels == nil {
			continue
		}
		if base == nil {
			base = make(map[T]T)
		}
		for k, v := range labels {
			if _, found := base[k]; !found {
				base[k] = v
			} else if base[k] != v {
				return nil, fmt.Errorf("conflicting key %v with value %v may not override %v", k, v, base[k])
			}
		}
	}
	return base, nil
}

// append env vars together where the first one wins
func AppendUniqueEnvVars(userEnvVars ...[]corev1.EnvVar) []corev1.EnvVar {
	base := []corev1.EnvVar{}
	for _, envVars := range userEnvVars {
		if envVars == nil {
			continue
		}
		for _, envVar := range envVars {
			if !containsEnvVar(base, envVar) {
				base = append(base, envVar)
			}
		}
	}
	return base
}

func containsEnvVar(envVars []corev1.EnvVar, envVar corev1.EnvVar) bool {
	for _, e := range envVars {
		if e.Name == envVar.Name {
			return true
		}
	}
	return false
}

func AppendUniqueValues[T comparable](slice []T, values ...T) []T {
	if values == nil || len(values) == 0 {
		return slice
	}
	slice = append(slice, values...)
	return RemoveDuplicateValues(slice)
}

type e struct{} // empty struct

func RemoveDuplicateValues[T comparable](slice []T) []T {
	if slice == nil {
		return nil
	}
	keys := make(map[T]e)
	list := []T{}
	for _, entry := range slice {
		if _, found := keys[entry]; !found { //add entry to list if not found in keys already
			keys[entry] = e{}
			list = append(list, entry)
		}
	}
	return list // return the result through the passed in argument
}

func AppendTTMapAsCopy[T comparable](add ...map[T]T) map[T]T {
	if add == nil || len(add) == 0 {
		return nil
	}
	base := map[T]T{}
	for k, v := range add[0] {
		base[k] = v
	}
	if len(add) == 1 {
		return base
	}
	for i := 1; i < len(add); i++ {
		for k, v := range add[i] {
			base[k] = v
		}
	}
	return base
}

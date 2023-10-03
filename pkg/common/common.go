package common

import (
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/vmware-tanzu/velero/pkg/restore"
	corev1 "k8s.io/api/core/v1"
)

const (
	Velero                     = "velero"
	NodeAgent                  = "node-agent"
	VeleroNamespace            = "oadp-operator"
	OADPOperator               = "oadp-operator"
	OADPOperatorVelero         = "oadp-operator-velero"
	OADPOperatorServiceAccount = "openshift-adp-controller-manager"
)

var DefaultRestoreResourcePriorities = restore.Priorities{
	HighPriorities: []string{
		"securitycontextconstraints",
		"customresourcedefinitions",
		"namespaces",
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

// Images
const (
	VeleroImage          = "quay.io/konveyor/velero:latest"
	OpenshiftPluginImage = "quay.io/konveyor/openshift-velero-plugin:latest"
	AWSPluginImage       = "quay.io/konveyor/velero-plugin-for-aws:latest"
	AzurePluginImage     = "quay.io/konveyor/velero-plugin-for-microsoft-azure:latest"
	GCPPluginImage       = "quay.io/konveyor/velero-plugin-for-gcp:latest"
	CSIPluginImage       = "quay.io/konveyor/velero-plugin-for-csi:latest"
	DummyPodImage        = "quay.io/konveyor/rsync-transfer:latest"
	RegistryImage        = "quay.io/konveyor/registry:latest"
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

// CCOWorkflow checks if the AWS STS secret is to be obtained from Cloud Credentials Operator (CCO)
// if the user provides role ARN during installation then the ARN gets set as env var on operator deployment
// during installation via OLM
func CCOWorkflow() bool {
	roleARN := os.Getenv("ROLEARN")
	if len(roleARN) > 0 {
		return true
	}
	return false
}

// StripDefaultPorts removes port 80 from HTTP URLs and 443 from HTTPS URLs.
// Defer to the actual AWS SDK implementation to match its behavior exactly.
func StripDefaultPorts(fromUrl string) (string, error) {
	u, err := url.Parse(fromUrl)
	if err != nil {
		return "", err
	}
	r := http.Request{
		URL: u,
	}
	request.SanitizeHostForHeader(&r)
	if r.Host != "" {
		r.URL.Host = r.Host
	}
	return r.URL.String(), nil
}

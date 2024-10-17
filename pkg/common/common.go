package common

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/vmware-tanzu/velero/pkg/types"
	corev1 "k8s.io/api/core/v1"
)

const (
	// From config/default/kustomization.yaml namePrefix field
	OADPOperatorPrefix         = "openshift-adp-"
	Velero                     = "velero"
	NodeAgent                  = "node-agent"
	VeleroNamespace            = "oadp-operator"
	OADPOperator               = "oadp-operator"
	OADPOperatorVelero         = "oadp-operator-velero"
	OADPOperatorServiceAccount = OADPOperatorPrefix + "controller-manager"
)

var DefaultRestoreResourcePriorities = types.Priorities{
	HighPriorities: []string{
		"securitycontextconstraints",
		"customresourcedefinitions",
		"klusterletconfigs.config.open-cluster-management.io",
		"managedcluster.cluster.open-cluster-management.io",
		"namespaces",
		"roles",
		"rolebindings",
		"clusterrolebindings",
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
	LegacyAWSPluginImage = "quay.io/konveyor/velero-plugin-for-legacy-aws:latest"
	AzurePluginImage     = "quay.io/konveyor/velero-plugin-for-microsoft-azure:latest"
	GCPPluginImage       = "quay.io/konveyor/velero-plugin-for-gcp:latest"
	RegistryImage        = "quay.io/konveyor/registry:latest"
	KubeVirtPluginImage  = "quay.io/konveyor/kubevirt-velero-plugin:v0.7.0"
)

// Plugin names
const (
	VeleroPluginForAWS       = "velero-plugin-for-aws"
	VeleroPluginForLegacyAWS = "velero-plugin-for-legacy-aws"
	VeleroPluginForAzure     = "velero-plugin-for-microsoft-azure"
	VeleroPluginForGCP       = "velero-plugin-for-gcp"
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

// Unsupported Server Args annotation keys
const (
	UnsupportedVeleroServerArgsAnnotation    = "oadp.openshift.io/unsupported-velero-server-args"
	UnsupportedNodeAgentServerArgsAnnotation = "oadp.openshift.io/unsupported-node-agent-server-args"
)

// Volume permissions
const (
	// Owner and Group can read; Public do not have any permissions
	DefaultSecretPermission = int32(0440)
	// Owner can read and write; Group and Public can read
	DefaultProjectedPermission = int32(0644)
)

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

// GetImagePullPolicy get imagePullPolicy for a container, based on its image, if an override is not provided.
// If override is provided, use the override imagePullPolicy.
// If image contains a sha256 or sha512 digest, use IfNotPresent; otherwise, Always.
// If an error occurs, Always is used.
// Reference: https://github.com/distribution/distribution/blob/v2.7.1/reference/reference.go
func GetImagePullPolicy(override *corev1.PullPolicy, image string) (corev1.PullPolicy, error) {
	if override != nil {
		return *override, nil
	}
	sha256regex, err := regexp.Compile("@sha256:[a-f0-9]{64}")
	if err != nil {
		return corev1.PullAlways, err
	}
	if sha256regex.Match([]byte(image)) {
		// image contains a sha256 digest
		return corev1.PullIfNotPresent, nil
	}
	sha512regex, err := regexp.Compile("@sha512:[a-f0-9]{128}")
	if err != nil {
		return corev1.PullAlways, err
	}
	if sha512regex.Match([]byte(image)) {
		// image contains a sha512 digest
		return corev1.PullIfNotPresent, nil
	}
	return corev1.PullAlways, nil
}

// GenerateCliArgsFromConfigMap generates CLI arguments from a ConfigMap.
//
// This function takes a ConfigMap and a CLI subcommand(s), and returns a slice of strings representing
// the CLI arguments from the subcommand(s) followed by the arguments from the ConfigMap.
// The function processes each key-value pair in the ConfigMap as follows:
//
//  1. If the ConfigMaps' key starts with single '-' or double '--', it is left unchanged.
//  2. If the key name does not start with `-` or `--`, then `--` is added as a prefix to the key.
//  3. If the ConfigMap value is "true" or "false" (case-insensitive), it is converted to lowercase
//     and used without single quotes surroundings (boolean value).
//  4. The formatted key-value pair is added to the result that is alphabetically sorted.
//
// Args:
//
//		configMap: A pointer to a corev1.ConfigMap containing key-value pairs.
//		cliSubCommand: The CLI subcommand(s) as a string, for example 'server'
//	                or 'node-agent', 'server'
//
// Returns:
//
//	A slice of strings representing the CLI arguments.
func GenerateCliArgsFromConfigMap(configMap *corev1.ConfigMap, cliSubCommand ...string) []string {

	var keyValueArgs []string

	// Iterate through each key-value pair in the ConfigMap
	for key, value := range configMap.Data {
		// Ensure the key is prefixed by "--" if it doesn't start with "--" or "-"
		if !strings.HasPrefix(key, "-") {
			key = fmt.Sprintf("--%s", key)
		}

		if strings.EqualFold(value, "true") || strings.EqualFold(value, "false") {
			// Convert true/false to lowercase if not surrounded by quotes - boolean
			value = strings.ToLower(value)
		}

		keyValueArgs = append(keyValueArgs, fmt.Sprintf("%s=%s", key, value))

	}
	// We ensure the flags are alphabetically sorted, so they
	// are always added to the cliSubCommand(s) the same way
	sort.Strings(keyValueArgs)

	// Append the formatted key-value pair to args
	cliSubCommand = append(cliSubCommand, keyValueArgs...)

	return cliSubCommand
}

// Apply Override unsupported Node agent Server Args
func ApplyUnsupportedServerArgsOverride(container *corev1.Container, unsupportedServerArgsCM corev1.ConfigMap, serverType string) error {

	switch serverType {
	case NodeAgent:
		// if server args is set, override the default server args
		container.Args = GenerateCliArgsFromConfigMap(&unsupportedServerArgsCM, "node-agent", "server")

	case Velero:
		// if server args is set, override the default server args
		container.Args = GenerateCliArgsFromConfigMap(&unsupportedServerArgsCM, "server")
	}
	return nil
}

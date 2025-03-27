package hcp

import (
	"context"
	"path/filepath"
	"time"

	hypershiftv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Constants
const (
	MCEName                 = "multicluster-engine"
	MCENamespace            = "multicluster-engine"
	MCEOperatorName         = "multicluster-engine-operator"
	MCEOperatorGroup        = "multicluster-engine-operatorgroup"
	HONamespace             = "hypershift"
	HypershiftOperatorName  = "operator"
	OCPMarketplaceNamespace = "openshift-marketplace"
	RHOperatorsNamespace    = "redhat-operators"
	MCEOperandName          = "mce-operand"

	ClustersNamespace       = "clusters"
	HostedClusterPrefix     = "test-hc"
	SampleETCDEncryptionKey = "7o9RQL/BlcNrBWfNBVrJg55oKrDDaDu2kfoULl9MNIE="
	HCOCPTestImage          = "quay.io/openshift-release-dev/ocp-release:4.18.6-multi"
)

// Template paths
var (
	MCEOperandManifest        = filepath.Join(getProjectRoot(), "tests/e2e/sample-applications/hostedcontrolplanes/mce/mce-operand.yaml")
	HCPNoneManifest           = filepath.Join(getProjectRoot(), "tests/e2e/sample-applications/hostedcontrolplanes/hypershift/hostedcluster-none.yaml")
	HCPAgentManifest          = filepath.Join(getProjectRoot(), "tests/e2e/sample-applications/hostedcontrolplanes/hypershift/hostedcluster-agent.yaml")
	PullSecretManifest        = filepath.Join(getProjectRoot(), "tests/e2e/sample-applications/hostedcontrolplanes/hypershift/hostedcluster-pull-secret.yaml")
	EtcdEncryptionKeyManifest = filepath.Join(getProjectRoot(), "tests/e2e/sample-applications/hostedcontrolplanes/hypershift/hostedcluster-etcd-enc-key.yaml")
	CapiProviderRoleManifest  = filepath.Join(getProjectRoot(), "tests/e2e/sample-applications/hostedcontrolplanes/hypershift/hostedcluster-agent-capi-role.yaml")
)

// Global variables
var (
	packageManifestGVR = schema.GroupVersionResource{
		Group:    "packages.operators.coreos.com",
		Version:  "v1",
		Resource: "packagemanifests",
	}

	mceGVR = schema.GroupVersionResource{
		Group:    "multicluster.openshift.io",
		Version:  "v1",
		Resource: "multiclusterengines",
	}

	capiAgentGVK = schema.GroupVersionKind{
		Group:   "capi-provider.agent-install.openshift.io",
		Version: "v1beta1",
		Kind:    "AgentCluster",
	}

	awsClusterGVK = schema.GroupVersionKind{
		Group:   "infrastructure.cluster.x-k8s.io",
		Version: "v1beta2",
		Kind:    "AWSCluster",
	}

	clusterGVK = schema.GroupVersionKind{
		Group:   "cluster.x-k8s.io",
		Version: "v1beta1",
		Kind:    "Cluster",
	}

	RequiredWorkingOperators = []string{
		"cluster-api",
		"control-plane-operator",
		"kube-apiserver",
		"kube-controller-manager",
		"kube-scheduler",
		"ignition-server",
		"cluster-image-registry-operator",
		"cluster-network-operator",
		"cluster-node-tuning-operator",
		"cluster-policy-controller",
		"cluster-storage-operator",
		"cluster-version-operator",
		"control-plane-pki-operator",
		"dns-operator",
		"hosted-cluster-config-operator",
		"ignition-server-proxy",
		"konnectivity-agent",
		"machine-approver",
		"oauth-openshift",
		"openshift-apiserver",
		"openshift-controller-manager",
		"openshift-oauth-apiserver",
		"openshift-route-controller-manager",
	}

	HCPIncludedNamespaces = []string{
		ClustersNamespace,
	}

	HCPIncludedResources = []string{
		"sa",
		"role",
		"rolebinding",
		"pod",
		"pvc",
		"pv",
		"configmap",
		"priorityclasses",
		"pdb",
		"hostedcluster",
		"nodepool",
		"secrets",
		"services",
		"deployments",
		"statefulsets",
		"hostedcontrolplane",
		"cluster",
		"awscluster",
		"awsmachinetemplate",
		"awsmachine",
		"machinedeployment",
		"machineset",
		"machine",
		"route",
		"clusterdeployment",
	}

	HCPExcludedResources = []string{}

	HCPErrorIgnorePatterns = []string{
		"-error-template",
	}

	// Timeout constants
	Wait10Min               = 10 * time.Minute
	WaitForNextCheckTimeout = 10 * time.Second
	ValidateHCPTimeout      = 25 * time.Minute
	HCPBackupTimeout        = 30 * time.Minute
)

// HCHandler handles operations related to HostedClusters
type HCHandler struct {
	Ctx            context.Context
	Client         client.Client
	HCOCPTestImage string
	HCPNamespace   string
	HostedCluster  *hypershiftv1.HostedCluster
}

type RequiredOperator struct {
	Name          string
	Namespace     string
	Channel       string
	Csv           string
	OperatorGroup string
}

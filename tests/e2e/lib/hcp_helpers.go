package lib

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	hypershiftv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

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

	AdditionalOperators = []string{
		"capi-provider",
		"catalog-operator",
		"certified-operators-catalog",
		"cloud-credential-operator",
		"cloud-network-config-controller",
		"community-operators-catalog",
		"olm-operator",
		"packageserver",
	}

	HCPIncludedNamespaces = []string{
		HCPNamespace,
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
)

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
	HostedClusterName       = "test-hc"
	HCPNamespace            = "clusters-test-hc"
	SampleETCDEncryptionKey = "7o9RQL/BlcNrBWfNBVrJg55oKrDDaDu2kfoULl9MNIE="
	HCOCPTestImage          = "quay.io/openshift-release-dev/ocp-release:4.18.6-x86_64"

	// Manifests
	MCEOperandManifest        = "./sample-applications/hostedcontrolplanes/mce/mce-operand.yaml"
	HCPNoneManifest           = "./sample-applications/hostedcontrolplanes/hypershift/hostedcluster-none.yaml"
	HCPAgentManifest          = "./sample-applications/hostedcontrolplanes/hypershift/hostedcluster-agent.yaml"
	PullSecretManifest        = "./sample-applications/hostedcontrolplanes/hypershift/hostedcluster-pull-secret.yaml"
	EtcdEncryptionKeyManifest = "./sample-applications/hostedcontrolplanes/hypershift/hostedcluster-etcd-enc-key.yaml"
	CapiProviderRoleManifest  = "./sample-applications/hostedcontrolplanes/hypershift/hostedcluster-agent-capi-role.yaml"
)

type RequiredOperator struct {
	Name          string
	Namespace     string
	Channel       string
	Csv           string
	OperatorGroup string
}

type HCHandler struct {
	Ctx            context.Context
	Client         client.Client
	HCOCPTestImage string
	HCPNamespace   string
	HostedCluster  *hypershiftv1.HostedCluster
}

// VerificationFunction is a function type that verifies the state of a resource
type VerificationFunction func(client.Client, string) error

func InstallRequiredOperators(ctx context.Context, c client.Client, reqOperators []RequiredOperator) (*HCHandler, error) {
	for _, op := range reqOperators {
		log.Printf("Installing operator %s", op.Name)
		err := op.InstallOperator(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("failed to install operator %s: %v", op.Name, err)
		}
	}

	return &HCHandler{
		Ctx:            ctx,
		Client:         c,
		HCOCPTestImage: HCOCPTestImage,
		HCPNamespace:   HCPNamespace,
	}, nil
}

func (op *RequiredOperator) InstallOperator(ctx context.Context, c client.Client) error {
	log.Printf("Getting PackageManifest for operator %s", op.Name)

	// Create an unstructured object for the PackageManifest
	pkg := &unstructured.Unstructured{}
	pkg.SetGroupVersionKind(packageManifestGVR.GroupVersion().WithKind("PackageManifest"))
	pkg.SetName(op.Name)
	pkg.SetNamespace(op.Namespace)

	err := c.Get(ctx, types.NamespacedName{Name: op.Name, Namespace: op.Namespace}, pkg)
	if err != nil {
		return fmt.Errorf("failed to get PackageManifest for operator %s: %v", op.Name, err)
	}

	log.Printf("Checking namespace for operator %s", op.Name)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: op.Namespace,
		},
	}

	tempNS := &corev1.Namespace{}
	if err := c.Get(ctx, types.NamespacedName{Name: op.Namespace}, tempNS); err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("Creating namespace for operator %s", op.Name)
			// Create the namespace if it doesn't exist

			err = c.Create(ctx, ns)
			if err != nil {
				return fmt.Errorf("failed to create namespace %s: %v", op.Namespace, err)
			}
		} else {
			return fmt.Errorf("failed to get namespace %s: %v", op.Namespace, err)
		}
	}

	opGroup := &operatorsv1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      op.OperatorGroup,
			Namespace: op.Namespace,
		},
		Spec: operatorsv1.OperatorGroupSpec{
			TargetNamespaces: []string{op.Namespace},
		},
	}

	log.Printf("Checking operator group for operator %s", op.Name)
	tempOpGroup := &operatorsv1.OperatorGroup{}
	if err := c.Get(ctx, types.NamespacedName{Name: op.OperatorGroup, Namespace: op.Namespace}, tempOpGroup); err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("Creating operator group for operator %s", op.Name)
			// Create the operator group
			err = c.Create(ctx, opGroup)
			if err != nil {
				return fmt.Errorf("failed to create operator group %s: %v", op.OperatorGroup, err)
			}
		} else {
			return fmt.Errorf("failed to get operator group %s: %v", op.OperatorGroup, err)
		}
	}

	// Create the subscription
	subscription := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      op.Name,
			Namespace: op.Namespace,
		},
		Spec: &operatorsv1alpha1.SubscriptionSpec{
			CatalogSource:          RHOperatorsNamespace,
			CatalogSourceNamespace: OCPMarketplaceNamespace,
			Package:                op.Name,
			InstallPlanApproval:    operatorsv1alpha1.ApprovalAutomatic,
		},
	}

	// If a channel is specified, use it
	if op.Channel != "" {
		subscription.Spec.Channel = op.Channel
	} else {
		// Get the default channel from the PackageManifest
		defaultChannel, ok, err := unstructured.NestedString(pkg.UnstructuredContent(), "status", "defaultChannel")
		if err != nil {
			return fmt.Errorf("failed to get default channel from PackageManifest: %v", err)
		}
		if !ok || defaultChannel == "" {
			return fmt.Errorf("no default channel found in PackageManifest for operator %s", op.Name)
		}
		subscription.Spec.Channel = defaultChannel
	}

	// If a CSV is specified, use it
	if op.Csv != "" {
		subscription.Spec.StartingCSV = op.Csv
	}

	log.Printf("Checking subscription for operator %s", op.Name)
	tempSub := &operatorsv1alpha1.Subscription{}
	if err := c.Get(ctx, types.NamespacedName{Name: op.Name, Namespace: op.Namespace}, tempSub); err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("Creating subscription for operator %s", op.Name)
			err = c.Create(ctx, subscription)
			if err != nil {
				return fmt.Errorf("failed to create subscription for operator %s: %v", op.Name, err)
			}
		} else {
			return fmt.Errorf("failed to get subscription for operator %s: %v", op.Name, err)
		}
	}

	return nil
}

func (op *HCHandler) DeployMCEManifest() error {
	log.Printf("Checking MCE manifest")

	// Create an unstructured object to check if the MCE operand exists
	mce := &unstructured.Unstructured{}
	mce.SetGroupVersionKind(mceGVR.GroupVersion().WithKind("MultiClusterEngine"))
	mce.SetName(MCEOperandName)
	mce.SetNamespace(MCENamespace)

	if err := op.Client.Get(op.Ctx, types.NamespacedName{Name: MCEOperandName, Namespace: MCENamespace}, mce); err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("Creating MCE manifest")
			err = ApplyYAMLTemplate(op.Ctx, op.Client, MCEOperandManifest, true, map[string]interface{}{
				"MCEOperandName":      MCEOperandName,
				"MCEOperandNamespace": MCENamespace,
			})
			if err != nil {
				return fmt.Errorf("failed to apply mce-operand from %s: %v", MCEOperandManifest, err)
			}
		}
	}

	return nil
}

func (op *HCHandler) DeployHCManifest(tmpl, provider string) (*hypershiftv1.HostedCluster, error) {
	log.Printf("Deploying HostedCluster manifest - %s", provider)
	// Create the clusters ns
	clustersNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ClustersNamespace,
		},
	}

	log.Printf("Creating clusters namespace")
	err := op.Client.Create(op.Ctx, clustersNS)
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create clusters namespace: %v", err)
		}
	}

	log.Printf("Getting pull secret")
	pullSecret, err := getPullSecret(op.Ctx, op.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull secret: %v", err)
	}

	log.Printf("Applying pull secret manifest")
	err = ApplyYAMLTemplate(op.Ctx, op.Client, PullSecretManifest, true, map[string]interface{}{
		"HostedClusterName": HostedClusterName,
		"ClustersNamespace": ClustersNamespace,
		"PullSecret":        base64.StdEncoding.EncodeToString([]byte(pullSecret)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to apply pull secret manifest from %s: %v", PullSecretManifest, err)
	}

	log.Printf("Applying etcd encryption key manifest")
	err = ApplyYAMLTemplate(op.Ctx, op.Client, EtcdEncryptionKeyManifest, true, map[string]interface{}{
		"HostedClusterName": HostedClusterName,
		"ClustersNamespace": ClustersNamespace,
		"EtcdEncryptionKey": SampleETCDEncryptionKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to apply etcd encryption key manifest from %s: %v", EtcdEncryptionKeyManifest, err)
	}

	if provider == "Agent" {
		log.Printf("Applying capi-provider-role manifest")
		err = ApplyYAMLTemplate(op.Ctx, op.Client, CapiProviderRoleManifest, true, map[string]interface{}{
			"ClustersNamespace": ClustersNamespace,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to apply capi-provider-role manifest from %s: %v", CapiProviderRoleManifest, err)
		}
	}

	// We don't want to override the hostedCluster manifest, if that cluster already exists means that something is wrong in the workflow
	log.Printf("Applying HostedCluster manifest - %s", provider)
	err = ApplyYAMLTemplate(op.Ctx, op.Client, tmpl, false, map[string]interface{}{
		"HostedClusterName": HostedClusterName,
		"ClustersNamespace": ClustersNamespace,
		"HCOCPTestImage":    op.HCOCPTestImage,
		"InfraIDSeed":       "test",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to apply hostedCluster manifest from %s: %v", tmpl, err)
	}

	log.Printf("Waiting for the hostedCluster to be present")
	// Wait for the hostedCluster to be present
	ctx, cancel := context.WithTimeout(op.Ctx, 10*time.Minute)
	defer cancel()

	hc := &hypershiftv1.HostedCluster{}
	err = wait.PollUntilContextCancel(ctx, 1*time.Second, true, func(ctx context.Context) (bool, error) {
		tempHostedCluster := &hypershiftv1.HostedCluster{}
		if err := op.Client.Get(ctx, types.NamespacedName{Namespace: ClustersNamespace, Name: HostedClusterName}, tempHostedCluster); err != nil {
			log.Printf("failed to get hosted cluster. Namespace: %s Name: %s: %v", ClustersNamespace, HostedClusterName, err)
			return false, nil
		}
		done := tempHostedCluster.Spec.Services != nil
		hc = tempHostedCluster
		return done, nil
	})

	log.Printf("Checking if the hostedCluster is present")
	if err != nil {
		return nil, fmt.Errorf("hostedcluster is empty: %w", err)
	}

	return hc, nil
}

// WaitForUnstructuredObject waits for an unstructured object to be deleted
func WaitForUnstructuredObject(ctx context.Context, c client.Client, obj *unstructured.Unstructured, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 1*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		log.Printf("\tWaiting for object %s in namespace %s to be deleted...", obj.GetName(), obj.GetNamespace())
		newObj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       obj.GetKind(),
				"apiVersion": obj.GetAPIVersion(),
				"metadata": map[string]interface{}{
					"name":      obj.GetName(),
					"namespace": obj.GetNamespace(),
				},
			},
		}
		err := c.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, newObj)
		log.Printf("\tObject %s exists in namespace %s: %v", obj.GetName(), obj.GetNamespace(), err)
		return apierrors.IsNotFound(err), nil
	})
}

// ApplyYAMLTemplate reads a YAML template file, renders it with the given data, and applies it using the client
func ApplyYAMLTemplate(ctx context.Context, c client.Client, manifestPath string, override bool, data interface{}) error {
	// Read the manifest
	log.Printf("\tReading YAML template %s", filepath.Base(manifestPath))
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest from %s: %v", manifestPath, err)
	}

	// Parse the manifest
	log.Printf("\tParsing manifest %s", filepath.Base(manifestPath))
	tmpl, err := template.New("manifest").Parse(string(manifest))
	if err != nil {
		return fmt.Errorf("failed to parse manifest from %s: %v", manifestPath, err)
	}

	// Execute the manifest
	log.Printf("\tExecuting manifest %s", filepath.Base(manifestPath))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute manifest from %s: %v", manifestPath, err)
	}

	// Create a decoder for YAML
	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	// Decode the YAML into an unstructured object
	log.Printf("\tDecoding YAML %s", filepath.Base(manifestPath))
	obj := &unstructured.Unstructured{}
	_, _, err = decoder.Decode(buf.Bytes(), nil, obj)
	if err != nil {
		return fmt.Errorf("failed to decode YAML from %s: %v", manifestPath, err)
	}

	// Apply the object using the client
	log.Printf("\tApplying object %s", filepath.Base(manifestPath))
	err = c.Create(ctx, obj)
	if err != nil {
		if override && apierrors.IsAlreadyExists(err) {
			log.Printf("\tObject already exists, overriding...")
			err = c.Update(ctx, obj)
			if err != nil {
				return fmt.Errorf("failed to update object from %s: %v", manifestPath, err)
			}
		} else {
			return fmt.Errorf("failed to create object from %s: %v", manifestPath, err)
		}
	}

	log.Printf("\tObject applied successfully")

	return nil
}

// getPullSecret gets the pull secret from the openshift-config namespace
func getPullSecret(ctx context.Context, c client.Client) (string, error) {
	secret := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{Name: "pull-secret", Namespace: "openshift-config"}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to get pull secret: %v", err)
	}
	if secret.Data == nil || len(secret.Data) == 0 {
		return "", fmt.Errorf("pull secret data is empty")
	}
	dockerConfig, ok := secret.Data[".dockerconfigjson"]
	if !ok {
		return "", fmt.Errorf("pull secret does not contain .dockerconfigjson key")
	}
	return string(dockerConfig), nil
}

// ValidateHCP returns a VerificationFunction that checks if the HostedCluster pods are running
func ValidateHCP(timeout time.Duration, deployments []string) func(client.Client, string) error {
	if len(deployments) == 0 {
		deployments = RequiredWorkingOperators
	}

	if timeout == 0 {
		timeout = time.Minute * 15
	}

	return func(ocClient client.Client, _ string) error {
		// First validate that no deployment is in both slices
		duplicates := make(map[string]bool)
		for _, op := range RequiredWorkingOperators {
			duplicates[op] = true
		}
		for _, op := range AdditionalOperators {
			if duplicates[op] {
				return fmt.Errorf("deployment %s cannot be in both RequiredWorkingOperators and AdditionalOperators", op)
			}
		}

		// Validate ETCD STS
		wait.PollUntilContextTimeout(context.Background(), time.Second*10, timeout, true, func(ctx context.Context) (bool, error) {
			etcdSts := &appsv1.StatefulSet{}
			err := ocClient.Get(ctx, types.NamespacedName{Name: "etcd", Namespace: HCPNamespace}, etcdSts)
			if err != nil {
				return false, fmt.Errorf("failed to get etcd statefulset: %v", err)
			}
			if etcdSts.Status.Replicas != etcdSts.Status.ReadyReplicas {
				log.Printf("\tETCD STS is not ready, waiting for it to be ready")
				return false, nil
			}
			log.Printf("\tETCD STS is ready")
			return true, nil
		})

		// List all deployments in the namespace
		deploymentList := &appsv1.DeploymentList{}
		err := ocClient.List(context.Background(), deploymentList, client.InNamespace(HCPNamespace))
		if err != nil {
			return fmt.Errorf("failed to list deployments in namespace %s: %v", HCPNamespace, err)
		}

		// Create maps for faster lookup
		requiredOperators := make(map[string]bool)
		additionalOperators := make(map[string]bool)
		for _, op := range RequiredWorkingOperators {
			requiredOperators[op] = true
		}
		for _, op := range AdditionalOperators {
			additionalOperators[op] = true
		}

		// Check each deployment
		for _, deployment := range deploymentList.Items {
			deploymentName := deployment.Name
			log.Printf("\tValidating deployment: %s", deploymentName)

			// Determine deployment type and handle accordingly
			switch {
			case additionalOperators[deploymentName]:
				if deployment.Status.AvailableReplicas != deployment.Status.Replicas || deployment.Status.Replicas == 0 {
					log.Printf("\tWARNING: Additional deployment %s is not ready (Available: %d, Replicas: %d)",
						deploymentName, deployment.Status.AvailableReplicas, deployment.Status.Replicas)
					continue
				}
				log.Printf("\tDeployment %s is ready", deploymentName)

			case requiredOperators[deploymentName]:
				err := wait.PollUntilContextTimeout(context.Background(), time.Second*10, timeout, true, func(ctx context.Context) (bool, error) {
					deploymentObj := &appsv1.Deployment{}
					err := ocClient.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: HCPNamespace}, deploymentObj)
					if err != nil {
						if apierrors.IsNotFound(err) {
							log.Printf("\tDeployment %s not found yet, continuing to wait", deploymentName)
							return false, nil
						}
						return false, err
					}

					if deploymentObj.Status.AvailableReplicas == deploymentObj.Status.Replicas && deploymentObj.Status.Replicas > 0 {
						log.Printf("\tDeployment %s is ready", deploymentName)
						return true, nil
					}

					log.Printf("\tDeployment %s not ready yet (Available: %d, Replicas: %d)",
						deploymentName, deploymentObj.Status.AvailableReplicas, deploymentObj.Status.Replicas)

					return false, fmt.Errorf("required deployment %s is not ready", deploymentName)
				})

				if err != nil {
					return fmt.Errorf("deployment %s failed to become ready: %v", deploymentName, err)
				}

			default: // Unknown deployment
				if deployment.Status.AvailableReplicas != deployment.Status.Replicas || deployment.Status.Replicas == 0 {
					log.Printf("\tWARNING: Unknown deployment %s is not ready (Available: %d, Replicas: %d)",
						deploymentName, deployment.Status.AvailableReplicas, deployment.Status.Replicas)
				} else {
					log.Printf("\tDeployment %s is ready", deploymentName)
				}
			}
		}

		return nil
	}
}

// WaitForHCPDeletion waits for the HCP to be deleted, if it's not deleted after the specified timeout,
// it will try to nuke it. Returns an error if the HCP is still present after 5 minutes of nuking.
func WaitForHCPDeletion(ctx context.Context, h *HCHandler, hcp *hypershiftv1.HostedControlPlane, timeout ...time.Duration) error {
	// Use provided timeout or default to 3 minutes
	waitTimeout := time.Minute * 3

	if len(timeout) > 0 {
		waitTimeout = timeout[0]
	}

	// First try normal deletion
	log.Printf("\tWaiting for the HCP to be deleted")
	err := wait.PollUntilContextTimeout(ctx, time.Second*10, waitTimeout, true, func(ctx context.Context) (bool, error) {
		return IsHCPDeleted(h, hcp), nil
	})

	if err == nil {
		log.Printf("\tHCP deleted normally")
		return nil
	}

	// If deletion failed, try to nuke it
	log.Printf("\tHCP was not deleted after %v", waitTimeout)
	err = h.NukeHostedCluster()
	if err != nil {
		return fmt.Errorf("failed to nuke HCP: %v", err)
	}

	// Wait for deletion after nuke
	log.Printf("\tWaiting for HCP to be deleted after nuke")
	err = wait.PollUntilContextTimeout(ctx, time.Second*10, waitTimeout, true, func(ctx context.Context) (bool, error) {
		return IsHCPDeleted(h, hcp), nil
	})

	if err != nil {
		return fmt.Errorf("HCP still present after %v of nuking: %v", waitTimeout, err)
	}

	log.Printf("\tHCP deleted after nuke")
	return nil
}

func RemoveHCP(h *HCHandler) error {
	deleteOptions := &client.DeleteOptions{
		GracePeriodSeconds: ptr.To(int64(0)),
	}

	// Delete the hostedCluster
	log.Printf("\tRemoving hostedCluster")
	err := h.Client.Delete(h.Ctx, h.HostedCluster, deleteOptions)
	if err != nil {
		return fmt.Errorf("failed to delete HostedCluster: %v", err)
	}
	log.Printf("\tHostedCluster deleted")

	// Delete HCP Namespace
	log.Printf("\tRemoving HCP namespace")
	err = h.Client.Delete(h.Ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: h.HCPNamespace,
		},
	}, deleteOptions)
	if err != nil {
		return fmt.Errorf("failed to delete HCP namespace: %v", err)
	}
	log.Printf("\tHCP namespace deleted")

	// Delete HCP
	log.Printf("\tRemoving HCP")
	hcp := hypershiftv1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.HostedCluster.Name,
			Namespace: h.HCPNamespace,
		},
	}
	err = h.Client.Delete(h.Ctx, &hcp, deleteOptions)
	if err != nil {
		return fmt.Errorf("failed to delete HostedControlPlane: %v", err)
	}

	// Wait for HCP deletion with timeout
	ctx, cancel := context.WithTimeout(h.Ctx, time.Minute*8)
	defer cancel()
	err = WaitForHCPDeletion(ctx, h, &hcp)
	if err != nil {
		return fmt.Errorf("failed to delete HCP: %v", err)
	}
	log.Printf("\tHCP deleted")

	// Delete HC Secrets
	log.Printf("\tRemoving HC secrets")
	err = h.Client.Delete(h.Ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-pull-secret", HostedClusterName),
			Namespace: ClustersNamespace,
		},
	}, deleteOptions)
	if err != nil {
		return fmt.Errorf("failed to delete pull secret: %v", err)
	}
	log.Printf("\tHC Pull secret deleted")

	// Delete HC Etcd Encryption Key
	log.Printf("\tRemoving HC etcd encryption key")
	err = h.Client.Delete(h.Ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-etcd-encryption-key", HostedClusterName),
			Namespace: ClustersNamespace,
		},
	}, deleteOptions)
	if err != nil {
		return fmt.Errorf("failed to delete etcd encryption key: %v", err)
	}
	log.Printf("\tHC etcd encryption key deleted")

	// Wait for the HC to be deleted
	log.Printf("\tWaiting for the HC to be deleted")
	wait.PollUntilContextTimeout(h.Ctx, time.Second*5, time.Minute*10, true, func(ctx context.Context) (bool, error) {
		log.Printf("\tAttempting to verify HC deletion...")
		result := IsHCDeleted(h)
		log.Printf("\tHC deletion check result: %v", result)
		return result, nil
	})

	log.Printf("\tHC deleted")

	return nil
}

func RemoveMCE(h *HCHandler) error {
	// Delete the MCE operand
	log.Printf("\tRemoving MCE operand")
	mce := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "MultiClusterEngine",
			"metadata": map[string]interface{}{
				"name":      MCEOperandName,
				"namespace": MCENamespace,
			},
		},
	}
	mce.SetGroupVersionKind(mceGVR.GroupVersion().WithKind("MultiClusterEngine"))
	err := h.Client.Delete(h.Ctx, mce)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete mce operand: %v", err)
	}
	log.Printf("\tMCE operand deleted")

	// Delete operatorGroup
	log.Printf("\tRemoving operatorGroup")
	err = h.Client.Delete(h.Ctx, &operatorsv1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MCEOperatorGroup,
			Namespace: MCENamespace,
		},
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete operatorGroup: %v", err)
	}
	log.Printf("\tOperatorGroup deleted")

	// Delete subscription
	log.Printf("\tRemoving subscription")
	err = h.Client.Delete(h.Ctx, &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MCEOperatorName,
			Namespace: MCENamespace,
		},
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete subscription: %v", err)
	}
	log.Printf("\tSubscription deleted")

	return nil
}

func IsHCPDeleted(h *HCHandler, hcp *hypershiftv1.HostedControlPlane) bool {
	if hcp == nil {
		log.Printf("\tNo HCP provided, assuming deleted")
		return true
	}
	log.Printf("\tChecking if HCP %s is deleted...", hcp.Name)
	newHCP := &hypershiftv1.HostedControlPlane{}
	err := h.Client.Get(h.Ctx, types.NamespacedName{Namespace: hcp.Namespace, Name: hcp.Name}, newHCP, &client.GetOptions{
		Raw: &metav1.GetOptions{},
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("\tHCP %s is confirmed deleted", hcp.Name)
			return true
		}
		log.Printf("\tHCP %s deletion check failed with error: %v", hcp.Name, err)
		return false
	}
	log.Printf("\tHCP %s still exists", hcp.Name)
	return false
}

func IsHCDeleted(h *HCHandler) bool {
	if h.HostedCluster == nil {
		log.Printf("\tNo HostedCluster provided, assuming deleted")
		return true
	}
	log.Printf("\tChecking if HC %s is deleted...", h.HostedCluster.Name)
	newHC := &hypershiftv1.HostedCluster{}
	err := h.Client.Get(h.Ctx, types.NamespacedName{Namespace: h.HostedCluster.Namespace, Name: h.HostedCluster.Name}, newHC, &client.GetOptions{
		Raw: &metav1.GetOptions{},
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("\tHC %s is confirmed deleted", h.HostedCluster.Name)
			return true
		}
		log.Printf("\tHC %s deletion check failed with error: %v", h.HostedCluster.Name, err)
		return false
	}
	log.Printf("\tHC %s still exists", h.HostedCluster.Name)
	return false
}

func AddHCPPluginToDPA(h *HCHandler, namespace, name string, overrides bool) error {
	addHCPlugin := true

	log.Printf("Adding HCP default plugin to DPA")
	dpa := &oadpv1alpha1.DataProtectionApplication{}
	err := h.Client.Get(h.Ctx, types.NamespacedName{Namespace: namespace, Name: name}, dpa)
	if err != nil {
		return err
	}

	// Check if the hypershift plugin is already in the default plugins
	for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
		if plugin == oadpv1alpha1.DefaultPluginHypershift {
			log.Printf("HCP plugin already in DPA")
			if overrides {
				log.Printf("Override set to true, removing HCP plugin from DPA")
				addHCPlugin = false
				break
			}
			return nil
		}
	}

	if addHCPlugin {
		dpa.Spec.Configuration.Velero.DefaultPlugins = append(dpa.Spec.Configuration.Velero.DefaultPlugins, oadpv1alpha1.DefaultPluginHypershift)
	}

	if overrides {
		dpa.Spec.UnsupportedOverrides = map[oadpv1alpha1.UnsupportedImageKey]string{
			// Now we are using the latest image from the hypershift-oadp-plugin repo but at some point we
			// will pin to a specific image version, so this will be relevant.
			oadpv1alpha1.HypershiftPluginImageKey: "quay.io/hypershift/hypershift-oadp-plugin:latest",
		}
	}

	err = h.Client.Update(h.Ctx, dpa)
	if err != nil {
		return fmt.Errorf("failed to update DPA: %v", err)
	}
	log.Printf("HCP plugin added to DPA")
	return nil
}

func RemoveHCPPluginFromDPA(h *HCHandler, namespace, name string) error {
	log.Printf("Removing HCP plugin from DPA")
	dpa := &oadpv1alpha1.DataProtectionApplication{}
	err := h.Client.Get(h.Ctx, types.NamespacedName{Namespace: namespace, Name: name}, dpa)
	if err != nil {
		return err
	}
	delete(dpa.Spec.UnsupportedOverrides, oadpv1alpha1.HypershiftPluginImageKey)
	// remove hypershift plugin from default plugins
	for i, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
		if plugin == oadpv1alpha1.DefaultPluginHypershift {
			dpa.Spec.Configuration.Velero.DefaultPlugins = append(dpa.Spec.Configuration.Velero.DefaultPlugins[:i], dpa.Spec.Configuration.Velero.DefaultPlugins[i+1:]...)
			break
		}
	}
	err = h.Client.Update(h.Ctx, dpa)
	if err != nil {
		return fmt.Errorf("failed to update DPA: %v", err)
	}
	log.Printf("HCP plugin removed from DPA")
	return nil
}

func IsHCPPluginAdded(c client.Client, namespace, name string) bool {
	dpa := &oadpv1alpha1.DataProtectionApplication{}
	err := c.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, dpa)
	if err != nil {
		return false
	}

	if dpa.Spec.Configuration == nil || dpa.Spec.Configuration.Velero == nil {
		return false
	}

	for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
		if plugin == oadpv1alpha1.DefaultPluginHypershift {
			return true
		}
	}

	return false
}

func FilterErrorLogs(logs []string) []string {
	filteredLogs := []string{}
	for _, logEntry := range logs {
		shouldInclude := true
		for _, pattern := range HCPErrorIgnorePatterns {
			if strings.Contains(logEntry, pattern) {
				shouldInclude = false
				break
			}
		}
		if shouldInclude {
			filteredLogs = append(filteredLogs, logEntry)
		}
	}
	return filteredLogs
}

func (h *HCHandler) NukeHostedCluster() error {
	// List of resource types to check
	log.Printf("\tNuking HostedCluster")
	resourceTypes := []struct {
		kind string
		gvk  schema.GroupVersionKind
	}{
		{"HostedControlPlane", hypershiftv1.GroupVersion.WithKind("HostedControlPlane")},
		{"Cluster", clusterGVK},
		{"AWSCluster", awsClusterGVK},
		{"AgentCluster", capiAgentGVK},
	}

	for _, rt := range resourceTypes {
		obj := &unstructured.UnstructuredList{}
		obj.SetGroupVersionKind(rt.gvk)

		if err := h.Client.List(h.Ctx, obj, &client.ListOptions{Namespace: h.HCPNamespace}); err != nil {
			log.Printf("Error listing %s: %v", rt.kind, err)
			continue
		}

		for _, item := range obj.Items {
			if len(item.GetFinalizers()) > 0 {
				log.Printf("\tNUKE: Removing finalizers from %s %s", rt.kind, item.GetName())
				item.SetFinalizers([]string{})
				if err := h.Client.Update(h.Ctx, &item); err != nil {
					return fmt.Errorf("\tNUKE: Error removing finalizers from %s %s: %v", rt.kind, item.GetName(), err)
				}
			}
		}
	}

	return nil
}

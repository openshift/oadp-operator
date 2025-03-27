package lib

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"encoding/base64"

	"github.com/onsi/gomega"
	hypershiftv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// Definir el GVR del PackageManifest
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

	RequiredWorkingOperators = []string{
		"cluster-api",
		"control-plane-operator",
		"kube-apiserver",
		"kube-controller-manager",
		"kube-scheduler",
		"ignition-server",
		"catalog-operator",
		"cluster-image-registry-operator",
		"cluster-network-operator",
		"cluster-node-tuning-operator",
		"cluster-policy-controller",
		"cluster-storage-operator",
		"cluster-version-operator",
		"control-plane-pki-operator",
		"csi-snapshot-controller",
		"csi-snapshot-controller-operator",
		"dns-operator",
		"hosted-cluster-config-operator",
		"ignition-server-proxy",
		"ingress-operator",
		"konnectivity-agent",
		"machine-approver",
		"oauth-openshift",
		"olm-operator",
		"openshift-apiserver",
		"openshift-controller-manager",
		"openshift-oauth-apiserver",
		"openshift-route-controller-manager",
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
	HCPManifest               = "./sample-applications/hostedcontrolplanes/hypershift/hostedcluster.yaml"
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
}

// VerificationFunction is a function type that verifies the state of a resource
type VerificationFunction func(client.Client, string) error

func InstallRequiredOperators(ctx context.Context, client client.Client, reqOperators []RequiredOperator) (*HCHandler, error) {
	for _, op := range reqOperators {
		log.Printf("Installing operator %s", op.Name)
		err := op.InstallOperator(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("failed to install operator %s: %v", op.Name, err)
		}
	}

	return &HCHandler{
		Ctx:            ctx,
		Client:         client,
		HCOCPTestImage: HCOCPTestImage,
	}, nil
}

func (op *RequiredOperator) InstallOperator(ctx context.Context, client client.Client) error {
	log.Printf("Getting PackageManifest for operator %s", op.Name)

	// Create an unstructured object for the PackageManifest
	pkg := &unstructured.Unstructured{}
	pkg.SetGroupVersionKind(packageManifestGVR.GroupVersion().WithKind("PackageManifest"))
	pkg.SetName(op.Name)
	pkg.SetNamespace(op.Namespace)

	err := client.Get(ctx, types.NamespacedName{Name: op.Name, Namespace: op.Namespace}, pkg)
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
	if err := client.Get(ctx, types.NamespacedName{Name: op.Namespace}, tempNS); err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("Creating namespace for operator %s", op.Name)
			// Create the namespace if it doesn't exist

			err = client.Create(ctx, ns)
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
	if err := client.Get(ctx, types.NamespacedName{Name: op.OperatorGroup, Namespace: op.Namespace}, tempOpGroup); err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("Creating operator group for operator %s", op.Name)
			// Create the operator group
			err = client.Create(ctx, opGroup)
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
	if err := client.Get(ctx, types.NamespacedName{Name: op.Name, Namespace: op.Namespace}, tempSub); err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("Creating subscription for operator %s", op.Name)
			err = client.Create(ctx, subscription)
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

func (op *HCHandler) DeployHCManifest() (*hypershiftv1.HostedCluster, error) {
	log.Printf("Deploying HostedCluster manifest")
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
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to apply pull secret manifest from %s: %v", HCPManifest, err)

	log.Printf("Applying etcd encryption key manifest")
	err = ApplyYAMLTemplate(op.Ctx, op.Client, EtcdEncryptionKeyManifest, true, map[string]interface{}{
		"HostedClusterName": HostedClusterName,
		"ClustersNamespace": ClustersNamespace,
		"EtcdEncryptionKey": SampleETCDEncryptionKey,
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to apply etcd encryption key manifest from %s: %v", HCPManifest, err)

	log.Printf("Applying capi-provider-role manifest")
	err = ApplyYAMLTemplate(op.Ctx, op.Client, CapiProviderRoleManifest, true, map[string]interface{}{
		"ClustersNamespace": ClustersNamespace,
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to apply capi-provider-role manifest from %s: %v", HCPManifest, err)

	log.Printf("Applying HostedCluster manifest")
	err = ApplyYAMLTemplate(op.Ctx, op.Client, HCPManifest, true, map[string]interface{}{
		"HostedClusterName": HostedClusterName,
		"ClustersNamespace": ClustersNamespace,
		"HCOCPTestImage":    op.HCOCPTestImage,
		"InfraIDSeed":       "test",
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to apply hostedCluster manifest from %s: %v", HCPManifest, err)

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
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "hostedcluster is empty: %w", err)

	return hc, nil
}

func (h *HCHandler) ValidateMCE() error {
	// Wait for pods to be running
	log.Printf("Validating MCE")
	gomega.Eventually(IsDeploymentReady(h.Client, MCENamespace, MCEOperatorName), time.Minute*5, time.Second*5).Should(gomega.BeTrue())
	return nil
}

func (h *HCHandler) ValidateHO() error {
	// Wait for pods to be running
	log.Printf("Validating Hypershift Operator")
	gomega.Eventually(IsDeploymentReady(h.Client, HONamespace, HypershiftOperatorName), time.Minute*5, time.Second*5).Should(gomega.BeTrue())
	return nil
}

// WaitForUnstructuredObject waits for an unstructured object to be deleted
func WaitForUnstructuredObject(ctx context.Context, client client.Client, obj *unstructured.Unstructured) error {
	return wait.PollUntilContextTimeout(ctx, 1*time.Second, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
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
		err := client.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, newObj)
		log.Printf("\tObject %s exists in namespace %s: %v", obj.GetName(), obj.GetNamespace(), err)
		return apierrors.IsNotFound(err), nil
	})
}

// ApplyYAMLTemplate reads a YAML template file, renders it with the given data, and applies it using the client
func ApplyYAMLTemplate(ctx context.Context, client client.Client, manifestPath string, override bool, data interface{}) error {
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
	err = client.Create(ctx, obj)
	if err != nil {
		if override && apierrors.IsAlreadyExists(err) {
			log.Printf("\tObject already exists, overriding...")
			err = client.Delete(ctx, obj)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to delete object from %s: %v", manifestPath, err)
			// wait for the object to be deleted
			err = WaitForUnstructuredObject(ctx, client, obj)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to wait for object deletion from %s: %v", manifestPath, err)
			err = client.Create(ctx, obj)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to create object from %s: %v", manifestPath, err)
		} else {
			return fmt.Errorf("failed to create object from %s: %v", manifestPath, err)
		}
	}

	log.Printf("\tObject applied successfully")

	return nil
}

// getPullSecret gets the pull secret from the openshift-config namespace
func getPullSecret(ctx context.Context, client client.Client) (string, error) {
	secret := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{Name: "pull-secret", Namespace: "openshift-config"}, secret)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get pull secret: %v", err)
	gomega.Expect(secret.Data).ToNot(gomega.BeEmpty(), "pull secret data is empty")

	return string(secret.Data[".dockerconfigjson"]), nil
}

// ValidateHCP returns a VerificationFunction that checks if the HostedCluster pods are running
func ValidateHCP() func(client.Client, string) error {
	return func(ocClient client.Client, _ string) error {
		// Wait for HCP critical deployments to be ready
		for _, deployment := range RequiredWorkingOperators {
			log.Printf("\tValidating deployment: %s", deployment)
			gomega.Eventually(IsDeploymentReady(ocClient, HCPNamespace, deployment), time.Minute*10, time.Second*10).Should(gomega.BeTrue())
			log.Printf("\tDeployment %s is ready", deployment)
		}

		return nil
	}
}

func RemoveHCP(h *HCHandler, hc *hypershiftv1.HostedCluster) error {
	gracePeriod := int64(0)
	deleteOptions := &client.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	}

	// Delete the hostedCluster
	log.Printf("\tRemoving hostedCluster")
	err := h.Client.Delete(h.Ctx, hc, deleteOptions)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to delete HostedCluster: %v", err)
	log.Printf("\tHostedCluster deleted")

	// Delete HCP
	log.Printf("\tRemoving HCP")
	hcp := hypershiftv1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hc.Name,
			Namespace: HCPNamespace,
		},
	}
	err = h.Client.Delete(h.Ctx, &hcp, deleteOptions)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to delete HostedControlPlane: %v", err)

	// Wait for the HCP to be deleted
	log.Printf("\tWaiting for the HCP to be deleted")
	gomega.Eventually(IsHCPDeleted(h, &hcp), time.Minute*10, time.Second*10).Should(gomega.BeTrue())
	log.Printf("\tHCP deleted")

	// Delete HCP Namespace
	log.Printf("\tRemoving HCP namespace")
	err = h.Client.Delete(h.Ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: HCPNamespace,
		},
	}, deleteOptions)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to delete HCP namespace: %v", err)
	log.Printf("\tHCP namespace deleted")

	// Delete HC Secrets
	log.Printf("\tRemoving HC secrets")
	err = h.Client.Delete(h.Ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-pull-secret", HostedClusterName),
			Namespace: ClustersNamespace,
		},
	}, deleteOptions)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to delete pull secret: %v", err)
	log.Printf("\tHC Pull secret deleted")

	// Delete HC Etcd Encryption Key
	log.Printf("\tRemoving HC etcd encryption key")
	err = h.Client.Delete(h.Ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-etcd-encryption-key", HostedClusterName),
			Namespace: ClustersNamespace,
		},
	}, deleteOptions)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to delete etcd encryption key: %v", err)
	log.Printf("\tHC etcd encryption key deleted")

	// Wait for the HC to be deleted
	log.Printf("\tWaiting for the HC to be deleted")
	gomega.Eventually(IsHCDeleted(h, hc), time.Minute*10, time.Second*10).Should(gomega.BeTrue())
	log.Printf("\tHC deleted")

	return nil
}

func RemoveMCE(h *HCHandler) error {
	// Delete the MCE operand
	log.Printf("\tRemoving MCE operand")
	err := h.Client.Delete(h.Ctx, &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "MultiClusterEngine",
			"metadata": map[string]interface{}{
				"name":      MCEOperandName,
				"namespace": MCENamespace,
			},
		},
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to delete mce operand: %v", err)
	log.Printf("\tMCE operand deleted")

	// Delete operatorGroup
	log.Printf("\tRemoving operatorGroup")
	err = h.Client.Delete(h.Ctx, &operatorsv1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MCEOperatorGroup,
			Namespace: MCENamespace,
		},
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to delete operatorGroup: %v", err)
	log.Printf("\tOperatorGroup deleted")

	// Delete subscription
	log.Printf("\tRemoving subscription")
	err = h.Client.Delete(h.Ctx, &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MCEOperatorName,
			Namespace: MCENamespace,
		},
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to delete subscription: %v", err)
	log.Printf("\tSubscription deleted")

	return nil
}

func IsHCPDeleted(h *HCHandler, hcp *hypershiftv1.HostedControlPlane) bool {
	err := h.Client.Get(h.Ctx, types.NamespacedName{Namespace: hcp.Namespace, Name: hcp.Name}, hcp)
	return apierrors.IsNotFound(err)
}

func IsHCDeleted(h *HCHandler, hc *hypershiftv1.HostedCluster) bool {
	err := h.Client.Get(h.Ctx, types.NamespacedName{Namespace: hc.Namespace, Name: hc.Name}, hc)
	return apierrors.IsNotFound(err)
}

func AddHCPPluginToDPA(h *HCHandler, namespace, name string, overrides bool) error {
	log.Printf("Adding HCP plugin to DPA")
	dpa := &oadpv1alpha1.DataProtectionApplication{}
	err := h.Client.Get(h.Ctx, types.NamespacedName{Namespace: namespace, Name: name}, dpa)
	if err != nil {
		return err
	}

	dpa.Spec.Configuration.Velero.DefaultPlugins = append(dpa.Spec.Configuration.Velero.DefaultPlugins, oadpv1alpha1.DefaultPluginHypershift)
	if overrides {
		dpa.Spec.UnsupportedOverrides = map[oadpv1alpha1.UnsupportedImageKey]string{
			oadpv1alpha1.HypershiftPluginImageKey: "quay.io/hypershift/hypershift-oadp-plugin:latest",
		}
	}

	err = h.Client.Update(h.Ctx, dpa)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to update DPA: %v", err)
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
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to update DPA: %v", err)
	log.Printf("HCP plugin removed from DPA")
	return nil
}

func IsHCPPluginAdded(client client.Client, namespace, name string) bool {
	dpa := &oadpv1alpha1.DataProtectionApplication{}
	err := client.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, dpa)
	if err != nil {
		return false
	}

	for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
		if plugin == oadpv1alpha1.DefaultPluginHypershift {
			return true
		}
	}

	return false
}

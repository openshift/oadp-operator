package lib

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VirtOperator struct {
	Client    client.Client
	Clientset *kubernetes.Clientset
	Dynamic   dynamic.Interface
	Namespace string
	Csv       string
	Version   *version.Version
}

// GetVirtOperator fills out a new VirtOperator
func GetVirtOperator(client client.Client, clientset *kubernetes.Clientset, dynamicClient dynamic.Interface) (*VirtOperator, error) {
	namespace := "openshift-cnv"

	csv, version, err := GetCsvFromPackageManifest(dynamicClient, "kubevirt-hyperconverged")
	if err != nil {
		log.Printf("Failed to get CSV from package manifest")
		return nil, err
	}

	v := &VirtOperator{
		Client:    client,
		Clientset: clientset,
		Dynamic:   dynamicClient,
		Namespace: namespace,
		Csv:       csv,
		Version:   version,
	}

	return v, nil
}

// GetCsvFromPackageManifest returns the current CSV from the first channel
// in the given PackageManifest name. Uses the dynamic client because adding
// the real PackageManifest API from OLM was actually more work than this.
func GetCsvFromPackageManifest(dynamicClient dynamic.Interface, name string) (string, *version.Version, error) {
	resourceId := schema.GroupVersionResource{
		Group:    "packages.operators.coreos.com",
		Resource: "packagemanifests",
		Version:  "v1",
	}

	log.Println("Getting packagemanifest...")
	unstructuredManifest, err := dynamicClient.Resource(resourceId).Namespace("default").Get(context.Background(), name, v1.GetOptions{})
	if err != nil {
		log.Printf("Error getting packagemanifest %s: %v", name, err)
		return "", nil, err
	}

	log.Println("Extracting channels...")
	channels, ok, err := unstructured.NestedSlice(unstructuredManifest.UnstructuredContent(), "status", "channels")
	if err != nil {
		log.Printf("Error getting channels from packagemanifest: %v", err)
		return "", nil, err
	}
	if !ok {
		return "", nil, errors.New("failed to get channels list from " + name + " packagemanifest")
	}
	if len(channels) < 1 {
		return "", nil, errors.New("no channels listed in package manifest " + name)
	}

	firstChannel, ok := channels[0].(map[string]interface{})
	if !ok {
		return "", nil, errors.New("failed to read first channel from package manifest " + name)
	}

	csv, ok, err := unstructured.NestedString(firstChannel, "currentCSV")
	if err != nil {
		return "", nil, err
	}
	if !ok {
		return "", nil, errors.New("failed to get current CSV from " + name + " packagemanifest")
	}
	log.Printf("Current CSV is: %s", csv)

	versionString, ok, err := unstructured.NestedString(firstChannel, "currentCSVDesc", "version")
	if err != nil {
		return "", nil, err
	}
	if !ok {
		return "", nil, errors.New("failed to get current operator version from " + name + " packagemanifest")
	}
	log.Printf("Current operator version is: %s", versionString)

	version, err := version.ParseGeneric(versionString)
	if err != nil {
		return "", nil, err
	}

	return csv, version, nil
}

// Checks the existence of the operator's target namespace
func (v *VirtOperator) checkNamespace() bool {
	// First check that the namespace exists
	exists, _ := DoesNamespaceExist(v.Clientset, v.Namespace)
	return exists
}

// Checks for the existence of the virtualization operator group
func (v *VirtOperator) checkOperatorGroup() bool {
	group := operatorsv1.OperatorGroup{}
	err := v.Client.Get(context.TODO(), client.ObjectKey{Namespace: v.Namespace, Name: "kubevirt-hyperconverged-group"}, &group)
	if err != nil {
		return false
	}
	return true
}

// Checks if there is a virtualization subscription
func (v *VirtOperator) checkSubscription() bool {
	subscription := operatorsv1alpha1.Subscription{}
	err := v.Client.Get(context.TODO(), client.ObjectKey{Namespace: v.Namespace, Name: "hco-operatorhub"}, &subscription)
	if err != nil {
		return false
	}
	return true
}

// Checks if the ClusterServiceVersion status has changed to ready
func (v *VirtOperator) checkCsv() bool {
	subscription, err := v.GetOperatorSubscription()
	if err != nil {
		if err.Error() == "no subscription found" {
			return false
		}
	}

	return subscription.CsvIsReady(v.Client)
}

// CheckHco looks for a HyperConvergedOperator and returns whether or not its
// health status field is "healthy". Uses dynamic client to avoid uprooting lots
// of package dependencies, which should probably be fixed later.
func (v *VirtOperator) checkHco() bool {
	resourceId := schema.GroupVersionResource{
		Group:    "hco.kubevirt.io",
		Resource: "hyperconvergeds",
		Version:  "v1beta1",
	}

	unstructuredHco, err := v.Dynamic.Resource(resourceId).Namespace(v.Namespace).Get(context.Background(), "kubevirt-hyperconverged", v1.GetOptions{})
	if err != nil {
		log.Printf("Error getting HCO: %v", err)
		return false
	}

	health, ok, err := unstructured.NestedString(unstructuredHco.UnstructuredContent(), "status", "systemHealthStatus")
	if err != nil {
		log.Printf("Error getting HCO health: %v", err)
		return false
	}
	if !ok {
		log.Printf("HCO health field not populated yet")
		return false
	}
	log.Printf("HCO health status is: %s", health)

	return health == "healthy"
}

// Creates the target virtualization namespace, likely openshift-cnv or kubevirt-hyperconverged
func (v *VirtOperator) installNamespace() error {
	err := v.Client.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: v.Namespace}})
	if err != nil {
		log.Printf("Failed to create namespace %s: %v", v.Namespace, err)
		return err
	}
	return nil
}

// Creates the virtualization operator group
func (v *VirtOperator) installOperatorGroup() error {
	group := &operatorsv1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubevirt-hyperconverged-group",
			Namespace: v.Namespace,
		},
		Spec: operatorsv1.OperatorGroupSpec{
			TargetNamespaces: []string{
				v.Namespace,
			},
		},
	}
	err := v.Client.Create(context.Background(), group)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			log.Printf("Failed to create operator group: %v", err)
			return err
		}
	}
	return nil
}

// Creates the subscription, which triggers creation of the ClusterServiceVersion.
func (v *VirtOperator) installSubscription() error {
	subscription := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hco-operatorhub",
			Namespace: v.Namespace,
		},
		Spec: &operatorsv1alpha1.SubscriptionSpec{
			CatalogSource:          "redhat-operators",
			CatalogSourceNamespace: "openshift-marketplace",
			Package:                "kubevirt-hyperconverged",
			Channel:                "stable",
			StartingCSV:            v.Csv,
			InstallPlanApproval:    operatorsv1alpha1.ApprovalAutomatic,
		},
	}
	err := v.Client.Create(context.Background(), subscription)
	if err != nil {
		log.Printf("Failed to create subscription: %v", err)
		return err
	}

	return nil
}

// Creates a HyperConverged Operator instance. Another dynamic client to avoid
// bringing in the KubeVirt APIs for now.
func (v *VirtOperator) installHco() error {
	resourceId := schema.GroupVersionResource{
		Group:    "hco.kubevirt.io",
		Resource: "hyperconvergeds",
		Version:  "v1beta1",
	}

	unstructuredHco := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hco.kubevirt.io/v1beta1",
			"kind":       "HyperConverged",
			"metadata": map[string]interface{}{
				"name":      "kubevirt-hyperconverged",
				"namespace": v.Namespace,
			},
			"spec": map[string]interface{}{},
		},
	}

	_, err := v.Dynamic.Resource(resourceId).Namespace(v.Namespace).Create(context.Background(), &unstructuredHco, v1.CreateOptions{})
	if err != nil {
		log.Printf("Error creating HCO: %v", err)
		return err
	}

	return nil
}

// Helper function to poll status with a timeout
func waitForCheck(timeout, period time.Duration, checkFunction func() bool, errorMessage string) error {
	timer := time.After(timeout)
	for {
		if check := checkFunction(); check {
			break
		}
		select {
		case <-timer:
			return errors.New(errorMessage)
		default:
			time.Sleep(period)
		}
	}
	return nil
}

// Creates target namespace if needed, and waits for it to exist
func (v *VirtOperator) ensureNamespace(timeout time.Duration) error {
	if !v.checkNamespace() {
		if err := v.installNamespace(); err != nil {
			return err
		}
		if err := waitForCheck(timeout, 1*time.Second, v.checkNamespace, "timed out waiting to create namespace "+v.Namespace); err != nil {
			return err
		}
	} else {
		log.Printf("Namespace %s already present, no action required", v.Namespace)
	}

	return nil
}

// Creates operator group if needed, and waits for it to exist
func (v *VirtOperator) ensureOperatorGroup(timeout time.Duration) error {
	if !v.checkOperatorGroup() {
		if err := v.installOperatorGroup(); err != nil {
			return err
		}
		if err := waitForCheck(timeout, 1*time.Second, v.checkOperatorGroup, "timed out waiting to create operator group kubevirt-hyperconverged-group"); err != nil {
			return err
		}
	} else {
		log.Printf("Operator group already present, no action required")
	}

	return nil
}

// Creates the virtualization subscription if needed, and waits for it to exist
func (v *VirtOperator) ensureSubscription(timeout time.Duration) error {
	if !v.checkSubscription() {
		if err := v.installSubscription(); err != nil {
			return err
		}
		if err := waitForCheck(timeout, 1*time.Second, v.checkSubscription, "timed out waiting to create subscription"); err != nil {
			return err
		}
	} else {
		log.Printf("Subscription already created, no action required")
	}

	return nil
}

// Waits for the ClusterServiceVersion to go to ready, triggered by subscription
func (v *VirtOperator) ensureCsv(timeout time.Duration) error {
	return waitForCheck(timeout, 5*time.Second, v.checkCsv, "timed out waiting for CSV to become ready")
}

// Creates HCO instance if needed, and waits for it to go healthy
func (v *VirtOperator) ensureHco(timeout time.Duration) error {
	if !v.checkHco() {
		if err := v.installHco(); err != nil {
			return err
		}
		if err := waitForCheck(timeout, 5*time.Second, v.checkHco, "timed out waiting to create HCO"); err != nil {
			return err
		}
	} else {
		log.Printf("HCO already created, no action required")
	}

	return nil
}

// IsVirtInstalled returns whether or not the OpenShift Virtualization operator
// is installed and ready, by checking for a HyperConverged operator resource.
func (v *VirtOperator) IsVirtInstalled() bool {
	if !v.checkNamespace() {
		return false
	}

	return v.checkHco()
}

// EnsureVirtInstallation makes sure the OpenShift Virtualization operator is
// installed. This will install the operator if it is not already present.
func (v *VirtOperator) EnsureVirtInstallation(timeout time.Duration) error {
	if v.IsVirtInstalled() {
		log.Printf("Virtualization operator already installed, no action needed")
		return nil
	}

	log.Printf("Creating virtualization namespace %s", v.Namespace)
	if err := v.ensureNamespace(10 * time.Second); err != nil {
		return err
	}
	log.Printf("Created namespace %s", v.Namespace)

	log.Printf("Creating operator group kubevirt-hyperconverged-group")
	if err := v.ensureOperatorGroup(10 * time.Second); err != nil {
		return err
	}
	log.Println("Created operator group")

	log.Printf("Creating virtualization operator subscription")
	if err := v.ensureSubscription(10 * time.Second); err != nil {
		return err
	}
	log.Println("Created subscription")

	log.Printf("Waiting for ClusterServiceVersion")
	if err := v.ensureCsv(5 * time.Minute); err != nil {
		return err
	}
	log.Println("CSV ready")

	log.Printf("Creating hyperconverged operator")
	if err := v.ensureHco(5 * time.Minute); err != nil {
		return err
	}
	log.Printf("Created HCO")

	return nil
}

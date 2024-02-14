package lib

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	emulationAnnotation = "kubevirt.kubevirt.io/jsonpatch"
	useEmulation        = `[{"op": "add", "path": "/spec/configuration/developerConfiguration", "value": {"useEmulation": true}}]`
)

var packageManifestsGvr = schema.GroupVersionResource{
	Group:    "packages.operators.coreos.com",
	Resource: "packagemanifests",
	Version:  "v1",
}

var hyperConvergedGvr = schema.GroupVersionResource{
	Group:    "hco.kubevirt.io",
	Resource: "hyperconvergeds",
	Version:  "v1beta1",
}

var virtualMachineGvr = schema.GroupVersionResource{
	Group:    "kubevirt.io",
	Resource: "virtualmachines",
	Version:  "v1",
}

var csvGvr = schema.GroupVersionResource{
	Group:    "operators.coreos.com",
	Resource: "clusterserviceversion",
	Version:  "v1alpha1",
}

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

	csv, version, err := getCsvFromPackageManifest(dynamicClient, "kubevirt-hyperconverged")
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

// Helper to create an operator group object, common to installOperatorGroup
// and removeOperatorGroup.
func (v *VirtOperator) makeOperatorGroup() *operatorsv1.OperatorGroup {
	return &operatorsv1.OperatorGroup{
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
}

// getCsvFromPackageManifest returns the current CSV from the first channel
// in the given PackageManifest name. Uses the dynamic client because adding
// the real PackageManifest API from OLM was actually more work than this.
// Takes the name of the package manifest, and returns the currentCSV string,
// like: kubevirt-hyperconverged-operator.v4.12.8
// Also returns just the version (e.g. 4.12.8 from above) as a comparable
// Version type, so it is easy to check against the current cluster version.
func getCsvFromPackageManifest(dynamicClient dynamic.Interface, name string) (string, *version.Version, error) {
	log.Println("Getting packagemanifest...")
	unstructuredManifest, err := dynamicClient.Resource(packageManifestsGvr).Namespace("default").Get(context.Background(), name, v1.GetOptions{})
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
	subscription, err := v.getOperatorSubscription()
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
	unstructuredHco, err := v.Dynamic.Resource(hyperConvergedGvr).Namespace(v.Namespace).Get(context.Background(), "kubevirt-hyperconverged", v1.GetOptions{})
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

// Check if KVM emulation is enabled.
func (v *VirtOperator) checkEmulation() bool {
	hco, err := v.Dynamic.Resource(hyperConvergedGvr).Namespace("openshift-cnv").Get(context.Background(), "kubevirt-hyperconverged", v1.GetOptions{})
	if err != nil {
		return false
	}
	if hco == nil {
		return false
	}

	// Look for JSON patcher annotation that enables emulation.
	patcher, ok, err := unstructured.NestedString(hco.UnstructuredContent(), "metadata", "annotations", emulationAnnotation)
	if err != nil {
		log.Printf("Failed to get KVM emulation annotation from HCO: %v", err)
		return false
	}
	if !ok {
		log.Printf("No KVM emulation annotation (%s) listed on HCO!", emulationAnnotation)
	}
	if strings.Compare(patcher, useEmulation) == 0 {
		return true
	}

	return false
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
	group := v.makeOperatorGroup()
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
	_, err := v.Dynamic.Resource(hyperConvergedGvr).Namespace(v.Namespace).Create(context.Background(), &unstructuredHco, v1.CreateOptions{})
	if err != nil {
		log.Printf("Error creating HCO: %v", err)
		return err
	}

	return nil
}

func (v *VirtOperator) configureEmulation() error {
	hco, err := v.Dynamic.Resource(hyperConvergedGvr).Namespace("openshift-cnv").Get(context.Background(), "kubevirt-hyperconverged", v1.GetOptions{})
	if err != nil {
		return err
	}
	if hco == nil {
		return fmt.Errorf("could not find hyperconverged operator to set emulation annotation")
	}

	annotations, ok, err := unstructured.NestedMap(hco.UnstructuredContent(), "metadata", "annotations")
	if err != nil {
		return err
	}
	if !ok {
		annotations = make(map[string]interface{})
	}
	annotations[emulationAnnotation] = useEmulation

	if err := unstructured.SetNestedMap(hco.UnstructuredContent(), annotations, "metadata", "annotations"); err != nil {
		return err
	}

	_, err = v.Dynamic.Resource(hyperConvergedGvr).Namespace("openshift-cnv").Update(context.Background(), hco, v1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

// Creates target namespace if needed, and waits for it to exist
func (v *VirtOperator) ensureNamespace(timeout time.Duration) error {
	if !v.checkNamespace() {
		if err := v.installNamespace(); err != nil {
			return err
		}
		err := wait.PollImmediate(time.Second, timeout, func() (bool, error) {
			return v.checkNamespace(), nil
		})
		if err != nil {
			return fmt.Errorf("timed out waiting to create namespace %s: %w", v.Namespace, err)
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
		err := wait.PollImmediate(time.Second, timeout, func() (bool, error) {
			return v.checkOperatorGroup(), nil
		})
		if err != nil {
			return fmt.Errorf("timed out waiting to create operator group kubevirt-hyperconverged-group: %w", err)
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
		err := wait.PollImmediate(time.Second, timeout, func() (bool, error) {
			return v.checkSubscription(), nil
		})
		if err != nil {
			return fmt.Errorf("timed out waiting to create subscription: %w", err)
		}
	} else {
		log.Printf("Subscription already created, no action required")
	}

	return nil
}

// Waits for the ClusterServiceVersion to go to ready, triggered by subscription
func (v *VirtOperator) ensureCsv(timeout time.Duration) error {
	err := wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		return v.checkCsv(), nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for CSV to become ready: %w", err)
	}
	return nil
}

// Creates HyperConverged Operator instance if needed, and waits for it to go healthy
func (v *VirtOperator) ensureHco(timeout time.Duration) error {
	if !v.checkHco() {
		if err := v.installHco(); err != nil {
			return err
		}
		err := wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
			return v.checkHco(), nil
		})
		if err != nil {
			return fmt.Errorf("timed out waiting to create HCO: %w", err)
		}
	} else {
		log.Printf("HCO already created, no action required")
	}

	return nil
}

// Deletes the virtualization operator namespace (likely openshift-cnv).
func (v *VirtOperator) removeNamespace() error {
	err := v.Client.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: v.Namespace}})
	if err != nil {
		log.Printf("Failed to delete namespace %s: %v", v.Namespace, err)
		return err
	}
	return nil
}

// Deletes the virtualization operator group
func (v *VirtOperator) removeOperatorGroup() error {
	group := v.makeOperatorGroup()
	err := v.Client.Delete(context.Background(), group)
	if err != nil {
		return err
	}
	return nil
}

// Deletes the kubvirt subscription
func (v *VirtOperator) removeSubscription() error {
	subscription, err := v.getOperatorSubscription()
	if err != nil {
		return err
	}
	return subscription.Delete(v.Client)
}

// Deletes the virt ClusterServiceVersion
func (v *VirtOperator) removeCsv() error {
	return v.Dynamic.Resource(csvGvr).Namespace(v.Namespace).Delete(context.Background(), v.Csv, v1.DeleteOptions{})
}

// Deletes a HyperConverged Operator instance.
func (v *VirtOperator) removeHco() error {
	err := v.Dynamic.Resource(hyperConvergedGvr).Namespace(v.Namespace).Delete(context.Background(), "kubevirt-hyperconverged", v1.DeleteOptions{})
	if err != nil {
		log.Printf("Error deleting HCO: %v", err)
		return err
	}

	return nil
}

// Makes sure the virtualization operator's namespace is removed.
func (v *VirtOperator) ensureNamespaceRemoved(timeout time.Duration) error {
	if !v.checkNamespace() {
		log.Printf("Namespace %s already removed, no action required", v.Namespace)
		return nil
	}

	if err := v.removeNamespace(); err != nil {
		return err
	}

	err := wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		return !v.checkNamespace(), nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting to delete namespace %s: %w", v.Namespace, err)
	}

	return nil
}

// Makes sure the operator group is removed.
func (v *VirtOperator) ensureOperatorGroupRemoved(timeout time.Duration) error {
	if !v.checkOperatorGroup() {
		log.Printf("Operator group already removed, no action required")
		return nil
	}

	if err := v.removeOperatorGroup(); err != nil {
		return err
	}

	err := wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		return !v.checkOperatorGroup(), nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for operator group to be removed: %w", err)
	}

	return nil
}

// Deletes the subscription
func (v *VirtOperator) ensureSubscriptionRemoved(timeout time.Duration) error {
	if !v.checkSubscription() {
		log.Printf("Subscription already removed, no action required")
		return nil
	}

	if err := v.removeSubscription(); err != nil {
		return err
	}

	err := wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		return !v.checkSubscription(), nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for subscription to be deleted: %w", err)
	}
	return nil
}

// Deletes the ClusterServiceVersion and waits for it to be removed
func (v *VirtOperator) ensureCsvRemoved(timeout time.Duration) error {
	if !v.checkCsv() {
		log.Printf("CSV already removed, no action required")
		return nil
	}

	if err := v.removeCsv(); err != nil {
		return err
	}

	err := wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		return !v.checkCsv(), nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for CSV to be deleted: %w", err)
	}
	return nil
}

// Deletes the HyperConverged Operator instance and waits for it to be removed.
func (v *VirtOperator) ensureHcoRemoved(timeout time.Duration) error {
	if !v.checkHco() {
		log.Printf("HCO already removed, no action required")
		return nil
	}

	if err := v.removeHco(); err != nil {
		return err
	}

	err := wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		return !v.checkHco(), nil
	})

	return err
}

func (v *VirtOperator) getVmStatus(namespace, name string) (string, error) {
	vm, err := v.Dynamic.Resource(virtualMachineGvr).Namespace(namespace).Get(context.Background(), name, v1.GetOptions{})
	if err != nil {
		return "", err
	}

	status, ok, err := unstructured.NestedString(vm.UnstructuredContent(), "status", "printableStatus")
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("status field not populated yet on VM %s/%s", namespace, name)
	}
	log.Printf("VM %s/%s status is: %s", namespace, name, status)

	return status, nil
}

func (v *VirtOperator) checkVmExists(namespace, name string) bool {
	_, err := v.getVmStatus(namespace, name)
	if err == nil {
		return true
	}
	return false
}

func (v *VirtOperator) checkVmStatus(namespace, name, expectedStatus string) bool {
	status, _ := v.getVmStatus(namespace, name)
	return status == expectedStatus
}

func (v *VirtOperator) createVm(namespace, name, source string) error {
	unstructuredVm := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"running": true,
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"domain": map[string]interface{}{
							"devices": map[string]interface{}{
								"disks": []map[string]interface{}{
									{
										"disk": map[string]interface{}{
											"bus": "virtio",
										},
										"name": "rootdisk",
									},
								},
							},
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{
									"cpu":    "1",
									"memory": "256Mi",
								},
							},
						},
						"volumes": []map[string]interface{}{
							{
								"dataVolume": map[string]interface{}{
									"name": name,
								},
								"name": "rootdisk",
							},
						},
					},
				},
			},
		},
	}

	if _, err := v.Dynamic.Resource(virtualMachineGvr).Namespace(namespace).Create(context.TODO(), &unstructuredVm, v1.CreateOptions{}); err != nil {
		return fmt.Errorf("error creating VM %s/%s: %w", namespace, name, err)
	}

	return nil
}

func (v *VirtOperator) removeVm(namespace, name string) error {
	if err := v.Dynamic.Resource(virtualMachineGvr).Namespace(namespace).Delete(context.TODO(), name, v1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("error deleting VM %s/%s: %w", namespace, name, err)
		}
		log.Printf("VM %s/%s not found, delete not necessary.", namespace, name)
	}

	return nil
}

func (v *VirtOperator) ensureVm(namespace, name, source string, timeout time.Duration) error {
	if err := v.createVm(namespace, name, source); err != nil {
		return fmt.Errorf("failed to create VM %s/%s: %w", namespace, name, err)
	}

	err := wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		return v.checkVmStatus(namespace, name, "Running"), nil
	})

	return err
}

func (v *VirtOperator) ensureVmRemoval(namespace, name string, timeout time.Duration) error {
	if !v.checkVmExists(namespace, name) {
		log.Printf("VM %s/%s already removed, no action required", namespace, name)
		return nil
	}

	if err := v.removeVm(namespace, name); err != nil {
		return err
	}

	err := wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		return !v.checkVmExists(namespace, name), nil
	})

	return err
}

// Enable KVM emulation for use on cloud clusters that do not have direct
// access to the host server's virtualization capabilities.
func (v *VirtOperator) EnsureEmulation(timeout time.Duration) error {
	if v.checkEmulation() {
		log.Printf("KVM emulation already enabled, no work needed to turn it on.")
		return nil
	}

	log.Printf("Enabling KVM emulation...")

	if err := v.configureEmulation(); err != nil {
		return err
	}

	err := wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		return v.checkEmulation(), nil
	})

	return err
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
func (v *VirtOperator) EnsureVirtInstallation() error {
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

// EnsureVirtRemoval makes sure the virtualization operator is removed.
func (v *VirtOperator) EnsureVirtRemoval() error {
	log.Printf("Removing hyperconverged operator")
	if err := v.ensureHcoRemoved(3 * time.Minute); err != nil {
		return err
	}
	log.Printf("Removed HCO")

	log.Printf("Deleting virtualization operator subscription")
	if err := v.ensureSubscriptionRemoved(10 * time.Second); err != nil {
		return err
	}
	log.Println("Deleted subscription")

	log.Printf("Deleting ClusterServiceVersion")
	if err := v.ensureCsvRemoved(2 * time.Minute); err != nil {
		return err
	}
	log.Println("CSV removed")

	log.Printf("Deleting operator group kubevirt-hyperconverged-group")
	if err := v.ensureOperatorGroupRemoved(10 * time.Second); err != nil {
		return err
	}
	log.Println("Deleted operator group")

	log.Printf("Deleting virtualization namespace %s", v.Namespace)
	if err := v.ensureNamespaceRemoved(3 * time.Minute); err != nil {
		return err
	}
	log.Printf("Deleting namespace %s", v.Namespace)

	return nil
}

// Create a virtual machine from an existing PVC.
func (v *VirtOperator) CreateVm(namespace, name, source string, timeout time.Duration) error {
	log.Printf("Creating virtual machine %s/%s", namespace, name)
	return v.ensureVm(namespace, name, source, timeout)
}

// Remove a virtual machine, but leave its data volume.
func (v *VirtOperator) RemoveVm(namespace, name string, timeout time.Duration) error {
	log.Printf("Removing virtual machine %s/%s", namespace, name)
	return v.ensureVmRemoval(namespace, name, timeout)
}

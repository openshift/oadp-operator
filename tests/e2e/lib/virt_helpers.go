package lib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	velerov2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	emulationAnnotation = "kubevirt.kubevirt.io/jsonpatch"
	emulationPatchPath  = "/spec/configuration/developerConfiguration"
	stopVmPath          = "/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachines/%s/stop"
	startVmPath         = "/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachines/%s/start"

	cbtJsonPatchPath = "/spec/configuration/changedBlockTrackingLabelSelectors"
)

var emulationPatch = map[string]interface{}{
	"op":    "add",
	"path":  emulationPatchPath,
	"value": map[string]interface{}{"useEmulation": true},
}

func parseJsonPatchAnnotation(raw string) ([]interface{}, error) {
	if raw == "" {
		return nil, nil
	}
	var patches []interface{}
	if err := json.Unmarshal([]byte(raw), &patches); err != nil {
		return nil, fmt.Errorf("failed to parse jsonpatch annotation: %w", err)
	}
	return patches, nil
}

func patchArrayContainsPath(patches []interface{}, targetPath string) bool {
	for _, p := range patches {
		m, ok := p.(map[string]interface{})
		if ok && m["path"] == targetPath {
			return true
		}
	}
	return false
}

func setPatchInArray(patches []interface{}, patch map[string]interface{}) []interface{} {
	targetPath := patch["path"]
	for i, p := range patches {
		m, ok := p.(map[string]interface{})
		if ok && m["path"] == targetPath {
			patches[i] = patch
			return patches
		}
	}
	return append(patches, patch)
}

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

var virtualMachineInstanceGvr = schema.GroupVersionResource{
	Group:    "kubevirt.io",
	Resource: "virtualmachineinstances",
	Version:  "v1",
}

var virtualMachineBackupTrackerGvr = schema.GroupVersionResource{
	Group:    "backup.kubevirt.io",
	Resource: "virtualmachinebackuptrackers",
	Version:  "v1alpha1",
}

// virtualMachineBackupGvr is the per-backup VirtualMachineBackup (VMB) resource — distinct
// from VirtualMachineBackupTracker (VMBT, tracked via virtualMachineBackupTrackerGvr above),
// which persists checkpoint state across backups. VMB.Status.Type reports "full" or
// "incremental" for a single completed backup.
var virtualMachineBackupGvr = schema.GroupVersionResource{
	Group:    "backup.kubevirt.io",
	Resource: "virtualmachinebackups",
	Version:  "v1alpha1",
}

var kubevirtCrGvr = schema.GroupVersionResource{
	Group:    "kubevirt.io",
	Resource: "kubevirts",
	Version:  "v1",
}

var catalogSourceGvr = schema.GroupVersionResource{
	Group:    "operators.coreos.com",
	Resource: "catalogsources",
	Version:  "v1alpha1",
}

const (
	communityHcoCatalogName = "kubevirt-community-catalog"
	communityHcoIndexImage  = "quay.io/kubevirt/hyperconverged-cluster-index"
)

type VirtOperator struct {
	Client         client.Client
	Clientset      *kubernetes.Clientset
	Dynamic        dynamic.Interface
	Namespace      string
	Csv            string
	Version        *version.Version
	Upstream       bool
	CommunityIndex string // HCO index image tag (e.g. "1.17.1"); empty means no custom catalog
}

// communityChannelFromTag derives the OLM subscription channel name from an HCO
// index tag, e.g. "1.18.0" → "stable-v1.18", "1.17.1" → "stable-v1.17".
func communityChannelFromTag(indexTag string) string {
	parts := strings.SplitN(indexTag, ".", 3)
	if len(parts) >= 2 {
		return "stable-v" + parts[0] + "." + parts[1]
	}
	return "stable-v" + indexTag
}

// EnsureCommunityHcoCatalog creates a CatalogSource in openshift-marketplace
// pointing to the community HCO index image with the given tag. It then waits
// for the corresponding PackageManifest to become available, which indicates
// the catalog's grpc pod is serving content.
func EnsureCommunityHcoCatalog(dynamicClient dynamic.Interface, indexTag string, timeout time.Duration) error {
	catalogSource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "CatalogSource",
			"metadata": map[string]interface{}{
				"name":      communityHcoCatalogName,
				"namespace": "openshift-marketplace",
			},
			"spec": map[string]interface{}{
				"sourceType":  "grpc",
				"image":       communityHcoIndexImage + ":" + indexTag,
				"displayName": "KubeVirt Community HCO",
				"publisher":   "KubeVirt",
			},
		},
	}

	existing, err := dynamicClient.Resource(catalogSourceGvr).Namespace("openshift-marketplace").Get(context.Background(), communityHcoCatalogName, metav1.GetOptions{})
	if err == nil {
		existingImage, _, _ := unstructured.NestedString(existing.UnstructuredContent(), "spec", "image")
		expectedImage := communityHcoIndexImage + ":" + indexTag
		if existingImage != expectedImage {
			log.Printf("CatalogSource %s exists with stale image %s, updating to %s", communityHcoCatalogName, existingImage, expectedImage)
			if err := unstructured.SetNestedField(existing.UnstructuredContent(), expectedImage, "spec", "image"); err != nil {
				return fmt.Errorf("failed to set CatalogSource image: %w", err)
			}
			_, err = dynamicClient.Resource(catalogSourceGvr).Namespace("openshift-marketplace").Update(context.Background(), existing, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("failed to update CatalogSource %s: %w", communityHcoCatalogName, err)
			}
		} else {
			log.Printf("CatalogSource %s already exists with correct image %s", communityHcoCatalogName, existingImage)
		}
	} else {
		log.Printf("Creating CatalogSource %s with image %s:%s", communityHcoCatalogName, communityHcoIndexImage, indexTag)
		_, err = dynamicClient.Resource(catalogSourceGvr).Namespace("openshift-marketplace").Create(context.Background(), catalogSource, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create CatalogSource %s: %w", communityHcoCatalogName, err)
		}
	}

	// Wait for the packagemanifest to include a channel from the community catalog.
	// The community-kubevirt-hyperconverged manifest may already exist from the
	// community-operators catalog (with only "stable","1.10.7","1.11.0"), so we
	// must wait until the new catalog's channels (e.g. "stable-v1.17") appear.
	log.Printf("Waiting for community-kubevirt-hyperconverged PackageManifest to appear")
	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		manifest, getErr := dynamicClient.Resource(packageManifestsGvr).Namespace("default").Get(context.Background(), "community-kubevirt-hyperconverged", metav1.GetOptions{})
		if getErr != nil {
			log.Printf("PackageManifest not yet available: %v", getErr)
			return false, nil
		}
		channels, _, _ := unstructured.NestedSlice(manifest.UnstructuredContent(), "status", "channels")
		for _, ch := range channels {
			chMap, ok := ch.(map[string]interface{})
			if !ok {
				continue
			}
			name, _, _ := unstructured.NestedString(chMap, "name")
			if strings.HasPrefix(name, "stable-v") {
				log.Printf("PackageManifest has community channel: %s", name)
				return true, nil
			}
		}
		log.Printf("PackageManifest exists but community stable-v* channel not yet populated, retrying...")
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for PackageManifest from CatalogSource %s: %w", communityHcoCatalogName, err)
	}
	log.Printf("CatalogSource %s is ready", communityHcoCatalogName)
	return nil
}

// RemoveCommunityHcoCatalog removes the custom community HCO CatalogSource.
func RemoveCommunityHcoCatalog(dynamicClient dynamic.Interface, timeout time.Duration) error {
	_, err := dynamicClient.Resource(catalogSourceGvr).Namespace("openshift-marketplace").Get(context.Background(), communityHcoCatalogName, metav1.GetOptions{})
	if err != nil {
		log.Printf("CatalogSource %s already removed, no action required", communityHcoCatalogName)
		return nil
	}

	log.Printf("Deleting CatalogSource %s", communityHcoCatalogName)
	err = dynamicClient.Resource(catalogSourceGvr).Namespace("openshift-marketplace").Delete(context.Background(), communityHcoCatalogName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete CatalogSource %s: %w", communityHcoCatalogName, err)
	}

	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		_, getErr := dynamicClient.Resource(catalogSourceGvr).Namespace("openshift-marketplace").Get(context.Background(), communityHcoCatalogName, metav1.GetOptions{})
		return getErr != nil, nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting to delete CatalogSource %s: %w", communityHcoCatalogName, err)
	}
	log.Printf("CatalogSource %s removed", communityHcoCatalogName)
	return nil
}

// GetVirtOperator fills out a new VirtOperator. Set communityIndexTag to a
// non-empty string (e.g. "1.17.1") to use a custom CatalogSource for the
// community HCO operator. The CatalogSource must already exist before calling
// this function (see EnsureCommunityHcoCatalog).
func GetVirtOperator(c client.Client, clientset *kubernetes.Clientset, dynamicClient dynamic.Interface, upstream bool, communityIndexTag string) (*VirtOperator, error) {
	namespace := "openshift-cnv"
	manifest := "kubevirt-hyperconverged"
	channel := "stable"
	if communityIndexTag != "" {
		namespace = "kubevirt-hyperconverged"
		manifest = "community-kubevirt-hyperconverged"
		channel = communityChannelFromTag(communityIndexTag)
	} else if upstream {
		namespace = "kubevirt-hyperconverged"
		manifest = "community-kubevirt-hyperconverged"
	}

	v := &VirtOperator{
		Client:         c,
		Clientset:      clientset,
		Dynamic:        dynamicClient,
		Namespace:      namespace,
		Upstream:       upstream || communityIndexTag != "",
		CommunityIndex: communityIndexTag,
	}

	// If virt is already installed, read the CSV directly from the existing
	// subscription instead of hitting the PackageManifest (which can be
	// inconsistent across OLM PackageServer replicas).
	if v.IsVirtInstalled() {
		log.Printf("Virt already installed, reading CSV from existing subscription")
		sub, subErr := v.getOperatorSubscription()
		if subErr == nil && sub.Status.InstalledCSV != "" {
			log.Printf("Found installed CSV: %s", sub.Status.InstalledCSV)
			v.Csv = sub.Status.InstalledCSV
			// Parse version from CSV name, e.g. "kubevirt-hyperconverged-operator.v1.17.1" -> "1.17.1"
			if parts := strings.SplitN(v.Csv, ".v", 2); len(parts) == 2 {
				if operatorVersion, parseErr := version.ParseGeneric(parts[1]); parseErr == nil {
					v.Version = operatorVersion
				}
			}
			return v, nil
		}
		log.Printf("Could not read CSV from subscription (%v), falling back to PackageManifest", subErr)
	}

	// Virt not yet installed (or subscription unreadable): look up CSV from
	// the PackageManifest. Retry to tolerate OLM PackageServer replica skew.
	var csv string
	var operatorVersion *version.Version
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		var getErr error
		csv, operatorVersion, getErr = getCsvFromPackageManifest(dynamicClient, manifest, channel)
		if getErr != nil {
			log.Printf("PackageManifest lookup failed, retrying: %v", getErr)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		log.Printf("Failed to get CSV from package manifest after retries")
		return nil, fmt.Errorf("failed to get CSV from package manifest for channel %s: %w", channel, err)
	}

	v.Csv = csv
	v.Version = operatorVersion

	return v, nil
}

// Helper to create an operator group object, common to installOperatorGroup
// and removeOperatorGroup.
func (v *VirtOperator) makeOperatorGroup() *operatorsv1.OperatorGroup {
	// Community operator fails with "cannot configure to watch own namespace",
	// need to remove target namespaces.
	spec := operatorsv1.OperatorGroupSpec{}
	if !v.Upstream {
		spec = operatorsv1.OperatorGroupSpec{
			TargetNamespaces: []string{
				v.Namespace,
			},
		}
	}

	return &operatorsv1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubevirt-hyperconverged-group",
			Namespace: v.Namespace,
		},
		Spec: spec,
	}
}

// getCsvFromPackageManifest returns the current CSV from the specified channel
// in the given PackageManifest name. Uses the dynamic client because adding
// the real PackageManifest API from OLM was actually more work than this.
// Takes the name of the package manifest and the channel name, and returns
// the currentCSV string, like: kubevirt-hyperconverged-operator.v4.12.8
// Also returns just the version (e.g. 4.12.8 from above) as a comparable
// Version type, so it is easy to check against the current cluster version.
func getCsvFromPackageManifest(dynamicClient dynamic.Interface, name string, channel string) (string, *version.Version, error) {
	log.Println("Getting packagemanifest...")
	unstructuredManifest, err := dynamicClient.Resource(packageManifestsGvr).Namespace("default").Get(context.Background(), name, metav1.GetOptions{})
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

	var stableChannel map[string]interface{}
	for _, ch := range channels {
		currentChannel, ok := ch.(map[string]interface{})
		if !ok {
			continue
		}
		channelName, ok, err := unstructured.NestedString(currentChannel, "name")
		if err != nil || !ok {
			continue
		}
		log.Printf("Found channel: %s", channelName)
		if channelName == channel {
			stableChannel = currentChannel
		}
	}

	if len(stableChannel) == 0 {
		return "", nil, errors.New("failed to get channel " + channel + " from " + name + " packagemanifest")
	}

	csv, ok, err := unstructured.NestedString(stableChannel, "currentCSV")
	if err != nil {
		return "", nil, err
	}
	if !ok {
		return "", nil, errors.New("failed to get current CSV from " + name + " packagemanifest")
	}
	log.Printf("Current CSV is: %s", csv)

	versionString, ok, err := unstructured.NestedString(stableChannel, "currentCSVDesc", "version")
	if err != nil {
		return "", nil, err
	}
	if !ok {
		return "", nil, errors.New("failed to get current operator version from " + name + " packagemanifest")
	}
	log.Printf("Current operator version is: %s", versionString)

	operatorVersion, err := version.ParseGeneric(versionString)
	if err != nil {
		return "", nil, err
	}

	return csv, operatorVersion, nil
}

// Checks the existence of the operator's target namespace
func (v *VirtOperator) checkNamespace(ns string) bool {
	// First check that the namespace exists
	exists, _ := DoesNamespaceExist(v.Clientset, ns)
	return exists
}

// Checks for the existence of the virtualization operator group
func (v *VirtOperator) checkOperatorGroup() bool {
	group := operatorsv1.OperatorGroup{}
	err := v.Client.Get(context.Background(), client.ObjectKey{Namespace: v.Namespace, Name: "kubevirt-hyperconverged-group"}, &group)
	return err == nil
}

// Checks if there is a virtualization subscription
func (v *VirtOperator) checkSubscription() bool {
	subscription := operatorsv1alpha1.Subscription{}
	err := v.Client.Get(context.Background(), client.ObjectKey{Namespace: v.Namespace, Name: "hco-operatorhub"}, &subscription)
	return err == nil
}

// Checks if the ClusterServiceVersion status has changed to ready
func (v *VirtOperator) checkCsv() bool {
	subscription, err := v.getOperatorSubscription()
	if err != nil {
		if err.Error() == "no subscription found" {
			return false
		}
	}

	isReady, err := subscription.CsvIsReady(v.Client)()
	if err != nil {
		return false
	}
	return isReady
}

// CheckHco looks for a HyperConvergedOperator and returns whether or not its
// health status field is "healthy". Uses dynamic client to avoid uprooting lots
// of package dependencies, which should probably be fixed later.
func (v *VirtOperator) checkHco() bool {
	unstructuredHco, err := v.Dynamic.Resource(hyperConvergedGvr).Namespace(v.Namespace).Get(context.Background(), "kubevirt-hyperconverged", metav1.GetOptions{})
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

// Check if KVM emulation is enabled by looking for the emulation patch inside
// the jsonpatch annotation array. This handles annotations that contain
// additional patches (e.g. CBT label selectors).
func (v *VirtOperator) checkEmulation() bool {
	hco, err := v.Dynamic.Resource(hyperConvergedGvr).Namespace(v.Namespace).Get(context.Background(), "kubevirt-hyperconverged", metav1.GetOptions{})
	if err != nil {
		return false
	}
	if hco == nil {
		return false
	}

	raw, ok, err := unstructured.NestedString(hco.UnstructuredContent(), "metadata", "annotations", emulationAnnotation)
	if err != nil {
		log.Printf("Failed to get KVM emulation annotation from HCO: %v", err)
		return false
	}
	if !ok || raw == "" {
		log.Printf("No KVM emulation annotation (%s) listed on HCO!", emulationAnnotation)
		return false
	}

	patches, err := parseJsonPatchAnnotation(raw)
	if err != nil {
		log.Printf("Failed to parse KVM emulation annotation: %v", err)
		return false
	}

	return patchArrayContainsPath(patches, emulationPatchPath)
}

// Creates the target namespace, likely openshift-cnv or kubevirt-hyperconverged,
// but also used for openshift-virtualization-os-images if not already present.
func (v *VirtOperator) installNamespace(ns string) error {
	err := v.Client.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
	if err != nil {
		log.Printf("Failed to create namespace %s: %v", ns, err)
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
	spec := &operatorsv1alpha1.SubscriptionSpec{
		CatalogSource:          "redhat-operators",
		CatalogSourceNamespace: "openshift-marketplace",
		Package:                "kubevirt-hyperconverged",
		Channel:                "stable",
		StartingCSV:            v.Csv,
		InstallPlanApproval:    operatorsv1alpha1.ApprovalAutomatic,
	}
	if v.CommunityIndex != "" {
		spec = &operatorsv1alpha1.SubscriptionSpec{
			CatalogSource:          communityHcoCatalogName,
			CatalogSourceNamespace: "openshift-marketplace",
			Package:                "community-kubevirt-hyperconverged",
			Channel:                communityChannelFromTag(v.CommunityIndex),
			StartingCSV:            v.Csv,
			InstallPlanApproval:    operatorsv1alpha1.ApprovalAutomatic,
		}
	} else if v.Upstream {
		spec = &operatorsv1alpha1.SubscriptionSpec{
			CatalogSource:          "community-operators",
			CatalogSourceNamespace: "openshift-marketplace",
			Package:                "community-kubevirt-hyperconverged",
			Channel:                "stable",
			StartingCSV:            v.Csv,
			InstallPlanApproval:    operatorsv1alpha1.ApprovalAutomatic,
		}
	}
	subscription := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hco-operatorhub",
			Namespace: v.Namespace,
		},
		Spec: spec,
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
	_, err := v.Dynamic.Resource(hyperConvergedGvr).Namespace(v.Namespace).Create(context.Background(), &unstructuredHco, metav1.CreateOptions{})
	if err != nil {
		log.Printf("Error creating HCO: %v", err)
		return err
	}

	return nil
}

func (v *VirtOperator) configureEmulation() error {
	hco, err := v.Dynamic.Resource(hyperConvergedGvr).Namespace(v.Namespace).Get(context.Background(), "kubevirt-hyperconverged", metav1.GetOptions{})
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

	var patches []interface{}
	if existing, isSet := annotations[emulationAnnotation]; isSet {
		existingStr, isStr := existing.(string)
		if isStr && existingStr != "" {
			patches, err = parseJsonPatchAnnotation(existingStr)
			if err != nil {
				return err
			}
		}
	}

	patches = setPatchInArray(patches, emulationPatch)

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return fmt.Errorf("failed to marshal jsonpatch: %w", err)
	}
	annotations[emulationAnnotation] = string(patchBytes)

	if err := unstructured.SetNestedMap(hco.UnstructuredContent(), annotations, "metadata", "annotations"); err != nil {
		return err
	}

	_, err = v.Dynamic.Resource(hyperConvergedGvr).Namespace(v.Namespace).Update(context.Background(), hco, metav1.UpdateOptions{})
	return err
}

// Creates target namespace if needed, and waits for it to exist
func (v *VirtOperator) EnsureNamespace(ns string, timeout time.Duration) error {
	if !v.checkNamespace(ns) {
		if err := v.installNamespace(ns); err != nil {
			return err
		}
		err := wait.PollUntilContextTimeout(context.Background(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			return v.checkNamespace(ns), nil
		})
		if err != nil {
			return fmt.Errorf("timed out waiting to create namespace %s: %w", ns, err)
		}
	} else {
		log.Printf("Namespace %s already present, no action required", ns)
	}

	return nil
}

// Creates operator group if needed, and waits for it to exist
func (v *VirtOperator) ensureOperatorGroup(timeout time.Duration) error {
	if !v.checkOperatorGroup() {
		if err := v.installOperatorGroup(); err != nil {
			return err
		}
		err := wait.PollUntilContextTimeout(context.Background(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
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
		err := wait.PollUntilContextTimeout(context.Background(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
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
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
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
		err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
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
func (v *VirtOperator) removeNamespace(ns string) error {
	err := v.Client.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
	if err != nil {
		log.Printf("Failed to delete namespace %s: %v", ns, err)
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
	return v.Dynamic.Resource(csvGvr).Namespace(v.Namespace).Delete(context.Background(), v.Csv, metav1.DeleteOptions{})
}

// Deletes a HyperConverged Operator instance.
func (v *VirtOperator) removeHco() error {
	err := v.Dynamic.Resource(hyperConvergedGvr).Namespace(v.Namespace).Delete(context.Background(), "kubevirt-hyperconverged", metav1.DeleteOptions{})
	if err != nil {
		log.Printf("Error deleting HCO: %v", err)
		return err
	}

	return nil
}

// Makes sure the virtualization operator's namespace is removed.
func (v *VirtOperator) ensureNamespaceRemoved(ns string, timeout time.Duration) error {
	if !v.checkNamespace(ns) {
		log.Printf("Namespace %s already removed, no action required", ns)
		return nil
	}

	if err := v.removeNamespace(ns); err != nil {
		return err
	}

	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		return !v.checkNamespace(ns), nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting to delete namespace %s: %w", ns, err)
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

	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
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

	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
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

	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
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

	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		return !v.checkHco(), nil
	})

	return err
}

func GetVmStatus(dynamicClient dynamic.Interface, namespace, name string) (string, error) {
	vm, err := dynamicClient.Resource(virtualMachineGvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	status, ok, err := unstructured.NestedString(vm.UnstructuredContent(), "status", "printableStatus")
	if err != nil {
		return "", err
	}
	if ok {
		log.Printf("VM %s/%s status is: %s", namespace, name, status)
	}

	return status, nil
}

func (v *VirtOperator) GetVmStatus(namespace, name string) (string, error) {
	return GetVmStatus(v.Dynamic, namespace, name)
}

func (v *VirtOperator) WaitForVMReady(namespace, name string, timeout time.Duration) error {
	log.Printf("Waiting for VMI %s/%s Ready condition", namespace, name)
	return wait.PollUntilContextTimeout(context.Background(), 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		vmi, err := v.Dynamic.Resource(virtualMachineInstanceGvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		conditions, found, err := unstructured.NestedSlice(vmi.UnstructuredContent(), "status", "conditions")
		if err != nil || !found {
			return false, nil
		}
		for _, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if cond["type"] == "Ready" && cond["status"] == "True" {
				log.Printf("VMI %s/%s is Ready", namespace, name)
				return true, nil
			}
		}
		return false, nil
	})
}

// HasQemuGuestAgent reports whether vmName's VMI currently has a connected
// qemu-guest-agent, via the VMI's own "AgentConnected" status condition --
// the same signal kubevirt-datamover's own filesystem-freeze attempt depends
// on. Useful for a test to decide up front whether a real, guest-quiesced
// filesystem freeze can be trusted for a given VM fixture (guest-agent-backed
// images like Fedora) versus one that can't (CirrOS, which ships no agent at
// all, so freeze always fails and the guest stays writable through backup --
// confirmed directly: kubevirt-datamover-controller logs a "Guest agent is not
// responding" warning on every CirrOS backup, and reading the same live
// CirrOS VM's disk twice a few seconds apart, with no backup involved at all,
// produces two different checksums purely from the guest's own filesystem
// churn). Returns false (not an error) if the VMI doesn't exist or has no
// conditions yet -- both mean "no agent connected right now," not "unknown".
func (v *VirtOperator) HasQemuGuestAgent(namespace, name string) (bool, error) {
	vmi, err := v.Dynamic.Resource(virtualMachineInstanceGvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get VMI %s/%s: %w", namespace, name, err)
	}
	conditions, found, err := unstructured.NestedSlice(vmi.UnstructuredContent(), "status", "conditions")
	if err != nil {
		return false, fmt.Errorf("failed to read status.conditions on VMI %s/%s: %w", namespace, name, err)
	}
	if !found {
		return false, nil
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "AgentConnected" && cond["status"] == "True" {
			return true, nil
		}
	}
	return false, nil
}

// StopVm stops a VM with a REST call to "stop". This is needed because a
// poweroff from inside the VM results in KubeVirt restarting it.
// From the KubeVirt API reference:
//
//	/apis/subresources.kubevirt.io/v1/namespaces/{namespace:[a-z0-9]}/virtualmachines/{name:[a-z0-9][a-z0-9\-]}/stop
func (v *VirtOperator) StopVm(namespace, name string) error {
	path := fmt.Sprintf(stopVmPath, namespace, name)
	return v.Clientset.RESTClient().Put().AbsPath(path).Do(context.Background()).Error()
}

func (v *VirtOperator) checkVmExists(namespace, name string) bool {
	_, err := v.GetVmStatus(namespace, name)
	return err == nil
}

func (v *VirtOperator) removeVm(namespace, name string) error {
	if err := v.Dynamic.Resource(virtualMachineGvr).Namespace(namespace).Delete(context.Background(), name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("error deleting VM %s/%s: %w", namespace, name, err)
		}
		log.Printf("VM %s/%s not found, delete not necessary.", namespace, name)
	}

	return nil
}

func (v *VirtOperator) ensureVmRemoval(namespace, name string, timeout time.Duration) error {
	if !v.checkVmExists(namespace, name) {
		log.Printf("VM %s/%s already removed, no action required", namespace, name)
		return nil
	}

	if err := v.removeVm(namespace, name); err != nil {
		return err
	}

	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
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

	// Retry if there are API server conflicts ("the object has been modified")
	timeTaken := 0 * time.Second
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		timeTaken += 5
		innerErr := v.configureEmulation()
		if innerErr != nil {
			if apierrors.IsConflict(innerErr) {
				log.Printf("HCO modification conflict, trying again...")
				return false, nil // Conflict: try again
			}
			return false, innerErr // Anything else: give up
		}
		return innerErr == nil, nil
	})
	if err != nil {
		return err
	}

	timeout = timeout - timeTaken
	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		return v.checkEmulation(), nil
	})

	return err
}

// IsVirtInstalled returns whether or not the OpenShift Virtualization operator
// is installed and ready, by checking for a HyperConverged operator resource.
func (v *VirtOperator) IsVirtInstalled() bool {
	if !v.checkNamespace(v.Namespace) {
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
	if err := v.EnsureNamespace(v.Namespace, 10*time.Second); err != nil {
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
	if err := v.ensureNamespaceRemoved(v.Namespace, 3*time.Minute); err != nil {
		return err
	}
	log.Printf("Deleted namespace %s", v.Namespace)

	if v.CommunityIndex != "" {
		log.Printf("Removing community HCO CatalogSource")
		if err := RemoveCommunityHcoCatalog(v.Dynamic, 1*time.Minute); err != nil {
			return err
		}
		log.Printf("Removed CatalogSource")
	}

	return nil
}

// Remove a virtual machine, but leave its data volume.
func (v *VirtOperator) RemoveVm(namespace, name string, timeout time.Duration) error {
	log.Printf("Removing virtual machine %s/%s", namespace, name)
	return v.ensureVmRemoval(namespace, name, timeout)
}

// StartVm starts a VM with a REST call to "start".
func (v *VirtOperator) StartVm(namespace, name string) error {
	path := fmt.Sprintf(startVmPath, namespace, name)
	return v.Clientset.RESTClient().Put().AbsPath(path).Do(context.Background()).Error()
}

// RestartVmAndWaitRunning stops a VM, waits for it to stop, starts it, and
// waits for it to be running again.
func (v *VirtOperator) RestartVmAndWaitRunning(namespace, name string, timeout time.Duration) error {
	log.Printf("Restarting VM %s/%s", namespace, name)

	if err := v.StopVm(namespace, name); err != nil {
		return fmt.Errorf("failed to stop VM %s/%s: %w", namespace, name, err)
	}

	halfTimeout := timeout / 2
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, halfTimeout, true, func(ctx context.Context) (bool, error) {
		status, err := v.GetVmStatus(namespace, name)
		if err != nil {
			return false, nil
		}
		return status == "Stopped", nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for VM %s/%s to stop: %w", namespace, name, err)
	}
	log.Printf("VM %s/%s stopped, starting again", namespace, name)

	if err := v.StartVm(namespace, name); err != nil {
		return fmt.Errorf("failed to start VM %s/%s: %w", namespace, name, err)
	}

	err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, halfTimeout, true, func(ctx context.Context) (bool, error) {
		status, err := v.GetVmStatus(namespace, name)
		if err != nil {
			return false, nil
		}
		return status == "Running", nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for VM %s/%s to start: %w", namespace, name, err)
	}
	log.Printf("VM %s/%s restarted successfully", namespace, name)

	return nil
}

// RequireVEP25Support is a pre-flight check that fails immediately if the
// installed HCO version is older than 1.18 or if the backup.kubevirt.io CRDs
// (VirtualMachineBackup, VirtualMachineBackupTracker) do not exist.
// Call this after EnsureVirtInstallation to gate the test suite early.
func (v *VirtOperator) RequireVEP25Support() error {
	if v.Version == nil {
		return fmt.Errorf("VirtOperator has no version — cannot verify VEP-25 support")
	}
	minVersion, err := version.ParseSemantic("1.18.0")
	if err != nil {
		return fmt.Errorf("failed to parse minimum version: %w", err)
	}
	if !v.Version.AtLeast(minVersion) {
		return fmt.Errorf("HCO version %s is too old for VEP-25 (IncrementalBackup); need >= 1.18.0 — upgrade the community HCO or set HCO_INDEX_TAG=1.18.0", v.Version)
	}
	log.Printf("HCO version %s satisfies VEP-25 minimum (>= 1.18.0)", v.Version)

	crdGvr := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	for _, crd := range []string{"virtualmachinebackups.backup.kubevirt.io", "virtualmachinebackuptrackers.backup.kubevirt.io"} {
		_, err := v.Dynamic.Resource(crdGvr).Get(context.Background(), crd, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("required CRD %s not found — VEP-25 is not available on this cluster: %w", crd, err)
		}
		log.Printf("VEP-25 CRD present: %s", crd)
	}
	return nil
}

// EnableCBTFeatureGate patches the HyperConverged CR to set
// spec.featureGates.incrementalBackup = true, then waits for the KubeVirt CR
// to reflect "IncrementalBackup" in its featureGates and for the
// backup.kubevirt.io CRDs to appear (requires KubeVirt >= 1.8 / HCO >= 1.18).
func (v *VirtOperator) EnableCBTFeatureGate(timeout time.Duration) error {
	log.Printf("Enabling incrementalBackup feature gate on HCO")

	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		hco, err := v.Dynamic.Resource(hyperConvergedGvr).Namespace(v.Namespace).Get(ctx, "kubevirt-hyperconverged", metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to get HCO: %w", err)
		}

		current, _, _ := unstructured.NestedBool(hco.UnstructuredContent(), "spec", "featureGates", "incrementalBackup")
		log.Printf("HCO spec.featureGates.incrementalBackup current value: %v — setting to true", current)

		if err := unstructured.SetNestedField(hco.UnstructuredContent(), true, "spec", "featureGates", "incrementalBackup"); err != nil {
			return false, fmt.Errorf("failed to set incrementalBackup feature gate: %w", err)
		}

		_, err = v.Dynamic.Resource(hyperConvergedGvr).Namespace(v.Namespace).Update(ctx, hco, metav1.UpdateOptions{})
		if err != nil {
			if apierrors.IsConflict(err) {
				log.Printf("HCO modification conflict setting incrementalBackup, retrying...")
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to enable CBT feature gate: %w", err)
	}
	log.Printf("incrementalBackup feature gate set on HCO, waiting for propagation to KubeVirt CR")

	// Wait for "IncrementalBackup" to appear in the KubeVirt CR featureGates.
	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		kvList, err := v.Dynamic.Resource(kubevirtCrGvr).Namespace(v.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil || len(kvList.Items) == 0 {
			log.Printf("KubeVirt CR not yet available: %v", err)
			return false, nil
		}
		kv := &kvList.Items[0]
		gates, _, _ := unstructured.NestedStringSlice(kv.UnstructuredContent(), "spec", "configuration", "developerConfiguration", "featureGates")
		for _, g := range gates {
			if g == "IncrementalBackup" {
				log.Printf("IncrementalBackup present in KubeVirt CR featureGates")
				return true, nil
			}
		}
		log.Printf("IncrementalBackup not yet in KubeVirt CR featureGates %v, retrying...", gates)
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for IncrementalBackup to propagate to KubeVirt CR: %w", err)
	}

	// Verify the backup.kubevirt.io CRDs exist (VirtualMachineBackup, VirtualMachineBackupTracker).
	for _, crd := range []string{"virtualmachinebackups.backup.kubevirt.io", "virtualmachinebackuptrackers.backup.kubevirt.io"} {
		crdGvr := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
		_, err := v.Dynamic.Resource(crdGvr).Get(context.Background(), crd, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("backup CRD %s not found after enabling IncrementalBackup: %w", crd, err)
		}
		log.Printf("CRD %s exists", crd)
	}

	log.Printf("incrementalBackup feature gate fully enabled and verified")
	return nil
}

// EnableCBTLabelSelector patches the HCO's kubevirt.kubevirt.io/jsonpatch
// annotation to inject changedBlockTrackingLabelSelectors into the KubeVirt CR.
// If the annotation already contains patches (e.g. emulation), the CBT patch
// is merged into the existing array.
func (v *VirtOperator) EnableCBTLabelSelector(timeout time.Duration) error {
	log.Printf("Enabling CBT label selector via HCO jsonpatch annotation")

	cbtPatch := map[string]interface{}{
		"op":   "add",
		"path": cbtJsonPatchPath,
		"value": map[string]interface{}{
			"virtualMachineLabelSelector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"changedBlockTracking": "true",
				},
			},
		},
	}

	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		hco, err := v.Dynamic.Resource(hyperConvergedGvr).Namespace(v.Namespace).Get(ctx, "kubevirt-hyperconverged", metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to get HCO: %w", err)
		}

		annotations, _, _ := unstructured.NestedMap(hco.UnstructuredContent(), "metadata", "annotations")
		if annotations == nil {
			annotations = make(map[string]interface{})
		}

		var patches []interface{}
		if existing, ok := annotations[emulationAnnotation]; ok {
			existingStr, isStr := existing.(string)
			if isStr && existingStr != "" {
				patches, err = parseJsonPatchAnnotation(existingStr)
				if err != nil {
					return false, err
				}
				if patchArrayContainsPath(patches, cbtJsonPatchPath) {
					log.Printf("CBT label selector patch already present in annotation")
					return true, nil
				}
			}
		}

		patches = setPatchInArray(patches, cbtPatch)
		patchBytes, err := json.Marshal(patches)
		if err != nil {
			return false, fmt.Errorf("failed to marshal jsonpatch: %w", err)
		}
		annotations[emulationAnnotation] = string(patchBytes)

		if err := unstructured.SetNestedMap(hco.UnstructuredContent(), annotations, "metadata", "annotations"); err != nil {
			return false, err
		}

		_, err = v.Dynamic.Resource(hyperConvergedGvr).Namespace(v.Namespace).Update(ctx, hco, metav1.UpdateOptions{})
		if err != nil {
			if apierrors.IsConflict(err) {
				log.Printf("HCO modification conflict setting CBT label selector, retrying...")
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to enable CBT label selector: %w", err)
	}

	log.Printf("CBT label selector enabled via HCO jsonpatch annotation")
	return nil
}

// WaitForCBTEnabled polls the VM's status.changedBlockTracking.state until it
// equals "Enabled" or the timeout is reached.
func (v *VirtOperator) WaitForCBTEnabled(namespace, name string, timeout time.Duration) error {
	log.Printf("Waiting for CBT to be enabled on VM %s/%s", namespace, name)

	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		vm, err := v.Dynamic.Resource(virtualMachineGvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			log.Printf("Error getting VM %s/%s: %v", namespace, name, err)
			return false, nil
		}

		state, ok, err := unstructured.NestedString(vm.UnstructuredContent(), "status", "changedBlockTracking", "state")
		if err != nil || !ok {
			log.Printf("CBT state not yet available on VM %s/%s", namespace, name)
			return false, nil
		}

		log.Printf("VM %s/%s CBT state: %s", namespace, name, state)
		return state == "Enabled", nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for CBT to be enabled on VM %s/%s: %w", namespace, name, err)
	}

	log.Printf("CBT is enabled on VM %s/%s", namespace, name)
	return nil
}

// CheckVMBackupTrackerExists checks if any VirtualMachineBackupTracker resources
// exist in the given namespace. Returns true if at least one VMBT is found.
func (v *VirtOperator) CheckVMBackupTrackerExists(namespace string) (bool, error) {
	list, err := v.Dynamic.Resource(virtualMachineBackupTrackerGvr).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list VirtualMachineBackupTrackers in %s: %w", namespace, err)
	}
	return len(list.Items) > 0, nil
}

// GetVMBBackupType finds the VirtualMachineBackup in namespace whose
// annotationDataUploadName annotation matches dataUploadName, and returns its
// status.type ("full"/"incremental") and status.checkpointName.
func (v *VirtOperator) GetVMBBackupType(namespace, dataUploadName string) (backupType, checkpointName string, err error) {
	list, err := v.Dynamic.Resource(virtualMachineBackupGvr).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", "", fmt.Errorf("failed to list VirtualMachineBackups in %s: %w", namespace, err)
	}
	for _, vmb := range list.Items {
		if vmb.GetAnnotations()[annotationDataUploadName] != dataUploadName {
			continue
		}
		found := false
		backupType, found, err = unstructured.NestedString(vmb.Object, "status", "type")
		if err != nil {
			return "", "", fmt.Errorf("failed to read status.type from VirtualMachineBackup %s/%s: %w", namespace, vmb.GetName(), err)
		}
		if !found {
			return "", "", fmt.Errorf("VirtualMachineBackup %s/%s has no status.type yet", namespace, vmb.GetName())
		}
		foundCheckpoint := false
		checkpointName, foundCheckpoint, err = unstructured.NestedString(vmb.Object, "status", "checkpointName")
		if err != nil {
			return "", "", fmt.Errorf("failed to read status.checkpointName from VirtualMachineBackup %s/%s: %w", namespace, vmb.GetName(), err)
		}
		if !foundCheckpoint {
			return "", "", fmt.Errorf("VirtualMachineBackup %s/%s has no status.checkpointName yet", namespace, vmb.GetName())
		}
		return backupType, checkpointName, nil
	}
	return "", "", fmt.Errorf("no VirtualMachineBackup found in %s with %s=%s", namespace, annotationDataUploadName, dataUploadName)
}

// vmbBackupProtectionFinalizer is the finalizer virt-controller stamps on a
// VirtualMachineBackup while it is protecting an in-progress backup.
const vmbBackupProtectionFinalizer = "backup.kubevirt.io/vmbackup-protection"

// ClearStuckVMBFinalizers is a workaround for https://github.com/kubevirt/kubevirt/issues/18724:
// once a VirtualMachineBackup's backing VirtualMachineBackupTracker no longer exists,
// virt-controller's VMBackupController.sync() returns early before removeBackupFinalizer() can
// run, so a VMB already being deleted never has its backup.kubevirt.io/vmbackup-protection
// finalizer released -- blocking namespace deletion forever. Best-effort and safe to call
// repeatedly (e.g. on every poll of a namespace-deletion wait); remove once that issue is fixed.
func (v *VirtOperator) ClearStuckVMBFinalizers(namespace string) {
	list, err := v.Dynamic.Resource(virtualMachineBackupGvr).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return
	}
	for i := range list.Items {
		item := &list.Items[i]
		if item.GetDeletionTimestamp() == nil || len(item.GetFinalizers()) == 0 {
			continue
		}
		remaining := make([]string, 0, len(item.GetFinalizers()))
		for _, f := range item.GetFinalizers() {
			if f != vmbBackupProtectionFinalizer {
				remaining = append(remaining, f)
			}
		}
		if len(remaining) == len(item.GetFinalizers()) {
			continue
		}
		patchObj := map[string]any{"metadata": map[string]any{"finalizers": remaining}}
		patch, err := json.Marshal(patchObj)
		if err != nil {
			log.Printf("workaround for kubevirt#18724: failed to marshal finalizer patch for VirtualMachineBackup %s/%s: %v", namespace, item.GetName(), err)
			continue
		}
		_, err = v.Dynamic.Resource(virtualMachineBackupGvr).Namespace(namespace).Patch(
			context.Background(), item.GetName(), types.MergePatchType, patch, metav1.PatchOptions{},
		)
		if err != nil && !apierrors.IsNotFound(err) {
			log.Printf("workaround for kubevirt#18724: failed to clear stuck finalizer on VirtualMachineBackup %s/%s: %v", namespace, item.GetName(), err)
			continue
		}
		log.Printf("workaround for kubevirt#18724: cleared stuck finalizer on VirtualMachineBackup %s/%s", namespace, item.GetName())
	}
}

// IsNamespaceDeletedClearingStuckVMBFinalizers wraps IsNamespaceDeleted, additionally calling
// ClearStuckVMBFinalizers on every poll -- a workaround for
// https://github.com/kubevirt/kubevirt/issues/18724 which otherwise leaves virt test namespaces
// stuck Terminating forever. Revert call sites to plain IsNamespaceDeleted once that issue is
// fixed.
func (v *VirtOperator) IsNamespaceDeletedClearingStuckVMBFinalizers(clientset *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		v.ClearStuckVMBFinalizers(namespace)
		return IsNamespaceDeleted(clientset, namespace)()
	}
}

// GetVirtLauncherPod finds the virt-launcher pod for vmName in namespace, by listing pods
// labeled kubevirt.io=virt-launcher and matching the kubevirt.io/domain annotation (which
// KubeVirt sets to the VMI name) — mirrors the pattern used by kubevirt-velero-plugin's
// GetLauncherPod (pkg/util/util.go).
func (v *VirtOperator) GetVirtLauncherPod(namespace, vmName string) (*corev1.Pod, error) {
	pods, err := GetAllPodsWithLabel(v.Clientset, namespace, "kubevirt.io=virt-launcher")
	if err != nil {
		return nil, fmt.Errorf("failed to list virt-launcher pods in %s: %w", namespace, err)
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if pod.Annotations["kubevirt.io/domain"] == vmName {
			return pod, nil
		}
	}
	return nil, fmt.Errorf("no virt-launcher pod found for VM %s/%s", namespace, vmName)
}

// RunVirshCommand execs `virsh <args...>` inside vmName's virt-launcher pod's compute
// container via ExecuteCommandInPodsSh. kubeConfig is the suite's *rest.Config (VirtOperator
// itself only carries a *kubernetes.Clientset, not a rest.Config, so it's passed in here).
func (v *VirtOperator) RunVirshCommand(kubeConfig *rest.Config, namespace, vmName string, args ...string) (string, error) {
	pod, err := v.GetVirtLauncherPod(namespace, vmName)
	if err != nil {
		return "", err
	}
	stdout, stderr, err := ExecuteCommandInPodsSh(ProxyPodParameters{
		KubeClient:    v.Clientset,
		KubeConfig:    kubeConfig,
		Namespace:     namespace,
		PodName:       pod.Name,
		ContainerName: "compute",
	}, "virsh "+strings.Join(args, " "))
	if err != nil {
		return "", fmt.Errorf("virsh command failed (stderr: %s): %w", stderr, err)
	}
	return stdout, nil
}

// ChecksumBlockDevice returns the sha256 checksum of the entire raw block device
// backing volumeName inside vmName's virt-launcher pod's compute container --
// reading the device directly rather than a file inside the guest filesystem.
// A Block-mode disk can carry stale bytes outside any file's own blocks (e.g.
// leftover data from whatever previously occupied the same underlying storage,
// which kubevirt-datamover-controller's flattenToRaw -S 0 flag specifically
// guards against not re-exposing on restore); a guest-level file check would
// never see a regression there, only a full-device read does. KubeVirt exposes
// a Block-mode disk inside the compute container at /dev/<volume-name>, where
// volume-name matches the VMI's spec.template.spec.volumes[].name (confirmed by
// direct inspection: `ls -la /dev/` in the compute container of a running VM
// using cirros-test-cbt.yaml's "volume0" volume shows exactly /dev/volume0,
// readable and correctly sized to the PVC's underlying PV).
//
// Used specifically for a VM that is still running and therefore still holds
// its own PVC attached (RWO) to its virt-launcher pod -- ChecksumPVCBlockDevice's
// throwaway helper pod would just hang waiting to attach a PVC that's already
// attached elsewhere, since RWO permits only one attachment at a time.
//
// Currently unused (no call sites) -- intentionally kept, not dead code left by
// accident. This is the natural counterpart to VirtOperator.HasQemuGuestAgent /
// VmBackupRestoreCase.HasGuestAgent: a guest-agent-equipped VM fixture (e.g.
// Fedora) is exactly the case where kubevirt-datamover's filesystem-freeze can
// actually succeed, making a live read of the running VM's disk trustworthy
// again -- the source-side data-integrity check for such a fixture should call
// this directly instead of reaching for the rebound-PVC path that CirrOS (no
// guest agent) needs. Remove this if that fixture never materializes; don't let
// it linger unexplained.
func (v *VirtOperator) ChecksumBlockDevice(kubeConfig *rest.Config, namespace, vmName, volumeName string) (string, error) {
	pod, err := v.GetVirtLauncherPod(namespace, vmName)
	if err != nil {
		return "", err
	}
	stdout, stderr, err := ExecuteShellCommandInPod(ProxyPodParameters{
		KubeClient:    v.Clientset,
		KubeConfig:    kubeConfig,
		Namespace:     namespace,
		PodName:       pod.Name,
		ContainerName: "compute",
	}, fmt.Sprintf("sha256sum /dev/%s", volumeName))
	if err != nil {
		return "", fmt.Errorf("checksum of /dev/%s in %s/%s failed (stderr: %s): %w", volumeName, namespace, pod.Name, stderr, err)
	}
	fields := strings.Fields(stdout)
	if len(fields) == 0 {
		return "", fmt.Errorf("sha256sum produced no output for /dev/%s in %s/%s", volumeName, namespace, pod.Name)
	}
	return fields[0], nil
}

// ChecksumUploadPodQcow2Flattened finds kubevirt-datamover's own "upload" pod
// for a given DataUpload while it's still Running, and returns the sha256
// checksum of its qcow2 payload flattened to a raw image via qemu-img (already
// installed in that pod's image) -- not a direct hash of the qcow2 file itself.
// qcow2 is a container format whose bytes don't correspond 1:1 to the raw disk
// contents it encodes (confirmed via direct research into
// kubevirt-datamover-controller's pod_builder.go and uploader/run.go: the
// upload pod mounts /backup-data read-only as a plain filesystem containing
// *.qcow2 file(s), not a raw block device, unlike the download side). Flattening
// to raw first is what makes this checksum directly comparable to a raw-device
// read on the restored side.
//
// This exists because reading the rebound PVC directly (kubevirt-dm-pvc-<name>,
// via a second pod mounting it independently) doesn't work: the upload pod
// itself holds that PVC attached (RWO) for its entire upload-to-S3 duration and
// only releases it moments before the controller deletes it -- confirmed
// directly, a competing helper pod just sits stuck ContainerCreating for the
// whole upload, then the PVC vanishes out from under it. Reading via the
// upload pod itself sidesteps the RWO contention entirely, since nothing else
// needs to attach anything -- it already has the data mounted.
//
// Safe window: kubevirt-datamover-controller creates the upload pod in
// handlePrepared, before the DataUpload flips to InProgress, and deletes both
// the pod and the rebound PVC/PV in the same reconcile that marks the
// DataUpload Completed -- there is no gap to catch it after success. Call this
// from runKubevirtDMBackup's onDataUploadFound callback, which fires well
// before that point.
//
// NOT CURRENTLY CALLED -- kept deliberately, not left behind by accident. For
// a VM/disk this small (CirrOS, ~150Mi), the upload pod's own natural lifetime
// turned out too short to reliably win: three consecutive live attempts were
// all SIGKILLed (exit 137) within a few seconds of the exec starting,
// regardless of approach (a temp raw file, then a streaming pipe to sha256sum
// to rule out the container's 256Mi memory limit as the cause) -- and a
// separate, independently-polling script checking every 2s lost the race to
// even run a trivial `touch` before the container was already gone. That's a
// wall-clock race external tooling can't reliably win for a backup this fast,
// not a bug fixable by tuning the script further (see the retry-loop and
// pipefail handling in the script below -- both guard against real failure
// modes, but neither addresses the wall-clock race described above).
//
// TODO(data-integrity): this becomes worth revisiting for any future fixture
// whose backup naturally takes longer to upload (a larger disk, a busier
// guest) -- the everything-here (UID-label lookup, qcow2-vs-raw flattening,
// the /backup-data population wait) should still be correct, just currently
// unable to reliably fit inside CirrOS's own tiny window. Re-verify the timing
// empirically before relying on it again, the same way this comment's claims
// were established: live, on a real cluster, not by re-reasoning from the
// source.
func (v *VirtOperator) ChecksumUploadPodQcow2Flattened(ocClient client.Client, kubeConfig *rest.Config, namespace, dataUploadName string) (string, error) {
	// The pod's own name is generated via GenerateName with a truncating prefix
	// (kubevirt-dm-<DataUpload.Name>-, cut to fit Kubernetes' 63-char label/DNS
	// limit before the random suffix is appended) -- confirmed directly this
	// truncation is real, not just theoretical: for DataUpload name
	// "du-cirros-cbt-restore-backup-cirros-test-cirros-test-b846d047" the actual
	// pod name observed live was "kubevirt-dm-du-cirros-cbt-restore-backup-cirros-
	// test-cirro<5 random chars>", cut off mid-word. A name-prefix guess can
	// never reliably reconstruct that truncation, so this looks the pod up by
	// the "velero.io/dataupload-uid" label instead -- the DataUpload's own UID,
	// always 36 chars, never truncated, and unique per DataUpload by
	// construction (kubevirt-datamover-controller's own equivalent lookup,
	// findPodByUID, errors if it ever finds more than one).
	du := velerov2alpha1.DataUpload{}
	if err := ocClient.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: dataUploadName}, &du); err != nil {
		return "", fmt.Errorf("failed to get DataUpload %s/%s: %w", namespace, dataUploadName, err)
	}

	var podName string
	err := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		pods, listErr := v.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "velero.io/dataupload-uid=" + string(du.UID),
		})
		if listErr != nil {
			return false, nil
		}
		for i := range pods.Items {
			p := &pods.Items[i]
			if p.Status.Phase == corev1.PodRunning {
				podName = p.Name
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return "", fmt.Errorf("kubevirt-datamover upload pod for DataUpload %s/%s (uid %s) never became Running: %w", namespace, dataUploadName, du.UID, err)
	}

	// The glob is deliberately counted rather than passed straight to qemu-img:
	// `qemu-img convert` accepts multiple input filenames and CONCATENATES them
	// into one output image, so a /backup-data holding more than one qcow2 (a
	// multi-disk VM) would silently produce a checksum of all disks joined
	// together and compare it against the restored side's single raw device --
	// a mismatch that reads exactly like data corruption. Fails loudly instead.
	// The flattened file is removed via an EXIT trap rather than a trailing rm, so
	// it goes away on the failure paths too: it is the disk's full virtual size and
	// lives in the upload pod's ephemeral storage while that pod is still uploading,
	// and a convert that dies partway through (out of space being the obvious way)
	// would otherwise leave the partial copy behind and risk an eviction that breaks
	// the very backup under test.
	script := `set -e
# pipefail is load-bearing, not boilerplate: a pipeline's exit status is its LAST
# command's, so without it a qemu-img that dies mid-convert still leaves sha256sum
# exiting 0 over the truncated stream, and set -e sees nothing wrong. That yields a
# valid-looking checksum of partial data, which the caller then reports as "restored
# data does not match" -- a false corruption verdict against the datamover, the exact
# failure this whole check exists to distinguish from a real one. Not hypothetical:
# qemu-img has already been SIGKILLed here once by the 256Mi limit. If a shell ever
# lacks pipefail this line fails outright under set -e, which is the right outcome --
# loudly unsupported beats silently unchecked.
set -o pipefail
n=0
attempt=0
# The pod reaching Running doesn't mean /backup-data is populated yet -- confirmed
# directly: an exec immediately after Running found 0 qcow2 files. Retries here
# instead of failing once, since this is a one-time startup lag, not a sign
# something is actually wrong. Capped at 15 (not 60): kubevirt-datamover-controller
# deletes this pod in the same reconcile that completes the DataUpload, so a long
# wait here risks losing the pod to that teardown mid-exec instead of getting a
# clean "found 0" -- if the file isn't visible within ~30s it's not a visibility
# lag at all, since the uploader itself expects the file present at startup.
while [ "$attempt" -lt 15 ]; do
	n=0
	for f in /backup-data/*.qcow2; do
		[ -e "$f" ] || continue
		n=$((n + 1))
		src=$f
	done
	[ "$n" -ge 1 ] && break
	attempt=$((attempt + 1))
	sleep 2
done
if [ "$n" -ne 1 ]; then
	echo "expected exactly 1 qcow2 file in /backup-data after waiting, found $n" >&2
	exit 1
fi
# Streamed straight to sha256sum via a pipe rather than written to a temp raw
# file first: this container's own memory limit is 256Mi, already shared with
# its own upload-to-S3 work running concurrently -- confirmed directly, a
# materialized-to-disk version of this got SIGKILLed (exit 137) mid-convert.
# Piping keeps the flattened bytes flowing through in small chunks instead of
# holding the whole disk's worth in page cache/buffers at once.
qemu-img convert -O raw "$src" /dev/stdout | sha256sum
`
	stdout, stderr, err := ExecuteShellCommandInPod(ProxyPodParameters{
		KubeClient:    v.Clientset,
		KubeConfig:    kubeConfig,
		Namespace:     namespace,
		PodName:       podName,
		ContainerName: "upload",
	}, script)
	if err != nil {
		return "", fmt.Errorf("failed to flatten+checksum qcow2 in upload pod %s/%s (stderr: %s): %w", namespace, podName, stderr, err)
	}
	fields := strings.Fields(stdout)
	if len(fields) == 0 {
		return "", fmt.Errorf("sha256sum produced no output in upload pod %s/%s", namespace, podName)
	}
	return fields[0], nil
}

// ChecksumPVCBlockDevice returns the sha256 checksum of the entire raw block device
// backing pvcName, read via a short-lived helper pod that mounts the PVC directly as
// a raw block device (volumeDevices) rather than via a VM's virt-launcher pod. Needed
// specifically for the restored side of a data-integrity check: while this PR's fix
// holds a restored VM Halted (no VMI, hence no virt-launcher pod at all) until every
// sibling DataDownload completes, there is no VM-owned pod to exec into at that point
// -- confirmed directly: ChecksumBlockDevice failed with "no virt-launcher pod found"
// immediately after DataDownload reached Completed, because the halt really does mean
// no launcher pod exists yet, not merely a paused one. The helper pod is deleted (and
// its deletion awaited) before returning, so the PVC is free again by the time the
// caller lets the VM resume and its virt-launcher pod tries to attach the same PVC --
// PVC access mode here is RWO, so the two can never overlap.
func (v *VirtOperator) ChecksumPVCBlockDevice(kubeConfig *rest.Config, namespace, pvcName string) (checksum string, err error) {
	podName := "checksum-helper-" + pvcName
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
			},
			Containers: []corev1.Container{
				{
					Name:    "checksum",
					Image:   "registry.access.redhat.com/ubi9/ubi:latest",
					Command: []string{"/bin/sleep", "infinity"},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					VolumeDevices: []corev1.VolumeDevice{
						{Name: "block", DevicePath: "/dev/block"},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "block",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
		},
	}

	// Surfaced rather than ignored: the caller's next step is letting the VM's own
	// virt-launcher pod attach this same RWO PVC, which a still-present helper pod
	// silently blocks. Returning a checksum while the pod is still holding the PVC
	// would turn that into a confusing VM-never-started failure much later on.
	defer func() {
		if delErr := v.Clientset.CoreV1().Pods(namespace).Delete(context.Background(), podName, metav1.DeleteOptions{GracePeriodSeconds: ptr.To(int64(0))}); delErr != nil && !apierrors.IsNotFound(delErr) {
			if err == nil {
				err = fmt.Errorf("failed to delete checksum helper pod %s/%s: %w", namespace, podName, delErr)
			}
			return
		}
		if waitErr := wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
			_, getErr := v.Clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
			return apierrors.IsNotFound(getErr), nil
		}); waitErr != nil && err == nil {
			err = fmt.Errorf("checksum helper pod %s/%s was not gone before returning, PVC %s may still be attached to it: %w", namespace, podName, pvcName, waitErr)
		}
	}()

	if _, createErr := v.Clientset.CoreV1().Pods(namespace).Create(context.Background(), pod, metav1.CreateOptions{}); createErr != nil {
		return "", fmt.Errorf("failed to create checksum helper pod for PVC %s/%s: %w", namespace, pvcName, createErr)
	}

	err = wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 120*time.Second, true, func(ctx context.Context) (bool, error) {
		p, getErr := v.Clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if getErr != nil {
			return false, getErr
		}
		return p.Status.Phase == corev1.PodRunning, nil
	})
	if err != nil {
		return "", fmt.Errorf("checksum helper pod for PVC %s/%s never became Running: %w", namespace, pvcName, err)
	}

	stdout, stderr, err := ExecuteShellCommandInPod(ProxyPodParameters{
		KubeClient:    v.Clientset,
		KubeConfig:    kubeConfig,
		Namespace:     namespace,
		PodName:       podName,
		ContainerName: "checksum",
	}, "sha256sum /dev/block")
	if err != nil {
		return "", fmt.Errorf("checksum of PVC %s/%s failed (stderr: %s): %w", namespace, pvcName, stderr, err)
	}
	fields := strings.Fields(stdout)
	if len(fields) == 0 {
		return "", fmt.Errorf("sha256sum produced no output for PVC %s/%s", namespace, pvcName)
	}
	return fields[0], nil
}

// SetVMAnnotation sets a single annotation on a VirtualMachine CR, retrying on update
// conflicts. Used e.g. to set the per-VM "kubevirt-datamover.io/max-incremental-backups"
// override, which takes precedence over the global DPA-level setting — scoped to one VM and
// takes effect immediately (no controller rollout to wait for), unlike patching the DPA.
func (v *VirtOperator) SetVMAnnotation(namespace, vmName, key, value string) error {
	return wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		vm, err := v.Dynamic.Resource(virtualMachineGvr).Namespace(namespace).Get(ctx, vmName, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to get VM %s/%s: %w", namespace, vmName, err)
		}
		annotations, _, _ := unstructured.NestedMap(vm.UnstructuredContent(), "metadata", "annotations")
		if annotations == nil {
			annotations = make(map[string]interface{})
		}
		annotations[key] = value
		if err := unstructured.SetNestedMap(vm.UnstructuredContent(), annotations, "metadata", "annotations"); err != nil {
			return false, fmt.Errorf("failed to set annotation %s on VM %s/%s: %w", key, namespace, vmName, err)
		}
		_, err = v.Dynamic.Resource(virtualMachineGvr).Namespace(namespace).Update(ctx, vm, metav1.UpdateOptions{})
		if err != nil {
			if apierrors.IsConflict(err) {
				log.Printf("VM %s/%s annotation update conflict, retrying...", namespace, vmName)
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

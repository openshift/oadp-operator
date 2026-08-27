package lib

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"strings"
	"time"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
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
	Client           client.Client
	Clientset        *kubernetes.Clientset
	Dynamic          dynamic.Interface
	Namespace        string
	Csv              string
	Version          *version.Version
	Upstream         bool
	CommunityIndex   string // HCO index image tag (e.g. "1.17.1"); empty means no custom catalog
	CommunityChannel string // OLM channel actually published by that tag's catalog (discovered, not guessed)
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
// for the corresponding PackageManifest to become available (indicating the
// catalog's grpc pod is serving content) and returns the channel that catalog
// actually publishes.
//
// The returned channel is DISCOVERED from the live PackageManifest, not
// guessed from indexTag's string shape (communityChannelFromTag's "1.18.0" ->
// "stable-v1.18" assumption only holds for numeric release tags -- a moving
// tag like "nightly" publishes something like "candidate-v1.20" instead, with
// no numeric relationship to the tag string at all). Filtered by the
// PackageManifest's own "catalog" label so a same-named manifest from a
// different CatalogSource (e.g. redhat-operators/community-operators, which
// can coexist under the same package name) is never mistaken for ours.
func EnsureCommunityHcoCatalog(dynamicClient dynamic.Interface, indexTag string, timeout time.Duration) (string, error) {
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
				return "", fmt.Errorf("failed to set CatalogSource image: %w", err)
			}
			_, err = dynamicClient.Resource(catalogSourceGvr).Namespace("openshift-marketplace").Update(context.Background(), existing, metav1.UpdateOptions{})
			if err != nil {
				return "", fmt.Errorf("failed to update CatalogSource %s: %w", communityHcoCatalogName, err)
			}
		} else {
			log.Printf("CatalogSource %s already exists with correct image %s", communityHcoCatalogName, existingImage)
		}
	} else {
		log.Printf("Creating CatalogSource %s with image %s:%s", communityHcoCatalogName, communityHcoIndexImage, indexTag)
		_, err = dynamicClient.Resource(catalogSourceGvr).Namespace("openshift-marketplace").Create(context.Background(), catalogSource, metav1.CreateOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to create CatalogSource %s: %w", communityHcoCatalogName, err)
		}
	}

	log.Printf("Waiting for community-kubevirt-hyperconverged PackageManifest from CatalogSource %s to appear", communityHcoCatalogName)
	var channel string
	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		manifests, listErr := dynamicClient.Resource(packageManifestsGvr).Namespace("default").List(context.Background(), metav1.ListOptions{
			LabelSelector: "catalog=" + communityHcoCatalogName,
		})
		if listErr != nil || len(manifests.Items) == 0 {
			log.Printf("PackageManifest for CatalogSource %s not yet available, retrying...", communityHcoCatalogName)
			return false, nil
		}
		manifest := manifests.Items[0]
		if defaultChannel, found, _ := unstructured.NestedString(manifest.UnstructuredContent(), "status", "defaultChannel"); found && defaultChannel != "" {
			channel = defaultChannel
			log.Printf("PackageManifest defaultChannel: %s", channel)
			return true, nil
		}
		channels, _, _ := unstructured.NestedSlice(manifest.UnstructuredContent(), "status", "channels")
		for _, ch := range channels {
			chMap, ok := ch.(map[string]interface{})
			if !ok {
				continue
			}
			if name, _, _ := unstructured.NestedString(chMap, "name"); name != "" {
				channel = name
				log.Printf("PackageManifest channel (no defaultChannel set): %s", channel)
				return true, nil
			}
		}
		log.Printf("PackageManifest exists but has no channels populated yet, retrying...")
		return false, nil
	})
	if err != nil {
		return "", fmt.Errorf("timed out waiting for PackageManifest from CatalogSource %s: %w", communityHcoCatalogName, err)
	}
	log.Printf("CatalogSource %s is ready, channel %s", communityHcoCatalogName, channel)
	return channel, nil
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
// this function (see EnsureCommunityHcoCatalog). communityChannel should be
// the channel EnsureCommunityHcoCatalog returned for that same tag; pass ""
// to fall back to guessing a channel from communityIndexTag's numeric shape
// (communityChannelFromTag), which only works for release-style tags.
func GetVirtOperator(c client.Client, clientset *kubernetes.Clientset, dynamicClient dynamic.Interface, upstream bool, communityIndexTag string, communityChannel string) (*VirtOperator, error) {
	namespace := "openshift-cnv"
	manifest := "kubevirt-hyperconverged"
	channel := "stable"
	if communityIndexTag != "" {
		namespace = "kubevirt-hyperconverged"
		manifest = "community-kubevirt-hyperconverged"
		if communityChannel != "" {
			channel = communityChannel
		} else {
			channel = communityChannelFromTag(communityIndexTag)
		}
	} else if upstream {
		namespace = "kubevirt-hyperconverged"
		manifest = "community-kubevirt-hyperconverged"
	}

	v := &VirtOperator{
		Client:           c,
		Clientset:        clientset,
		Dynamic:          dynamicClient,
		Namespace:        namespace,
		Upstream:         upstream || communityIndexTag != "",
		CommunityIndex:   communityIndexTag,
		CommunityChannel: channel,
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
	catalogSourceName := ""
	if communityIndexTag != "" {
		catalogSourceName = communityHcoCatalogName
	}
	var csv string
	var operatorVersion *version.Version
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		var getErr error
		csv, operatorVersion, getErr = getCsvFromPackageManifest(dynamicClient, manifest, channel, catalogSourceName)
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
func getCsvFromPackageManifest(dynamicClient dynamic.Interface, name string, channel string, catalogSourceName string) (string, *version.Version, error) {
	log.Println("Getting packagemanifest...")
	var unstructuredManifest *unstructured.Unstructured
	if catalogSourceName != "" {
		// Plain Get-by-name is ambiguous when more than one CatalogSource
		// publishes a PackageManifest under the same package name (e.g. our
		// community catalog and community-operators both publish
		// "community-kubevirt-hyperconverged") -- OLM's synthetic
		// PackageManifest aggregation can return either one on a given call,
		// observed live flapping between our catalog's channel and a
		// generic community-operators one across consecutive polls. List
		// filtered by the PackageManifest's own "catalog" label to reliably
		// target the manifest OUR CatalogSource actually produced.
		manifests, listErr := dynamicClient.Resource(packageManifestsGvr).Namespace("default").List(context.Background(), metav1.ListOptions{
			LabelSelector: "catalog=" + catalogSourceName,
		})
		if listErr != nil {
			log.Printf("Error listing packagemanifests for catalog %s: %v", catalogSourceName, listErr)
			return "", nil, listErr
		}
		if len(manifests.Items) == 0 {
			return "", nil, errors.New("no packagemanifest found for catalog " + catalogSourceName)
		}
		unstructuredManifest = &manifests.Items[0]
	} else {
		m, getErr := dynamicClient.Resource(packageManifestsGvr).Namespace("default").Get(context.Background(), name, metav1.GetOptions{})
		if getErr != nil {
			log.Printf("Error getting packagemanifest %s: %v", name, getErr)
			return "", nil, getErr
		}
		unstructuredManifest = m
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
			Channel:                v.CommunityChannel,
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

// NudgeVmiToTriggerResync patches a harmless annotation onto every
// VirtualMachineInstance in vmNamespace to force a fresh Update watch event on
// it.
//
// WORKAROUND for https://redhat.atlassian.net/browse/CNV-85377 (also reported
// as https://redhat.atlassian.net/browse/CNV-89684) -- kubevirt/kubevirt#18949
// has the real upstream fix; once that merges and rolls out to a released
// build, this function becomes an unneeded no-op and should be removed along
// with its call site. virt-controller's VirtualMachineBackup reconcile
// (pkg/storage/cbt/backup.go startBackup()) can permanently stop advancing
// after successfully attaching the backup target PVC to the VMI: that branch
// returns without writing a status condition or requeuing, so recovery
// depends entirely on the VMI's own informer watch firing again -- which
// never happens if the VMI's status doesn't independently change afterward
// (e.g. because virt-handler's own hotplug-attach status write stalls, see
// kubevirt/kubevirt#18812). Patching an annotation is a real Update event on
// that exact watched object, giving the stuck reconcile a chance to notice
// the attach already succeeded and proceed -- it does not touch anything the
// reconcile loop itself inspects, so it can't mask a genuine failure, only
// unstick a missed watch event.
func NudgeVmiToTriggerResync(dynamicClient dynamic.Interface, vmNamespace string) error {
	vmis, err := dynamicClient.Resource(virtualMachineInstanceGvr).Namespace(vmNamespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	patch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{"oadp-e2e.io/cnv-85377-nudge":%q}}}`, time.Now().UTC().Format(time.RFC3339Nano)))
	for _, vmi := range vmis.Items {
		if _, patchErr := dynamicClient.Resource(virtualMachineInstanceGvr).Namespace(vmNamespace).Patch(context.Background(), vmi.GetName(), types.MergePatchType, patch, metav1.PatchOptions{}); patchErr != nil {
			log.Printf("CNV-85377 workaround: failed to nudge VMI %s/%s: %v", vmNamespace, vmi.GetName(), patchErr)
		} else {
			log.Printf("CNV-85377 workaround: nudged VMI %s/%s to retrigger a stuck VirtualMachineBackup reconcile", vmNamespace, vmi.GetName())
		}
	}
	return nil
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

// IsEmulationEnabled reads spec.configuration.developerConfiguration.useEmulation
// off the live KubeVirt CR. When true, KubeVirt is running under software emulation
// (no /dev/kvm) -- some guest OSes (Fedora in particular) are known unreliable under
// emulation, so callers use this to skip those VM variants rather than fail flakily.
func (v *VirtOperator) IsEmulationEnabled() (bool, error) {
	kvList, err := v.Dynamic.Resource(kubevirtCrGvr).Namespace(v.Namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list KubeVirt CR: %w", err)
	}
	if len(kvList.Items) == 0 {
		return false, fmt.Errorf("no KubeVirt CR found in namespace %s", v.Namespace)
	}
	kv := &kvList.Items[0]
	useEmulation, _, _ := unstructured.NestedBool(kv.UnstructuredContent(), "spec", "configuration", "developerConfiguration", "useEmulation")
	return useEmulation, nil
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

// ChecksumBlockDeviceRegion hashes only a fixed-size region starting at
// offsetMiB of the raw block device backing volumeName inside vmName's
// virt-launcher pod's compute container -- pairs with
// WriteRandomPayloadToGuestBlockDevice (which writes the region THROUGH THE
// GUEST, so the write is CBT-tracked) on the source side and
// ChecksumPVCBlockDeviceRegion on the restored side for the payload-bracketing
// hard assertion (see the comments around the payload checks in
// virt_backup_restore_suite_test.go). iflag=direct bypasses the page cache so
// the read reflects what is actually on the device, not a cached copy.
func (v *VirtOperator) ChecksumBlockDeviceRegion(kubeConfig *rest.Config, namespace, vmName, volumeName string, offsetMiB, sizeMiB int) (string, error) {
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
	}, fmt.Sprintf("dd if=/dev/%s bs=1M skip=%d count=%d iflag=direct 2>/dev/null | sha256sum", volumeName, offsetMiB, sizeMiB))
	if err != nil {
		return "", fmt.Errorf("checksum of /dev/%s region (offset=%dMiB size=%dMiB) in %s/%s failed (stderr: %s): %w", volumeName, offsetMiB, sizeMiB, namespace, pod.Name, stderr, err)
	}
	fields := strings.Fields(stdout)
	if len(fields) == 0 {
		return "", fmt.Errorf("sha256sum produced no output for /dev/%s region in %s/%s", volumeName, namespace, pod.Name)
	}
	return fields[0], nil
}

// GuestExecResult is the outcome of a qemu-guest-agent "guest-exec" call made
// via RunGuestExecScript.
type GuestExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// lastNonEmptyLine returns the last non-empty line of s, tolerating both \n
// and \r\n line endings (execArgsInPod's TTY can inject \r). virsh's own JSON
// reply to a qemu-agent-command is always the final line of output -- this
// strips any leading noise a given libvirt/polkit setup prints first.
// Confirmed live: on at least one cluster, every virsh qemu-agent-command
// invocation prints "Authorization not available. Check if polkit service is
// running or see debug message for more information." on its own line before
// the actual {"return":...} reply.
func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// RunGuestExecScript runs script INSIDE the guest OS (not the virt-launcher
// pod) via qemu-guest-agent's "guest-exec" QMP command, dispatched through
// `virsh qemu-agent-command` in the compute container. This is what makes a
// write CBT-tracked: the write goes guest -> virtio-blk -> qemu's own block
// I/O path, which is exactly what the dirty-bitmap observes -- unlike a
// host-side dd straight to the virt-launcher pod's /dev/<volume>, which
// bypasses that path entirely.
//
// domainName is the libvirt domain name virsh addresses this VM by, which is
// NOT necessarily vmName -- KubeVirt commonly namespaces it (e.g.
// "<namespace>_<vmName>"). Confirmed live against the 260716-aws-amd64
// cluster via `virsh list --all` in the compute container: KubeVirt names the
// libvirt domain "<namespace>_<vmName>", not the bare VM name -- don't assume
// the bare name is correct for a different cluster/KubeVirt version without
// re-confirming.
//
// Deliberately does NOT go through RunVirshCommand: that helper dispatches
// via ExecuteCommandInPodsSh, which naively does strings.Split(cmd, " ") with
// no shell involved -- fine for space-free args like "checkpoint-list", but
// it would corrupt a JSON payload containing spaces. This instead builds the
// full `virsh qemu-agent-command <domain> '<json>'` string itself and runs it
// via ExecuteShellCommandInPod (a real `sh -c`, so the JSON survives as one
// argv element).
//
// script itself is base64-encoded before being embedded in the guest-exec
// JSON's "arg" list (as `echo <b64> | base64 -d | /bin/sh`) so it can contain
// arbitrary shell content (quotes, pipes, redirects) without needing any
// escaping inside the JSON string or the outer single-quoted shell wrapper --
// the base64 alphabet has no JSON- or shell-special characters, and
// json.Marshal's own output never contains an unescaped single quote, so
// wrapping it in single quotes for virsh's own argv is safe.
func (v *VirtOperator) RunGuestExecScript(kubeConfig *rest.Config, namespace, vmName, domainName, script string) (*GuestExecResult, error) {
	pod, err := v.GetVirtLauncherPod(namespace, vmName)
	if err != nil {
		return nil, err
	}
	params := ProxyPodParameters{
		KubeClient:    v.Clientset,
		KubeConfig:    kubeConfig,
		Namespace:     namespace,
		PodName:       pod.Name,
		ContainerName: "compute",
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(script))
	execJSON, err := json.Marshal(map[string]any{
		"execute": "guest-exec",
		"arguments": map[string]any{
			"path":           "/bin/sh",
			"arg":            []string{"-c", "echo " + encoded + " | base64 -d | /bin/sh"},
			"capture-output": true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal guest-exec dispatch JSON: %w", err)
	}
	dispatchCmd := fmt.Sprintf("virsh qemu-agent-command %s '%s' --timeout 30", domainName, execJSON)
	stdout, stderr, err := ExecuteShellCommandInPod(params, dispatchCmd)
	if err != nil {
		return nil, fmt.Errorf("guest-exec dispatch failed for VM %s/%s (domain %s, stderr: %s): %w", namespace, vmName, domainName, stderr, err)
	}
	var dispatchReply struct {
		Return struct {
			PID int `json:"pid"`
		} `json:"return"`
	}
	// execArgsInPod's exec uses a TTY, which merges stderr into stdout and can
	// inject \r. Confirmed live: on at least one cluster, virsh prints a
	// polkit warning line ("Authorization not available. Check if polkit
	// service is running...") BEFORE its actual JSON reply, so the reply is
	// not necessarily the only thing in stdout -- lastNonEmptyLine takes
	// virsh's own last line rather than assuming the whole output is JSON.
	// The raw output is always included in the error below so an unexpected
	// preamble (or a real virsh error) is visible, not just an opaque
	// "invalid character" JSON error.
	if err := json.Unmarshal([]byte(lastNonEmptyLine(stdout)), &dispatchReply); err != nil {
		return nil, fmt.Errorf("failed to parse guest-exec dispatch reply for VM %s/%s (domain %s, raw output: %q): %w", namespace, vmName, domainName, stdout, err)
	}

	statusJSON, err := json.Marshal(map[string]any{
		"execute": "guest-exec-status",
		"arguments": map[string]any{
			"pid": dispatchReply.Return.PID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal guest-exec-status JSON: %w", err)
	}
	statusCmd := fmt.Sprintf("virsh qemu-agent-command %s '%s' --timeout 30", domainName, statusJSON)

	var result GuestExecResult
	err = wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		out, stderr, err := ExecuteShellCommandInPod(params, statusCmd)
		if err != nil {
			return false, fmt.Errorf("guest-exec-status poll failed for VM %s/%s (domain %s, stderr: %s): %w", namespace, vmName, domainName, stderr, err)
		}
		var statusReply struct {
			Return struct {
				Exited   bool   `json:"exited"`
				ExitCode int    `json:"exitcode"`
				OutData  string `json:"out-data"`
				ErrData  string `json:"err-data"`
			} `json:"return"`
		}
		if err := json.Unmarshal([]byte(lastNonEmptyLine(out)), &statusReply); err != nil {
			return false, fmt.Errorf("failed to parse guest-exec-status reply for VM %s/%s (domain %s, raw output: %q): %w", namespace, vmName, domainName, out, err)
		}
		if !statusReply.Return.Exited {
			return false, nil
		}
		outBytes, err := base64.StdEncoding.DecodeString(statusReply.Return.OutData)
		if err != nil {
			return false, fmt.Errorf("failed to base64-decode guest-exec out-data for VM %s/%s (domain %s): %w", namespace, vmName, domainName, err)
		}
		errBytes, err := base64.StdEncoding.DecodeString(statusReply.Return.ErrData)
		if err != nil {
			return false, fmt.Errorf("failed to base64-decode guest-exec err-data for VM %s/%s (domain %s): %w", namespace, vmName, domainName, err)
		}
		result = GuestExecResult{ExitCode: statusReply.Return.ExitCode, Stdout: string(outBytes), Stderr: string(errBytes)}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("guest-exec did not complete for VM %s/%s (domain %s): %w", namespace, vmName, domainName, err)
	}
	return &result, nil
}

// WriteRandomPayloadToGuestBlockDevice writes sizeMiB of random bytes at
// offsetMiB into guestDevice (e.g. "/dev/vda") FROM INSIDE THE GUEST via
// RunGuestExecScript, rather than from the host side. Because the write goes
// through the guest's own block driver -> qemu I/O path, it IS visible to
// qemu's CBT dirty-bitmap tracking -- unlike a host-side dd straight to the
// virt-launcher pod's /dev/<volume>, this is safe to use for
// incremental-backup data-integrity assertions.
//
// conv=fsync plus a following `sync` ensure the bytes are actually flushed
// through to the virtio-blk device before this returns, not left sitting in
// the guest's page cache -- the same bracket-verify pattern this pairs with
// (checksumming immediately before/after a backup window) depends on the
// write being durable at the moment it completes, not still buffered.
//
// There is deliberately no guest-side checksum counterpart to this function:
// reading the payload back via a guest-exec `sha256sum` would only prove the
// guest's own page cache believes the bytes are there, not that they reached
// qemu's block layer, which would make a bracket-verify built on it
// worthless. Use the existing host-side ChecksumBlockDeviceRegion (source)
// and ChecksumPVCBlockDeviceRegion (restored side) for all reads instead.
func (v *VirtOperator) WriteRandomPayloadToGuestBlockDevice(kubeConfig *rest.Config, namespace, vmName, domainName, guestDevice string, offsetMiB, sizeMiB int) error {
	script := fmt.Sprintf("dd if=/dev/urandom of=%s bs=1M seek=%d count=%d conv=fsync && sync", guestDevice, offsetMiB, sizeMiB)
	result, err := v.RunGuestExecScript(kubeConfig, namespace, vmName, domainName, script)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("guest write to %s in VM %s/%s (domain %s) failed (exit %d): %s", guestDevice, namespace, vmName, domainName, result.ExitCode, result.Stderr)
	}
	return nil
}

// runInPVCBlockDeviceHelperPod creates a throwaway pod mounting pvcName as a raw
// block device at /dev/block, waits for it to reach Running, execs command inside
// it, then tears the pod down before returning. Needed specifically for the
// restored side of a data-integrity check: while this PR's fix holds a restored
// VM Halted (no VMI, hence no virt-launcher pod at all) until every sibling
// DataDownload completes, there is no VM-owned pod to exec into at that point --
// confirmed directly: ChecksumBlockDeviceRegion failed with "no virt-launcher pod
// found" immediately after DataDownload reached Completed, because the halt
// really does mean no launcher pod exists yet, not merely a paused one. The
// helper pod is deleted (and its deletion awaited) before returning, so the PVC
// is free again by the time the caller lets the VM resume and its virt-launcher
// pod tries to attach the same PVC -- PVC access mode here is RWO, so the two
// can never overlap. Shared plumbing for ChecksumPVCBlockDeviceRegion so the pod
// lifecycle (creation, Running wait, teardown-before-return) exists in exactly
// one place.
// checksumHelperPodName builds "checksum-helper-"+pvcName, truncated with a
// short hash suffix if that would exceed Kubernetes' 63-char DNS label limit
// for object names. Not a concern for this suite's own short fixture PVC
// names today, but a future longer-named PVC scenario would otherwise fail
// pod creation with an opaque "invalid name" error instead of running.
func checksumHelperPodName(pvcName string) string {
	const prefix = "checksum-helper-"
	name := prefix + pvcName
	if len(name) <= 63 {
		return name
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(pvcName))
	suffix := fmt.Sprintf("-%08x", h.Sum32())
	return name[:63-len(suffix)] + suffix
}

func (v *VirtOperator) runInPVCBlockDeviceHelperPod(kubeConfig *rest.Config, namespace, pvcName, command string) (stdout string, err error) {
	podName := checksumHelperPodName(pvcName)
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

	var stderr string
	stdout, stderr, err = ExecuteShellCommandInPod(ProxyPodParameters{
		KubeClient:    v.Clientset,
		KubeConfig:    kubeConfig,
		Namespace:     namespace,
		PodName:       podName,
		ContainerName: "checksum",
	}, command)
	if err != nil {
		return "", fmt.Errorf("command %q against PVC %s/%s failed (stderr: %s): %w", command, namespace, pvcName, stderr, err)
	}
	return stdout, nil
}

// ChecksumPVCBlockDeviceRegion hashes only a fixed-size region of the block
// device starting at offsetMiB -- pairs with
// WriteRandomPayloadToGuestBlockDevice/ChecksumBlockDeviceRegion on the source
// side for the payload-bracketing hard assertion (see the comments around the
// payload checks in virt_backup_restore_suite_test.go). iflag=direct bypasses
// the page cache so the read reflects what is actually on the PV, not a
// cached copy.
func (v *VirtOperator) ChecksumPVCBlockDeviceRegion(kubeConfig *rest.Config, namespace, pvcName string, offsetMiB, sizeMiB int) (string, error) {
	stdout, err := v.runInPVCBlockDeviceHelperPod(kubeConfig, namespace, pvcName,
		fmt.Sprintf("dd if=/dev/block bs=1M skip=%d count=%d iflag=direct 2>/dev/null | sha256sum", offsetMiB, sizeMiB))
	if err != nil {
		return "", err
	}
	fields := strings.Fields(stdout)
	if len(fields) == 0 {
		return "", fmt.Errorf("sha256sum produced no output for PVC %s/%s region (offset=%dMiB size=%dMiB)", namespace, pvcName, offsetMiB, sizeMiB)
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

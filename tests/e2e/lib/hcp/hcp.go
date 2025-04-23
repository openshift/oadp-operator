package hcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	hypershiftv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

func (h *HCHandler) RemoveHCP(timeout time.Duration) error {
	// Delete the hostedCluster
	if err := h.DeleteHostedCluster(); err != nil {
		return err
	}

	// Delete HCP Namespace
	if err := h.DeleteHCPNamespace(false); err != nil {
		return err
	}

	// Delete HCP
	if err := h.DeleteHostedControlPlane(); err != nil {
		return err
	}

	// Wait for HCP deletion with timeout
	var hcpName string
	if h.HostedCluster != nil {
		hcpName = h.HostedCluster.Name
	} else {
		// If HostedCluster is nil, try to get the HCP name from the namespace
		hcpName = "test-hc" // Default name if we can't determine it
	}

	hcp := hypershiftv1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hcpName,
			Namespace: h.HCPNamespace,
		},
	}
	if err := h.WaitForHCPDeletion(&hcp); err != nil {
		return fmt.Errorf("failed to delete HCP: %v", err)
	}
	log.Printf("\tHCP deleted")

	// Delete HC Secrets
	if err := h.DeleteHCSecrets(); err != nil {
		return err
	}

	// Wait for the HC to be deleted
	log.Printf("\tWaiting for the HC to be deleted")
	err := wait.PollUntilContextTimeout(h.Ctx, time.Second*5, timeout, true, func(ctx context.Context) (bool, error) {
		log.Printf("\tAttempting to verify HC deletion...")
		result := IsHCDeleted(h)
		log.Printf("\tHC deletion check result: %v", result)
		return result, nil
	})

	if err != nil {
		return fmt.Errorf("failed to wait for HC deletion: %v", err)
	}

	return nil
}

// DeleteHostedCluster deletes a HostedCluster and waits for its deletion
func (h *HCHandler) DeleteHostedCluster() error {
	if h.HostedCluster == nil {
		log.Printf("No HostedCluster to delete")
		return nil
	}

	log.Printf("Deleting HostedCluster %s in namespace %s", h.HostedCluster.Name, h.HostedCluster.Namespace)
	if err := h.deleteResource(h.HostedCluster); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete HostedCluster: %v", err)
	}

	// Wait for HC deletion
	if err := h.WaitForHCDeletion(); err != nil {
		return fmt.Errorf("failed waiting for HostedCluster deletion: %v", err)
	}

	return nil
}

// DeleteHCPNamespace deletes the HCP namespace and waits for its deletion if needed
func (h *HCHandler) DeleteHCPNamespace(shouldWait bool) error {
	if h.HCPNamespace == "" {
		log.Printf("No HCP namespace to delete")
		return nil
	}

	log.Printf("Deleting HCP namespace %s", h.HCPNamespace)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: h.HCPNamespace,
		},
	}

	if err := h.deleteResource(ns); err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("Namespace %s already deleted", h.HCPNamespace)
			return nil
		}
		return fmt.Errorf("failed to delete HCP namespace %s: %v", h.HCPNamespace, err)
	}

	if !shouldWait {
		return nil
	}

	log.Printf("Waiting for namespace %s to be deleted", h.HCPNamespace)
	err := wait.PollUntilContextTimeout(h.Ctx, WaitForNextCheckTimeout, Wait10Min, true, func(ctx context.Context) (bool, error) {
		err := h.Client.Get(ctx, types.NamespacedName{Name: h.HCPNamespace}, ns)
		if err == nil {
			log.Printf("Namespace %s still exists, waiting...", h.HCPNamespace)
			return false, nil
		}

		if apierrors.IsNotFound(err) {
			log.Printf("Namespace %s successfully deleted", h.HCPNamespace)
			return true, nil
		}

		// Handle retryable errors
		if apierrors.IsTooManyRequests(err) || apierrors.IsServerTimeout(err) || apierrors.IsTimeout(err) {
			log.Printf("Retryable error while checking namespace %s deletion: %v", h.HCPNamespace, err)
			return false, nil
		}

		return false, fmt.Errorf("unexpected error while checking namespace %s deletion: %v", h.HCPNamespace, err)
	})

	if err != nil {
		return fmt.Errorf("timeout waiting for namespace %s to be deleted: %v", h.HCPNamespace, err)
	}

	return nil
}

// DeleteHostedControlPlane deletes a HostedControlPlane and waits for its deletion
func (h *HCHandler) DeleteHostedControlPlane() error {
	if h.HCPNamespace == "" {
		log.Printf("No HCP namespace specified")
		return nil
	}

	// Get the HCP name from HostedCluster if available, otherwise use default
	var hcpName string
	if h.HostedCluster != nil {
		hcpName = h.HostedCluster.Name
	} else {
		hcpName = "test-hc" // Default name if HostedCluster is nil
	}

	hcp := &hypershiftv1.HostedControlPlane{}
	err := h.Client.Get(h.Ctx, types.NamespacedName{
		Namespace: h.HCPNamespace,
		Name:      hcpName,
	}, hcp)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("No HostedControlPlane found in namespace %s", h.HCPNamespace)
			return nil
		}
		return fmt.Errorf("failed to get HostedControlPlane: %v", err)
	}

	log.Printf("Deleting HostedControlPlane %s in namespace %s", hcp.Name, hcp.Namespace)
	if err := h.deleteResource(hcp); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete HostedControlPlane: %v", err)
	}

	// Wait for HCP deletion
	if err := h.WaitForHCPDeletion(hcp); err != nil {
		return fmt.Errorf("failed waiting for HostedControlPlane deletion: %v", err)
	}

	return nil
}

// DeleteHCSecrets deletes secrets in the HCP namespace
func (h *HCHandler) DeleteHCSecrets() error {
	if h.HCPNamespace == "" {
		log.Printf("No HCP namespace specified")
		return nil
	}

	log.Printf("Deleting secrets in namespace %s", h.HCPNamespace)
	secretList := &corev1.SecretList{}
	if err := h.Client.List(h.Ctx, secretList, &client.ListOptions{
		Namespace: h.HCPNamespace,
	}); err != nil {
		return fmt.Errorf("failed to list secrets: %v", err)
	}

	for _, secret := range secretList.Items {
		if err := h.deleteResource(&secret); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete secret %s: %v", secret.Name, err)
		}
	}

	return nil
}

// WaitForHCDeletion waits for the HostedCluster to be deleted
func (h *HCHandler) WaitForHCDeletion() error {
	return wait.PollUntilContextTimeout(h.Ctx, WaitForNextCheckTimeout, Wait10Min, true, func(ctx context.Context) (bool, error) {
		return IsHCDeleted(h), nil
	})
}

// WaitForHCPDeletion waits for the HostedControlPlane to be deleted
func (h *HCHandler) WaitForHCPDeletion(hcp *hypershiftv1.HostedControlPlane) error {
	return wait.PollUntilContextTimeout(h.Ctx, WaitForNextCheckTimeout, Wait10Min, true, func(ctx context.Context) (bool, error) {
		return IsHCPDeleted(h, hcp), nil
	})
}

// NukeHostedCluster removes all resources associated with a HostedCluster
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

// DeployHCManifest deploys a HostedCluster manifest
func (h *HCHandler) DeployHCManifest(tmpl, provider string, hcName string) (*hypershiftv1.HostedCluster, error) {
	log.Printf("Deploying HostedCluster manifest - %s", provider)
	// Create the clusters ns
	clustersNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ClustersNamespace,
		},
	}

	log.Printf("Creating clusters namespace")
	err := h.Client.Create(h.Ctx, clustersNS)
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create clusters namespace: %v", err)
		}
	}

	log.Printf("Getting pull secret")
	pullSecret, err := getPullSecret(h.Ctx, h.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull secret: %v", err)
	}

	log.Printf("Applying pull secret manifest")
	err = ApplyYAMLTemplate(h.Ctx, h.Client, PullSecretManifest, true, map[string]interface{}{
		"HostedClusterName": hcName,
		"ClustersNamespace": ClustersNamespace,
		"PullSecret":        base64.StdEncoding.EncodeToString([]byte(pullSecret)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to apply pull secret manifest: %v", err)
	}

	log.Printf("Applying encryption key manifest")
	err = ApplyYAMLTemplate(h.Ctx, h.Client, EtcdEncryptionKeyManifest, true, map[string]interface{}{
		"HostedClusterName": hcName,
		"ClustersNamespace": ClustersNamespace,
		"EtcdEncryptionKey": SampleETCDEncryptionKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to apply encryption key manifest: %v", err)
	}

	if provider == "Agent" {
		log.Printf("Applying capi-provider-role manifest")
		err = ApplyYAMLTemplate(h.Ctx, h.Client, CapiProviderRoleManifest, true, map[string]interface{}{
			"ClustersNamespace": ClustersNamespace,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to apply capi-provider-role manifest from %s: %v", CapiProviderRoleManifest, err)
		}
	}

	log.Printf("Applying HostedCluster manifest")
	err = ApplyYAMLTemplate(h.Ctx, h.Client, tmpl, false, map[string]interface{}{
		"HostedClusterName": hcName,
		"ClustersNamespace": ClustersNamespace,
		"HCOCPTestImage":    h.HCOCPTestImage,
		"InfraIDSeed":       "test",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to apply HostedCluster manifest: %v", err)
	}

	// Wait for HC to be present
	var hc hypershiftv1.HostedCluster
	err = wait.PollUntilContextTimeout(h.Ctx, WaitForNextCheckTimeout, Wait10Min, true, func(ctx context.Context) (bool, error) {
		err := h.Client.Get(ctx, types.NamespacedName{
			Name:      hcName,
			Namespace: ClustersNamespace,
		}, &hc)
		if err != nil {
			if !apierrors.IsNotFound(err) && !apierrors.IsTooManyRequests(err) && !apierrors.IsServerTimeout(err) && !apierrors.IsTimeout(err) {
				return false, fmt.Errorf("failed to get HostedCluster %s: %v", hcName, err)
			}
			log.Printf("Error getting HostedCluster %s, retrying...: %v", hcName, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed waiting for HostedCluster to be present: %v", err)
	}

	return &hc, nil
}

// ValidateETCD validates that the ETCD StatefulSet is ready
func ValidateETCD(ctx context.Context, ocClient client.Client, hcpNamespace string, timeout time.Duration) error {
	log.Printf("Validating ETCD StatefulSet with timeout: %v", timeout)

	// Create a separate context for ETCD validation with a longer timeout
	etcdCtx, etcdCancel := context.WithTimeout(ctx, timeout)
	defer etcdCancel()

	err := wait.PollUntilContextTimeout(etcdCtx, time.Second*10, timeout, true, func(ctx context.Context) (bool, error) {
		etcdSts := &appsv1.StatefulSet{}
		err := ocClient.Get(ctx, types.NamespacedName{Name: "etcd", Namespace: hcpNamespace}, etcdSts)
		if err != nil {
			if !apierrors.IsNotFound(err) && !apierrors.IsTooManyRequests(err) && !apierrors.IsServerTimeout(err) && !apierrors.IsTimeout(err) {
				log.Printf("ETCD StatefulSet not found yet, waiting...")
				return false, fmt.Errorf("failed to get etcd statefulset: %v", err)
			}
			log.Printf("Error getting etcd statefulset, retrying...: %v", err)
			return false, nil
		}
		if etcdSts.Status.Replicas != etcdSts.Status.ReadyReplicas {
			log.Printf("ETCD STS is not ready (Available: %d, Replicas: %d)", etcdSts.Status.ReadyReplicas, etcdSts.Status.Replicas)
			return false, nil
		}
		log.Printf("ETCD STS is ready")
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait for ETCD StatefulSet: %v", err)
	}
	return nil
}

// ValidateDeployments validates that all required deployments are ready
func ValidateDeployments(ctx context.Context, ocClient client.Client, hcpNamespace string, deployments []string, contingencyTimeout time.Duration) error {
	for _, depName := range deployments {
		log.Printf("Checking deployment: %s", depName)
		ready := false
		err := wait.PollUntilContextTimeout(ctx, time.Second*10, contingencyTimeout, true, func(ctx context.Context) (bool, error) {
			deployment := &appsv1.Deployment{}
			err := ocClient.Get(ctx, types.NamespacedName{Name: depName, Namespace: hcpNamespace}, deployment)
			if err != nil {
				if !apierrors.IsNotFound(err) && !apierrors.IsTooManyRequests(err) && !apierrors.IsServerTimeout(err) && !apierrors.IsTimeout(err) {
					return false, fmt.Errorf("failed to get deployment %s: %v", depName, err)
				}
				log.Printf("Error getting deployment %s: %v", depName, err)
				return false, nil
			}
			if deployment.Status.AvailableReplicas != deployment.Status.Replicas {
				log.Printf("Deployment %s is not ready (Available: %d, Replicas: %d)", depName, deployment.Status.AvailableReplicas, deployment.Status.Replicas)
				return false, nil
			}
			ready = true
			return true, nil
		})

		if err != nil || !ready {
			log.Printf("Deployment %s validation failed", depName)
			err := handleDeploymentValidationFailure(ctx, ocClient, hcpNamespace, deployments, contingencyTimeout)
			if err != nil {
				return fmt.Errorf("deployment %s failed after contingency applied: %v", depName, err)
			}
		}
	}
	log.Printf("All deployments validated successfully")
	return nil
}

// ValidateHCP returns a VerificationFunction that checks if the HostedCluster pods are running
func ValidateHCP(timeout time.Duration, contingencyTimeout time.Duration, deployments []string, hcpNamespace string) func(client.Client, string) error {
	log.Printf("Starting HCP validation with timeout: %v, contingency timeout: %v", timeout, contingencyTimeout)

	if len(deployments) == 0 {
		deployments = RequiredWorkingOperators
	}

	if timeout == 0 {
		timeout = ValidateHCPTimeout
	}

	if contingencyTimeout == 0 {
		contingencyTimeout = Wait10Min
	}

	return func(ocClient client.Client, _ string) error {
		log.Printf("Checking deployments in namespace: %s", hcpNamespace)

		// Create a new context for validation that won't be canceled by the parent context
		valCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		// Validate ETCD StatefulSet
		if err := ValidateETCD(valCtx, ocClient, hcpNamespace, timeout); err != nil {
			return err
		}

		// Validate deployments
		if err := ValidateDeployments(valCtx, ocClient, hcpNamespace, deployments, contingencyTimeout); err != nil {
			return err
		}

		return nil
	}
}

// handleDeploymentValidationFailure handles the case when a deployment validation fails
// The function should list all the pods in the HCP namespace and restart them if they are not running.
// This is because after the restore of an HCP, the pods got stuck and
func handleDeploymentValidationFailure(ctx context.Context, ocClient client.Client, namespace string, deployments []string, timeout time.Duration) error {
	log.Printf("Handling validation failure for deployments in namespace %s", namespace)
	// List all pods in the HCP namespace
	pods := &corev1.PodList{}
	err := ocClient.List(ctx, pods, &client.ListOptions{Namespace: namespace})
	if err != nil {
		log.Printf("Error listing pods in namespace %s: %v", namespace, err)
		return err
	}

	// Delete all non-running pods
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			log.Printf("Deleting non-running pod %s", pod.Name)
			err := ocClient.Delete(ctx, &pod)
			if err != nil {
				log.Printf("Error deleting pod %s: %v", pod.Name, err)
				return err
			}
		}
	}

	// Check if all deployments are ready with timeout
	for _, deployment := range deployments {
		err := wait.PollUntilContextTimeout(ctx, time.Second*10, timeout, true, func(ctx context.Context) (bool, error) {
			dep := &appsv1.Deployment{}
			err := ocClient.Get(ctx, types.NamespacedName{Name: deployment, Namespace: namespace}, dep)
			if err != nil {
				log.Printf("Error getting deployment %s, retrying...: %v", deployment, err)
				return false, nil
			}
			done, err := lib.IsDeploymentReady(ocClient, dep.Namespace, dep.Name)()
			if !done || err != nil {
				return false, nil
			}

			return true, nil
		})

		if err != nil {
			return fmt.Errorf("deployment %s is not ready after timeout: %v", deployment, err)
		}
	}

	return nil
}

// IsHCPDeleted checks if a HostedControlPlane has been deleted
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

// IsHCDeleted checks if a HostedCluster has been deleted
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

// GetHCPNamespace returns the namespace for a HostedControlPlane
func GetHCPNamespace(name, namespace string) string {
	return fmt.Sprintf("%s-%s", namespace, name)
}

// RestartHCPPods restarts the pods for a HostedControlPlane namespace which stays in Init state
func RestartHCPPods(HCPNamespace string, c client.Client) error {
	pl := &corev1.PodList{}
	err := c.List(context.Background(), pl, &client.ListOptions{Namespace: HCPNamespace})
	if err != nil {
		return fmt.Errorf("failed to list pods: %v", err)
	}
	for _, pod := range pl.Items {
		if pod.Status.Phase != corev1.PodRunning {
			return fmt.Errorf("pod %s is not running", pod.Name)
		}
	}
	return nil
}

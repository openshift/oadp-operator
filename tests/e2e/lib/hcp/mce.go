package hcp

import (
	"context"
	"fmt"
	"log"
	"time"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// MCE related constants
	MCEOperatorNamespace = "multicluster-engine"
	MCEOperatorGroupName = "multicluster-engine"
	MCESubscriptionName  = "multicluster-engine"
)

// DeleteMCEOperand deletes the MCE operand
func (h *HCHandler) DeleteMCEOperand() error {
	log.Printf("Deleting MCE operand %s", MCEOperandName)
	mce := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "MultiClusterEngine",
			"apiVersion": mceGVR.GroupVersion().String(),
			"metadata": map[string]interface{}{
				"name":      MCEOperandName,
				"namespace": MCENamespace,
			},
		},
	}
	return h.deleteResource(mce)
}

// DeleteMCEOperatorGroup deletes the MCE operator group
func (h *HCHandler) DeleteMCEOperatorGroup() error {
	log.Printf("Deleting MCE operator group %s", MCEOperatorGroup)
	og := &operatorsv1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MCEOperatorGroup,
			Namespace: MCENamespace,
		},
	}
	return h.deleteResource(og)
}

// DeleteMCESubscription deletes the MCE subscription
func (h *HCHandler) DeleteMCESubscription() error {
	log.Printf("Deleting MCE subscription %s", MCEOperatorName)
	sub := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MCEOperatorName,
			Namespace: MCENamespace,
		},
	}
	return h.deleteResource(sub)
}

// RemoveMCE removes the MCE operand, operator group, and subscription
func (h *HCHandler) RemoveMCE() error {
	log.Printf("Removing MCE resources")

	// Delete MCE operand
	if err := h.DeleteMCEOperand(); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete MCE operand: %v", err)
	}

	// Delete MCE operator group
	if err := h.DeleteMCEOperatorGroup(); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete MCE operator group: %v", err)
	}

	// Delete MCE subscription
	if err := h.DeleteMCESubscription(); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete MCE subscription: %v", err)
	}

	// Wait for MCE operand to be deleted
	mce := &unstructured.Unstructured{}
	mce.SetGroupVersionKind(mceGVR.GroupVersion().WithKind("MultiClusterEngine"))
	mce.SetName(MCEOperandName)
	mce.SetNamespace(MCENamespace)

	err := wait.PollUntilContextTimeout(h.Ctx, WaitForNextCheckTimeout, Wait10Min, true, func(ctx context.Context) (bool, error) {
		if err := h.Client.Get(ctx, types.NamespacedName{Name: MCEOperandName, Namespace: MCENamespace}, mce); err != nil {
			if !apierrors.IsNotFound(err) && !apierrors.IsTooManyRequests(err) && !apierrors.IsServerTimeout(err) && !apierrors.IsTimeout(err) {
				return false, fmt.Errorf("failed to get MCE operand: %v", err)
			}
			log.Printf("Error getting MCE operand, retrying...: %v", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed waiting for MCE operand deletion: %v", err)
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

func (h *HCHandler) IsMCEDeployed() bool {
	log.Printf("Checking if MCE deployment is finished...")
	mcePods := &corev1.PodList{}
	err := h.Client.List(h.Ctx, mcePods, client.InNamespace(MCENamespace))
	if err != nil {
		return false
	}

	if len(mcePods.Items) == 0 {
		return false
	}

	for _, pod := range mcePods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			return false
		}
	}

	return true
}

// WaitForCatalogSourceReady waits for a CatalogSource to be ready
func WaitForCatalogSourceReady(ctx context.Context, c client.Client, catalogSourceName, namespace string, timeout time.Duration) error {
	log.Printf("Waiting for CatalogSource %s/%s to be ready...", namespace, catalogSourceName)

	catalogSource := &unstructured.Unstructured{}
	catalogSource.SetGroupVersionKind(schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "catalogsources",
	}.GroupVersion().WithKind("CatalogSource"))

	err := wait.PollUntilContextTimeout(ctx, time.Second*10, timeout, true, func(ctx context.Context) (bool, error) {
		err := c.Get(ctx, types.NamespacedName{
			Name:      catalogSourceName,
			Namespace: namespace,
		}, catalogSource)

		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Printf("CatalogSource %s/%s not found, waiting...", namespace, catalogSourceName)
				return false, nil
			}
			log.Printf("Error getting CatalogSource: %v", err)
			return false, nil
		}

		// Check connection state
		state, found, err := unstructured.NestedString(catalogSource.Object, "status", "connectionState", "lastObservedState")
		if err != nil {
			log.Printf("Error getting connection state: %v", err)
			return false, nil
		}

		if !found {
			log.Printf("Connection state not found yet, waiting...")
			return false, nil
		}

		log.Printf("CatalogSource %s/%s state: %s", namespace, catalogSourceName, state)
		return state == "READY", nil
	})

	if err != nil {
		return fmt.Errorf("timeout waiting for CatalogSource %s/%s to be ready: %v", namespace, catalogSourceName, err)
	}

	log.Printf("CatalogSource %s/%s is ready", namespace, catalogSourceName)
	return nil
}

// WaitForPackageManifest waits for a PackageManifest to be available
func WaitForPackageManifest(ctx context.Context, c client.Client, packageName, namespace string, timeout time.Duration) error {
	log.Printf("Waiting for PackageManifest %s/%s to be available...", namespace, packageName)

	pkg := &unstructured.Unstructured{}
	pkg.SetGroupVersionKind(schema.GroupVersionResource{
		Group:    "packages.operators.coreos.com",
		Version:  "v1",
		Resource: "packagemanifests",
	}.GroupVersion().WithKind("PackageManifest"))

	err := wait.PollUntilContextTimeout(ctx, time.Second*10, timeout, true, func(ctx context.Context) (bool, error) {
		err := c.Get(ctx, types.NamespacedName{
			Name:      packageName,
			Namespace: namespace,
		}, pkg)

		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Printf("PackageManifest %s/%s not found, waiting...", namespace, packageName)
				return false, nil
			}
			log.Printf("Error getting PackageManifest: %v", err)
			return false, nil
		}

		// Check if it has channels
		channels, found, err := unstructured.NestedSlice(pkg.Object, "status", "channels")
		if err != nil {
			log.Printf("Error getting channels: %v", err)
			return false, nil
		}

		if !found || len(channels) == 0 {
			log.Printf("PackageManifest %s/%s found but no channels available yet", namespace, packageName)
			return false, nil
		}

		log.Printf("PackageManifest %s/%s is available with %d channels", namespace, packageName, len(channels))
		return true, nil
	})

	if err != nil {
		return fmt.Errorf("timeout waiting for PackageManifest %s/%s: %v", namespace, packageName, err)
	}

	log.Printf("PackageManifest %s/%s is ready", namespace, packageName)
	return nil
}

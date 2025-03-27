package hcp

import (
	"context"
	"fmt"
	"log"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

package hcp

import (
	"context"
	"fmt"
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
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
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

// diagnosticLogKeywords are matched case-insensitively against MCE operator pod log
// lines. Deliberately narrow rather than dumping full logs: these are operator-authored
// diagnostic lines, not free-text user/backup content, but still worth filtering down
// to only the lines an investigator would act on.
var diagnosticLogKeywords = []string{"error", "denied", "forbidden", "minimum version", "reconcil"}

func filterLogLines(logs string, keywords []string) []string {
	var matched []string
	for _, line := range strings.Split(logs, "\n") {
		lower := strings.ToLower(line)
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				matched = append(matched, line)
				break
			}
		}
	}
	return matched
}

// DumpHypershiftDiagnostics logs best-effort diagnostics for the MultiClusterEngine
// operand and the hypershift/multicluster-engine namespaces. Intended to be called
// right before failing an Eventually that waits on MCE/HyperShift operator readiness,
// since none of this state is otherwise captured by CI artifacts. Every step is
// best-effort: a failure here must never panic or obscure the real test failure.
// clientset may be nil, in which case pod log capture is skipped (conditions/components/
// pods/events are still captured via c).
func DumpHypershiftDiagnostics(ctx context.Context, c client.Client, clientset *kubernetes.Clientset) {
	log.Printf("=== HyperShift/MCE diagnostics dump ===")

	mce := &unstructured.Unstructured{}
	mce.SetGroupVersionKind(mceGVR.GroupVersion().WithKind("MultiClusterEngine"))
	if err := c.Get(ctx, types.NamespacedName{Name: MCEOperandName, Namespace: MCENamespace}, mce); err != nil {
		log.Printf("diagnostics: failed to get MultiClusterEngine %s/%s: %v", MCENamespace, MCEOperandName, err)
	} else {
		conditions, _, _ := unstructured.NestedSlice(mce.Object, "status", "conditions")
		for _, condRaw := range conditions {
			cond, ok := condRaw.(map[string]interface{})
			if !ok {
				continue
			}
			log.Printf("diagnostics: MultiClusterEngine %s/%s condition type=%v status=%v reason=%v",
				MCENamespace, MCEOperandName, cond["type"], cond["status"], cond["reason"])
		}
		components, _, _ := unstructured.NestedSlice(mce.Object, "status", "components")
		for _, compRaw := range components {
			comp, ok := compRaw.(map[string]interface{})
			if !ok {
				continue
			}
			log.Printf("diagnostics: MultiClusterEngine %s/%s component name=%v type=%v status=%v reason=%v",
				MCENamespace, MCEOperandName, comp["name"], comp["type"], comp["status"], comp["reason"])
		}
	}

	for _, ns := range []string{HONamespace, MCENamespace} {
		pods := &corev1.PodList{}
		if err := c.List(ctx, pods, client.InNamespace(ns)); err != nil {
			log.Printf("diagnostics: failed to list pods in %s: %v", ns, err)
		} else {
			for _, pod := range pods.Items {
				log.Printf("diagnostics: pod %s/%s phase=%s", ns, pod.Name, pod.Status.Phase)

				// Capture logs from the MCE operator pod specifically -- its reconciler
				// can reject the MultiClusterEngine CR outright (e.g. an OCP-version
				// floor check) before any managed component, including the HyperShift
				// operator Deployment, is ever created. That failure mode is otherwise
				// invisible: MCE's own Deployment/CSV still report Ready/Succeeded.
				if clientset == nil || ns != MCENamespace || !strings.HasPrefix(pod.Name, MCEOperatorName) {
					continue
				}
				for _, container := range pod.Spec.Containers {
					logs, err := lib.GetPodContainerLogs(clientset, ns, pod.Name, container.Name)
					if err != nil {
						log.Printf("diagnostics: failed to get logs for %s/%s container %s: %v", ns, pod.Name, container.Name, err)
						continue
					}
					for _, line := range filterLogLines(logs, diagnosticLogKeywords) {
						log.Printf("diagnostics: pod %s/%s container %s log: %s", ns, pod.Name, container.Name, line)
					}
				}
			}
		}

		events := &corev1.EventList{}
		if err := c.List(ctx, events, client.InNamespace(ns)); err != nil {
			log.Printf("diagnostics: failed to list events in %s: %v", ns, err)
		} else {
			for _, event := range events.Items {
				log.Printf("diagnostics: event %s/%s reason=%s type=%s count=%d", ns, event.InvolvedObject.Name, event.Reason, event.Type, event.Count)
			}
		}
	}

	log.Printf("=== end HyperShift/MCE diagnostics dump ===")
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

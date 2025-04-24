package hcp

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

// createTestScheme creates a new scheme with all required types registered
func createTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	oadpv1alpha1.AddToScheme(scheme)
	operatorsv1.AddToScheme(scheme)
	operatorsv1alpha1.AddToScheme(scheme)
	return scheme
}

func TestRemoveMCE(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		Name           string
		MCE            *unstructured.Unstructured
		OperatorGroup  *operatorsv1.OperatorGroup
		Subscription   *operatorsv1alpha1.Subscription
		ExpectedResult bool
	}{
		{
			Name: "All resources exist",
			MCE: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "MultiClusterEngine",
					"apiVersion": mceGVR.GroupVersion().String(),
					"metadata": map[string]interface{}{
						"name":      MCEOperandName,
						"namespace": MCENamespace,
					},
				},
			},
			OperatorGroup: &operatorsv1.OperatorGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MCEOperatorGroup,
					Namespace: MCENamespace,
				},
			},
			Subscription: &operatorsv1alpha1.Subscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MCEOperatorName,
					Namespace: MCENamespace,
				},
			},
			ExpectedResult: true,
		},
		{
			Name: "Only MCE exists",
			MCE: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "MultiClusterEngine",
					"apiVersion": mceGVR.GroupVersion().String(),
					"metadata": map[string]interface{}{
						"name":      MCEOperandName,
						"namespace": MCENamespace,
					},
				},
			},
			OperatorGroup:  nil,
			Subscription:   nil,
			ExpectedResult: true,
		},
		{
			Name: "Only OperatorGroup exists",
			MCE:  nil,
			OperatorGroup: &operatorsv1.OperatorGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MCEOperatorGroup,
					Namespace: MCENamespace,
				},
			},
			Subscription:   nil,
			ExpectedResult: true,
		},
		{
			Name:          "Only Subscription exists",
			MCE:           nil,
			OperatorGroup: nil,
			Subscription: &operatorsv1alpha1.Subscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MCEOperatorName,
					Namespace: MCENamespace,
				},
			},
			ExpectedResult: true,
		},
		{
			Name:           "No resources exist",
			MCE:            nil,
			OperatorGroup:  nil,
			Subscription:   nil,
			ExpectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			// Create a new scheme and client for each test case
			scheme := createTestScheme()
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Create the handler
			h := &HCHandler{
				Ctx:    context.Background(),
				Client: client,
			}

			// Create resources if they exist in the test case
			if tt.MCE != nil {
				// Set the GVK for the MCE using the constant
				tt.MCE.SetGroupVersionKind(mceGVR.GroupVersion().WithKind("MultiClusterEngine"))
				err := client.Create(context.Background(), tt.MCE)
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}

			if tt.OperatorGroup != nil {
				err := client.Create(context.Background(), tt.OperatorGroup)
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}

			if tt.Subscription != nil {
				err := client.Create(context.Background(), tt.Subscription)
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}

			// Call RemoveMCE
			err := h.RemoveMCE()
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Verify resources are deleted
			mce := &unstructured.Unstructured{}
			mce.SetGroupVersionKind(mceGVR.GroupVersion().WithKind("MultiClusterEngine"))
			err = client.Get(context.Background(), types.NamespacedName{Name: MCEOperandName, Namespace: MCENamespace}, mce)
			g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())

			og := &operatorsv1.OperatorGroup{}
			err = client.Get(context.Background(), types.NamespacedName{Name: MCEOperatorGroup, Namespace: MCENamespace}, og)
			g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())

			sub := &operatorsv1alpha1.Subscription{}
			err = client.Get(context.Background(), types.NamespacedName{Name: MCEOperatorName, Namespace: MCENamespace}, sub)
			g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
		})
	}
}

package lib

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/gomega"
	hypershiftv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

// createTestScheme creates a new scheme with all required types registered
func createTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	err = hypershiftv1.AddToScheme(scheme)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	err = oadpv1alpha1.AddToScheme(scheme)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	err = operatorsv1.AddToScheme(scheme)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	err = operatorsv1alpha1.AddToScheme(scheme)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	err = appsv1.AddToScheme(scheme)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	return scheme
}

func TestWaitForHCPDeletion(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name           string
		setupResources func(*testing.T, client.Client) *hypershiftv1.HostedControlPlane
		timeout        time.Duration
		expectedError  bool
		verifyDeletion func(*testing.T, client.Client)
	}{
		{
			name: "Successfully wait for HCP deletion",
			setupResources: func(t *testing.T, client client.Client) *hypershiftv1.HostedControlPlane {
				// Create namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: HCPNamespace,
					},
				}
				g.Expect(client.Create(context.Background(), ns)).To(gomega.Succeed())

				// Create HCP
				hcp := &hypershiftv1.HostedControlPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:      HostedClusterName,
						Namespace: HCPNamespace,
					},
				}
				g.Expect(client.Create(context.Background(), hcp)).To(gomega.Succeed())

				// Delete HCP immediately
				g.Expect(client.Delete(context.Background(), hcp)).To(gomega.Succeed())

				return hcp
			},
			timeout:       time.Second * 5,
			expectedError: false,
			verifyDeletion: func(t *testing.T, client client.Client) {
				err := client.Get(context.Background(), types.NamespacedName{
					Name:      HostedClusterName,
					Namespace: HCPNamespace,
				}, &hypershiftv1.HostedControlPlane{})
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
			},
		},
		{
			name: "HCP with finalizers requires nuking",
			setupResources: func(t *testing.T, client client.Client) *hypershiftv1.HostedControlPlane {
				// Create namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: HCPNamespace,
					},
				}
				g.Expect(client.Create(context.Background(), ns)).To(gomega.Succeed())

				// Create HCP with finalizer
				hcp := &hypershiftv1.HostedControlPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:       HostedClusterName,
						Namespace:  HCPNamespace,
						Finalizers: []string{"test-finalizer"},
					},
				}
				g.Expect(client.Create(context.Background(), hcp)).To(gomega.Succeed())

				// Delete HCP immediately
				g.Expect(client.Delete(context.Background(), hcp)).To(gomega.Succeed())

				return hcp
			},
			timeout:       time.Second * 5,
			expectedError: false,
			verifyDeletion: func(t *testing.T, client client.Client) {
				err := client.Get(context.Background(), types.NamespacedName{
					Name:      HostedClusterName,
					Namespace: HCPNamespace,
				}, &hypershiftv1.HostedControlPlane{})
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
			},
		},
		{
			name: "Timeout waiting for HCP deletion",
			setupResources: func(t *testing.T, client client.Client) *hypershiftv1.HostedControlPlane {
				// Create namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: HCPNamespace,
					},
				}
				g.Expect(client.Create(context.Background(), ns)).To(gomega.Succeed())

				// Create HCP with finalizer that won't be removed
				hcp := &hypershiftv1.HostedControlPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:       HostedClusterName,
						Namespace:  HCPNamespace,
						Finalizers: []string{"test-finalizer"},
					},
				}
				g.Expect(client.Create(context.Background(), hcp)).To(gomega.Succeed())

				return hcp
			},
			timeout:       time.Second * 2,
			expectedError: true,
			verifyDeletion: func(t *testing.T, client client.Client) {
				err := client.Get(context.Background(), types.NamespacedName{
					Name:      HostedClusterName,
					Namespace: HCPNamespace,
				}, &hypershiftv1.HostedControlPlane{})
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
			},
		},
		{
			name: "HCP already deleted",
			setupResources: func(t *testing.T, client client.Client) *hypershiftv1.HostedControlPlane {
				// Create namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: HCPNamespace,
					},
				}
				g.Expect(client.Create(context.Background(), ns)).To(gomega.Succeed())

				// Return HCP that doesn't exist in the cluster
				return &hypershiftv1.HostedControlPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:      HostedClusterName,
						Namespace: HCPNamespace,
					},
				}
			},
			timeout:       time.Minute,
			expectedError: false,
			verifyDeletion: func(t *testing.T, client client.Client) {
				err := client.Get(context.Background(), types.NamespacedName{
					Name:      HostedClusterName,
					Namespace: HCPNamespace,
				}, &hypershiftv1.HostedControlPlane{})
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
			},
		},
		{
			name: "Nil HCP",
			setupResources: func(t *testing.T, client client.Client) *hypershiftv1.HostedControlPlane {
				return nil
			},
			timeout:       time.Minute,
			expectedError: false,
			verifyDeletion: func(t *testing.T, client client.Client) {
				// No verification needed for nil HCP
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme and client for each test case
			scheme := createTestScheme()
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Create HCHandler
			h := &HCHandler{
				Ctx:          context.Background(),
				Client:       client,
				HCPNamespace: HCPNamespace,
			}

			// Setup test resources and get HCP
			hcp := tt.setupResources(t, client)

			// Create context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			// Test WaitForHCPDeletion with the specified timeout
			err := WaitForHCPDeletion(ctx, h, hcp, tt.timeout)

			// Check results
			if tt.expectedError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				if tt.verifyDeletion != nil {
					tt.verifyDeletion(t, client)
				}
			}
		})
	}
}

func TestRemoveHCP(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name           string
		setupResources func(*testing.T, client.Client)
		expectedError  bool
		errorContains  string
		verifyDeletion func(*testing.T, client.Client)
	}{
		{
			name: "Successfully remove HCP with all resources",
			setupResources: func(t *testing.T, client client.Client) {
				// Create namespaces
				clustersNS := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: ClustersNamespace,
					},
				}
				hcpNS := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: HCPNamespace,
					},
				}
				g.Expect(client.Create(context.Background(), clustersNS)).To(gomega.Succeed())
				g.Expect(client.Create(context.Background(), hcpNS)).To(gomega.Succeed())

				// Create hosted cluster
				hostedCluster := &hypershiftv1.HostedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      HostedClusterName,
						Namespace: ClustersNamespace,
					},
				}
				g.Expect(client.Create(context.Background(), hostedCluster)).To(gomega.Succeed())

				// Create hosted control plane
				hcp := &hypershiftv1.HostedControlPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:      HostedClusterName,
						Namespace: HCPNamespace,
					},
				}
				g.Expect(client.Create(context.Background(), hcp)).To(gomega.Succeed())

				// Create secrets
				pullSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-pull-secret", HostedClusterName),
						Namespace: ClustersNamespace,
					},
				}
				etcdSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-etcd-encryption-key", HostedClusterName),
						Namespace: ClustersNamespace,
					},
				}
				g.Expect(client.Create(context.Background(), pullSecret)).To(gomega.Succeed())
				g.Expect(client.Create(context.Background(), etcdSecret)).To(gomega.Succeed())
			},
			expectedError: false,
			verifyDeletion: func(t *testing.T, client client.Client) {
				// Verify hosted cluster is deleted
				err := client.Get(context.Background(), types.NamespacedName{
					Name:      HostedClusterName,
					Namespace: ClustersNamespace,
				}, &hypershiftv1.HostedCluster{})
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())

				// Verify hosted control plane is deleted
				err = client.Get(context.Background(), types.NamespacedName{
					Name:      HostedClusterName,
					Namespace: HCPNamespace,
				}, &hypershiftv1.HostedControlPlane{})
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())

				// Verify secrets are deleted
				err = client.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-pull-secret", HostedClusterName),
					Namespace: ClustersNamespace,
				}, &corev1.Secret{})
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())

				err = client.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-etcd-encryption-key", HostedClusterName),
					Namespace: ClustersNamespace,
				}, &corev1.Secret{})
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
			},
		},
		{
			name: "Remove HCP with finalizers",
			setupResources: func(t *testing.T, client client.Client) {
				// Create namespaces
				clustersNS := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: ClustersNamespace,
					},
				}
				hcpNS := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: HCPNamespace,
					},
				}
				g.Expect(client.Create(context.Background(), clustersNS)).To(gomega.Succeed())
				g.Expect(client.Create(context.Background(), hcpNS)).To(gomega.Succeed())

				// Create hosted cluster with finalizers
				hostedCluster := &hypershiftv1.HostedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       HostedClusterName,
						Namespace:  ClustersNamespace,
						Finalizers: []string{"test-finalizer"},
					},
				}
				g.Expect(client.Create(context.Background(), hostedCluster)).To(gomega.Succeed())

				// Create hosted control plane with finalizers
				hcp := &hypershiftv1.HostedControlPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:       HostedClusterName,
						Namespace:  HCPNamespace,
						Finalizers: []string{"test-finalizer"},
					},
				}
				g.Expect(client.Create(context.Background(), hcp)).To(gomega.Succeed())
			},
			expectedError: false,
			verifyDeletion: func(t *testing.T, client client.Client) {
				// Verify hosted cluster is deleted
				err := client.Get(context.Background(), types.NamespacedName{
					Name:      HostedClusterName,
					Namespace: ClustersNamespace,
				}, &hypershiftv1.HostedCluster{})
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())

				// Verify hosted control plane is deleted
				err = client.Get(context.Background(), types.NamespacedName{
					Name:      HostedClusterName,
					Namespace: HCPNamespace,
				}, &hypershiftv1.HostedControlPlane{})
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
			},
		},
		{
			name: "Remove HCP with missing resources",
			setupResources: func(t *testing.T, client client.Client) {
				// Create only the hosted cluster
				hostedCluster := &hypershiftv1.HostedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      HostedClusterName,
						Namespace: ClustersNamespace,
					},
				}
				g.Expect(client.Create(context.Background(), hostedCluster)).To(gomega.Succeed())
			},
			expectedError: false,
			verifyDeletion: func(t *testing.T, client client.Client) {
				// Verify hosted cluster is deleted
				err := client.Get(context.Background(), types.NamespacedName{
					Name:      HostedClusterName,
					Namespace: ClustersNamespace,
				}, &hypershiftv1.HostedCluster{})
				g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme and client for each test case
			scheme := createTestScheme()
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Setup test resources
			tt.setupResources(t, client)

			// Create HCHandler
			h := &HCHandler{
				Ctx:          context.Background(),
				Client:       client,
				HCPNamespace: HCPNamespace,
				HostedCluster: &hypershiftv1.HostedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      HostedClusterName,
						Namespace: ClustersNamespace,
					},
				},
			}

			// Test RemoveHCP
			err := RemoveHCP(h)

			// Check results
			if tt.expectedError {
				g.Expect(err).To(gomega.HaveOccurred())
				if tt.errorContains != "" {
					g.Expect(err.Error()).To(gomega.ContainSubstring(tt.errorContains))
				}
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				if tt.verifyDeletion != nil {
					tt.verifyDeletion(t, client)
				}
			}
		})
	}
}

func TestInstallRequiredOperators(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name            string
		packageManifest *unstructured.Unstructured
		reqOperators    []RequiredOperator
		expectedError   bool
		errorContains   string
		verifyHandler   func(*testing.T, *HCHandler)
	}{
		{
			name: "Successfully install MCE operator",
			packageManifest: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"defaultChannel": "stable-2.0",
					},
				},
			},
			reqOperators: []RequiredOperator{
				{
					Name:          MCEName,
					Namespace:     MCENamespace,
					OperatorGroup: MCEOperatorGroup,
				},
			},
			expectedError: false,
			verifyHandler: func(t *testing.T, h *HCHandler) {
				g.Expect(h).ToNot(gomega.BeNil())
				g.Expect(h.HCPNamespace).To(gomega.Equal(HCPNamespace))
				g.Expect(h.HCOCPTestImage).To(gomega.Equal(HCOCPTestImage))
			},
		},
		{
			name: "Successfully install operator with specific channel",
			packageManifest: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"defaultChannel": "stable-2.0",
					},
				},
			},
			reqOperators: []RequiredOperator{
				{
					Name:          MCEName,
					Namespace:     MCENamespace,
					OperatorGroup: MCEOperatorGroup,
					Channel:       "stable-2.1",
				},
			},
			expectedError: false,
			verifyHandler: func(t *testing.T, h *HCHandler) {
				g.Expect(h).ToNot(gomega.BeNil())
			},
		},
		{
			name: "Successfully install operator with specific CSV",
			packageManifest: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"defaultChannel": "stable-2.0",
					},
				},
			},
			reqOperators: []RequiredOperator{
				{
					Name:          MCEName,
					Namespace:     MCENamespace,
					OperatorGroup: MCEOperatorGroup,
					Csv:           "multicluster-engine.v2.1.0",
				},
			},
			expectedError: false,
			verifyHandler: func(t *testing.T, h *HCHandler) {
				g.Expect(h).ToNot(gomega.BeNil())
			},
		},
		{
			name:            "Fail to install operator with missing package manifest",
			packageManifest: nil,
			reqOperators: []RequiredOperator{
				{
					Name:          MCEName,
					Namespace:     MCENamespace,
					OperatorGroup: MCEOperatorGroup,
				},
			},
			expectedError: true,
			errorContains: "failed to get PackageManifest",
		},
		{
			name: "Fail to install operator with missing default channel",
			packageManifest: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{},
				},
			},
			reqOperators: []RequiredOperator{
				{
					Name:          MCEName,
					Namespace:     MCENamespace,
					OperatorGroup: MCEOperatorGroup,
				},
			},
			expectedError: true,
			errorContains: "no default channel found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme and client for each test case
			scheme := createTestScheme()
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Create package manifest if provided
			if tt.packageManifest != nil {
				tt.packageManifest.SetGroupVersionKind(packageManifestGVR.GroupVersion().WithKind("PackageManifest"))
				tt.packageManifest.SetName(MCEName)
				tt.packageManifest.SetNamespace(MCENamespace)
				err := client.Create(context.Background(), tt.packageManifest)
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}

			// Test InstallRequiredOperators
			h, err := InstallRequiredOperators(context.Background(), client, tt.reqOperators)

			if tt.expectedError {
				g.Expect(err).To(gomega.HaveOccurred())
				if tt.errorContains != "" {
					g.Expect(err.Error()).To(gomega.ContainSubstring(tt.errorContains))
				}
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				if tt.verifyHandler != nil {
					tt.verifyHandler(t, h)
				}
			}
		})
	}
}

func TestWaitForUnstructuredObject(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name          string
		obj           *unstructured.Unstructured
		createObj     bool
		deleteObj     bool
		timeout       time.Duration
		expectedError bool
		errorContains string
	}{
		{
			name: "Object already deleted",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Test",
					"apiVersion": "test/v1",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test",
					},
				},
			},
			createObj:     false,
			deleteObj:     false,
			timeout:       time.Minute,
			expectedError: false,
		},
		{
			name: "Object deleted during wait",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Test",
					"apiVersion": "test/v1",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test",
					},
				},
			},
			createObj:     true,
			deleteObj:     true,
			timeout:       time.Minute,
			expectedError: false,
		},
		{
			name: "Object not deleted within timeout",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Test",
					"apiVersion": "test/v1",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test",
					},
				},
			},
			createObj:     true,
			deleteObj:     false,
			timeout:       time.Second * 2,
			expectedError: true,
			errorContains: "context deadline exceeded",
		},
		{
			name: "Object with finalizers",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Test",
					"apiVersion": "test/v1",
					"metadata": map[string]interface{}{
						"name":       "test",
						"namespace":  "test",
						"finalizers": []interface{}{"test-finalizer"},
					},
				},
			},
			createObj:     true,
			deleteObj:     true,
			timeout:       time.Second * 2,
			expectedError: true,
			errorContains: "context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme and client for each test case
			scheme := createTestScheme()
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Create the object if needed
			if tt.createObj {
				err := client.Create(context.Background(), tt.obj)
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}

			// Start deletion in background if needed
			if tt.deleteObj {
				go func() {
					time.Sleep(100 * time.Millisecond) // Give time for the test to start
					err := client.Delete(context.Background(), tt.obj)
					g.Expect(err).ToNot(gomega.HaveOccurred())
				}()
			}

			// Test WaitForUnstructuredObject
			err := WaitForUnstructuredObject(context.Background(), client, tt.obj, tt.timeout)

			// Check results
			if tt.expectedError {
				g.Expect(err).To(gomega.HaveOccurred())
				if tt.errorContains != "" {
					g.Expect(err.Error()).To(gomega.ContainSubstring(tt.errorContains))
				}
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
}

func TestApplyYAMLTemplate(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)
	manifestPath := "../sample-applications/hostedcontrolplanes/hypershift/hostedcluster-etcd-enc-key.yaml"

	tests := []struct {
		name           string
		manifestPath   string
		data           map[string]interface{}
		override       bool
		expectedError  bool
		errorContains  string
		verifyResource func(*testing.T, client.Client)
	}{
		{
			name:         "Valid etcd encryption key manifest",
			manifestPath: manifestPath,
			data: map[string]interface{}{
				"HostedClusterName": HostedClusterName,
				"ClustersNamespace": ClustersNamespace,
				"EtcdEncryptionKey": SampleETCDEncryptionKey,
			},
			override:      false,
			expectedError: false,
			verifyResource: func(t *testing.T, client client.Client) {
				secret := &corev1.Secret{}
				err := client.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-etcd-encryption-key", HostedClusterName),
					Namespace: ClustersNamespace,
				}, secret)
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(secret.Data).To(gomega.HaveKey("key"))
			},
		},
		{
			name:         "Missing template variable",
			manifestPath: manifestPath,
			data: map[string]interface{}{
				"HostedClusterName": HostedClusterName,
				// Missing ClustersNamespace and EtcdEncryptionKey
			},
			override:      false,
			expectedError: true,
		},
		{
			name:         "Override existing resource",
			manifestPath: manifestPath,
			data: map[string]interface{}{
				"HostedClusterName": HostedClusterName,
				"ClustersNamespace": ClustersNamespace,
				"EtcdEncryptionKey": SampleETCDEncryptionKey,
			},
			override:      true,
			expectedError: false,
			verifyResource: func(t *testing.T, client client.Client) {
				secret := &corev1.Secret{}
				err := client.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-etcd-encryption-key", HostedClusterName),
					Namespace: ClustersNamespace,
				}, secret)
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(secret.Data).To(gomega.HaveKey("key"))
			},
		},
		{
			name:         "Non-existent manifest file",
			manifestPath: "non-existent-file.yaml",
			data: map[string]interface{}{
				"HostedClusterName": HostedClusterName,
				"ClustersNamespace": ClustersNamespace,
				"EtcdEncryptionKey": SampleETCDEncryptionKey,
			},
			override:      false,
			expectedError: true,
			errorContains: "failed to read manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme and client for each test case
			scheme := createTestScheme()
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Test ApplyYAMLTemplate
			err := ApplyYAMLTemplate(context.Background(), client, tt.manifestPath, tt.override, tt.data)

			// Check results
			if tt.expectedError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				if tt.verifyResource != nil {
					tt.verifyResource(t, client)
				}
			}
		})
	}
}

func TestGetPullSecret(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name           string
		pullSecret     *corev1.Secret
		expectedSecret string
		expectError    bool
	}{
		{
			name: "Valid pull secret",
			pullSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pull-secret",
					Namespace: "openshift-config",
				},
				Data: map[string][]byte{
					".dockerconfigjson": []byte("test-secret-data"),
				},
			},
			expectedSecret: "test-secret-data",
			expectError:    false,
		},
		{
			name: "Pull secret without dockerconfigjson",
			pullSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pull-secret",
					Namespace: "openshift-config",
				},
				Data: map[string][]byte{
					"other-key": []byte("other-data"),
				},
			},
			expectedSecret: "",
			expectError:    true,
		},
		{
			name:        "No pull secret exists",
			pullSecret:  nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme and client for each test case
			scheme := createTestScheme()
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Create pull secret if provided
			if tt.pullSecret != nil {
				err := client.Create(context.Background(), tt.pullSecret)
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}

			// Test getPullSecret
			secret, err := getPullSecret(context.Background(), client)

			// Check results
			if tt.expectError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(secret).To(gomega.Equal(tt.expectedSecret))
			}
		})
	}
}

func TestValidateHCP(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)

	// Create a temporary slice with only the deployments we want to test
	testDeployments := []string{
		RequiredWorkingOperators[0],
		RequiredWorkingOperators[1],
	}

	tests := []struct {
		name          string
		deployments   []*appsv1.Deployment
		expectedError bool
		errorContains string
	}{
		{
			name: "All deployments ready",
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testDeployments[0],
						Namespace: HCPNamespace,
					},
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 1,
						Replicas:          1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testDeployments[1],
						Namespace: HCPNamespace,
					},
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 1,
						Replicas:          1,
					},
				},
			},
			expectedError: false,
		},
		{
			name:          "No deployments",
			deployments:   []*appsv1.Deployment{},
			expectedError: true,
			errorContains: "failed to become ready",
		},
		{
			name: "Deployment not ready",
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testDeployments[0],
						Namespace: HCPNamespace,
					},
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 0,
						Replicas:          1,
					},
				},
			},
			expectedError: true,
			errorContains: "failed to become ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme and client for each test case
			scheme := createTestScheme()
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Create deployments if provided
			for _, deployment := range tt.deployments {
				err := client.Create(context.Background(), deployment)
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}

			// Get the verification function with a short timeout and specific deployments
			verifyFunc := ValidateHCP(5*time.Second, testDeployments)

			// Run the verification
			err := verifyFunc(client, HCPNamespace)

			// Check the results
			if tt.expectedError {
				g.Expect(err).To(gomega.HaveOccurred())
				if tt.errorContains != "" {
					g.Expect(err.Error()).To(gomega.ContainSubstring(tt.errorContains))
				}
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
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
			err := RemoveMCE(h)
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

func TestIsHCPDeleted(t *testing.T) {
	gomega.RegisterTestingT(t)

	tests := []struct {
		name     string
		hcp      *hypershiftv1.HostedControlPlane
		expected bool
	}{
		{
			name: "Existing HCP",
			hcp: &hypershiftv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "test-namespace",
				},
			},
			expected: false,
		},
		{
			name:     "Non-existent HCP",
			hcp:      nil,
			expected: true,
		},
		{
			name: "HCP with finalizers",
			hcp: &hypershiftv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-hcp",
					Namespace:  "test-namespace",
					Finalizers: []string{"test-finalizer"},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new scheme and client for each test case
			scheme := createTestScheme()
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			h := &HCHandler{
				Ctx:    context.Background(),
				Client: client,
			}

			// Create the HCP if provided
			if tt.hcp != nil {
				err := client.Create(context.Background(), tt.hcp)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			}

			result := IsHCPDeleted(h, tt.hcp)
			gomega.Expect(result).To(gomega.Equal(tt.expected))
		})
	}
}

func TestIsHCDeleted(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)

	// Create a fake client with the scheme
	fakeClient := fake.NewClientBuilder().WithScheme(createTestScheme()).Build()

	tests := []struct {
		Name           string
		HC             *hypershiftv1.HostedCluster
		ExpectedResult bool
	}{
		{
			Name: "Existing HostedCluster",
			HC: &hypershiftv1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-ns",
				},
				Spec: hypershiftv1.HostedClusterSpec{
					Services: []hypershiftv1.ServicePublishingStrategyMapping{
						{
							Service: hypershiftv1.APIServer,
						},
					},
				},
			},
			ExpectedResult: false,
		},
		{
			Name:           "Non-existent HostedCluster",
			HC:             nil,
			ExpectedResult: true,
		},
		{
			Name: "HostedCluster with finalizers",
			HC: &hypershiftv1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-ns",
					Finalizers: []string{
						"hypershift.openshift.io/finalizer",
					},
				},
				Spec: hypershiftv1.HostedClusterSpec{
					Services: []hypershiftv1.ServicePublishingStrategyMapping{
						{
							Service: hypershiftv1.APIServer,
						},
					},
				},
			},
			ExpectedResult: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			// Clean up any existing HostedCluster
			_ = fakeClient.Delete(context.Background(), &hypershiftv1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-ns",
				},
			})

			// Create HostedCluster if provided
			if tc.HC != nil {
				err := fakeClient.Create(context.Background(), tc.HC)
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}

			// Create HCHandler
			h := &HCHandler{
				Ctx:           context.Background(),
				Client:        fakeClient,
				HostedCluster: tc.HC,
			}

			// Test IsHCDeleted
			result := IsHCDeleted(h)
			g.Expect(result).To(gomega.Equal(tc.ExpectedResult))
		})
	}
}

func TestAddHCPPluginToDPA(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)

	// Create a fake client with the scheme
	fakeClient := fake.NewClientBuilder().WithScheme(createTestScheme()).Build()

	tests := []struct {
		Name           string
		DPA            *oadpv1alpha1.DataProtectionApplication
		Overrides      bool
		ExpectedResult bool
	}{
		{
			Name: "DPA without plugin",
			DPA: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{},
						},
					},
				},
			},
			Overrides:      false,
			ExpectedResult: true,
		},
		{
			Name: "DPA with plugin",
			DPA: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginHypershift,
							},
						},
					},
				},
			},
			Overrides:      false,
			ExpectedResult: true,
		},
		{
			Name: "DPA with plugin and overrides",
			DPA: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginHypershift,
							},
						},
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.HypershiftPluginImageKey: "quay.io/hypershift/hypershift-oadp-plugin:latest",
					},
				},
			},
			Overrides:      true,
			ExpectedResult: true,
		},
		{
			Name: "DPA with other plugins",
			DPA: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								"aws",
								"csi",
							},
						},
					},
				},
			},
			Overrides:      false,
			ExpectedResult: true,
		},
		{
			Name:           "Non-existent DPA",
			DPA:            nil,
			Overrides:      false,
			ExpectedResult: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			// Clean up any existing DPA
			_ = fakeClient.Delete(context.Background(), &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
			})

			// Create DPA if provided
			if tc.DPA != nil {
				err := fakeClient.Create(context.Background(), tc.DPA)
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}

			// Create HCHandler
			h := &HCHandler{
				Ctx:    context.Background(),
				Client: fakeClient,
			}

			// Test AddHCPPluginToDPA
			err := AddHCPPluginToDPA(h, "test-ns", "test-dpa", tc.Overrides)
			if tc.ExpectedResult {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			} else {
				g.Expect(err).To(gomega.HaveOccurred())
			}

			// Verify the plugin was added
			if tc.DPA != nil {
				updatedDPA := &oadpv1alpha1.DataProtectionApplication{}
				err := fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "test-ns", Name: "test-dpa"}, updatedDPA)
				g.Expect(err).ToNot(gomega.HaveOccurred())

				// Check that the hypershift plugin was added
				pluginFound := false
				for _, plugin := range updatedDPA.Spec.Configuration.Velero.DefaultPlugins {
					if plugin == oadpv1alpha1.DefaultPluginHypershift {
						pluginFound = true
						break
					}
				}
				g.Expect(pluginFound).To(gomega.BeTrue())

				// Check that the override was added if requested
				if tc.Overrides {
					g.Expect(updatedDPA.Spec.UnsupportedOverrides).To(gomega.HaveKey(oadpv1alpha1.HypershiftPluginImageKey))
					g.Expect(updatedDPA.Spec.UnsupportedOverrides[oadpv1alpha1.HypershiftPluginImageKey]).To(gomega.Equal("quay.io/hypershift/hypershift-oadp-plugin:latest"))
				}
			}
		})
	}
}

func TestRemoveHCPPluginFromDPA(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)

	// Create a fake client with the scheme
	fakeClient := fake.NewClientBuilder().WithScheme(createTestScheme()).Build()

	tests := []struct {
		Name           string
		DPA            *oadpv1alpha1.DataProtectionApplication
		ExpectedResult bool
	}{
		{
			Name: "DPA with plugin and overrides",
			DPA: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginHypershift,
							},
						},
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.HypershiftPluginImageKey: "quay.io/hypershift/hypershift-oadp-plugin:latest",
					},
				},
			},
			ExpectedResult: true,
		},
		{
			Name: "DPA without plugin",
			DPA: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{},
						},
					},
				},
			},
			ExpectedResult: true,
		},
		{
			Name: "DPA with other plugins",
			DPA: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								"aws",
								"csi",
							},
						},
					},
				},
			},
			ExpectedResult: true,
		},
		{
			Name:           "Non-existent DPA",
			DPA:            nil,
			ExpectedResult: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			// Clean up any existing DPA
			_ = fakeClient.Delete(context.Background(), &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
			})

			// Create DPA if provided
			if tc.DPA != nil {
				err := fakeClient.Create(context.Background(), tc.DPA)
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}

			// Create HCHandler
			h := &HCHandler{
				Ctx:    context.Background(),
				Client: fakeClient,
			}

			// Test RemoveHCPPluginFromDPA
			err := RemoveHCPPluginFromDPA(h, "test-ns", "test-dpa")
			if tc.ExpectedResult {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			} else {
				g.Expect(err).To(gomega.HaveOccurred())
			}

			// Verify the plugin was removed
			if tc.DPA != nil {
				updatedDPA := &oadpv1alpha1.DataProtectionApplication{}
				err := fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "test-ns", Name: "test-dpa"}, updatedDPA)
				g.Expect(err).ToNot(gomega.HaveOccurred())

				// Check that the hypershift plugin was removed
				for _, plugin := range updatedDPA.Spec.Configuration.Velero.DefaultPlugins {
					g.Expect(plugin).ToNot(gomega.Equal(oadpv1alpha1.DefaultPluginHypershift))
				}

				// Check that the override was removed
				g.Expect(updatedDPA.Spec.UnsupportedOverrides).ToNot(gomega.HaveKey(oadpv1alpha1.HypershiftPluginImageKey))
			}
		})
	}
}

func TestIsHCPPluginAdded(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)

	// Create a fake client with the scheme
	fakeClient := fake.NewClientBuilder().WithScheme(createTestScheme()).Build()

	tests := []struct {
		Name           string
		DPA            *oadpv1alpha1.DataProtectionApplication
		ExpectedResult bool
	}{
		{
			Name: "DPA without plugin",
			DPA: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{},
						},
					},
				},
			},
			ExpectedResult: false,
		},
		{
			Name: "DPA with plugin",
			DPA: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginHypershift,
							},
						},
					},
				},
			},
			ExpectedResult: true,
		},
		{
			Name: "DPA without Velero configuration",
			DPA: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{},
			},
			ExpectedResult: false,
		},
		{
			Name:           "Non-existent DPA",
			DPA:            nil,
			ExpectedResult: false,
		},
		{
			Name: "DPA with other plugins",
			DPA: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								"aws",
								"csi",
							},
						},
					},
				},
			},
			ExpectedResult: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			// Clean up any existing DPA
			_ = fakeClient.Delete(context.Background(), &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
			})

			// Create DPA if provided
			if tc.DPA != nil {
				err := fakeClient.Create(context.Background(), tc.DPA)
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}

			// Test IsHCPPluginAdded
			result := IsHCPPluginAdded(fakeClient, "test-ns", "test-dpa")
			g.Expect(result).To(gomega.Equal(tc.ExpectedResult))
		})
	}
}

func TestFilterErrorLogs(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)
	tests := []struct {
		Name          string
		EntrySlice    []string
		ExpectedSlice []string
	}{
		{
			Name:          "Should filter out logs containing error pattern",
			EntrySlice:    []string{"some-error-template-test", "other-log"},
			ExpectedSlice: []string{"other-log"},
		},
		{
			Name:          "Should keep logs not containing error pattern",
			EntrySlice:    []string{"normal-log", "another-normal-log"},
			ExpectedSlice: []string{"normal-log", "another-normal-log"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			result := FilterErrorLogs(tc.EntrySlice)
			g.Expect(result).To(gomega.Equal(tc.ExpectedSlice))
		})
	}
}

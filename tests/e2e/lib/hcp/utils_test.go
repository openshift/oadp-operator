package hcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/onsi/gomega"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFilterErrorLogs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name           string
		logs           []string
		expectedResult []string
	}{
		{
			name: "No error logs to filter",
			logs: []string{
				"Normal log 1",
				"Normal log 2",
				"Normal log 3",
			},
			expectedResult: []string{
				"Normal log 1",
				"Normal log 2",
				"Normal log 3",
			},
		},
		{
			name: "Filter error logs",
			logs: []string{
				"Normal log 1",
				"Error log with -error-template",
				"Normal log 2",
				"Another error with -error-template",
				"Normal log 3",
			},
			expectedResult: []string{
				"Normal log 1",
				"Normal log 2",
				"Normal log 3",
			},
		},
		{
			name:           "Empty logs",
			logs:           []string{},
			expectedResult: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterErrorLogs(tt.logs)
			g.Expect(result).To(gomega.Equal(tt.expectedResult))
		})
	}
}

func TestApplyYAMLTemplate(t *testing.T) {
	// Register Gomega fail handler
	gomega.RegisterTestingT(t)
	g := gomega.NewGomegaWithT(t)
	manifestPath := "../../sample-applications/hostedcontrolplanes/hypershift/hostedcluster-etcd-enc-key.yaml"
	hostedClusterName := "test-hc"

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
				"HostedClusterName": hostedClusterName,
				"ClustersNamespace": ClustersNamespace,
				"EtcdEncryptionKey": SampleETCDEncryptionKey,
			},
			override:      false,
			expectedError: false,
			verifyResource: func(t *testing.T, client client.Client) {
				secret := &corev1.Secret{}
				err := client.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-etcd-encryption-key", hostedClusterName),
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
				"HostedClusterName": hostedClusterName,
				// Missing ClustersNamespace and EtcdEncryptionKey
			},
			override:      false,
			expectedError: true,
		},
		{
			name:         "Override existing resource",
			manifestPath: manifestPath,
			data: map[string]interface{}{
				"HostedClusterName": hostedClusterName,
				"ClustersNamespace": ClustersNamespace,
				"EtcdEncryptionKey": SampleETCDEncryptionKey,
			},
			override:      true,
			expectedError: false,
			verifyResource: func(t *testing.T, client client.Client) {
				secret := &corev1.Secret{}
				err := client.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-etcd-encryption-key", hostedClusterName),
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
				"HostedClusterName": hostedClusterName,
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
		verifyHandler   func(*testing.T, *HCHandler, client.Client)
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
			verifyHandler: func(t *testing.T, h *HCHandler, c client.Client) {
				g.Expect(h).ToNot(gomega.BeNil())
				g.Expect(h.HCOCPTestImage).To(gomega.Equal(HCOCPTestImage))

				// Verify namespace was created
				ns := &corev1.Namespace{}
				err := c.Get(context.Background(), types.NamespacedName{Name: MCENamespace}, ns)
				g.Expect(err).ToNot(gomega.HaveOccurred())

				// Verify operator group was created
				og := &operatorsv1.OperatorGroup{}
				err = c.Get(context.Background(), types.NamespacedName{Name: MCEOperatorGroup, Namespace: MCENamespace}, og)
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(og.Spec.TargetNamespaces).To(gomega.Equal([]string{MCENamespace}))

				// Verify subscription was created
				sub := &operatorsv1alpha1.Subscription{}
				err = c.Get(context.Background(), types.NamespacedName{Name: MCEName, Namespace: MCENamespace}, sub)
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(sub.Spec.Channel).To(gomega.Equal("stable-2.0"))
				g.Expect(sub.Spec.CatalogSource).To(gomega.Equal(RHOperatorsNamespace))
				g.Expect(sub.Spec.CatalogSourceNamespace).To(gomega.Equal(OCPMarketplaceNamespace))
				g.Expect(sub.Spec.Package).To(gomega.Equal(MCEName))
				g.Expect(sub.Spec.InstallPlanApproval).To(gomega.Equal(operatorsv1alpha1.ApprovalAutomatic))
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
			verifyHandler: func(t *testing.T, h *HCHandler, c client.Client) {
				g.Expect(h).ToNot(gomega.BeNil())

				// Verify subscription was created with correct channel
				sub := &operatorsv1alpha1.Subscription{}
				err := c.Get(context.Background(), types.NamespacedName{Name: MCEName, Namespace: MCENamespace}, sub)
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(sub.Spec.Channel).To(gomega.Equal("stable-2.1"))
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
			verifyHandler: func(t *testing.T, h *HCHandler, c client.Client) {
				g.Expect(h).ToNot(gomega.BeNil())

				// Verify subscription was created with correct CSV
				sub := &operatorsv1alpha1.Subscription{}
				err := c.Get(context.Background(), types.NamespacedName{Name: MCEName, Namespace: MCENamespace}, sub)
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(sub.Spec.StartingCSV).To(gomega.Equal("multicluster-engine.v2.1.0"))
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
					tt.verifyHandler(t, h, client)
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

func TestGetProjectRoot(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Get the project root using the function
	root := getProjectRoot()

	// Verify that the path exists
	_, err := os.Stat(root)
	g.Expect(err).ToNot(gomega.HaveOccurred(), "project root path should exist")

	// Verify that the path contains expected directories
	expectedDirs := []string{
		"tests",
		"go.mod",
		"go.sum",
	}

	for _, dir := range expectedDirs {
		path := filepath.Join(root, dir)
		_, err := os.Stat(path)
		g.Expect(err).ToNot(gomega.HaveOccurred(), "expected directory/file %s should exist in project root", dir)
	}

	// Verify that the path is absolute
	g.Expect(filepath.IsAbs(root)).To(gomega.BeTrue(), "project root path should be absolute")

	// Verify that the path points to the correct directory by checking for a known file
	knownFile := filepath.Join(root, "tests", "e2e", "lib", "hcp", "utils.go")
	_, err = os.Stat(knownFile)
	g.Expect(err).ToNot(gomega.HaveOccurred(), "should be able to find utils.go in the expected location")
}

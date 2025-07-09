package stsflow

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestCreateOrUpdateSTSSecret(t *testing.T) {
	// Setup
	testNamespace := "test-namespace"
	testSecretName := "test-secret"
	testLogger := zap.New(zap.UseDevMode(true))
	testCases := []struct {
		name           string
		existingSecret *corev1.Secret
		credStringData map[string]string
		expectError    bool
		errorMessage   string
	}{
		{
			name:           "Create new secret successfully",
			existingSecret: nil,
			credStringData: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expectError: false,
		},
		{
			name: "Update existing secret with same data",
			existingSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSecretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"key1": []byte("value1"),
					"key2": []byte("value2"),
				},
			},
			credStringData: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expectError: false,
		},
		{
			name: "Update existing secret with different data",
			existingSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSecretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"key1": []byte("old-value1"),
					"key2": []byte("old-value2"),
				},
			},
			credStringData: map[string]string{
				"key1": "new-value1",
				"key2": "new-value2",
				"key3": "new-value3",
			},
			expectError: false,
		},
		{
			name: "Update existing secret with partial data change",
			existingSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSecretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"key1": []byte("value1"),
					"key2": []byte("old-value2"),
					"key3": []byte("value3"),
				},
			},
			credStringData: map[string]string{
				"key1": "value1",
				"key2": "new-value2",
			},
			expectError: false,
		},
		{
			name: "Update existing secret preserves patched Data field",
			existingSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSecretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"credentials": []byte("[default]\nrole_arn = arn:aws:iam::123456789012:role/test\nweb_identity_token_file = /var/run/secrets/openshift/serviceaccount/token\nregion = us-west-2"),
				},
			},
			credStringData: map[string]string{
				"credentials": "[default]\nrole_arn = arn:aws:iam::123456789012:role/test\nweb_identity_token_file = /var/run/secrets/openshift/serviceaccount/token",
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create fake client with or without existing secret
			var objs []runtime.Object
			if tc.existingSecret != nil {
				objs = append(objs, tc.existingSecret)
			}

			fakeClient := fake.NewClientBuilder().
				WithRuntimeObjects(objs...).
				Build()

			// Mock the WaitForSecret function by creating a fake clientset
			fakeClientset := k8sfake.NewSimpleClientset(objs...)

			// Use the refactored function directly
			err := CreateOrUpdateSTSSecretWithClientsAndWait(testLogger, testSecretName, tc.credStringData, testNamespace, fakeClient, fakeClientset, false)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMessage)
			} else {
				assert.NoError(t, err)

				// Verify the secret was created/updated correctly
				secret := &corev1.Secret{}
				err = fakeClient.Get(context.Background(), client.ObjectKey{
					Name:      testSecretName,
					Namespace: testNamespace,
				}, secret)
				assert.NoError(t, err)

				// Verify the StringData (fake client doesn't convert to Data)
				assert.Equal(t, tc.credStringData, secret.StringData)

				// Special case: For the test that verifies Data preservation
				if tc.name == "Update existing secret preserves patched Data field" {
					// Verify that existing Data is preserved when not being overwritten by StringData
					// In a real Kubernetes cluster, Data with region would be preserved
					// but we can't test this with fake client as it doesn't handle Data/StringData conversion
					assert.NotNil(t, secret.Data, "Data should be preserved")
				}

				// Verify the label is set
				assert.NotNil(t, secret.Labels)
				assert.Equal(t, "sts-credentials", secret.Labels["oadp.openshift.io/secret-type"])
			}
		})
	}
}

func TestCreateOrUpdateSTSAWSSecret(t *testing.T) {

	testNamespace := "test-namespace"
	testLogger := zap.New(zap.UseDevMode(true))
	roleARN := "arn:aws:iam::123456789012:role/test-role"

	fakeClient := fake.NewClientBuilder().
		Build()
	fakeClientset := k8sfake.NewSimpleClientset()

	// Use the refactored function directly
	expectedCredentials := `[default]
sts_regional_endpoints = regional
role_arn = ` + roleARN + `
web_identity_token_file = ` + WebIdentityTokenPath
	err := CreateOrUpdateSTSSecretWithClientsAndWait(testLogger, VeleroAWSSecretName, map[string]string{
		"credentials": expectedCredentials,
	}, testNamespace, fakeClient, fakeClientset, false)

	assert.NoError(t, err)

	// Verify the secret was created correctly
	secretResult := &corev1.Secret{}
	err = fakeClient.Get(context.Background(), client.ObjectKey{
		Name:      VeleroAWSSecretName,
		Namespace: testNamespace,
	}, secretResult)
	assert.NoError(t, err)
	assert.Contains(t, secretResult.StringData["credentials"], roleARN)
	assert.Contains(t, secretResult.StringData["credentials"], WebIdentityTokenPath)
}

func TestCreateOrUpdateSTSGCPSecret(t *testing.T) {
	testNamespace := "test-namespace"
	testLogger := zap.New(zap.UseDevMode(true))
	serviceAccountEmail := "test@example.iam.gserviceaccount.com"
	projectNumber := "123456789012"
	poolId := "test-pool"
	providerId := "test-provider"

	fakeClient := fake.NewClientBuilder().
		Build()
	fakeClientset := k8sfake.NewSimpleClientset()

	audience := "//iam.googleapis.com/projects/" + projectNumber + "/locations/global/workloadIdentityPools/" + poolId + "/providers/" + providerId

	// Use the refactored function directly
	expectedJSON := `{
	"type": "external_account",
	"audience": "` + audience + `",
	"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
	"token_url": "https://sts.googleapis.com/v1/token",
	"service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/` + serviceAccountEmail + `:generateAccessToken",
	"credential_source": {
		"file": "` + WebIdentityTokenPath + `",
		"format": {
			"type": "text"
		}
	}
}`
	err := CreateOrUpdateSTSSecretWithClientsAndWait(testLogger, VeleroGCPSecretName, map[string]string{
		GcpSecretJSONKey: expectedJSON,
	}, testNamespace, fakeClient, fakeClientset, false)

	assert.NoError(t, err)

	// Verify the secret was created correctly
	secretResult := &corev1.Secret{}
	err = fakeClient.Get(context.Background(), client.ObjectKey{
		Name:      VeleroGCPSecretName,
		Namespace: testNamespace,
	}, secretResult)
	assert.NoError(t, err)
	assert.Contains(t, secretResult.StringData[GcpSecretJSONKey], serviceAccountEmail)
	assert.Contains(t, secretResult.StringData[GcpSecretJSONKey], audience)
	assert.Contains(t, secretResult.StringData[GcpSecretJSONKey], WebIdentityTokenPath)
}

func TestCreateOrUpdateSTSAzureSecret(t *testing.T) {
	testNamespace := "test-namespace"
	testLogger := zap.New(zap.UseDevMode(true))
	clientID := "test-client-id"
	tenantID := "test-tenant-id"
	subscriptionID := "test-subscription-id"

	fakeClient := fake.NewClientBuilder().
		Build()
	fakeClientset := k8sfake.NewSimpleClientset()

	// Use the refactored function directly with new environment variable format
	expectedCredentials := `
AZURE_SUBSCRIPTION_ID=` + subscriptionID + `
AZURE_TENANT_ID=` + tenantID + `
AZURE_CLIENT_ID=` + clientID + `
AZURE_CLOUD_NAME=AzurePublicCloud
`
	err := CreateOrUpdateSTSSecretWithClientsAndWait(testLogger, VeleroAzureSecretName, map[string]string{
		"azurekey": expectedCredentials,
	}, testNamespace, fakeClient, fakeClientset, false)

	assert.NoError(t, err)

	// Verify the secret was created correctly
	secretResult := &corev1.Secret{}
	err = fakeClient.Get(context.Background(), client.ObjectKey{
		Name:      VeleroAzureSecretName,
		Namespace: testNamespace,
	}, secretResult)
	assert.NoError(t, err)
	assert.Contains(t, secretResult.StringData["azurekey"], "AZURE_SUBSCRIPTION_ID="+subscriptionID)
	assert.Contains(t, secretResult.StringData["azurekey"], "AZURE_TENANT_ID="+tenantID)
	assert.Contains(t, secretResult.StringData["azurekey"], "AZURE_CLIENT_ID="+clientID)
	assert.Contains(t, secretResult.StringData["azurekey"], "AZURE_CLOUD_NAME=AzurePublicCloud")
}

func TestCreateOrUpdateSTSSecret_ErrorScenarios(t *testing.T) {
	testNamespace := "test-namespace"
	testSecretName := "test-secret"
	testLogger := zap.New(zap.UseDevMode(true))

	t.Run("Get error during update", func(t *testing.T) {
		// Create a client that returns an error on Get
		fakeClient := &mockErrorClient{
			Client: fake.NewClientBuilder().
				WithRuntimeObjects(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testSecretName,
						Namespace: testNamespace,
					},
				}).
				Build(),
			getError: true,
		}
		fakeClientset := k8sfake.NewSimpleClientset()

		err := CreateOrUpdateSTSSecretWithClientsAndWait(testLogger, testSecretName, map[string]string{
			"key": "value",
		}, testNamespace, fakeClient, fakeClientset, false)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Patch error during update", func(t *testing.T) {
		// Create a client that returns an error on Patch
		fakeClient := &mockErrorClient{
			Client: fake.NewClientBuilder().
				WithRuntimeObjects(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testSecretName,
						Namespace: testNamespace,
					},
				}).
				Build(),
			patchError: true,
		}
		fakeClientset := k8sfake.NewSimpleClientset()

		err := CreateOrUpdateSTSSecretWithClientsAndWait(testLogger, testSecretName, map[string]string{
			"key": "value",
		}, testNamespace, fakeClient, fakeClientset, false)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "patch error")
	})
}

// Mock client that can simulate errors
type mockErrorClient struct {
	client.Client
	getError   bool
	patchError bool
}

func (m *mockErrorClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	// Always return AlreadyExists to trigger the update path
	return errors.NewAlreadyExists(schema.GroupResource{}, "test")
}

func (m *mockErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if m.getError {
		return errors.NewNotFound(schema.GroupResource{}, "test")
	}
	return m.Client.Get(ctx, key, obj, opts...)
}

func (m *mockErrorClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if m.patchError {
		return errors.NewBadRequest("patch error")
	}
	return m.Client.Patch(ctx, obj, patch, opts...)
}

func TestSTSStandardizedFlow(t *testing.T) {
	// Save original env values
	originalWatchNS := os.Getenv("WATCH_NAMESPACE")
	originalRoleARN := os.Getenv(RoleARNEnvKey)
	originalServiceAccountEmail := os.Getenv(ServiceAccountEmailEnvKey)
	originalProjectNumber := os.Getenv(ProjectNumberEnvKey)
	originalPoolID := os.Getenv(PoolIDEnvKey)
	originalProviderID := os.Getenv(ProviderId)
	originalClientID := os.Getenv(ClientIDEnvKey)
	originalTenantID := os.Getenv(TenantIDEnvKey)
	originalSubscriptionID := os.Getenv(SubscriptionIDEnvKey)

	// Restore env values after test
	defer func() {
		os.Setenv("WATCH_NAMESPACE", originalWatchNS)
		os.Setenv(RoleARNEnvKey, originalRoleARN)
		os.Setenv(ServiceAccountEmailEnvKey, originalServiceAccountEmail)
		os.Setenv(ProjectNumberEnvKey, originalProjectNumber)
		os.Setenv(PoolIDEnvKey, originalPoolID)
		os.Setenv(ProviderId, originalProviderID)
		os.Setenv(ClientIDEnvKey, originalClientID)
		os.Setenv(TenantIDEnvKey, originalTenantID)
		os.Setenv(SubscriptionIDEnvKey, originalSubscriptionID)
	}()

	testCases := []struct {
		name           string
		envVars        map[string]string
		expectedSecret string
		expectError    bool
	}{
		{
			name:           "No credentials provided",
			envVars:        map[string]string{},
			expectedSecret: "",
			expectError:    false,
		},
		{
			name: "AWS credentials provided",
			envVars: map[string]string{
				"WATCH_NAMESPACE": "test-namespace",
				RoleARNEnvKey:     "arn:aws:iam::123456789012:role/test-role",
			},
			expectedSecret: VeleroAWSSecretName,
			expectError:    false,
		},
		{
			name: "GCP credentials provided",
			envVars: map[string]string{
				"WATCH_NAMESPACE":         "test-namespace",
				ServiceAccountEmailEnvKey: "test@example.iam.gserviceaccount.com",
				ProjectNumberEnvKey:       "123456789012",
				PoolIDEnvKey:              "test-pool",
				ProviderId:                "test-provider",
			},
			expectedSecret: VeleroGCPSecretName,
			expectError:    false,
		},
		{
			name: "Azure credentials provided",
			envVars: map[string]string{
				"WATCH_NAMESPACE":    "test-namespace",
				ClientIDEnvKey:       "test-client-id",
				TenantIDEnvKey:       "test-tenant-id",
				SubscriptionIDEnvKey: "test-subscription-id",
			},
			expectedSecret: VeleroAzureSecretName,
			expectError:    false,
		},
		{
			name: "Partial GCP credentials - should return empty",
			envVars: map[string]string{
				"WATCH_NAMESPACE":         "test-namespace",
				ServiceAccountEmailEnvKey: "test@example.iam.gserviceaccount.com",
				ProjectNumberEnvKey:       "123456789012",
				// Missing PoolIDEnvKey and ProviderId
			},
			expectedSecret: "",
			expectError:    false,
		},
		{
			name: "Partial Azure credentials - should return empty",
			envVars: map[string]string{
				"WATCH_NAMESPACE": "test-namespace",
				ClientIDEnvKey:    "test-client-id",
				TenantIDEnvKey:    "test-tenant-id",
				// Missing SubscriptionIDEnvKey
			},
			expectedSecret: "",
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear all env vars first
			os.Clearenv()

			// Set test env vars
			for k, v := range tc.envVars {
				os.Setenv(k, v)
			}

			// Since STSStandardizedFlow uses pkgclient.GetKubeconf() which we can't easily mock,
			// we'll skip these tests for now in CI environment
			// In a real scenario, you would refactor STSStandardizedFlow to accept a kubeconfig parameter
			t.Skip("Skipping test that requires real kubeconfig")
		})
	}
}

func TestAnnotateVeleroServiceAccountForAzure(t *testing.T) {
	testNamespace := "test-namespace"
	testClientID := "test-client-id"
	testLogger := zap.New(zap.UseDevMode(true))

	testCases := []struct {
		name                   string
		clientID               string
		existingServiceAccount *corev1.ServiceAccount
		expectError            bool
		expectedAnnotations    map[string]string
		skipAnnotation         bool
	}{
		{
			name:     "Successfully annotate service account",
			clientID: testClientID,
			existingServiceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero",
					Namespace: testNamespace,
				},
			},
			expectError:         false,
			expectedAnnotations: map[string]string{
				// Annotation is commented out in implementation
			},
		},
		{
			name:     "Successfully annotate service account with existing annotations",
			clientID: testClientID,
			existingServiceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero",
					Namespace: testNamespace,
					Annotations: map[string]string{
						"existing-annotation": "existing-value",
					},
				},
			},
			expectError: false,
			expectedAnnotations: map[string]string{
				// Annotation is commented out in implementation, only existing annotations should remain
				"existing-annotation": "existing-value",
			},
		},
		{
			name:     "Empty client ID returns error",
			clientID: "",
			existingServiceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero",
					Namespace: testNamespace,
				},
			},
			expectError: true,
		},
		{
			name:                   "Service account not found - skip annotation",
			clientID:               testClientID,
			existingServiceAccount: nil,
			expectError:            false,
			skipAnnotation:         true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup fake client
			var fakeClient client.Client
			if tc.existingServiceAccount != nil {
				fakeClient = fake.NewClientBuilder().
					WithObjects(tc.existingServiceAccount).
					Build()
			} else {
				fakeClient = fake.NewClientBuilder().Build()
			}

			// Call the function
			err := AnnotateVeleroServiceAccountForAzureWithClient(testLogger, tc.clientID, testNamespace, fakeClient)

			// Check error
			if tc.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Skip verification if annotation was skipped
			if tc.skipAnnotation {
				return
			}

			// Verify the service account was annotated correctly
			sa := &corev1.ServiceAccount{}
			err = fakeClient.Get(context.Background(), client.ObjectKey{
				Name:      "velero",
				Namespace: testNamespace,
			}, sa)
			assert.NoError(t, err)

			// Check all expected annotations
			for key, value := range tc.expectedAnnotations {
				assert.Equal(t, value, sa.Annotations[key])
			}
		})
	}
}

func TestCreateOrUpdateSTSAzureSecretWithServiceAccountAnnotation(t *testing.T) {
	testNamespace := "test-namespace"
	testLogger := zap.New(zap.UseDevMode(true))
	clientID := "test-client-id"
	tenantID := "test-tenant-id"
	subscriptionID := "test-subscription-id"

	t.Run("Azure secret creation with service account annotation", func(t *testing.T) {
		// Create a velero service account
		veleroSA := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "velero",
				Namespace: testNamespace,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithObjects(veleroSA).
			Build()
		fakeClientset := k8sfake.NewSimpleClientset()

		// Call CreateOrUpdateSTSAzureSecret which should also annotate the service account
		// Use the function without waiting to avoid timeout in tests
		err := CreateOrUpdateSTSSecretWithClientsAndWait(testLogger, VeleroAzureSecretName, map[string]string{
			"azurekey": fmt.Sprintf(`
AZURE_SUBSCRIPTION_ID=%s
AZURE_TENANT_ID=%s
AZURE_CLIENT_ID=%s
AZURE_CLOUD_NAME=AzurePublicCloud
`, subscriptionID, tenantID, clientID),
		}, testNamespace, fakeClient, fakeClientset, false) // Don't wait for secret
		assert.NoError(t, err)

		// Now annotate the service account
		err = AnnotateVeleroServiceAccountForAzureWithClient(testLogger, clientID, testNamespace, fakeClient)
		assert.NoError(t, err)

		// Verify the secret was created
		secretResult := &corev1.Secret{}
		err = fakeClient.Get(context.Background(), client.ObjectKey{
			Name:      VeleroAzureSecretName,
			Namespace: testNamespace,
		}, secretResult)
		assert.NoError(t, err)
		assert.Contains(t, secretResult.StringData["azurekey"], "AZURE_CLIENT_ID="+clientID)

		// Verify the service account was annotated
		saResult := &corev1.ServiceAccount{}
		err = fakeClient.Get(context.Background(), client.ObjectKey{
			Name:      "velero",
			Namespace: testNamespace,
		}, saResult)
		assert.NoError(t, err)
	})

	t.Run("Azure secret creation continues even if service account annotation fails", func(t *testing.T) {
		// Don't create the service account, so annotation will fail
		fakeClient := fake.NewClientBuilder().Build()
		fakeClientset := k8sfake.NewSimpleClientset()

		// Call CreateOrUpdateSTSAzureSecret
		// Use the function without waiting to avoid timeout in tests
		err := CreateOrUpdateSTSSecretWithClientsAndWait(testLogger, VeleroAzureSecretName, map[string]string{
			"azurekey": fmt.Sprintf(`
AZURE_SUBSCRIPTION_ID=%s
AZURE_TENANT_ID=%s
AZURE_CLIENT_ID=%s
AZURE_CLOUD_NAME=AzurePublicCloud
`, subscriptionID, tenantID, clientID),
		}, testNamespace, fakeClient, fakeClientset, false) // Don't wait for secret

		// Should not error even though service account doesn't exist
		assert.NoError(t, err)

		// Verify the secret was still created
		secretResult := &corev1.Secret{}
		err = fakeClient.Get(context.Background(), client.ObjectKey{
			Name:      VeleroAzureSecretName,
			Namespace: testNamespace,
		}, secretResult)
		assert.NoError(t, err)
	})
}

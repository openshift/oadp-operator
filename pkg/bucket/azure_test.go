package bucket

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials/stsflow"
)

func TestValidateContainerName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"valid name", "mycontainer", false},
		{"valid with numbers", "container123", false},
		{"valid with hyphens", "my-container", false},
		{"too short", "ab", true},
		{"too long", strings.Repeat("a", 64), true},
		{"starts with hyphen", "-container", true},
		{"consecutive hyphens", "my--container", true},
		{"uppercase letters", "MyContainer", true},
		{"special characters", "my_container", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContainerName(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateStorageAccountName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"valid name", "mystorageaccount", false},
		{"valid with numbers", "storage123", false},
		{"too short", "ab", true},
		{"too long", strings.Repeat("a", 25), true},
		{"uppercase letters", "MyStorage", true},
		{"special characters", "my-storage", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStorageAccountName(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetStorageAccountName(t *testing.T) {
	tests := []struct {
		name        string
		secretData  map[string][]byte
		config      map[string]string
		expectError bool
		expected    string
	}{
		{
			name: "valid account from secret",
			secretData: map[string][]byte{
				"AZURE_STORAGE_ACCOUNT": []byte("mystorageaccount"),
			},
			expectError: false,
			expected:    "mystorageaccount",
		},
		{
			name: "valid account from azurekey",
			secretData: map[string][]byte{
				"azurekey": []byte(`
AZURE_SUBSCRIPTION_ID=12345678-1234-1234-1234-123456789012
AZURE_STORAGE_ACCOUNT=azurekeystorageaccount
AZURE_TENANT_ID=87654321-4321-4321-4321-210987654321
`),
			},
			expectError: false,
			expected:    "azurekeystorageaccount",
		},
		{
			name:       "valid account from config",
			secretData: map[string][]byte{},
			config: map[string]string{
				"storageAccount": "configstorageaccount",
			},
			expectError: false,
			expected:    "configstorageaccount",
		},
		{
			name: "priority: secret over azurekey",
			secretData: map[string][]byte{
				"AZURE_STORAGE_ACCOUNT": []byte("secretaccount"),
				"azurekey":              []byte("AZURE_STORAGE_ACCOUNT=azurekeyaccount"),
			},
			expectError: false,
			expected:    "secretaccount",
		},
		{
			name: "priority: azurekey over config",
			secretData: map[string][]byte{
				"azurekey": []byte("AZURE_STORAGE_ACCOUNT=azurekeyaccount"),
			},
			config: map[string]string{
				"storageAccount": "configaccount",
			},
			expectError: false,
			expected:    "azurekeyaccount",
		},
		{
			name:        "missing account name",
			secretData:  map[string][]byte{},
			expectError: true,
		},
		{
			name: "empty account name in secret",
			secretData: map[string][]byte{
				"AZURE_STORAGE_ACCOUNT": []byte(""),
			},
			expectError: true,
		},
		{
			name: "azurekey without storage account",
			secretData: map[string][]byte{
				"azurekey": []byte(`
AZURE_SUBSCRIPTION_ID=12345678-1234-1234-1234-123456789012
AZURE_TENANT_ID=87654321-4321-4321-4321-210987654321
`),
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket := v1alpha1.CloudStorage{
				Spec: v1alpha1.CloudStorageSpec{
					Config: tt.config,
				},
			}
			secret := &corev1.Secret{Data: tt.secretData}

			result, err := getStorageAccountName(bucket, secret)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestValidateAndConvertTags(t *testing.T) {
	azureClient := &azureBucketClient{
		bucket: v1alpha1.CloudStorage{
			Spec: v1alpha1.CloudStorageSpec{
				Tags: map[string]string{
					"environment": "production",
					"team":        "backup",
				},
			},
		},
	}

	err := azureClient.validateAndConvertTags()
	assert.NoError(t, err)

	// Test too many tags
	manyTags := make(map[string]string)
	for i := 0; i < 51; i++ {
		manyTags[fmt.Sprintf("key%d", i)] = "value"
	}
	azureClient.bucket.Spec.Tags = manyTags

	err = azureClient.validateAndConvertTags()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too many tags")

	// Test tag name too long
	azureClient.bucket.Spec.Tags = map[string]string{
		strings.Repeat("a", 513): "value",
	}
	err = azureClient.validateAndConvertTags()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be between 1 and 512 characters")

	// Test tag value too long
	azureClient.bucket.Spec.Tags = map[string]string{
		"key": strings.Repeat("v", 257),
	}
	err = azureClient.validateAndConvertTags()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be 256 characters or less")
}

func TestIsRetryableError(t *testing.T) {
	// Test retryable errors
	retryableErr := &azcore.ResponseError{
		StatusCode: 503,
		ErrorCode:  "ServiceUnavailable",
	}
	assert.True(t, isRetryableError(retryableErr))

	// Test Too Many Requests
	tooManyRequestsErr := &azcore.ResponseError{
		StatusCode: 429,
		ErrorCode:  "TooManyRequests",
	}
	assert.True(t, isRetryableError(tooManyRequestsErr))

	// Test server errors
	serverErr := &azcore.ResponseError{
		StatusCode: 500,
		ErrorCode:  "InternalError",
	}
	assert.True(t, isRetryableError(serverErr))

	// Test non-retryable errors
	nonRetryableErr := &azcore.ResponseError{
		StatusCode: 401,
		ErrorCode:  "AuthenticationFailed",
	}
	assert.False(t, isRetryableError(nonRetryableErr))

	// Test authorization error
	authzErr := &azcore.ResponseError{
		StatusCode: 403,
		ErrorCode:  "AuthorizationFailed",
	}
	assert.False(t, isRetryableError(authzErr))

	// Test bad request
	badRequestErr := &azcore.ResponseError{
		StatusCode: 400,
		ErrorCode:  "InvalidResourceName",
	}
	assert.False(t, isRetryableError(badRequestErr))

	// Test not found
	notFoundErr := &azcore.ResponseError{
		StatusCode: 404,
		ErrorCode:  "AccountNotFound",
	}
	assert.False(t, isRetryableError(notFoundErr))

	// Test network-related errors
	networkErr := fmt.Errorf("connection timeout")
	assert.True(t, isRetryableError(networkErr))

	networkErr2 := fmt.Errorf("network unreachable")
	assert.True(t, isRetryableError(networkErr2))
}

func TestHasWorkloadIdentityCredentials(t *testing.T) {
	azureClient := &azureBucketClient{}

	tests := []struct {
		name         string
		secretData   map[string][]byte
		secretLabels map[string]string
		expected     bool
	}{
		{
			name: "valid STS secret with azurekey",
			secretData: map[string][]byte{
				"azurekey": []byte(`
AZURE_SUBSCRIPTION_ID=12345678-1234-1234-1234-123456789012
AZURE_TENANT_ID=87654321-4321-4321-4321-210987654321
AZURE_CLIENT_ID=abcdef12-3456-7890-abcd-ef1234567890
AZURE_CLOUD_NAME=AzurePublicCloud
`),
			},
			secretLabels: map[string]string{
				stsflow.STSSecretLabelKey: stsflow.STSSecretLabelValue,
			},
			expected: true,
		},
		{
			name: "STS secret missing required fields",
			secretData: map[string][]byte{
				"azurekey": []byte(`
AZURE_SUBSCRIPTION_ID=12345678-1234-1234-1234-123456789012
AZURE_CLOUD_NAME=AzurePublicCloud
`),
			},
			secretLabels: map[string]string{
				stsflow.STSSecretLabelKey: stsflow.STSSecretLabelValue,
			},
			expected: false,
		},
		{
			name: "STS secret without label",
			secretData: map[string][]byte{
				"azurekey": []byte(`
AZURE_SUBSCRIPTION_ID=12345678-1234-1234-1234-123456789012
AZURE_TENANT_ID=87654321-4321-4321-4321-210987654321
AZURE_CLIENT_ID=abcdef12-3456-7890-abcd-ef1234567890
`),
			},
			expected: false,
		},
		{
			name: "secret with AZURE_FEDERATED_TOKEN_FILE",
			secretData: map[string][]byte{
				"AZURE_TENANT_ID":            []byte("tenant-id"),
				"AZURE_CLIENT_ID":            []byte("client-id"),
				"AZURE_FEDERATED_TOKEN_FILE": []byte("/var/run/secrets/openshift/serviceaccount/token"),
			},
			expected: true,
		},
		{
			name: "secret with AZURE_FEDERATED_TOKEN_FILE missing tenant",
			secretData: map[string][]byte{
				"AZURE_CLIENT_ID":            []byte("client-id"),
				"AZURE_FEDERATED_TOKEN_FILE": []byte("/var/run/secrets/openshift/serviceaccount/token"),
			},
			expected: false,
		},
		{
			name:       "empty secret",
			secretData: map[string][]byte{},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: tt.secretLabels,
				},
				Data: tt.secretData,
			}
			result := azureClient.hasWorkloadIdentityCredentials(secret)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasServicePrincipalCredentials(t *testing.T) {
	azureClient := &azureBucketClient{}

	tests := []struct {
		name       string
		secretData map[string][]byte
		expected   bool
	}{
		{
			name: "valid service principal credentials",
			secretData: map[string][]byte{
				"AZURE_TENANT_ID":     []byte("tenant-id"),
				"AZURE_CLIENT_ID":     []byte("client-id"),
				"AZURE_CLIENT_SECRET": []byte("client-secret"),
			},
			expected: true,
		},
		{
			name: "missing tenant ID",
			secretData: map[string][]byte{
				"AZURE_CLIENT_ID":     []byte("client-id"),
				"AZURE_CLIENT_SECRET": []byte("client-secret"),
			},
			expected: false,
		},
		{
			name: "missing client ID",
			secretData: map[string][]byte{
				"AZURE_TENANT_ID":     []byte("tenant-id"),
				"AZURE_CLIENT_SECRET": []byte("client-secret"),
			},
			expected: false,
		},
		{
			name: "missing client secret",
			secretData: map[string][]byte{
				"AZURE_TENANT_ID": []byte("tenant-id"),
				"AZURE_CLIENT_ID": []byte("client-id"),
			},
			expected: false,
		},
		{
			name:       "empty secret",
			secretData: map[string][]byte{},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := &corev1.Secret{Data: tt.secretData}
			result := azureClient.hasServicePrincipalCredentials(secret)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Mock implementations for testing

type mockAzureServiceClient struct {
	containerClient azureContainerClient
}

func (m *mockAzureServiceClient) NewContainerClient(containerName string) azureContainerClient {
	return m.containerClient
}

type mockAzureContainerClient struct {
	getPropertiesErr error
	createErr        error
	deleteErr        error
}

func (m *mockAzureContainerClient) GetProperties(ctx context.Context, options *container.GetPropertiesOptions) (container.GetPropertiesResponse, error) {
	return container.GetPropertiesResponse{}, m.getPropertiesErr
}

func (m *mockAzureContainerClient) Create(ctx context.Context, options *container.CreateOptions) (container.CreateResponse, error) {
	return container.CreateResponse{}, m.createErr
}

func (m *mockAzureContainerClient) Delete(ctx context.Context, options *container.DeleteOptions) (container.DeleteResponse, error) {
	return container.DeleteResponse{}, m.deleteErr
}

// TestAzureBucketClient_Delete tests the Delete method with various error scenarios
func TestAzureBucketClient_Delete(t *testing.T) {
	tests := []struct {
		name           string
		containerName  string
		deleteError    error
		expectedResult bool
		expectedError  bool
		errorContains  string
	}{
		{
			name:           "successful deletion",
			containerName:  "test-container",
			deleteError:    nil,
			expectedResult: true,
			expectedError:  false,
		},
		{
			name:           "invalid container name",
			containerName:  "INVALID_NAME",
			expectedResult: false,
			expectedError:  true,
			errorContains:  "invalid container name",
		},
		{
			name:          "container not found - idempotent",
			containerName: "test-container",
			deleteError: &azcore.ResponseError{
				StatusCode: 404,
				ErrorCode:  "ContainerNotFound",
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name:          "account not found - idempotent",
			containerName: "test-container",
			deleteError: &azcore.ResponseError{
				StatusCode: 404,
				ErrorCode:  "AccountNotFound",
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name:          "authentication failed",
			containerName: "test-container",
			deleteError: &azcore.ResponseError{
				StatusCode: 401,
				ErrorCode:  "AuthenticationFailed",
			},
			expectedResult: false,
			expectedError:  true,
			errorContains:  "authentication failed",
		},
		{
			name:          "authorization failed",
			containerName: "test-container",
			deleteError: &azcore.ResponseError{
				StatusCode: 403,
				ErrorCode:  "AuthorizationFailed",
			},
			expectedResult: false,
			expectedError:  true,
			errorContains:  "authorization failed",
		},
		{
			name:           "generic error",
			containerName:  "test-container",
			deleteError:    fmt.Errorf("network error"),
			expectedResult: false,
			expectedError:  true,
			errorContains:  "failed to delete container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock container client
			mockContainerClient := &mockAzureContainerClient{
				deleteErr: tt.deleteError,
			}

			// Create mock service client
			mockServiceClient := &mockAzureServiceClient{
				containerClient: mockContainerClient,
			}

			// Create test secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
				},
				Data: map[string][]byte{
					"AZURE_STORAGE_ACCOUNT": []byte("teststorageaccount"),
				},
			}

			// Create mock k8s client
			mockK8sClient := &mockK8sClient{
				secret: secret,
			}

			// Create azureBucketClient with factory
			client := &azureBucketClient{
				bucket: v1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cloudstorage",
						Namespace: "test-namespace",
					},
					Spec: v1alpha1.CloudStorageSpec{
						Name:     tt.containerName,
						Provider: v1alpha1.AzureBucketProvider,
						CreationSecret: corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "test-secret",
							},
							Key: "azurekey",
						},
					},
				},
				client: mockK8sClient,
				clientFactory: func(serviceURL string, credential azcore.TokenCredential, sharedKey *azblob.SharedKeyCredential) (azureServiceClient, error) {
					return mockServiceClient, nil
				},
			}

			// Test Delete method
			result, err := client.Delete()

			// Verify results
			assert.Equal(t, tt.expectedResult, result)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// mockK8sClient is a mock implementation of the Kubernetes client
type mockK8sClient struct {
	client.Client
	secret *corev1.Secret
}

func (m *mockK8sClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if secret, ok := obj.(*corev1.Secret); ok {
		*secret = *m.secret
		return nil
	}
	return fmt.Errorf("object not found")
}

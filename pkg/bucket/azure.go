package bucket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials/stsflow"
)

// azureServiceClient abstracts the Azure blob service client for testing
type azureServiceClient interface {
	NewContainerClient(containerName string) azureContainerClient
}

// azureContainerClient abstracts container operations for testing
type azureContainerClient interface {
	GetProperties(ctx context.Context, options *container.GetPropertiesOptions) (container.GetPropertiesResponse, error)
	Create(ctx context.Context, options *container.CreateOptions) (container.CreateResponse, error)
	Delete(ctx context.Context, options *container.DeleteOptions) (container.DeleteResponse, error)
}

// azureClientFactory creates Azure clients (for dependency injection in tests)
type azureClientFactory func(serviceURL string, credential azcore.TokenCredential, sharedKey *azblob.SharedKeyCredential) (azureServiceClient, error)

// realAzureServiceClient wraps the real Azure SDK client to implement our interface
type realAzureServiceClient struct {
	client *azblob.Client
}

func (r *realAzureServiceClient) NewContainerClient(containerName string) azureContainerClient {
	return &realAzureContainerClient{
		client: r.client.ServiceClient().NewContainerClient(containerName),
	}
}

// realAzureContainerClient wraps the real Azure SDK container client
type realAzureContainerClient struct {
	client *container.Client
}

func (r *realAzureContainerClient) GetProperties(ctx context.Context, options *container.GetPropertiesOptions) (container.GetPropertiesResponse, error) {
	return r.client.GetProperties(ctx, options)
}

func (r *realAzureContainerClient) Create(ctx context.Context, options *container.CreateOptions) (container.CreateResponse, error) {
	return r.client.Create(ctx, options)
}

func (r *realAzureContainerClient) Delete(ctx context.Context, options *container.DeleteOptions) (container.DeleteResponse, error) {
	return r.client.Delete(ctx, options)
}

type azureBucketClient struct {
	bucket        v1alpha1.CloudStorage
	client        client.Client
	clientFactory azureClientFactory // Optional, for testing
}

// Exists checks if the container exists in the storage account
func (a *azureBucketClient) Exists() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	containerName := a.bucket.Spec.Name
	if err := validateContainerName(containerName); err != nil {
		return false, fmt.Errorf("invalid container name: %w", err)
	}

	azureClient, err := a.createAzureClient()
	if err != nil {
		return false, fmt.Errorf("failed to create Azure client: %w", err)
	}

	containerClient := azureClient.NewContainerClient(containerName)

	_, err = containerClient.GetProperties(ctx, nil)
	if err != nil {
		// Use bloberror package for Azure-specific error handling (following Velero pattern)
		if bloberror.HasCode(err, bloberror.ContainerNotFound) {
			return false, nil
		}

		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			switch respErr.ErrorCode {
			case string(bloberror.AuthenticationFailed):
				return false, fmt.Errorf("authentication failed: check credentials")
			case "AuthorizationFailed":
				return false, fmt.Errorf("authorization failed: check permissions")
			case "AccountNotFound":
				return false, fmt.Errorf("storage account not found")
			}
		}
		return false, fmt.Errorf("failed to check container existence: %w", err)
	}

	return true, nil
}

// Create creates a new container with the specified configuration
func (a *azureBucketClient) Create() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	containerName := a.bucket.Spec.Name
	if err := validateContainerName(containerName); err != nil {
		return false, fmt.Errorf("invalid container name: %w", err)
	}

	// Check if container already exists
	exists, err := a.Exists()
	if err != nil {
		return false, fmt.Errorf("failed to check container existence: %w", err)
	}
	if exists {
		return true, nil // Idempotent behavior
	}

	azureClient, err := a.createAzureClient()
	if err != nil {
		return false, fmt.Errorf("failed to create Azure client: %w", err)
	}

	containerClient := azureClient.NewContainerClient(containerName)

	// Create container with private access level (security requirement)
	createOptions := &container.CreateOptions{}

	// Apply tags if specified (convert to metadata format required by Azure)
	if len(a.bucket.Spec.Tags) > 0 {
		if err := a.validateAndConvertTags(); err != nil {
			return false, fmt.Errorf("invalid tags: %w", err)
		}
		// Convert tags to metadata (which requires pointer values)
		metadata := make(map[string]*string)
		for k, v := range a.bucket.Spec.Tags {
			metadata[k] = to.Ptr(v)
		}
		createOptions.Metadata = metadata
	}

	_, err = containerClient.Create(ctx, createOptions)
	if err != nil {
		// Use bloberror package for Azure-specific error handling (following Velero pattern)
		if bloberror.HasCode(err, bloberror.ContainerAlreadyExists) {
			return true, nil // Idempotent behavior
		}

		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			switch respErr.ErrorCode {
			case string(bloberror.AuthenticationFailed):
				return false, fmt.Errorf("authentication failed: check credentials")
			case "AuthorizationFailed":
				return false, fmt.Errorf("authorization failed: check permissions")
			case string(bloberror.InvalidResourceName):
				return false, fmt.Errorf("invalid container name: %s", containerName)
			case "AccountNotFound":
				return false, fmt.Errorf("storage account not found")
			}
		}
		return false, fmt.Errorf("failed to create container: %w", err)
	}

	return true, nil
}

// Delete removes the container (idempotent operation)
func (a *azureBucketClient) Delete() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	containerName := a.bucket.Spec.Name
	if err := validateContainerName(containerName); err != nil {
		return false, fmt.Errorf("invalid container name: %w", err)
	}

	azureClient, err := a.createAzureClient()
	if err != nil {
		return false, fmt.Errorf("failed to create Azure client: %w", err)
	}

	containerClient := azureClient.NewContainerClient(containerName)

	_, err = containerClient.Delete(ctx, nil)
	if err != nil {
		// Use bloberror package for Azure-specific error handling (following Velero pattern)
		if bloberror.HasCode(err, bloberror.ContainerNotFound) {
			return true, nil // Idempotent behavior
		}

		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			switch respErr.ErrorCode {
			case string(bloberror.AuthenticationFailed):
				return false, fmt.Errorf("authentication failed: check credentials")
			case "AuthorizationFailed":
				return false, fmt.Errorf("authorization failed: check permissions")
			case "AccountNotFound":
				// Storage account doesn't exist - treat as successful deletion (idempotent)
				return true, nil
			}
		}
		return false, fmt.Errorf("failed to delete container: %w", err)
	}

	return true, nil
}

// createAzureClient creates an Azure blob service client with appropriate authentication
//
// Unlike AWS and GCP providers which use getCredentialFromCloudStorageSecret to create
// temporary credential files, Azure handles credentials directly without file intermediaries.
//
// This approach is designed to be compatible with Velero's Azure credential handling
// without requiring upstream changes to Velero. Velero expects Azure credentials as:
// https://github.com/vmware-tanzu/velero/blob/main/pkg/util/azure/credential.go
// - Environment variables (for Workload Identity)
// - Direct secret values (for storage keys and service principals)
//
// By handling credentials directly in our Azure implementation, we maintain compatibility
// with Velero's expectations while avoiding the complexity of temporary credential files.
// The Azure SDK supports creating credentials programmatically from values, making the
// file-based approach unnecessary.
//
// The three authentication methods supported are:
// 1. Storage Account Key - uses NewSharedKeyCredential
// 2. Workload Identity (federated tokens) - uses NewWorkloadIdentityCredential
// 3. Service Principal - uses NewClientSecretCredential
func (a *azureBucketClient) createAzureClient() (azureServiceClient, error) {
	secret, err := a.getSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	storageAccountName, err := getStorageAccountName(a.bucket, secret)
	if err != nil {
		return nil, err
	}

	if err := validateStorageAccountName(storageAccountName); err != nil {
		return nil, fmt.Errorf("invalid storage account name: %w", err)
	}

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", storageAccountName)

	// Use factory if provided (for testing)
	if a.clientFactory != nil {
		// Try storage account key authentication first (higher priority)
		if accessKey, ok := secret.Data["AZURE_STORAGE_ACCOUNT_ACCESS_KEY"]; ok && len(accessKey) > 0 {
			credential, err := azblob.NewSharedKeyCredential(storageAccountName, string(accessKey))
			if err != nil {
				return nil, fmt.Errorf("failed to create shared key credential: %w", err)
			}
			return a.clientFactory(serviceURL, nil, credential)
		}

		// For other auth methods, we'll pass the token credential
		var tokenCred azcore.TokenCredential
		var err error

		if a.hasWorkloadIdentityCredentials(secret) {
			tokenCred, err = a.createWorkloadIdentityCredential(secret)
		} else if a.hasServicePrincipalCredentials(secret) {
			tokenCred, err = a.createServicePrincipalCredential(secret)
		} else {
			tokenCred, err = azidentity.NewDefaultAzureCredential(nil)
		}

		if err != nil {
			return nil, err
		}

		return a.clientFactory(serviceURL, tokenCred, nil)
	}

	// Default implementation (production code)
	// Try storage account key authentication first (higher priority)
	if accessKey, ok := secret.Data["AZURE_STORAGE_ACCOUNT_ACCESS_KEY"]; ok && len(accessKey) > 0 {
		credential, err := azblob.NewSharedKeyCredential(storageAccountName, string(accessKey))
		if err != nil {
			return nil, fmt.Errorf("failed to create shared key credential: %w", err)
		}

		azClient, err := azblob.NewClientWithSharedKeyCredential(serviceURL, credential, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create Azure client with shared key: %w", err)
		}

		return &realAzureServiceClient{client: azClient}, nil
	}

	// Check if Azure Workload Identity is configured (federated tokens)
	if a.hasWorkloadIdentityCredentials(secret) {
		credential, err := a.createWorkloadIdentityCredential(secret)
		if err != nil {
			return nil, fmt.Errorf("failed to create workload identity credential: %w", err)
		}

		azClient, err := azblob.NewClient(serviceURL, credential, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create Azure client with workload identity: %w", err)
		}

		return &realAzureServiceClient{client: azClient}, nil
	}

	// Fall back to service principal authentication
	if a.hasServicePrincipalCredentials(secret) {
		credential, err := a.createServicePrincipalCredential(secret)
		if err != nil {
			return nil, fmt.Errorf("failed to create service principal credential: %w", err)
		}

		azClient, err := azblob.NewClient(serviceURL, credential, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create Azure client with service principal: %w", err)
		}

		return &realAzureServiceClient{client: azClient}, nil
	}

	// Try DefaultAzureCredential as last resort (supports various auth methods including managed identity)
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create default Azure credential: %w", err)
	}

	azClient, err := azblob.NewClient(serviceURL, credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure client with default credential: %w", err)
	}

	return &realAzureServiceClient{client: azClient}, nil
}

// hasWorkloadIdentityCredentials checks if the secret contains workload identity credentials
func (a *azureBucketClient) hasWorkloadIdentityCredentials(secret *corev1.Secret) bool {
	// Check if this is an STS-type secret created by OADP operator
	if labels, ok := secret.Labels[stsflow.STSSecretLabelKey]; ok && labels == stsflow.STSSecretLabelValue {
		// For Azure STS secrets, check if it has the azurekey field
		if azureKey, ok := secret.Data["azurekey"]; ok && len(azureKey) > 0 {
			// Parse the azurekey to ensure it has the required fields
			keyStr := string(azureKey)
			return strings.Contains(keyStr, "AZURE_CLIENT_ID") &&
				strings.Contains(keyStr, "AZURE_TENANT_ID") &&
				strings.Contains(keyStr, "AZURE_SUBSCRIPTION_ID")
		}
	}

	// Check if AZURE_FEDERATED_TOKEN_FILE is in secret data
	if _, ok := secret.Data["AZURE_FEDERATED_TOKEN_FILE"]; ok {
		// Check if required fields are in the secret
		requiredFields := []string{"AZURE_TENANT_ID", "AZURE_CLIENT_ID"}
		for _, field := range requiredFields {
			if _, ok := secret.Data[field]; !ok {
				return false
			}
		}
		return true
	}

	// Also check for environment variable (for backward compatibility)
	if os.Getenv("AZURE_FEDERATED_TOKEN_FILE") != "" {
		// Check if required fields are in the secret
		requiredFields := []string{"AZURE_TENANT_ID", "AZURE_CLIENT_ID"}
		for _, field := range requiredFields {
			if _, ok := secret.Data[field]; !ok {
				return false
			}
		}
		return true
	}
	return false
}

// createWorkloadIdentityCredential creates a workload identity credential for federated tokens
func (a *azureBucketClient) createWorkloadIdentityCredential(secret *corev1.Secret) (azcore.TokenCredential, error) {
	var tenantID, clientID, tokenFile string

	// First try to parse from azurekey field (STS secret format)
	if azureKey, ok := secret.Data["azurekey"]; ok && len(azureKey) > 0 {
		// Parse the environment variable format
		keyStr := string(azureKey)
		lines := strings.Split(keyStr, "\n")
		envVars := make(map[string]string)

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				envVars[parts[0]] = parts[1]
			}
		}

		tenantID = envVars["AZURE_TENANT_ID"]
		clientID = envVars["AZURE_CLIENT_ID"]

		if tenantID == "" || clientID == "" {
			return nil, fmt.Errorf("missing AZURE_TENANT_ID or AZURE_CLIENT_ID in azurekey")
		}
	} else {
		// Fall back to individual fields
		tenantID = string(secret.Data["AZURE_TENANT_ID"])
		clientID = string(secret.Data["AZURE_CLIENT_ID"])
	}

	// Check for federated token file in order of priority
	// 1. From secret data
	if tokenFileData, ok := secret.Data["AZURE_FEDERATED_TOKEN_FILE"]; ok && len(tokenFileData) > 0 {
		tokenFile = string(tokenFileData)
	} else if envTokenFile := os.Getenv("AZURE_FEDERATED_TOKEN_FILE"); envTokenFile != "" {
		// 2. From environment variable
		tokenFile = envTokenFile
	} else {
		// 3. Default for OpenShift environments
		tokenFile = "/var/run/secrets/openshift/serviceaccount/token"
	}

	// WorkloadIdentityCredential handles federated tokens automatically
	options := &azidentity.WorkloadIdentityCredentialOptions{
		ClientID:      clientID,
		TenantID:      tenantID,
		TokenFilePath: tokenFile,
	}

	credential, err := azidentity.NewWorkloadIdentityCredential(options)
	if err != nil {
		return nil, fmt.Errorf("failed to create workload identity credential: %w", err)
	}

	return credential, nil
}

// hasServicePrincipalCredentials checks if the secret contains service principal credentials
func (a *azureBucketClient) hasServicePrincipalCredentials(secret *corev1.Secret) bool {
	requiredFields := []string{"AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET"}
	for _, field := range requiredFields {
		if _, ok := secret.Data[field]; !ok {
			return false
		}
	}
	return true
}

// createServicePrincipalCredential creates a service principal credential
func (a *azureBucketClient) createServicePrincipalCredential(secret *corev1.Secret) (azcore.TokenCredential, error) {
	tenantID := string(secret.Data["AZURE_TENANT_ID"])
	clientID := string(secret.Data["AZURE_CLIENT_ID"])
	clientSecret := string(secret.Data["AZURE_CLIENT_SECRET"])

	credential, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create client secret credential: %w", err)
	}

	return credential, nil
}

// validateAndConvertTags validates tags meet Azure requirements
func (a *azureBucketClient) validateAndConvertTags() error {
	if len(a.bucket.Spec.Tags) > 50 {
		return fmt.Errorf("too many tags: Azure supports maximum 50 tags per resource")
	}

	for key, value := range a.bucket.Spec.Tags {
		if len(key) == 0 || len(key) > 512 {
			return fmt.Errorf("tag name '%s' must be between 1 and 512 characters", key)
		}
		if len(value) > 256 {
			return fmt.Errorf("tag value for '%s' must be 256 characters or less", key)
		}
	}

	return nil
}

// getSecret retrieves the secret referenced in the CloudStorage spec
func (a *azureBucketClient) getSecret() (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Namespace: a.bucket.Namespace,
		Name:      a.bucket.Spec.CreationSecret.Name,
	}

	if err := a.client.Get(context.Background(), secretKey, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w",
			a.bucket.Namespace, a.bucket.Spec.CreationSecret.Name, err)
	}

	return secret, nil
}

// validateContainerName validates Azure container naming rules
func validateContainerName(name string) error {
	if len(name) < 3 || len(name) > 63 {
		return fmt.Errorf("container name must be between 3 and 63 characters")
	}

	// Must start with letter or number
	if !regexp.MustCompile(`^[a-z0-9]`).MatchString(name) {
		return fmt.Errorf("container name must start with a letter or number")
	}

	// Can only contain lowercase letters, numbers, and hyphens
	if !regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(name) {
		return fmt.Errorf("container name can only contain lowercase letters, numbers, and hyphens")
	}

	// Cannot have consecutive hyphens
	if strings.Contains(name, "--") {
		return fmt.Errorf("container name cannot contain consecutive hyphens")
	}

	return nil
}

// validateStorageAccountName validates Azure storage account naming rules
func validateStorageAccountName(name string) error {
	if len(name) < 3 || len(name) > 24 {
		return fmt.Errorf("storage account name must be between 3 and 24 characters")
	}

	// Can only contain lowercase letters and numbers
	if !regexp.MustCompile(`^[a-z0-9]+$`).MatchString(name) {
		return fmt.Errorf("storage account name can only contain lowercase letters and numbers")
	}

	return nil
}

func getStorageAccountName(bucket v1alpha1.CloudStorage, secret *corev1.Secret) (string, error) {
	// Priority 1: From secret - check standard field first
	if accountName, ok := secret.Data["AZURE_STORAGE_ACCOUNT"]; ok && len(accountName) > 0 {
		return string(accountName), nil
	}

	// Priority 2: Parse from azurekey field (STS secret format)
	if azureKey, ok := secret.Data["azurekey"]; ok && len(azureKey) > 0 {
		// Parse the environment variable format
		keyStr := string(azureKey)
		lines := strings.Split(keyStr, "\n")

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "AZURE_STORAGE_ACCOUNT=") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 && parts[1] != "" {
					return parts[1], nil
				}
			}
		}
	}

	// Priority 3: From CloudStorage config
	if bucket.Spec.Config != nil {
		if accountName, ok := bucket.Spec.Config["storageAccount"]; ok && accountName != "" {
			return accountName, nil
		}
	}

	return "", fmt.Errorf("storage account name not found in secret (AZURE_STORAGE_ACCOUNT), azurekey, or config")
}

// isRetryableError determines if an Azure error should be retried
// Note: Some error codes like "AuthorizationFailed", "AccountNotFound", and "ServiceUnavailable"
// are not defined in the bloberror package constants but are actual error codes returned by Azure
func isRetryableError(err error) bool {
	// Use bloberror package for specific Azure Storage errors (following Velero pattern)
	if bloberror.HasCode(err, bloberror.ContainerNotFound) ||
		bloberror.HasCode(err, bloberror.ContainerAlreadyExists) {
		return false // Don't retry these specific storage errors
	}

	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		switch respErr.StatusCode {
		case http.StatusTooManyRequests: // Too Many Requests
			return true
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout: // Server errors
			return true
		case http.StatusUnauthorized, http.StatusForbidden: // Authentication/Authorization failures
			return false
		case http.StatusBadRequest: // Bad Request (invalid names, etc.)
			return false
		case http.StatusNotFound: // Not Found
			return false
		}

		// Check specific error codes
		switch respErr.ErrorCode {
		case string(bloberror.InternalError), string(bloberror.ServerBusy):
			return true
		case string(bloberror.AuthenticationFailed):
			return false
		case string(bloberror.InvalidResourceName):
			return false
		// Note: "ServiceUnavailable", "AuthorizationFailed", and "AccountNotFound"
		// are not defined in bloberror constants, keeping as strings
		case "ServiceUnavailable", "AuthorizationFailed", "AccountNotFound":
			return false
		}
	}

	// Network-related errors are typically retryable
	if strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "connection") ||
		strings.Contains(err.Error(), "network") {
		return true
	}

	return false
}

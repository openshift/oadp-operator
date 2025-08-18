package cloudprovider

import (
	"testing"
)

func TestParseAzureCredentials_CloudKeyFormat(t *testing.T) {
	// Test Velero BSL secret format (single 'cloud' key with env vars)
	cloudContent := `AZURE_SUBSCRIPTION_ID=test-subscription-id
AZURE_TENANT_ID=test-tenant-id
AZURE_CLIENT_ID=test-client-id
AZURE_CLIENT_SECRET=test-client-secret
AZURE_RESOURCE_GROUP=test-rg
AZURE_STORAGE_ACCOUNT_ID=teststorageaccount`

	data := map[string][]byte{
		"cloud": []byte(cloudContent),
	}

	creds := ParseAzureCredentials(data)

	if creds.SubscriptionID != "test-subscription-id" {
		t.Errorf("Expected SubscriptionID to be 'test-subscription-id', got '%s'", creds.SubscriptionID)
	}
	if creds.TenantID != "test-tenant-id" {
		t.Errorf("Expected TenantID to be 'test-tenant-id', got '%s'", creds.TenantID)
	}
	if creds.ClientID != "test-client-id" {
		t.Errorf("Expected ClientID to be 'test-client-id', got '%s'", creds.ClientID)
	}
	if creds.ClientSecret != "test-client-secret" {
		t.Errorf("Expected ClientSecret to be 'test-client-secret', got '%s'", creds.ClientSecret)
	}
	if creds.ResourceGroupName != "test-rg" {
		t.Errorf("Expected ResourceGroupName to be 'test-rg', got '%s'", creds.ResourceGroupName)
	}
	if creds.StorageAccountName != "teststorageaccount" {
		t.Errorf("Expected StorageAccountName to be 'teststorageaccount', got '%s'", creds.StorageAccountName)
	}
}

func TestParseAzureCredentials_IndividualKeysFormat(t *testing.T) {
	// Test individual keys format (existing DPT format)
	data := map[string][]byte{
		"AZURE_SUBSCRIPTION_ID":    []byte("test-subscription-id"),
		"AZURE_TENANT_ID":          []byte("test-tenant-id"),
		"AZURE_CLIENT_ID":          []byte("test-client-id"),
		"AZURE_CLIENT_SECRET":      []byte("test-client-secret"),
		"AZURE_RESOURCE_GROUP":     []byte("test-rg"),
		"AZURE_STORAGE_ACCOUNT_ID": []byte("teststorageaccount"),
	}

	creds := ParseAzureCredentials(data)

	if creds.SubscriptionID != "test-subscription-id" {
		t.Errorf("Expected SubscriptionID to be 'test-subscription-id', got '%s'", creds.SubscriptionID)
	}
	if creds.TenantID != "test-tenant-id" {
		t.Errorf("Expected TenantID to be 'test-tenant-id', got '%s'", creds.TenantID)
	}
	if creds.ClientID != "test-client-id" {
		t.Errorf("Expected ClientID to be 'test-client-id', got '%s'", creds.ClientID)
	}
	if creds.ClientSecret != "test-client-secret" {
		t.Errorf("Expected ClientSecret to be 'test-client-secret', got '%s'", creds.ClientSecret)
	}
	if creds.ResourceGroupName != "test-rg" {
		t.Errorf("Expected ResourceGroupName to be 'test-rg', got '%s'", creds.ResourceGroupName)
	}
	if creds.StorageAccountName != "teststorageaccount" {
		t.Errorf("Expected StorageAccountName to be 'teststorageaccount', got '%s'", creds.StorageAccountName)
	}
}

func TestParseAzureCredentials_EmptyCloudKey(t *testing.T) {
	// Test with empty cloud key should fall back to individual keys
	data := map[string][]byte{
		"cloud":                 []byte(""),
		"AZURE_SUBSCRIPTION_ID": []byte("test-subscription"),
		"AZURE_TENANT_ID":       []byte("test-tenant"),
	}

	creds := ParseAzureCredentials(data)

	if creds.SubscriptionID != "test-subscription" {
		t.Errorf("Expected SubscriptionID to be 'test-subscription', got '%s'", creds.SubscriptionID)
	}
	if creds.TenantID != "test-tenant" {
		t.Errorf("Expected TenantID to be 'test-tenant', got '%s'", creds.TenantID)
	}
}

func TestParseAzureCredentials_StorageAccountKeyFormat(t *testing.T) {
	// Test Velero BSL secret format with storage account key authentication
	cloudContent := `AZURE_SUBSCRIPTION_ID=test-subscription-id
AZURE_TENANT_ID=test-tenant-id
AZURE_CLIENT_ID=test-client-id
AZURE_CLIENT_SECRET=test-client-secret
AZURE_RESOURCE_GROUP=test-rg
AZURE_STORAGE_ACCOUNT_ID=teststorageaccount
AZURE_STORAGE_ACCOUNT_ACCESS_KEY=test-storage-key`

	data := map[string][]byte{
		"cloud": []byte(cloudContent),
	}

	creds := ParseAzureCredentials(data)

	// Verify all fields are parsed correctly for storage account key auth
	if creds.SubscriptionID != "test-subscription-id" {
		t.Errorf("Expected SubscriptionID to be 'test-subscription-id', got '%s'", creds.SubscriptionID)
	}
	if creds.TenantID != "test-tenant-id" {
		t.Errorf("Expected TenantID to be 'test-tenant-id', got '%s'", creds.TenantID)
	}
	if creds.ClientID != "test-client-id" {
		t.Errorf("Expected ClientID to be 'test-client-id', got '%s'", creds.ClientID)
	}
	if creds.ClientSecret != "test-client-secret" {
		t.Errorf("Expected ClientSecret to be 'test-client-secret', got '%s'", creds.ClientSecret)
	}
	if creds.ResourceGroupName != "test-rg" {
		t.Errorf("Expected ResourceGroupName to be 'test-rg', got '%s'", creds.ResourceGroupName)
	}
	if creds.StorageAccountName != "teststorageaccount" {
		t.Errorf("Expected StorageAccountName to be 'teststorageaccount', got '%s'", creds.StorageAccountName)
	}
	if creds.StorageAccountKey != "test-storage-key" {
		t.Errorf("Expected StorageAccountKey to be 'test-storage-key', got '%s'", creds.StorageAccountKey)
	}
}

func TestParseCloudCredentials(t *testing.T) {
	cloudData := `AZURE_SUBSCRIPTION_ID=test-sub
AZURE_TENANT_ID=test-tenant

# This is a comment
AZURE_CLIENT_ID=test-client
INVALID_LINE_WITHOUT_EQUALS
AZURE_CLIENT_SECRET=test-secret`

	envVars := parseCloudCredentials(cloudData)

	expected := map[string]string{
		"AZURE_SUBSCRIPTION_ID": "test-sub",
		"AZURE_TENANT_ID":       "test-tenant",
		"AZURE_CLIENT_ID":       "test-client",
		"AZURE_CLIENT_SECRET":   "test-secret",
	}

	for key, expectedValue := range expected {
		if value, exists := envVars[key]; !exists {
			t.Errorf("Expected key '%s' to exist", key)
		} else if value != expectedValue {
			t.Errorf("Expected %s to be '%s', got '%s'", key, expectedValue, value)
		}
	}

	// Should not include invalid lines
	if _, exists := envVars["INVALID_LINE_WITHOUT_EQUALS"]; exists {
		t.Error("Should not include invalid lines without equals sign")
	}
}

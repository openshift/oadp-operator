package cloudprovider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/go-logr/logr"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/utils"
)

const (
	maxTestSizeBytesAzure = 200 * 1024 * 1024
)

type AzureCredentials struct {
	SubscriptionID     string
	TenantID           string
	ClientID           string
	ClientSecret       string
	ResourceGroupName  string
	StorageAccountName string
	StorageAccountKey  string
	CertificatePath    string
}

type AzureProvider struct {
	creds  AzureCredentials
	client *azblob.Client
}

func ParseAzureCredentials(data map[string][]byte) AzureCredentials {
	creds := AzureCredentials{}
	creds.SubscriptionID = string(data["AZURE_SUBSCRIPTION_ID"])
	creds.TenantID = string(data["AZURE_TENANT_ID"])
	creds.ClientID = string(data["AZURE_CLIENT_ID"])
	creds.ClientSecret = string(data["AZURE_CLIENT_SECRET"])
	creds.ResourceGroupName = string(data["AZURE_RESOURCE_GROUP"])
	creds.StorageAccountName = string(data["AZURE_STORAGE_ACCOUNT_ID"])
	creds.StorageAccountKey = string(data["AZURE_STORAGE_ACCOUNT_ACCESS_KEY"])
	creds.CertificatePath = string(data["AZURE_CLIENT_CERTIFICATE_PATH"])
	return creds
}

func NewAzureProvider(creds AzureCredentials) (*AzureProvider, error) {
	var err error
	var client *azblob.Client
	var tokenCred azcore.TokenCredential

	if creds.StorageAccountKey != "" {
		sharedKeyCred, err := azblob.NewSharedKeyCredential(creds.StorageAccountName, creds.StorageAccountKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create shared key credential: %w", err)
		}
		serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", creds.StorageAccountName)
		client, err = azblob.NewClientWithSharedKeyCredential(serviceURL, sharedKeyCred, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create azure client: %w", err)
		}
	} else if creds.ClientSecret != "" {
		tokenCred, err = azidentity.NewClientSecretCredential(creds.TenantID, creds.ClientID, creds.ClientSecret, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create client secret credential: %w", err)
		}
	} else if creds.CertificatePath != "" {
		certData, err := os.ReadFile(creds.CertificatePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read certificate file: %w", err)
		}
		certs, key, err := azidentity.ParseCertificates(certData, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to parse certificates: %w", err)
		}
		tokenCred, err = azidentity.NewClientCertificateCredential(creds.TenantID, creds.ClientID, certs, key, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create client certificate credential: %w", err)
		}
	} else {
		tokenCred, err = azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create default azure credential: %w", err)
		}
	}

	if client == nil {
		serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", creds.StorageAccountName)
		client, err = azblob.NewClient(serviceURL, tokenCred, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create azure client: %w", err)
		}
	}

	return &AzureProvider{
		creds:  creds,
		client: client,
	}, nil
}

func (a *AzureProvider) UploadTest(ctx context.Context, config oadpv1alpha1.UploadSpeedTestConfig, bucket string, log logr.Logger) (int64, time.Duration, error) {
	log.Info("Starting upload speed test", "fileSize", config.FileSize, "timeout", config.Timeout.Duration.String())

	testDataBytes, err := utils.ParseFileSize(config.FileSize)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid file size: %w", err)
	}

	if testDataBytes > maxTestSizeBytesAzure {
		return 0, 0, fmt.Errorf("test file size %d exceeds max allowed %dMB (due to pod mem limit)", testDataBytes, maxTestSizeBytesAzure/1024/1024)
	}

	timeoutDuration := 30 * time.Second
	if config.Timeout.Duration != 0 {
		timeoutDuration = config.Timeout.Duration
	}

	log.Info("Generating test payload for upload", "bytes", testDataBytes)
	payload := bytes.Repeat([]byte("0"), int(testDataBytes))
	key := fmt.Sprintf("dpt-upload-test-%d", time.Now().UnixNano())
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()

	log.Info("Uploading to bucket...")
	start := time.Now()

	_, err = a.client.UploadBuffer(ctxWithTimeout, bucket, key, payload, &azblob.UploadBufferOptions{})

	duration := time.Since(start)

	if err != nil {
		return 0, duration, fmt.Errorf("upload failed: %w", err)
	}

	speedMbps := (float64(testDataBytes*8) / duration.Seconds()) / 1_000_000
	log.Info("Upload completed", "duration", duration.String(), "speedMbps", speedMbps)

	return int64(speedMbps), duration, nil
}

func (a *AzureProvider) IsStorageAccountKeyAuth() bool {
	return a.creds.StorageAccountKey != ""
}

//nolint:unparam // The bucket parameter is unused because in Azure, versioning and encryption are properties of the storage account, not the container.
func (a *AzureProvider) GetBucketMetadata(ctx context.Context, bucket string, log logr.Logger) (*oadpv1alpha1.BucketMetadata, error) {
	var err error
	var tokenCred azcore.TokenCredential

	// Choose the correct auth method for ARM APIs
	if a.creds.StorageAccountKey != "" {
		tokenCred, err = azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create default azure credential: %w", err)
		}
	} else if a.creds.ClientSecret != "" {
		tokenCred, err = azidentity.NewClientSecretCredential(a.creds.TenantID, a.creds.ClientID, a.creds.ClientSecret, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create client secret credential: %w", err)
		}
	} else if a.creds.CertificatePath != "" {
		certData, err := os.ReadFile(a.creds.CertificatePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read certificate file: %w", err)
		}
		certs, key, err := azidentity.ParseCertificates(certData, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to parse certificates: %w", err)
		}
		tokenCred, err = azidentity.NewClientCertificateCredential(a.creds.TenantID, a.creds.ClientID, certs, key, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create client certificate credential: %w", err)
		}
	} else {
		tokenCred, err = azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create default azure credential: %w", err)
		}
	}

	result := &oadpv1alpha1.BucketMetadata{}

	// Get versioning status using BlobServicesClient from the ARM SDK
	blobSvcClient, err := armstorage.NewBlobServicesClient(a.creds.SubscriptionID, tokenCred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create blob services client: %w", err)
	}

	blobSvcResp, err := blobSvcClient.GetServiceProperties(ctx, a.creds.ResourceGroupName, a.creds.StorageAccountName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get blob service properties: %w", err)
	}

	props := blobSvcResp.BlobServiceProperties.BlobServiceProperties
	if props != nil && props.IsVersioningEnabled != nil && *props.IsVersioningEnabled {
		result.VersioningStatus = "Enabled"
	} else {
		result.VersioningStatus = "Disabled"
	}

	// Encryption details via AccountsClient
	accountsClient, err := armstorage.NewAccountsClient(a.creds.SubscriptionID, tokenCred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create accounts client: %w", err)
	}

	accountProps, err := accountsClient.GetProperties(ctx, a.creds.ResourceGroupName, a.creds.StorageAccountName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage account properties: %w", err)
	}

	if accountProps.Account.Properties != nil &&
		accountProps.Account.Properties.Encryption != nil &&
		accountProps.Account.Properties.Encryption.KeySource != nil {
		result.EncryptionAlgorithm = string(*accountProps.Account.Properties.Encryption.KeySource)
	} else {
		result.EncryptionAlgorithm = "Unknown"
	}

	return result, nil
}

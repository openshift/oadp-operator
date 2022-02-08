package bucket

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/openshift/oadp-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type azureConfigMap map[azureConfigKey]string
type azureConfigKey string

// Config Keys for Azure
const (
	resourceGroup           azureConfigKey = "resourceGroup"
	storageAccount          azureConfigKey = "storageAccount"
	storageAccountKeyEnvVar azureConfigKey = "storageAccountKeyEnvVar"
	subscriptionId          azureConfigKey = "subscriptionId"
	blockSizeInBytes        azureConfigKey = "blockSizeInBytes"
)

type azureBucketClient struct {
	bucket v1alpha1.CloudStorage
	client client.Client
}

func (a azureBucketClient) Exists() (bool, error) {
	panic("implement me")
	return true, nil
}

func (a azureBucketClient) Create() (bool, error) {
	panic("implement me")
	return true, nil
}

func (a azureBucketClient) ForceCredentialRefresh() error {
	panic("implement me")
	return nil
}

func (a azureBucketClient) Delete() (bool, error) {
	panic("implement me")
	return true, nil
}

// https://github.com/Azure-Samples/azure-sdk-for-go-samples/blob/f5b23158ccd57bad9340fc6d6ef4c03aee210d83/storage/container.go#L17-L31
var (
	blobFormatString = `https://%s.blob.core.windows.net`
)

func (a azureBucketClient) getContainerClient(accountName string, accountKey string) azblob.ContainerClient {
	azcred, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		panic(err)
	}
	sc, err := azblob.NewServiceClientWithSharedKey(fmt.Sprintf(blobFormatString, accountName), azcred, &azblob.ClientOptions{})
	if err != nil {
		panic(err)
	}
	return sc.NewContainerClient(a.bucket.Name)
}

// func (a azureBucketClient) getServiceClient() (azblob.ServiceClient, error) {
// 	// cred, err := getCredentialFromSecret(a)
// 	// if err != nil {
// 	// 	return nil, err
// 	// }
// 	// azcred, err := azblob.NewSharedKeyCredential(accountName, accountKey)
// 	// if err != nil {
// 	// 	panic(err)
// 	// }
// 	panic("implement me")
// 	// return azblob.NewServiceClientWithSharedKey(fmt.Sprintf(blobFormatString, a.bucket.Spec.CreationSecret), "azcred", &azblob.ClientOptions{})
// }

func (a azureBucketClient) getClient() client.Client {
	return a.client
}

func (a azureBucketClient) getCloudStorage() v1alpha1.CloudStorage {
	return a.bucket
}

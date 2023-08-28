package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CloudStorage types are APIs for automatic bucket creation at cloud providers if defined name do not exists.

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

type CloudStorage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudStorageSpec   `json:"spec,omitempty"`
	Status CloudStorageStatus `json:"status,omitempty"`
}

type CloudStorageProvider string

const (
	AWSBucketProvider   CloudStorageProvider = CloudStorageProvider(DefaultPluginAWS)
	AzureBucketProvider CloudStorageProvider = CloudStorageProvider(DefaultPluginMicrosoftAzure)
	GCPBucketProvider   CloudStorageProvider = CloudStorageProvider(DefaultPluginGCP)
)

type CloudStorageSpec struct {
	// name is the name requested for the bucket (aws, gcp) or container (azure)
	Name string `json:"name"`
	// creationSecret is the secret that is needed to be used while creating the bucket.
	CreationSecret corev1.SecretKeySelector `json:"creationSecret"`
	// enableSharedConfig enable the use of shared config loading for AWS Buckets
	EnableSharedConfig *bool `json:"enableSharedConfig,omitempty"`
	// tags for the bucket
	// +kubebuilder:validation:Optional
	Tags map[string]string `json:"tags,omitempty"`
	// region for the bucket to be in, will be us-east-1 if not set.
	Region string `json:"region,omitempty"`
	// provider is the provider of the cloud storage
	// +kubebuilder:validation:Enum=aws
	Provider CloudStorageProvider `json:"provider"`

	// https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/storage/azblob@v0.2.0#section-readme
	// azure blob primary endpoint
	// az storage account show -g <resource-group> -n <storage-account>
	// need storage account name and key to create azure container
	// az storage container create -n <container-name> --account-name <storage-account-name> --account-key <storage-account-key>
	// azure account key will use CreationSecret to store key and account name

}

type CloudStorageStatus struct {
	Name       string       `json:"name"`
	LastSynced *metav1.Time `json:"lastSyncTimestamp,omitempty"`
}

//+kubebuilder:object:root=true

type CloudStorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudStorage `json:"items"`
}

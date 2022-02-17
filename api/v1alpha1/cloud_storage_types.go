package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	AWSBucketProvider   CloudStorageProvider = "aws"
	AzureBucketProvider CloudStorageProvider = "azure"
	GCPBucketProvider   CloudStorageProvider = "gcp"
)

type CloudStorageSpec struct {
	// Name is the name requested for the bucket (aws, gcp) or container (azure)
	Name string `json:"name"`
	// CreationSecret is the secret that is needed to be used while creating the bucket.
	CreationSecret corev1.SecretKeySelector `json:"creationSecret"`
	// EnableSharedConfig enable the use of shared config loading for AWS Buckets
	EnableSharedConfig *bool `json:"enableSharedConfig,omitempty"`
	// Tags for the bucket
	// - For GCP, this will be used for labels
	// +kubebuilder:validation:Optional
	Tags map[string]string `json:"tags,omitempty"`
	// Region for the bucket to be in,
	// - AWS, will be us-east-1 if not set.
	// - GCP, will be US if not set.
	Region string `json:"region,omitempty"`
	// Required for GCP: ProjectID for the bucket to be in.
	// If not set, a best attempt will be made to retrieve Project ID from secret JSON.
	ProjectID string `json:"projectID,omitempty"`
	// +kubebuilder:validation:Enum=aws;gcp
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

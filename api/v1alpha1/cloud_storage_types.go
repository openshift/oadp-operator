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

type BucketProvider string

const (
	AWSBucketProvider BucketProvider = "AWS"
)

type CloudStorageSpec struct {
	// Name is the name requested for the bucket
	Name string `json:"name"`
	// CreationSecret is the secret that is needed to be used while creating the bucket.
	CreationSecret corev1.SecretKeySelector `json:"creationSecret"`
	// EnableSharedConfig enable the use of shared config loading for AWS Buckets
	EnableSharedConfig *bool `json:"enableSharedConfig,omitempty"`
	// Tags for the bucket
	// +kubebuilder:validation:Optional
	Tags map[string]string `json:"tags,omitempty"`
	// Region for the bucket to be in, will be us-east-1 if not set.
	Region string `json:"region,omitempty"`
	// +kubebuilder:validation:Enum=AWS
	Provider BucketProvider `json:"provider"`
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

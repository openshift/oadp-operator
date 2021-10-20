package v1alpha1

import (
	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true

type Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BucketSpec   `json:"spec,omitempty"`
	Status BucketStatus `json:"status,omitempty"`
}

type BucketSpec struct {
	Name           string                      `json:"name"`
	CreationSecret corev1api.SecretKeySelector `json:"bucketCreationSecret,omitempty"`
	Tags           map[string]string           `json:"bucketTags,omitempty"`
	Region         string                      `json:"region"`
}

type BucketStatus struct {
	Name              string       `json:"name"`
	LastSyncTimestamp *metav1.Time `json:"lastSyncTimestamp,omitempty"`
}

//+kubebuilder:object:root=true

type BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bucket `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Bucket{}, &BucketList{})
}

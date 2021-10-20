package bucket

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/openshift/oadp-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	fileBucketCache = map[types.NamespacedName]string{}
)

func init() {
	fileBucketCache = make(map[types.NamespacedName]string)
}

type Client interface {
	Exists() (bool, error)
	Create() (bool, error)
	Name() (string, error)
	ForceCredentialRefresh() error
}

func NewClient(b v1alpha1.Bucket, c client.Client) (Client, error) {
	return &awsBucketClient{bucket: b, client: c}, nil
}

type awsBucketClient struct {
	bucket v1alpha1.Bucket
	client client.Client
}

var _ Client = &awsBucketClient{}

func (a awsBucketClient) Exists() (bool, error) {
	return false, nil
}

func (a awsBucketClient) Create() (bool, error) {
	return false, nil
}

func (a awsBucketClient) Name() (string, error) {
	return "", nil
}

func (a awsBucketClient) getCredentialFromSecret() (*credentials.Credentials, error) {
	// Look for file in tmp based on name.
	// TODO: handle force credential refesh
	secret := &corev1.Secret{}
	err := a.client.Get(context.TODO(),
		types.NamespacedName{
			Name:      a.bucket.Spec.CreationSecret.Name,
			Namespace: a.bucket.Namespace,
		},
		secret)
	if err != nil {
		return nil, err
	}

	// cred := secret.Data[a.bucket.Spec.CreationSecret.Key]
	//create a tmp file based on the bucket name, if it does not exist
	// os.CreateTemp()
	return nil, nil

}

func (a awsBucketClient) ForceCredentialRefresh() error {
	return nil
}

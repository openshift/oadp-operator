package bucket

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openshift/oadp-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
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
	Delete() (bool, error)
	ForceCredentialRefresh() error
}

func NewClient(b v1alpha1.CloudStorage, c client.Client) (Client, error) {
	switch b.Spec.Provider {
	case v1alpha1.AWSBucketProvider:
		return &awsBucketClient{bucket: b, client: c}, nil
	case v1alpha1.GCPBucketProvider:
		return &gcpBucketClient{bucket: b, client: c}, nil
	default:
		return nil, fmt.Errorf("unable to determine bucket client")
	}
}

func getCredentialFromCloudStorageSecretAsFilename(a client.Client, cloudStorage v1alpha1.CloudStorage) (string, error) {
	var filename string
	var ok bool
	cloudStorageNamespacedName := types.NamespacedName{
		Name:      cloudStorage.Name,
		Namespace: cloudStorage.Namespace,
	}
	if filename, ok = fileBucketCache[cloudStorageNamespacedName]; !ok {
		// Look for file in tmp based on name.
		// TODO: handle force credential refesh

		// cred := secret.Data[cloudStorage.Spec.CreationSecret.Key]
		cred, err := getCredentialFromCloudStorageSecret(a, cloudStorage)
		if err != nil {
			return "", err
		}
		//create a tmp file based on the bucket name, if it does not exist
		dir, err := os.MkdirTemp("", fmt.Sprintf("secret-%v-%v", cloudStorage.Namespace, cloudStorage.Name))
		if err != nil {
			return "", err
		}
		f, err := os.CreateTemp(dir, "cloudstoragesecret")
		if err != nil {
			return "", err
		}
		defer f.Close()
		f.Write(cred)
		filename = filepath.Join(f.Name())
		fileBucketCache[cloudStorageNamespacedName] = filename
	}

	return filename, nil
}

func getCredentialFromCloudStorageSecret(a client.Client, cloudStorage v1alpha1.CloudStorage) ([]byte, error) {
	secret := &corev1.Secret{}
	err := a.Get(context.TODO(), types.NamespacedName{
		Name:      cloudStorage.Spec.CreationSecret.Name,
		Namespace: cloudStorage.Namespace,
	}, secret)
	if err != nil {
		return nil, err
	}
	return secret.Data[cloudStorage.Spec.CreationSecret.Key], nil
}

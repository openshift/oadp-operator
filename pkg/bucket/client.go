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
	getCloudStorage() v1alpha1.CloudStorage
	getClient() client.Client
}

func NewClient(b v1alpha1.CloudStorage, c client.Client) (Client, error) {
	switch b.Spec.Provider {
	case v1alpha1.AWSBucketProvider:
		return &awsBucketClient{bucket: b, client: c}, nil
	default:
		return nil, fmt.Errorf("unable to determine bucket client")
	}
}

func getCredentialFromSecret(a Client) (string, error) {
	var filename string
	var ok bool
	namespaceName := types.NamespacedName{Namespace: a.getCloudStorage().Namespace, Name: a.getCloudStorage().Name}
	if filename, ok = fileBucketCache[namespaceName]; !ok {
		// Look for file in tmp based on name.
		// TODO: handle force credential refesh
		secret := &corev1.Secret{}
		err := a.getClient().Get(context.TODO(),
			types.NamespacedName{
				Name:      a.getCloudStorage().Spec.CreationSecret.Name,
				Namespace: a.getCloudStorage().Namespace,
			},
			secret)
		if err != nil {
			return "", err
		}

		cred := secret.Data[a.getCloudStorage().Spec.CreationSecret.Key]
		//create a tmp file based on the bucket name, if it does not exist
		dir, err := os.MkdirTemp("", fmt.Sprintf("secret-%v-%v", a.getCloudStorage().Namespace, a.getCloudStorage().Name))
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
		fileBucketCache[namespaceName] = filename
	}

	return filename, nil
}

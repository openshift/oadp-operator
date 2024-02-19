package bucket

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
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
	default:
		return nil, fmt.Errorf("unable to determine bucket client")
	}
}

func getCredentialFromCloudStorageSecret(a client.Client, cloudStorage v1alpha1.CloudStorage) (string, error) {
	var filename string
	var ok bool
	cloudStorageNamespacedName := types.NamespacedName{
		Name:      cloudStorage.Name,
		Namespace: cloudStorage.Namespace,
	}
	if filename, ok = fileBucketCache[cloudStorageNamespacedName]; !ok {
		// Look for file in tmp based on name.
		// TODO: handle force credential refesh
		secret := &corev1.Secret{}
		err := a.Get(context.TODO(), types.NamespacedName{
			Name:      cloudStorage.Spec.CreationSecret.Name,
			Namespace: cloudStorage.Namespace,
		}, secret)
		if err != nil {
			return "", err
		}

		if common.CCOWorkflow() {
			filename, err = SharedCredentialsFileFromSecret(secret)
			if err != nil {
				return "", err
			}
			return filename, nil
		}

		cred := secret.Data[cloudStorage.Spec.CreationSecret.Key]
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

func SharedCredentialsFileFromSecret(secret *corev1.Secret) (string, error) {
	if len(secret.Data["credentials"]) == 0 {
		return "", errors.New("invalid secret for aws credentials")
	}

	f, err := ioutil.TempFile("", "aws-shared-credentials")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(secret.Data["credentials"]); err != nil {
		return "", err
	}
	return f.Name(), nil
}

package bucket

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials/stsflow"
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
}

func NewClient(b v1alpha1.CloudStorage, c client.Client) (Client, error) {
	switch b.Spec.Provider {
	case v1alpha1.AWSBucketProvider:
		return &awsBucketClient{bucket: b, client: c}, nil
	case v1alpha1.AzureBucketProvider:
		return &azureBucketClient{bucket: b, client: c}, nil
	case v1alpha1.GCPBucketProvider:
		return &gcpBucketClient{bucket: b, client: c}, nil
	default:
		return nil, fmt.Errorf("unsupported bucket provider: %s", b.Spec.Provider)
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

		if stsSecret, err := stsflow.STSStandardizedFlow(); err == nil && stsSecret != "" {
			err := a.Get(context.TODO(), types.NamespacedName{
				Name:      stsSecret,
				Namespace: cloudStorage.Namespace,
			}, secret)
			if err != nil {
				return "", err
			}
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
	// Check for AWS credentials key
	if credData, exists := secret.Data[stsflow.AWSSecretCredentialsKey]; exists && len(credData) > 0 {
		f, err := os.CreateTemp("", "cloud-credentials-aws-")
		if err != nil {
			return "", err
		}
		defer f.Close()
		if _, err := f.Write(credData); err != nil {
			return "", err
		}
		return f.Name(), nil
	}

	// Check for GCP service account JSON key
	if serviceAccountData, exists := secret.Data[stsflow.GcpSecretJSONKey]; exists && len(serviceAccountData) > 0 {
		f, err := os.CreateTemp("", "cloud-credentials-gcp-")
		if err != nil {
			return "", err
		}
		defer f.Close()
		if _, err := f.Write(serviceAccountData); err != nil {
			return "", err
		}
		return f.Name(), nil
	}

	return "", fmt.Errorf("invalid secret: missing %s key (for AWS) or %s key (for GCP)", stsflow.AWSSecretCredentialsKey, stsflow.GcpSecretJSONKey)
}

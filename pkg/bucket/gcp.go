package bucket

import (
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/storage"
	"github.com/openshift/oadp-operator/api/v1alpha1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

/**
In order to create GCP Bucket, we need to create a GCP project.
We expect user to create a GCP project and provide the project ID in config.

*/

type gcpBucketClient struct {
	bucket v1alpha1.CloudStorage
	client client.Client
}

// Return true if bucket got created
func (g gcpBucketClient) Create() (bool, error) {
	sc, err := g.getClient()
	if err != nil {
		return false, err
	}
	defer sc.Close()
	if g.bucket.Name == "" {
		return false, fmt.Errorf("bucket name is empty")
	}
	// Don't check for empty project ID. Defer to API defaults.
	// if g.bucket.Spec.ProjectID == "" {
	// 	return false, fmt.Errorf("project id is empty")
	// }
	// Create bucket 🪣
	projectID, err := g.getProjectID()
	if err != nil {
		return false, err
	}
	err = sc.Bucket(g.bucket.Spec.Name).Create(context.Background(), projectID,
		&storage.BucketAttrs{
			Location: g.bucket.Spec.Region,
			// PublicAccessPrevention: ,
			// StorageClass: ,
			// VersioningEnabled: ,
			Labels: g.bucket.Spec.Tags,
		})
	return err == nil, err
}

// Retusn true if bucket exists
// Return false if bucket does not exist
func (g gcpBucketClient) Exists() (bool, error) {
	sc, err := g.getClient()
	if err != nil {
		return false, err
	}
	defer sc.Close()
	_, err = sc.Bucket(g.bucket.Spec.Name).Attrs(context.Background())
	if err == nil {
		return true, nil
	}
	if err != nil && err.Error() == storage.ErrBucketNotExist.Error() {
		return false, nil
	}
	// no error means bucket exists
	return false, err
}

const errorCodeBucketDoesNotExist = 404

// Returns true if bucket is deleted
// Returns false if bucket is not deleted
func (g gcpBucketClient) Delete() (bool, error) {
	sc, err := g.getClient()
	if err != nil {
		return false, err
	}
	defer sc.Close()
	err = sc.Bucket(g.bucket.Spec.Name).Delete(context.Background())
	if err != nil && err.(*googleapi.Error).Code == errorCodeBucketDoesNotExist {
		err = nil
	}
	return err == nil, err
}

func (g gcpBucketClient) ForceCredentialRefresh() error {
	//TODO: implement
	return nil
}

// helper function to get GCP Storage client
func (g gcpBucketClient) getClient() (*storage.Client, error) {
	ctx := context.Background()
	cred, err := getCredentialFromCloudStorageSecretAsFilename(g.client, g.bucket)
	if err != nil {
		return nil, err
	}
	sc, err := storage.NewClient(ctx, option.WithCredentialsFile(cred))
	return sc, err
}

// helper function to get project ID
// Creation of storage bucket is done in a project.
// We need to get project ID to use to create storage bucket.
// We expect to receive Project ID in bucket.Spec.ProjectID.
// Alternatively we can try to retrieve Project ID from secret.
func (g gcpBucketClient) getProjectID() (string, error) {
	if g.bucket.Spec.ProjectID != "" {
		return g.bucket.Spec.ProjectID, nil
	}
	cred, err := getCredentialFromCloudStorageSecret(g.client, g.bucket)
	if err != nil {
		return "", err
	}
	var credMap map[string]interface{}
	err = json.Unmarshal(cred, &credMap)
	if err != nil {
		return "", err
	}
	_, foundProjectIDInSecret := credMap["project_id"]
	if foundProjectIDInSecret {
		return credMap["project_id"].(string), nil
	}
	return "", fmt.Errorf("project id is not set in secret or CloudStorage spec")
}

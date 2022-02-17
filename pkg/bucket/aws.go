package bucket

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/openshift/oadp-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type awsBucketClient struct {
	bucket v1alpha1.CloudStorage
	client client.Client
}

func (a awsBucketClient) Exists() (bool, error) {
	s3Client, err := a.getS3Client()
	if err != nil {
		return false, err
	}
	input := &s3.HeadBucketInput{
		Bucket: aws.String(a.bucket.Spec.Name),
	}
	_, err = s3Client.HeadBucket(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			// This is supposed to say "NoSuchBucket", but actually emits "NotFound"
			// https://github.com/aws/aws-sdk-go/issues/2593
			case s3.ErrCodeNoSuchBucket, "NotFound":
				return false, nil
			default:
				// Return true, because we are unable to detemine if bucket exists or not
				return true, fmt.Errorf("unable to determine bucket %v status: %v", a.bucket.Spec.Name, aerr.Error())
			}
		} else {
			// Return true, because we are unable to detemine if bucket exists or not
			return true, fmt.Errorf("unable to determine bucket %v status: %v", a.bucket.Spec.Name, aerr.Error())
		}
	}

	err = a.tagBucket()
	if err != nil {
		return true, err
	}

	return true, nil
}

func (a awsBucketClient) Create() (bool, error) {
	s3Client, err := a.getS3Client()
	if err != nil {
		return false, err
	}
	createBucketInput := &s3.CreateBucketInput{
		ACL:    aws.String(s3.BucketCannedACLPrivate),
		Bucket: aws.String(a.bucket.Spec.Name),
	}
	if a.bucket.Spec.Region != "us-east-1" {
		createBucketConfiguration := &s3.CreateBucketConfiguration{
			LocationConstraint: &a.bucket.Spec.Region,
		}
		createBucketInput.SetCreateBucketConfiguration(createBucketConfiguration)
	}
	if err := createBucketInput.Validate(); err != nil {
		return false, fmt.Errorf("unable to validate %v bucket creation configuration: %v", a.bucket.Spec.Name, err)
	}

	_, err = s3Client.CreateBucket(createBucketInput)
	if err != nil {
		return false, err
	}

	// tag Bucket.
	err = a.tagBucket()
	if err != nil {
		return true, err
	}

	return true, nil
}

func (a awsBucketClient) tagBucket() error {
	s3Client, err := a.getS3Client()
	// Clear bucket tags.
	if err != nil {
		return err
	}
	deleteInput := &s3.DeleteBucketTaggingInput{Bucket: aws.String(a.bucket.Spec.Name)}
	_, err = s3Client.DeleteBucketTagging(deleteInput)
	if err != nil {
		return err
	}
	input := CreateBucketTaggingInput(a.bucket.Spec.Name, a.bucket.Spec.Tags)

	_, err = s3Client.PutBucketTagging(input)
	if err != nil {
		return err
	}
	return nil
}

// CreateBucketTaggingInput creates an S3 PutBucketTaggingInput object,
// which is used to associate a list of tags with a bucket.
func CreateBucketTaggingInput(bucketname string, tags map[string]string) *s3.PutBucketTaggingInput {
	putInput := &s3.PutBucketTaggingInput{
		Bucket: aws.String(bucketname),
		Tagging: &s3.Tagging{
			TagSet: []*s3.Tag{},
		},
	}
	for key, value := range tags {
		newTag := s3.Tag{
			Key:   aws.String(key),
			Value: aws.String(value),
		}
		putInput.Tagging.TagSet = append(putInput.Tagging.TagSet, &newTag)
	}
	return putInput
}

func (a awsBucketClient) getS3Client() (s3iface.S3API, error) {
	awsConfig := &aws.Config{Region: &a.bucket.Spec.Region}
	cred, err := getCredentialFromCloudStorageSecretAsFilename(a.client, a.bucket)
	if err != nil {
		return nil, err
	}

	opts := session.Options{
		Config:            *awsConfig,
		SharedConfigFiles: []string{cred},
	}

	if a.bucket.Spec.EnableSharedConfig != nil && *a.bucket.Spec.EnableSharedConfig {
		opts.SharedConfigState = session.SharedConfigEnable
	}

	s, err := session.NewSessionWithOptions(opts)
	if err != nil {
		return nil, err
	}
	return s3.New(s), nil
}

func (a awsBucketClient) ForceCredentialRefresh() error {
	return fmt.Errorf("force credential refresh is not yet implemented")
}

func (a awsBucketClient) Delete() (bool, error) {
	s3Client, err := a.getS3Client()
	if err != nil {
		return false, err
	}
	deleteBucketInput := &s3.DeleteBucketInput{
		Bucket: aws.String(a.bucket.Spec.Name),
	}

	if err := deleteBucketInput.Validate(); err != nil {
		return false, fmt.Errorf("unable to validate %v bucket deletion configuration: %v", a.bucket.Spec.Name, err)
	}

	_, err = s3Client.DeleteBucket(deleteBucketInput)
	if err != nil {
		return false, err
	}

	return true, nil
}

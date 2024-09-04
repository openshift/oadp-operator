package aws

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func BucketRegionIsDiscoverable(bucket string) bool {
	_, err := GetBucketRegion(bucket)
	return err == nil
}

// GetBucketRegion returns the AWS region that a bucket is in, or an error
// if the region cannot be determined.
// copied from https://github.com/openshift/openshift-velero-plugin/pull/223/files#diff-da482ef606b3938b09ae46990a60eb0ad49ebfb4885eb1af327d90f215bf58b1
// modified to aws-sdk-go-v2
func GetBucketRegion(bucket string) (string, error) {
	var region string
	// GetBucketRegion will attempt to get the region for a bucket using the client's configured region to determine
	// which AWS partition to perform the query on.
	// Client therefore needs to be configured with region.
	// In local dev environments, you might have ~/.aws/config that could be loaded and set with default region.
	// In cluster/CI environment, ~/.aws/config may not be configured, so set hinting region server explicitly.
	// Also set to use anonymous credentials. If the bucket is private, this function would not work unless we modify it to take credentials.
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), // This is not default region being used, this is to specify a region hinting server that we will use to get region from.
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	)
	if err != nil {
		return "", err
	}
	region, err = manager.GetBucketRegion(context.Background(), s3.NewFromConfig(cfg), bucket)
	if region != "" {
		return region, nil
	}
	return "", errors.New("unable to determine bucket's region: " + err.Error())
}

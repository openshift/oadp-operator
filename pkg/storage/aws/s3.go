package aws

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go/aws/request"
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
	)
	if err != nil {
		return "", err
	}
	region, err = manager.GetBucketRegion(context.Background(), s3.NewFromConfig(cfg), bucket, func(o *s3.Options) {
	    // TODO: get creds from bsl 
		o.Credentials = credentials.NewStaticCredentialsProvider("anon-credentials", "anon-secret", "") // this works with private buckets.. why? supposed to require cred with s3:ListBucket https://docs.aws.amazon.com/AmazonS3/latest/API/API_HeadBucket.html
	})
	if region != "" {
		return region, nil
	}
	return "", errors.New("unable to determine bucket's region: " + err.Error())
}

// StripDefaultPorts removes port 80 from HTTP URLs and 443 from HTTPS URLs.
// Defer to the actual AWS SDK implementation to match its behavior exactly.
func StripDefaultPorts(fromUrl string) (string, error) {
	u, err := url.Parse(fromUrl)
	if err != nil {
		return "", err
	}
	r := http.Request{
		URL: u,
	}
	request.SanitizeHostForHeader(&r)
	if r.Host != "" {
		r.URL.Host = r.Host
	}
	return r.URL.String(), nil
}

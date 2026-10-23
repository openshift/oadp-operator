package aws

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go/aws/request"
)

// GetBucketRegionFunc is the function used to get bucket region.
// It can be replaced in tests for mocking.
var GetBucketRegionFunc = getBucketRegionImpl

// Bucket region lookups hit the public S3 HeadBucket API, which occasionally fails
// transiently (e.g. connection resets). Retry a few times so a single blip does not
// surface as "region ... not automatically discoverable" on an otherwise healthy DPA.
// Variables so tests can shrink them.
var (
	bucketRegionAttempts     = 3
	bucketRegionRetryBackoff = 500 * time.Millisecond
	// bucketRegionAttemptTimeout bounds each lookup attempt so retries cannot hang a reconcile.
	bucketRegionAttemptTimeout = 15 * time.Second
)

func BucketRegionIsDiscoverable(bucket string) bool {
	_, err := GetBucketRegionFunc(bucket)
	return err == nil
}

// GetBucketRegion returns the AWS region that a bucket is in, or an error
// if the region cannot be determined. This is a wrapper that calls GetBucketRegionFunc.
func GetBucketRegion(bucket string) (string, error) {
	return GetBucketRegionFunc(bucket)
}

// getBucketRegionImpl is the actual implementation that calls AWS.
// copied from https://github.com/openshift/openshift-velero-plugin/pull/223/files#diff-da482ef606b3938b09ae46990a60eb0ad49ebfb4885eb1af327d90f215bf58b1
// modified to aws-sdk-go-v2
func getBucketRegionImpl(bucket string) (string, error) {
	return retryBucketRegion(func() (string, error) { return getBucketRegionOnce(bucket) })
}

// retryBucketRegion calls lookup up to bucketRegionAttempts times with doubling backoff,
// returning the first non-empty region or the last error.
func retryBucketRegion(lookup func() (string, error)) (string, error) {
	var (
		region string
		err    error
	)
	backoff := bucketRegionRetryBackoff
	for attempt := 1; attempt <= bucketRegionAttempts; attempt++ {
		if region, err = lookup(); err == nil {
			return region, nil
		}
		if attempt < bucketRegionAttempts {
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	return "", err
}

func getBucketRegionOnce(bucket string) (string, error) {
	var region string
	ctx, cancel := context.WithTimeout(context.Background(), bucketRegionAttemptTimeout)
	defer cancel()
	// GetBucketRegion will attempt to get the region for a bucket using the client's configured region to determine
	// which AWS partition to perform the query on.
	// Client therefore needs to be configured with region.
	// In local dev environments, you might have ~/.aws/config that could be loaded and set with default region.
	// In cluster/CI environment, ~/.aws/config may not be configured, so set hinting region server explicitly.
	// Also set to use anonymous credentials. This works for both public and private buckets as AWS Security
	// confirmed that HeadBucket API (used by GetBucketRegion) doesn't enforce s3:ListBucket permissions
	// for region retrieval - this is expected AWS behavior.
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"), // This is not default region being used, this is to specify a region hinting server that we will use to get region from.
	)
	if err != nil {
		return "", err
	}
	region, err = manager.GetBucketRegion(ctx, s3.NewFromConfig(cfg), bucket, func(o *s3.Options) {
		// AWS Security confirmed that anonymous credentials can be used here for GetBucketRegion.
		// The HeadBucket API endpoint used internally by GetBucketRegion does not enforce
		// s3:ListBucket permissions for retrieving bucket region information.
		// Reference: AWS Security response (Engagement ID: CACenGS4Mha_KeJ=e3jBSLD6rPZ2iNtfuJUv9QJViaCOt7GVNDg)
		// This is expected AWS behavior, not a security vulnerability.
		o.Credentials = credentials.NewStaticCredentialsProvider("anon-credentials", "anon-secret", "")
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

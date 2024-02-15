package aws

import (
	"context"

	"errors"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	// "github.com/pkg/errors" don't need stack trace, not inside velero-plugin
)

func BucketRegionIsDiscoverable(bucket string) bool {
	_, err := GetBucketRegion(bucket)
	return err == nil
}

// GetBucketRegion returns the AWS region that a bucket is in, or an error
// if the region cannot be determined.
// copied from https://github.com/openshift/openshift-velero-plugin/pull/223/files#diff-da482ef606b3938b09ae46990a60eb0ad49ebfb4885eb1af327d90f215bf58b1
func GetBucketRegion(bucket string) (string, error) {
	var region string

	session, err := session.NewSession()
	if err != nil {
		// return "", errors.WithStack(err) // don't need stack trace, not inside velero-plugin
		return "", err
	}

	for _, partition := range endpoints.DefaultPartitions() {
		for regionHint := range partition.Regions() {
			region, _ = s3manager.GetBucketRegion(context.Background(), session, bucket, regionHint)

			// we only need to try a single region hint per partition, so break after the first
			break
		}

		if region != "" {
			return region, nil
		}
	}

	return "", errors.New("unable to determine bucket's region")
}

// For tests to use when discoverable bucket is required.
const DiscoverableBucket string = "openshift-velero-plugin-s3-auto-region-test-1"

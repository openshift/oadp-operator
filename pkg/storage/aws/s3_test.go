package aws

import (
	"testing"
)

// from https://github.com/openshift/openshift-velero-plugin/pull/223/files#diff-4f17f1708744bd4d8cb7a4232212efa0e3bfde2b9c7b12e3a23dcc913b9fc2ec
func TestGetBucketRegion(t *testing.T) {
	tests := []struct {
		name    string
		bucket  string
		region  string
		wantErr bool
	}{
		{
			name:    "openshift-velero-plugin-s3-auto-region-test-1",
			bucket:  "openshift-velero-plugin-s3-auto-region-test-1",
			region:  "us-east-1",
			wantErr: false,
		},
		{
			name:    "openshift-velero-plugin-s3-auto-region-test-2",
			bucket:  "openshift-velero-plugin-s3-auto-region-test-2",
			region:  "us-west-1",
			wantErr: false,
		},
		{
			name:    "openshift-velero-plugin-s3-auto-region-test-3",
			bucket:  "openshift-velero-plugin-s3-auto-region-test-3",
			region:  "eu-central-1",
			wantErr: false,
		},
		{
			name:    "openshift-velero-plugin-s3-auto-region-test-4",
			bucket:  "openshift-velero-plugin-s3-auto-region-test-4",
			region:  "sa-east-1",
			wantErr: false,
		},
		{
			name:    "velero-6109f5e9711c8c58131acdd2f490f451", // oadp prow aws bucket name
			bucket:  "velero-6109f5e9711c8c58131acdd2f490f451",
			region:  "us-east-1",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetBucketRegion(tt.bucket)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetBucketRegion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.region {
				t.Errorf("GetBucketRegion() = %v, want %v", got, tt.region)
			}
		})
	}
}

func TestBucketRegionIsDiscoverable(t *testing.T) {
	tests := []struct {
		name         string
		bucket       string
		discoverable bool
	}{
		{
			name:         "openshift-velero-plugin-s3-auto-region-test-1 region is discoverable",
			bucket:       "openshift-velero-plugin-s3-auto-region-test-1",
			discoverable: true,
		},
		{
			name:         "bucketNamesOverSixtyThreeCharactersAndNowItIsAboutTimeToTestThisFunction is an invalid aws bucket name so region is not discoverable",
			bucket:       "bucketNamesOverSixtyThreeCharactersAndNowItIsAboutTimeToTestThisFunction",
			discoverable: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BucketRegionIsDiscoverable(tt.bucket); got != tt.discoverable {
				t.Errorf("BucketRegionIsDiscoverable() = %v, want %v", got, tt.discoverable)
			}
		})
	}
}

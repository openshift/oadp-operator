package aws

import (
	"reflect"
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
			// This should work anonymously, this bucket is made public with Bucket policy
			// {
			// 	"Version": "2012-10-17",
			// 	"Statement": [
			// 		{
			// 			"Sid": "publicList",
			// 			"Effect": "Allow",
			// 			"Principal": "*",
			// 			"Action": "s3:ListBucket",
			// 			"Resource": "arn:aws:s3:::openshift-velero-plugin-s3-auto-region-test-1"
			// 		}
			// 	]
			// }
			// ❯ aws s3api head-bucket --bucket openshift-velero-plugin-s3-auto-region-test-1 --no-sign-request 
			// {
			//     "BucketRegion": "us-east-1",
			//     "AccessPointAlias": false
			// }
			name:    "openshift-velero-plugin-s3-auto-region-test-1",
			bucket:  "openshift-velero-plugin-s3-auto-region-test-1",
			region:  "us-east-1",
			wantErr: false,
		},
		{
			// This should require creds
			// ❯ aws s3api head-bucket --bucket openshift-velero-plugin-s3-auto-region-test-2 --no-sign-request

			// An error occurred (403) when calling the HeadBucket operation: Forbidden 
			name:    "openshift-velero-plugin-s3-auto-region-test-2",
			bucket:  "openshift-velero-plugin-s3-auto-region-test-2",
			region:  "us-west-1",
			wantErr: false,
			// TODO: path/param to creds on ci
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
			// TODO: add creds usage here.
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

func TestStripDefaultPorts(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{
			name: "port-free URL is returned unchanged",
			base: "https://s3.region.cloud-object-storage.appdomain.cloud/bucket-name",
			want: "https://s3.region.cloud-object-storage.appdomain.cloud/bucket-name",
		},
		{
			name: "HTTPS port is removed from URL",
			base: "https://s3.region.cloud-object-storage.appdomain.cloud:443/bucket-name",
			want: "https://s3.region.cloud-object-storage.appdomain.cloud/bucket-name",
		},
		{
			name: "HTTP port is removed from URL",
			base: "http://s3.region.cloud-object-storage.appdomain.cloud:80/bucket-name",
			want: "http://s3.region.cloud-object-storage.appdomain.cloud/bucket-name",
		},
		{
			name: "alternate HTTP port is preserved",
			base: "http://10.0.188.30:9000",
			want: "http://10.0.188.30:9000",
		},
		{
			name: "alternate HTTPS port is preserved",
			base: "https://10.0.188.30:9000",
			want: "https://10.0.188.30:9000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := StripDefaultPorts(tt.base)
			if err != nil {
				t.Errorf("An error occurred: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StripDefaultPorts() = %v, want %v", got, tt.want)
			}
		})
	}
}

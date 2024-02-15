package aws

import (
	"testing"
)

// from https://github.com/openshift/openshift-velero-plugin/pull/223/files#diff-4f17f1708744bd4d8cb7a4232212efa0e3bfde2b9c7b12e3a23dcc913b9fc2ec
func TestGetBucketRegion(t *testing.T) {
	type args struct {
		bucket string
	}
	tests := []struct {
		name    string
		bucket  string
		want    string
		wantErr bool
	}{
		{
			name:    "openshift-velero-plugin-s3-auto-region-test-1",
			bucket:  "openshift-velero-plugin-s3-auto-region-test-1",
			want:    "us-east-1",
			wantErr: false,
		},
		{
			name:    "openshift-velero-plugin-s3-auto-region-test-2",
			bucket:  "openshift-velero-plugin-s3-auto-region-test-2",
			want:    "us-west-1",
			wantErr: false,
		},
		{
			name:    "openshift-velero-plugin-s3-auto-region-test-3",
			bucket:  "openshift-velero-plugin-s3-auto-region-test-3",
			want:    "eu-central-1",
			wantErr: false,
		},
		{
			name:    "openshift-velero-plugin-s3-auto-region-test-4",
			bucket:  "openshift-velero-plugin-s3-auto-region-test-4",
			want:    "sa-east-1",
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
			if got != tt.want {
				t.Errorf("GetBucketRegion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBucketRegionIsDiscoverable(t *testing.T) {
	type args struct {
		bucket string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "openshift-velero-plugin-s3-auto-region-test-1 region is discoverable",
			args: args{bucket: "openshift-velero-plugin-s3-auto-region-test-1"},
			want: true,
		},
		{
			name: "bucketNamesOverSixtyThreeCharactersAndNowItIsAboutTimeToTestThisFunction is an invalid aws bucket name so region is not discoverable",
			args: args{bucket: "bucketNamesOverSixtyThreeCharactersAndNowItIsAboutTimeToTestThisFunction"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BucketRegionIsDiscoverable(tt.args.bucket); got != tt.want {
				t.Errorf("BucketRegionIsDiscoverable() = %v, want %v", got, tt.want)
			}
		})
	}
}

package aws

import (
	"errors"
	"testing"
)

func TestGetBucketRegion(t *testing.T) {
	tests := []struct {
		name       string
		bucket     string
		mockRegion string
		mockErr    error
		wantRegion string
		wantErr    bool
	}{
		{
			name:       "successful region discovery",
			bucket:     "test-bucket",
			mockRegion: "us-east-1",
			mockErr:    nil,
			wantRegion: "us-east-1",
			wantErr:    false,
		},
		{
			name:       "region discovery for eu-central-1",
			bucket:     "eu-bucket",
			mockRegion: "eu-central-1",
			mockErr:    nil,
			wantRegion: "eu-central-1",
			wantErr:    false,
		},
		{
			name:       "region discovery fails",
			bucket:     "non-existent-bucket",
			mockRegion: "",
			mockErr:    errors.New("bucket not found"),
			wantRegion: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original and restore after test
			originalFunc := GetBucketRegionFunc
			defer func() { GetBucketRegionFunc = originalFunc }()

			// Set up mock
			GetBucketRegionFunc = func(bucket string) (string, error) {
				if bucket != tt.bucket {
					t.Errorf("GetBucketRegion called with wrong bucket: got %v, want %v", bucket, tt.bucket)
				}
				return tt.mockRegion, tt.mockErr
			}

			got, err := GetBucketRegion(tt.bucket)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetBucketRegion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantRegion {
				t.Errorf("GetBucketRegion() = %v, want %v", got, tt.wantRegion)
			}
		})
	}
}

func TestBucketRegionIsDiscoverable(t *testing.T) {
	tests := []struct {
		name         string
		bucket       string
		mockRegion   string
		mockErr      error
		discoverable bool
	}{
		{
			name:         "bucket region is discoverable",
			bucket:       "discoverable-bucket",
			mockRegion:   "us-east-1",
			mockErr:      nil,
			discoverable: true,
		},
		{
			name:         "bucket region is not discoverable due to error",
			bucket:       "non-existent-bucket",
			mockRegion:   "",
			mockErr:      errors.New("bucket not found"),
			discoverable: false,
		},
		{
			name:         "invalid bucket name - region not discoverable",
			bucket:       "bucketNamesOverSixtyThreeCharactersAndNowItIsAboutTimeToTestThisFunction",
			mockRegion:   "",
			mockErr:      errors.New("invalid bucket name"),
			discoverable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original and restore after test
			originalFunc := GetBucketRegionFunc
			defer func() { GetBucketRegionFunc = originalFunc }()

			// Set up mock
			GetBucketRegionFunc = func(bucket string) (string, error) {
				return tt.mockRegion, tt.mockErr
			}

			if got := BucketRegionIsDiscoverable(tt.bucket); got != tt.discoverable {
				t.Errorf("BucketRegionIsDiscoverable() = %v, want %v", got, tt.discoverable)
			}
		})
	}
}

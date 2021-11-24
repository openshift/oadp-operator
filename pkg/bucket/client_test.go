package bucket_test

import (
	"testing"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/bucket"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		bucket  oadpv1alpha1.CloudStorage
		wantErr bool
		want    bool
	}{
		{
			name: "Test AWS",
			bucket: oadpv1alpha1.CloudStorage{
				Spec: oadpv1alpha1.CloudStorageSpec{
					Provider: oadpv1alpha1.AWSBucketProvider,
				},
			},
			wantErr: false,
			want:    true,
		},
		{
			name: "Error when invalid provider",
			bucket: oadpv1alpha1.CloudStorage{
				Spec: oadpv1alpha1.CloudStorageSpec{
					Provider: "invalid",
				},
			},
			wantErr: true,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bclnt, err := bucket.NewClient(tt.bucket, nil)

			if (err != nil) != tt.wantErr {
				t.Errorf("wanted err: %v but did not get one want err: %v", err, tt.wantErr)
				return
			}
			if (bclnt != nil) != tt.want {
				t.Errorf("want: %v but did got: %#v", tt.want, bclnt)
				return
			}
		})
	}
}

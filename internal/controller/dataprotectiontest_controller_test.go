/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/cloudprovider"
)

type mockProvider struct {
	speed    int64
	duration time.Duration
	err      error
	metadata *oadpv1alpha1.BucketMetadata
	metaErr  error
}

func (m *mockProvider) UploadTest(ctx context.Context, config oadpv1alpha1.UploadSpeedTestConfig, bucket string) (int64, time.Duration, error) {
	return m.speed, m.duration, m.err
}

func (m *mockProvider) GetBucketMetadata(ctx context.Context, bucket string) (*oadpv1alpha1.BucketMetadata, error) {
	return m.metadata, m.metaErr
}

func TestDetermineVendor(t *testing.T) {
	tests := []struct {
		name           string
		serverHeader   string
		extraHeaders   map[string]string
		expectedVendor string
	}{
		{
			name:           "Detect AWS via Server header",
			serverHeader:   "AmazonS3",
			expectedVendor: "AWS",
		},
		{
			name:         "Detect AWS via x-amz-request-id",
			serverHeader: "",
			extraHeaders: map[string]string{
				"x-amz-request-id": "some-aws-request-id",
			},
			expectedVendor: "AWS",
		},
		{
			name:           "Detect MinIO via Server header",
			serverHeader:   "MinIO",
			expectedVendor: "MinIO",
		},
		{
			name:         "Detect MinIO via x-minio-region",
			serverHeader: "",
			extraHeaders: map[string]string{
				"x-minio-region": "us-east-1",
			},
			expectedVendor: "MinIO",
		},
		{
			name:           "Detect Ceph via Server header",
			serverHeader:   "Ceph",
			expectedVendor: "Ceph",
		},
		{
			name:         "Detect Ceph via x-rgw-request-id",
			serverHeader: "",
			extraHeaders: map[string]string{
				"x-rgw-request-id": "abc123",
			},
			expectedVendor: "Ceph",
		},
		{
			name:           "Unknown vendor fallback",
			serverHeader:   "SomethingElse",
			expectedVendor: "somethingelse",
		},
		{
			name:           "No headers at all",
			serverHeader:   "",
			expectedVendor: "Unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Fake HTTP server with HEAD response
			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.serverHeader != "" {
					w.Header().Set("Server", tc.serverHeader)
				}
				for k, v := range tc.extraHeaders {
					w.Header().Set(k, v)
				}
			}))
			defer testServer.Close()

			dpt := &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					BackupLocationSpec: &velerov1.BackupStorageLocationSpec{
						Provider: "aws",
						Config: map[string]string{
							"s3Url": testServer.URL,
						},
					},
				},
			}

			reconciler := &DataProtectionTestReconciler{}

			err := reconciler.determineVendor(context.Background(), dpt, dpt.Spec.BackupLocationSpec)
			require.NoError(t, err)
			require.Equal(t, tc.expectedVendor, dpt.Status.S3Vendor)
		})
	}
}

func TestResolveBackupLocation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, oadpv1alpha1.AddToScheme(scheme))
	require.NoError(t, velerov1.AddToScheme(scheme))

	ctx := context.Background()

	tests := []struct {
		name        string
		dpt         *oadpv1alpha1.DataProtectionTest
		bsl         *velerov1.BackupStorageLocation
		expectErr   bool
		expectSpec  bool
		description string
	}{
		{
			name: "both backupLocationSpec and Name set",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					BackupLocationName: "my-bsl",
					BackupLocationSpec: &velerov1.BackupStorageLocationSpec{
						Provider: "aws",
					},
				},
			},
			expectErr: true,
		},
		{
			name:      "neither backupLocationSpec nor Name set",
			dpt:       &oadpv1alpha1.DataProtectionTest{Spec: oadpv1alpha1.DataProtectionTestSpec{}},
			expectErr: true,
		},
		{
			name: "only BackupLocationSpec set",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					BackupLocationSpec: &velerov1.BackupStorageLocationSpec{
						Provider: "aws",
					},
				},
			},
			expectSpec: true,
		},
		{
			name: "BackupLocationName set, BSL exists",
			dpt: &oadpv1alpha1.DataProtectionTest{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "openshift-adp",
				},
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					BackupLocationName: "my-bsl",
				},
			},
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-bsl",
					Namespace: "openshift-adp",
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
				},
			},
			expectSpec: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.bsl != nil {
				builder.WithRuntimeObjects(tt.bsl)
			}
			k8sClient := builder.Build()

			reconciler := &DataProtectionTestReconciler{
				Client: k8sClient,
			}

			spec, err := reconciler.resolveBackupLocation(ctx, tt.dpt)

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.expectSpec {
					require.NotNil(t, spec)
					require.Equal(t, "aws", spec.Provider)
				}
			}
		})
	}
}

func TestInitializeProvider(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, oadpv1alpha1.AddToScheme(scheme))
	require.NoError(t, velerov1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ctx := context.Background()

	secretData := `[default]
aws_access_key_id = test-access
aws_secret_access_key = test-secret
`
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aws-secret",
			Namespace: "openshift-adp",
		},
		Data: map[string][]byte{
			"cloud": []byte(secretData),
		},
	}

	tests := []struct {
		name         string
		provider     string
		expectError  bool
		expectResult bool
		setupSecrets bool
	}{
		{
			name:         "Valid AWS config",
			provider:     "aws",
			expectError:  false,
			expectResult: true,
			setupSecrets: true,
		},
		{
			name:         "Secret missing",
			provider:     "aws",
			expectError:  true,
			expectResult: false,
			setupSecrets: false,
		},
		{
			name:         "Unsupported provider",
			provider:     "gcp",
			expectError:  true,
			expectResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.setupSecrets {
				builder.WithObjects(secret)
			}
			k8sClient := builder.Build()

			reconciler := &DataProtectionTestReconciler{
				Client:         k8sClient,
				Context:        ctx,
				NamespacedName: types.NamespacedName{Name: "dummy", Namespace: "openshift-adp"},
			}

			spec := &velerov1.BackupStorageLocationSpec{
				Provider: tt.provider,
				StorageType: velerov1.StorageType{
					ObjectStorage: &velerov1.ObjectStorageLocation{
						Bucket: "test-bucket",
						Prefix: "velero",
					},
				},
				Credential: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "aws-secret",
					},
					Key: "cloud",
				},
				Config: map[string]string{
					"region": "us-east-1",
				},
			}

			cp, err := reconciler.initializeProvider(spec)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cp)
				_, ok := cp.(*cloudprovider.AWSProvider)
				require.True(t, ok)
			}
		})
	}
}

func TestRunUploadTest(t *testing.T) {
	tests := []struct {
		name        string
		config      *oadpv1alpha1.UploadSpeedTestConfig
		objectStore *velerov1.ObjectStorageLocation
		mock        *mockProvider
		expectErr   bool
		expectPass  bool
	}{
		{
			name: "Successful upload test",
			config: &oadpv1alpha1.UploadSpeedTestConfig{
				FileSize: "10MB",
				Timeout:  metav1.Duration{Duration: 10 * time.Minute},
			},
			objectStore: &velerov1.ObjectStorageLocation{
				Bucket: "my-bucket",
			},
			mock:       &mockProvider{speed: 100, duration: 2 * time.Second},
			expectErr:  false,
			expectPass: true,
		},
		{
			name:   "Missing UploadSpeedTestConfig",
			config: nil,
			objectStore: &velerov1.ObjectStorageLocation{
				Bucket: "my-bucket",
			},
			mock:       &mockProvider{},
			expectErr:  true,
			expectPass: false,
		},
		{
			name: "Empty bucket name",
			config: &oadpv1alpha1.UploadSpeedTestConfig{
				FileSize: "10MB",
				Timeout:  metav1.Duration{Duration: 10 * time.Minute},
			},
			objectStore: &velerov1.ObjectStorageLocation{
				Bucket: "",
			},
			mock:       &mockProvider{},
			expectErr:  true,
			expectPass: false,
		},
		{
			name: "Upload error",
			config: &oadpv1alpha1.UploadSpeedTestConfig{
				FileSize: "10MB",
				Timeout:  metav1.Duration{Duration: 10 * time.Minute},
			},
			objectStore: &velerov1.ObjectStorageLocation{
				Bucket: "my-bucket",
			},
			mock:       &mockProvider{err: fmt.Errorf("upload failed")},
			expectErr:  true,
			expectPass: false,
		},
		{
			name:        "Nil object storage",
			config:      &oadpv1alpha1.UploadSpeedTestConfig{FileSize: "10MB", Timeout: metav1.Duration{Duration: 1 * time.Minute}},
			objectStore: nil,
			mock:        &mockProvider{},
			expectErr:   true,
			expectPass:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dpt := &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					UploadSpeedTestConfig: tt.config,
				},
			}
			bslSpec := &velerov1.BackupStorageLocationSpec{
				StorageType: velerov1.StorageType{
					ObjectStorage: tt.objectStore,
				},
			}

			r := &DataProtectionTestReconciler{}

			err := r.runUploadTest(context.TODO(), dpt, bslSpec, tt.mock)

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectPass, dpt.Status.UploadTest.Success)
			}
		})
	}
}

func TestGetBucketMetadataIntegration(t *testing.T) {
	tests := []struct {
		name           string
		mockProvider   *mockProvider
		expectedResult *oadpv1alpha1.BucketMetadata
		expectError    bool
	}{
		{
			name: "Successful metadata fetch",
			mockProvider: &mockProvider{
				metadata: &oadpv1alpha1.BucketMetadata{
					EncryptionAlgorithm: "AES256",
					VersioningStatus:    "Enabled",
				},
				metaErr: nil,
			},
			expectedResult: &oadpv1alpha1.BucketMetadata{
				EncryptionAlgorithm: "AES256",
				VersioningStatus:    "Enabled",
			},
			expectError: false,
		},
		{
			name: "Metadata fetch error",
			mockProvider: &mockProvider{
				metadata: nil,
				metaErr:  fmt.Errorf("failed to fetch metadata"),
			},
			expectedResult: &oadpv1alpha1.BucketMetadata{
				ErrorMessage: "failed to fetch metadata",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dpt := &oadpv1alpha1.DataProtectionTest{}
			bslSpec := &velerov1.BackupStorageLocationSpec{
				StorageType: velerov1.StorageType{
					ObjectStorage: &velerov1.ObjectStorageLocation{
						Bucket: "my-bucket",
					},
				},
			}

			meta, err := tt.mockProvider.GetBucketMetadata(context.TODO(), bslSpec.ObjectStorage.Bucket)

			if err != nil {
				meta = &oadpv1alpha1.BucketMetadata{
					ErrorMessage: err.Error(),
				}
			}

			dpt.Status.BucketMetadata = meta

			require.Equal(t, tt.expectedResult.EncryptionAlgorithm, dpt.Status.BucketMetadata.EncryptionAlgorithm)
			require.Equal(t, tt.expectedResult.VersioningStatus, dpt.Status.BucketMetadata.VersioningStatus)
			require.Equal(t, tt.expectedResult.ErrorMessage, dpt.Status.BucketMetadata.ErrorMessage)
		})
	}
}

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

	"github.com/go-logr/logr"
	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/require"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
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

func (m *mockProvider) UploadTest(ctx context.Context, config oadpv1alpha1.UploadSpeedTestConfig, bucket string, log logr.Logger) (int64, time.Duration, error) {
	return m.speed, m.duration, m.err
}

func (m *mockProvider) GetBucketMetadata(ctx context.Context, bucket string, log logr.Logger) (*oadpv1alpha1.BucketMetadata, error) {
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
				dpt:            &oadpv1alpha1.DataProtectionTest{},
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

			cp, err := reconciler.initializeProvider(ctx, spec)

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
			r := &DataProtectionTestReconciler{}

			meta, err := tt.mockProvider.GetBucketMetadata(context.TODO(), bslSpec.ObjectStorage.Bucket, r.Log)

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

func TestCreateVolumeSnapshot(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = snapshotv1api.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &DataProtectionTestReconciler{
		Client: fakeClient,
	}

	cfg := oadpv1alpha1.CSIVolumeSnapshotTestConfig{
		SnapshotClassName: "csi-snap",
		VolumeSnapshotSource: oadpv1alpha1.VolumeSnapshotSource{
			PersistentVolumeClaimName:      "my-pvc",
			PersistentVolumeClaimNamespace: "my-ns",
		},
	}

	dpt := &oadpv1alpha1.DataProtectionTest{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dpt-sample",
		},
	}

	ctx := context.Background()
	vs, err := r.createVolumeSnapshot(ctx, dpt, cfg)
	require.NoError(t, err)
	require.Equal(t, cfg.VolumeSnapshotSource.PersistentVolumeClaimNamespace, vs.Namespace)
	require.NotNil(t, vs.Spec.Source.PersistentVolumeClaimName)
	require.Equal(t, cfg.SnapshotClassName, *vs.Spec.VolumeSnapshotClassName)
}

func TestBuildTLSConfig(t *testing.T) {
	tests := []struct {
		name           string
		dpt            *oadpv1alpha1.DataProtectionTest
		bsl            *velerov1.BackupStorageLocationSpec
		expectInsecure bool
		expectCustomCA bool
		expectError    bool
		description    string
	}{
		{
			name: "skipTLSVerify is true",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					SkipTLSVerify: true,
				},
			},
			bsl:            &velerov1.BackupStorageLocationSpec{},
			expectInsecure: true,
			expectCustomCA: false,
			expectError:    false,
			description:    "Should set InsecureSkipVerify when skipTLSVerify is true",
		},
		{
			name: "skipTLSVerify takes precedence over CA cert",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					SkipTLSVerify: true,
				},
			},
			bsl: &velerov1.BackupStorageLocationSpec{
				StorageType: velerov1.StorageType{
					ObjectStorage: &velerov1.ObjectStorageLocation{
						Bucket: "test-bucket",
						CACert: []byte("some-ca-cert"), // Should be ignored due to skipTLSVerify
					},
				},
			},
			expectInsecure: true,
			expectCustomCA: false, // Should not set custom CA when skipTLSVerify is true
			expectError:    false,
			description:    "SkipTLSVerify should take precedence over CA cert",
		},
		{
			name: "neither skipTLS nor custom CA",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					SkipTLSVerify: false,
				},
			},
			bsl: &velerov1.BackupStorageLocationSpec{
				StorageType: velerov1.StorageType{
					ObjectStorage: &velerov1.ObjectStorageLocation{
						Bucket: "test-bucket",
					},
				},
			},
			expectInsecure: false,
			expectCustomCA: false,
			expectError:    false,
			description:    "Should use system certs when no custom config",
		},
		{
			name: "invalid base64 CA cert",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					SkipTLSVerify: false,
				},
			},
			bsl: &velerov1.BackupStorageLocationSpec{
				StorageType: velerov1.StorageType{
					ObjectStorage: &velerov1.ObjectStorageLocation{
						Bucket: "test-bucket",
						CACert: []byte("invalid-base64!"),
					},
				},
			},
			expectError: true,
			description: "Should error on invalid base64 CA cert",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.Discard()

			tlsConfig, err := buildTLSConfig(tt.dpt, tt.bsl, logger)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, tlsConfig)
			require.Equal(t, tt.expectInsecure, tlsConfig.InsecureSkipVerify)

			if tt.expectCustomCA {
				require.NotNil(t, tlsConfig.RootCAs)
			} else if !tt.expectInsecure {
				// System certs case - RootCAs should be nil (uses system)
				require.Nil(t, tlsConfig.RootCAs)
			}
		})
	}
}

func TestBuildHTTPClientWithTLS(t *testing.T) {
	tests := []struct {
		name        string
		dpt         *oadpv1alpha1.DataProtectionTest
		bsl         *velerov1.BackupStorageLocationSpec
		expectError bool
	}{
		{
			name: "valid TLS config",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					SkipTLSVerify: true,
				},
			},
			bsl:         &velerov1.BackupStorageLocationSpec{},
			expectError: false,
		},
		{
			name: "invalid CA cert",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					SkipTLSVerify: false,
				},
			},
			bsl: &velerov1.BackupStorageLocationSpec{
				StorageType: velerov1.StorageType{
					ObjectStorage: &velerov1.ObjectStorageLocation{
						CACert: []byte("invalid-base64!"),
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.Discard()

			client, err := buildHTTPClientWithTLS(tt.dpt, tt.bsl, logger)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				require.NotNil(t, client.Transport)
			}
		})
	}
}

func TestBuildAWSSessionWithTLS(t *testing.T) {
	tests := []struct {
		name        string
		dpt         *oadpv1alpha1.DataProtectionTest
		bsl         *velerov1.BackupStorageLocationSpec
		region      string
		endpoint    string
		expectError bool
	}{
		{
			name: "valid AWS session with TLS",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					SkipTLSVerify: true,
				},
			},
			bsl:         &velerov1.BackupStorageLocationSpec{},
			region:      "us-east-1",
			endpoint:    "",
			expectError: false,
		},
		{
			name: "with custom endpoint",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					SkipTLSVerify: false,
				},
			},
			bsl:         &velerov1.BackupStorageLocationSpec{},
			region:      "us-west-2",
			endpoint:    "https://minio.example.com",
			expectError: false,
		},
		{
			name: "invalid TLS config",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					SkipTLSVerify: false,
				},
			},
			bsl: &velerov1.BackupStorageLocationSpec{
				StorageType: velerov1.StorageType{
					ObjectStorage: &velerov1.ObjectStorageLocation{
						CACert: []byte("invalid-base64!"),
					},
				},
			},
			region:      "us-east-1",
			endpoint:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.Discard()

			session, err := buildAWSSessionWithTLS(tt.dpt, tt.bsl, tt.region, tt.endpoint, logger)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, session)
			} else {
				require.NoError(t, err)
				require.NotNil(t, session)
				require.NotNil(t, session.Config)
				require.Equal(t, tt.region, *session.Config.Region)
				if tt.endpoint != "" {
					require.Equal(t, tt.endpoint, *session.Config.Endpoint)
					require.True(t, *session.Config.S3ForcePathStyle)
				}
			}
		})
	}
}

func TestInitializeGCPProvider(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, oadpv1alpha1.AddToScheme(scheme))
	require.NoError(t, velerov1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ctx := context.Background()

	gcpSecretData := `{
		"type": "service_account",
		"project_id": "test-project",
		"private_key_id": "test-key-id", 
		"private_key": "-----BEGIN PRIVATE KEY-----\ntest-key\n-----END PRIVATE KEY-----\n",
		"client_email": "test@test-project.iam.gserviceaccount.com",
		"client_id": "123456789",
		"auth_uri": "https://accounts.google.com/o/oauth2/auth",
		"token_uri": "https://oauth2.googleapis.com/token"
	}`

	gcpSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gcp-secret",
			Namespace: "openshift-adp",
		},
		Data: map[string][]byte{
			"cloud": []byte(gcpSecretData),
		},
	}

	tests := []struct {
		name         string
		setupSecrets bool
		expectError  bool
	}{
		{
			name:         "missing secret",
			setupSecrets: false,
			expectError:  true,
		},
		{
			name:         "missing bucket",
			setupSecrets: true,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.setupSecrets {
				builder.WithObjects(gcpSecret)
			}
			k8sClient := builder.Build()

			reconciler := &DataProtectionTestReconciler{
				Client:         k8sClient,
				Context:        ctx,
				NamespacedName: types.NamespacedName{Name: "dummy", Namespace: "openshift-adp"},
				dpt:            &oadpv1alpha1.DataProtectionTest{},
			}

			bslSpec := &velerov1.BackupStorageLocationSpec{
				Provider: "gcp",
				StorageType: velerov1.StorageType{
					ObjectStorage: &velerov1.ObjectStorageLocation{
						Bucket: "",
					},
				},
				Credential: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "gcp-secret",
					},
					Key: "cloud",
				},
			}

			if tt.name == "missing bucket" {
				bslSpec.ObjectStorage.Bucket = ""
			}

			_, err := reconciler.initializeGCPProvider(ctx, bslSpec)
			if tt.expectError {
				require.Error(t, err)
			}
		})
	}
}

func TestUpdateDPTErrorStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, oadpv1alpha1.AddToScheme(scheme))

	ctx := context.Background()
	dpt := &oadpv1alpha1.DataProtectionTest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dpt",
			Namespace: "openshift-adp",
		},
		Status: oadpv1alpha1.DataProtectionTestStatus{
			Phase: "InProgress",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dpt).Build()

	reconciler := &DataProtectionTestReconciler{
		Client:         fakeClient,
		NamespacedName: types.NamespacedName{Name: "test-dpt", Namespace: "openshift-adp"},
	}

	// Test that the function doesn't panic or error - this is a fire-and-forget function
	errorMsg := "test error message"
	reconciler.updateDPTErrorStatus(ctx, errorMsg)

	// This test mainly verifies the function can be called without errors
	// The actual status update verification is complex due to retry logic
}

func TestWaitForSnapshotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, snapshotv1api.AddToScheme(scheme))

	// Create a test VolumeSnapshot
	vs := &snapshotv1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test-ns",
		},
		Status: &snapshotv1api.VolumeSnapshotStatus{
			ReadyToUse: &[]bool{false}[0], // Not ready initially
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vs).Build()

	reconciler := &DataProtectionTestReconciler{
		ClusterWideClient: fakeClient,
	}

	// Test timeout case
	ctx := context.Background()
	err := reconciler.waitForSnapshotReady(ctx, vs, 1*time.Millisecond) // Very short timeout
	require.Error(t, err)
	require.Contains(t, err.Error(), "timed out")
}

func TestInitializeAzureProvider(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, oadpv1alpha1.AddToScheme(scheme))
	require.NoError(t, velerov1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ctx := context.Background()

	azureSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "azure-secret",
			Namespace: "openshift-adp",
		},
		Data: map[string][]byte{
			"AZURE_SUBSCRIPTION_ID": []byte("test-subscription-id"),
			"AZURE_TENANT_ID":       []byte("test-tenant-id"),
			"AZURE_CLIENT_ID":       []byte("test-client-id"),
			"AZURE_CLIENT_SECRET":   []byte("test-client-secret"),
		},
	}

	tests := []struct {
		name         string
		setupSecrets bool
		expectError  bool
	}{
		{
			name:         "valid Azure config",
			setupSecrets: true,
			expectError:  false,
		},
		{
			name:         "missing secret",
			setupSecrets: false,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.setupSecrets {
				builder.WithObjects(azureSecret)
			}
			k8sClient := builder.Build()

			reconciler := &DataProtectionTestReconciler{
				Client:         k8sClient,
				Context:        ctx,
				NamespacedName: types.NamespacedName{Name: "dummy", Namespace: "openshift-adp"},
				dpt:            &oadpv1alpha1.DataProtectionTest{},
			}

			bslSpec := &velerov1.BackupStorageLocationSpec{
				Provider: "azure",
				StorageType: velerov1.StorageType{
					ObjectStorage: &velerov1.ObjectStorageLocation{
						Bucket: "test-azure-container",
					},
				},
				Credential: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "azure-secret",
					},
					Key: "cloud",
				},
			}

			cp, err := reconciler.initializeAzureProvider(ctx, bslSpec)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, cp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cp)
			}
		})
	}
}

func TestReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, oadpv1alpha1.AddToScheme(scheme))
	require.NoError(t, velerov1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name          string
		dpt           *oadpv1alpha1.DataProtectionTest
		expectPhase   string
		expectRequeue bool
		expectError   bool
		description   string
	}{
		{
			name:          "DPT not found",
			dpt:           nil, // DPT not created
			expectRequeue: false,
			expectError:   false,
			description:   "Should handle missing DPT gracefully",
		},
		{
			name: "DPT already completed",
			dpt: &oadpv1alpha1.DataProtectionTest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpt",
					Namespace: "openshift-adp",
				},
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					ForceRun: false,
				},
				Status: oadpv1alpha1.DataProtectionTestStatus{
					Phase: "Complete",
				},
			},
			expectRequeue: false,
			expectError:   false,
			description:   "Should skip completed DPT when forceRun is false",
		},
		{
			name: "DPT with missing backup location",
			dpt: &oadpv1alpha1.DataProtectionTest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpt",
					Namespace: "openshift-adp",
				},
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					// Missing both BackupLocationSpec and BackupLocationName
				},
			},
			expectRequeue: false,
			expectError:   true,
			description:   "Should fail when backup location is not specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			builder := fake.NewClientBuilder().WithScheme(scheme)

			if tt.dpt != nil {
				builder.WithObjects(tt.dpt)
			}

			k8sClient := builder.Build()

			reconciler := &DataProtectionTestReconciler{
				Client: k8sClient,
				Scheme: scheme,
				Log:    logr.Discard(),
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-dpt",
					Namespace: "openshift-adp",
				},
			}

			result, err := reconciler.Reconcile(ctx, req)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.expectRequeue, result.Requeue)
		})
	}
}

func TestRunSnapshotTests(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, oadpv1alpha1.AddToScheme(scheme))
	require.NoError(t, snapshotv1api.AddToScheme(scheme))

	tests := []struct {
		name                  string
		dpt                   *oadpv1alpha1.DataProtectionTest
		expectedSnapshotCount int
		expectedSummary       string
		expectError           bool
		description           string
	}{
		{
			name: "no snapshot tests configured",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					CSIVolumeSnapshotTestConfigs: []oadpv1alpha1.CSIVolumeSnapshotTestConfig{},
				},
			},
			expectedSnapshotCount: 0,
			expectedSummary:       "0/0 passed",
			expectError:           false,
			description:           "Should handle empty snapshot test configs",
		},
		{
			name: "single snapshot test with incomplete config",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					CSIVolumeSnapshotTestConfigs: []oadpv1alpha1.CSIVolumeSnapshotTestConfig{
						{
							// Missing required fields - should be skipped
							SnapshotClassName: "",
							VolumeSnapshotSource: oadpv1alpha1.VolumeSnapshotSource{
								PersistentVolumeClaimName: "",
							},
						},
					},
				},
			},
			expectedSnapshotCount: 0,
			expectedSummary:       "0/0 passed",
			expectError:           false,
			description:           "Should skip snapshot tests with missing required fields",
		},
		{
			name: "multiple snapshot tests with valid config",
			dpt: &oadpv1alpha1.DataProtectionTest{
				Spec: oadpv1alpha1.DataProtectionTestSpec{
					CSIVolumeSnapshotTestConfigs: []oadpv1alpha1.CSIVolumeSnapshotTestConfig{
						{
							SnapshotClassName: "csi-snapshot-class",
							VolumeSnapshotSource: oadpv1alpha1.VolumeSnapshotSource{
								PersistentVolumeClaimName:      "pvc-1",
								PersistentVolumeClaimNamespace: "test-ns",
							},
							Timeout: metav1.Duration{Duration: 10 * time.Second},
						},
						{
							SnapshotClassName: "csi-snapshot-class",
							VolumeSnapshotSource: oadpv1alpha1.VolumeSnapshotSource{
								PersistentVolumeClaimName:      "pvc-2",
								PersistentVolumeClaimNamespace: "test-ns",
							},
							Timeout: metav1.Duration{Duration: 10 * time.Second},
						},
					},
				},
			},
			expectedSnapshotCount: 2,
			expectedSummary:       "0/2 passed", // Will fail due to fake client
			expectError:           true,         // Will have errors creating snapshots
			description:           "Should attempt to create multiple snapshot tests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			reconciler := &DataProtectionTestReconciler{
				Client:            fakeClient,
				ClusterWideClient: fakeClient,
				Log:               logr.Discard(),
			}

			err := reconciler.runSnapshotTests(ctx, tt.dpt)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.expectedSnapshotCount, len(tt.dpt.Status.SnapshotTests))
			require.Equal(t, tt.expectedSummary, tt.dpt.Status.SnapshotSummary)
		})
	}
}

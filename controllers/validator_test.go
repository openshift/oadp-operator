package controllers

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

func TestDPAReconciler_ValidateDataProtectionCR(t *testing.T) {
	tests := []struct {
		name       string
		dpa        *oadpv1alpha1.DataProtectionApplication
		objects    []client.Object
		wantErr    bool
		messageErr string
	}{
		{
			name: "given valid DPA CR, no default backup location, no backup images, no error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: pointer.Bool(false),
				},
			},
			objects: []client.Object{},
			wantErr: false,
		},
		{
			name: "given valid DPA CR, no default backup location, no backup images, MTC type override, no error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: pointer.Bool(false),
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.OperatorTypeKey: oadpv1alpha1.OperatorTypeMTC,
					},
				},
			},
			objects: []client.Object{},
			wantErr: false,
		},
		{
			name: "given valid DPA CR, no default backup location, no backup images, notMTC type override, error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: pointer.Bool(false),
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.OperatorTypeKey: "not" + oadpv1alpha1.OperatorTypeMTC,
					},
				},
			},
			objects:    []client.Object{},
			wantErr:    true,
			messageErr: "only mtc operator type override is supported",
		},
		{
			name: "given valid DPA CR, no default backup location, backup images cannot be nil, error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
							NoDefaultBackupLocation: true,
						},
					},
				},
			},
			objects:    []client.Object{},
			wantErr:    true,
			messageErr: "backupImages needs to be set to false when noDefaultBackupLocation is set",
		},
		{
			name: "given valid DPA CR, no default backup location, backup images cannot be true, error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: pointer.Bool(true),
				},
			},
			objects:    []client.Object{},
			wantErr:    true,
			messageErr: "backupImages needs to be set to false when noDefaultBackupLocation is set",
		},
		{
			name: "given invalid DPA CR, velero configuration is nil, error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
				},
			},
			wantErr:    true,
			messageErr: "no backupstoragelocations configured, ensure a backupstoragelocation has been configured or use the noDefaultBackupLocation flag",
		},
		{
			name: "given valid DPA CR, no BSL configured and noDefaultBackupLocation flag is set, error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
				},
			},
			wantErr:    true,
			messageErr: "no backupstoragelocations configured, ensure a backupstoragelocation has been configured or use the noDefaultBackupLocation flag",
		},
		{
			name: "given valid DPA CR bucket BSL configured with creds and AWS Default Plugin",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "testing",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "credentials",
								},
								Default: true,
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
					BackupImages: pointer.Bool(false),
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": []byte("dummy_data")},
				},
			},
			wantErr: false,
		},
		{
			name: "given valid DPA CR with valid velero resource requirements ",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "testing",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "credentials",
								},
								Default: true,
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
							PodConfig: &oadpv1alpha1.PodConfig{
								ResourceAllocations: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("2"),
									},
								},
							},
						},
					},
					BackupImages: pointer.Bool(false),
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": []byte("dummy_data")},
				},
			},
			wantErr: false,
		},
		{
			name: "given valid DPA CR with valid restic resource requirements ",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "testing",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "credentials",
								},
								Default: true,
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
							PodConfig: &oadpv1alpha1.PodConfig{
								ResourceAllocations: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("2"),
									},
								},
							},
						},
						Restic: &oadpv1alpha1.ResticConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									ResourceAllocations: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("2"),
										},
									},
								},
							},
						},
					},
					BackupImages: pointer.Bool(false),
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": []byte("dummy_data")},
				},
			},
			wantErr: false,
		},
		{
			name: "given valid DPA CR bucket BSL configured with creds and VSL and AWS Default Plugin with no secret",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "testing",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "testing",
									},
									Key: "credentials",
								},
								Default: true,
							},
						},
					},
					SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &v1.VolumeSnapshotLocationSpec{
								Provider: "aws",
								Config: map[string]string{
									AWSRegion: "us-east-1",
								},
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
					BackupImages: pointer.Bool(false),
				},
			},
			objects:    []client.Object{},
			wantErr:    true,
			messageErr: "secrets \"testing\" not found",
		},
		{
			name: "given valid DPA CR bucket BSL configured with creds and VSL and AWS Default Plugin with no secret, with no-secrets feature enabled",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "testing",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "testing",
									},
									Key: "credentials",
								},
								Default: true,
							},
						},
					},
					SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &v1.VolumeSnapshotLocationSpec{
								Provider: "aws",
								Config: map[string]string{
									AWSRegion: "us-east-1",
								},
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							FeatureFlags: []string{"no-secret"},
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
					BackupImages: pointer.Bool(false),
				},
			},
			objects: []client.Object{},
			wantErr: false,
		},
		{
			name: "given invalid DPA CR bucket BSL configured and AWS Default Plugin with no secret",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "testing",
								},
							},
						},
					},
					SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &v1.VolumeSnapshotLocationSpec{
								Provider: "aws",
								Config: map[string]string{
									AWSRegion: "us-east-1",
								},
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
					BackupImages: pointer.Bool(false),
				},
			},
			objects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testing",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: "aws",
					},
				},
			},
			wantErr:    true,
			messageErr: "must provide a valid credential secret",
		},
		{
			name: "given valid DPA CR bucket BSL configured and AWS Default Plugin with secret",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "testing",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "credentials",
								},
								Default: true,
							},
						},
					},
					SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &v1.VolumeSnapshotLocationSpec{
								Provider: "aws",
								Config: map[string]string{
									AWSRegion: "us-east-1",
								},
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
					BackupImages: pointer.Bool(false),
				},
			},
			objects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testing",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: "aws",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": []byte("dummy_data"), "cloud": []byte("dummy_data")},
				},
			},
			wantErr: false,
		},
		{
			name: "given valid DPA CR BSL configured and GCP Default Plugin with secret",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/gcp",
								Credential: &corev1.SecretKeySelector{
									Key: "credentials",
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials-gcp",
									},
								},
								Default: true,
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginGCP,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials-gcp",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": []byte("dummy_data")},
				},
			},
			wantErr: false,
		},
		{
			name: "given valid DPA CR BSL configured and GCP Default Plugin without secret with no-secret feature flag",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/gcp",
								Default:  true,
							},
						},
					},

					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginGCP,
							},
							FeatureFlags: []string{"no-secret"},
						},
					},
				},
			},
			objects: []client.Object{},
			wantErr: false,
		},
		{
			name: "should error: given valid DPA CR BSL configured and GCP Default Plugin without secret without no-secret feature flag",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/gcp",
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginGCP,
							},
						},
					},
				},
			},
			objects:    []client.Object{},
			wantErr:    true,
			messageErr: "secrets \"\" not found",
		},
		{
			name: "given valid DPA CR VSL configured and GCP Default Plugin without secret",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &v1.VolumeSnapshotLocationSpec{
								Provider: "velerio.io/gcp",
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginGCP,
							},
						},
					},
				},
			},
			objects:    []client.Object{},
			wantErr:    true,
			messageErr: "no backupstoragelocations configured, ensure a backupstoragelocation has been configured or use the noDefaultBackupLocation flag",
		},
		{
			name: "given valid DPA CR AWS Default Plugin with credentials",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "Test",
									},
									Key:      "Creds",
									Optional: new(bool),
								},
								Config: map[string]string{
									"region": "us-east-1",
								},
								Default: true,
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "Test",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"Creds": []byte("dummy_data")},
				},
			},
			wantErr: false,
		},
		{
			name: "given valid DPA CR AWS Default Plugin with credentials and one without",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "Test",
									},
									Key:      "Creds",
									Optional: new(bool),
								},
								Config: map[string]string{
									"region": "us-east-1",
								},
							},
						},
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/aws",
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "Test",
						Namespace: "test-ns",
					},
				},
			},
			wantErr:    true,
			messageErr: "Secret name Test is missing data for key Creds",
		},
		{
			name: "given valid DPA CR AWS Default Plugin with credentials and a VSL, and default secret specified, passes",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key:      "cloud",
									Optional: new(bool),
								},
								Config: map[string]string{
									"region": "us-east-1",
								},
								Default: true,
							},
						},
					},
					SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &v1.VolumeSnapshotLocationSpec{
								Provider: "aws",
								Config: map[string]string{
									AWSRegion: "us-east-1",
								},
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"cloud": []byte("dummy_data")},
				},
			},
			wantErr: false,
		},
		{
			name: "given valid DPA CR AWS Default Plugin with credentials and a VSL, and without secret in cluster, fails",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key:      "cloud",
									Optional: new(bool),
								},
								Config: map[string]string{
									"region": "us-east-1",
								},
							},
						},
					},
					SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &v1.VolumeSnapshotLocationSpec{
								Provider: "velero.io/aws",
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			objects:    []client.Object{},
			wantErr:    true,
			messageErr: "secrets \"cloud-credentials\" not found",
		},
		{
			name: "given valid DPA CR AWS with VSL credentials referencing a non-existent secret",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key:      "cloud",
									Optional: new(bool),
								},
								Config: map[string]string{
									"region": "us-east-1",
								},
							},
						},
					},
					SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &v1.VolumeSnapshotLocationSpec{
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "bad-credentials",
									},
									Key:      "bad-key",
									Optional: new(bool),
								},
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
				},
			},
			wantErr:    true,
			messageErr: "Secret name cloud-credentials is missing data for key cloud",
		},
		{
			name: "given valid DPA CR AWS with BSL and VSL credentials referencing a custom secret",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "custom-bsl-credentials",
									},
									Key:      "cloud",
									Optional: new(bool),
								},
								Config: map[string]string{
									"region": "us-east-1",
								},
								Default: true,
							},
						},
					},
					SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &v1.VolumeSnapshotLocationSpec{
								Provider: "aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "custom-vsl-credentials",
									},
									Key:      "cloud",
									Optional: new(bool),
								},
								Config: map[string]string{
									"region": "us-east-1",
								},
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "custom-bsl-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"cloud": []byte("dummy_data")},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "custom-vsl-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"cloud": []byte("dummy_data")},
				},
			},
			wantErr: false,
		},
		{
			name: "If DPA CR has CloudStorageLocation without Prefix defined with backupImages enabled, error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "testing",
								},
								Prefix: "",
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{},
						},
					},
					BackupImages: pointer.Bool(true),
				},
			},
			objects:    []client.Object{},
			wantErr:    true,
			messageErr: "BackupLocation must have cloud storage prefix when backupImages is not set to false",
		},
		{
			name: "If DPA CR has CloudStorageLocation with Prefix defined with backupImages enabled, no error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "testing",
								},
								Prefix: "some-prefix",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "cloud",
								},
								Default: true,
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{},
						},
					},
					BackupImages: pointer.Bool(true),
				},
			},
			objects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testing",
						Namespace: "test-ns",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"cloud": []byte("dummy_data")},
				},
			},
			wantErr: false,
		},
		{
			name: "given invalid DPA CR, BSL secret key name not match the secret key name, error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key:      "no-match-key",
									Optional: new(bool),
								},
								Config: map[string]string{
									"region": "us-east-1",
								},
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": []byte("dummy_data")},
				},
			},
			wantErr:    true,
			messageErr: "Secret name cloud-credentials is missing data for key no-match-key",
		},
		{
			name: "given invalid DPA CR, BSL secret is missing data, error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key:      "credentials",
									Optional: new(bool),
								},
								Config: map[string]string{
									"region": "us-east-1",
								},
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": []byte("")},
				},
			},
			wantErr:    true,
			messageErr: "Secret name cloud-credentials is missing data for key credentials",
		},
		{
			name: "given invalid DPA CR, BSL secret key is empty, error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key:      "",
									Optional: new(bool),
								},
								Config: map[string]string{
									"region": "us-east-1",
								},
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": []byte("dummy_data")},
				},
			},
			wantErr:    true,
			messageErr: "Secret key specified in BackupLocation  cannot be empty",
		},
		{
			name: "given invalid DPA CR, BSL secret name is empty, error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								StorageType: v1.StorageType{
									ObjectStorage: &v1.ObjectStorageLocation{
										Bucket: "test-bucket",
										Prefix: "test-prefix",
									},
								},
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "",
									},
									Key:      "credentials",
									Optional: new(bool),
								},
								Config: map[string]string{
									"region": "us-east-1",
								},
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": []byte("dummy_data")},
				},
			},
			wantErr:    true,
			messageErr: "Secret name specified in BackupLocation  cannot be empty",
		},
		{
			name: "given invalid DPA CR tech-preview-ack not set as true but non-admin is enabled error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: pointer.Bool(true),
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.TechPreviewAck: "false",
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: pointer.Bool(false),
				},
			},
			objects:    []client.Object{},
			wantErr:    true,
			messageErr: "in order to enable/disable the non-admin feature please set dpa.spec.unsupportedOverrides[tech-preview-ack]: 'true'",
		},
		{
			name: "given invalid DPA CR aws and legacy-aws plugins both specified",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
								oadpv1alpha1.DefaultPluginLegacyAWS,
							},
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: pointer.Bool(false),
				},
			},
			objects:    []client.Object{},
			wantErr:    true,
			messageErr: "aws and legacy-aws can not be both specified in DPA spec.configuration.velero.defaultPlugins",
		},
	}
	for _, tt := range tests {
		tt.objects = append(tt.objects, tt.dpa)
		fakeClient, err := getFakeClientFromObjects(tt.objects...)
		if err != nil {
			t.Errorf("error in creating fake client, likely programmer error")
		}
		r := &DPAReconciler{
			Client:  fakeClient,
			Scheme:  fakeClient.Scheme(),
			Log:     logr.Discard(),
			Context: newContextForTest(tt.name),
			NamespacedName: types.NamespacedName{
				Namespace: tt.dpa.Namespace,
				Name:      tt.dpa.Name,
			},
			dpa:           tt.dpa,
			EventRecorder: record.NewFakeRecorder(10),
		}
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.ValidateDataProtectionCR(r.Log)
			if err != nil && !tt.wantErr {
				t.Errorf("ValidateDataProtectionCR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && err.Error() != tt.messageErr {
				t.Errorf("Error messages are not the same: got %v, expected %v", err.Error(), tt.messageErr)
				return
			}
			if got != !tt.wantErr {
				t.Errorf("ValidateDataProtectionCR() got = %v, want %v", got, !tt.wantErr)
			}
		})
	}
}

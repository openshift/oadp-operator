package controller

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
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
			name: "[invalid] DPA CR: multiple DPAs in same namespace",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{},
			},
			objects: []client.Object{
				&oadpv1alpha1.DataProtectionApplication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "another-DPA-CR",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{},
				},
			},
			wantErr:    true,
			messageErr: "only one DPA CR can exist per OADP installation namespace",
		},
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
					Data: map[string][]byte{"credentials": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
				},
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
					Data: map[string][]byte{"credentials": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
				},
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
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									ResourceAllocations: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("2"),
										},
									},
								},
							},
							UploaderType: "restic",
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
					Data: map[string][]byte{"credentials": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
				},
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
							Velero: &velerov1.VolumeSnapshotLocationSpec{
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
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"cloud": []byte("ss"),
					},
				},
			},
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
							Velero: &velerov1.VolumeSnapshotLocationSpec{
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
							Velero: &velerov1.VolumeSnapshotLocationSpec{
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
							Velero: &velerov1.VolumeSnapshotLocationSpec{
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
					Data: map[string][]byte{"credentials": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"), "cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
					Data: map[string][]byte{"credentials": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
							Velero: &velerov1.VolumeSnapshotLocationSpec{
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
					Data: map[string][]byte{"Creds": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
			messageErr: "secret name Test is missing data for key Creds",
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
							Velero: &velerov1.VolumeSnapshotLocationSpec{
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
					Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
							Velero: &velerov1.VolumeSnapshotLocationSpec{
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
							Velero: &velerov1.VolumeSnapshotLocationSpec{
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
			messageErr: "secret name cloud-credentials is missing data for key cloud",
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
							Velero: &velerov1.VolumeSnapshotLocationSpec{
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
					Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "custom-vsl-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "cloud-credentials"},
								},
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
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
				},
			},
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
					Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
					Data: map[string][]byte{"credentials": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
				},
			},
			wantErr:    true,
			messageErr: "secret name cloud-credentials is missing data for key no-match-key",
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
			messageErr: "secret name cloud-credentials is missing data for key credentials",
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
					Data: map[string][]byte{"credentials": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
				},
			},
			wantErr:    true,
			messageErr: "secret key specified for location cannot be empty",
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
							Velero: &velerov1.BackupStorageLocationSpec{
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
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
					Data: map[string][]byte{"credentials": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
				},
			},
			wantErr:    true,
			messageErr: "secret name specified for location cannot be empty",
		},
		{
			name: "[valid] DPA CR: spec.nonAdmin.enable true",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: pointer.Bool(true),
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
		},
		{
			name: "[invalid] DPA CR: multiple NACs in cluster",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: pointer.Bool(true),
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
			objects: []client.Object{
				&oadpv1alpha1.DataProtectionApplication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "another-DPA-CR",
						Namespace: "test-another-ns",
					},
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						NonAdmin: &oadpv1alpha1.NonAdmin{
							Enable: pointer.Bool(true),
						},
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "non-admin-controller",
						Namespace: "test-another-ns",
					},
				},
			},
			wantErr:    true,
			messageErr: "only a single instance of Non-Admin Controller can be installed across the entire cluster. Non-Admin controller is already configured and installed in test-another-ns namespace",
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
		{
			name: "[valid] DPA CR: spec.nonAdmin.enforceBackupSpec set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						EnforceBackupSpec: &velerov1.BackupSpec{
							SnapshotVolumes: ptr.To(false),
						},
					},
				},
			},
		},
		{
			name: "[Invalid] DPA CR: spec.nonAdmin.enforceBackupSpec.storageLocation set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						EnforceBackupSpec: &velerov1.BackupSpec{
							SnapshotVolumes: ptr.To(false),
							StorageLocation: "foo-bsl",
						},
					},
				},
			},
			wantErr:    true,
			messageErr: fmt.Sprintf(NACNonEnforceableErr, "spec.nonAdmin.enforcedBackupSpec.storageLocation"),
		},
		{
			name: "[Invalid] DPA CR: spec.nonAdmin.enforceBackupSpec.volumeSnapshotLocations set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						EnforceBackupSpec: &velerov1.BackupSpec{
							SnapshotVolumes:         ptr.To(false),
							VolumeSnapshotLocations: []string{"foo-vsl", "bar-vsl"},
						},
					},
				},
			},
			wantErr:    true,
			messageErr: fmt.Sprintf(NACNonEnforceableErr, "spec.nonAdmin.enforcedBackupSpec.volumeSnapshotLocations"),
		},
		{
			name: "[Invalid] DPA CR: spec.nonAdmin.enforceBackupSpec.includedNamespaces set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						EnforceBackupSpec: &velerov1.BackupSpec{
							IncludedNamespaces: []string{"banana"},
							SnapshotVolumes:    ptr.To(false),
						},
					},
				},
			},
			wantErr:    true,
			messageErr: fmt.Sprintf(NACNonEnforceableErr, "spec.nonAdmin.enforcedBackupSpec.includedNamespaces"),
		},
		{
			name: "[Invalid] DPA CR: spec.nonAdmin.enforceBackupSpec.excludedNamespaces set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						EnforceBackupSpec: &velerov1.BackupSpec{
							ExcludedNamespaces: []string{"banana"},
							SnapshotVolumes:    ptr.To(false),
						},
					},
				},
			},
			wantErr:    true,
			messageErr: fmt.Sprintf(NACNonEnforceableErr, "spec.nonAdmin.enforcedBackupSpec.excludedNamespaces"),
		},
		{
			name: "[Invalid] DPA CR: spec.nonAdmin.enforceBackupSpec.includeClusterResources set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						EnforceBackupSpec: &velerov1.BackupSpec{
							IncludeClusterResources: ptr.To(true),
						},
					},
				},
			},
			wantErr:    true,
			messageErr: fmt.Sprintf(NACNonEnforceableErr+" as true, must be set to false if enforced by admins", "spec.nonAdmin.enforcedBackupSpec.includeClusterResources"),
		},
		{
			name: "[Invalid] DPA CR: spec.nonAdmin.enforceBackupSpec.includedClusterScopedResources set as a non-empty list",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						EnforceBackupSpec: &velerov1.BackupSpec{
							IncludedClusterScopedResources: []string{"foo", "bar"},
						},
					},
				},
			},
			wantErr:    true,
			messageErr: fmt.Sprintf(NACNonEnforceableErr+" and must remain empty", "spec.nonAdmin.enforcedBackupSpec.includedClusterScopedResources"),
		},
		{
			name: "[valid] DPA CR: spec.nonAdmin.enforceRestoreSpec set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						EnforceRestoreSpec: &velerov1.RestoreSpec{
							RestorePVs: ptr.To(true),
						},
					},
				},
			},
		},
		{
			name: "[invalid] DPA CR: spec.nonAdmin.enforceRestoreSpec.scheduleName is set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						EnforceRestoreSpec: &velerov1.RestoreSpec{
							ScheduleName: "foo-schedule-set",
							RestorePVs:   ptr.To(true),
						},
					},
				},
			},
			wantErr:    true,
			messageErr: fmt.Sprintf(NACNonEnforceableErr, "spec.nonAdmin.enforcedRestoreSpec.scheduleName"),
		},
		{
			name: "[invalid] DPA CR: spec.nonAdmin.enforceRestoreSpec.includedNamespaces is set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						EnforceRestoreSpec: &velerov1.RestoreSpec{
							IncludedNamespaces: []string{"included-ns-foo"},
							RestorePVs:         ptr.To(true),
						},
					},
				},
			},
			wantErr:    true,
			messageErr: fmt.Sprintf(NACNonEnforceableErr, "spec.nonAdmin.enforcedRestoreSpec.includedNamespaces"),
		},
		{
			name: "[invalid] DPA CR: spec.nonAdmin.enforceRestoreSpec.excludedNamespaces is set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						EnforceRestoreSpec: &velerov1.RestoreSpec{
							ExcludedNamespaces: []string{"excluded-ns-foo"},
							RestorePVs:         ptr.To(true),
						},
					},
				},
			},
			wantErr:    true,
			messageErr: fmt.Sprintf(NACNonEnforceableErr, "spec.nonAdmin.enforcedRestoreSpec.excludedNamespaces"),
		},
		{
			name: "[invalid] DPA CR: spec.nonAdmin.enforceRestoreSpec.namespaceMapping is set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						EnforceRestoreSpec: &velerov1.RestoreSpec{
							NamespaceMapping: map[string]string{
								"foo-ns-1": "bar-ns-2",
							},
							RestorePVs: ptr.To(true),
						},
					},
				},
			},
			wantErr:    true,
			messageErr: fmt.Sprintf(NACNonEnforceableErr, "spec.nonAdmin.enforcedRestoreSpec.namespaceMapping"),
		},
		{
			name: "[valid] DPA CR: spec.nonAdmin.enforceBSLSpec set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						EnforceBSLSpec: &oadpv1alpha1.EnforceBackupStorageLocationSpec{
							Provider: "foo-provider",
						},
					},
				},
			},
		},
		{
			name: "[valid] DPA CR: spec.nonAdmin.enable true",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "[invalid] DPA CR: spec.nonAdmin.garbageCollectionPeriod negative",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						GarbageCollectionPeriod: &metav1.Duration{
							Duration: -3 * time.Hour,
						},
					},
				},
			},
			wantErr:    true,
			messageErr: "DPA spec.nonAdmin.garbageCollectionPeriod can not be negative",
		},
		{
			name: "[invalid] DPA CR: spec.nonAdmin.backupSyncPeriod negative",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						BackupSyncPeriod: &metav1.Duration{
							Duration: -5 * time.Minute,
						},
					},
				},
			},
			wantErr:    true,
			messageErr: "DPA spec.nonAdmin.backupSyncPeriod can not be negative",
		},
		{
			name: "[invalid] DPA CR: spec.nonAdmin.backupSyncPeriod greater than spec.nonAdmin.garbageCollectionPeriod",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						GarbageCollectionPeriod: &metav1.Duration{
							Duration: 1 * time.Minute,
						},
					},
				},
			},
			wantErr:    true,
			messageErr: "DPA spec.nonAdmin.backupSyncPeriod (2m0s) can not be greater or equal spec.nonAdmin.garbageCollectionPeriod (1m0s)",
		},
		{
			name: "[invalid] DPA CR: spec.nonAdmin.enforcedBSLSpec.backupSyncPeriod equal to spec.nonAdmin.backupSyncPeriod",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						BackupSyncPeriod: &metav1.Duration{
							Duration: 3 * time.Minute,
						},
						EnforceBSLSpec: &oadpv1alpha1.EnforceBackupStorageLocationSpec{
							BackupSyncPeriod: &metav1.Duration{
								Duration: 3 * time.Minute,
							},
						},
					},
				},
			},
			wantErr:    true,
			messageErr: "DPA spec.nonAdmin.enforcedBSLSpec.backupSyncPeriod (3m0s) can not be greater or equal DPA spec.nonAdmin.backupSyncPeriod (3m0s)",
		},
		{
			name: "[valid] DPA CR: spec.nonAdmin.enforcedBSLSpec.backupSyncPeriod lower than spec.nonAdmin.backupSyncPeriod",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						BackupSyncPeriod: &metav1.Duration{
							Duration: 20 * time.Minute,
						},
						EnforceBSLSpec: &oadpv1alpha1.EnforceBackupStorageLocationSpec{
							BackupSyncPeriod: &metav1.Duration{
								Duration: 10 * time.Minute,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "[invalid] DPA CR: spec.nonAdmin.enforcedBSLSpec.backupSyncPeriod greater than spec.nonAdmin.backupSyncPeriod",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
					BackupImages: ptr.To(false),
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: ptr.To(true),
						BackupSyncPeriod: &metav1.Duration{
							Duration: 15 * time.Minute,
						},
						EnforceBSLSpec: &oadpv1alpha1.EnforceBackupStorageLocationSpec{
							BackupSyncPeriod: &metav1.Duration{
								Duration: 16 * time.Minute,
							},
						},
					},
				},
			},
			wantErr:    true,
			messageErr: "DPA spec.nonAdmin.enforcedBSLSpec.backupSyncPeriod (16m0s) can not be greater or equal DPA spec.nonAdmin.backupSyncPeriod (15m0s)",
		},
		{
			name: "[valid] Both PodConfig and LoadAffinityConfig are identical - single node selector",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: ptr.To(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									NodeSelector: map[string]string{"key1": "value1"},
								},
							},
							NodeAgentConfigMapSettings: oadpv1alpha1.NodeAgentConfigMapSettings{
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"key1": "value1"},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "[valid] Both PodConfig and LoadAffinityConfig are identical - multiple node selectors, different order",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: ptr.To(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									NodeSelector: map[string]string{"key2": "value2", "key1": "value1"},
								},
							},
							NodeAgentConfigMapSettings: oadpv1alpha1.NodeAgentConfigMapSettings{
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"key1": "value1", "key2": "value2"},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "[invalid] All Labels from the LoadAffinityConfig are present in the PodConfig, and the PodConfig has more labels than the LoadAffinityConfig",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: ptr.To(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									NodeSelector: map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
								},
							},
							NodeAgentConfigMapSettings: oadpv1alpha1.NodeAgentConfigMapSettings{
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"key1": "value1", "key2": "value2"},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr:    true,
			messageErr: "when spec.configuration.nodeAgent.PodConfig is set, all labels from the spec.configuration.nodeAgent.PodConfig must be present in spec.configuration.nodeAgent.LoadAffinityConfig",
		},
		{
			name: "[valid] All Labels from the PodConfig are present in the LoadAffinityConfig, and the LoadAffinityConfig has more labels than the PodConfig",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: ptr.To(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									NodeSelector: map[string]string{"key1": "value1", "key2": "value2"},
								},
							},
							NodeAgentConfigMapSettings: oadpv1alpha1.NodeAgentConfigMapSettings{
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "[invalid] When PodConfig is specified, the LoadAffinityConfig must not specify MatchExpressions",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: ptr.To(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									NodeSelector: map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
								},
							},
							NodeAgentConfigMapSettings: oadpv1alpha1.NodeAgentConfigMapSettings{
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"key1": "value1", "key2": "value2"},
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      "key3",
													Operator: metav1.LabelSelectorOpIn,
													Values:   []string{"value3"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr:    true,
			messageErr: "when spec.configuration.nodeAgent.PodConfig is set, spec.configuration.nodeAgent.LoadAffinityConfig must not define matchExpressions",
		},
		{
			name: "[invalid] PodConfig and LoadAffinityConfig are different - single node selector",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: ptr.To(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									NodeSelector: map[string]string{"key3": "value3"},
								},
							},
							NodeAgentConfigMapSettings: oadpv1alpha1.NodeAgentConfigMapSettings{
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"key3": "value4"},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr:    true,
			messageErr: "when spec.configuration.nodeAgent.PodConfig is set, all labels from the spec.configuration.nodeAgent.PodConfig must be present in spec.configuration.nodeAgent.LoadAffinityConfig",
		},
		{
			name: "[invalid] PodConfig and LoadAffinityConfig are different - multiple node selectors",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: ptr.To(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									NodeSelector: map[string]string{"key3": "value3", "key4": "value4"},
								},
							},
							NodeAgentConfigMapSettings: oadpv1alpha1.NodeAgentConfigMapSettings{
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"key3": "value3", "key4": "value5"},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr:    true,
			messageErr: "when spec.configuration.nodeAgent.PodConfig is set, all labels from the spec.configuration.nodeAgent.PodConfig must be present in spec.configuration.nodeAgent.LoadAffinityConfig",
		},
		{
			name: "[valid] PodConfig is a subset of the LoadAffinityConfig",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: ptr.To(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									NodeSelector: map[string]string{"key3": "value3"},
								},
							},
							NodeAgentConfigMapSettings: oadpv1alpha1.NodeAgentConfigMapSettings{
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"key3": "value3", "key4": "value4"},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "[invalid] PodConfig and LoadAffinityConfig with no match labels",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: ptr.To(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									NodeSelector: map[string]string{"key3": "value3"},
								},
							},
							NodeAgentConfigMapSettings: oadpv1alpha1.NodeAgentConfigMapSettings{
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{{}},
							},
						},
					},
				},
			},
			wantErr:    true,
			messageErr: "when spec.configuration.nodeAgent.PodConfig is set, spec.configuration.nodeAgent.LoadAffinityConfig must define matchLabels",
		},
		{
			name: "[valid] Only PodConfig is set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: ptr.To(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									NodeSelector: map[string]string{"key2": "value2", "key1": "value1"},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "[valid] Only LoadAffinityConfig is set with MatchExpressions",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: ptr.To(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentConfigMapSettings: oadpv1alpha1.NodeAgentConfigMapSettings{
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"key1": "value1", "key2": "value2"},
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      "key3",
													Operator: metav1.LabelSelectorOpIn,
													Values:   []string{"value3"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "[valid] Only LoadAffinityConfig is set with multiple node selectors",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: ptr.To(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentConfigMapSettings: oadpv1alpha1.NodeAgentConfigMapSettings{
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"key1": "value1", "key2": "value2"},
										},
									},
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"key3": "value3", "key4": "value4"},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt.objects = append(tt.objects, tt.dpa)
		fakeClient, err := getFakeClientFromObjects(tt.objects...)
		if err != nil {
			t.Errorf("error in creating fake client, likely programmer error")
		}
		r := &DataProtectionApplicationReconciler{
			Client:            fakeClient,
			ClusterWideClient: fakeClient,
			Scheme:            fakeClient.Scheme(),
			Log:               logr.Discard(),
			Context:           newContextForTest(),
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

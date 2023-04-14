package controllers

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func getSchemeForFakeClient() (*runtime.Scheme, error) {
	err := oadpv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	err = velerov1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	return scheme.Scheme, nil
}

func getFakeClientFromObjects(objs ...client.Object) (client.WithWatch, error) {
	schemeForFakeClient, err := getSchemeForFakeClient()
	if err != nil {
		return nil, err
	}

	return fake.NewClientBuilder().WithScheme(schemeForFakeClient).WithObjects(objs...).Build(), nil
}

func TestDPAReconciler_ValidateBackupStorageLocations(t *testing.T) {
	tests := []struct {
		name    string
		dpa     *oadpv1alpha1.DataProtectionApplication
		secret  *corev1.Secret
		want    bool
		wantErr bool
	}{
		{
			name: "test no BSLs, no NoDefaultBackupLocation",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test no BSLs, with NoDefaultBackupLocation",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
				},
			},
			want:    true,
			wantErr: false,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								Config: map[string]string{
									Region: "us-east-1",
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, invalid provider",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "foo",
								Config: map[string]string{
									Region: "us-east-1",
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, aws configured but no provider specified",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Config: map[string]string{
									Region: "us-east-1",
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, aws configured appropriately but no aws credentials are incorrect",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								Config: map[string]string{
									Region: "us-east-1",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "aws-creds",
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, aws configured appropriately but no object storage configuration",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								Config: map[string]string{
									Region: "us-east-1",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, aws configured appropriately but no bucket specified",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "",
									},
								},
								Config: map[string]string{
									Region: "us-east-1",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, aws configured for image backup, but no region or prefix is specified",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-aws-bucket",
									},
								},
								Config: map[string]string{
									Region: "",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, aws configured for image backup with region specified, but no prefix is specified",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-aws-bucket",
									},
								},
								Config: map[string]string{
									Region: "us-east-1",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, aws configured properly for image backup with region and prefix specified",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-aws-bucket",
										Prefix: "test-prefix",
									},
								},
								Config: map[string]string{
									Region: "us-east-1",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    true,
			wantErr: false,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, azure configured appropriately but no resource group is specified",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "azure",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-azure-bucket",
									},
								},
								Config: map[string]string{
									ResourceGroup: "",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, azure configured appropriately but no storage account is specified",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "azure",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-azure-bucket",
									},
								},
								Config: map[string]string{
									ResourceGroup:  "test-rg",
									StorageAccount: "",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, gcp configured appropriately but no bucket is specified",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "gcp",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{},
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, aws configured appropriately, no error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-aws-bucket",
										Prefix: "velero",
									},
								},
								Config: map[string]string{
									Region: "test-region",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    true,
			wantErr: false,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, prefix not present for aws BSL",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-aws-bucket",
									},
								},
								Config: map[string]string{
									Region: "test-region",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, prefix not present for gcp BSL",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "gcp",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-gcp-bucket",
									},
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, prefix not present for azure BSL",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "azure",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-azure-bucket",
									},
								},
								Config: map[string]string{
									ResourceGroup:  "test-rg",
									StorageAccount: "test-sa",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, multiple appropriate BSLs configured, no error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-aws-bucket",
										Prefix: "velero",
									},
								},
								Config: map[string]string{
									Region: "test-region",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "azure",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-azure-bucket",
										Prefix: "velero",
									},
								},
								Config: map[string]string{
									ResourceGroup:  "test-rg",
									StorageAccount: "test-sa",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "gcp",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-gcp-bucket",
										Prefix: "velero",
									},
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    true,
			wantErr: false,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name:    "test get error",
			dpa:     &oadpv1alpha1.DataProtectionApplication{},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSL specified, with both bucket and velero",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-aws-bucket",
										Prefix: "velero",
									},
								},
								Config: map[string]string{
									Region: "test-region",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef:  corev1.LocalObjectReference{},
								Config:           map[string]string{},
								Credential:       &corev1.SecretKeySelector{},
								Default:          false,
								BackupSyncPeriod: &metav1.Duration{},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSL specified, bucket with no name",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef:  corev1.LocalObjectReference{},
								Config:           map[string]string{},
								Credential:       &corev1.SecretKeySelector{},
								Default:          false,
								BackupSyncPeriod: &metav1.Duration{},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSL specified, bucket with no credential",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "testing",
								},
								Config:           map[string]string{},
								Credential:       nil,
								Default:          false,
								BackupSyncPeriod: &metav1.Duration{},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSL specified, bucket with no credential name",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "testing",
								},
								Config:           map[string]string{},
								Credential:       &corev1.SecretKeySelector{},
								Default:          false,
								BackupSyncPeriod: &metav1.Duration{},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "test BSLs specified, multiple appropriate BSLs configured, no error case with bucket",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-aws-bucket",
										Prefix: "velero",
									},
								},
								Config: map[string]string{
									Region: "test-region",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "azure",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-azure-bucket",
										Prefix: "velero",
									},
								},
								Config: map[string]string{
									ResourceGroup:  "test-rg",
									StorageAccount: "test-sa",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "gcp",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-gcp-bucket",
										Prefix: "velero",
									},
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "testing",
								},
								Config: map[string]string{},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "testing",
								},
								BackupSyncPeriod: &metav1.Duration{},
							},
						},
					},
				},
			},
			want:    true,
			wantErr: false,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "BSL Region not set for aws provider without S3ForcePathStyle expect to fail",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-aws-bucket",
										Prefix: "test-prefix",
									},
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "BSL Region not set for aws provider without S3ForcePathStyle with BackupImages false expect to succeed",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: pointer.Bool(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "bucket",
										Prefix: "prefix",
									},
								},
							},
						},
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "BSL Region set for aws provider with S3ForcePathStyle expect to succeed",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								Config: map[string]string{
									S3ForcePathStyle: "true",
									Region:           "noobaa",
								},
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "bucket",
										Prefix: "prefix",
									},
								},
							},
						},
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "BSL Region not set for aws provider with S3ForcePathStyle expect to fail",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								Config: map[string]string{
									S3ForcePathStyle: "true",
								},
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "bucket",
										Prefix: "prefix",
									},
								},
							},
						},
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(tt.dpa, tt.secret)
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
				EventRecorder: record.NewFakeRecorder(10),
			}
			got, err := r.ValidateBackupStorageLocations(r.Log)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBackupStorageLocations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateBackupStorageLocations() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func newContextForTest(name string) context.Context {
	return context.TODO()
}

func TestDPAReconciler_updateBSLFromSpec(t *testing.T) {
	tests := []struct {
		name    string
		bsl     *velerov1.BackupStorageLocation
		dpa     *oadpv1alpha1.DataProtectionApplication
		wantErr bool
	}{
		{
			name: "BSL without owner reference and labels",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-1",
					Namespace: "bar",
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, err := getSchemeForFakeClient()
			if err != nil {
				t.Errorf("error getting scheme for the test: %#v", err)
			}
			r := &DPAReconciler{
				Scheme: scheme,
			}

			wantBSl := &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-1",
					Namespace: "bar",
					Labels: map[string]string{
						"app.kubernetes.io/name":     "oadp-operator-velero",
						"app.kubernetes.io/instance": tt.dpa.Name + "-1",
						//"app.kubernetes.io/version":    "x.y.z",
						"app.kubernetes.io/managed-by":       "oadp-operator",
						"app.kubernetes.io/component":        "bsl",
						oadpv1alpha1.OadpOperatorLabel:       "True",
						oadpv1alpha1.RegistryDeploymentLabel: "True",
					},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         oadpv1alpha1.SchemeBuilder.GroupVersion.String(),
						Kind:               "DataProtectionApplication",
						Name:               tt.dpa.Name,
						UID:                tt.dpa.UID,
						Controller:         pointer.BoolPtr(true),
						BlockOwnerDeletion: pointer.BoolPtr(true),
					}},
				},
			}

			err = r.updateBSLFromSpec(tt.bsl, tt.dpa, tt.bsl.Spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateBSLFromSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(tt.bsl.Labels, wantBSl.Labels) {
				t.Errorf("expected bsl labels to be %#v, got %#v", wantBSl.Labels, tt.bsl.Labels)
			}
			if !reflect.DeepEqual(tt.bsl.OwnerReferences, wantBSl.OwnerReferences) {
				t.Errorf("expected bsl owner references to be %#v, got %#v", wantBSl.OwnerReferences, tt.bsl.OwnerReferences)
			}
		})
	}
}

func TestDPAReconciler_ensureBackupLocationHasVeleroOrCloudStorage(t *testing.T) {
	tests := []struct {
		name    string
		dpa     *oadpv1alpha1.DataProtectionApplication
		wantErr bool
	}{
		{
			name: "one bsl configured per provider",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
							},
						},
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "azure",
							},
						},
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "gcp",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "wantErr: a bsl has both velero and cloudstorage configured",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
							},
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: v1.LocalObjectReference{
									Name: "foo",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "two bsl configured per provider",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
							},
						},
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
							},
						},
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "azure",
							},
						},
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "azure",
							},
						},
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "gcp",
							},
						},
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "gcp",
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, err := getSchemeForFakeClient()
			if err != nil {
				t.Errorf("error getting scheme for the test: %#v", err)
			}
			r := &DPAReconciler{
				Scheme: scheme,
			}
			if err := r.ensureBackupLocationHasVeleroOrCloudStorage(tt.dpa); (err != nil) != tt.wantErr {
				t.Errorf("ensureBSLProviderMapping() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDPAReconciler_ReconcileBackupStorageLocations(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloud-credentials",
			Namespace: "test-ns",
		},
	}
	cs := &oadpv1alpha1.CloudStorage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cs",
			Namespace: "test-ns",
		},
		Spec: oadpv1alpha1.CloudStorageSpec{
			CreationSecret: v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{
					Name: "cloud-credentials",
				},
				Key: "credentials",
			},
			Name:     "test-cs",
			Provider: "aws",
		},
	}

	tests := []struct {
		name    string
		dpa     *oadpv1alpha1.DataProtectionApplication
		secret  *corev1.Secret
		cs      *oadpv1alpha1.CloudStorage
		want    bool
		wantErr bool
	}{
		{
			name: "check owner references on Velero BSL",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
							},
						},
					},
				},
			},
			cs:      cs,
			secret:  secret,
			want:    true,
			wantErr: false,
		},
		{
			name: "check owner references on CloudStorage BSL",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: v1.LocalObjectReference{
									Name: "test-cs",
								},
								Credential: &v1.SecretKeySelector{
									LocalObjectReference: v1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "credentials",
								},
							},
						},
					},
				},
			},
			cs:      cs,
			secret:  secret,
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(tt.dpa, tt.secret, tt.cs)
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
				EventRecorder: record.NewFakeRecorder(10),
			}
			wantBSL := &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa-1",
					Namespace: "test-ns",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         oadpv1alpha1.SchemeBuilder.GroupVersion.String(),
						Kind:               "DataProtectionApplication",
						Name:               tt.dpa.Name,
						UID:                tt.dpa.UID,
						Controller:         pointer.BoolPtr(true),
						BlockOwnerDeletion: pointer.BoolPtr(true),
					}},
				},
			}
			got, err := r.ReconcileBackupStorageLocations(r.Log)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileBackupStorageLocations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ReconcileBackupStorageLocations() got = %v, want %v", got, tt.want)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileBackupStorageLocations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			bsl := &velerov1.BackupStorageLocation{}
			err = r.Get(r.Context, client.ObjectKey{Namespace: "test-ns", Name: "test-dpa-1"}, bsl)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileBackupStorageLocations() error =%v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(bsl.OwnerReferences, wantBSL.OwnerReferences) {
				t.Errorf("ReconcileBackupStorageLocations() expected BSL owner references to be %#v, got %#v", wantBSL.OwnerReferences, bsl.OwnerReferences)
			}
		})
	}
}

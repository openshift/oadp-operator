package controllers

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	configv1 "github.com/openshift/api/config/v1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/storage/aws"
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

// A bucket that region can be automatically discovered
const DiscoverableBucket string = "openshift-velero-plugin-s3-auto-region-test-1"

func getSchemeForFakeClient() (*runtime.Scheme, error) {
	err := oadpv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	err = velerov1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	err = configv1.AddToScheme((scheme.Scheme))
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
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{},
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
				Data: map[string][]byte{"cloud": []byte("dummy_data")},
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
									Key: "cloud",
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
				Data: map[string][]byte{"cloud": []byte("dummy_data")},
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
									Key: "cloud",
								},
								Default: true,
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
				Data: map[string][]byte{"cloud": []byte("dummy_data")},
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
									Key: "cloud",
								},
								Default: true,
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
				Data: map[string][]byte{"cloud": []byte("dummy_data")},
			},
		},
		{
			name: "test BSLs specified, no default set",
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
								Default: false,
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
									Key: "cloud",
								},
								Default: true,
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
									Key: "cloud",
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
									Key: "cloud",
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
				Data: map[string][]byte{"cloud": []byte("dummy_data")},
			},
		},
		{
			name: "test get error",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{},
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
									Key: "cloud",
								},
								Default: true,
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
									Key: "cloud",
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
									Key: "cloud",
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
									Key: "cloud",
								},
								Prefix:           "prefix",
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
				Data: map[string][]byte{"cloud": []byte("dummy_data")},
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
								Default: true,
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
				Data: map[string][]byte{"cloud": []byte("dummy_data")},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "BSL without config section for aws provider and default backupImages is true behavior",
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
										Bucket: DiscoverableBucket,
										Prefix: "prefix",
									},
								},
								Default: true,
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
		{
			name: "BSL with config section having only profile and s3ForcePathStyle is true for aws provider and default backup images is true behavior",
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
									Profile:          "default",
									S3ForcePathStyle: "true",
								},
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "bucket",
										Prefix: "prefix",
									},
								},
								Default: true,
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
		{
			name: "BSL with config section having only profile and default backup images is true behavior",
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
									Profile: "default",
								},
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "bucket",
										Prefix: "prefix",
									},
								},
								Default: true,
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
		{
			name: "BSL with no region and S3ForcePathStyle as false and default backup images is false",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupImages: pointer.Bool(false),
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								Config: map[string]string{
									S3ForcePathStyle: "false",
								},
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "bucket",
										Prefix: "prefix",
									},
								},
								Default: true,
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
				Data: map[string][]byte{"cloud": []byte("dummy_data")},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "BSL with no region and S3ForcePathStyle as true error case",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					BackupImages: pointer.Bool(false),
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								Config: map[string]string{
									S3ForcePathStyle: "true",
								},
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: DiscoverableBucket,
										Prefix: "prefix",
									},
								},
								Default: true,
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
								Default: true,
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
				Data: map[string][]byte{"cloud": []byte("dummy_data")},
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
	// Mock GetBucketRegionFunc to return a region for all buckets
	originalGetBucketRegionFunc := aws.GetBucketRegionFunc
	aws.GetBucketRegionFunc = func(bucket string) (string, error) {
		// Return us-east-1 for any valid bucket name
		// This simulates successful region discovery for test buckets
		return "us-east-1", nil
	}
	defer func() { aws.GetBucketRegionFunc = originalGetBucketRegionFunc }()

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
			got, err := r.ValidateBackupStorageLocations(*tt.dpa)
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
		wantBSL *velerov1.BackupStorageLocation
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
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
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
									Key: "cloud",
								},
								Default: true,
							},
						},
					},
				},
			},
			wantBSL: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-1",
					Namespace: "bar",
					Labels: map[string]string{
						"app.kubernetes.io/name":     "oadp-operator-velero",
						"app.kubernetes.io/instance": "foo" + "-1",
						//"app.kubernetes.io/version":    "x.y.z",
						"app.kubernetes.io/managed-by":       "oadp-operator",
						"app.kubernetes.io/component":        "bsl",
						oadpv1alpha1.OadpOperatorLabel:       "True",
						oadpv1alpha1.RegistryDeploymentLabel: "True",
					},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         oadpv1alpha1.SchemeBuilder.GroupVersion.String(),
						Kind:               "DataProtectionApplication",
						Name:               "foo",
						Controller:         pointer.BoolPtr(true),
						BlockOwnerDeletion: pointer.BoolPtr(true),
					}},
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "test-aws-bucket",
							Prefix: "velero",
						},
					},
					Config: map[string]string{
						Region:            "test-region",
						checksumAlgorithm: "",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "cloud-credentials",
						},
						Key: "cloud",
					},
					Default: true,
				},
			},
			wantErr: false,
		},
		{
			name: "BSL spec config is nil, no BSL spec update",
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
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
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
				},
			},
			wantBSL: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-1",
					Namespace: "bar",
					Labels: map[string]string{
						"app.kubernetes.io/name":     "oadp-operator-velero",
						"app.kubernetes.io/instance": "foo" + "-1",
						//"app.kubernetes.io/version":    "x.y.z",
						"app.kubernetes.io/managed-by":       "oadp-operator",
						"app.kubernetes.io/component":        "bsl",
						oadpv1alpha1.OadpOperatorLabel:       "True",
						oadpv1alpha1.RegistryDeploymentLabel: "True",
					},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         oadpv1alpha1.SchemeBuilder.GroupVersion.String(),
						Kind:               "DataProtectionApplication",
						Name:               "foo",
						Controller:         pointer.BoolPtr(true),
						BlockOwnerDeletion: pointer.BoolPtr(true),
					}},
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "test-aws-bucket",
							Prefix: "velero",
						},
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "cloud-credentials",
						},
						Key: "cloud",
					},
					Default: true,
				},
			},
			wantErr: false,
		},
		{
			name: "checksumAlgorithm config is not specified by the user, add it as an empty string for BSL config",
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
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
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
									Key: "cloud",
								},
								Default: true,
							},
						},
					},
				},
			},

			wantBSL: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-1",
					Namespace: "bar",
					Labels: map[string]string{
						"app.kubernetes.io/name":     "oadp-operator-velero",
						"app.kubernetes.io/instance": "foo" + "-1",
						//"app.kubernetes.io/version":    "x.y.z",
						"app.kubernetes.io/managed-by":       "oadp-operator",
						"app.kubernetes.io/component":        "bsl",
						oadpv1alpha1.OadpOperatorLabel:       "True",
						oadpv1alpha1.RegistryDeploymentLabel: "True",
					},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         oadpv1alpha1.SchemeBuilder.GroupVersion.String(),
						Kind:               "DataProtectionApplication",
						Name:               "foo",
						Controller:         pointer.BoolPtr(true),
						BlockOwnerDeletion: pointer.BoolPtr(true),
					}},
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "test-aws-bucket",
							Prefix: "velero",
						},
					},
					Config: map[string]string{
						Region:            "test-region",
						checksumAlgorithm: "",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "cloud-credentials",
						},
						Key: "cloud",
					},
					Default: true,
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

			err = r.updateBSLFromSpec(tt.bsl, tt.dpa, *tt.dpa.Spec.BackupLocations[0].Velero)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateBSLFromSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(tt.bsl.Labels, tt.wantBSL.Labels) {
				t.Errorf("expected bsl labels to be %#v, got %#v", tt.wantBSL.Labels, tt.bsl.Labels)
			}
			if !reflect.DeepEqual(tt.bsl.OwnerReferences, tt.wantBSL.OwnerReferences) {
				t.Errorf("expected bsl owner references to be %#v, got %#v", tt.wantBSL.OwnerReferences, tt.bsl.OwnerReferences)
			}
			if !reflect.DeepEqual(tt.bsl.Spec, tt.wantBSL.Spec) {
				t.Errorf("expected bsl Spec to be %#v, got %#v", tt.wantBSL.Spec, tt.bsl.Spec)
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
			for _, bsl := range tt.dpa.Spec.BackupLocations {
				if err := r.ensureBackupLocationHasVeleroOrCloudStorage(&bsl); (err != nil) != tt.wantErr {
					t.Errorf("ensureBSLProviderMapping() error = %v, wantErr %v", err, tt.wantErr)
				}
			}

		})
	}
}

func TestDPAReconciler_ensurePrefixWhenBackupImages(t *testing.T) {
	tests := []struct {
		name        string
		dpa         *oadpv1alpha1.DataProtectionApplication
		wantErr     bool
		expectedErr string
	}{
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
			wantErr:     true,
			expectedErr: "BackupLocation must have cloud storage prefix when backupImages is not set to false",
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
			wantErr: false,
		},
		{
			name: "If DPA CR has Velero with Prefix defined with backupImages enabled, no error case",
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
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{},
						},
					},
					BackupImages: pointer.Bool(true),
				},
			},
			wantErr: false,
		},
		{
			name: "If DPA CR has Velero with No Prefix defined with backupImages enabled,  error case",
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
										Prefix: "",
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
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{},
						},
					},
					BackupImages: pointer.Bool(true),
				},
			},
			wantErr:     true,
			expectedErr: "BackupLocation must have velero prefix when backupImages is not set to false",
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
			for _, bsl := range tt.dpa.Spec.BackupLocations {
				err := r.ensurePrefixWhenBackupImages(tt.dpa, &bsl)
				if (err != nil) != tt.wantErr {
					t.Errorf("ensurePrefixWhenBackupImages() error = %v, wantErr %v", err, tt.wantErr)
				}

				if tt.wantErr && err != nil && err.Error() != tt.expectedErr {
					t.Errorf("ensurePrefixWhenBackupImages() error message = %v, expectedErr = %v", err.Error(), tt.expectedErr)
				}
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
		Data: map[string][]byte{"credentials": {}},
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

	ownerReferenceTests := []struct {
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
								Config: map[string]string{
									Region: "us-east-1",
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
	for _, tt := range ownerReferenceTests {
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
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
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
	bslPrefixCATests := []struct {
		name    string
		objects []client.Object
		want    bool
		wantErr bool
		wantBSL velerov1.BackupStorageLocation
	}{
		{
			name: "dpa.spec.backupLocation.Velero has Prefix set",
			objects: []client.Object{
				&oadpv1alpha1.DataProtectionApplication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dpa",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						BackupLocations: []oadpv1alpha1.BackupLocation{
							{
								Velero: &velerov1.BackupStorageLocationSpec{
									Provider: "aws",
									Config: map[string]string{
										Region: "us-east-1",
									},
									StorageType: velerov1.StorageType{
										ObjectStorage: &velerov1.ObjectStorageLocation{
											Prefix: "test-prefix",
										},
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
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": {}},
				},
			},
			want:    true,
			wantErr: false,
			wantBSL: velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa-1",
					Namespace: "test-ns",
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					Config: map[string]string{
						Region:            "us-east-1",
						checksumAlgorithm: "",
					},
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Prefix: "test-prefix",
						},
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
		{
			name: "dpa.spec.backupLocation.CloudStorage has Prefix set",
			objects: []client.Object{
				&oadpv1alpha1.DataProtectionApplication{
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
									Prefix: "test-prefix",
								},
							},
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": {}},
				},
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cs",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: "aws",
						CreationSecret: v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{
								Name: "cloud-credentials",
							},
							Key: "credentials",
						},
					},
				},
			},
			want:    true,
			wantErr: false,
			wantBSL: velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa-1",
					Namespace: "test-ns",
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Prefix: "test-prefix",
						},
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "cloud-credentials",
						},
						Key: "credentials",
					},
				},
			},
		},
		{
			name: "dpa.spec.backupLocation.Velero has Prefix set and CA set",
			objects: []client.Object{
				&oadpv1alpha1.DataProtectionApplication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dpa",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						BackupLocations: []oadpv1alpha1.BackupLocation{
							{
								Velero: &velerov1.BackupStorageLocationSpec{
									Provider: "aws",
									Config: map[string]string{
										Region: "us-east-1",
									},
									StorageType: velerov1.StorageType{
										ObjectStorage: &velerov1.ObjectStorageLocation{
											Bucket: "test-bucket",
											Prefix: "test-prefix",
											CACert: []byte("test-ca"),
										},
									},
									Credential: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "cloud-credentials",
										},
										Key: "credentials",
									},
								},
							},
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": {}},
				},
			},
			want:    true,
			wantErr: false,
			wantBSL: velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa-1",
					Namespace: "test-ns",
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					Config: map[string]string{
						Region:            "us-east-1",
						checksumAlgorithm: "",
					},
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Prefix: "test-prefix",
							Bucket: "test-bucket",
							CACert: []byte("test-ca"),
						},
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "cloud-credentials",
						},
						Key: "credentials",
					},
				},
			},
		},
		{
			name: "dpa.spec.backupLocation.CloudStorage has Prefix set and CA set",
			objects: []client.Object{
				&oadpv1alpha1.DataProtectionApplication{
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
									Prefix: "test-prefix",
									CACert: []byte("test-ca"),
								},
							},
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": {}},
				},
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cs",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: "aws",
						CreationSecret: v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{
								Name: "cloud-credentials",
							},
							Key: "credentials",
						},
						Region: "test-region",
						Name:   "test-bucket",
					},
				},
			},
			want:    true,
			wantErr: false,
			wantBSL: velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa-1",
					Namespace: "test-ns",
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "test-bucket",
							Prefix: "test-prefix",
							CACert: []byte("test-ca"),
						},
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "cloud-credentials",
						},
						Key: "credentials",
					},
				},
			},
		},
	}
	for _, tt := range bslPrefixCATests {
		t.Run(tt.name, func(t *testing.T) {
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
					Namespace: tt.objects[0].GetNamespace(),
					Name:      tt.objects[0].GetName(),
				},
				EventRecorder: record.NewFakeRecorder(10),
			}
			got, err := r.ReconcileBackupStorageLocations(r.Log)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileBackupStorageLocations() error =%v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ReconcileBackupStorageLocations() got = %v, want %v", got, tt.want)
			}
			bsl := &velerov1.BackupStorageLocation{}
			err = r.Get(r.Context, client.ObjectKey{Namespace: tt.objects[0].GetNamespace(), Name: "test-dpa-1"}, bsl)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileBackupStorageLocations() error =%v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(bsl.Spec, tt.wantBSL.Spec) {
				fmt.Println(cmp.Diff(bsl.Spec, tt.wantBSL.Spec))
				t.Errorf("ReconcileBackupStorageLocations() expected BSL spec to be %#v, got %#v", tt.wantBSL.Spec, bsl.Spec)
			}
		})
	}

	// Test case to ensure BSL reconciliation happens only once when no changes are needed
	t.Run("BSL should not be updated on subsequent reconciliations when no changes", func(t *testing.T) {
		// Setup DPA with BSL configuration
		dpa := &oadpv1alpha1.DataProtectionApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-dpa",
				Namespace: "test-ns",
				UID:       "test-uid",
			},
			Spec: oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velerov1.BackupStorageLocationSpec{
							Provider: "aws",
							Config: map[string]string{
								Region: "us-east-1",
							},
							StorageType: velerov1.StorageType{
								ObjectStorage: &velerov1.ObjectStorageLocation{
									Bucket: "test-bucket",
									Prefix: "test-prefix",
								},
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
			},
		}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cloud-credentials",
				Namespace: "test-ns",
			},
			Data: map[string][]byte{"credentials": []byte("test-credentials")},
		}

		// Create fake client with the DPA and secret
		fakeClient, err := getFakeClientFromObjects(dpa, secret)
		if err != nil {
			t.Fatalf("error creating fake client: %v", err)
		}

		r := &DPAReconciler{
			Client:  fakeClient,
			Scheme:  fakeClient.Scheme(),
			Log:     logr.Discard(),
			Context: newContextForTest(t.Name()),
			NamespacedName: types.NamespacedName{
				Namespace: dpa.Namespace,
				Name:      dpa.Name,
			},
			EventRecorder: record.NewFakeRecorder(10),
		}

		// First reconciliation - should create BSL
		success, err := r.ReconcileBackupStorageLocations(r.Log)
		if err != nil {
			t.Fatalf("first ReconcileBackupStorageLocations() failed: %v", err)
		}
		if !success {
			t.Fatal("first ReconcileBackupStorageLocations() returned false")
		}

		// Get the created BSL and store its generation and resource version
		bsl := &velerov1.BackupStorageLocation{}
		err = r.Get(r.Context, client.ObjectKey{Namespace: "test-ns", Name: "test-dpa-1"}, bsl)
		if err != nil {
			t.Fatalf("failed to get BSL after first reconciliation: %v", err)
		}

		firstGeneration := bsl.Generation
		firstResourceVersion := bsl.ResourceVersion

		// Verify BSL was created with expected configuration
		if bsl.Spec.Provider != "aws" {
			t.Errorf("BSL provider = %v, want aws", bsl.Spec.Provider)
		}
		if bsl.Spec.Config[Region] != "us-east-1" {
			t.Errorf("BSL region = %v, want us-east-1", bsl.Spec.Config[Region])
		}
		if bsl.Spec.ObjectStorage.Bucket != "test-bucket" {
			t.Errorf("BSL bucket = %v, want test-bucket", bsl.Spec.ObjectStorage.Bucket)
		}
		if bsl.Spec.ObjectStorage.Prefix != "test-prefix" {
			t.Errorf("BSL prefix = %v, want test-prefix", bsl.Spec.ObjectStorage.Prefix)
		}

		// Second reconciliation - should not update BSL if nothing changed
		success, err = r.ReconcileBackupStorageLocations(r.Log)
		if err != nil {
			t.Fatalf("second ReconcileBackupStorageLocations() failed: %v", err)
		}
		if !success {
			t.Fatal("second ReconcileBackupStorageLocations() returned false")
		}

		// Get BSL again and verify generation and resource version didn't change
		bsl2 := &velerov1.BackupStorageLocation{}
		err = r.Get(r.Context, client.ObjectKey{Namespace: "test-ns", Name: "test-dpa-1"}, bsl2)
		if err != nil {
			t.Fatalf("failed to get BSL after second reconciliation: %v", err)
		}

		// Generation should remain the same if no spec changes occurred
		if bsl2.Generation != firstGeneration {
			t.Errorf("BSL generation changed unnecessarily: first = %v, second = %v", firstGeneration, bsl2.Generation)
		}

		// Resource version might change even without updates in fake client,
		// but in production it shouldn't change if no updates were made.
		// For a more accurate test, we could track Update calls on the fake client.
		// For now, we'll just log this for information
		if bsl2.ResourceVersion != firstResourceVersion {
			t.Logf("Note: ResourceVersion changed from %v to %v (this may be normal in fake client)", firstResourceVersion, bsl2.ResourceVersion)
		}

		// Third reconciliation - verify it still doesn't change
		success, err = r.ReconcileBackupStorageLocations(r.Log)
		if err != nil {
			t.Fatalf("third ReconcileBackupStorageLocations() failed: %v", err)
		}
		if !success {
			t.Fatal("third ReconcileBackupStorageLocations() returned false")
		}

		bsl3 := &velerov1.BackupStorageLocation{}
		err = r.Get(r.Context, client.ObjectKey{Namespace: "test-ns", Name: "test-dpa-1"}, bsl3)
		if err != nil {
			t.Fatalf("failed to get BSL after third reconciliation: %v", err)
		}

		// Generation should still be the same
		if bsl3.Generation != firstGeneration {
			t.Errorf("BSL generation changed after third reconciliation: first = %v, third = %v", firstGeneration, bsl3.Generation)
		}
	})

	// Test case to ensure BSL with all comprehensive fields doesn't trigger reconciliation loops
	t.Run("Comprehensive BSL with all fields should not trigger reconciliation loops", func(t *testing.T) {
		// Setup DPA with comprehensive BSL configuration including all possible fields
		dpa := &oadpv1alpha1.DataProtectionApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-dpa-comprehensive",
				Namespace: "test-ns",
				UID:       "test-uid-comprehensive",
			},
			Spec: oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Name: "test-bsl-comprehensive",
						Velero: &velerov1.BackupStorageLocationSpec{
							Provider:         "aws",
							AccessMode:       velerov1.BackupStorageLocationAccessMode("ReadWrite"),
							BackupSyncPeriod: &metav1.Duration{Duration: 30 * 1000000000}, // 30s in nanoseconds
							Config: map[string]string{
								Region:            "test-region-1",
								S3ForcePathStyle:  "true",
								S3URL:             "https://test-s3-endpoint.example.com",
								checksumAlgorithm: "",
							},
							Credential: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "test-bsl-secret",
								},
								Key: "cloud",
							},
							Default: false,
							StorageType: velerov1.StorageType{
								ObjectStorage: &velerov1.ObjectStorageLocation{
									Bucket: "test-bucket-comprehensive",
									Prefix: "test-prefix/comprehensive-test",
								},
							},
						},
					},
				},
			},
		}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-bsl-secret",
				Namespace: "test-ns",
			},
			Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=TESTKEY123\naws_secret_access_key=TESTSECRET456")},
		}

		// Create fake client with the DPA and secret
		fakeClient, err := getFakeClientFromObjects(dpa, secret)
		if err != nil {
			t.Fatalf("error creating fake client: %v", err)
		}

		r := &DPAReconciler{
			Client:  fakeClient,
			Scheme:  fakeClient.Scheme(),
			Log:     logr.Discard(),
			Context: newContextForTest(t.Name()),
			NamespacedName: types.NamespacedName{
				Namespace: dpa.Namespace,
				Name:      dpa.Name,
			},
			EventRecorder: record.NewFakeRecorder(10),
		}

		// First reconciliation - should create BSL
		success, err := r.ReconcileBackupStorageLocations(r.Log)
		if err != nil {
			t.Fatalf("first ReconcileBackupStorageLocations() failed: %v", err)
		}
		if !success {
			t.Fatal("first ReconcileBackupStorageLocations() returned false")
		}

		// Get the created BSL and verify all fields are set correctly
		bsl := &velerov1.BackupStorageLocation{}
		bslName := "test-bsl-comprehensive"
		err = r.Get(r.Context, client.ObjectKey{Namespace: "test-ns", Name: bslName}, bsl)
		if err != nil {
			t.Fatalf("failed to get BSL after first reconciliation: %v", err)
		}

		// Store initial generation
		firstGeneration := bsl.Generation

		// Verify all fields are set correctly
		if bsl.Spec.Provider != "aws" {
			t.Errorf("BSL provider = %v, want aws", bsl.Spec.Provider)
		}
		if string(bsl.Spec.AccessMode) != "ReadWrite" {
			t.Errorf("BSL accessMode = %v, want ReadWrite", bsl.Spec.AccessMode)
		}
		if bsl.Spec.BackupSyncPeriod == nil || bsl.Spec.BackupSyncPeriod.Duration != 30*1000000000 {
			t.Errorf("BSL backupSyncPeriod = %v, want 30s", bsl.Spec.BackupSyncPeriod)
		}
		if bsl.Spec.Config[Region] != "test-region-1" {
			t.Errorf("BSL config.region = %v, want test-region-1", bsl.Spec.Config[Region])
		}
		if bsl.Spec.Config[S3ForcePathStyle] != "true" {
			t.Errorf("BSL config.s3ForcePathStyle = %v, want true", bsl.Spec.Config[S3ForcePathStyle])
		}
		if bsl.Spec.Config[S3URL] != "https://test-s3-endpoint.example.com" {
			t.Errorf("BSL config.s3Url = %v, want https://test-s3-endpoint.example.com", bsl.Spec.Config[S3URL])
		}
		if bsl.Spec.Credential.Name != "test-bsl-secret" {
			t.Errorf("BSL credential.name = %v, want test-bsl-secret", bsl.Spec.Credential.Name)
		}
		if bsl.Spec.Credential.Key != "cloud" {
			t.Errorf("BSL credential.key = %v, want cloud", bsl.Spec.Credential.Key)
		}
		if bsl.Spec.Default != false {
			t.Errorf("BSL default = %v, want false", bsl.Spec.Default)
		}
		if bsl.Spec.ObjectStorage.Bucket != "test-bucket-comprehensive" {
			t.Errorf("BSL objectStorage.bucket = %v, want test-bucket-comprehensive", bsl.Spec.ObjectStorage.Bucket)
		}
		if bsl.Spec.ObjectStorage.Prefix != "test-prefix/comprehensive-test" {
			t.Errorf("BSL objectStorage.prefix = %v, want test-prefix/comprehensive-test", bsl.Spec.ObjectStorage.Prefix)
		}

		// Perform 5 reconciliations to ensure no loops occur
		for i := 2; i <= 5; i++ {
			success, err = r.ReconcileBackupStorageLocations(r.Log)
			if err != nil {
				t.Fatalf("reconciliation %d failed: %v", i, err)
			}
			if !success {
				t.Fatalf("reconciliation %d returned false", i)
			}

			// Get BSL and check generation hasn't changed
			bsl := &velerov1.BackupStorageLocation{}
			err = r.Get(r.Context, client.ObjectKey{Namespace: "test-ns", Name: bslName}, bsl)
			if err != nil {
				t.Fatalf("failed to get BSL after reconciliation %d: %v", i, err)
			}

			// Generation should remain the same - this is the key check for no reconciliation loops
			if bsl.Generation != firstGeneration {
				t.Errorf("BSL generation changed unnecessarily at reconciliation %d: first = %v, current = %v",
					i, firstGeneration, bsl.Generation)
			}

			// Verify all fields remain unchanged
			if bsl.Spec.Provider != "aws" {
				t.Errorf("BSL provider changed at reconciliation %d", i)
			}
			if string(bsl.Spec.AccessMode) != "ReadWrite" {
				t.Errorf("BSL accessMode changed at reconciliation %d", i)
			}
			if bsl.Spec.BackupSyncPeriod == nil || bsl.Spec.BackupSyncPeriod.Duration != 30*1000000000 {
				t.Errorf("BSL backupSyncPeriod changed at reconciliation %d", i)
			}
			if bsl.Spec.Config[Region] != "test-region-1" {
				t.Errorf("BSL config.region changed at reconciliation %d", i)
			}
			if bsl.Spec.Config[S3ForcePathStyle] != "true" {
				t.Errorf("BSL config.s3ForcePathStyle changed at reconciliation %d", i)
			}
			if bsl.Spec.Config[S3URL] != "https://test-s3-endpoint.example.com" {
				t.Errorf("BSL config.s3Url changed at reconciliation %d", i)
			}
			if bsl.Spec.Default != false {
				t.Errorf("BSL default changed at reconciliation %d", i)
			}
			if bsl.Spec.ObjectStorage.Bucket != "test-bucket-comprehensive" {
				t.Errorf("BSL objectStorage.bucket changed at reconciliation %d", i)
			}
			if bsl.Spec.ObjectStorage.Prefix != "test-prefix/comprehensive-test" {
				t.Errorf("BSL objectStorage.prefix changed at reconciliation %d", i)
			}
		}

		// Final check - get BSL one more time to ensure stability
		finalBSL := &velerov1.BackupStorageLocation{}
		err = r.Get(r.Context, client.ObjectKey{Namespace: "test-ns", Name: bslName}, finalBSL)
		if err != nil {
			t.Fatalf("failed to get BSL for final check: %v", err)
		}

		// Generation should still be 1 (or whatever the initial was)
		if finalBSL.Generation != firstGeneration {
			t.Errorf("BSL generation changed after all reconciliations: first = %v, final = %v",
				firstGeneration, finalBSL.Generation)
		}

		t.Logf("Successfully completed %d reconciliations without generation changes. Initial generation: %d, Final generation: %d",
			5, firstGeneration, finalBSL.Generation)
	})
}

func TestProcessCACertForBSLs(t *testing.T) {
	testCACertPEM := `-----BEGIN CERTIFICATE-----
MIIDNzCCAh+gAwIBAgIJAJ7qAHESwpNwMA0GCSqGSIb3DQEBCwUAMDMxMTAvBgNV
BAMMKGVjMi01NC0yMTEtOC0yNDguY29tcHV0ZS0xLmFtYXpvbmF3cy5jb20wHhcN
MjUwMTI0MTcxNjQyWhcNMjYwMTI0MTcxNjQyWjAzMTEwLwYDVQQDDChlYzItNTQt
-----END CERTIFICATE-----`

	tests := []struct {
		name              string
		backupLocations   []oadpv1alpha1.BackupLocation
		cloudStorages     []client.Object // CloudStorage objects to add to fake client
		wantConfigMapName string
		wantError         bool
	}{
		{
			name: "BSL with Velero CA certificate",
			backupLocations: []oadpv1alpha1.BackupLocation{
				{
					Velero: &velerov1.BackupStorageLocationSpec{
						Provider: "aws",
						StorageType: velerov1.StorageType{
							ObjectStorage: &velerov1.ObjectStorageLocation{
								Bucket: "test-bucket",
								CACert: []byte(testCACertPEM),
							},
						},
					},
				},
			},
			cloudStorages:     nil, // No CloudStorage objects needed for Velero BSL
			wantConfigMapName: caBundleConfigMapName,
			wantError:         false,
		},
		{
			name: "BSL with CloudStorage CA certificate",
			backupLocations: []oadpv1alpha1.BackupLocation{
				{
					CloudStorage: &oadpv1alpha1.CloudStorageLocation{
						CloudStorageRef: corev1.LocalObjectReference{Name: "test-bucket"},
						CACert:          []byte(testCACertPEM),
					},
				},
			},
			cloudStorages: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bucket",
						Namespace: "test-namespace",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Name:     "test-bucket",
						Provider: oadpv1alpha1.AWSBucketProvider,
					},
				},
			},
			wantConfigMapName: caBundleConfigMapName,
			wantError:         false,
		},
		{
			name: "BSL without CA certificate",
			backupLocations: []oadpv1alpha1.BackupLocation{
				{
					Velero: &velerov1.BackupStorageLocationSpec{
						Provider: "aws",
						StorageType: velerov1.StorageType{
							ObjectStorage: &velerov1.ObjectStorageLocation{
								Bucket: "test-bucket",
							},
						},
					},
				},
			},
			cloudStorages:     nil, // No CloudStorage objects needed
			wantConfigMapName: "",
			wantError:         false,
		},
		{
			name:              "No backup locations",
			backupLocations:   []oadpv1alpha1.BackupLocation{},
			cloudStorages:     nil, // No CloudStorage objects needed
			wantConfigMapName: "",
			wantError:         false,
		},
		{
			name: "Multiple BSLs with different CA certificates - should concatenate",
			backupLocations: []oadpv1alpha1.BackupLocation{
				{
					Velero: &velerov1.BackupStorageLocationSpec{
						Provider: "aws",
						StorageType: velerov1.StorageType{
							ObjectStorage: &velerov1.ObjectStorageLocation{
								Bucket: "test-bucket-1",
								CACert: []byte("-----BEGIN CERTIFICATE-----\nFirst CA Certificate\n-----END CERTIFICATE-----"),
							},
						},
					},
				},
				{
					CloudStorage: &oadpv1alpha1.CloudStorageLocation{
						CloudStorageRef: corev1.LocalObjectReference{Name: "test-bucket-2"},
						CACert:          []byte("-----BEGIN CERTIFICATE-----\nSecond CA Certificate\n-----END CERTIFICATE-----"),
					},
				},
				{
					Velero: &velerov1.BackupStorageLocationSpec{
						Provider: "azure",
						StorageType: velerov1.StorageType{
							ObjectStorage: &velerov1.ObjectStorageLocation{
								Bucket: "test-bucket-3",
								CACert: []byte("-----BEGIN CERTIFICATE-----\nThird CA Certificate\n-----END CERTIFICATE-----"),
							},
						},
					},
				},
			},
			cloudStorages: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bucket-2",
						Namespace: "test-namespace",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Name:     "test-bucket-2",
						Provider: oadpv1alpha1.AWSBucketProvider,
					},
				},
			},
			wantConfigMapName: caBundleConfigMapName,
			wantError:         false,
		},
		{
			name: "Multiple BSLs with duplicate CA certificates - should deduplicate",
			backupLocations: []oadpv1alpha1.BackupLocation{
				{
					Velero: &velerov1.BackupStorageLocationSpec{
						Provider: "aws",
						StorageType: velerov1.StorageType{
							ObjectStorage: &velerov1.ObjectStorageLocation{
								Bucket: "test-bucket-1",
								CACert: []byte(testCACertPEM),
							},
						},
					},
				},
				{
					CloudStorage: &oadpv1alpha1.CloudStorageLocation{
						CloudStorageRef: corev1.LocalObjectReference{Name: "test-bucket-2"},
						CACert:          []byte(testCACertPEM), // Same certificate
					},
				},
			},
			cloudStorages: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bucket-2",
						Namespace: "test-namespace",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Name:     "test-bucket-2",
						Provider: oadpv1alpha1.AWSBucketProvider,
					},
				},
			},
			wantConfigMapName: caBundleConfigMapName,
			wantError:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dpa := &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: tt.backupLocations,
				},
			}

			// Create fake client with the DPA and CloudStorage objects
			objects := []client.Object{dpa}
			if tt.cloudStorages != nil {
				objects = append(objects, tt.cloudStorages...)
			}
			fakeClient, err := getFakeClientFromObjects(objects...)
			if err != nil {
				t.Fatalf("error creating fake client: %v", err)
			}

			r := &DPAReconciler{
				Client:        fakeClient,
				Scheme:        fakeClient.Scheme(),
				Log:           logr.Discard(),
				Context:       context.Background(),
				EventRecorder: record.NewFakeRecorder(10),
				NamespacedName: types.NamespacedName{
					Name:      dpa.Name,
					Namespace: dpa.Namespace,
				},
			}

			gotConfigMapName, err := r.processCACertForBSLs(dpa)

			if tt.wantError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if gotConfigMapName != tt.wantConfigMapName {
				t.Errorf("expected ConfigMap name %q, got %q", tt.wantConfigMapName, gotConfigMapName)
			}

			if tt.wantConfigMapName != "" {
				configMap := &corev1.ConfigMap{}
				err := fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      tt.wantConfigMapName,
					Namespace: dpa.Namespace,
				}, configMap)
				if err != nil {
					t.Fatalf("expected ConfigMap to exist: %v", err)
				}

				// Verify ConfigMap contains the CA certificate
				assert.Contains(t, configMap.Data, caBundleFileName)

				// Verify content based on test case
				bundleContent := configMap.Data[caBundleFileName]
				if strings.Contains(tt.name, "Multiple BSLs with different CA certificates") {
					// Verify only AWS certificates are concatenated (Azure is filtered out)
					assert.Contains(t, bundleContent, "First CA Certificate")
					assert.Contains(t, bundleContent, "Second CA Certificate")
					// Azure certificate should NOT be included (provider filtering)
					assert.NotContains(t, bundleContent, "Third CA Certificate")
				} else if strings.Contains(tt.name, "Multiple BSLs with duplicate CA certificates") {
					// Verify duplicate is only included once
					assert.Equal(t, 1, strings.Count(bundleContent, testCACertPEM))
				} else {
					// Single certificate case
					assert.Contains(t, bundleContent, testCACertPEM)
				}

				if configMap.Labels["app.kubernetes.io/name"] != "velero" {
					t.Errorf("expected label app.kubernetes.io/name=velero, got %s", configMap.Labels["app.kubernetes.io/name"])
				}
				if configMap.Labels["app.kubernetes.io/component"] != "ca-bundle" {
					t.Errorf("expected label app.kubernetes.io/component=ca-bundle, got %s", configMap.Labels["app.kubernetes.io/component"])
				}
				if configMap.Labels[oadpv1alpha1.OadpOperatorLabel] != "True" {
					t.Errorf("expected OADP operator label to be True")
				}
			}
		})
	}
}

// TestDPAReconciler_ensureBSLPreservesDefaultField tests that BSL reconciliation preserves the default field
// to avoid conflicts with Velero's management of default BSLs
func TestDPAReconciler_ensureBSLPreservesDefaultField(t *testing.T) {
	tests := []struct {
		name                 string
		dpa                  *oadpv1alpha1.DataProtectionApplication
		existingBSL          *velerov1.BackupStorageLocation
		wantDefaultPreserved bool
		wantDefaultValue     bool
	}{
		{
			name: "New BSL creation should set default from DPA spec",
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
								Default:  true,
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-bucket",
									},
								},
								Config: map[string]string{
									"region": "us-east-1",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "cloud",
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
			existingBSL:          nil, // New BSL
			wantDefaultPreserved: false,
			wantDefaultValue:     true, // Should use value from DPA
		},
		{
			name: "Existing BSL update should preserve default field managed by Velero",
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
								Default:  true, // DPA says true
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-bucket",
									},
								},
								Config: map[string]string{
									"region": "us-east-1",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "cloud",
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
			existingBSL: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa-1",
					Namespace:       "test-ns",
					ResourceVersion: "12345", // Has resourceVersion, indicating it exists
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Default: false, // Velero has set it to false
				},
			},
			wantDefaultPreserved: true,
			wantDefaultValue:     false, // Should preserve Velero's value
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build objects for fake client
			var objs []client.Object
			objs = append(objs, tt.dpa)
			if tt.existingBSL != nil {
				objs = append(objs, tt.existingBSL)
			}

			// Add required credential secret
			credSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: tt.dpa.Namespace,
				},
				Data: map[string][]byte{
					"cloud": []byte("[default]\naws_access_key_id=test\naws_secret_access_key=test\n"),
				},
			}
			objs = append(objs, credSecret)

			// Create fake client
			fakeClient, err := getFakeClientFromObjects(objs...)
			if err != nil {
				t.Fatalf("error creating fake client: %v", err)
			}

			// Create reconciler
			r := &DPAReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: context.Background(),
				NamespacedName: types.NamespacedName{
					Name:      tt.dpa.Name,
					Namespace: tt.dpa.Namespace,
				},
				EventRecorder: record.NewFakeRecorder(100),
			}

			// Call the BSL reconciliation
			_, err = r.ReconcileBackupStorageLocations(r.Log)
			assert.NoError(t, err)

			// Verify the BSL was created/updated correctly
			bsl := &velerov1.BackupStorageLocation{}
			err = fakeClient.Get(context.Background(), types.NamespacedName{
				Name:      "test-dpa-1",
				Namespace: tt.dpa.Namespace,
			}, bsl)
			assert.NoError(t, err)

			// Check if default field is preserved correctly
			assert.Equal(t, tt.wantDefaultValue, bsl.Spec.Default,
				"Default field should be %v but got %v", tt.wantDefaultValue, bsl.Spec.Default)

			// Verify resource version exists (indicates successful update without conflict)
			assert.NotEmpty(t, bsl.ResourceVersion, "BSL should have a resource version after reconciliation")
		})
	}
}

// TestValidatePEMCertificate tests the validatePEMCertificate function
func TestValidatePEMCertificate(t *testing.T) {
	// Valid certificate (real self-signed certificate)
	validCert := `-----BEGIN CERTIFICATE-----
MIIDQTCCAimgAwIBAgIUJQPjA2PvLt+8L2KIrVukS1QRq5kwDQYJKoZIhvcNAQEL
BQAwMDEOMAwGA1UEAwwFVGVzdDExDjAMBgNVBAoMBVRlc3QxMQ4wDAYDVQQLDAVU
ZXN0MTAeFw0yNDAxMDEwMDAwMDBaFw0zNDAxMDEwMDAwMDBaMDAxDjAMBgNVBAMM
BVRlc3QxMQ4wDAYDVQQKDAVUZXN0MTEOMAwGA1UECwwFVGVzdDEwggEiMA0GCSqG
SIb3DQEBAQUAA4IBDwAwggEKAoIBAQDXlGGbLWoz3s/Kpua2DXDw8xIiCBSQx2hn
hQz9d+83NkF9Y6G9X/odV8o2JqftS3N5YbjP5wxF65EuxQ8EQc3u7LvQF8/k7tYN
QcxQuPL7+W3sZQWu0oyPK6c0fKGn0w3l7N5KpQN9mKt0OqGUY/N3c6qKLcbTDNMS
NTMm5B6OqDw7dNjNWpMsDaLaODIHmGJIhz1cR49gBQULQ7p0LxOUO6u/9K+/jk7M
C+s2vE3ovf5fSsjL7rZClOQBcJNZGq7eCQW7LCfLEZ1xsfOqGDXQVIdqP5ty+peH
u6OwzLWJ8ChE8HvNlQxBlKrQvnQ9CMorqVEeeLqVMUdNZ+DuSgV9AgMBAAGjUzBR
MB0GA1UdDgQWBBR8OoVW0pWitaen1uRglCpL8kErojAfBgNVHSMEGDAWgBR8OoVW
0pWitaen1uRglCpL8kErojAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUA
A4IBAQCJlg5ppNqJFCwMzctR9yDLgbaFH9ls+cOaLrZIB7qRqHBtHZ8U7PljabKI
9S/cBPwFYUssQb/fC1pq9QB8J4y7hZc5d4oOuKMpVoHHy6QLTM5qbsNm4MQcRWU0
ogVVYIY8s5gVn2AWVUEXDZvGaWHXVVgPNBhDQXGBH7TG4HgbnkTDrxuTt1kNW5xb
M4LM/BhgpiqTshTB1z5l5n3lL+4gPGDe2pA7L9nsvgAR4dS7N4A7MOYW3Ff9c3Cm
USy+h6LGQKI9hBfNL7lE1+ESNjx0dEKKuGCLv0vQJ7L1PezqMDztLPlkre9C+1YM
OJmJ3SBo31J5zoFoXYh3gzI3OA/C
-----END CERTIFICATE-----`

	// Invalid PEM block (not a certificate)
	invalidPEMType := `-----BEGIN PRIVATE KEY-----
MIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQDBiEEb/Pc5IysO
-----END PRIVATE KEY-----`

	// Malformed PEM (invalid base64)
	malformedPEM := `-----BEGIN CERTIFICATE-----
INVALID BASE64 CONTENT!!!
-----END CERTIFICATE-----`

	// Not a PEM format at all
	notPEM := `This is not a PEM formatted certificate`

	// Empty certificate
	emptyCert := ``

	// Valid certificate bundle (multiple certificates - using same cert twice)
	validBundle := validCert + "\n" + validCert

	// Dummy certificate from e2e tests (should fail validation but be handled gracefully)
	dummyCertFromE2E := `-----BEGIN CERTIFICATE-----
MIIDazCCAlOgAwIBAgIUUf8+3K8zsP/w1P3VQ5jlMxALinkwDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExDjAMBgNVBAoM
BU9BQVBQMREWFAYDVQQDDA1EVU1NWS1DQS1DRVJUMB4XDTI0MDEwMTAwMDAwMFoX
DTM0MDEwMTAwMDAwMFowRTELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3Ju
aWExDjAMBgNVBAoMBU9BQVBQMREWFAYDVQQDDA1EVU1NWS1DQS1DRVJUMIIBIDAN
BgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA0VUxbPWcfcOJC2qKZVv5nKqY7OZw
TEST-CERT-CONTENT-TEST-CERT-CONTENT-TEST-CERT-CONTENT-TEST
ngpurposesonly1234567890QIDAQABMA0GCSqGSIb3DQEBCwUAA4IBAQBYfMVqNb
iVL1x+dummyenddummyenddummyenddummyenddummyenddummyenddummyenddum
TEST-CERT-END-TEST-CERT-END-TEST-CERT-END-TEST
ddummyenddummyenddummyenddummyend
-----END CERTIFICATE-----`

	tests := []struct {
		name        string
		cert        []byte
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid certificate",
			cert:    []byte(validCert),
			wantErr: false,
		},
		{
			name:        "invalid PEM type (private key)",
			cert:        []byte(invalidPEMType),
			wantErr:     true,
			errContains: "PEM block is not a certificate",
		},
		{
			name:        "malformed PEM",
			cert:        []byte(malformedPEM),
			wantErr:     true,
			errContains: "no valid PEM block found",
		},
		{
			name:        "not PEM format",
			cert:        []byte(notPEM),
			wantErr:     true,
			errContains: "no valid PEM block found",
		},
		{
			name:        "empty certificate",
			cert:        []byte(emptyCert),
			wantErr:     true,
			errContains: "no valid PEM block found",
		},
		{
			name:    "valid certificate bundle",
			cert:    []byte(validBundle),
			wantErr: false,
		},
		{
			name:        "dummy certificate from e2e (invalid x509)",
			cert:        []byte(dummyCertFromE2E),
			wantErr:     true,
			errContains: "no valid PEM block found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePEMCertificate(tt.cert)
			if tt.wantErr {
				assert.Error(t, err, "validatePEMCertificate() should have returned an error")
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains, "Error message should contain expected string")
				}
			} else {
				assert.NoError(t, err, "validatePEMCertificate() should not have returned an error")
			}
		})
	}
}

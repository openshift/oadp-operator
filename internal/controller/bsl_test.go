package controller

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
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
		objects []client.Object
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
				Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
				Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
				Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
				Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
				Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
			objects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{Name: "testing", Namespace: "test-ns"},
				},
			},
			want:    true,
			wantErr: false,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
				Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
				Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
				Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "CloudStorage with different providers - AWS",
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
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "aws-cloudstorage",
								},
								Prefix: "velero",
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
			objects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "aws-cloudstorage",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.AWSBucketProvider,
						Name:     "test-bucket",
						Region:   "us-east-1",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "CloudStorage with different providers - Azure",
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
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "azure-cloudstorage",
								},
								Prefix: "backups",
								Config: map[string]string{
									"resourceGroup":  "test-rg",
									"storageAccount": "testsa",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "azure-credentials",
									},
									Key: "cloud",
								},
								Default: true,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "azure-cloudstorage",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.AzureBucketProvider,
						Name:     "test-container",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-credentials",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{"cloud": []byte("AZURE_STORAGE_ACCOUNT_ACCESS_KEY=test-key\nAZURE_CLOUD_NAME=AzurePublicCloud")},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "CloudStorage with different providers - GCP",
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
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "gcp-cloudstorage",
								},
								Prefix: "velero-backups",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "gcp-credentials",
									},
									Key: "cloud",
								},
								Default: true,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gcp-cloudstorage",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.GCPBucketProvider,
						Name:     "test-gcp-bucket",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcp-credentials",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{"cloud": []byte(`{"type":"service_account","project_id":"test-project"}`)},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "CloudStorage with backupSyncPeriod",
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
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "sync-period-cs",
								},
								Prefix:           "prefix",
								BackupSyncPeriod: &metav1.Duration{Duration: 5 * 60 * 1000000000}, // 5 minutes
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
			objects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sync-period-cs",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.AWSBucketProvider,
						Name:     "sync-bucket",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "CloudStorage with various config combinations",
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
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "config-cs",
								},
								Prefix: "velero",
								Config: map[string]string{
									"s3ForcePathStyle":      "true",
									"serverSideEncryption":  "AES256",
									"insecureSkipTLSVerify": "true",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "cloud",
								},
								CACert:  []byte("test-ca-cert"),
								Default: true,
							},
						},
					},
				},
			},
			objects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-cs",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.AWSBucketProvider,
						Name:     "config-bucket",
						Region:   "us-west-2",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{"cloud": []byte("[default]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")},
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
		{
			name: "test duplicate backup location names should fail validation",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
								oadpv1alpha1.DefaultPluginMicrosoftAzure,
							},
						},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Name: "duplicate-name",
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								Default:  true,
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-aws-bucket",
										Prefix: "velero/backups",
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
						{
							Name: "duplicate-name", // Same name as above
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "azure",
								Default:  false,
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-azure-bucket",
										Prefix: "velero/backups",
									},
								},
								Config: map[string]string{
									"resourceGroup":  "test-rg",
									"storageAccount": "test-sa",
								},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "azure-credentials",
									},
									Key: "cloud",
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
				Data: map[string][]byte{
					"cloud": []byte("[default]\naws_access_key_id=test\naws_secret_access_key=test"),
				},
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "azure-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"cloud": []byte("AZURE_SUBSCRIPTION_ID=test\nAZURE_TENANT_ID=test\nAZURE_CLIENT_ID=test\nAZURE_CLIENT_SECRET=test\nAZURE_RESOURCE_GROUP=test-rg\nAZURE_CLOUD_NAME=AzurePublicCloud"),
					},
				},
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "test backup location with whitespace-only name should fail validation",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
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
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Name: "   ", // Whitespace-only name should fail
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: "aws",
								Default:  true,
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: "test-aws-bucket",
										Prefix: "velero/backups",
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
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"cloud": []byte("[default]\naws_access_key_id=test\naws_secret_access_key=test"),
				},
			},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.objects = append(tt.objects, tt.dpa, tt.secret)
			fakeClient, err := getFakeClientFromObjects(tt.objects...)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DataProtectionApplicationReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(),
				NamespacedName: types.NamespacedName{
					Namespace: tt.dpa.Namespace,
					Name:      tt.dpa.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
				dpa:           tt.dpa,
			}
			got, err := r.ValidateBackupStorageLocations()
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

func newContextForTest() context.Context {
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
						"app.kubernetes.io/managed-by": "oadp-operator",
						"app.kubernetes.io/component":  "bsl",
						oadpv1alpha1.OadpOperatorLabel: "True",
						common.RegistryDeploymentLabel: "True",
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
						"app.kubernetes.io/managed-by": "oadp-operator",
						"app.kubernetes.io/component":  "bsl",
						oadpv1alpha1.OadpOperatorLabel: "True",
						common.RegistryDeploymentLabel: "True",
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
						"app.kubernetes.io/managed-by": "oadp-operator",
						"app.kubernetes.io/component":  "bsl",
						oadpv1alpha1.OadpOperatorLabel: "True",
						common.RegistryDeploymentLabel: "True",
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
			r := &DataProtectionApplicationReconciler{
				Scheme: scheme,
				dpa:    tt.dpa,
			}

			err = r.updateBSLFromSpec(tt.bsl, *tt.dpa.Spec.BackupLocations[0].Velero)
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
								CloudStorageRef: corev1.LocalObjectReference{
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
			r := &DataProtectionApplicationReconciler{
				Scheme: scheme,
				dpa:    tt.dpa,
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
			r := &DataProtectionApplicationReconciler{
				Scheme: scheme,
				dpa:    tt.dpa,
			}
			for _, bsl := range tt.dpa.Spec.BackupLocations {
				err := r.ensurePrefixWhenBackupImages(&bsl)
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
			CreationSecret: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
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
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "test-cs",
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
			r := &DataProtectionApplicationReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(),
				NamespacedName: types.NamespacedName{
					Namespace: tt.dpa.Namespace,
					Name:      tt.dpa.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
				dpa:           tt.dpa,
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
									CloudStorageRef: corev1.LocalObjectReference{
										Name: "test-cs",
									},
									Credential: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
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
						CreationSecret: corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
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
									CloudStorageRef: corev1.LocalObjectReference{
										Name: "test-cs",
									},
									Credential: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
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
						CreationSecret: corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
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
		{
			name: "CloudStorage with AWS provider and enableSharedConfig",
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
									CloudStorageRef: corev1.LocalObjectReference{
										Name: "aws-shared-config-cs",
									},
									Credential: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "cloud-credentials",
										},
										Key: "credentials",
									},
									Prefix: "backups",
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
						Name:      "aws-shared-config-cs",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider:           oadpv1alpha1.AWSBucketProvider,
						Name:               "shared-config-bucket",
						EnableSharedConfig: pointer.Bool(true),
						Region:             "us-east-1",
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
					Config: map[string]string{
						"enableSharedConfig": "true",
					},
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "shared-config-bucket",
							Prefix: "backups",
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
			name: "CloudStorage with Azure provider",
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
									CloudStorageRef: corev1.LocalObjectReference{
										Name: "azure-cs",
									},
									Credential: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "azure-credentials",
										},
										Key: "credentials",
									},
									Config: map[string]string{
										"storageAccount": "mystorageaccount",
										"resourceGroup":  "myresourcegroup",
									},
									Prefix: "velero-backups",
								},
							},
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "azure-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": {}},
				},
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "azure-cs",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.AzureBucketProvider,
						Name:     "my-azure-container",
						Region:   "eastus",
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
					Provider: "azure",
					Config: map[string]string{
						"storageAccount": "mystorageaccount",
						"resourceGroup":  "myresourcegroup",
					},
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "my-azure-container",
							Prefix: "velero-backups",
						},
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "azure-credentials",
						},
						Key: "credentials",
					},
				},
			},
		},
		{
			name: "CloudStorage with GCP provider",
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
									CloudStorageRef: corev1.LocalObjectReference{
										Name: "gcp-cs",
									},
									Credential: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "gcp-credentials",
										},
										Key: "credentials",
									},
									Prefix:  "velero",
									Default: true,
								},
							},
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gcp-credentials",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": {}},
				},
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gcp-cs",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.GCPBucketProvider,
						Name:     "my-gcp-bucket",
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
					Provider: "gcp",
					Config:   map[string]string(nil),
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "my-gcp-bucket",
							Prefix: "velero",
						},
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "gcp-credentials",
						},
						Key: "credentials",
					},
					Default: true,
				},
			},
		},
		{
			name: "Multiple CloudStorage BSLs",
			objects: []client.Object{
				&oadpv1alpha1.DataProtectionApplication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dpa",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						BackupLocations: []oadpv1alpha1.BackupLocation{
							{
								Name: "aws-bsl",
								CloudStorage: &oadpv1alpha1.CloudStorageLocation{
									CloudStorageRef: corev1.LocalObjectReference{
										Name: "aws-cs-1",
									},
									Credential: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "aws-creds",
										},
										Key: "credentials",
									},
									Prefix:  "aws-backups",
									Default: true,
								},
							},
							{
								Name: "gcp-bsl",
								CloudStorage: &oadpv1alpha1.CloudStorageLocation{
									CloudStorageRef: corev1.LocalObjectReference{
										Name: "gcp-cs-1",
									},
									Credential: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "gcp-creds",
										},
										Key: "credentials",
									},
									Prefix: "gcp-backups",
								},
							},
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "aws-creds",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": {}},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gcp-creds",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{"credentials": {}},
				},
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "aws-cs-1",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.AWSBucketProvider,
						Name:     "aws-bucket-1",
						Region:   "us-west-2",
					},
				},
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gcp-cs-1",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.GCPBucketProvider,
						Name:     "gcp-bucket-1",
					},
				},
			},
			want:    true,
			wantErr: false,
			wantBSL: velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-bsl",
					Namespace: "test-ns",
				},
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					Config:   map[string]string(nil),
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "aws-bucket-1",
							Prefix: "aws-backups",
						},
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-creds",
						},
						Key: "credentials",
					},
					Default: true,
				},
			},
		},
		{
			name: "CloudStorage with backupSyncPeriod",
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
									CloudStorageRef: corev1.LocalObjectReference{
										Name: "aws-cs-sync",
									},
									Credential: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "cloud-credentials",
										},
										Key: "credentials",
									},
									Prefix:           "sync-test",
									BackupSyncPeriod: &metav1.Duration{Duration: 300},
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
						Name:      "aws-cs-sync",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.AWSBucketProvider,
						Name:     "sync-test-bucket",
						Region:   "eu-west-1",
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
					Config:   map[string]string(nil),
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "sync-test-bucket",
							Prefix: "sync-test",
						},
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "cloud-credentials",
						},
						Key: "credentials",
					},
					BackupSyncPeriod: &metav1.Duration{Duration: 300},
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
			r := &DataProtectionApplicationReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(),
				NamespacedName: types.NamespacedName{
					Namespace: tt.objects[0].GetNamespace(),
					Name:      tt.objects[0].GetName(),
				},
				EventRecorder: record.NewFakeRecorder(10),
				dpa:           tt.objects[0].(*oadpv1alpha1.DataProtectionApplication),
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
			bslName := tt.wantBSL.Name
			if bslName == "" {
				bslName = "test-dpa-1"
			}
			err = r.Get(r.Context, client.ObjectKey{Namespace: tt.objects[0].GetNamespace(), Name: bslName}, bsl)
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
}

func TestPatchSecretsForBSL(t *testing.T) {
	tests := []struct {
		name          string
		bsl           *velerov1.BackupStorageLocation
		bslSpec       oadpv1alpha1.BackupLocation
		secret        *corev1.Secret
		cloudStorage  *oadpv1alpha1.CloudStorage
		expectError   bool
		errorContains string
		verifyFunc    func(t *testing.T, secret *corev1.Secret)
	}{
		{
			name: "AWS provider with known bucket openshift-velero-plugin-s3-auto-region-test-1",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					Config: map[string]string{
						"region": "us-east-1",
						"bucket": "openshift-velero-plugin-s3-auto-region-test-1",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-secret",
						},
						Key: "credentials",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						"oadp.openshift.io/secret-type": "sts-credentials",
					},
				},
				Data: map[string][]byte{
					"credentials": []byte(`[default]
role_arn = arn:aws:iam::123456789012:role/test-role
web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token`),
				},
			},
			verifyFunc: func(t *testing.T, secret *corev1.Secret) {
				assert.Contains(t, string(secret.Data["credentials"]), "region = us-east-1")
			},
		},
		{
			name: "AWS provider with known bucket openshift-velero-plugin-s3-auto-region-test-2",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					Config: map[string]string{
						"region": "us-west-1",
						"bucket": "openshift-velero-plugin-s3-auto-region-test-2",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-secret",
						},
						Key: "credentials",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						"oadp.openshift.io/secret-type": "sts-credentials",
					},
				},
				Data: map[string][]byte{
					"credentials": []byte(`[default]
role_arn = arn:aws:iam::123456789012:role/test-role
web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token`),
				},
			},
			verifyFunc: func(t *testing.T, secret *corev1.Secret) {
				assert.Contains(t, string(secret.Data["credentials"]), "region = us-west-1")
			},
		},
		{
			name: "AWS provider with known bucket openshift-velero-plugin-s3-auto-region-test-3",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					Config: map[string]string{
						"region": "eu-central-1",
						"bucket": "openshift-velero-plugin-s3-auto-region-test-3",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-secret",
						},
						Key: "credentials",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						"oadp.openshift.io/secret-type": "sts-credentials",
					},
				},
				Data: map[string][]byte{
					"credentials": []byte(`[default]
role_arn = arn:aws:iam::123456789012:role/test-role
web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token`),
				},
			},
			verifyFunc: func(t *testing.T, secret *corev1.Secret) {
				assert.Contains(t, string(secret.Data["credentials"]), "region = eu-central-1")
			},
		},
		{
			name: "Azure provider with resource group patching",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "azure",
					Config: map[string]string{
						"resourceGroup": "test-rg",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "azure-secret",
						},
						Key: "azurekey",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						"oadp.openshift.io/secret-type": "sts-credentials",
					},
				},
				Data: map[string][]byte{
					"azurekey": []byte(`AZURE_CLIENT_ID=test-client
AZURE_TENANT_ID=test-tenant`),
				},
			},
			verifyFunc: func(t *testing.T, secret *corev1.Secret) {
				assert.Contains(t, string(secret.Data["azurekey"]), "AZURE_RESOURCE_GROUP=test-rg")
			},
		},
		{
			name: "GCP provider - no patching needed",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "gcp",
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "gcp-secret",
						},
						Key: "cloud",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcp-secret",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"cloud": []byte(`{"type":"service_account"}`),
				},
			},
			verifyFunc: func(t *testing.T, secret *corev1.Secret) {
				// Should remain unchanged
				assert.Equal(t, `{"type":"service_account"}`, string(secret.Data["cloud"]))
			},
		},
		{
			name: "CloudStorage AWS provider",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "test-cloudstorage",
					},
					Config: map[string]string{
						"region": "eu-west-1",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-secret",
						},
						Key: "credentials",
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cloudstorage",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Provider: oadpv1alpha1.AWSBucketProvider,
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						"oadp.openshift.io/secret-type": "sts-credentials",
					},
				},
				Data: map[string][]byte{
					"credentials": []byte(`[default]
role_arn = arn:aws:iam::123456789012:role/test-role
web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token`),
				},
			},
			verifyFunc: func(t *testing.T, secret *corev1.Secret) {
				assert.Contains(t, string(secret.Data["credentials"]), "region = eu-west-1")
			},
		},
		{
			name: "No secret name - uses default cloud-credentials",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "aws",
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"cloud": []byte(`[default]
aws_access_key_id=test-key
aws_secret_access_key=test-secret`),
				},
			},
			expectError: false,
			verifyFunc: func(t *testing.T, secret *corev1.Secret) {
				// Should not be patched since it doesn't have the STS label
				assert.NotContains(t, string(secret.Data["cloud"]), "region")
			},
		},
		{
			name: "Secret not found",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "non-existent-secret",
						},
						Key: "cloud",
					},
				},
			},
			expectError:   true,
			errorContains: "failed to get secret",
		},
		{
			name: "CloudStorage not found",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "non-existent-cloudstorage",
					},
				},
			},
			expectError:   true,
			errorContains: "not found",
		},
		{
			name: "AWS provider with STS credentials - should be patched",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					Config: map[string]string{
						"region": "us-east-1",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-sts-secret",
						},
						Key: "credentials",
					},
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "test-bucket",
						},
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-sts-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						"oadp.openshift.io/secret-type": "sts-credentials",
					},
				},
				Data: map[string][]byte{
					"credentials": []byte(`[default]
role_arn = arn:aws:iam::123456789012:role/test-role
web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token`),
				},
			},
			verifyFunc: func(t *testing.T, secret *corev1.Secret) {
				assert.Contains(t, string(secret.Data["credentials"]), "region = us-east-1")
			},
		},
		{
			name: "AWS provider without STS label - should NOT be patched",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					Config: map[string]string{
						"region": "us-east-1",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-regular-secret",
						},
						Key: "credentials",
					},
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "test-bucket",
						},
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-regular-secret",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"credentials": []byte(`[default]
aws_access_key_id=test-key
aws_secret_access_key=test-secret`),
				},
			},
			verifyFunc: func(t *testing.T, secret *corev1.Secret) {
				// Should NOT contain region since it's not an STS secret
				assert.NotContains(t, string(secret.Data["credentials"]), "region = ")
			},
		},
		{
			name: "Azure provider with STS credentials - should be patched",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "azure",
					Config: map[string]string{
						"resourceGroup": "test-rg",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "azure-sts-secret",
						},
						Key: "azurekey",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-sts-secret",
					Namespace: "test-ns",
					Labels: map[string]string{
						"oadp.openshift.io/secret-type": "sts-credentials",
					},
				},
				Data: map[string][]byte{
					"azurekey": []byte(`AZURE_CLIENT_ID=test-client
AZURE_TENANT_ID=test-tenant`),
				},
			},
			verifyFunc: func(t *testing.T, secret *corev1.Secret) {
				assert.Contains(t, string(secret.Data["azurekey"]), "AZURE_RESOURCE_GROUP=test-rg")
			},
		},
		{
			name: "Azure provider without STS label - should NOT be patched",
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
			bslSpec: oadpv1alpha1.BackupLocation{
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "azure",
					Config: map[string]string{
						"resourceGroup": "test-rg",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "azure-regular-secret",
						},
						Key: "azurekey",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-regular-secret",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"azurekey": []byte(`AZURE_CLIENT_ID=test-client
AZURE_CLIENT_SECRET=test-secret
AZURE_TENANT_ID=test-tenant`),
				},
			},
			verifyFunc: func(t *testing.T, secret *corev1.Secret) {
				// Should NOT contain resource group since it's not an STS secret
				assert.NotContains(t, string(secret.Data["azurekey"]), "AZURE_RESOURCE_GROUP=")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with objects
			objs := []client.Object{}
			if tt.secret != nil {
				objs = append(objs, tt.secret)
			}
			if tt.cloudStorage != nil {
				objs = append(objs, tt.cloudStorage)
			}
			schemeForFakeClient, err := getSchemeForFakeClient()
			if err != nil {
				t.Error(err)
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(schemeForFakeClient).
				WithObjects(objs...).
				Build()

			r := &DataProtectionApplicationReconciler{
				Client:  fakeClient,
				Scheme:  schemeForFakeClient,
				Context: context.Background(),
				Log:     logr.Discard(),
			}

			// Call the function
			err = r.patchSecretsForBSL(tt.bsl, tt.bslSpec)

			// Check error
			if tt.expectError {
				assert.Error(t, err)
				if err != nil && tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify secret if needed
				if tt.verifyFunc != nil && tt.secret != nil {
					// Get the updated secret
					updatedSecret := &corev1.Secret{}
					err := fakeClient.Get(context.Background(), client.ObjectKey{
						Name:      tt.secret.Name,
						Namespace: tt.secret.Namespace,
					}, updatedSecret)
					assert.NoError(t, err)
					tt.verifyFunc(t, updatedSecret)
				}
			}
		})
	}
}
func TestDPAReconciler_populateBSLFromCloudStorage(t *testing.T) {
	tests := []struct {
		name         string
		bslSpec      *oadpv1alpha1.BackupLocation
		cloudStorage *oadpv1alpha1.CloudStorage
		expectedBSL  *oadpv1alpha1.BackupLocation
		wantErr      bool
		errorMsg     string
	}{
		{
			name: "AWS provider mapping",
			bslSpec: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "aws-bucket",
					},
					Prefix: "velero-backups",
					Config: map[string]string{
						"serverSideEncryption": "AES256",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-creds",
						},
						Key: "credentials",
					},
					Default:          true,
					BackupSyncPeriod: &metav1.Duration{Duration: 300},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-bucket",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Provider: oadpv1alpha1.AWSBucketProvider,
					Name:     "my-aws-bucket",
					Region:   "us-east-1",
				},
			},
			expectedBSL: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "aws-bucket",
					},
					Prefix: "velero-backups",
					Config: map[string]string{
						"serverSideEncryption": "AES256",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-creds",
						},
						Key: "credentials",
					},
					Default:          true,
					BackupSyncPeriod: &metav1.Duration{Duration: 300},
				},
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "my-aws-bucket",
							Prefix: "velero-backups",
						},
					},
					Config: map[string]string{
						"region":               "us-east-1",
						"serverSideEncryption": "AES256",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-creds",
						},
						Key: "credentials",
					},
					Default:          true,
					BackupSyncPeriod: &metav1.Duration{Duration: 300},
				},
			},
			wantErr: false,
		},
		{
			name: "Azure provider mapping",
			bslSpec: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "azure-bucket",
					},
					Prefix: "backup-prefix",
					Config: map[string]string{
						"storageAccount": "mystorageaccount",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "azure-creds",
						},
						Key: "credentials",
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-bucket",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Provider: oadpv1alpha1.AzureBucketProvider,
					Name:     "my-azure-container",
					Region:   "eastus",
				},
			},
			expectedBSL: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "azure-bucket",
					},
					Prefix: "backup-prefix",
					Config: map[string]string{
						"storageAccount": "mystorageaccount",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "azure-creds",
						},
						Key: "credentials",
					},
				},
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "azure",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "my-azure-container",
							Prefix: "backup-prefix",
						},
					},
					Config: map[string]string{
						"region":         "eastus",
						"storageAccount": "mystorageaccount",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "azure-creds",
						},
						Key: "credentials",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "GCP provider mapping",
			bslSpec: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "gcp-bucket",
					},
					Prefix: "velero",
					CACert: []byte("test-ca-cert"),
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "gcp-creds",
						},
						Key: "credentials",
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcp-bucket",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Provider: oadpv1alpha1.GCPBucketProvider,
					Name:     "my-gcp-bucket",
				},
			},
			expectedBSL: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "gcp-bucket",
					},
					Prefix: "velero",
					CACert: []byte("test-ca-cert"),
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "gcp-creds",
						},
						Key: "credentials",
					},
				},
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "gcp",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "my-gcp-bucket",
							Prefix: "velero",
							CACert: []byte("test-ca-cert"),
						},
					},
					Config: map[string]string{},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "gcp-creds",
						},
						Key: "credentials",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "CloudStorage with enableSharedConfig",
			bslSpec: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "aws-shared-config",
					},
					Prefix: "backups",
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-shared-config",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Provider:           oadpv1alpha1.AWSBucketProvider,
					Name:               "shared-bucket",
					EnableSharedConfig: pointer.Bool(true),
				},
			},
			expectedBSL: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "aws-shared-config",
					},
					Prefix: "backups",
				},
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "shared-bucket",
							Prefix: "backups",
						},
					},
					Config: map[string]string{
						"enableSharedConfig": "true",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Config merging - CloudStorage config overridden by BSL config",
			bslSpec: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "aws-bucket-override",
					},
					Config: map[string]string{
						"region":               "eu-west-1", // This should override
						"serverSideEncryption": "AES256",
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-bucket-override",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Provider: oadpv1alpha1.AWSBucketProvider,
					Name:     "override-bucket",
					Region:   "us-east-1", // This will be overridden
				},
			},
			expectedBSL: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "aws-bucket-override",
					},
					Config: map[string]string{
						"region":               "eu-west-1",
						"serverSideEncryption": "AES256",
					},
				},
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "override-bucket",
						},
					},
					Config: map[string]string{
						"region":               "eu-west-1",
						"serverSideEncryption": "AES256",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Empty CloudStorage reference",
			bslSpec: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "",
					},
				},
			},
			wantErr:  true,
			errorMsg: "CloudStorage reference is required",
		},
		{
			name: "CloudStorage not found",
			bslSpec: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "non-existent",
					},
				},
			},
			wantErr:  true,
			errorMsg: "failed to get CloudStorage non-existent",
		},
		{
			name: "Unsupported provider",
			bslSpec: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "unsupported-provider",
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unsupported-provider",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Provider: "unsupported",
					Name:     "test-bucket",
				},
			},
			wantErr:  true,
			errorMsg: "unsupported CloudStorage provider: unsupported",
		},
		{
			name: "Velero spec already exists - should be preserved and updated",
			bslSpec: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "aws-bucket-existing",
					},
				},
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "oldprovider",
					Config: map[string]string{
						"oldkey": "oldvalue",
					},
				},
			},
			cloudStorage: &oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-bucket-existing",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.CloudStorageSpec{
					Provider: oadpv1alpha1.AWSBucketProvider,
					Name:     "existing-bucket",
					Region:   "us-west-2",
				},
			},
			expectedBSL: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "aws-bucket-existing",
					},
				},
				Velero: &velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "existing-bucket",
						},
					},
					Config: map[string]string{
						"oldkey": "oldvalue",
						"region": "us-west-2",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = oadpv1alpha1.AddToScheme(scheme)

			objs := []client.Object{}
			if tt.cloudStorage != nil {
				objs = append(objs, tt.cloudStorage)
			}

			fakeClient := getFakeClientFromObjectsForTest(t, objs...)

			r := &DataProtectionApplicationReconciler{
				Client:  fakeClient,
				Scheme:  scheme,
				Context: context.Background(),
				Log:     logr.Discard(),
			}

			err := r.populateBSLFromCloudStorage(tt.bslSpec, "test-ns")

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedBSL, tt.bslSpec)
			}
		})
	}
}

func TestDPAReconciler_ensureSecretDataExists_CloudStorage(t *testing.T) {
	tests := []struct {
		name    string
		dpa     *oadpv1alpha1.DataProtectionApplication
		bsl     *oadpv1alpha1.BackupLocation
		secret  *corev1.Secret
		objects []client.Object
		wantErr bool
		errMsg  string
	}{
		{
			name: "CloudStorage with valid AWS credentials",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: pointer.Bool(true),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			bsl: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "aws-cs",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-secret",
						},
						Key: "credentials",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-secret",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"credentials": []byte(`[default]
aws_access_key_id=AKIAIOSFODNN7EXAMPLE
aws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`),
				},
			},
			objects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "aws-cs",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.AWSBucketProvider,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "CloudStorage with AWS profile in config",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: pointer.Bool(true),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			bsl: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "aws-cs-profile",
					},
					Config: map[string]string{
						"profile": "custom-profile",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-secret-profile",
						},
						Key: "credentials",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-secret-profile",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"credentials": []byte(`[custom-profile]
aws_access_key_id=AKIAIOSFODNN7EXAMPLE
aws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`),
				},
			},
			objects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "aws-cs-profile",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.AWSBucketProvider,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "CloudStorage with Azure credentials",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: pointer.Bool(true),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			bsl: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "azure-cs",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "azure-secret",
						},
						Key: "credentials",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-secret",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"credentials": []byte(`AZURE_STORAGE_ACCOUNT_ACCESS_KEY=your_key
AZURE_CLOUD_NAME=AzurePublicCloud`),
				},
			},
			objects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "azure-cs",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.AzureBucketProvider,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "CloudStorage without credentials",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			bsl: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "no-cred-cs",
					},
					Credential: nil,
				},
			},
			wantErr: true,
			errMsg:  "must provide a valid credential secret",
		},
		{
			name: "CloudStorage with empty credential name",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			bsl: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "empty-name-cs",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "must provide a valid credential secret name",
		},
		{
			name: "CloudStorage with empty credential key",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			bsl: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "empty-key-cs",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "some-secret",
						},
						Key: "",
					},
				},
			},
			wantErr: true,
			errMsg:  "must provide a valid credential secret key",
		},
		{
			name: "CloudStorage not found",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			bsl: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "non-existent-cs",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "some-secret",
						},
						Key: "credentials",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-secret",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"credentials": []byte("test"),
				},
			},
			wantErr: true,
			errMsg:  "not found",
		},
		{
			name: "CloudStorage with invalid AWS secret format",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: pointer.Bool(true),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			bsl: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "aws-cs-invalid",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "aws-secret-invalid",
						},
						Key: "credentials",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-secret-invalid",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"credentials": []byte("invalid-format"),
				},
			},
			objects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "aws-cs-invalid",
						Namespace: "test-ns",
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						Provider: oadpv1alpha1.AWSBucketProvider,
					},
				},
			},
			wantErr: true,
			errMsg:  "error parsing AWS secret",
		},
		{
			name: "CloudStorage with no-secret feature flag",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							FeatureFlags: []string{"no-secret"},
						},
					},
				},
			},
			bsl: &oadpv1alpha1.BackupLocation{
				CloudStorage: &oadpv1alpha1.CloudStorageLocation{
					CloudStorageRef: corev1.LocalObjectReference{
						Name: "no-secret-cs",
					},
				},
			},
			wantErr: false, // Should not validate secrets when no-secret flag is set
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := tt.objects
			if tt.secret != nil {
				objs = append(objs, tt.secret)
			}

			fakeClient := getFakeClientFromObjectsForTest(t, objs...)

			r := &DataProtectionApplicationReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Context: context.Background(),
				Log:     logr.Discard(),
				dpa:     tt.dpa,
				NamespacedName: types.NamespacedName{
					Namespace: tt.dpa.Namespace,
					Name:      tt.dpa.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
			}

			err := r.ensureSecretDataExists(tt.bsl)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProcessCACertForBSLs(t *testing.T) {
	testCACertPEM := `-----BEGIN CERTIFICATE-----
MIIDNzCCAh+gAwIBAgIJAJ7qAHESwpNwMA0GCSqGSIb3DQEBCwUAMDMxMTAvBgNV
BAMMKGVjMi01NC0yMTEtOC0yNDguY29tcHV0ZS0xLmFtYXpvbmF3cy5jb20wHhcN
MjUwODI1MjA0NjA2WhcNMjYwODI1MjA0NjA2WjAzMTEwLwYDVQQDDChIYzItNTQt
MjExLTgtMjQ4LmNvbXB1dGUtMS5hbWF6b25hd3MuY29tMIIBIjANBgkqhkiG9w0B
AQEFAAOCAQSAMIIBCgKCAQEArowngodR8QhYPphdTalrwVqHow4N5m9GMko774J2
LWgSjYcpuaR3FEYMjzIzVCQWts/J9mqd8rYagYOfP9azYO+U96/ztoiJVMld2R+p
QK/2MzdvZNXD2mi/9MpaS40HFh8ifd07mcFMt+qzKb4VgauS1jJAuzXHS7VElqwZ
vi4v0yvh6T3C2bdXouBwibFe5jGnzsGmNWq7S/+Litynx2HDNcZGbCyQE1xZ1+B6
QPmvgmO5LPpFlBQmu7aDePXxt76BJbrQrmUloNRqwlk4n9jYLic/FJtWw1kjp7fB
Pa86W2GlMreSNlzI5ViUhoVYEB2sdsXesi4JK6KW3baiRwIDAQABo04wTDBKBgNV
HREEQTBM----END CERTIFICATE-----`

	tests := []struct {
		name              string
		backupLocations   []oadpv1alpha1.BackupLocation
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
			wantConfigMapName: "",
			wantError:         false,
		},
		{
			name:              "No BSLs configured",
			backupLocations:   []oadpv1alpha1.BackupLocation{},
			wantConfigMapName: "",
			wantError:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test DPA
			dpa := &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-namespace",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: tt.backupLocations,
				},
			}

			// Create fake client with the DPA
			fakeClient := getFakeClientFromObjectsForTest(t, dpa)

			// Create reconciler
			r := &DataProtectionApplicationReconciler{
				Client:        fakeClient,
				Scheme:        fakeClient.Scheme(),
				Log:           logr.Discard(),
				Context:       context.Background(),
				EventRecorder: record.NewFakeRecorder(10),
				NamespacedName: types.NamespacedName{
					Name:      dpa.Name,
					Namespace: dpa.Namespace,
				},
				dpa: dpa,
			}

			// Test the function
			gotConfigMapName, err := r.processCACertForBSLs()

			// Check error expectation
			if tt.wantError {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}

			// Check ConfigMap name
			assert.Equal(t, tt.wantConfigMapName, gotConfigMapName)

			// If we expect a ConfigMap, verify it was created with correct content
			if tt.wantConfigMapName != "" {
				configMap := &corev1.ConfigMap{}
				err := fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      tt.wantConfigMapName,
					Namespace: dpa.Namespace,
				}, configMap)
				assert.NoError(t, err)

				// Verify ConfigMap contains the CA certificate
				assert.Contains(t, configMap.Data, caBundleFileName)
				assert.Equal(t, testCACertPEM, configMap.Data[caBundleFileName])

				// Verify labels are set correctly
				assert.Equal(t, common.Velero, configMap.Labels["app.kubernetes.io/name"])
				assert.Equal(t, common.OADPOperator, configMap.Labels["app.kubernetes.io/managed-by"])
				assert.Equal(t, "ca-bundle", configMap.Labels["app.kubernetes.io/component"])
				assert.Equal(t, "True", configMap.Labels[oadpv1alpha1.OadpOperatorLabel])
			}
		})
	}
}

// Helper function to create fake client for tests
func getFakeClientFromObjectsForTest(t *testing.T, objs ...client.Object) client.WithWatch {
	testScheme, err := getSchemeForFakeClient()
	if err != nil {
		t.Fatalf("error getting scheme for fake client: %v", err)
	}

	return fake.NewClientBuilder().WithScheme(testScheme).WithObjects(objs...).Build()
}

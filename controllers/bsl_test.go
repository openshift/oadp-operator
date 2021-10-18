package controllers

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"

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

func TestVeleroReconciler_ValidateBackupStorageLocations(t *testing.T) {
	tests := []struct {
		name     string
		VeleroCR *oadpv1alpha1.Velero
		secret   *corev1.Secret
		want     bool
		wantErr  bool
	}{
		{
			name: "test no BSLs, no NoDefaultBackupLocation",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{},
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					NoDefaultBackupLocation: true,
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
							Provider: "aws",
							Config: map[string]string{
								Region: "us-east-1",
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
							Provider: "foo",
							Config: map[string]string{
								Region: "us-east-1",
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
							Config: map[string]string{
								Region: "us-east-1",
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
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
						{
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
						{
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
			name:     "test get error",
			VeleroCR: &oadpv1alpha1.Velero{},
			want:     false,
			wantErr:  true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: "test-ns",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(tt.VeleroCR, tt.secret)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &VeleroReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(tt.name),
				NamespacedName: types.NamespacedName{
					Namespace: tt.VeleroCR.Namespace,
					Name:      tt.VeleroCR.Name,
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

func TestVeleroReconciler_updateBSLFromSpec(t *testing.T) {
	tests := []struct {
		name    string
		bsl     *velerov1.BackupStorageLocation
		velero  *oadpv1alpha1.Velero
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
			velero: &oadpv1alpha1.Velero{
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
			r := &VeleroReconciler{
				Scheme: scheme,
			}

			wantBSl := &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-1",
					Namespace: "bar",
					Labels: map[string]string{
						"app.kubernetes.io/name":     "oadp-operator-velero",
						"app.kubernetes.io/instance": tt.velero.Name + "-1",
						//"app.kubernetes.io/version":    "x.y.z",
						"app.kubernetes.io/managed-by": "oadp-operator",
						"app.kubernetes.io/component":  "bsl",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         oadpv1alpha1.SchemeBuilder.GroupVersion.String(),
						Kind:               "Velero",
						Name:               tt.velero.Name,
						UID:                tt.velero.UID,
						Controller:         pointer.BoolPtr(true),
						BlockOwnerDeletion: pointer.BoolPtr(true),
					}},
				},
			}

			err = r.updateBSLFromSpec(tt.bsl, tt.velero, tt.bsl.Spec)
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

func TestVeleroReconciler_ensureBSLProviderMapping(t *testing.T) {
	type fields struct {
		Client         client.Client
		Scheme         *runtime.Scheme
		Log            logr.Logger
		Context        context.Context
		NamespacedName types.NamespacedName
		EventRecorder  record.EventRecorder
	}
	type args struct {
		velero *oadpv1alpha1.Velero
	}
	tests := []struct {
		name     string
		VeleroCR *oadpv1alpha1.Velero
		wantErr  bool
	}{
		{
			name: "one bsl configured per provider",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
							Provider: "aws",
						},
						{
							Provider: "azure",
						},
						{
							Provider: "gcp",
						},
						{
							Provider: "thirdpary-objectstorage-provider",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "two bsl configured for aws provider",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
							Provider: "aws",
						},
						{
							Provider: "azure",
						},
						{
							Provider: "aws",
						},
						{
							Provider: "thirdpary-objectstorage-provider",
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, err := getSchemeForFakeClient()
			if err != nil {
				t.Errorf("error getting scheme for the test: %#v", err)
			}
			r := &VeleroReconciler{
				Scheme: scheme,
			}
			if err := r.ensureBSLProviderMapping(tt.VeleroCR); (err != nil) != tt.wantErr {
				t.Errorf("ensureBSLProviderMapping() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

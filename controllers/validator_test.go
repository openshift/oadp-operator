package controllers

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	logrTesting "github.com/go-logr/logr/testing"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	oadpScheme "github.com/openshift/oadp-operator/pkg/scheme"
	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	discoverFake "k8s.io/client-go/discovery/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientFake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDPAReconciler_ValidateDataProtectionCR(t *testing.T) {
	tests := []struct {
		name    string
		dpa     *oadpv1alpha1.DataProtectionApplication
		objects []client.Object
		want    bool
		wantErr bool
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
			want:    true,
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
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
				},
			},
			wantErr: false,
			want:    true,
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
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
				},
			},
			wantErr: true,
			want:    false,
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
			objects: []client.Object{},
			wantErr: true,
			want:    false,
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
			objects: []client.Object{},
			wantErr: true,
			want:    false,
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
			wantErr: true,
			want:    false,
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
			wantErr: true,
			want:    false,
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
										Name: "testing",
									},
									Key: "credentials",
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
			wantErr: false,
			want:    true,
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
										Name: "testing",
									},
									Key: "credentials",
								},
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
			wantErr: false,
			want:    true,
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
										Name: "testing",
									},
									Key: "credentials",
								},
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
			},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloud-credentials",
						Namespace: "test-ns",
					},
				},
			},
			wantErr: false,
			want:    true,
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
							},
						},
					},
					SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &v1.VolumeSnapshotLocationSpec{
								Provider: "aws",
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
			objects: []client.Object{},
			wantErr: false,
			want:    true,
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
			wantErr: true,
			want:    false,
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
							},
						},
					},
					SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &v1.VolumeSnapshotLocationSpec{
								Provider: "aws",
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
				},
			},
			wantErr: false,
			want:    true,
		},
		{
			name: "given valid DPA CR BSL configured and GCP Default Plugin without secret",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
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
			objects: []client.Object{},
			wantErr: true,
			want:    false,
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
			objects: []client.Object{},
			wantErr: true,
			want:    false,
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
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "Test",
									},
									Key:      "Creds",
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
			objects: []client.Object{},
			wantErr: false,
			want:    true,
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
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "Test",
									},
									Key:      "Creds",
									Optional: new(bool),
								},
							},
						},
						{
							Velero: &v1.BackupStorageLocationSpec{
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
			objects: []client.Object{},
			wantErr: true,
			want:    false,
		},
		{
			name: "given valid DPA CR AWS Default Plugin with credentials and a VSL",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-DPA-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &v1.BackupStorageLocationSpec{
								Provider: "velero.io/aws",
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key:      "cloud",
									Optional: new(bool),
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
			objects: []client.Object{},
			wantErr: false,
			want:    true,
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
			EventRecorder: record.NewFakeRecorder(10),
		}
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.ValidateDataProtectionCR(r.Log)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDataProtectionCR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateDataProtectionCR() got = %v, want %v", got, tt.want)
			}
		})
	}
}

var namespacedName = types.NamespacedName{
	Namespace: "test-ns",
	Name:      "test-DPA-CR",
}
func TestDPAReconciler_ValidateVeleroPlugins(t *testing.T) {
	type fields struct {
		objects []runtime.Object
		FakedServerVersion *version.Info
		Scheme             *runtime.Scheme
		NamespacedName     types.NamespacedName
		EventRecorder      record.EventRecorder
	}
	tests := []struct {
		name    string
		fields  fields
		want    bool
		wantErr bool
	}{
		{
			name: "given valid DPA CR with AWS, CSI Default Plugin and secret",
			fields: fields{
				objects: []runtime.Object{
					&oadpv1alpha1.DataProtectionApplication{
						TypeMeta: metav1.TypeMeta{
							Kind:       "DataProtectionApplication",
							APIVersion: "oadp.openshift.io/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-DPA-CR",
							Namespace: "test-ns",
						},
						Spec: oadpv1alpha1.DataProtectionApplicationSpec{
							Configuration: &oadpv1alpha1.ApplicationConfig{
								Velero: &oadpv1alpha1.VeleroConfig{
									DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
										oadpv1alpha1.DefaultPluginAWS,
										oadpv1alpha1.DefaultPluginCSI,
									},
								},
							},
						},
					},
					// secret
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "cloud-credentials",
							Namespace: "test-ns",
						},
						Data: map[string][]byte{
							"cloud": []byte("test"),
						},
					},
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "Cluster version too high for CSI plugin",
			fields: fields{
				objects: []runtime.Object{
					&oadpv1alpha1.DataProtectionApplication{
						TypeMeta: metav1.TypeMeta{
							Kind:       "DataProtectionApplication",
							APIVersion: "oadp.openshift.io/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-DPA-CR",
							Namespace: "test-ns",
						},
						Spec: oadpv1alpha1.DataProtectionApplicationSpec{
							Configuration: &oadpv1alpha1.ApplicationConfig{
								Velero: &oadpv1alpha1.VeleroConfig{
									DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
										oadpv1alpha1.DefaultPluginAWS,
										oadpv1alpha1.DefaultPluginCSI,
									},
								},
							},
						},
					},
					// secret
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "cloud-credentials",
							Namespace: "test-ns",
						},
						Data: map[string][]byte{
							"cloud": []byte("test"),
						},
					},
				},
				FakedServerVersion: &version.Info{
					Major: "1",
					Minor: "24",
				},
			},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.fields.FakedServerVersion == nil {
				tt.fields.FakedServerVersion = &version.Info{
					Major: "1",
					Minor: "23",
				}
			}
			log := logrTesting.TestLogger{T: t}
			scheme := runtime.NewScheme()
			oadpScheme.AddToScheme(scheme, log)
			r := &DPAReconciler{
				Client: clientFake.NewFakeClientWithScheme(scheme, tt.fields.objects...),
				DiscoveryInterface: &discoverFake.FakeDiscovery{
					Fake:               &clienttesting.Fake{},
					FakedServerVersion: tt.fields.FakedServerVersion},
				Log:     log,
				NamespacedName: namespacedName,
				Context: context.Background(),
			}
			got, err := r.ValidateVeleroPlugins(r.Log)
			if (err != nil) != tt.wantErr {
				t.Errorf("DPAReconciler.ValidateVeleroPlugins() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DPAReconciler.ValidateVeleroPlugins() = %v, want %v", got, tt.want)
			}
		})
	}
}

package controllers

import (
	"testing"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

func TestVeleroReconciler_ValidateVolumeSnapshotLocation(t *testing.T) {
	tests := []struct {
		name     string
		VeleroCR *oadpv1alpha1.Velero
		secret   *corev1.Secret
		want     bool
		wantErr  bool
	}{
		{
			name: "test no VSLs specified",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{},
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

		// AWS tests
		{
			name: "test AWS VSL with only region specified",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: AWSProvider,
							Config: map[string]string{
								Region: "us-east-1",
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
			name: "test AWS VSL with no region specified",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: AWSProvider,
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
			name: "test AWS VSL with region and profile specified",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: AWSProvider,
							Config: map[string]string{
								Region:     "us-east-1",
								AWSProfile: "test-profile",
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
			name: "test AWS VSL with region specified and invalid config value",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: AWSProvider,
							Config: map[string]string{
								Region:         "us-east-1",
								"invalid-test": "foo",
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

		// GCP tests
		{
			name: "test GCP VSL with no config values",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: GCPProvider,
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
			name: "test GCP VSL with snapshotLocation specified",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: GCPProvider,
							Config: map[string]string{
								GCPSnapshotLocation: "test-location",
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
			name: "test GCP VSL with project specified",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: GCPProvider,
							Config: map[string]string{
								GCPProject: "alt-project",
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
			name: "test GCP VSL with invalid config value",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: GCPProvider,
							Config: map[string]string{
								"invalid-test": "foo",
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

		// Azure tests
		{
			name: "test Azure VSL with no config values",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: AzureProvider,
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
			name: "test Azure VSL with apiTimeout specified",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: AzureProvider,
							Config: map[string]string{
								AzureApiTimeout: "5m",
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
			name: "test Azure VSL with resourceGroup specified",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: AzureProvider,
							Config: map[string]string{
								ResourceGroup: "test-rg",
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
			name: "test Azure VSL with subscriptionId specified",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: AzureProvider,
							Config: map[string]string{
								AzureSubscriptionId: "test-alt-sub",
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
			name: "test Azure VSL with incremental specified",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: AzureProvider,
							Config: map[string]string{
								AzureIncremental: "false",
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
			name: "test AzureVSL with invalid config value",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-VSL",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					VolumeSnapshotLocations: []velerov1.VolumeSnapshotLocationSpec{
						{
							Provider: GCPProvider,
							Config: map[string]string{
								"invalid-test": "foo",
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
			got, err := r.ValidateVolumeSnapshotLocations(r.Log)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVolumeSnapshotLocations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateVolumeSnapshotLocations() got %v, want %v", got, tt.want)
			}
		})
	}

}

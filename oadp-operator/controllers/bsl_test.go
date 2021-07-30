package controllers

import (
	"context"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
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
		want     bool
		wantErr  bool
	}{
		{
			name: "test no BSLs, no noobaa",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: oadpv1alpha1.VeleroSpec{},
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "test BSLs specified, no noobaa",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
						{
							// TODO: foo is invalid provider, add test cases for it
							Provider: "foo",
						},
					},
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "test no BSL, noobaa configured",
			VeleroCR: &oadpv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: oadpv1alpha1.VeleroSpec{
					Noobaa: true,
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name:     "test get error",
			VeleroCR: &oadpv1alpha1.Velero{},
			want:     false,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(tt.VeleroCR)
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

func TestVeleroReconciler_ReconcileBackupStorageLocations(t *testing.T) {
	tests := []struct {
		name     string
		objs     []client.Object
		wantBSLs []velerov1.BackupStorageLocation
		want     bool
		wantErr  bool
	}{
		{
			name: "test 1 BSLs configured, create, no noobaa",
			objs: []client.Object{
				&oadpv1alpha1.Velero{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "bar",
					},
					Spec: oadpv1alpha1.VeleroSpec{
						BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
							{
								Provider: "foo",
							},
						},
					},
				},
			},
			wantErr: false,
			want:    true,
			wantBSLs: []velerov1.BackupStorageLocation{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-1",
						Namespace: "bar",
					},
					Spec: velerov1.BackupStorageLocationSpec{
						Provider: "foo",
					},
				},
			},
		},
		{
			name: "test 1 BSLs configured, updated, no noobaa",
			objs: []client.Object{
				&oadpv1alpha1.Velero{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "bar",
					},
					Spec: oadpv1alpha1.VeleroSpec{
						BackupStorageLocations: []velerov1.BackupStorageLocationSpec{
							{
								Provider: "foo",
							},
						},
					},
				},
				&velerov1.BackupStorageLocation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-1",
						Namespace: "bar",
					},
				},
			},
			wantErr: false,
			want:    true,
			wantBSLs: []velerov1.BackupStorageLocation{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-1",
						Namespace: "bar",
					},
					Spec: velerov1.BackupStorageLocationSpec{
						Provider: "foo",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(tt.objs...)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			fakeRecorder := record.NewFakeRecorder(10)
			r := &VeleroReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(tt.name),
				NamespacedName: types.NamespacedName{
					Namespace: tt.objs[0].GetNamespace(),
					Name:      tt.objs[0].GetName(),
				},
				EventRecorder: fakeRecorder,
			}
			got, err := r.ReconcileBackupStorageLocations(r.Log)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileBackupStorageLocations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ReconcileBackupStorageLocations() got = %v, want %v", got, tt.want)
			}

			for _, obj := range tt.wantBSLs {
				gotObj := velerov1.BackupStorageLocation{}
				err := fakeClient.Get(r.Context, client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}, &gotObj)
				if err != nil {
					t.Errorf("error getting bsl for asserting, %#v", err)
				}
				if gotObj.Namespace != obj.Namespace && gotObj.Name != obj.Name && reflect.DeepEqual(gotObj.Spec, obj.Spec) && len(gotObj.OwnerReferences) == 0 {
					t.Errorf("got %#v, want %#v, with owner reference", gotObj, obj)
				}
			}
		})
	}
}

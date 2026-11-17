package lib

import (
	"testing"
	"time"

	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBSLsAreUpdated(t *testing.T) {
	updateTime := time.Date(2026, 10, 20, 5, 0, 0, 0, time.UTC)
	validatedBefore := metav1.NewTime(updateTime.Add(-time.Minute))
	validatedAfter := metav1.NewTime(updateTime.Add(time.Minute))

	newBSL := func(name string, lastValidation *metav1.Time) *velero.BackupStorageLocation {
		return &velero.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "openshift-adp"},
			Status:     velero.BackupStorageLocationStatus{LastValidationTime: lastValidation},
		}
	}

	tests := []struct {
		name string
		bsls []*velero.BackupStorageLocation
		want bool
	}{
		{
			name: "BSL not yet validated by Velero is not updated",
			bsls: []*velero.BackupStorageLocation{newBSL("bsl-1", nil)},
			want: false,
		},
		{
			name: "one validated and one not yet validated BSL is not updated",
			bsls: []*velero.BackupStorageLocation{newBSL("bsl-1", &validatedAfter), newBSL("bsl-2", nil)},
			want: false,
		},
		{
			name: "BSL validated before the update is not updated",
			bsls: []*velero.BackupStorageLocation{newBSL("bsl-1", &validatedBefore)},
			want: false,
		},
		{
			name: "all BSLs validated after the update are updated",
			bsls: []*velero.BackupStorageLocation{newBSL("bsl-1", &validatedAfter), newBSL("bsl-2", &validatedAfter)},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := velero.AddToScheme(scheme); err != nil {
				t.Fatalf("failed to add velero types to scheme: %v", err)
			}
			objs := make([]runtime.Object, 0, len(tt.bsls))
			for _, bsl := range tt.bsls {
				objs = append(objs, bsl)
			}
			dpa := &DpaCustomResource{
				Namespace: "openshift-adp",
				Client:    fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build(),
			}

			got, err := dpa.BSLsAreUpdated(updateTime)()
			if err != nil {
				t.Fatalf("BSLsAreUpdated() returned unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("BSLsAreUpdated() = %v, want %v", got, tt.want)
			}
		})
	}
}

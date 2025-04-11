package controller

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

func TestDataProtectionApplicationReconciler_updateBackupRepositoryCM(t *testing.T) {
	tests := []struct {
		name   string
		cm     *corev1.ConfigMap
		dpa    *oadpv1alpha1.DataProtectionApplication
		wantCM *corev1.ConfigMap
	}{
		{
			name: "backup repository cm is updated successfully with full config",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backup-repository-test-dpa",
					Namespace: "test-ns",
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					BackupImages: ptr.To(false),
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							KopiaRepoOptions: oadpv1alpha1.KopiaRepoOptions{
								CacheLimitMB:            ptr.To(int64(4096)),
								FullMaintenanceInterval: "fastGC",
							},
						},
					},
				},
			},
			wantCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backup-repository-test-dpa",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/instance":   "test-dpa",
						"app.kubernetes.io/managed-by": "oadp-operator",
						"app.kubernetes.io/component":  "backup-repository-config",
						"openshift.io/oadp":            "True",
					},
				},
				Data: map[string]string{
					"kopia": `{"cacheLimitMB":4096,"fullMaintenanceInterval":"fastGC"}`,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(tt.cm, tt.dpa)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DataProtectionApplicationReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: context.Background(),
				NamespacedName: types.NamespacedName{
					Namespace: tt.cm.Namespace,
					Name:      tt.cm.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
				dpa:           tt.dpa,
			}

			err = r.updateBackupRepositoryCM(tt.cm)
			require.NoError(t, err)
			require.Equal(t, tt.wantCM.ObjectMeta.Name, tt.cm.ObjectMeta.Name, "ConfigMap Name does not match")
			require.Equal(t, tt.wantCM.ObjectMeta.Namespace, tt.cm.ObjectMeta.Namespace, "ConfigMap Namespace does not match")
			require.Equal(t, tt.wantCM.ObjectMeta.Labels, tt.cm.ObjectMeta.Labels, "ConfigMap Labels do not match")

			// Compare Data fields, we need to unmarshal the JSON to ignore key order
			expectedData := tt.wantCM.Data["kopia"]
			actualData := tt.cm.Data["kopia"]

			var expectedMap map[string]interface{}
			var actualMap map[string]interface{}

			require.NoError(t, json.Unmarshal([]byte(expectedData), &expectedMap), "Failed to unmarshal expected Data")
			require.NoError(t, json.Unmarshal([]byte(actualData), &actualMap), "Failed to unmarshal actual Data")
			require.Equal(t, expectedMap, actualMap, "ConfigMap Data does not match")
		})
	}
}

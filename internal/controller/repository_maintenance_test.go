package controller

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"github.com/vmware-tanzu/velero/pkg/util/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

func TestDataProtectionApplicationReconciler_updateRepositoryMaintenanceCM(t *testing.T) {
	tests := []struct {
		name   string
		cm     *corev1.ConfigMap
		dpa    *oadpv1alpha1.DataProtectionApplication
		wantCM *corev1.ConfigMap
	}{
		{
			name: "repository maintenance cm is updated successfully with full config",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "repository-maintenance-test-dpa",
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
						RepositoryMaintenance: map[string]oadpv1alpha1.RepositoryMaintenanceConfig{
							"global": {
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"app.kubernetes.io/name": "test-dpa"},
										},
									},
								},
								PodResources: &kube.PodResources{
									CPURequest:    "100m",
									MemoryRequest: "128Mi",
									CPULimit:      "200m",
									MemoryLimit:   "256Mi",
								},
							},
							"maintenance-job-1": {
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"app.kubernetes.io/name": "test-dpa"},
										},
									},
								},
							},
						},
					},
				},
			},
			wantCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "repository-maintenance-test-dpa",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/instance":   "test-dpa",
						"app.kubernetes.io/managed-by": "oadp-operator",
						"app.kubernetes.io/component":  "repository-maintenance-config",
						"openshift.io/oadp":            "True",
					},
				},
				Data: map[string]string{
					"global":            `{"loadAffinity":[{"nodeSelector":{"matchLabels":{"app.kubernetes.io/name":"test-dpa"}}}],"podResources":{"cpuRequest":"100m","memoryRequest":"128Mi","cpuLimit":"200m","memoryLimit":"256Mi","ephemeralStorageRequest":"0","ephemeralStorageLimit":"0"},"podLabels":{"oadp.openshift.io/network-policy":"velero"}}`,
					"maintenance-job-1": `{"loadAffinity":[{"nodeSelector":{"matchLabels":{"app.kubernetes.io/name":"test-dpa"}}}]}`,
				},
			},
		},
		{
			name: "repository maintenance cm is updated with StorageClass in LoadAffinity",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "repository-maintenance-test-dpa",
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
						RepositoryMaintenance: map[string]oadpv1alpha1.RepositoryMaintenanceConfig{
							"global": {
								LoadAffinityConfig: []*oadpv1alpha1.LoadAffinity{
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"node-type": "fast"},
										},
										StorageClass: "gp3-csi",
									},
									{
										NodeSelector: metav1.LabelSelector{
											MatchLabels: map[string]string{"node-type": "general"},
										},
									},
								},
							},
						},
					},
				},
			},
			wantCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "repository-maintenance-test-dpa",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/instance":   "test-dpa",
						"app.kubernetes.io/managed-by": "oadp-operator",
						"app.kubernetes.io/component":  "repository-maintenance-config",
						"openshift.io/oadp":            "True",
					},
				},
				Data: map[string]string{
					"global": `{"loadAffinity":[{"nodeSelector":{"matchLabels":{"node-type":"fast"}},"storageClass":"gp3-csi"},{"nodeSelector":{"matchLabels":{"node-type":"general"}}}],"podLabels":{"oadp.openshift.io/network-policy":"velero"}}`,
				},
			},
		},
		{
			name: "repository maintenance cm is updated with podLabels and podAnnotations",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "repository-maintenance-test-dpa",
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
						RepositoryMaintenance: map[string]oadpv1alpha1.RepositoryMaintenanceConfig{
							"global": {
								PodLabels: map[string]string{
									"network-access": "allowed",
								},
								PodAnnotations: map[string]string{
									"sidecar.istio.io/inject": "false",
								},
								PodResources: &kube.PodResources{
									CPURequest:    "100m",
									MemoryRequest: "128Mi",
								},
							},
						},
					},
				},
			},
			wantCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "repository-maintenance-test-dpa",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/instance":   "test-dpa",
						"app.kubernetes.io/managed-by": "oadp-operator",
						"app.kubernetes.io/component":  "repository-maintenance-config",
						"openshift.io/oadp":            "True",
					},
				},
				Data: map[string]string{
					"global": `{"podResources":{"cpuRequest":"100m","memoryRequest":"128Mi","cpuLimit":"0","memoryLimit":"0","ephemeralStorageRequest":"0","ephemeralStorageLimit":"0"},"podAnnotations":{"sidecar.istio.io/inject":"false"},"podLabels":{"network-access":"allowed","oadp.openshift.io/network-policy":"velero"}}`,
				},
			},
		},
		{
			// No RepositoryMaintenance config supplied by the user at all. OADP must still
			// always produce a "global" entry carrying networkPolicyMoverLabel, so
			// repository-maintenance job pods (which otherwise get no default labels) can be
			// selected by reconcileVeleroMoverNetworkPolicy.
			name: "repository maintenance cm always includes global networkPolicyMoverLabel even with no user config",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "repository-maintenance-test-dpa",
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
					},
				},
			},
			wantCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "repository-maintenance-test-dpa",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/instance":   "test-dpa",
						"app.kubernetes.io/managed-by": "oadp-operator",
						"app.kubernetes.io/component":  "repository-maintenance-config",
						"openshift.io/oadp":            "True",
					},
				},
				Data: map[string]string{
					"global": `{"podLabels":{"oadp.openshift.io/network-policy":"velero"}}`,
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

			var dpaSpecBeforeTest = oadpv1alpha1.DataProtectionApplicationSpec{}
			if tt.dpa != nil {
				// Snapshot the DPA Spec before calling updateRepositoryMaintenanceCM,
				// Required to test the DPA is unchanged.
				dpaSpecBeforeTest = *tt.dpa.Spec.DeepCopy()
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

			err = r.updateRepositoryMaintenanceCM(tt.cm)
			require.NoError(t, err)
			require.Equal(t, tt.wantCM.ObjectMeta.Name, tt.cm.ObjectMeta.Name, "ConfigMap Name does not match")
			require.Equal(t, tt.wantCM.ObjectMeta.Namespace, tt.cm.ObjectMeta.Namespace, "ConfigMap Namespace does not match")
			require.Equal(t, tt.wantCM.ObjectMeta.Labels, tt.cm.ObjectMeta.Labels, "ConfigMap Labels do not match")

			// Compare Data fields, we need to unmarshal the JSON to ignore key order
			require.Equal(t, len(tt.wantCM.Data), len(tt.cm.Data), "ConfigMap Data key count does not match")

			for key, expectedData := range tt.wantCM.Data {
				actualData, exists := tt.cm.Data[key]
				require.True(t, exists, "ConfigMap Data key %s not found", key)

				var expectedMap map[string]interface{}
				var actualMap map[string]interface{}

				require.NoError(t, json.Unmarshal([]byte(expectedData), &expectedMap), "Failed to unmarshal expected Data for key %s", key)
				require.NoError(t, json.Unmarshal([]byte(actualData), &actualMap), "Failed to unmarshal actual Data for key %s", key)
				require.Equal(t, expectedMap, actualMap, "ConfigMap Data does not match for key %s", key)
			}

			// Require that updateRepositoryMaintenanceCM did not mutate the DPA Spec.
			// PodResource output will not match the original object if not all fields are set.
			if tt.dpa != nil {
				require.Truef(t, reflect.DeepEqual(tt.dpa.Spec, dpaSpecBeforeTest),
					"updateRepositoryMaintenanceCM must not modify the DPA Spec: diff=%s",
					cmp.Diff(dpaSpecBeforeTest, tt.dpa.Spec))
			}
		})
	}
}

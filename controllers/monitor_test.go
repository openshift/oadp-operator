package controllers

import (
	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	monitor "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func getSchemeForFakeClientForMonitor() (*runtime.Scheme, error) {
	err := oadpv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	err = velerov1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	err = monitor.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	err = rbacv1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	return scheme.Scheme, nil
}

func getFakeClientFromObjectsForMonitor(objs ...client.Object) (client.WithWatch, error) {
	schemeForFakeClient, err := getSchemeForFakeClientForMonitor()
	if err != nil {
		return nil, err
	}

	return fake.NewClientBuilder().WithScheme(schemeForFakeClient).WithObjects(objs...).Build(), nil
}

func TestDPAReconciler_buildVeleroServiceMonitor(t *testing.T) {
	tests := []struct {
		name                     string
		serviceMonitor           *monitor.ServiceMonitor
		dpa                      *oadpv1alpha1.DataProtectionApplication
		wantErr                  bool
		wantVeleroServiceMonitor *monitor.ServiceMonitor
	}{
		{
			name: "given appropriate dpa valid service monitor is built",
			serviceMonitor: &monitor.ServiceMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openshift-adp-velero-metrics-sm",
					Namespace: "test-ns",
				},
				Spec: monitor.ServiceMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-dpa",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
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
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "bucket-123",
								},
								Config: map[string]string{},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "creds",
								},
								Default:          false,
								BackupSyncPeriod: &metav1.Duration{},
							},
						},
					},
				},
			},
			wantErr: false,
			wantVeleroServiceMonitor: &monitor.ServiceMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openshift-adp-velero-metrics-sm",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/name":       common.Velero,
						"app.kubernetes.io/instance":   "test-dpa",
						"app.kubernetes.io/managed-by": common.OADPOperator,
						"app.kubernetes.io/component":  Server,
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				Spec: monitor.ServiceMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name":       common.Velero,
							"app.kubernetes.io/instance":   "test-dpa",
							"app.kubernetes.io/managed-by": common.OADPOperator,
							"app.kubernetes.io/component":  Server,
							oadpv1alpha1.OadpOperatorLabel: "True",
						},
					},
					Endpoints: []monitor.Endpoint{
						{
							Interval: "30s",
							Port:     "monitoring",
						},
					},
					NamespaceSelector: monitor.NamespaceSelector{
						MatchNames: []string{
							"test-ns",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjectsForMonitor(tt.serviceMonitor, tt.dpa)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(tt.name),
				NamespacedName: types.NamespacedName{
					Namespace: tt.serviceMonitor.Namespace,
					Name:      tt.serviceMonitor.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
			}

			err = r.buildVeleroServiceMonitor(tt.serviceMonitor, tt.dpa)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildVeleroServiceMonitor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(tt.wantVeleroServiceMonitor.Labels, tt.serviceMonitor.Labels) {
				t.Errorf("expected velero service monitor labels to be %#v, got %#v", tt.wantVeleroServiceMonitor.Labels, tt.serviceMonitor.Labels)
			}
			if !reflect.DeepEqual(tt.wantVeleroServiceMonitor.Spec, tt.serviceMonitor.Spec) {
				t.Errorf("expected velero service monitor spec to be %#v, got %#v", tt.wantVeleroServiceMonitor.Spec, tt.serviceMonitor.Spec)
			}
		})
	}
}

func TestDPAReconciler_updateVeleroMetricsSVC(t *testing.T) {
	tests := []struct {
		name                string
		svc                 *corev1.Service
		dpa                 *oadpv1alpha1.DataProtectionApplication
		wantErr             bool
		wantVeleroMtricsSVC *corev1.Service
	}{
		{
			name: "velero metrics svc gets updated for a valid dpa",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openshift-adp-velero-metrics-svc",
					Namespace: "test-ns",
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
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
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "bucket-123",
								},
								Config: map[string]string{},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "creds",
								},
								Default:          false,
								BackupSyncPeriod: &metav1.Duration{},
							},
						},
					},
				},
			},
			wantErr: false,
			wantVeleroMtricsSVC: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openshift-adp-velero-metrics-svc",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/name":       common.Velero,
						"app.kubernetes.io/instance":   "test-dpa",
						"app.kubernetes.io/managed-by": common.OADPOperator,
						"app.kubernetes.io/component":  Server,
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"app.kubernetes.io/name":       common.Velero,
						"app.kubernetes.io/instance":   "test-dpa",
						"app.kubernetes.io/managed-by": common.OADPOperator,
						"app.kubernetes.io/component":  Server,
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{
						{
							Name: "monitoring",
							Port: int32(8085),
							TargetPort: intstr.IntOrString{
								IntVal: int32(8085),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjectsForMonitor(tt.svc, tt.dpa)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(tt.name),
				NamespacedName: types.NamespacedName{
					Namespace: tt.svc.Namespace,
					Name:      tt.svc.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
			}

			err = r.updateVeleroMetricsSVC(tt.svc, tt.dpa)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildVeleroServiceMonitor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(tt.wantVeleroMtricsSVC.Labels, tt.svc.Labels) {
				t.Errorf("expected velero metrics svc labels to be %#v, got %#v", tt.wantVeleroMtricsSVC.Labels, tt.svc.Labels)
			}
			if !reflect.DeepEqual(tt.wantVeleroMtricsSVC.Spec, tt.svc.Spec) {
				t.Errorf("expected velero metrics svc spec to be %#v, got %#v", tt.wantVeleroMtricsSVC.Spec, tt.svc.Spec)
			}
		})
	}
}

func TestDPAReconciler_updateMetricsRole(t *testing.T) {

	tests := []struct {
		name            string
		role            *rbacv1.Role
		dpa             *oadpv1alpha1.DataProtectionApplication
		wantErr         bool
		wantMetricsRole *rbacv1.Role
	}{
		{
			name: "metrics role gets updated for a valid dpa",
			role: &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prometheus-k8s",
					Namespace: "test-ns",
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
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
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "bucket-123",
								},
								Config: map[string]string{},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "creds",
								},
								Default:          false,
								BackupSyncPeriod: &metav1.Duration{},
							},
						},
					},
				},
			},
			wantErr: false,
			wantMetricsRole: &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prometheus-k8s",
					Namespace: "test-ns",
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{
							"",
						},
						Resources: []string{
							"services",
							"endpoints",
							"pods",
						},
						Verbs: []string{
							"get",
							"list",
							"watch",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjectsForMonitor(tt.role, tt.dpa)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(tt.name),
				NamespacedName: types.NamespacedName{
					Namespace: tt.role.Namespace,
					Name:      tt.role.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
			}

			err = r.updateMetricsRole(tt.role, tt.dpa)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateMetricsRole() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(tt.wantMetricsRole.Labels, tt.role.Labels) {
				t.Errorf("expected metrics role labels to be %#v, got %#v", tt.wantMetricsRole.Labels, tt.role.Labels)
			}
			if !reflect.DeepEqual(tt.wantMetricsRole.Rules, tt.role.Rules) {
				t.Errorf("expected metrics role rules to be %#v, got %#v", tt.wantMetricsRole.Rules, tt.role.Rules)
			}
		})
	}
}

func TestDPAReconciler_updateMetricsRoleBinding(t *testing.T) {
	tests := []struct {
		name                   string
		rolebinding            *rbacv1.RoleBinding
		dpa                    *oadpv1alpha1.DataProtectionApplication
		wantErr                bool
		wantMetricsRoleBinding *rbacv1.RoleBinding
	}{
		{
			name: "metrics role binding gets updated for a valid dpa",
			rolebinding: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prometheus-k8s",
					Namespace: "test-ns",
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
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
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "bucket-123",
								},
								Config: map[string]string{},
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cloud-credentials",
									},
									Key: "creds",
								},
								Default:          false,
								BackupSyncPeriod: &metav1.Duration{},
							},
						},
					},
				},
			},
			wantErr: false,
			wantMetricsRoleBinding: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prometheus-k8s",
					Namespace: "test-ns",
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "Role",
					Name:     "prometheus-k8s",
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "prometheus-k8s",
						Namespace: "openshift-monitoring",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjectsForMonitor(tt.rolebinding, tt.dpa)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(tt.name),
				NamespacedName: types.NamespacedName{
					Namespace: tt.rolebinding.Namespace,
					Name:      tt.rolebinding.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
			}

			err = r.updateMetricsRoleBinding(tt.rolebinding, tt.dpa)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateMetricsRoleBinding() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(tt.wantMetricsRoleBinding.Labels, tt.rolebinding.Labels) {
				t.Errorf("expected metrics role binding labels to be %#v, got %#v", tt.wantMetricsRoleBinding.Labels, tt.rolebinding.Labels)
			}
			if !reflect.DeepEqual(tt.wantMetricsRoleBinding.RoleRef, tt.rolebinding.RoleRef) {
				t.Errorf("expected metrics role binding role refs to be %#v, got %#v", tt.wantMetricsRoleBinding.RoleRef, tt.rolebinding.RoleRef)
			}
			if !reflect.DeepEqual(tt.wantMetricsRoleBinding.Subjects, tt.rolebinding.Subjects) {
				t.Errorf("expected metrics role binding subjects to be %#v, got %#v", tt.wantMetricsRoleBinding.Subjects, tt.rolebinding.Subjects)
			}
		})
	}
}

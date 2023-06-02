package controllers

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func getSchemeForFakeClientForDatamover() (*runtime.Scheme, error) {
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

func getFakeClientFromObjectsForDatamover(objs ...client.Object) (client.WithWatch, error) {
	schemeForFakeClient, err := getSchemeForFakeClientForRegistry()
	if err != nil {
		return nil, err
	}

	return fake.NewClientBuilder().WithScheme(schemeForFakeClient).WithObjects(objs...).Build(), nil
}

func TestDPAReconciler_buildDataMoverDeployment(t *testing.T) {

	tests := []struct {
		name                    string
		dataMoverDeployment     *appsv1.Deployment
		wantDataMoverDeployment *appsv1.Deployment
		dpa                     *oadpv1alpha1.DataProtectionApplication
		wantErr                 bool
	}{
		{
			name:                    "DPA is nil",
			dataMoverDeployment:     &appsv1.Deployment{},
			dpa:                     nil,
			wantErr:                 true,
			wantDataMoverDeployment: &appsv1.Deployment{},
		},
		{
			name: "given a valid dpa with enableDataMover flag set to true, get appropriate data mover deployment",
			dataMoverDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      common.DataMover,
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"component": common.DataMoverController,
						},
					},
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-Velero-CR",
					Namespace: "test-ns",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					Features: &oadpv1alpha1.Features{
						DataMover: &oadpv1alpha1.DataMover{
							Enable: true,
						},
					},
				},
			},
			wantErr: false,
			wantDataMoverDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      common.DataMover,
					Namespace: "test-ns",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "oadp.openshift.io/v1alpha1",
							Kind:               "DataProtectionApplication",
							Name:               "test-Velero-CR",
							UID:                "test-ns",
							Controller:         pointer.BoolPtr(true),
							BlockOwnerDeletion: pointer.BoolPtr(true),
						},
					},
					Labels: map[string]string{
						"app.kubernetes.io/name":              common.OADPOperatorVelero,
						"app.kubernetes.io/instance":          common.DataMover,
						"app.kubernetes.io/managed-by":        common.OADPOperator,
						"app.kubernetes.io/component":         common.DataMover,
						oadpv1alpha1.OadpOperatorLabel:        "True",
						oadpv1alpha1.DataMoverDeploymentLabel: "True",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"component": common.DataMoverController,
						},
					},
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.DataMoverController,
							},
						},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyAlways,
							Containers: []corev1.Container{
								{
									Image:           common.DataMoverImage,
									Name:            common.DataMoverControllerContainer,
									ImagePullPolicy: corev1.PullAlways,
									Env: []corev1.EnvVar{
										{
											Name:  DataMoverConcurrentBackup,
											Value: DefaultConcurrentBackupVolumes,
										},
										{
											Name:  DataMoverConcurrentRestore,
											Value: DefaultConcurrentRestoreVolumes,
										},
										{
											Name:  DataMoverDummyPodImageEnvVar,
											Value: common.DummyPodImage,
										},
									},
								},
							},
							ServiceAccountName: "openshift-adp-controller-manager",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjectsForDatamover(tt.dataMoverDeployment)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(tt.name),
				NamespacedName: types.NamespacedName{
					Namespace: tt.dataMoverDeployment.Namespace,
					Name:      tt.dataMoverDeployment.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
			}
			if err := r.buildDataMoverDeployment(tt.dataMoverDeployment, tt.dpa); (err != nil) != tt.wantErr {
				t.Errorf("buildDataMoverDeployment() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.wantDataMoverDeployment.Labels, tt.dataMoverDeployment.Labels) {
				t.Errorf("expected dataMoverDeployment labels to be %#v, got %#v", tt.wantDataMoverDeployment.Labels, tt.dataMoverDeployment.Labels)
			}
			if !reflect.DeepEqual(tt.wantDataMoverDeployment.Spec, tt.dataMoverDeployment.Spec) {
				fmt.Println(cmp.Diff(tt.wantDataMoverDeployment.Spec, tt.dataMoverDeployment.Spec))
				t.Errorf("expected dataMoverDeployment spec to be %#v, got %#v", tt.wantDataMoverDeployment, tt.dataMoverDeployment)
			}
		})
	}
}

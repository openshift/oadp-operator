package controllers

import (
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"reflect"
	"testing"
)

func TestVeleroReconciler_buildRegistryDeployment(t *testing.T) {
	tests := []struct {
		name               string
		registryDeployment *appsv1.Deployment
		bsl                *velerov1.BackupStorageLocation
		wantErr            bool
	}{
		{
			name: "registry without owner reference as well as labels",
			registryDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-registry",
					Namespace: "test-ns",
				},
			},
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "registry without owner reference but has labels",
			registryDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-registry",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/name":       OADPOperatorVelero,
						"app.kubernetes.io/instance":   "oadp-test-bsl-test-ns-registry",
						"app.kubernetes.io/managed-by": OADPOperator,
						"app.kubernetes.io/component":  Registry,
					},
				},
			},
			bsl: &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bsl",
					Namespace: "test-ns",
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
			wantRegistryDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-registry",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/name":       OADPOperatorVelero,
						"app.kubernetes.io/instance":   "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry",
						"app.kubernetes.io/managed-by": OADPOperator,
						"app.kubernetes.io/component":  Registry,
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "velero.io/v1",
							Kind:               "BackupStorageLocation",
							Name:               tt.bsl.Name,
							UID:                tt.bsl.UID,
							Controller:         pointer.BoolPtr(true),
							BlockOwnerDeletion: pointer.BoolPtr(true),
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: pointer.Int32(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": "oadp-" + tt.bsl.Name + "-" + tt.bsl.Spec.Provider + "-registry",
							},
						},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyAlways,
							//Containers:    r.buildRegistryContainer(bsl),
						},
					},
				},
			}

			err = r.buildRegistryDeployment(tt.registryDeployment, tt.bsl)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildRegistryDeployment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(wantRegistryDeployment.Labels, tt.registryDeployment.Labels) {
				t.Errorf("expected registry deployment labels to be %#v, got %#v", wantRegistryDeployment.Labels, tt.registryDeployment.Labels)
			}
			if !reflect.DeepEqual(wantRegistryDeployment.OwnerReferences, tt.registryDeployment.OwnerReferences) {
				t.Errorf("expected registry deployment owner references to be %#v, got %#v", wantRegistryDeployment.OwnerReferences, tt.registryDeployment.OwnerReferences)
			}
			if !reflect.DeepEqual(wantRegistryDeployment.Spec.Replicas, tt.registryDeployment.Spec.Replicas) {
				t.Errorf("expected registry deployment replicas to be %#v, got %#v", wantRegistryDeployment.Spec.Replicas, tt.registryDeployment.Spec.Replicas)
			}
		})
	}
}

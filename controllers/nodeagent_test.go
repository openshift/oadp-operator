package controllers

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
)

func TestDPAReconciler_ReconcileNodeAgentDaemonset(t *testing.T) {
	type fields struct {
		Client         client.Client
		Scheme         *runtime.Scheme
		Log            logr.Logger
		Context        context.Context
		NamespacedName types.NamespacedName
		EventRecorder  record.EventRecorder
	}
	type args struct {
		log logr.Logger
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		//TODO: Add tests
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &DPAReconciler{
				Client:         tt.fields.Client,
				Scheme:         tt.fields.Scheme,
				Log:            tt.fields.Log,
				Context:        tt.fields.Context,
				NamespacedName: tt.fields.NamespacedName,
				EventRecorder:  tt.fields.EventRecorder,
			}
			got, err := r.ReconcileNodeAgentDaemonset(tt.args.log)
			if (err != nil) != tt.wantErr {
				t.Errorf("DPAReconciler.ReconcileNodeAgentDaemonset() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DPAReconciler.ReconcileNodeAgentDaemonset() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDPAReconciler_buildNodeAgentDaemonset(t *testing.T) {
	type fields struct {
		Client         client.Client
		Scheme         *runtime.Scheme
		Log            logr.Logger
		Context        context.Context
		NamespacedName types.NamespacedName
		EventRecorder  record.EventRecorder
	}
	type args struct {
		dpa *oadpv1alpha1.DataProtectionApplication
		ds  *appsv1.DaemonSet
	}
	r := &DPAReconciler{}
	dpa := oadpv1alpha1.DataProtectionApplication{
		Spec: oadpv1alpha1.DataProtectionApplicationSpec{
			Configuration: &oadpv1alpha1.ApplicationConfig{
				NodeAgent: &oadpv1alpha1.NodeAgentConfig{
					NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
						PodConfig: &oadpv1alpha1.PodConfig{},
					},
					UploaderType: "",
				},
			},
		},
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *appsv1.DaemonSet
		wantErr bool
	}{
		{
			name:   "dpa is nil",
			fields: fields{NamespacedName: types.NamespacedName{Namespace: "velero"}},
			args: args{
				nil, &appsv1.DaemonSet{},
			},
			wantErr: true,
			want:    nil,
		},
		{
			name: "DaemonSet is nil",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{}, nil,
			},
			wantErr: true,
			want:    nil,
		},
		{
			name: "Valid velero and daemonset",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{
								NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{},
								UploaderType:          "",
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getNodeAgentObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: nodeAgentLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.NodeAgent,
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector:       dpa.Spec.Configuration.NodeAgent.PodConfig.NodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: fsPvHostPath,
										},
									},
								},
								{
									Name: HostPlugins,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/var/lib/kubelet/plugins",
										},
									},
								},
								{
									Name: "scratch",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "certs",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.NodeAgent.PodConfig.Tolerations,
							Containers: []corev1.Container{
								{
									Name: common.NodeAgent,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										common.NodeAgent,
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        "/var/lib/kubelet/plugins",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:      "scratch",
											MountPath: "/scratch",
										},
										{
											Name:      "certs",
											MountPath: "/etc/ssl/certs",
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Valid velero with Env PodConfig and daemonset",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{
								NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
									PodConfig: &oadpv1alpha1.PodConfig{
										Env: []corev1.EnvVar{
											{
												Name:  "TEST_ENV",
												Value: "TEST_VALUE",
											},
										},
									},
								},
								UploaderType: "",
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getNodeAgentObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: nodeAgentLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.NodeAgent,
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector:       dpa.Spec.Configuration.NodeAgent.PodConfig.NodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: fsPvHostPath,
										},
									},
								},
								{
									Name: HostPlugins,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/var/lib/kubelet/plugins",
										},
									},
								},
								{
									Name: "scratch",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "certs",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.NodeAgent.PodConfig.Tolerations,
							Containers: []corev1.Container{
								{
									Name: common.NodeAgent,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										"node-agent",
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        "/var/lib/kubelet/plugins",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:      "scratch",
											MountPath: "/scratch",
										},
										{
											Name:      "certs",
											MountPath: "/etc/ssl/certs",
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
										{
											Name:  "TEST_ENV",
											Value: "TEST_VALUE",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "podConfig label for velero and NodeAgent",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{
								NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
									PodConfig: &oadpv1alpha1.PodConfig{
										Labels: map[string]string{
											"nodeAgentLabel": "this is a label",
										},
									},
								},
								UploaderType: "",
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{
									Labels: map[string]string{
										"veleroLabel": "this is a label",
									},
								},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getNodeAgentObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: nodeAgentLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component":      common.Velero,
								"name":           common.NodeAgent,
								"nodeAgentLabel": "this is a label",
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector:       dpa.Spec.Configuration.NodeAgent.PodConfig.NodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: fsPvHostPath,
										},
									},
								},
								{
									Name: HostPlugins,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/var/lib/kubelet/plugins",
										},
									},
								},
								{
									Name: "scratch",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "certs",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.NodeAgent.PodConfig.Tolerations,
							Containers: []corev1.Container{
								{
									Name: common.NodeAgent,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										common.NodeAgent,
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        "/var/lib/kubelet/plugins",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:      "scratch",
											MountPath: "/scratch",
										},
										{
											Name:      "certs",
											MountPath: "/etc/ssl/certs",
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Invalid podConfig label for velero and NodeAgent",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{
								NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
									PodConfig: &oadpv1alpha1.PodConfig{
										Labels: map[string]string{
											"name": "not-node-agent", // this label is already defined by https://github.com/openshift/velero/blob/8b2f7dbdb510434b9c05180bae7a3fb2a8081e2f/pkg/install/daemonset.go#L71
										},
									},
								},
								UploaderType: "",
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{
									Labels: map[string]string{
										"veleroLabel": "this is a label",
									},
								},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: true,
			want:    nil,
		},
		{
			name: "test NodeAgent nodeselector customization via dpa",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{
								NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
									PodConfig: &oadpv1alpha1.PodConfig{
										NodeSelector: map[string]string{
											"foo": "bar",
										},
									},
								},
								UploaderType: "",
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
								DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
									oadpv1alpha1.DefaultPluginAWS,
								},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getNodeAgentObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: nodeAgentLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.NodeAgent,
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"foo": "bar",
							},
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: fsPvHostPath,
										},
									},
								},
								{
									Name: HostPlugins,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/var/lib/kubelet/plugins",
										},
									},
								},
								{
									Name: "scratch",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "certs",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "cloud-credentials",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.NodeAgent.PodConfig.Tolerations,
							Containers: []corev1.Container{
								{
									Name: common.NodeAgent,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										common.NodeAgent,
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        "/var/lib/kubelet/plugins",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:      "scratch",
											MountPath: "/scratch",
										},
										{
											Name:      "certs",
											MountPath: "/etc/ssl/certs",
										},
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
										{
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "test NodeAgent resource reqs customization via dpa",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{
								NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
									PodConfig: &oadpv1alpha1.PodConfig{
										NodeSelector: map[string]string{
											"foo": "bar",
										},
										ResourceAllocations: corev1.ResourceRequirements{
											Limits: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("2"),
												corev1.ResourceMemory: resource.MustParse("128Mi"),
											},
											Requests: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("1"),
												corev1.ResourceMemory: resource.MustParse("256Mi"),
											},
										},
									},
								},
								UploaderType: "",
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
								DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
									oadpv1alpha1.DefaultPluginAWS,
								},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getNodeAgentObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: nodeAgentLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.NodeAgent,
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"foo": "bar",
							},
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: fsPvHostPath,
										},
									},
								},
								{
									Name: HostPlugins,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/var/lib/kubelet/plugins",
										},
									},
								},
								{
									Name: "scratch",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "certs",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "cloud-credentials",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.NodeAgent.PodConfig.Tolerations,
							Containers: []corev1.Container{
								{
									Name: common.NodeAgent,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										common.NodeAgent,
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        "/var/lib/kubelet/plugins",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:      "scratch",
											MountPath: "/scratch",
										},
										{
											Name:      "certs",
											MountPath: "/etc/ssl/certs",
										},
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("2"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("1"),
											corev1.ResourceMemory: resource.MustParse("256Mi"),
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
										{
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "test NodeAgent resource reqs only NodeAgent cpu limit customization via dpa",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{
								NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
									PodConfig: &oadpv1alpha1.PodConfig{
										NodeSelector: map[string]string{
											"foo": "bar",
										},
										ResourceAllocations: corev1.ResourceRequirements{
											Limits: corev1.ResourceList{
												corev1.ResourceCPU: resource.MustParse("2"),
											},
										},
									},
								},
								UploaderType: "",
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
								DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
									oadpv1alpha1.DefaultPluginAWS,
								},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getNodeAgentObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: nodeAgentLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.NodeAgent,
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"foo": "bar",
							},
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: fsPvHostPath,
										},
									},
								},
								{
									Name: HostPlugins,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/var/lib/kubelet/plugins",
										},
									},
								},
								{
									Name: "scratch",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "certs",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "cloud-credentials",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.NodeAgent.PodConfig.Tolerations,
							Containers: []corev1.Container{
								{
									Name: common.NodeAgent,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										common.NodeAgent,
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        "/var/lib/kubelet/plugins",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:      "scratch",
											MountPath: "/scratch",
										},
										{
											Name:      "certs",
											MountPath: "/etc/ssl/certs",
										},
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("2"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
										{
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "test NodeAgent resource reqs only NodeAgent cpu request customization via dpa",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{
								NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
									PodConfig: &oadpv1alpha1.PodConfig{
										NodeSelector: map[string]string{
											"foo": "bar",
										},
										ResourceAllocations: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceCPU: resource.MustParse("2"),
											},
										},
									},
								},
								UploaderType: "",
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
								DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
									oadpv1alpha1.DefaultPluginAWS,
								},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getNodeAgentObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: nodeAgentLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.NodeAgent,
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"foo": "bar",
							},
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: fsPvHostPath,
										},
									},
								},
								{
									Name: HostPlugins,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/var/lib/kubelet/plugins",
										},
									},
								},
								{
									Name: "scratch",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "certs",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "cloud-credentials",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.NodeAgent.PodConfig.Tolerations,
							Containers: []corev1.Container{
								{
									Name: common.NodeAgent,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										common.NodeAgent,
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        "/var/lib/kubelet/plugins",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:      "scratch",
											MountPath: "/scratch",
										},
										{
											Name:      "certs",
											MountPath: "/etc/ssl/certs",
										},
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("2"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
										{
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "test NodeAgent resource reqs only NodeAgent memory limit customization via dpa",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{
								NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
									PodConfig: &oadpv1alpha1.PodConfig{
										NodeSelector: map[string]string{
											"foo": "bar",
										},
										ResourceAllocations: corev1.ResourceRequirements{
											Limits: corev1.ResourceList{
												corev1.ResourceMemory: resource.MustParse("256Mi"),
											},
										},
									},
								},
								UploaderType: "",
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
								DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
									oadpv1alpha1.DefaultPluginAWS,
								},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getNodeAgentObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: nodeAgentLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.NodeAgent,
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"foo": "bar",
							},
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: fsPvHostPath,
										},
									},
								},
								{
									Name: HostPlugins,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/var/lib/kubelet/plugins",
										},
									},
								},
								{
									Name: "scratch",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "certs",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "cloud-credentials",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.NodeAgent.PodConfig.Tolerations,
							Containers: []corev1.Container{
								{
									Name: common.NodeAgent,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										common.NodeAgent,
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        "/var/lib/kubelet/plugins",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:      "scratch",
											MountPath: "/scratch",
										},
										{
											Name:      "certs",
											MountPath: "/etc/ssl/certs",
										},
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("256Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
										{
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "test NodeAgent resource reqs only NodeAgent memory request customization via dpa",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{
								NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
									PodConfig: &oadpv1alpha1.PodConfig{
										NodeSelector: map[string]string{
											"foo": "bar",
										},
										ResourceAllocations: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceMemory: resource.MustParse("256Mi"),
											},
										},
									},
								},
								UploaderType: "",
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
								DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
									oadpv1alpha1.DefaultPluginAWS,
								},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getNodeAgentObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: nodeAgentLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.NodeAgent,
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"foo": "bar",
							},
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: fsPvHostPath,
										},
									},
								},
								{
									Name: HostPlugins,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/var/lib/kubelet/plugins",
										},
									},
								},
								{
									Name: "scratch",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "certs",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "cloud-credentials",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.NodeAgent.PodConfig.Tolerations,
							Containers: []corev1.Container{
								{
									Name: common.NodeAgent,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										common.NodeAgent,
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        "/var/lib/kubelet/plugins",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:      "scratch",
											MountPath: "/scratch",
										},
										{
											Name:      "certs",
											MountPath: "/etc/ssl/certs",
										},
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("256Mi"),
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
										{
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "test NodeAgent tolerations customization via dpa",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{
								NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
									PodConfig: &oadpv1alpha1.PodConfig{
										Tolerations: []corev1.Toleration{
											{
												Key:      "key1",
												Operator: "Equal",
												Value:    "value1",
												Effect:   "NoSchedule",
											},
										},
									},
								},
								UploaderType: "",
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
								DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
									oadpv1alpha1.DefaultPluginAWS,
								},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getNodeAgentObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: nodeAgentLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.NodeAgent,
							},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: fsPvHostPath,
										},
									},
								},
								{
									Name: HostPlugins,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/var/lib/kubelet/plugins",
										},
									},
								},
								{
									Name: "scratch",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "certs",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "cloud-credentials",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: []corev1.Toleration{
								{
									Key:      "key1",
									Operator: "Equal",
									Value:    "value1",
									Effect:   "NoSchedule",
								},
							},
							Containers: []corev1.Container{
								{
									Name: common.NodeAgent,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										common.NodeAgent,
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        "/var/lib/kubelet/plugins",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:      "scratch",
											MountPath: "/scratch",
										},
										{
											Name:      "certs",
											MountPath: "/etc/ssl/certs",
										},
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
										{
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Valid velero and daemonset for aws as bsl",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{
								NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
									PodConfig: &oadpv1alpha1.PodConfig{},
								},
								UploaderType: "",
							},
							Velero: &oadpv1alpha1.VeleroConfig{
								PodConfig: &oadpv1alpha1.PodConfig{},
								DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
									oadpv1alpha1.DefaultPluginAWS,
								},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getNodeAgentObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: nodeAgentLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.NodeAgent,
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector:       dpa.Spec.Configuration.NodeAgent.PodConfig.NodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: fsPvHostPath,
										},
									},
								},
								{
									Name: HostPlugins,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/var/lib/kubelet/plugins",
										},
									},
								},
								{
									Name: "scratch",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "certs",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "cloud-credentials",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.NodeAgent.PodConfig.Tolerations,
							Containers: []corev1.Container{
								{
									Name: common.NodeAgent,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										common.NodeAgent,
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        "/var/lib/kubelet/plugins",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:      "scratch",
											MountPath: "/scratch",
										},
										{
											Name:      "certs",
											MountPath: "/etc/ssl/certs",
										},
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
										{
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Valid velero with annotation and daemonset for aws as bsl with default secret name",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							Velero: &oadpv1alpha1.VeleroConfig{
								DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
									oadpv1alpha1.DefaultPluginAWS,
								},
							},
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{},
						},
						BackupLocations: []oadpv1alpha1.BackupLocation{
							{
								Velero: &velerov1.BackupStorageLocationSpec{
									Provider: AWSProvider,
									StorageType: velerov1.StorageType{
										ObjectStorage: &velerov1.ObjectStorageLocation{
											Bucket: "aws-bucket",
										},
									},
									Config: map[string]string{
										Region:                "aws-region",
										S3URL:                 "https://sr-url-aws-domain.com",
										InsecureSkipTLSVerify: "false",
									},
									Credential: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "cloud-credentials",
										},
									},
								},
							},
						},
						PodAnnotations: map[string]string{
							"test-annotation": "awesome annotation",
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getNodeAgentObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: nodeAgentLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.NodeAgent,
							},
							Annotations: map[string]string{
								"test-annotation": "awesome annotation",
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector:       dpa.Spec.Configuration.NodeAgent.PodConfig.NodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: fsPvHostPath,
										},
									},
								},
								{
									Name: HostPlugins,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/var/lib/kubelet/plugins",
										},
									},
								},
								{
									Name: "scratch",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "certs",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.NodeAgent.PodConfig.Tolerations,
							Containers: []corev1.Container{
								{
									Name: common.NodeAgent,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										common.NodeAgent,
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        "/var/lib/kubelet/plugins",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:      "scratch",
											MountPath: "/scratch",
										},
										{
											Name:      "certs",
											MountPath: "/etc/ssl/certs",
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Valid velero with DNS Policy/Config with annotation and daemonset for aws as bsl with default secret name not specified",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							Velero: &oadpv1alpha1.VeleroConfig{
								DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
									oadpv1alpha1.DefaultPluginAWS,
								},
							},
							NodeAgent: &oadpv1alpha1.NodeAgentConfig{},
						},
						BackupLocations: []oadpv1alpha1.BackupLocation{
							{
								Velero: &velerov1.BackupStorageLocationSpec{
									Provider: AWSProvider,
									StorageType: velerov1.StorageType{
										ObjectStorage: &velerov1.ObjectStorageLocation{
											Bucket: "aws-bucket",
										},
									},
									Config: map[string]string{
										Region:                "aws-region",
										S3URL:                 "https://sr-url-aws-domain.com",
										InsecureSkipTLSVerify: "false",
									},
								},
							},
						},
						PodAnnotations: map[string]string{
							"test-annotation": "awesome annotation",
						},
						PodDnsPolicy: "None",
						PodDnsConfig: corev1.PodDNSConfig{
							Nameservers: []string{
								"1.1.1.1",
								"8.8.8.8",
							},
							Options: []corev1.PodDNSConfigOption{
								{
									Name:  "ndots",
									Value: pointer.String("2"),
								},
								{
									Name: "edns0",
								},
							},
						},
					},
				}, &appsv1.DaemonSet{
					ObjectMeta: getNodeAgentObjectMeta(r),
				},
			},
			wantErr: false,
			want: &appsv1.DaemonSet{
				ObjectMeta: getNodeAgentObjectMeta(r),
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSet",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.RollingUpdateDaemonSetStrategyType,
					},
					Selector: nodeAgentLabelSelector,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"component": common.Velero,
								"name":      common.NodeAgent,
							},
							Annotations: map[string]string{
								"test-annotation": "awesome annotation",
							},
						},
						Spec: corev1.PodSpec{
							NodeSelector:       dpa.Spec.Configuration.NodeAgent.PodConfig.NodeSelector,
							ServiceAccountName: common.Velero,
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:          pointer.Int64(0),
								SupplementalGroups: dpa.Spec.Configuration.NodeAgent.SupplementalGroups,
							},
							DNSPolicy: "None",
							DNSConfig: &corev1.PodDNSConfig{
								Nameservers: []string{
									"1.1.1.1",
									"8.8.8.8",
								},
								Options: []corev1.PodDNSConfigOption{
									{
										Name:  "ndots",
										Value: pointer.String("2"),
									},
									{
										Name: "edns0",
									},
								},
							},
							Volumes: []corev1.Volume{
								// Cloud Provider volumes are dynamically added in the for loop below
								{
									Name: HostPods,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: fsPvHostPath,
										},
									},
								},
								{
									Name: HostPlugins,
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{
											Path: "/var/lib/kubelet/plugins",
										},
									},
								},
								{
									Name: "scratch",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "certs",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "cloud-credentials",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "cloud-credentials",
										},
									},
								},
							},
							Tolerations: dpa.Spec.Configuration.NodeAgent.PodConfig.Tolerations,
							Containers: []corev1.Container{
								{
									Name: common.NodeAgent,
									SecurityContext: &corev1.SecurityContext{
										Privileged: pointer.Bool(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										common.NodeAgent,
										"server",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        "/var/lib/kubelet/plugins",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:      "scratch",
											MountPath: "/scratch",
										},
										{
											Name:      "certs",
											MountPath: "/etc/ssl/certs",
										},
										{
											Name:      "cloud-credentials",
											MountPath: "/credentials",
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
									Env: []corev1.EnvVar{
										{
											Name: "NODE_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "spec.nodeName",
												},
											},
										},
										{
											Name: "VELERO_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
										{
											Name:  "VELERO_SCRATCH_DIR",
											Value: "/scratch",
										},
										{
											Name:  common.AWSSharedCredentialsFileEnvKey,
											Value: "/credentials/cloud",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &DPAReconciler{
				Client:         tt.fields.Client,
				Scheme:         tt.fields.Scheme,
				Log:            tt.fields.Log,
				Context:        tt.fields.Context,
				NamespacedName: tt.fields.NamespacedName,
				EventRecorder:  tt.fields.EventRecorder,
			}
			got, err := r.buildNodeAgentDaemonset(tt.args.dpa, tt.args.ds)
			if (err != nil) != tt.wantErr {
				t.Errorf("DPAReconciler.buildNodeAgentDaemonset() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.args.dpa != nil && tt.want != nil {
				setPodTemplateSpecDefaults(&tt.want.Spec.Template)
				if len(tt.want.Spec.Template.Spec.Containers) > 0 {
					setContainerDefaults(&tt.want.Spec.Template.Spec.Containers[0])
				}
				if tt.want.Spec.UpdateStrategy.Type == appsv1.RollingUpdateDaemonSetStrategyType {
					tt.want.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateDaemonSet{
						MaxUnavailable: &intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: 1,
						},
						MaxSurge: &intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: 0,
						},
					}
				}
				if tt.want.Spec.RevisionHistoryLimit == nil {
					tt.want.Spec.RevisionHistoryLimit = pointer.Int32(10)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				fmt.Printf(cmp.Diff(got, tt.want))
				t.Errorf("DPAReconciler.buildNodeAgentDaemonset() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDPAReconciler_updateFsRestoreHelperCM(t *testing.T) {

	tests := []struct {
		name                  string
		fsRestoreHelperCM     *corev1.ConfigMap
		dpa                   *oadpv1alpha1.DataProtectionApplication
		wantErr               bool
		wantFsRestoreHelperCM *corev1.ConfigMap
	}{
		{
			name: "Given DPA CR instance, appropriate NodeAgent restore helper cm is created",
			fsRestoreHelperCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      FsRestoreHelperCM,
					Namespace: "test-ns",
				},
			},
			dpa:     &oadpv1alpha1.DataProtectionApplication{},
			wantErr: false,
			wantFsRestoreHelperCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      FsRestoreHelperCM,
					Namespace: "test-ns",
					Labels: map[string]string{
						"velero.io/plugin-config":      "",
						"velero.io/pod-volume-restore": "RestoreItemAction",
						oadpv1alpha1.OadpOperatorLabel: "True",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         oadpv1alpha1.SchemeBuilder.GroupVersion.String(),
							Kind:               "DataProtectionApplication",
							Name:               "",
							UID:                "",
							Controller:         pointer.BoolPtr(true),
							BlockOwnerDeletion: pointer.BoolPtr(true),
						},
					},
				},
				Data: map[string]string{
					"image": os.Getenv("RELATED_IMAGE_VELERO_RESTORE_HELPER"),
				},
			},
		},
	}
	for _, tt := range tests {
		fakeClient, err := getFakeClientFromObjects()
		if err != nil {
			t.Errorf("error in creating fake client, likely programmer error")
		}
		t.Run(tt.name, func(t *testing.T) {
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
			if err := r.updateFsRestoreHelperCM(tt.fsRestoreHelperCM, tt.dpa); (err != nil) != tt.wantErr {
				t.Errorf("updateFsRestoreHelperCM() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.fsRestoreHelperCM, tt.wantFsRestoreHelperCM) {
				t.Errorf("updateFsRestoreHelperCM() got CM = %v, want CM %v", tt.fsRestoreHelperCM, tt.wantFsRestoreHelperCM)
			}
		})
	}
}

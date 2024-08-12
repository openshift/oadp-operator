package controllers

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/operator-framework/operator-lib/proxy"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
)

type ReconcileNodeAgentControllerScenario struct {
	namespace string
	dpaName   string
	envVar    corev1.EnvVar
}

var _ = ginkgo.Describe("Test ReconcileNodeAgentDaemonSet function", func() {
	var (
		ctx                 = context.Background()
		currentTestScenario ReconcileNodeAgentControllerScenario
		updateTestScenario  = func(scenario ReconcileNodeAgentControllerScenario) {
			currentTestScenario = scenario
		}
	)

	ginkgo.BeforeEach(func() {
		clusterInfraObject := &configv1.Infrastructure{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster",
			},
			Spec: configv1.InfrastructureSpec{
				PlatformSpec: configv1.PlatformSpec{
					Type: IBMCloudPlatform,
				},
			},
		}

		gomega.Expect(k8sClient.Create(ctx, clusterInfraObject)).To(gomega.Succeed())
	})

	ginkgo.AfterEach(func() {
		os.Unsetenv(currentTestScenario.envVar.Name)

		daemonSet := &appsv1.DaemonSet{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      common.NodeAgent,
				Namespace: currentTestScenario.namespace,
			},
			daemonSet,
		) == nil {
			gomega.Expect(k8sClient.Delete(ctx, daemonSet)).To(gomega.Succeed())
		}

		dpa := &oadpv1alpha1.DataProtectionApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name:      currentTestScenario.dpaName,
				Namespace: currentTestScenario.namespace,
			},
		}
		gomega.Expect(k8sClient.Delete(ctx, dpa)).To(gomega.Succeed())

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: currentTestScenario.namespace,
			},
		}
		gomega.Expect(k8sClient.Delete(ctx, namespace)).To(gomega.Succeed())

		clusterInfraObject := &configv1.Infrastructure{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster",
			},
		}

		if k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterInfraObject), clusterInfraObject) == nil {
			gomega.Expect(k8sClient.Delete(ctx, clusterInfraObject)).To(gomega.Succeed())
		}
	})

	ginkgo.DescribeTable("Check if Subscription Config environment variables are passed to NodeAgent Containers",
		func(scenario ReconcileNodeAgentControllerScenario) {
			updateTestScenario(scenario)

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: scenario.namespace,
				},
			}
			gomega.Expect(k8sClient.Create(ctx, namespace)).To(gomega.Succeed())

			dpa := &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scenario.dpaName,
					Namespace: scenario.namespace,
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							UploaderType: "kopia",
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								Enable: ptr.To(true),
							},
						},
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
						},
					},
				},
			}
			gomega.Expect(k8sClient.Create(ctx, dpa)).To(gomega.Succeed())

			// Subscription Config environment variables are passed to controller-manager Pod
			// https://github.com/operator-framework/operator-lifecycle-manager/blob/d8500d88932b17aa9b1853f0f26086f6ee6b35f9/doc/design/subscription-config.md
			os.Setenv(scenario.envVar.Name, scenario.envVar.Value)

			event := record.NewFakeRecorder(5)
			r := &DPAReconciler{
				Client:  k8sClient,
				Scheme:  testEnv.Scheme,
				Context: ctx,
				NamespacedName: types.NamespacedName{
					Name:      scenario.dpaName,
					Namespace: scenario.namespace,
				},
				EventRecorder: event,
			}
			result, err := r.ReconcileNodeAgentDaemonset(logr.Discard())

			gomega.Expect(result).To(gomega.BeTrue())
			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))

			gomega.Expect(len(event.Events)).To(gomega.Equal(1))
			message := <-event.Events
			for _, word := range []string{"Normal", "NodeAgentDaemonsetReconciled", "created"} {
				gomega.Expect(message).To(gomega.ContainSubstring(word))
			}

			daemonSet := &appsv1.DaemonSet{}
			err = k8sClient.Get(
				ctx,
				types.NamespacedName{
					Name:      common.NodeAgent,
					Namespace: scenario.namespace,
				},
				daemonSet,
			)
			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))

			if slices.Contains(proxy.ProxyEnvNames, scenario.envVar.Name) {
				for _, container := range daemonSet.Spec.Template.Spec.Containers {
					gomega.Expect(container.Env).To(gomega.ContainElement(scenario.envVar))
				}
			} else {
				for _, container := range daemonSet.Spec.Template.Spec.Containers {
					gomega.Expect(container.Env).To(gomega.Not(gomega.ContainElement(scenario.envVar)))
				}
			}
		},
		ginkgo.Entry("Should add HTTP_PROXY environment variable to NodeAgent Containers", ReconcileNodeAgentControllerScenario{
			namespace: "test-node-agent-environment-variables-1",
			dpaName:   "test-node-agent-environment-variables-1-dpa",
			envVar: corev1.EnvVar{
				Name:  "HTTP_PROXY",
				Value: "http://proxy.example.com:8080",
			},
		}),
		ginkgo.Entry("Should add HTTPS_PROXY environment variable to NodeAgent Containers", ReconcileNodeAgentControllerScenario{
			namespace: "test-node-agent-environment-variables-2",
			dpaName:   "test-node-agent-environment-variables-2-dpa",
			envVar: corev1.EnvVar{
				Name:  "HTTPS_PROXY",
				Value: "localhost",
			},
		}),
		ginkgo.Entry("Should add NO_PROXY environment variable to NodeAgent Containers", ReconcileNodeAgentControllerScenario{
			namespace: "test-node-agent-environment-variables-3",
			dpaName:   "test-node-agent-environment-variables-3-dpa",
			envVar: corev1.EnvVar{
				Name:  "NO_PROXY",
				Value: "1.1.1.1",
			},
		}),
		ginkgo.Entry("Should NOT add WRONG environment variable to NodeAgent Containers", ReconcileNodeAgentControllerScenario{
			namespace: "test-node-agent-environment-variables-4",
			dpaName:   "test-node-agent-environment-variables-4-dpa",
			envVar: corev1.EnvVar{
				Name:  "WRONG",
				Value: "I do not know what is happening here",
			},
		}),
	)
})

func TestDPAReconciler_buildNodeAgentDaemonset(t *testing.T) {
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
		name          string
		args          args
		want          *appsv1.DaemonSet
		wantErr       bool
		clientObjects []client.Object
	}{
		{
			name: "dpa is nil",
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
			want: nil,
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			name: "Valid DPA CR with Unsupported NodeAgent Args, appropriate NodeAgent DaemonSet is built",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sample-dpa",
						Namespace: "sample-ns",
						Annotations: map[string]string{
							common.UnsupportedNodeAgentServerArgsAnnotation: "unsupported-node-agent-server-args-cm",
						},
					},
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
			clientObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unsupported-node-agent-server-args-cm",
						Namespace: "sample-ns",
					},
					Data: map[string]string{
						"unsupported-arg":      "value1",
						"unsupported-bool-arg": "True",
					},
				},
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
									},
									Image:           getVeleroImage(&dpa),
									ImagePullPolicy: corev1.PullAlways,
									Command: []string{
										"/velero",
									},
									Args: []string{
										common.NodeAgent,
										"server",
										"--unsupported-arg=value1",
										"--unsupported-bool-arg=true",
									},
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			name: "Valid DPA CR with empty value for Unsupported NodeAgent Args cm annotation, appropriate NodeAgent DaemonSet is built",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sample-dpa",
						Namespace: "sample-ns",
						Annotations: map[string]string{
							common.UnsupportedNodeAgentServerArgsAnnotation: "",
						},
					},
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
			clientObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unsupported-node-agent-server-args-cm",
						Namespace: "sample-ns",
					},
					Data: map[string]string{
						"unsupported-arg":      "value1",
						"unsupported-bool-arg": "True",
					},
				},
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			name: "Valid DPA CR with Unsupported NodeAgent Args cm missing, DPA error case",
			args: args{
				&oadpv1alpha1.DataProtectionApplication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sample-dpa",
						Namespace: "sample-ns",
						Annotations: map[string]string{
							common.UnsupportedNodeAgentServerArgsAnnotation: "unsupported-node-agent-server-args-cm",
						},
					},
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
			wantErr: true,
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
			want: nil,
		},
		{
			name: "Valid velero and daemon set for aws as bsl",
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
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
								RunAsUser:          ptr.To(int64(0)),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
									Value: ptr.To("2"),
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
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
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
								RunAsUser:          ptr.To(int64(0)),
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
										Value: ptr.To("2"),
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
											Path: pluginsHostPath,
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
										Privileged: ptr.To(true),
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
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8085,
											Protocol:      "TCP",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:             HostPods,
											MountPath:        "/host_pods",
											MountPropagation: &mountPropagationToHostContainer,
										},
										{
											Name:             HostPlugins,
											MountPath:        pluginsHostPath,
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
			fakeClient, err := getFakeClientFromObjects(tt.clientObjects...)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{
				Client: fakeClient,
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
					tt.want.Spec.RevisionHistoryLimit = ptr.To(int32(10))
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
							Controller:         ptr.To(true),
							BlockOwnerDeletion: ptr.To(true),
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

func TestDPAReconciler_getPlatformType(t *testing.T) {
	tests := []struct {
		name          string
		dpa           *oadpv1alpha1.DataProtectionApplication
		clientObjects []client.Object
		want          string
		wantErr       bool
	}{
		{
			name: "get IBMCloud platform type from infrastructure object",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sample-dpa",
					Namespace: "sample-ns",
				},
			},
			clientObjects: []client.Object{
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
					Status: configv1.InfrastructureStatus{
						PlatformStatus: &configv1.PlatformStatus{
							Type: configv1.IBMCloudPlatformType,
						},
					},
				},
			},
			want:    IBMCloudPlatform,
			wantErr: false,
		},
		{
			name: "get empty platform type for non existing infrastructure object",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sample-dpa",
					Namespace: "sample-ns",
				},
			},
			clientObjects: []client.Object{
				&configv1.Infrastructure{},
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		fakeClient, err := getFakeClientFromObjects(tt.clientObjects...)
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
			got, err := r.getPlatformType()
			if (err != nil) != tt.wantErr {
				t.Errorf("getPlatformType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getPlatformType() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getFsPvHostPath(t *testing.T) {
	tests := []struct {
		name         string
		platformType string
		envRestic    string
		envFS        string
		want         string
	}{
		{
			name:         "generic pv host path returned for empty platform type case",
			platformType: "",
			envRestic:    "",
			envFS:        "",
			want:         GenericPVHostPath,
		},
		{
			name:         "IBMCloud pv host path returned for IBMCloud platform type",
			platformType: IBMCloudPlatform,
			envRestic:    "",
			envFS:        "",
			want:         IBMCloudPVHostPath,
		},
		{
			name:         "empty platform type with restic env var set",
			platformType: "",
			envRestic:    "/foo/restic/bar",
			envFS:        "",
			want:         "/foo/restic/bar",
		},
		{
			name:         "empty platform type with fs env var set",
			platformType: "",
			envRestic:    "",
			envFS:        "/foo/file-system/bar",
			want:         "/foo/file-system/bar",
		},
		{
			name:         "IBMCloud platform type but env var also set, env var takes precedence",
			platformType: IBMCloudPlatform,
			envRestic:    "",
			envFS:        "/foo/file-system/env/var/override",
			want:         "/foo/file-system/env/var/override",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ResticPVHostPathEnvVar, tt.envRestic)
			t.Setenv(FSPVHostPathEnvVar, tt.envFS)
			if got := getFsPvHostPath(tt.platformType); got != tt.want {
				t.Errorf("getFsPvHostPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getPluginsHostPath(t *testing.T) {
	tests := []struct {
		name         string
		platformType string
		env          string
		want         string
	}{
		{
			name:         "generic plugins host path returned for empty platform type case",
			platformType: "",
			env:          "",
			want:         GenericPluginsHostPath,
		},
		{
			name:         "IBMCloud plugins host path returned for IBMCloud platform type",
			platformType: IBMCloudPlatform,
			env:          "",
			want:         IBMCloudPluginsHostPath,
		},
		{
			name:         "empty platform type with env var set",
			platformType: "",
			env:          "/foo/plugins/bar",
			want:         "/foo/plugins/bar",
		},
		{
			name:         "IBMClout platform type and env var also set, env var takes precedence",
			platformType: IBMCloudPlatform,
			env:          "/foo/plugins/bar",
			want:         "/foo/plugins/bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(PluginsHostPathEnvVar, tt.env)
			if got := getPluginsHostPath(tt.platformType); got != tt.want {
				t.Errorf("getPluginsHostPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

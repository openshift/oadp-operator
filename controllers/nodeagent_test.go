package controllers

import (
	"context"
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

var (
	testNodeAgentDaemonSet = &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.NodeAgent,
			Namespace: testNamespaceName,
			Labels:    nodeAgentMatchLabels,
		},
	}
	testGenericInfrastructure = &configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}
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
				dpa:           dpa,
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

type TestBuiltNodeAgentDaemonSetOptions struct {
	args             []string
	labels           map[string]string
	annotations      map[string]string
	volumes          []corev1.Volume
	volumeMounts     []corev1.VolumeMount
	env              []corev1.EnvVar
	dnsPolicy        corev1.DNSPolicy
	dnsConfig        *corev1.PodDNSConfig
	resourceLimits   corev1.ResourceList
	resourceRequests corev1.ResourceList
	toleration       []corev1.Toleration
	nodeSelector     map[string]string
}

func createTestBuiltNodeAgentDaemonSet(options TestBuiltNodeAgentDaemonSetOptions) *appsv1.DaemonSet {
	testBuiltNodeAgentDaemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.NodeAgent,
			Namespace: testNamespaceName,
			Labels:    nodeAgentMatchLabels,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "DaemonSet",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: nodeAgentLabelSelector,
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 1,
					},
					MaxSurge: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 0,
					},
				},
			},
			RevisionHistoryLimit: ptr.To(int32(10)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"component": common.Velero,
						"name":      common.NodeAgent,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:                 corev1.RestartPolicyAlways,
					ServiceAccountName:            common.Velero,
					TerminationGracePeriodSeconds: ptr.To(int64(30)),
					DNSPolicy:                     corev1.DNSClusterFirst,
					DeprecatedServiceAccount:      common.Velero,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser: ptr.To(int64(0)),
					},
					SchedulerName: "default-scheduler",
					Containers: []corev1.Container{
						{
							Name:                     common.NodeAgent,
							Image:                    common.VeleroImage,
							ImagePullPolicy:          corev1.PullAlways,
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							SecurityContext:          &corev1.SecurityContext{Privileged: ptr.To(true)},
							Ports: []corev1.ContainerPort{
								{
									Name:          "metrics",
									ContainerPort: 8085,
									Protocol:      "TCP",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
							Command: []string{"/velero"},
							Args:    append([]string{common.NodeAgent, "server"}, options.args...),
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
							Env: []corev1.EnvVar{
								{
									Name: "NODE_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											APIVersion: "v1",
											FieldPath:  "spec.nodeName",
										},
									},
								},
								{
									Name: "VELERO_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											APIVersion: "v1",
											FieldPath:  "metadata.namespace",
										},
									},
								},
								{Name: common.VeleroScratchDirEnvKey, Value: "/scratch"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: HostPods,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/lib/kubelet/pods",
									Type: ptr.To(corev1.HostPathUnset),
								},
							},
						},
						{
							Name: HostPlugins,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/lib/kubelet/plugins",
									Type: ptr.To(corev1.HostPathUnset),
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
				},
			},
		},
	}

	if options.labels != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Labels = common.AppendTTMapAsCopy(testBuiltNodeAgentDaemonSet.Spec.Template.Labels, options.labels)
	}

	if options.annotations != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Annotations = common.AppendTTMapAsCopy(testBuiltNodeAgentDaemonSet.Spec.Template.Annotations, options.annotations)
	}

	if options.env != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].Env = append(testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].Env, options.env...)
	}

	if options.volumes != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Volumes = append(testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Volumes, options.volumes...)
	}

	if options.volumeMounts != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].VolumeMounts = append(testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].VolumeMounts, options.volumeMounts...)
	}

	if options.nodeSelector != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.NodeSelector = options.nodeSelector
	}

	if options.resourceLimits != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].Resources.Limits = options.resourceLimits
	}

	if options.resourceRequests != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].Resources.Requests = options.resourceRequests
	}

	if options.toleration != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Tolerations = options.toleration
	}

	if len(options.dnsPolicy) > 0 {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.DNSPolicy = options.dnsPolicy
	}

	if options.dnsConfig != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.DNSConfig = options.dnsConfig
	}

	return testBuiltNodeAgentDaemonSet
}

func TestDPAReconciler_buildNodeAgentDaemonset(t *testing.T) {
	tests := []struct {
		name                   string
		dpa                    *oadpv1alpha1.DataProtectionApplication
		testProxy              bool
		clientObjects          []client.Object
		nodeAgentDaemonSet     *appsv1.DaemonSet
		wantNodeAgentDaemonSet *appsv1.DaemonSet
		errorMessage           string
	}{
		{
			name:         "DPA CR is nil, error is returned",
			errorMessage: "dpa cannot be nil",
		},
		{
			name:         "NodeAgent DaemonSet is nil, error is returned",
			dpa:          &oadpv1alpha1.DataProtectionApplication{},
			errorMessage: "ds cannot be nil",
		},
		{
			name: "valid DPA CR, NodeAgent DaemonSet is built",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{},
							UploaderType:          "kopia",
						},
					},
				},
			),
			clientObjects:          []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet:     testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{}),
		},
		{
			name: "valid DPA CR with PodConfig Env, NodeAgent DaemonSet is built with Container Env",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									Env: []corev1.EnvVar{
										{Name: "TEST_ENV", Value: "TEST_VALUE"},
									},
								},
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				env: []corev1.EnvVar{{Name: "TEST_ENV", Value: "TEST_VALUE"}},
			}),
		},
		{
			name: "valid DPA CR with PodConfig label, NodeAgent DaemonSet is built with template labels",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									Labels: map[string]string{
										"nodeAgentLabel": "this is a label",
									},
								},
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				labels: map[string]string{"nodeAgentLabel": "this is a label"},
			}),
		},
		{
			name: "invalid DPA CR with podConfig label, error is returned",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									Labels: map[string]string{
										"name": "not-node-agent", // this label is already defined by https://github.com/openshift/velero/blob/8b2f7dbdb510434b9c05180bae7a3fb2a8081e2f/pkg/install/daemonset.go#L71
									},
								},
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			errorMessage:       "NodeAgent daemonset template custom label: conflicting key name with value not-node-agent may not override node-agent",
		},
		{
			name: "valid DPA CR with Pod annotations, NodeAgent DaemonSet is built with template annotations",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{},
							UploaderType:          "kopia",
						},
					},
					PodAnnotations: map[string]string{
						"test-annotation": "awesome annotation",
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				annotations: map[string]string{"test-annotation": "awesome annotation"},
			}),
		},
		{
			name: "valid DPA CR with Unsupported NodeAgent Server Args, NodeAgent DaemonSet is built with Unsupported NodeAgent Server Args",
			dpa: createTestDpaWith(
				map[string]string{common.UnsupportedNodeAgentServerArgsAnnotation: "unsupported-node-agent-server-args-cm"},
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{},
							UploaderType:          "kopia",
						},
					},
				},
			),
			clientObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unsupported-node-agent-server-args-cm",
						Namespace: testNamespaceName,
					},
					Data: map[string]string{
						"unsupported-arg":      "value1",
						"unsupported-bool-arg": "True",
					},
				},
				testGenericInfrastructure,
			},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				args: []string{
					"--unsupported-arg=value1",
					"--unsupported-bool-arg=true",
				},
			}),
		},
		{
			name: "valid DPA CR with Empty String Unsupported NodeAgent Server Args, NodeAgent DaemonSet is built",
			dpa: createTestDpaWith(
				map[string]string{common.UnsupportedNodeAgentServerArgsAnnotation: ""},
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{},
							UploaderType:          "kopia",
						},
					},
				},
			),
			clientObjects:          []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet:     testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{}),
		},
		{
			name: "valid DPA CR with Unsupported NodeAgent Server Args and missing ConfigMap, error is returned",
			dpa: createTestDpaWith(
				map[string]string{common.UnsupportedNodeAgentServerArgsAnnotation: "missing-unsupported-node-agent-server-args-cm"},
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{},
							UploaderType:          "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			errorMessage:       "configmaps \"missing-unsupported-node-agent-server-args-cm\" not found",
		},
		{
			name: "valid DPA CR with NodeAgent resource allocations, NodeAgent DaemonSet is built with resource allocations",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									ResourceAllocations: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:              resource.MustParse("2"),
											corev1.ResourceMemory:           resource.MustParse("700Mi"),
											corev1.ResourceEphemeralStorage: resource.MustParse("400Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:              resource.MustParse("1"),
											corev1.ResourceMemory:           resource.MustParse("256Mi"),
											corev1.ResourceEphemeralStorage: resource.MustParse("300Mi"),
										},
									},
								},
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				resourceLimits: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("2"),
					corev1.ResourceMemory:           resource.MustParse("700Mi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("400Mi"),
				},
				resourceRequests: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("1"),
					corev1.ResourceMemory:           resource.MustParse("256Mi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("300Mi"),
				},
			}),
		},
		{
			name: "valid DPA CR with NodeAgent cpu limit, NodeAgent DaemonSet is built with cpu limit",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									ResourceAllocations: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("2"),
										},
									},
								},
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				resourceLimits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("2"),
				},
			}),
		},
		{
			name: "valid DPA CR with NodeAgent cpu request, NodeAgent DaemonSet is built with cpu request",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									ResourceAllocations: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("2"),
										},
									},
								},
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				resourceRequests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			}),
		},
		{
			name: "valid DPA CR with NodeAgent memory limit, NodeAgent DaemonSet is built with memory limit",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									ResourceAllocations: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("256Mi"),
										},
									},
								},
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				resourceLimits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			}),
		},
		{
			name: "valid DPA CR with NodeAgent memory request, NodeAgent DaemonSet is built with memory request",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									ResourceAllocations: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("256Mi"),
										},
									},
								},
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				resourceRequests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			}),
		},
		{
			name: "valid DPA CR with NodeAgent ephemeral-storage limit, NodeAgent DaemonSet is built with ephemeral-storage limit",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									ResourceAllocations: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceEphemeralStorage: resource.MustParse("300Mi"),
										},
									},
								},
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				resourceLimits: corev1.ResourceList{
					corev1.ResourceEphemeralStorage: resource.MustParse("300Mi"),
				},
			}),
		},
		{
			name: "valid DPA CR with NodeAgent ephemeral-storage request, NodeAgent DaemonSet is built with ephemeral-storage request",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									ResourceAllocations: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceEphemeralStorage: resource.MustParse("300Mi"),
										},
									},
								},
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				resourceRequests: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("500m"),
					corev1.ResourceMemory:           resource.MustParse("128Mi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("300Mi"),
				},
			}),
		},
		{
			name: "valid DPA CR with NodeAgent tolerations, NodeAgent DaemonSet is built with tolerations",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
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
							UploaderType: "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				toleration: []corev1.Toleration{
					{
						Key:      "key1",
						Operator: "Equal",
						Value:    "value1",
						Effect:   "NoSchedule",
					},
				},
			}),
		},
		{
			name: "valid DPA CR with NodeAgent nodeselector, NodeAgent DaemonSet is built with nodeselector",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									NodeSelector: map[string]string{"foo": "bar"},
								},
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				nodeSelector: map[string]string{"foo": "bar"},
			}),
		},
		{
			name: "valid DPA CR with aws plugin, NodeAgent DaemonSet is built",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{},
							UploaderType:          "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				volumes: []corev1.Volume{deploymentVolumeSecret("cloud-credentials")},
				volumeMounts: []corev1.VolumeMount{
					{Name: "cloud-credentials", MountPath: "/credentials"},
				},
				env: []corev1.EnvVar{
					{Name: common.AWSSharedCredentialsFileEnvKey, Value: "/credentials/cloud"},
				},
			}),
		},
		{
			name: "valid DPA CR with aws and kubevirt plugin, NodeAgent DaemonSet is built",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
								oadpv1alpha1.DefaultPluginKubeVirt,
							},
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{},
							UploaderType:          "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				volumes: []corev1.Volume{deploymentVolumeSecret("cloud-credentials")},
				volumeMounts: []corev1.VolumeMount{
					{Name: "cloud-credentials", MountPath: "/credentials"},
				},
				env: []corev1.EnvVar{
					{Name: common.AWSSharedCredentialsFileEnvKey, Value: "/credentials/cloud"},
				},
			}),
		},
		{
			name: "valid DPA CR with aws plugin from CloudStorage, NodeAgent DaemonSet is built",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{},
							UploaderType:          "kopia",
						},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							CloudStorage: &oadpv1alpha1.CloudStorageLocation{
								CloudStorageRef: corev1.LocalObjectReference{
									Name: "bucket-123",
								},
								Config: nil,
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
			),
			clientObjects: []client.Object{
				&oadpv1alpha1.CloudStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "bucket-123",
						Namespace: testNamespaceName,
					},
					Spec: oadpv1alpha1.CloudStorageSpec{
						EnableSharedConfig: ptr.To(true),
					},
				},
				testGenericInfrastructure,
			},
			nodeAgentDaemonSet:     testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{}),
		},
		{
			name: "valid DPA CR with aws plugin and BSL, NodeAgent DaemonSet is built",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{},
							UploaderType:          "kopia",
						},
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
				},
			),
			clientObjects:          []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet:     testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{}),
		},
		{
			name: "valid DPA CR with PodDNS Policy/Config, NodeAgent DaemonSet is built with DNS Policy/Config",
			dpa: createTestDpaWith(
				map[string]string{common.UnsupportedNodeAgentServerArgsAnnotation: ""},
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{},
							UploaderType:          "kopia",
						},
					},
					PodDnsPolicy: "None",
					PodDnsConfig: corev1.PodDNSConfig{
						Nameservers: []string{"1.1.1.1", "8.8.8.8"},
						Options: []corev1.PodDNSConfigOption{
							{Name: "ndots", Value: ptr.To("2")},
							{Name: "edns0"},
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				dnsPolicy: corev1.DNSNone,
				dnsConfig: &corev1.PodDNSConfig{
					Nameservers: []string{"1.1.1.1", "8.8.8.8"},
					Options: []corev1.PodDNSConfigOption{
						{Name: "ndots", Value: ptr.To("2")},
						{Name: "edns0"},
					},
				},
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(test.clientObjects...)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DPAReconciler{Client: fakeClient, dpa: test.dpa}
			if result, err := r.buildNodeAgentDaemonset(test.nodeAgentDaemonSet); err != nil {
				if test.errorMessage != err.Error() {
					t.Errorf("buildNodeAgentDaemonset() error = %v, errorMessage %v", err, test.errorMessage)
				}
			} else {
				if !reflect.DeepEqual(test.wantNodeAgentDaemonSet, result) {
					t.Errorf("expected NodeAgent DaemonSet diffs.\nDIFF:%v", cmp.Diff(test.wantNodeAgentDaemonSet, result))
				}
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
				dpa:           tt.dpa,
			}
			if err := r.updateFsRestoreHelperCM(tt.fsRestoreHelperCM); (err != nil) != tt.wantErr {
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
				dpa:           tt.dpa,
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

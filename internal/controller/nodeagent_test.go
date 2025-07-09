package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/operator-framework/operator-lib/proxy"
	"github.com/stretchr/testify/require"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/nodeagent"
	"github.com/vmware-tanzu/velero/pkg/util/kube"
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
	"github.com/openshift/oadp-operator/pkg/credentials/stsflow"
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
			r := &DataProtectionApplicationReconciler{
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
	args                    []string
	labels                  map[string]string
	annotations             map[string]string
	volumes                 []corev1.Volume
	volumeMounts            []corev1.VolumeMount
	env                     []corev1.EnvVar
	envFrom                 []corev1.EnvFromSource
	dnsPolicy               corev1.DNSPolicy
	dnsConfig               *corev1.PodDNSConfig
	resourceLimits          corev1.ResourceList
	resourceRequests        corev1.ResourceList
	dataMoverPrepareTimeout *string
	resourceTimeout         *string
	logFormat               *string
	toleration              []corev1.Toleration
	nodeSelector            map[string]string
	disableFsBackup         *bool
}

func createTestBuiltNodeAgentDaemonSet(options TestBuiltNodeAgentDaemonSetOptions) *appsv1.DaemonSet {

	containerVolumeMounts := []corev1.VolumeMount{}
	podVolumes := []corev1.Volume{}
	podSecurityContext := &corev1.PodSecurityContext{}

	if options.disableFsBackup == nil || !*options.disableFsBackup {
		podVolumes = append(podVolumes, corev1.Volume{
			Name: HostPods,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/kubelet/pods",
					Type: ptr.To(corev1.HostPathUnset),
				},
			},
		}, corev1.Volume{
			Name: HostPlugins,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/kubelet/plugins",
					Type: ptr.To(corev1.HostPathUnset),
				},
			},
		})

		containerVolumeMounts = append(containerVolumeMounts,
			corev1.VolumeMount{
				Name:             HostPods,
				MountPath:        "/host_pods",
				MountPropagation: &mountPropagationToHostContainer,
			},
			corev1.VolumeMount{
				Name:             HostPlugins,
				MountPath:        "/var/lib/kubelet/plugins",
				MountPropagation: &mountPropagationToHostContainer,
			},
		)

		podSecurityContext = &corev1.PodSecurityContext{
			RunAsNonRoot: ptr.To(false),
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeUnconfined,
			},
			RunAsUser: ptr.To(int64(0)),
		}
	} else {
		podSecurityContext = &corev1.PodSecurityContext{
			RunAsNonRoot: ptr.To(true),
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		}
	}
	podVolumes = append(podVolumes,
		corev1.Volume{
			Name: "scratch",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		corev1.Volume{
			Name: "certs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		corev1.Volume{
			Name: "home-velero",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		corev1.Volume{
			Name: "credentials",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium:    "",
					SizeLimit: nil,
				},
			},
		},
		corev1.Volume{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		corev1.Volume{
			Name: "bound-sa-token",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
								Audience:          "openshift",
								ExpirationSeconds: ptr.To(int64(3600)),
								Path:              "token",
							},
						},
					},
					DefaultMode: ptr.To(common.DefaultProjectedPermission),
				},
			},
		},
	)
	containerVolumeMounts = append(containerVolumeMounts,
		corev1.VolumeMount{
			Name:      "scratch",
			MountPath: "/scratch",
		},
		corev1.VolumeMount{
			Name:      "certs",
			MountPath: "/etc/ssl/certs",
		},
		corev1.VolumeMount{
			Name:      "bound-sa-token",
			MountPath: "/var/run/secrets/openshift/serviceaccount",
			ReadOnly:  true,
		},
		corev1.VolumeMount{
			Name:      "credentials",
			MountPath: "/tmp/credentials",
		},
		corev1.VolumeMount{
			Name:      "home-velero",
			MountPath: "/home/velero",
			ReadOnly:  false,
		},
		corev1.VolumeMount{
			Name:      "tmp",
			MountPath: "/tmp",
			ReadOnly:  false,
		},
	)

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
						"component":                          common.Velero,
						"name":                               common.NodeAgent,
						"openshift.io/node-agent-cm-version": "",
						"role":                               common.NodeAgent,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:                 corev1.RestartPolicyAlways,
					ServiceAccountName:            common.Velero,
					TerminationGracePeriodSeconds: ptr.To(int64(30)),
					DNSPolicy:                     corev1.DNSClusterFirst,
					NodeSelector: map[string]string{
						"kubernetes.io/os": "linux",
					},
					OS: &corev1.PodOS{
						Name: "linux",
					},
					DeprecatedServiceAccount: common.Velero,
					SecurityContext:          podSecurityContext,
					SchedulerName:            "default-scheduler",
					Containers: []corev1.Container{
						{
							Name:                     common.NodeAgent,
							Image:                    common.VeleroImage,
							ImagePullPolicy:          corev1.PullAlways,
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
							SecurityContext: &corev1.SecurityContext{
								ReadOnlyRootFilesystem: ptr.To(true),
							},
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
							Command:      []string{"/velero"},
							Args:         append([]string{common.NodeAgent, "server"}, options.args...),
							VolumeMounts: containerVolumeMounts,
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
					Volumes: podVolumes,
				},
			},
		},
	}

	if options.disableFsBackup != nil && *options.disableFsBackup {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].SecurityContext.Privileged = ptr.To(false)
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation = ptr.To(false)
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].SecurityContext.Capabilities = &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		}
	} else {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].SecurityContext.Privileged = ptr.To(true)
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation = ptr.To(true)
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

	if options.envFrom != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].EnvFrom = append(testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].EnvFrom, options.envFrom...)
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

	if options.dataMoverPrepareTimeout != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].Args = append(testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].Args, fmt.Sprintf("--data-mover-prepare-timeout=%s", *options.dataMoverPrepareTimeout))
	}

	if options.resourceTimeout != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].Args = append(testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].Args, fmt.Sprintf("--resource-timeout=%s", *options.resourceTimeout))
	}

	if options.logFormat != nil {
		testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].Args = append(testBuiltNodeAgentDaemonSet.Spec.Template.Spec.Containers[0].Args, fmt.Sprintf("--log-format=%s", *options.logFormat))
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
		envVars                map[string]string
	}{
		{
			name:         "NodeAgent DaemonSet is nil, error is returned",
			dpa:          &oadpv1alpha1.DataProtectionApplication{},
			errorMessage: "DaemonSet cannot be nil",
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
			clientObjects:      []client.Object{testGenericInfrastructure},
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
			name: "valid DPA CR with DataMoverPrepareTimeout, NodeAgent DaemonSet is built with DataMoverPrepareTimeout",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							DataMoverPrepareTimeout: &metav1.Duration{Duration: 10 * time.Second},
							UploaderType:            "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				dataMoverPrepareTimeout: ptr.To("10s"),
			}),
		},
		{
			name: "valid DPA CR with ResourceTimeout, NodeAgent DaemonSet is built with ResourceTimeout",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							ResourceTimeout: &metav1.Duration{Duration: 100 * time.Minute},
							UploaderType:    "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				resourceTimeout: ptr.To("1h40m0s"),
			}),
		},
		{
			name: "valid DPA CR with LogFormat set to json, NodeAgent DaemonSet is built with LogFormat set to json",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					LogFormat: "json",
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero:    &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				logFormat: ptr.To("json"),
			}),
		},
		{
			name: "valid DPA CR with LogFormat set to text, NodeAgent DaemonSet is built with LogFormat set to text",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					LogFormat: "text",
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero:    &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				logFormat: ptr.To("text"),
			}),
		},
		{
			name: "valid DPA CR with DataMoverPrepareTimeout and ResourceTimeout, NodeAgent DaemonSet is built with DataMoverPrepareTimeout and ResourceTimeout",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							DataMoverPrepareTimeout: &metav1.Duration{Duration: 10 * time.Second},
							ResourceTimeout:         &metav1.Duration{Duration: 10 * time.Minute},
							UploaderType:            "kopia",
						},
					},
				},
			),
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				dataMoverPrepareTimeout: ptr.To("10s"),
				resourceTimeout:         ptr.To("10m0s"),
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
			clientObjects:          []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet:     testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{}),
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
			clientObjects:          []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet:     testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{}),
		},
		{
			name: "valid DPA CR with disabled FS backup, NodeAgent DaemonSet is built",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
							DisableFsBackup: ptr.To(true),
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
				disableFsBackup: ptr.To(true),
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
			name: "valid DPA CR with aws and hypershift plugin, Velero Deployment is built with aws and hypershift plugin",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
								oadpv1alpha1.DefaultPluginHypershift,
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
		{
			name: "valid DPA CR with LoadConcurrency, NodeAgent DaemonSet is built with pointer to the config map",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{},
							NodeAgentConfigMapSettings: oadpv1alpha1.NodeAgentConfigMapSettings{
								LoadConcurrency: &oadpv1alpha1.LoadConcurrency{
									GlobalConfig: 10,
									PerNodeConfig: []oadpv1alpha1.RuledConfigs{
										{
											NodeSelector: metav1.LabelSelector{
												MatchLabels: map[string]string{"app": "velero"},
											},
											Number: 1,
										},
									},
								},
							},
						},
					},
				},
			),
			clientObjects: []client.Object{
				testGenericInfrastructure,
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      common.NodeAgentConfigMapPrefix + testDpaName,
						Namespace: testNamespaceName,
					},
					Data: map[string]string{
						"loadConcurrency": `{"globalConfig":10,"perNodeConfig":[{"nodeSelector":{"app":"velero"},"number":1}]}`,
					},
				},
			},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				args: []string{
					"--node-agent-configmap=node-agent-test-DPA-CR",
				},
				labels: map[string]string{"openshift.io/node-agent-cm-version": "999"},
			}),
		},
		{
			name: "valid DPA CR with NodeSelector from PodConfig, NodeAgent DaemonSet is built with pointer to the config map",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									NodeSelector: map[string]string{"foos": "bars"},
								},
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			clientObjects: []client.Object{
				testGenericInfrastructure,
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      common.NodeAgentConfigMapPrefix + testDpaName,
						Namespace: testNamespaceName,
					},
					Data: map[string]string{
						"loadAffinity": `{"nodeSelector":{"foos":"bars"}}`,
					},
				},
			},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				// We do override the nodeSelector with the values from the DPA CR's PodConfig
				// https://github.com/openshift/oadp-operator/pull/1666#discussion_r1998805581
				nodeSelector: map[string]string{"foos": "bars"},
				args: []string{
					"--node-agent-configmap=node-agent-test-DPA-CR",
				},
				labels: map[string]string{"openshift.io/node-agent-cm-version": "999"},
			}),
		},
		{
			name: "valid DPA CR with Azure workload identity env vars, NodeAgent DaemonSet is built with Azure env vars",
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
			envVars: map[string]string{
				stsflow.ClientIDEnvKey:       "test-azure-client-id",
				stsflow.TenantIDEnvKey:       "test-azure-tenant-id",
				stsflow.SubscriptionIDEnvKey: "test-azure-subscription-id",
			},
			clientObjects:      []client.Object{testGenericInfrastructure},
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			wantNodeAgentDaemonSet: createTestBuiltNodeAgentDaemonSet(TestBuiltNodeAgentDaemonSetOptions{
				envFrom: []corev1.EnvFromSource{
					{
						SecretRef: &corev1.SecretEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: stsflow.AzureWorkloadIdentitySecretName,
							},
						},
					},
				},
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Set environment variables if specified
			for key, value := range test.envVars {
				t.Setenv(key, value)
			}

			fakeClient, err := getFakeClientFromObjects(test.clientObjects...)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := &DataProtectionApplicationReconciler{Client: fakeClient, dpa: test.dpa}
			if r.dpa != nil && r.dpa.Spec.Configuration != nil {
				r.dpa.AutoCorrect()
			}
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

func createTestBuiltNodeAgentCM(data map[string]string) *corev1.ConfigMap {
	// Normalize multi-line JSON values
	for key, value := range data {
		var compactJSON bytes.Buffer
		if err := json.Compact(&compactJSON, []byte(value)); err != nil {
			fmt.Printf("Warning: Invalid JSON for key %s: %v\n", key, err)
		} else {
			data[key] = compactJSON.String()
		}
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.NodeAgentConfigMapPrefix + "test-dpa-configmap-cm",
			Namespace: "test-configmap-ns",
			Labels: map[string]string{
				"app.kubernetes.io/instance":   "test-dpa-configmap-cm",
				"app.kubernetes.io/managed-by": common.OADPOperator,
				"app.kubernetes.io/component":  "node-agent-config",
				oadpv1alpha1.OadpOperatorLabel: "True",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         oadpv1alpha1.SchemeBuilder.GroupVersion.String(),
					Kind:               "DataProtectionApplication",
					Name:               "test-dpa-configmap-cm",
					UID:                "",
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				},
			},
		},
		Data: data,
	}
}
func TestDPAReconciler_updateNodeAgentCM(t *testing.T) {
	testCmNs := "test-configmap-ns"
	testCmName := "test-dpa-configmap-cm"
	tests := []struct {
		name                   string
		nodeAgentConfigMap     *corev1.ConfigMap
		dpa                    *oadpv1alpha1.DataProtectionApplication
		wantErr                bool
		wantNodeAgentConfigMap *corev1.ConfigMap
	}{
		{
			name: "Given DPA CR instance, appropriate NodeAgent config cm is created with NodeSelector",
			nodeAgentConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      common.NodeAgentConfigMapPrefix + testCmName,
					Namespace: testCmNs,
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCmName,
					Namespace: testCmNs,
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{
									NodeSelector: map[string]string{"foos": "bars"},
								},
							},
						},
					},
				},
			},
			wantErr: false,
			wantNodeAgentConfigMap: createTestBuiltNodeAgentCM(map[string]string{
				"node-agent-config": `{
					"loadAffinity": [
						{
							"nodeSelector": {
								"matchLabels": {
									"foos": "bars"
								}
							}
						}
					]
				}`,
			}),
		},
		{
			name: "Given DPA CR instance, appropriate NodeAgent config cm is created with LoadConcurrency, BackupPVCConfig, RestorePVCConfig, PodResources",
			nodeAgentConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      common.NodeAgentConfigMapPrefix + testCmName,
					Namespace: testCmNs,
				},
			},
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCmName,
					Namespace: testCmNs,
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{},
							NodeAgentConfigMapSettings: oadpv1alpha1.NodeAgentConfigMapSettings{
								LoadConcurrency: &oadpv1alpha1.LoadConcurrency{
									GlobalConfig: 10,
									PerNodeConfig: []oadpv1alpha1.RuledConfigs{
										{
											NodeSelector: metav1.LabelSelector{
												MatchLabels: map[string]string{"app": "velero"},
											},
											Number: 1,
										},
										{
											NodeSelector: metav1.LabelSelector{
												MatchLabels: map[string]string{"app": "velero-2"},
											},
											Number: 5,
										},
									},
								},
								BackupPVCConfig: map[string]nodeagent.BackupPVC{
									"storage-class-1": {
										StorageClass: "backupPVC-storage-class",
										ReadOnly:     true,
									},
									"storage-class-2": {
										StorageClass: "backupPVC-storage-class",
									},
									"storage-class-3": {
										ReadOnly: true,
									},
									"storage-class-4": {
										ReadOnly:        true,
										SPCNoRelabeling: true,
									},
								},
								RestorePVCConfig: &nodeagent.RestorePVC{
									IgnoreDelayBinding: true,
								},
								PodResources: &kube.PodResources{
									CPURequest:    "100m",
									MemoryRequest: "100Mi",
									CPULimit:      "200m",
									MemoryLimit:   "200Mi",
								},
							},
						},
					},
				},
			},
			wantErr: false,
			wantNodeAgentConfigMap: createTestBuiltNodeAgentCM(map[string]string{
				"node-agent-config": `{
					"loadConcurrency": {
						"globalConfig": 10,
						"perNodeConfig": [
							{
								"nodeSelector": {
									"matchLabels": {
										"app": "velero"
									}
								},
								"number": 1
							},
							{
								"nodeSelector": {
									"matchLabels": {
										"app": "velero-2"
									}
								},
								"number": 5
							}
						]
					},
					"backupPVC": {
						"storage-class-1": {
							"storageClass": "backupPVC-storage-class",
							"readOnly": true
						},
						"storage-class-2": {
							"storageClass": "backupPVC-storage-class"
						},
						"storage-class-3": {
							"readOnly": true
						},
						"storage-class-4": {
							"readOnly": true,
							"spcNoRelabeling": true
						}
					},
					"podResources": {
						"cpuRequest": "100m",
						"memoryRequest": "100Mi",
						"cpuLimit": "200m",
						"memoryLimit": "200Mi"
					},
					"restorePVC": {
						"ignoreDelayBinding": true
					}
				}`,
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects()
			if err != nil {
				t.Fatalf("error in creating fake client, likely programmer error")
			}
			if tt.dpa != nil && tt.dpa.Spec.Configuration != nil {
				tt.dpa.AutoCorrect()
			}

			r := &DataProtectionApplicationReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(),
				NamespacedName: types.NamespacedName{
					Namespace: tt.dpa.Namespace,
					Name:      tt.dpa.Name,
				},
				EventRecorder: record.NewFakeRecorder(10),
				dpa:           tt.dpa,
			}
			err = r.updateNodeAgentCM(tt.nodeAgentConfigMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateNodeAgentCM() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Serialize both ConfigMaps to JSON strings
			wantJSON := tt.wantNodeAgentConfigMap.Data["node-agent-config"]
			gotJSON := tt.nodeAgentConfigMap.Data["node-agent-config"]

			// Unmarshal the JSON strings into maps to ignore key order, this is
			// required because the ConfigMap data is a string and we cannot
			// compare the maps directly.
			// Also we need to unmarshal into maps to ignore key order which is random.
			var wantMap map[string]interface{}
			var gotMap map[string]interface{}

			require.NoError(t, json.Unmarshal([]byte(wantJSON), &wantMap), "Failed to unmarshal wantJSON into map")
			require.NoError(t, json.Unmarshal([]byte(gotJSON), &gotMap), "Failed to unmarshal gotJSON into map")

			// Compare the unmarshalled maps
			require.Equal(t, wantMap, gotMap, "ConfigMaps are not equal")

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
			r := &DataProtectionApplicationReconciler{
				Client:  fakeClient,
				Scheme:  fakeClient.Scheme(),
				Log:     logr.Discard(),
				Context: newContextForTest(),
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
		envFS        string
		want         string
	}{
		{
			name:         "generic pv host path returned for empty platform type case",
			platformType: "",
			envFS:        "",
			want:         GenericPVHostPath,
		},
		{
			name:         "IBMCloud pv host path returned for IBMCloud platform type",
			platformType: IBMCloudPlatform,
			envFS:        "",
			want:         IBMCloudPVHostPath,
		},
		{
			name:         "empty platform type with fs env var set",
			platformType: "",
			envFS:        "/foo/file-system/bar",
			want:         "/foo/file-system/bar",
		},
		{
			name:         "IBMCloud platform type but env var also set, env var takes precedence",
			platformType: IBMCloudPlatform,
			envFS:        "/foo/file-system/env/var/override",
			want:         "/foo/file-system/env/var/override",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

func TestDPAReconciler_buildNodeAgentDaemonsetWithAzureWorkloadIdentity(t *testing.T) {
	tests := []struct {
		name               string
		dpa                *oadpv1alpha1.DataProtectionApplication
		envVars            map[string]string
		wantAzureEnvVars   bool
		nodeAgentDaemonSet *appsv1.DaemonSet
		clientObjects      []client.Object
	}{
		{
			name: "Azure STS credentials present - should add workload identity env vars",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								Enable: ptr.To(true),
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			envVars: map[string]string{
				"CLIENTID":       "test-client-id",
				"TENANTID":       "test-tenant-id",
				"SUBSCRIPTIONID": "test-subscription-id",
			},
			wantAzureEnvVars:   true,
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			clientObjects:      []client.Object{testGenericInfrastructure},
		},
		{
			name: "No Azure credentials - should not add workload identity env vars",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								Enable: ptr.To(true),
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			envVars:            map[string]string{},
			wantAzureEnvVars:   false,
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			clientObjects:      []client.Object{testGenericInfrastructure},
		},
		{
			name: "Partial Azure credentials - should not add workload identity env vars",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								Enable: ptr.To(true),
							},
							UploaderType: "kopia",
						},
					},
				},
			),
			envVars: map[string]string{
				"CLIENTID": "test-client-id",
				"TENANTID": "test-tenant-id",
				// Missing SUBSCRIPTIONID
			},
			wantAzureEnvVars:   false,
			nodeAgentDaemonSet: testNodeAgentDaemonSet.DeepCopy(),
			clientObjects:      []client.Object{testGenericInfrastructure},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			fakeClient, err := getFakeClientFromObjects(tt.clientObjects...)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}

			r := &DataProtectionApplicationReconciler{
				Client: fakeClient,
				dpa:    tt.dpa,
				Log:    logr.Discard(),
			}

			// Build the daemonset
			_, err = r.buildNodeAgentDaemonset(tt.nodeAgentDaemonSet)
			if err != nil {
				t.Errorf("buildNodeAgentDaemonset() error = %v", err)
				return
			}

			// Check if Azure workload identity env vars are present
			if tt.wantAzureEnvVars {
				// Check that Azure workload identity secret reference is added via envFrom
				foundAzureSecretRef := false
				for _, container := range tt.nodeAgentDaemonSet.Spec.Template.Spec.Containers {
					if container.Name == common.NodeAgent {
						for _, envFrom := range container.EnvFrom {
							if envFrom.SecretRef != nil && envFrom.SecretRef.Name == stsflow.AzureWorkloadIdentitySecretName {
								foundAzureSecretRef = true
								break
							}
						}
						break
					}
				}
				if !foundAzureSecretRef {
					t.Errorf("Expected %s secret reference in envFrom", stsflow.AzureWorkloadIdentitySecretName)
				}
			} else {
				// Check that Azure workload identity secret reference is NOT set
				for _, container := range tt.nodeAgentDaemonSet.Spec.Template.Spec.Containers {
					if container.Name == common.NodeAgent {
						for _, envFrom := range container.EnvFrom {
							if envFrom.SecretRef != nil && envFrom.SecretRef.Name == stsflow.AzureWorkloadIdentitySecretName {
								t.Errorf("Expected %s secret reference to NOT be set in envFrom", stsflow.AzureWorkloadIdentitySecretName)
							}
						}
						// Also check that Azure environment variables are NOT set directly
						for _, env := range container.Env {
							if env.Name == "AZURE_CLIENT_ID" || env.Name == "AZURE_FEDERATED_TOKEN_FILE" {
								t.Errorf("Expected Azure environment variables to NOT be set, but found %s", env.Name)
							}
						}
						break
					}
				}
			}
		})
	}
}

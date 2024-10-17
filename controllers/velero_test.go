package controllers

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/operator-framework/operator-lib/proxy"
	"github.com/sirupsen/logrus"
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
	oadpclient "github.com/openshift/oadp-operator/pkg/client"
	"github.com/openshift/oadp-operator/pkg/common"
)

const (
	proxyEnvKey                      = "HTTP_PROXY"
	proxyEnvValue                    = "http://proxy.example.com:8080"
	argsMetricsPortTest              = 69420
	defaultFileSystemBackupTimeout   = "--fs-backup-timeout=4h"
	defaultRestoreResourcePriorities = "--restore-resource-priorities=securitycontextconstraints,customresourcedefinitions,klusterletconfigs.config.open-cluster-management.io,managedcluster.cluster.open-cluster-management.io,namespaces,roles,rolebindings,clusterrolebindings,klusterletaddonconfig.agent.open-cluster-management.io,managedclusteraddon.addon.open-cluster-management.io,storageclasses,volumesnapshotclass.snapshot.storage.k8s.io,volumesnapshotcontents.snapshot.storage.k8s.io,volumesnapshots.snapshot.storage.k8s.io,datauploads.velero.io,persistentvolumes,persistentvolumeclaims,serviceaccounts,secrets,configmaps,limitranges,pods,replicasets.apps,clusterclasses.cluster.x-k8s.io,endpoints,services,-,clusterbootstraps.run.tanzu.vmware.com,clusters.cluster.x-k8s.io,clusterresourcesets.addons.cluster.x-k8s.io"
	defaultDisableInformerCache      = "--disable-informer-cache=false"

	testNamespaceName        = "test-ns"
	testDpaName              = "test-DPA-CR"
	testVeleroDeploymentName = "test-velero-deployment"
)

var (
	veleroDeploymentLabel = map[string]string{
		"app.kubernetes.io/name":       common.Velero,
		"app.kubernetes.io/instance":   testDpaName,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  Server,
		"component":                    "velero",
		oadpv1alpha1.OadpOperatorLabel: "True",
	}
	veleroPodLabelAppend        = map[string]string{"deploy": "velero"}
	veleroDeploymentMatchLabels = common.AppendTTMapAsCopy(veleroDeploymentLabel, veleroPodLabelAppend)
	veleroPodAnnotations        = map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/port":   "8085",
		"prometheus.io/path":   "/metrics",
	}
	veleroPodObjectMeta = metav1.ObjectMeta{
		Labels:      veleroDeploymentMatchLabels,
		Annotations: veleroPodAnnotations,
	}

	baseObjectMeta = metav1.ObjectMeta{
		Name:      testVeleroDeploymentName,
		Namespace: testNamespaceName,
		Labels:    veleroDeploymentLabel,
	}

	baseTypeMeta = metav1.TypeMeta{
		Kind:       "Deployment",
		APIVersion: appsv1.SchemeGroupVersion.String(),
	}

	baseEnvVars = []corev1.EnvVar{
		{Name: common.VeleroScratchDirEnvKey, Value: "/scratch"},
		{
			Name: common.VeleroNamespaceEnvKey,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.namespace",
				},
			},
		},
		{Name: common.LDLibraryPathEnvKey, Value: "/plugins"},
		{Name: "OPENSHIFT_IMAGESTREAM_BACKUP", Value: "true"},
	}

	baseVolumeMounts = []corev1.VolumeMount{
		{Name: "plugins", MountPath: "/plugins"},
		{Name: "scratch", MountPath: "/scratch"},
		{Name: "certs", MountPath: "/etc/ssl/certs"},
	}

	baseVolumes = []corev1.Volume{
		{
			Name:         "plugins",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
		{
			Name:         "scratch",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
		{
			Name:         "certs",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
	}
	baseContainer = corev1.Container{
		Image:           common.AWSPluginImage,
		Name:            common.VeleroPluginForAWS,
		ImagePullPolicy: corev1.PullAlways,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: "File",
		VolumeMounts: []corev1.VolumeMount{
			{MountPath: "/target", Name: "plugins"},
		},
	}

	allDefaultPluginsList = []oadpv1alpha1.DefaultPlugin{
		oadpv1alpha1.DefaultPluginAWS,
		oadpv1alpha1.DefaultPluginGCP,
		oadpv1alpha1.DefaultPluginMicrosoftAzure,
		oadpv1alpha1.DefaultPluginKubeVirt,
		oadpv1alpha1.DefaultPluginOpenShift,
		oadpv1alpha1.DefaultPluginCSI,
	}

	testVeleroDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testVeleroDeploymentName,
			Namespace: testNamespaceName,
		},
	}
)

type ReconcileVeleroControllerScenario struct {
	namespace string
	dpaName   string
	envVar    corev1.EnvVar
}

var _ = ginkgo.Describe("Test ReconcileVeleroDeployment function", func() {
	var (
		ctx                 = context.Background()
		currentTestScenario ReconcileVeleroControllerScenario
		updateTestScenario  = func(scenario ReconcileVeleroControllerScenario) {
			currentTestScenario = scenario
		}
	)

	ginkgo.AfterEach(func() {
		os.Unsetenv(currentTestScenario.envVar.Name)

		deployment := &appsv1.Deployment{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      common.Velero,
				Namespace: currentTestScenario.namespace,
			},
			deployment,
		) == nil {
			gomega.Expect(k8sClient.Delete(ctx, deployment)).To(gomega.Succeed())
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
	})

	ginkgo.DescribeTable("Check if Subscription Config environment variables are passed to Velero Containers",
		func(scenario ReconcileVeleroControllerScenario) {
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
			result, err := r.ReconcileVeleroDeployment(logr.Discard())

			gomega.Expect(result).To(gomega.BeTrue())
			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))

			gomega.Expect(len(event.Events)).To(gomega.Equal(1))
			message := <-event.Events
			for _, word := range []string{"Normal", "VeleroDeploymentReconciled", "created"} {
				gomega.Expect(message).To(gomega.ContainSubstring(word))
			}

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(
				ctx,
				types.NamespacedName{
					Name:      common.Velero,
					Namespace: scenario.namespace,
				},
				deployment,
			)
			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))

			if slices.Contains(proxy.ProxyEnvNames, scenario.envVar.Name) {
				for _, container := range deployment.Spec.Template.Spec.Containers {
					gomega.Expect(container.Env).To(gomega.ContainElement(scenario.envVar))
				}
			} else {
				for _, container := range deployment.Spec.Template.Spec.Containers {
					gomega.Expect(container.Env).To(gomega.Not(gomega.ContainElement(scenario.envVar)))
				}
			}
		},
		ginkgo.Entry("Should add HTTP_PROXY environment variable to Velero Containers", ReconcileVeleroControllerScenario{
			namespace: "test-velero-environment-variables-1",
			dpaName:   "test-velero-environment-variables-1-dpa",
			envVar: corev1.EnvVar{
				Name:  "HTTP_PROXY",
				Value: "http://proxy.example.com:8080",
			},
		}),
		ginkgo.Entry("Should add HTTPS_PROXY environment variable to Velero Containers", ReconcileVeleroControllerScenario{
			namespace: "test-velero-environment-variables-2",
			dpaName:   "test-velero-environment-variables-2-dpa",
			envVar: corev1.EnvVar{
				Name:  "HTTPS_PROXY",
				Value: "localhost",
			},
		}),
		ginkgo.Entry("Should add NO_PROXY environment variable to Velero Containers", ReconcileVeleroControllerScenario{
			namespace: "test-velero-environment-variables-3",
			dpaName:   "test-velero-environment-variables-3-dpa",
			envVar: corev1.EnvVar{
				Name:  "NO_PROXY",
				Value: "1.1.1.1",
			},
		}),
		ginkgo.Entry("Should NOT add WRONG environment variable to Velero Containers", ReconcileVeleroControllerScenario{
			namespace: "test-velero-environment-variables-4",
			dpaName:   "test-velero-environment-variables-4-dpa",
			envVar: corev1.EnvVar{
				Name:  "WRONG",
				Value: "I do not know what is happening here",
			},
		}),
	)
})

func pluginContainer(name, image string) corev1.Container {
	container := baseContainer
	container.Name = name
	container.Image = image
	return container
}

func deploymentVolumeSecret(name string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  name,
				DefaultMode: ptr.To(int32(0440)),
			},
		},
	}
}

func createTestDpaWith(
	dpaAnnotations map[string]string,
	dpaSpec oadpv1alpha1.DataProtectionApplicationSpec,
) *oadpv1alpha1.DataProtectionApplication {
	return &oadpv1alpha1.DataProtectionApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testDpaName,
			Namespace:   testNamespaceName,
			Annotations: dpaAnnotations,
		},
		Spec: dpaSpec,
	}
}

type TestBuiltVeleroDeploymentOptions struct {
	args             []string
	customLabels     map[string]string
	labels           map[string]string
	annotations      map[string]string
	metricsPort      int
	initContainers   []corev1.Container
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

func createTestBuiltVeleroDeployment(options TestBuiltVeleroDeploymentOptions) *appsv1.Deployment {
	testBuiltVeleroDeployment := &appsv1.Deployment{
		ObjectMeta: baseObjectMeta,
		TypeMeta:   baseTypeMeta,
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: veleroDeploymentMatchLabels},
			Replicas: ptr.To(int32(1)),
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
					MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
				},
			},
			RevisionHistoryLimit:    ptr.To(int32(10)),
			ProgressDeadlineSeconds: ptr.To(int32(600)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: veleroPodObjectMeta,
				Spec: corev1.PodSpec{
					RestartPolicy:                 corev1.RestartPolicyAlways,
					ServiceAccountName:            common.Velero,
					TerminationGracePeriodSeconds: ptr.To(int64(30)),
					DNSPolicy:                     corev1.DNSClusterFirst,
					DeprecatedServiceAccount:      common.Velero,
					SecurityContext:               &corev1.PodSecurityContext{},
					SchedulerName:                 "default-scheduler",
					Containers: []corev1.Container{
						{
							Name:                     common.Velero,
							Image:                    common.VeleroImage,
							ImagePullPolicy:          corev1.PullAlways,
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							Ports: []corev1.ContainerPort{{
								Name:          "metrics",
								ContainerPort: 8085,
								Protocol:      corev1.ProtocolTCP,
							}},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
							Command:      []string{"/velero"},
							Args:         append([]string{"server"}, options.args...),
							VolumeMounts: baseVolumeMounts,
							Env:          baseEnvVars,
						},
					},
					Volumes:        baseVolumes,
					InitContainers: []corev1.Container{},
				},
			},
		},
	}

	if options.customLabels != nil {
		testBuiltVeleroDeployment.Labels = common.AppendTTMapAsCopy(testBuiltVeleroDeployment.Labels, options.customLabels)
		testBuiltVeleroDeployment.Spec.Selector.MatchLabels = common.AppendTTMapAsCopy(testBuiltVeleroDeployment.Spec.Selector.MatchLabels, options.customLabels)
		testBuiltVeleroDeployment.Spec.Template.Labels = common.AppendTTMapAsCopy(testBuiltVeleroDeployment.Spec.Template.Labels, options.customLabels)
	}

	if options.labels != nil {
		testBuiltVeleroDeployment.Spec.Template.Labels = common.AppendTTMapAsCopy(testBuiltVeleroDeployment.Spec.Template.Labels, options.labels)
	}

	if options.annotations != nil {
		testBuiltVeleroDeployment.Spec.Template.Annotations = common.AppendTTMapAsCopy(testBuiltVeleroDeployment.Spec.Template.Annotations, options.annotations)
	}

	if options.initContainers != nil {
		testBuiltVeleroDeployment.Spec.Template.Spec.InitContainers = options.initContainers
	}

	if options.volumes != nil {
		testBuiltVeleroDeployment.Spec.Template.Spec.Volumes = append(baseVolumes, options.volumes...)
	}

	if options.volumeMounts != nil {
		testBuiltVeleroDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(baseVolumeMounts, options.volumeMounts...)
	}

	if options.env != nil {
		testBuiltVeleroDeployment.Spec.Template.Spec.Containers[0].Env = options.env
	}

	if options.resourceLimits != nil {
		testBuiltVeleroDeployment.Spec.Template.Spec.Containers[0].Resources.Limits = options.resourceLimits
	}

	if options.resourceRequests != nil {
		testBuiltVeleroDeployment.Spec.Template.Spec.Containers[0].Resources.Requests = options.resourceRequests
	}

	if options.toleration != nil {
		testBuiltVeleroDeployment.Spec.Template.Spec.Tolerations = options.toleration
	}

	if options.nodeSelector != nil {
		testBuiltVeleroDeployment.Spec.Template.Spec.NodeSelector = options.nodeSelector
	}

	if options.metricsPort != 0 {
		testBuiltVeleroDeployment.Spec.Template.Annotations = common.AppendTTMapAsCopy(testBuiltVeleroDeployment.Spec.Template.Annotations, map[string]string{"prometheus.io/port": strconv.Itoa(options.metricsPort)})
		testBuiltVeleroDeployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort = int32(options.metricsPort)
	}

	if len(options.dnsPolicy) > 0 {
		testBuiltVeleroDeployment.Spec.Template.Spec.DNSPolicy = options.dnsPolicy
	}

	if options.dnsConfig != nil {
		testBuiltVeleroDeployment.Spec.Template.Spec.DNSConfig = options.dnsConfig
	}

	return testBuiltVeleroDeployment
}

func TestDPAReconciler_buildVeleroDeployment(t *testing.T) {
	tests := []struct {
		name                 string
		dpa                  *oadpv1alpha1.DataProtectionApplication
		testProxy            bool
		clientObjects        []client.Object
		veleroDeployment     *appsv1.Deployment
		wantVeleroDeployment *appsv1.Deployment
		errorMessage         string
	}{
		{
			name:         "DPA CR is nil, error is returned",
			errorMessage: "DPA CR cannot be nil",
		},
		{
			name:         "Velero Deployment is nil, error is returned",
			dpa:          &oadpv1alpha1.DataProtectionApplication{},
			errorMessage: "velero deployment cannot be nil",
		},
		{
			name: "valid DPA CR, Velero Deployment is built with default args",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR, Velero Deployment is built with custom labels",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			),
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testVeleroDeploymentName,
					Namespace: testNamespaceName,
					Labels:    map[string]string{"foo": "bar"},
				},
			},
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				customLabels: map[string]string{"foo": "bar"},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with PodConfig Env, Velero Deployment is built with Container Env",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								Env: []corev1.EnvVar{
									{Name: "TEST_ENV", Value: "TEST_VALUE"},
								},
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				env: slices.Insert(baseEnvVars, 3, []corev1.EnvVar{{Name: "TEST_ENV", Value: "TEST_VALUE"}}...),
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with proxy Env, Velero Deployment is built with Container Env",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			),
			testProxy:        true,
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				env: slices.Insert(baseEnvVars, 3, []corev1.EnvVar{
					{Name: proxyEnvKey, Value: proxyEnvValue},
					{Name: strings.ToLower(proxyEnvKey), Value: proxyEnvValue},
				}...),
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with PodConfig label, Velero Deployment is built with template labels",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								Labels: map[string]string{
									"thisIsVelero": "yes",
								},
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				labels: map[string]string{"thisIsVelero": "yes"},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "invalid DPA CR with podConfig label, error is returned",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								Labels: map[string]string{
									"component": common.NodeAgent,
								},
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			errorMessage:     "velero deployment template custom label: conflicting key component with value node-agent may not override velero",
		},
		{
			name: "valid DPA CR with Pod annotations, Velero Deployment is built with template annotations",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					PodAnnotations: map[string]string{
						"test-annotation": "awesome annotation",
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				annotations: map[string]string{"test-annotation": "awesome annotation"},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with noDefaultBackupLocation, all default plugins and unsupportedOverrides operatorType MTC, Velero Deployment is built with secret volumes",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							NoDefaultBackupLocation: true,
							DefaultPlugins:          allDefaultPluginsList,
						},
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.OperatorTypeKey: oadpv1alpha1.OperatorTypeMTC,
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				initContainers: []corev1.Container{
					pluginContainer(common.VeleroPluginForAWS, common.AWSPluginImage),
					pluginContainer(common.VeleroPluginForGCP, common.GCPPluginImage),
					pluginContainer(common.VeleroPluginForAzure, common.AzurePluginImage),
					pluginContainer(common.KubeVirtPlugin, common.KubeVirtPluginImage),
					pluginContainer(common.VeleroPluginForOpenshift, common.OpenshiftPluginImage),
				},
				volumes: []corev1.Volume{
					deploymentVolumeSecret("cloud-credentials"),
					deploymentVolumeSecret("cloud-credentials-gcp"),
					deploymentVolumeSecret("cloud-credentials-azure"),
				},
				volumeMounts: []corev1.VolumeMount{
					{Name: "cloud-credentials", MountPath: "/credentials"},
					{Name: "cloud-credentials-gcp", MountPath: "/credentials-gcp"},
					{Name: "cloud-credentials-azure", MountPath: "/credentials-azure"},
				},
				env: append(baseEnvVars, []corev1.EnvVar{
					{Name: common.AWSSharedCredentialsFileEnvKey, Value: "/credentials/cloud"},
					{Name: common.GCPCredentialsEnvKey, Value: "/credentials-gcp/cloud"},
					{Name: common.AzureCredentialsFileEnvKey, Value: "/credentials-azure/cloud"},
				}...),
				args: []string{
					"--features=EnableCSI",
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Unsupported Server Args, Velero Deployment is built with Unsupported Server Args",
			dpa: createTestDpaWith(
				map[string]string{common.UnsupportedVeleroServerArgsAnnotation: "unsupported-server-args-cm"},
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			),
			clientObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unsupported-server-args-cm",
						Namespace: testNamespaceName,
					},
					Data: map[string]string{
						"unsupported-arg":      "value1",
						"unsupported-bool-arg": "True",
					},
				},
			},
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					"--unsupported-arg=value1",
					"--unsupported-bool-arg=true",
				},
			}),
		},
		{
			name: "valid DPA CR with Empty String Unsupported Server Args, Velero Deployment is built with default args",
			dpa: createTestDpaWith(
				map[string]string{common.UnsupportedVeleroServerArgsAnnotation: ""},
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Unsupported Server Args and multiple options, Velero Deployment is built with Unsupported Server Args only",
			dpa: createTestDpaWith(
				map[string]string{common.UnsupportedVeleroServerArgsAnnotation: "unsupported-server-args-cm"},
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DisableInformerCache:       ptr.To(true),
							DefaultSnapshotMoveData:    ptr.To(true),
							ItemOperationSyncFrequency: "7m",
						},
					},
				},
			),
			clientObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unsupported-server-args-cm",
						Namespace: testNamespaceName,
					},
					Data: map[string]string{
						"unsupported-arg":      "value1",
						"unsupported-bool-arg": "True",
					},
				},
			},
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					"--unsupported-arg=value1",
					"--unsupported-bool-arg=true",
				},
			}),
		},
		{
			name: "valid DPA CR with Unsupported Server Args and missing ConfigMap, error is returned",
			dpa: createTestDpaWith(
				map[string]string{"oadp.openshift.io/unsupported-velero-server-args": "missing-unsupported-server-args-cm"},
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			errorMessage:     "configmaps \"missing-unsupported-server-args-cm\" not found",
		},
		{
			name: "valid DPA CR with ItemOperationSyncFrequency, Velero Deployment is built with ItemOperationSyncFrequency arg",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							ItemOperationSyncFrequency: "5m",
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					"--item-operation-sync-frequency=5m",
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with multiple options, Velero Deployment is built with multiple Args",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							LogLevel:                    logrus.InfoLevel.String(),
							ItemOperationSyncFrequency:  "5m",
							DefaultItemOperationTimeout: "2h",
							DefaultSnapshotMoveData:     ptr.To(false),
							NoDefaultBackupLocation:     true,
							DefaultVolumesToFSBackup:    ptr.To(true),
							DefaultPlugins:              []oadpv1alpha1.DefaultPlugin{oadpv1alpha1.DefaultPluginCSI},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					"--features=EnableCSI",
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					"--log-level",
					logrus.InfoLevel.String(),
					"--item-operation-sync-frequency=5m",
					"--default-item-operation-timeout=2h",
					"--default-snapshot-move-data=false",
					"--default-volumes-to-fs-backup=true",
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with SnapshotMovedata false, Velero Deployment is built with SnapshotMovedata false",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultSnapshotMoveData: ptr.To(false),
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					"--default-snapshot-move-data=false",
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with SnapshotMovedata true, Velero Deployment is built with SnapshotMovedata true",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultSnapshotMoveData: ptr.To(true),
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					"--default-snapshot-move-data=true",
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with DefaultVolumesToFSBackup true, Velero Deployment is built with DefaultVolumesToFSBackup true",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultVolumesToFSBackup: ptr.To(true),
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					"--default-volumes-to-fs-backup=true",
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with DisableInformerCache true, Velero Deployment is built with DisableInformerCache true",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DisableInformerCache: ptr.To(true),
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					"--disable-informer-cache=true",
				},
			}),
		},
		{
			name: "valid DPA CR with DisableInformerCache false, Velero Deployment is built with DisableInformerCache false",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DisableInformerCache: ptr.To(false),
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with DefaultItemOperationTimeout, Velero Deployment is built with DefaultItemOperationTimeout arg",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultItemOperationTimeout: "2h",
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					"--default-item-operation-timeout=2h",
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with log level, Velero Deployment is built with log level arg",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							LogLevel: logrus.InfoLevel.String(),
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					"--log-level",
					logrus.InfoLevel.String(),
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "invalid DPA CR with log level, error is returned",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							LogLevel: logrus.InfoLevel.String() + "typo",
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			errorMessage:     "invalid log level infotypo, use: trace, debug, info, warning, error, fatal, or panic",
		},
		{
			name: "valid DPA CR with ResourceTimeout, Velero Deployment is built with ResourceTimeout arg",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							ResourceTimeout: "5m",
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					"--resource-timeout=5m",
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Velero resource allocations, Velero Deployment is built with resource allocations",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
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
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
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
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Velero cpu limit, Velero Deployment is built with cpu limit",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								ResourceAllocations: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("2"),
									},
								},
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				resourceLimits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("2"),
				},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Velero cpu request, Velero Deployment is built with cpu request",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								ResourceAllocations: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("2"),
									},
								},
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				resourceRequests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Velero memory limit, Velero Deployment is built with memory limit",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								ResourceAllocations: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
								},
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				resourceLimits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Velero memory request, Velero Deployment is built with memory request",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								ResourceAllocations: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
								},
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				resourceRequests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Velero ephemeral-storage limit, Velero Deployment is built with ephemeral-storage limit",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								ResourceAllocations: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceEphemeralStorage: resource.MustParse("400Mi"),
									},
								},
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				resourceLimits: corev1.ResourceList{
					corev1.ResourceEphemeralStorage: resource.MustParse("400Mi"),
				},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Velero ephemeral-storage request, Velero Deployment is built with ephemeral-storage request",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								ResourceAllocations: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceEphemeralStorage: resource.MustParse("300Mi"),
									},
								},
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				resourceRequests: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("500m"),
					corev1.ResourceMemory:           resource.MustParse("128Mi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("300Mi"),
				},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Velero tolerations, Velero Deployment is built with tolerations",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
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
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				toleration: []corev1.Toleration{
					{
						Key:      "key1",
						Operator: "Equal",
						Value:    "value1",
						Effect:   "NoSchedule",
					},
				},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Velero nodeselector, Velero Deployment is built with nodeselector",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							PodConfig: &oadpv1alpha1.PodConfig{
								NodeSelector: map[string]string{"foo": "bar"},
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				nodeSelector: map[string]string{"foo": "bar"},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with aws plugin, Velero Deployment is built with aws plugin",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				initContainers: []corev1.Container{pluginContainer(common.VeleroPluginForAWS, common.AWSPluginImage)},
				volumes:        []corev1.Volume{deploymentVolumeSecret("cloud-credentials")},
				volumeMounts: []corev1.VolumeMount{
					{Name: "cloud-credentials", MountPath: "/credentials"},
				},
				env: append(baseEnvVars, []corev1.EnvVar{
					{Name: common.AWSSharedCredentialsFileEnvKey, Value: "/credentials/cloud"},
				}...),
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with legacy aws plugin, Velero Deployment is built with legacy aws plugin",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginLegacyAWS,
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				initContainers: []corev1.Container{pluginContainer(common.VeleroPluginForLegacyAWS, common.LegacyAWSPluginImage)},
				volumes:        []corev1.Volume{deploymentVolumeSecret("cloud-credentials")},
				volumeMounts: []corev1.VolumeMount{
					{Name: "cloud-credentials", MountPath: "/credentials"},
				},
				env: append(baseEnvVars, []corev1.EnvVar{
					{Name: common.AWSSharedCredentialsFileEnvKey, Value: "/credentials/cloud"},
				}...),
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with aws and kubevirt plugin, Velero Deployment is built with aws and kubevirt plugin",
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
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				initContainers: []corev1.Container{
					pluginContainer(common.VeleroPluginForAWS, common.AWSPluginImage),
					pluginContainer(common.KubeVirtPlugin, common.KubeVirtPluginImage),
				},
				volumes: []corev1.Volume{deploymentVolumeSecret("cloud-credentials")},
				volumeMounts: []corev1.VolumeMount{
					{Name: "cloud-credentials", MountPath: "/credentials"},
				},
				env: append(baseEnvVars, []corev1.EnvVar{
					{Name: common.AWSSharedCredentialsFileEnvKey, Value: "/credentials/cloud"},
				}...),
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with aws plugin from CloudStorage, Velero Deployment is built with aws plugin",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
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
			},
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				initContainers: []corev1.Container{pluginContainer(common.VeleroPluginForAWS, common.AWSPluginImage)},
				volumes: []corev1.Volume{
					{
						Name: "bound-sa-token",
						VolumeSource: corev1.VolumeSource{
							Projected: &corev1.ProjectedVolumeSource{
								DefaultMode: ptr.To(int32(0644)),
								Sources: []corev1.VolumeProjection{
									{
										ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
											Audience:          "openshift",
											ExpirationSeconds: ptr.To(int64(3600)),
											Path:              "token",
										},
									},
								},
							},
						},
					},
				},
				volumeMounts: []corev1.VolumeMount{
					{Name: "bound-sa-token", MountPath: "/var/run/secrets/openshift/serviceaccount", ReadOnly: true},
				},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with aws plugin and BSL, Velero Deployment is built with aws plugin",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
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
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				initContainers: []corev1.Container{pluginContainer(common.VeleroPluginForAWS, common.AWSPluginImage)},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with PodDNS Policy/Config, Velero Deployment is built with DNS Policy/Config",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
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
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				dnsPolicy: corev1.DNSNone,
				dnsConfig: &corev1.PodDNSConfig{
					Nameservers: []string{"1.1.1.1", "8.8.8.8"},
					Options: []corev1.PodDNSConfigOption{
						{Name: "ndots", Value: ptr.To("2")},
						{Name: "edns0"},
					},
				},
				args: []string{
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with metrics address, Velero Deployment is built with metrics address",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							Args: &oadpv1alpha1.VeleroServerArgs{
								ServerFlags: oadpv1alpha1.ServerFlags{
									MetricsAddress: fmt.Sprintf(":%v", argsMetricsPortTest),
								},
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				metricsPort: argsMetricsPortTest,
				args: []string{
					fmt.Sprintf("--metrics-address=:%v", argsMetricsPortTest),
					"--fs-backup-timeout=4h0m0s",
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with restore resource priorities, Velero Deployment is built with restore resource priorities",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							Args: &oadpv1alpha1.VeleroServerArgs{
								ServerFlags: oadpv1alpha1.ServerFlags{
									RestoreResourcePriorities: "securitycontextconstraints,test",
								},
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					"--fs-backup-timeout=4h0m0s",
					"--restore-resource-priorities=securitycontextconstraints,test",
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Velero time fields, Velero Deployment is built with time fields args",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							Args: &oadpv1alpha1.VeleroServerArgs{
								ServerFlags: oadpv1alpha1.ServerFlags{
									BackupSyncPeriod:            ptr.To(time.Duration(1)),
									PodVolumeOperationTimeout:   ptr.To(time.Duration(1)),
									ResourceTerminatingTimeout:  ptr.To(time.Duration(1)),
									DefaultBackupTTL:            ptr.To(time.Duration(1)),
									StoreValidationFrequency:    ptr.To(time.Duration(1)),
									ItemOperationSyncFrequency:  ptr.To(time.Duration(1)),
									RepoMaintenanceFrequency:    ptr.To(time.Duration(1)),
									GarbageCollectionFrequency:  ptr.To(time.Duration(1)),
									DefaultItemOperationTimeout: ptr.To(time.Duration(1)),
									ResourceTimeout:             ptr.To(time.Duration(1)),
								},
							},
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					"--backup-sync-period=1ns",
					"--default-backup-ttl=1ns",
					"--default-item-operation-timeout=1ns",
					"--resource-timeout=1ns",
					"--default-repo-maintain-frequency=1ns",
					"--garbage-collection-frequency=1ns",
					"--fs-backup-timeout=1ns",
					"--item-operation-sync-frequency=1ns",
					defaultRestoreResourcePriorities,
					"--store-validation-frequency=1ns",
					"--terminating-resource-timeout=1ns",
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Velero burst and qps, Velero Deployment is built with burst and qps args",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							ClientBurst: ptr.To(123),
							ClientQPS:   ptr.To(123),
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							UploaderType: "kopia",
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					"--uploader-type=kopia",
					defaultFileSystemBackupTimeout,
					defaultRestoreResourcePriorities,
					"--client-burst=123",
					"--client-qps=123",
					defaultDisableInformerCache,
				},
			}),
		},
		{
			name: "valid DPA CR with Velero Args burst and qps, Velero Deployment is built with burst and qps args",
			dpa: createTestDpaWith(
				nil,
				oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							ClientBurst: ptr.To(123),
							ClientQPS:   ptr.To(123),
							Args: &oadpv1alpha1.VeleroServerArgs{
								ServerFlags: oadpv1alpha1.ServerFlags{
									ClientBurst: ptr.To(321),
									ClientQPS:   ptr.To("321"),
								},
							},
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							UploaderType: "kopia",
						},
					},
				},
			),
			veleroDeployment: testVeleroDeployment.DeepCopy(),
			wantVeleroDeployment: createTestBuiltVeleroDeployment(TestBuiltVeleroDeploymentOptions{
				args: []string{
					// should be present... "--uploader-type=kopia",
					"--client-burst=321",
					"--client-qps=321",
					"--fs-backup-timeout=4h0m0s",
					defaultRestoreResourcePriorities,
					defaultDisableInformerCache,
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
			r := DPAReconciler{Client: fakeClient, dpa: test.dpa}
			oadpclient.SetClient(fakeClient)
			if test.testProxy {
				t.Setenv(proxyEnvKey, proxyEnvValue)
			}
			if err := r.buildVeleroDeployment(test.veleroDeployment); err != nil {
				if test.errorMessage != err.Error() {
					t.Errorf("buildVeleroDeployment() error = %v, errorMessage %v", err, test.errorMessage)
				}
			} else {
				if !reflect.DeepEqual(test.wantVeleroDeployment, test.veleroDeployment) {
					t.Errorf("expected velero deployment diffs.\nDIFF:%v", cmp.Diff(test.wantVeleroDeployment, test.veleroDeployment))
				}
			}
		})
	}
}

func TestDPAReconciler_getVeleroImage(t *testing.T) {
	tests := []struct {
		name       string
		DpaCR      *oadpv1alpha1.DataProtectionApplication
		pluginName string
		wantImage  string
		setEnvVars map[string]string
	}{
		{
			name: "given Velero image override, custom Velero image should be returned",
			DpaCR: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testDpaName,
					Namespace: testNamespaceName,
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.VeleroImageKey: "test-image",
					},
				},
			},
			pluginName: common.Velero,
			wantImage:  "test-image",
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default DPA CR with no env var, default velero image should be returned",
			DpaCR: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testDpaName,
					Namespace: testNamespaceName,
				},
			},
			pluginName: common.Velero,
			wantImage:  common.VeleroImage,
			setEnvVars: make(map[string]string),
		},
		{
			name: "given default DPA CR with env var set, image should be built via env vars",
			DpaCR: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testDpaName,
					Namespace: testNamespaceName,
				},
			},
			pluginName: common.Velero,
			wantImage:  "quay.io/konveyor/velero:latest",
			setEnvVars: map[string]string{
				"REGISTRY":    "quay.io",
				"PROJECT":     "konveyor",
				"VELERO_REPO": "velero",
				"VELERO_TAG":  "latest",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.setEnvVars {
				t.Setenv(key, value)
			}
			gotImage := getVeleroImage(tt.DpaCR)
			if gotImage != tt.wantImage {
				t.Errorf("Expected plugin image %v did not match %v", tt.wantImage, gotImage)
			}
		})
	}
}
func Test_removeDuplicateValues(t *testing.T) {
	type args struct {
		slice []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "nill slice",
			args: args{slice: nil},
			want: nil,
		},
		{
			name: "empty slice",
			args: args{slice: []string{}},
			want: []string{},
		},
		{
			name: "one item in slice",
			args: args{slice: []string{"yo"}},
			want: []string{"yo"},
		},
		{
			name: "duplicate item in slice",
			args: args{slice: []string{"ice", "ice", "baby"}},
			want: []string{"ice", "baby"},
		},
		{
			name: "maintain order of first appearance in slice",
			args: args{slice: []string{"ice", "ice", "baby", "ice"}},
			want: []string{"ice", "baby"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := common.RemoveDuplicateValues(tt.args.slice); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("removeDuplicateValues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_validateVeleroPlugins(t *testing.T) {
	tests := []struct {
		name   string
		dpa    *oadpv1alpha1.DataProtectionApplication
		secret *corev1.Secret
	}{
		{
			name: "given valid Velero default plugin, default secret gets mounted as volume mounts",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testDpaName,
					Namespace: testNamespaceName,
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: testNamespaceName,
				},
			},
		},
		{
			name: "given valid Velero default plugin that is not a cloud provider, no secrets get mounted",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testDpaName,
					Namespace: testNamespaceName,
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginOpenShift,
							},
						},
					},
				},
			},
			secret: &corev1.Secret{},
		},
		{
			name: "given valid multiple Velero default plugins, default secrets gets mounted for each plugin if applicable",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testDpaName,
					Namespace: testNamespaceName,
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
								oadpv1alpha1.DefaultPluginOpenShift,
							},
						},
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloud-credentials",
					Namespace: testNamespaceName,
				},
			},
		},
		{
			name: "given aws default plugin without bsl, the valid plugin check passes",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testDpaName,
					Namespace: testNamespaceName,
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
							},
						},
					},
				},
			},
			secret: &corev1.Secret{},
		},
	}
	for _, tt := range tests {
		fakeClient, err := getFakeClientFromObjects(tt.dpa, tt.secret)
		if err != nil {
			t.Errorf("error in creating fake client, likely programmer error")
		}
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
		t.Run(tt.name, func(t *testing.T) {
			result, err := r.ValidateVeleroPlugins(r.Log)
			if err != nil {
				t.Errorf("ValidateVeleroPlugins() error = %v", err)
			}
			if !reflect.DeepEqual(result, true) {
				t.Errorf("ValidateVeleroPlugins() = %v, want %v", result, true)
			}
		})
	}
}

func TestDPAReconciler_noDefaultCredentials(t *testing.T) {
	type args struct {
		dpa oadpv1alpha1.DataProtectionApplication
	}
	tests := []struct {
		name                string
		args                args
		want                map[string]bool
		wantHasCloudStorage bool
		wantErr             bool
	}{
		{
			name: "dpa with all plugins but with noDefaultBackupLocation should not require default credentials",
			args: args{
				dpa: oadpv1alpha1.DataProtectionApplication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testDpaName,
						Namespace: testNamespaceName,
					},
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							Velero: &oadpv1alpha1.VeleroConfig{
								DefaultPlugins:          allDefaultPluginsList,
								NoDefaultBackupLocation: true,
							},
						},
					},
				},
			},
			want: map[string]bool{
				"aws":   false,
				"gcp":   false,
				"azure": false,
			},
			wantHasCloudStorage: false,
			wantErr:             false,
		},
		{
			name: "dpa no default cloudprovider plugins should not require default credentials",
			args: args{
				dpa: oadpv1alpha1.DataProtectionApplication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testDpaName,
						Namespace: testNamespaceName,
					},
					Spec: oadpv1alpha1.DataProtectionApplicationSpec{
						Configuration: &oadpv1alpha1.ApplicationConfig{
							Velero: &oadpv1alpha1.VeleroConfig{
								DefaultPlugins:          []oadpv1alpha1.DefaultPlugin{oadpv1alpha1.DefaultPluginOpenShift},
								NoDefaultBackupLocation: true,
							},
						},
					},
				},
			},
			want:                map[string]bool{},
			wantHasCloudStorage: false,
			wantErr:             false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(&tt.args.dpa)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := DPAReconciler{
				Client: fakeClient,
				dpa:    &tt.args.dpa,
			}
			got, got1, err := r.noDefaultCredentials()
			if (err != nil) != tt.wantErr {
				t.Errorf("DPAReconciler.noDefaultCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DPAReconciler.noDefaultCredentials() got = \n%v, \nwant \n%v", got, tt.want)
			}
			if got1 != tt.wantHasCloudStorage {
				t.Errorf("DPAReconciler.noDefaultCredentials() got1 = %v, want %v", got1, tt.wantHasCloudStorage)
			}
		})
	}
}

func TestDPAReconciler_VeleroDebugEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		replicas *int
		wantErr  bool
	}{
		{
			name:     "debug replica override not set",
			replicas: nil,
			wantErr:  false,
		},
		{
			name:     "debug replica override set to 1",
			replicas: ptr.To(1),
			wantErr:  false,
		},
		{
			name:     "debug replica override set to 0",
			replicas: ptr.To(0),
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		if tt.replicas != nil {
			t.Setenv(VeleroReplicaOverride, strconv.Itoa(*tt.replicas))
		}

		dpa := &oadpv1alpha1.DataProtectionApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testDpaName,
				Namespace: testNamespaceName,
			},
			Spec: oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						DefaultPlugins:          allDefaultPluginsList,
						NoDefaultBackupLocation: true,
					},
				},
			},
		}

		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      common.Velero,
				Namespace: dpa.Namespace,
			},
		}

		t.Run(tt.name, func(t *testing.T) {
			fakeClient, err := getFakeClientFromObjects(dpa)
			if err != nil {
				t.Errorf("error in creating fake client, likely programmer error")
			}
			r := DPAReconciler{
				Client: fakeClient,
				dpa:    dpa,
			}
			err = r.buildVeleroDeployment(deployment)
			if (err != nil) != tt.wantErr {
				t.Errorf("DPAReconciler.VeleroDebugEnvironment error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if deployment.Spec.Replicas == nil {
				t.Error("deployment replicas not set")
				return
			}
			if tt.replicas == nil {
				if *deployment.Spec.Replicas != 1 {
					t.Errorf("unexpected deployment replica count: %d", *deployment.Spec.Replicas)
					return
				}
			} else {
				if *deployment.Spec.Replicas != int32(*tt.replicas) {
					t.Error("debug replica override did not apply")
					return
				}
			}
		})
	}
}

package controller

import (
	"context"
	"flag"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
)

const (
	defaultKubevirtDatamoverImage = "quay.io/konveyor/kubevirt-datamover-controller:latest"
	// 90% of the default 1Gi memory limit, in bytes
	expectedDefaultKubevirtDatamoverGoMemLimit = "966367638"
)

type ReconcileKubevirtDatamoverControllerScenario struct {
	namespace                string
	dpa                      string
	errMessage               string
	eventWords               []string
	kubevirtDatamoverEnabled bool
	deployment               *appsv1.Deployment
	kubevirtDatamoverConfig  *oadpv1alpha1.KubevirtDatamoverConfig
	veleroLogLevel           string
	logFormat                oadpv1alpha1.LogFormat
	expectedArgs             []string
	expectedResources        *corev1.ResourceRequirements
	expectedGoMemLimit       string
}

func expectedDefaultKubevirtDatamoverResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
	}
}

// legacyKubevirtDatamoverContainer returns the manager container as rendered
// by operator versions that did not set probe timeouts, used 128Mi/500m
// resources and set the LOG_LEVEL and LOG_FORMAT env vars.
func legacyKubevirtDatamoverContainer() corev1.Container {
	legacyProbe := func(path string, initialDelaySeconds, periodSeconds int32) *corev1.Probe {
		return &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   path,
					Port:   intstr.FromInt(8081),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: initialDelaySeconds,
			PeriodSeconds:       periodSeconds,
			TimeoutSeconds:      1,
			SuccessThreshold:    1,
			FailureThreshold:    3,
		}
	}
	return corev1.Container{
		Name:    "manager",
		Image:   "old",
		Command: []string{"/manager"},
		Args:    []string{"--leader-elect", "--health-probe-bind-address=:8081", "--metrics-bind-address=:8443", "--metrics-secure=true"},
		Env: []corev1.EnvVar{
			{Name: "WATCH_NAMESPACE", Value: "test-namespace"},
			{Name: "DATAMOVER_IMAGE", Value: "old"},
			{Name: common.LogLevelEnvVar, Value: ""},
			{Name: common.LogFormatEnvVar, Value: "text"},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
		LivenessProbe:  legacyProbe("/healthz", 15, 20),
		ReadinessProbe: legacyProbe("/readyz", 5, 10),
	}
}

func kubevirtDatamoverEnvValue(container *corev1.Container, name string) (string, bool) {
	for _, env := range container.Env {
		if env.Name == name {
			return env.Value, true
		}
	}
	return "", false
}

// assertKubevirtDatamoverProbes checks both probes, including the fields the
// API server would otherwise default.
func assertKubevirtDatamoverProbes(t interface {
	Helper()
	Errorf(format string, args ...any)
}, container *corev1.Container) {
	t.Helper()
	expectedProbe := func(path string, initialDelaySeconds, periodSeconds int32) *corev1.Probe {
		return &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   path,
					Port:   intstr.FromInt(8081),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: initialDelaySeconds,
			PeriodSeconds:       periodSeconds,
			TimeoutSeconds:      5,
			SuccessThreshold:    1,
			FailureThreshold:    5,
		}
	}
	if expected := expectedProbe("/healthz", 15, 20); !equality.Semantic.DeepEqual(container.LivenessProbe, expected) {
		t.Errorf("liveness probe: expected %+v, got %+v", expected, container.LivenessProbe)
	}
	if expected := expectedProbe("/readyz", 5, 10); !equality.Semantic.DeepEqual(container.ReadinessProbe, expected) {
		t.Errorf("readiness probe: expected %+v, got %+v", expected, container.ReadinessProbe)
	}
}

func createTestKubevirtDatamoverDeployment(namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubevirtDatamoverObjectName,
			Namespace: namespace,
			Labels: map[string]string{
				"test":                           "test",
				"app.kubernetes.io/name":         "wrong",
				kubevirtDatamoverControlPlaneKey: "super-wrong",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(2)),
			Selector: &metav1.LabelSelector{
				MatchLabels: kubevirtDatamoverControlPlaneLabel,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: kubevirtDatamoverControlPlaneLabel,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "manager",
							Image: "wrong",
						},
					},
					ServiceAccountName: "wrong-one",
				},
			},
		},
	}
}

func runReconcileKubevirtDatamoverControllerTest(
	scenario ReconcileKubevirtDatamoverControllerScenario,
	updateTestScenario func(scenario ReconcileKubevirtDatamoverControllerScenario),
	ctx context.Context,
	imageEnvValue string,
) {
	updateTestScenario(scenario)

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: scenario.namespace,
		},
	}
	gomega.Expect(k8sClient.Create(ctx, namespace)).To(gomega.Succeed())

	dpa := &oadpv1alpha1.DataProtectionApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scenario.dpa,
			Namespace: scenario.namespace,
		},
		Spec: oadpv1alpha1.DataProtectionApplicationSpec{
			Configuration: &oadpv1alpha1.ApplicationConfig{
				Velero: &oadpv1alpha1.VeleroConfig{
					DefaultPlugins: []oadpv1alpha1.DefaultPlugin{},
				},
			},
		},
	}

	if scenario.kubevirtDatamoverEnabled {
		dpa.Spec.Configuration.Velero.DefaultPlugins = append(
			dpa.Spec.Configuration.Velero.DefaultPlugins,
			oadpv1alpha1.DefaultPluginKubeVirtDataMover,
		)
	}

	if scenario.kubevirtDatamoverConfig != nil {
		dpa.Spec.Configuration.KubevirtDatamover = scenario.kubevirtDatamoverConfig
	}
	dpa.Spec.Configuration.Velero.LogLevel = scenario.veleroLogLevel
	dpa.Spec.LogFormat = scenario.logFormat

	gomega.Expect(k8sClient.Create(ctx, dpa)).To(gomega.Succeed())

	if scenario.deployment != nil {
		gomega.Expect(k8sClient.Create(ctx, scenario.deployment)).To(gomega.Succeed())
	}

	os.Setenv("RELATED_IMAGE_KUBEVIRT_DATAMOVER_CONTROLLER", imageEnvValue)

	event := record.NewFakeRecorder(5)
	r := &DataProtectionApplicationReconciler{
		Client:  k8sClient,
		Scheme:  testEnv.Scheme,
		Context: ctx,
		NamespacedName: types.NamespacedName{
			Name:      scenario.dpa,
			Namespace: scenario.namespace,
		},
		EventRecorder: event,
		dpa:           dpa,
	}
	result, err := r.ReconcileKubevirtDatamoverController(logr.Discard())

	if len(scenario.errMessage) == 0 {
		gomega.Expect(result).To(gomega.BeTrue())
		gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
	} else {
		gomega.Expect(result).To(gomega.BeFalse())
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(err.Error()).To(gomega.ContainSubstring(scenario.errMessage))
	}

	if scenario.eventWords != nil {
		gomega.Expect(len(event.Events)).To(gomega.Equal(1))
		message := <-event.Events
		for _, word := range scenario.eventWords {
			gomega.Expect(message).To(gomega.ContainSubstring(word))
		}
	} else {
		gomega.Expect(len(event.Events)).To(gomega.Equal(0))
	}

	if scenario.kubevirtDatamoverConfig != nil && scenario.kubevirtDatamoverConfig.MaxConcurrentDataMovers != nil {
		deployment := &appsv1.Deployment{}
		gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      kubevirtDatamoverObjectName,
			Namespace: scenario.namespace,
		}, deployment)).To(gomega.Succeed())

		var container *corev1.Container
		for i := range deployment.Spec.Template.Spec.Containers {
			if deployment.Spec.Template.Spec.Containers[i].Name == "manager" {
				container = &deployment.Spec.Template.Spec.Containers[i]
				break
			}
		}
		gomega.Expect(container).ToNot(gomega.BeNil())
		gomega.Expect(container.Args).To(gomega.ContainElement(
			fmt.Sprintf("--max-concurrent-data-movers=%d", *scenario.kubevirtDatamoverConfig.MaxConcurrentDataMovers),
		))
	}

	if len(scenario.errMessage) != 0 || !scenario.kubevirtDatamoverEnabled {
		return
	}

	deployment := &appsv1.Deployment{}
	gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{
		Name:      kubevirtDatamoverObjectName,
		Namespace: scenario.namespace,
	}, deployment)).To(gomega.Succeed())
	gomega.Expect(deployment.Spec.Template.Spec.Containers).To(gomega.HaveLen(1))
	container := &deployment.Spec.Template.Spec.Containers[0]

	assertKubevirtDatamoverProbes(ginkgo.GinkgoT(), container)

	expectedResources := expectedDefaultKubevirtDatamoverResources()
	if scenario.expectedResources != nil {
		expectedResources = *scenario.expectedResources
	}
	gomega.Expect(equality.Semantic.DeepEqual(container.Resources, expectedResources)).To(gomega.BeTrue(),
		"resources: expected %v, got %v", expectedResources, container.Resources)

	expectedGoMemLimit := expectedDefaultKubevirtDatamoverGoMemLimit
	if scenario.expectedGoMemLimit != "" {
		expectedGoMemLimit = scenario.expectedGoMemLimit
	}
	gomega.Expect(container.Env).To(gomega.ContainElement(corev1.EnvVar{Name: "GOMEMLIMIT", Value: expectedGoMemLimit}))
	for _, env := range container.Env {
		gomega.Expect(env.Name).NotTo(gomega.BeElementOf(common.LogLevelEnvVar, common.LogFormatEnvVar))
	}

	// The CRD defaults logFormat to text, so --zap-encoder=console is set
	// even when the scenario leaves it empty.
	expectedArgs := scenario.expectedArgs
	if expectedArgs == nil {
		expectedArgs = []string{"--zap-log-level=info", "--zap-encoder=console"}
	}
	for _, arg := range expectedArgs {
		gomega.Expect(container.Args).To(gomega.ContainElement(arg))
	}

	// Reconciling again without changes must not update the deployment: the
	// probes and volumes are built with every field the API server defaults.
	secondEvent := record.NewFakeRecorder(5)
	r.EventRecorder = secondEvent
	result, err = r.ReconcileKubevirtDatamoverController(logr.Discard())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(result).To(gomega.BeTrue())
	gomega.Expect(secondEvent.Events).To(gomega.BeEmpty())

	reconciledDeployment := &appsv1.Deployment{}
	gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{
		Name:      kubevirtDatamoverObjectName,
		Namespace: scenario.namespace,
	}, reconciledDeployment)).To(gomega.Succeed())
	gomega.Expect(reconciledDeployment.ResourceVersion).To(gomega.Equal(deployment.ResourceVersion))
	gomega.Expect(reconciledDeployment.Generation).To(gomega.Equal(deployment.Generation))
}

var _ = ginkgo.Describe("Test ReconcileKubevirtDatamoverController function", func() {
	var (
		ctx                 = context.Background()
		currentTestScenario ReconcileKubevirtDatamoverControllerScenario
		updateTestScenario  = func(scenario ReconcileKubevirtDatamoverControllerScenario) {
			currentTestScenario = scenario
		}
	)

	ginkgo.AfterEach(func() {
		os.Unsetenv("RELATED_IMAGE_KUBEVIRT_DATAMOVER_CONTROLLER")

		deployment := &appsv1.Deployment{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      kubevirtDatamoverObjectName,
				Namespace: currentTestScenario.namespace,
			},
			deployment,
		) == nil {
			gomega.Expect(k8sClient.Delete(ctx, deployment)).To(gomega.Succeed())
		}

		dpa := &oadpv1alpha1.DataProtectionApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name:      currentTestScenario.dpa,
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

	ginkgo.DescribeTable("Reconcile is true",
		func(scenario ReconcileKubevirtDatamoverControllerScenario) {
			runReconcileKubevirtDatamoverControllerTest(
				scenario,
				updateTestScenario,
				ctx,
				defaultKubevirtDatamoverImage,
			)
		},
		ginkgo.Entry("Should create kubevirt-datamover deployment", ReconcileKubevirtDatamoverControllerScenario{
			namespace:                "kdm-test-1",
			dpa:                      "kdm-test-1-dpa",
			eventWords:               []string{"Normal", "KubevirtDatamoverDeploymentReconciled", "created"},
			kubevirtDatamoverEnabled: true,
		}),
		ginkgo.Entry("Should create kubevirt-datamover deployment with max-concurrent-data-movers configured", ReconcileKubevirtDatamoverControllerScenario{
			namespace:                "kdm-test-1b",
			dpa:                      "kdm-test-1b-dpa",
			eventWords:               []string{"Normal", "KubevirtDatamoverDeploymentReconciled", "created"},
			kubevirtDatamoverEnabled: true,
			kubevirtDatamoverConfig: &oadpv1alpha1.KubevirtDatamoverConfig{
				MaxConcurrentDataMovers: ptr.To(int32(3)),
			},
		}),
		ginkgo.Entry("Should update kubevirt-datamover deployment", ReconcileKubevirtDatamoverControllerScenario{
			namespace:                "kdm-test-2",
			dpa:                      "kdm-test-2-dpa",
			eventWords:               []string{"Normal", "KubevirtDatamoverDeploymentReconciled", "updated"},
			kubevirtDatamoverEnabled: true,
			deployment:               createTestKubevirtDatamoverDeployment("kdm-test-2"),
		}),
		ginkgo.Entry("Should update probes, resources, env and args of a legacy kubevirt-datamover deployment", ReconcileKubevirtDatamoverControllerScenario{
			namespace:                "kdm-test-2b",
			dpa:                      "kdm-test-2b-dpa",
			eventWords:               []string{"Normal", "KubevirtDatamoverDeploymentReconciled", "updated"},
			kubevirtDatamoverEnabled: true,
			deployment: func() *appsv1.Deployment {
				deployment := createTestKubevirtDatamoverDeployment("kdm-test-2b")
				deployment.Spec.Template.Spec.Containers = []corev1.Container{legacyKubevirtDatamoverContainer()}
				return deployment
			}(),
		}),
		ginkgo.Entry("Should create kubevirt-datamover deployment with resourceAllocations configured", ReconcileKubevirtDatamoverControllerScenario{
			namespace:                "kdm-test-2c",
			dpa:                      "kdm-test-2c-dpa",
			eventWords:               []string{"Normal", "KubevirtDatamoverDeploymentReconciled", "created"},
			kubevirtDatamoverEnabled: true,
			kubevirtDatamoverConfig: &oadpv1alpha1.KubevirtDatamoverConfig{
				PodConfig: &oadpv1alpha1.KubevirtDatamoverPodConfig{
					ResourceAllocations: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("0.5"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("2048Mi"),
						},
					},
				},
			},
			expectedResources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			expectedGoMemLimit: "1932735276",
		}),
		ginkgo.Entry("Should create kubevirt-datamover deployment with log level and format configured", ReconcileKubevirtDatamoverControllerScenario{
			namespace:                "kdm-test-2d",
			dpa:                      "kdm-test-2d-dpa",
			eventWords:               []string{"Normal", "KubevirtDatamoverDeploymentReconciled", "created"},
			kubevirtDatamoverEnabled: true,
			veleroLogLevel:           "warning",
			logFormat:                oadpv1alpha1.LogFormatJSON,
			expectedArgs:             []string{"--zap-log-level=info", "--zap-encoder=json"},
		}),
		ginkgo.Entry("Should delete kubevirt-datamover deployment", ReconcileKubevirtDatamoverControllerScenario{
			namespace:                "kdm-test-3",
			dpa:                      "kdm-test-3-dpa",
			eventWords:               []string{"Normal", "KubevirtDatamoverDeploymentDeleteSucceed", "deleted"},
			kubevirtDatamoverEnabled: false,
			deployment:               createTestKubevirtDatamoverDeployment("kdm-test-3"),
		}),
		ginkgo.Entry("Should do nothing when disabled", ReconcileKubevirtDatamoverControllerScenario{
			namespace:                "kdm-test-4",
			dpa:                      "kdm-test-4-dpa",
			kubevirtDatamoverEnabled: false,
		}),
	)

	ginkgo.DescribeTable("Reconcile is false",
		func(scenario ReconcileKubevirtDatamoverControllerScenario) {
			runReconcileKubevirtDatamoverControllerTest(
				scenario,
				updateTestScenario,
				ctx,
				defaultKubevirtDatamoverImage,
			)
		},
		ginkgo.Entry("Should error because manager container was not found in Deployment", ReconcileKubevirtDatamoverControllerScenario{
			namespace:                "kdm-test-error-1",
			dpa:                      "kdm-test-error-1-dpa",
			errMessage:               "could not find kubevirt-datamover container in Deployment",
			kubevirtDatamoverEnabled: true,
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubevirtDatamoverObjectName,
					Namespace: "kdm-test-error-1",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: kubevirtDatamoverControlPlaneLabel,
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: kubevirtDatamoverControlPlaneLabel,
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "wrong",
								Image: defaultKubevirtDatamoverImage,
							}},
						},
					},
				},
			},
		}),
		ginkgo.Entry("Should error because resourceAllocations request is above its limit", ReconcileKubevirtDatamoverControllerScenario{
			namespace:                "kdm-test-error-2",
			dpa:                      "kdm-test-error-2-dpa",
			errMessage:               "requests.memory (2Gi) must be less than or equal to limits.memory (1Gi)",
			kubevirtDatamoverEnabled: true,
			kubevirtDatamoverConfig: &oadpv1alpha1.KubevirtDatamoverConfig{
				PodConfig: &oadpv1alpha1.KubevirtDatamoverPodConfig{
					ResourceAllocations: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		}),
	)
})

func TestCheckKubevirtDatamoverEnabled(t *testing.T) {
	tests := []struct {
		name           string
		dpa            *oadpv1alpha1.DataProtectionApplication
		expectedResult bool
	}{
		{
			name: "Should return false when Configuration is nil",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: nil,
				},
			},
			expectedResult: false,
		},
		{
			name: "Should return false when Velero config is nil",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: nil,
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "Should return false when DefaultPlugins is nil",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: nil,
						},
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "Should return false when plugin not in list",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
								oadpv1alpha1.DefaultPluginKubeVirt,
							},
						},
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "Should return true when plugin in list",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
								oadpv1alpha1.DefaultPluginAWS,
								oadpv1alpha1.DefaultPluginKubeVirtDataMover,
							},
						},
					},
				},
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &DataProtectionApplicationReconciler{dpa: tt.dpa}
			result := r.checkKubevirtDatamoverEnabled()

			if result != tt.expectedResult {
				t.Errorf("expected %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

func TestGetKubevirtDatamoverControllerImage(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		overrideValue  string
		expectedResult string
	}{
		{
			name:           "Should return override value when set",
			envValue:       "env-image:v1",
			overrideValue:  "override-image:v1",
			expectedResult: "override-image:v1",
		},
		{
			name:           "Should return env value when override not set",
			envValue:       "env-image:v1",
			overrideValue:  "",
			expectedResult: "env-image:v1",
		},
		{
			name:           "Should return default when neither set",
			envValue:       "",
			overrideValue:  "",
			expectedResult: defaultKubevirtDatamoverImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("RELATED_IMAGE_KUBEVIRT_DATAMOVER_CONTROLLER", tt.envValue)
				defer os.Unsetenv("RELATED_IMAGE_KUBEVIRT_DATAMOVER_CONTROLLER")
			}

			dpa := &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{},
			}

			if tt.overrideValue != "" {
				dpa.Spec.UnsupportedOverrides = map[oadpv1alpha1.UnsupportedImageKey]string{
					oadpv1alpha1.KubeVirtDatamoverControllerImageKey: tt.overrideValue,
				}
			}

			r := &DataProtectionApplicationReconciler{dpa: dpa}
			result := r.getKubevirtDatamoverControllerImage()

			if result != tt.expectedResult {
				t.Errorf("expected %s, got %s", tt.expectedResult, result)
			}
		})
	}

	// Verify plugin image override does not affect controller image
	t.Run("Should not use plugin image override for controller", func(t *testing.T) {
		dpa := &oadpv1alpha1.DataProtectionApplication{
			Spec: oadpv1alpha1.DataProtectionApplicationSpec{
				UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
					oadpv1alpha1.KubeVirtDatamoverPluginImageKey: "quay.io/some-user/plugin-image:latest",
				},
			},
		}

		r := &DataProtectionApplicationReconciler{dpa: dpa}
		result := r.getKubevirtDatamoverControllerImage()

		if result != defaultKubevirtDatamoverImage {
			t.Errorf("plugin override should not affect controller image: expected %s, got %s", defaultKubevirtDatamoverImage, result)
		}
	})
}

func TestGetKubevirtDatamoverResourceReqs(t *testing.T) {
	dpaWithResourceAllocations := func(allocations corev1.ResourceRequirements) *oadpv1alpha1.DataProtectionApplication {
		return &oadpv1alpha1.DataProtectionApplication{
			Spec: oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					KubevirtDatamover: &oadpv1alpha1.KubevirtDatamoverConfig{
						PodConfig: &oadpv1alpha1.KubevirtDatamoverPodConfig{
							ResourceAllocations: allocations,
						},
					},
				},
			},
		}
	}
	defaultResources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
	}

	tests := []struct {
		name              string
		dpa               *oadpv1alpha1.DataProtectionApplication
		expectedResources corev1.ResourceRequirements
		errorContains     string
	}{
		{
			name:              "Should return defaults when configuration is nil",
			dpa:               &oadpv1alpha1.DataProtectionApplication{},
			expectedResources: defaultResources,
		},
		{
			name: "Should return defaults when kubevirtDatamover config is nil",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{},
				},
			},
			expectedResources: defaultResources,
		},
		{
			name: "Should return defaults when podConfig is nil",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						KubevirtDatamover: &oadpv1alpha1.KubevirtDatamoverConfig{
							MaxConcurrentDataMovers: ptr.To(int32(5)),
						},
					},
				},
			},
			expectedResources: defaultResources,
		},
		{
			name:              "Should return defaults when resourceAllocations is empty",
			dpa:               dpaWithResourceAllocations(corev1.ResourceRequirements{}),
			expectedResources: defaultResources,
		},
		{
			name: "Should keep the default memory request when only the cpu request is set",
			dpa: dpaWithResourceAllocations(corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m")},
			}),
			expectedResources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
			},
		},
		{
			name: "Should keep the default memory limit when only a cpu limit is set",
			dpa: dpaWithResourceAllocations(corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
			}),
			expectedResources: corev1.ResourceRequirements{
				Requests: defaultResources.Requests,
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
		},
		{
			name: "Should apply requests and limits set in the DPA",
			dpa: dpaWithResourceAllocations(corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("250m"),
					corev1.ResourceMemory:           resource.MustParse("512Mi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			}),
			expectedResources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("250m"),
					corev1.ResourceMemory:           resource.MustParse("512Mi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
		},
		{
			name: "Should raise the default memory limit to a memory request above it",
			dpa: dpaWithResourceAllocations(corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
			}),
			expectedResources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
			},
		},
		{
			name: "Should lower the default memory request to a memory limit below it",
			dpa: dpaWithResourceAllocations(corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")},
			}),
			expectedResources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")},
			},
		},
		{
			name: "Should lower the default cpu request to a cpu limit below it",
			dpa: dpaWithResourceAllocations(corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
			}),
			expectedResources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
		},
		{
			name: "Should ignore zero values",
			dpa: dpaWithResourceAllocations(corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("0")},
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("0")},
			}),
			expectedResources: defaultResources,
		},
		{
			name: "Should error when a request set in the DPA is above a limit set in the DPA",
			dpa: dpaWithResourceAllocations(corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
			}),
			errorContains: "spec.configuration.kubevirtDatamover.podConfig.resourceAllocations: requests.memory (2Gi) must be less than or equal to limits.memory (1Gi)",
		},
		{
			name: "Should error on a negative request",
			dpa: dpaWithResourceAllocations(corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("-100m")},
			}),
			errorContains: "spec.configuration.kubevirtDatamover.podConfig.resourceAllocations: requests.cpu must not be negative, got -100m",
		},
		{
			name: "Should error on a negative limit",
			dpa: dpaWithResourceAllocations(corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("-1Gi")},
			}),
			errorContains: "spec.configuration.kubevirtDatamover.podConfig.resourceAllocations: limits.memory must not be negative, got -1Gi",
		},
		{
			name: "Should error on an unsupported resource request",
			dpa: dpaWithResourceAllocations(corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory:                 resource.MustParse("512Mi"),
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
				},
			}),
			errorContains: "spec.configuration.kubevirtDatamover.podConfig.resourceAllocations: requests.nvidia.com/gpu is not supported; only cpu, memory and ephemeral-storage can be set",
		},
		{
			name: "Should error on an unsupported resource limit",
			dpa: dpaWithResourceAllocations(corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceName("hugepages-2Mi"): resource.MustParse("1Gi")},
			}),
			errorContains: "spec.configuration.kubevirtDatamover.podConfig.resourceAllocations: limits.hugepages-2Mi is not supported; only cpu, memory and ephemeral-storage can be set",
		},
		{
			name: "Should error on resource claims",
			dpa: dpaWithResourceAllocations(corev1.ResourceRequirements{
				Claims: []corev1.ResourceClaim{{Name: "gpu"}},
			}),
			errorContains: "spec.configuration.kubevirtDatamover.podConfig.resourceAllocations: claims is not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getKubevirtDatamoverResourceReqs(tt.dpa)

			if tt.errorContains != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got none", tt.errorContains)
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equality.Semantic.DeepEqual(result, tt.expectedResources) {
				t.Errorf("resources: expected %v, got %v", tt.expectedResources, result)
			}
		})
	}

	t.Run("Should not modify the package defaults", func(t *testing.T) {
		_, err := getKubevirtDatamoverResourceReqs(dpaWithResourceAllocations(corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result, err := getKubevirtDatamoverResourceReqs(&oadpv1alpha1.DataProtectionApplication{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !equality.Semantic.DeepEqual(result, defaultResources) {
			t.Errorf("defaults changed after an override: expected %v, got %v", defaultResources, result)
		}
	})
}

func TestKubevirtDatamoverGoMemLimit(t *testing.T) {
	tests := []struct {
		name          string
		limits        corev1.ResourceList
		expectedValue string
		expectedOK    bool
	}{
		{
			name:          "Should be 90% of a 1Gi limit",
			limits:        corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
			expectedValue: "966367638",
			expectedOK:    true,
		},
		{
			name:          "Should be 90% of a 512Mi limit",
			limits:        corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")},
			expectedValue: "483183819",
			expectedOK:    true,
		},
		{
			name:          "Should be 90% of a 2Gi limit",
			limits:        corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
			expectedValue: "1932735276",
			expectedOK:    true,
		},
		{
			name:          "Should handle a decimal limit",
			limits:        corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1G")},
			expectedValue: "900000000",
			expectedOK:    true,
		},
		{
			name:       "Should not be set without limits",
			limits:     nil,
			expectedOK: false,
		},
		{
			name:       "Should not be set with only a cpu limit",
			limits:     corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
			expectedOK: false,
		},
		{
			name:       "Should not be set when 90% of the limit rounds down to zero",
			limits:     corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1")},
			expectedOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := kubevirtDatamoverGoMemLimit(tt.limits)
			if ok != tt.expectedOK {
				t.Fatalf("ok: expected %v, got %v", tt.expectedOK, ok)
			}
			if value != tt.expectedValue {
				t.Errorf("value: expected %q, got %q", tt.expectedValue, value)
			}
		})
	}
}

func TestKubevirtDatamoverLogArgs(t *testing.T) {
	dpaWithLogSettings := func(logLevel string, logFormat oadpv1alpha1.LogFormat) *oadpv1alpha1.DataProtectionApplication {
		return &oadpv1alpha1.DataProtectionApplication{
			Spec: oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{LogLevel: logLevel},
				},
				LogFormat: logFormat,
			},
		}
	}

	tests := []struct {
		name         string
		dpa          *oadpv1alpha1.DataProtectionApplication
		expectedArgs []string
	}{
		{
			name:         "Should default to info and console without configuration",
			dpa:          &oadpv1alpha1.DataProtectionApplication{},
			expectedArgs: []string{"--zap-log-level=info", "--zap-encoder=console"},
		},
		{
			name:         "Should default to info when the log level is unset",
			dpa:          dpaWithLogSettings("", oadpv1alpha1.LogFormatText),
			expectedArgs: []string{"--zap-log-level=info", "--zap-encoder=console"},
		},
		{
			name:         "Should map trace to verbosity 2",
			dpa:          dpaWithLogSettings("trace", oadpv1alpha1.LogFormatText),
			expectedArgs: []string{"--zap-log-level=2", "--zap-encoder=console"},
		},
		{
			name:         "Should map debug to debug",
			dpa:          dpaWithLogSettings("debug", oadpv1alpha1.LogFormatJSON),
			expectedArgs: []string{"--zap-log-level=debug", "--zap-encoder=json"},
		},
		{
			name:         "Should map info to info",
			dpa:          dpaWithLogSettings("info", oadpv1alpha1.LogFormatJSON),
			expectedArgs: []string{"--zap-log-level=info", "--zap-encoder=json"},
		},
		{
			name:         "Should map warning to info",
			dpa:          dpaWithLogSettings("warning", oadpv1alpha1.LogFormatJSON),
			expectedArgs: []string{"--zap-log-level=info", "--zap-encoder=json"},
		},
		{
			name:         "Should map error to error",
			dpa:          dpaWithLogSettings("error", oadpv1alpha1.LogFormatText),
			expectedArgs: []string{"--zap-log-level=error", "--zap-encoder=console"},
		},
		{
			name:         "Should map fatal to error",
			dpa:          dpaWithLogSettings("fatal", oadpv1alpha1.LogFormatText),
			expectedArgs: []string{"--zap-log-level=error", "--zap-encoder=console"},
		},
		{
			name:         "Should map panic to error",
			dpa:          dpaWithLogSettings("panic", oadpv1alpha1.LogFormatText),
			expectedArgs: []string{"--zap-log-level=error", "--zap-encoder=console"},
		},
		{
			name:         "Should map an unknown log level to info",
			dpa:          dpaWithLogSettings("bogus", oadpv1alpha1.LogFormatText),
			expectedArgs: []string{"--zap-log-level=info", "--zap-encoder=console"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := kubevirtDatamoverLogArgs(tt.dpa)
			if !reflect.DeepEqual(args, tt.expectedArgs) {
				t.Fatalf("args: expected %v, got %v", tt.expectedArgs, args)
			}

			// The controller exits on startup if controller-runtime rejects a
			// flag value, so check the args parse with the same flag set.
			flagSet := flag.NewFlagSet("kubevirt-datamover-controller", flag.ContinueOnError)
			opts := crzap.Options{Development: true}
			opts.BindFlags(flagSet)
			if err := flagSet.Parse(args); err != nil {
				t.Errorf("controller-runtime rejected args %v: %v", args, err)
			}
		})
	}
}

func TestKubevirtDatamoverProbe(t *testing.T) {
	expected := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/healthz",
				Port:   intstr.FromInt(8081),
				Scheme: corev1.URISchemeHTTP,
			},
		},
		InitialDelaySeconds: 15,
		PeriodSeconds:       20,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
		FailureThreshold:    5,
	}
	if probe := kubevirtDatamoverProbe("/healthz", 15, 20); !reflect.DeepEqual(probe, expected) {
		t.Errorf("probe: expected %+v, got %+v", expected, probe)
	}
}

func TestEnsureKubevirtDatamoverRequiredLabels(t *testing.T) {
	tests := []struct {
		name           string
		existingLabels map[string]string
		expectedLabels map[string]string
	}{
		{
			name:           "Should set labels when deployment has no labels",
			existingLabels: nil,
			expectedLabels: map[string]string{
				"control-plane":                "oadp-kubevirt-datamover-controller",
				"app.kubernetes.io/component":  "manager",
				"app.kubernetes.io/created-by": "oadp-operator",
				"app.kubernetes.io/instance":   "oadp-kubevirt-datamover-controller-manager",
				"app.kubernetes.io/managed-by": "kustomize",
				"app.kubernetes.io/name":       "deployment",
				"app.kubernetes.io/part-of":    "oadp-operator",
			},
		},
		{
			name: "Should preserve existing labels and add required labels",
			existingLabels: map[string]string{
				"custom-label": "custom-value",
			},
			expectedLabels: map[string]string{
				"custom-label":                 "custom-value",
				"control-plane":                "oadp-kubevirt-datamover-controller",
				"app.kubernetes.io/component":  "manager",
				"app.kubernetes.io/created-by": "oadp-operator",
				"app.kubernetes.io/instance":   "oadp-kubevirt-datamover-controller-manager",
				"app.kubernetes.io/managed-by": "kustomize",
				"app.kubernetes.io/name":       "deployment",
				"app.kubernetes.io/part-of":    "oadp-operator",
			},
		},
		{
			name: "Should overwrite existing required labels with correct values",
			existingLabels: map[string]string{
				"control-plane":               "wrong-value",
				"app.kubernetes.io/component": "wrong-component",
			},
			expectedLabels: map[string]string{
				"control-plane":                "oadp-kubevirt-datamover-controller",
				"app.kubernetes.io/component":  "manager",
				"app.kubernetes.io/created-by": "oadp-operator",
				"app.kubernetes.io/instance":   "oadp-kubevirt-datamover-controller-manager",
				"app.kubernetes.io/managed-by": "kustomize",
				"app.kubernetes.io/name":       "deployment",
				"app.kubernetes.io/part-of":    "oadp-operator",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Labels: tt.existingLabels,
				},
			}

			ensureKubevirtDatamoverRequiredLabels(deployment)

			labels := deployment.GetLabels()
			for key, expectedValue := range tt.expectedLabels {
				if actualValue, exists := labels[key]; !exists {
					t.Errorf("expected label %s to exist", key)
				} else if actualValue != expectedValue {
					t.Errorf("label %s: expected %s, got %s", key, expectedValue, actualValue)
				}
			}

			// Verify no unexpected labels were added (beyond expected and existing custom ones)
			for key := range labels {
				if _, expected := tt.expectedLabels[key]; !expected {
					t.Errorf("unexpected label %s in deployment", key)
				}
			}
		})
	}
}

func TestEnsureKubevirtDatamoverRequiredSpecs(t *testing.T) {
	tests := []struct {
		name               string
		dpa                *oadpv1alpha1.DataProtectionApplication
		existingContainers []corev1.Container
		expectedEnvCount   int
		expectedArgs       []string
		expectedResources  *corev1.ResourceRequirements
		expectedGoMemLimit string
		expectError        bool
		errorContains      string
	}{
		{
			name: "Should create container when no containers exist",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			existingContainers: nil,
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectError:        false,
		},
		{
			name: "Should update existing manager container",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			existingContainers: []corev1.Container{{Name: "manager", Image: "old"}},
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectError:        false,
		},
		{
			name: "Should replace legacy probes, resources and env on existing manager container",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			existingContainers: []corev1.Container{legacyKubevirtDatamoverContainer()},
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectedArgs:       []string{"--zap-log-level=info", "--zap-encoder=console"},
			expectError:        false,
		},
		{
			name: "Should pass --zap-log-level when log level configured",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							LogLevel: "error",
						},
					},
				},
			},
			existingContainers: nil,
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectedArgs:       []string{"--zap-log-level=error", "--zap-encoder=console"},
			expectError:        false,
		},
		{
			name: "Should pass --zap-encoder=json when log format is json",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					LogFormat: oadpv1alpha1.LogFormatJSON,
				},
			},
			existingContainers: nil,
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectedArgs:       []string{"--zap-log-level=info", "--zap-encoder=json"},
			expectError:        false,
		},
		{
			name: "Should pass both log flags when log level and format configured",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							LogLevel: "debug",
						},
					},
					LogFormat: oadpv1alpha1.LogFormatText,
				},
			},
			existingContainers: nil,
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectedArgs:       []string{"--zap-log-level=debug", "--zap-encoder=console"},
			expectError:        false,
		},
		{
			name: "Should default log flags to info and console when not configured",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			existingContainers: nil,
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectedArgs:       []string{"--zap-log-level=info", "--zap-encoder=console"},
			expectError:        false,
		},
		{
			name: "Should apply resourceAllocations from the DPA and derive GOMEMLIMIT from the memory limit",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						KubevirtDatamover: &oadpv1alpha1.KubevirtDatamoverConfig{
							PodConfig: &oadpv1alpha1.KubevirtDatamoverPodConfig{
								ResourceAllocations: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("512Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
			},
			existingContainers: []corev1.Container{legacyKubevirtDatamoverContainer()},
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectedResources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			expectedGoMemLimit: "1932735276",
			expectError:        false,
		},
		{
			name: "Should error when resourceAllocations request is above its limit",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						KubevirtDatamover: &oadpv1alpha1.KubevirtDatamoverConfig{
							PodConfig: &oadpv1alpha1.KubevirtDatamoverPodConfig{
								ResourceAllocations: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
							},
						},
					},
				},
			},
			existingContainers: nil,
			expectError:        true,
			errorContains:      "requests.memory (2Gi) must be less than or equal to limits.memory (1Gi)",
		},
		{
			name: "Should set pod security context when not already set",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			existingContainers: nil,
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectError:        false,
		},
		{
			name: "Should include --max-incremental-backups arg when configured",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						KubevirtDatamover: &oadpv1alpha1.KubevirtDatamoverConfig{
							MaxIncrementalBackups: ptr.To(int32(5)),
						},
					},
				},
			},
			existingContainers: nil,
			expectedEnvCount:   3,
			expectError:        false,
		},
		{
			name: "Should not include --max-incremental-backups arg when not configured",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			existingContainers: nil,
			expectedEnvCount:   3,
			expectError:        false,
		},
		{
			name: "Should update --max-incremental-backups arg on existing container",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						KubevirtDatamover: &oadpv1alpha1.KubevirtDatamoverConfig{
							MaxIncrementalBackups: ptr.To(int32(10)),
						},
					},
				},
			},
			existingContainers: []corev1.Container{{
				Name:  "manager",
				Image: "old",
				Args:  []string{"--leader-elect", "--max-incremental-backups=2", "--old-arg"},
			}},
			expectedEnvCount: 3,
			expectError:      false,
		},
		{
			name: "Should include --max-concurrent-data-movers arg when configured",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						KubevirtDatamover: &oadpv1alpha1.KubevirtDatamoverConfig{
							MaxConcurrentDataMovers: ptr.To(int32(5)),
						},
					},
				},
			},
			existingContainers: nil,
			expectedEnvCount:   3,
			expectError:        false,
		},
		{
			name: "Should include --stale-dataupload-threshold arg when configured",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						KubevirtDatamover: &oadpv1alpha1.KubevirtDatamoverConfig{
							StaleDataUploadThreshold: &metav1.Duration{Duration: 1 * time.Hour},
						},
					},
				},
			},
			existingContainers: nil,
			expectedEnvCount:   3,
			expectError:        false,
		},
		{
			name: "Should not include --max-concurrent-data-movers arg when not configured",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			existingContainers: nil,
			expectedEnvCount:   3,
			expectError:        false,
		},
		{
			name: "Should not include --stale-dataupload-threshold arg when not configured",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			existingContainers: nil,
			expectedEnvCount:   3,
			expectError:        false,
		},
		{
			name: "Should update --max-concurrent-data-movers arg on existing container",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						KubevirtDatamover: &oadpv1alpha1.KubevirtDatamoverConfig{
							MaxConcurrentDataMovers: ptr.To(int32(10)),
						},
					},
				},
			},
			existingContainers: []corev1.Container{{
				Name:  "manager",
				Image: "old",
				Args:  []string{"--leader-elect", "--max-concurrent-data-movers=2", "--old-arg"},
			}},
			expectedEnvCount: 3,
			expectError:      false,
		},
		{
			name: "Should update --stale-dataupload-threshold arg on existing container",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
						KubevirtDatamover: &oadpv1alpha1.KubevirtDatamoverConfig{
							StaleDataUploadThreshold: &metav1.Duration{Duration: 3 * time.Hour},
						},
					},
				},
			},
			existingContainers: []corev1.Container{{
				Name:  "manager",
				Image: "old",
				Args:  []string{"--leader-elect", "--stale-dataupload-threshold=1h0m0s", "--old-arg"},
			}},
			expectedEnvCount: 3,
			expectError:      false,
		},
		{
			name: "Should error when manager container not found",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			existingContainers: []corev1.Container{{Name: "wrong", Image: "wrong"}},
			expectError:        true,
			errorContains:      "could not find kubevirt-datamover container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &DataProtectionApplicationReconciler{
				dpa: tt.dpa,
			}

			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubevirtDatamoverObjectName,
					Namespace: tt.dpa.Namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: tt.existingContainers,
						},
					},
				},
			}

			err := ensureKubevirtDatamoverRequiredSpecs(deployment, tt.dpa, defaultKubevirtDatamoverImage, corev1.PullAlways, r)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify replicas
			if *deployment.Spec.Replicas != 1 {
				t.Errorf("replicas: expected 1, got %d", *deployment.Spec.Replicas)
			}

			// Verify service account name
			if deployment.Spec.Template.Spec.ServiceAccountName != "velero" {
				t.Errorf("serviceAccountName: expected velero, got %s", deployment.Spec.Template.Spec.ServiceAccountName)
			}

			// Verify container
			if len(deployment.Spec.Template.Spec.Containers) == 0 {
				t.Fatalf("no containers found in deployment")
			}

			container := deployment.Spec.Template.Spec.Containers[0]
			if container.Name != "manager" {
				t.Errorf("container name: expected 'manager', got %s", container.Name)
			}

			// Verify environment variables
			if len(container.Env) != tt.expectedEnvCount {
				t.Errorf("env var count: expected %d, got %d", tt.expectedEnvCount, len(container.Env))
			}

			// Verify WATCH_NAMESPACE is always present
			hasWatchNamespace := false
			for _, env := range container.Env {
				if env.Name == "WATCH_NAMESPACE" {
					hasWatchNamespace = true
					if env.Value != deployment.Namespace {
						t.Errorf("WATCH_NAMESPACE: expected %s, got %s", deployment.Namespace, env.Value)
					}
					break
				}
			}
			if !hasWatchNamespace {
				t.Error("WATCH_NAMESPACE env var not found")
			}

			// Verify DATAMOVER_IMAGE is always present and matches the image
			hasDatamoverImage := false
			for _, env := range container.Env {
				if env.Name == "DATAMOVER_IMAGE" {
					hasDatamoverImage = true
					if env.Value != defaultKubevirtDatamoverImage {
						t.Errorf("DATAMOVER_IMAGE: expected %s, got %s", defaultKubevirtDatamoverImage, env.Value)
					}
					break
				}
			}
			if !hasDatamoverImage {
				t.Error("DATAMOVER_IMAGE env var not found")
			}

			// Verify --max-incremental-backups arg
			if tt.dpa.Spec.Configuration != nil && tt.dpa.Spec.Configuration.KubevirtDatamover != nil &&
				tt.dpa.Spec.Configuration.KubevirtDatamover.MaxIncrementalBackups != nil {
				expectedArg := fmt.Sprintf("--max-incremental-backups=%d",
					*tt.dpa.Spec.Configuration.KubevirtDatamover.MaxIncrementalBackups)
				hasArg := false
				maxArgCount := 0
				for _, arg := range container.Args {
					if strings.HasPrefix(arg, "--max-incremental-backups=") {
						maxArgCount++
					}
					if arg == expectedArg {
						hasArg = true
					}
				}
				if !hasArg {
					t.Errorf("expected arg %s in container args %v", expectedArg, container.Args)
				}
				if maxArgCount != 1 {
					t.Errorf("expected exactly one --max-incremental-backups arg, got %d in %v", maxArgCount, container.Args)
				}
			} else {
				for _, arg := range container.Args {
					if strings.Contains(arg, "--max-incremental-backups") {
						t.Errorf("unexpected --max-incremental-backups arg found: %s", arg)
					}
				}
			}

			// Verify --max-concurrent-data-movers arg
			if tt.dpa.Spec.Configuration != nil && tt.dpa.Spec.Configuration.KubevirtDatamover != nil &&
				tt.dpa.Spec.Configuration.KubevirtDatamover.MaxConcurrentDataMovers != nil {
				expectedArg := fmt.Sprintf("--max-concurrent-data-movers=%d",
					*tt.dpa.Spec.Configuration.KubevirtDatamover.MaxConcurrentDataMovers)
				hasArg := false
				maxConcurrentArgCount := 0
				for _, arg := range container.Args {
					if strings.HasPrefix(arg, "--max-concurrent-data-movers=") {
						maxConcurrentArgCount++
					}
					if arg == expectedArg {
						hasArg = true
					}
				}
				if !hasArg {
					t.Errorf("expected arg %s in container args %v", expectedArg, container.Args)
				}
				if maxConcurrentArgCount != 1 {
					t.Errorf("expected exactly one --max-concurrent-data-movers arg, got %d in %v", maxConcurrentArgCount, container.Args)
				}
			} else {
				for _, arg := range container.Args {
					if strings.Contains(arg, "--max-concurrent-data-movers") {
						t.Errorf("unexpected --max-concurrent-data-movers arg found: %s", arg)
					}
				}
			}

			// Verify --stale-dataupload-threshold arg
			if tt.dpa.Spec.Configuration != nil && tt.dpa.Spec.Configuration.KubevirtDatamover != nil &&
				tt.dpa.Spec.Configuration.KubevirtDatamover.StaleDataUploadThreshold != nil {
				expectedArg := fmt.Sprintf("--stale-dataupload-threshold=%s",
					tt.dpa.Spec.Configuration.KubevirtDatamover.StaleDataUploadThreshold.Duration.String())
				hasArg := false
				staleArgCount := 0
				for _, arg := range container.Args {
					if strings.HasPrefix(arg, "--stale-dataupload-threshold=") {
						staleArgCount++
					}
					if arg == expectedArg {
						hasArg = true
					}
				}
				if !hasArg {
					t.Errorf("expected arg %s in container args %v", expectedArg, container.Args)
				}
				if staleArgCount != 1 {
					t.Errorf("expected exactly one --stale-dataupload-threshold arg, got %d in %v", staleArgCount, container.Args)
				}
			} else {
				for _, arg := range container.Args {
					if strings.Contains(arg, "--stale-dataupload-threshold") {
						t.Errorf("unexpected --stale-dataupload-threshold arg found: %s", arg)
					}
				}
			}

			// Verify security contexts (only checked for new deployments)
			// Note: The function only sets security contexts when creating new containers,
			// not when updating existing ones (static fields are not changed)
			if len(tt.existingContainers) == 0 {
				if deployment.Spec.Template.Spec.SecurityContext == nil {
					t.Error("expected pod security context to be set")
				} else {
					if deployment.Spec.Template.Spec.SecurityContext.RunAsNonRoot == nil || !*deployment.Spec.Template.Spec.SecurityContext.RunAsNonRoot {
						t.Error("expected runAsNonRoot to be true")
					}
					if deployment.Spec.Template.Spec.SecurityContext.SeccompProfile == nil {
						t.Error("expected seccomp profile to be set")
					}
				}

				if container.SecurityContext == nil {
					t.Error("expected container security context to be set")
				} else {
					if container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
						t.Error("expected allowPrivilegeEscalation to be false")
					}

					if container.SecurityContext.ReadOnlyRootFilesystem == nil || !*container.SecurityContext.ReadOnlyRootFilesystem {
						t.Error("expected readOnlyRootFilesystem to be true")
					}
				}

				// Verify ports (only for new containers)
				if len(container.Ports) == 0 {
					t.Error("expected container ports to be set")
				} else if container.Ports[0].Name != "https" || container.Ports[0].ContainerPort != 8443 {
					t.Error("expected https port 8443")
				}
			}

			// Verify probes on both new and existing containers
			assertKubevirtDatamoverProbes(t, &container)

			// Verify resources and GOMEMLIMIT
			expectedResources := expectedDefaultKubevirtDatamoverResources()
			if tt.expectedResources != nil {
				expectedResources = *tt.expectedResources
			}
			if !equality.Semantic.DeepEqual(container.Resources, expectedResources) {
				t.Errorf("resources: expected %v, got %v", expectedResources, container.Resources)
			}
			expectedGoMemLimit := tt.expectedGoMemLimit
			if expectedGoMemLimit == "" {
				expectedGoMemLimit = expectedDefaultKubevirtDatamoverGoMemLimit
			}
			if value, found := kubevirtDatamoverEnvValue(&container, "GOMEMLIMIT"); !found || value != expectedGoMemLimit {
				t.Errorf("GOMEMLIMIT: expected %q, got %q (found %v)", expectedGoMemLimit, value, found)
			}

			// The controller does not read LOG_LEVEL or LOG_FORMAT
			for _, name := range []string{common.LogLevelEnvVar, common.LogFormatEnvVar} {
				if _, found := kubevirtDatamoverEnvValue(&container, name); found {
					t.Errorf("unexpected %s env var", name)
				}
			}

			// Verify log flags
			for _, expectedArg := range tt.expectedArgs {
				if !slices.Contains(container.Args, expectedArg) {
					t.Errorf("expected arg %s in container args %v", expectedArg, container.Args)
				}
			}

			// Verify /tmp emptyDir volume is present on pod spec
			volumes := deployment.Spec.Template.Spec.Volumes
			hasTmpVolume := false
			for _, v := range volumes {
				if v.Name == "tmp" && v.VolumeSource.EmptyDir != nil {
					hasTmpVolume = true
					break
				}
			}
			if !hasTmpVolume {
				t.Error("expected /tmp emptyDir volume on pod spec")
			}

			hasBoundSATokenVolume := false
			for _, v := range volumes {
				if v.Name == "bound-sa-token" && v.VolumeSource.Projected != nil {
					hasBoundSATokenVolume = true
					if v.VolumeSource.Projected.DefaultMode == nil || *v.VolumeSource.Projected.DefaultMode != common.DefaultProjectedPermission {
						t.Errorf("bound-sa-token defaultMode: expected %o, got %v", common.DefaultProjectedPermission, v.VolumeSource.Projected.DefaultMode)
					}
					break
				}
			}
			if !hasBoundSATokenVolume {
				t.Error("expected bound-sa-token projected volume on pod spec")
			}

			// Verify /tmp volume mount on container (only for new containers)
			if len(tt.existingContainers) == 0 {
				hasTmpMount := false
				for _, vm := range container.VolumeMounts {
					if vm.Name == "tmp" && vm.MountPath == "/tmp" {
						hasTmpMount = true
						break
					}
				}
				if !hasTmpMount {
					t.Error("expected /tmp volumeMount on container")
				}

				hasBoundSATokenMount := false
				for _, vm := range container.VolumeMounts {
					if vm.Name == "bound-sa-token" && vm.MountPath == "/var/run/secrets/openshift/serviceaccount" && vm.ReadOnly {
						hasBoundSATokenMount = true
						break
					}
				}
				if !hasBoundSATokenMount {
					t.Error("expected bound-sa-token volumeMount on container")
				}
			}

			// Verify template labels include control-plane
			if deployment.Spec.Template.Labels == nil {
				t.Error("expected template labels to be set")
			} else if deployment.Spec.Template.Labels[kubevirtDatamoverControlPlaneKey] != kubevirtDatamoverControlPlaneValue {
				t.Errorf("expected control-plane label to be %s", kubevirtDatamoverControlPlaneValue)
			}

			// Verify template annotations include DPA resource version
			if deployment.Spec.Template.Annotations == nil {
				t.Error("expected template annotations to be set")
			} else if _, exists := deployment.Spec.Template.Annotations[kdmDpaResourceVersionAnnotation]; !exists {
				t.Error("expected DPA resource version annotation")
			}
		})
	}
}

func TestBuildKubevirtDatamoverDeployment(t *testing.T) {
	tests := []struct {
		name             string
		dpa              *oadpv1alpha1.DataProtectionApplication
		envVars          map[string]string
		expectedImage    string
		expectedEnvCount int
		expectedArgs     []string
		expectedReplicas int32
		expectedSAName   string
		expectError      bool
		errorContains    string
	}{
		{
			name: "Should build deployment with default configuration",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			expectedImage:    defaultKubevirtDatamoverImage,
			expectedEnvCount: 3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectedReplicas: 1,
			expectedSAName:   "velero",
			expectError:      false,
		},
		{
			name: "Should use custom image via override",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.KubeVirtDatamoverControllerImageKey: "custom-registry.io/kdm:v1.0",
					},
				},
			},
			expectedImage:    "custom-registry.io/kdm:v1.0",
			expectedEnvCount: 3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectedReplicas: 1,
			expectedSAName:   "velero",
			expectError:      false,
		},
		{
			name: "Should use custom image via env var",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			envVars: map[string]string{
				"RELATED_IMAGE_KUBEVIRT_DATAMOVER_CONTROLLER": "env-registry.io/kdm:v2.0",
			},
			expectedImage:    "env-registry.io/kdm:v2.0",
			expectedEnvCount: 3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectedReplicas: 1,
			expectedSAName:   "velero",
			expectError:      false,
		},
		{
			name: "Should pass log level and format as zap flags",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							LogLevel: "debug",
						},
					},
					LogFormat: oadpv1alpha1.LogFormatJSON,
				},
			},
			expectedImage:    defaultKubevirtDatamoverImage,
			expectedEnvCount: 3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectedArgs:     []string{"--zap-log-level=debug", "--zap-encoder=json"},
			expectedReplicas: 1,
			expectedSAName:   "velero",
			expectError:      false,
		},
		{
			name: "Should apply resource labels and annotations correctly",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			expectedImage:    defaultKubevirtDatamoverImage,
			expectedEnvCount: 3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, GOMEMLIMIT
			expectedReplicas: 1,
			expectedSAName:   "velero",
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables if provided
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			r := &DataProtectionApplicationReconciler{
				dpa: tt.dpa,
			}

			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubevirtDatamoverObjectName,
					Namespace: tt.dpa.Namespace,
				},
			}

			err := r.buildKubevirtDatamoverDeployment(deployment)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify deployment spec
			if *deployment.Spec.Replicas != tt.expectedReplicas {
				t.Errorf("replicas: expected %d, got %d", tt.expectedReplicas, *deployment.Spec.Replicas)
			}

			if deployment.Spec.Template.Spec.ServiceAccountName != tt.expectedSAName {
				t.Errorf("serviceAccountName: expected %s, got %s", tt.expectedSAName, deployment.Spec.Template.Spec.ServiceAccountName)
			}

			// Verify container exists and has correct image
			if len(deployment.Spec.Template.Spec.Containers) == 0 {
				t.Fatalf("no containers found in deployment")
			}

			container := deployment.Spec.Template.Spec.Containers[0]
			if container.Name != "manager" {
				t.Errorf("container name: expected 'manager', got %s", container.Name)
			}

			if container.Image != tt.expectedImage {
				t.Errorf("container image: expected %s, got %s", tt.expectedImage, container.Image)
			}

			// Verify environment variables
			if len(container.Env) != tt.expectedEnvCount {
				t.Errorf("env var count: expected %d, got %d", tt.expectedEnvCount, len(container.Env))
			}

			// Verify DATAMOVER_IMAGE env var matches the container image
			hasDatamoverImage := false
			for _, env := range container.Env {
				if env.Name == "DATAMOVER_IMAGE" {
					hasDatamoverImage = true
					if env.Value != tt.expectedImage {
						t.Errorf("DATAMOVER_IMAGE: expected %s, got %s", tt.expectedImage, env.Value)
					}
					break
				}
			}
			if !hasDatamoverImage {
				t.Error("DATAMOVER_IMAGE env var not found")
			}

			// Verify security contexts
			if deployment.Spec.Template.Spec.SecurityContext.RunAsNonRoot == nil || !*deployment.Spec.Template.Spec.SecurityContext.RunAsNonRoot {
				t.Error("expected runAsNonRoot to be true")
			}

			if container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
				t.Error("expected allowPrivilegeEscalation to be false")
			}

			if container.SecurityContext.ReadOnlyRootFilesystem == nil || !*container.SecurityContext.ReadOnlyRootFilesystem {
				t.Error("expected readOnlyRootFilesystem to be true")
			}

			// Verify probes exist
			if container.LivenessProbe == nil {
				t.Error("expected liveness probe to be set")
			}

			if container.ReadinessProbe == nil {
				t.Error("expected readiness probe to be set")
			}

			// Verify /tmp emptyDir volume and mount
			hasTmpVolume := false
			for _, v := range deployment.Spec.Template.Spec.Volumes {
				if v.Name == "tmp" && v.VolumeSource.EmptyDir != nil {
					hasTmpVolume = true
					break
				}
			}
			if !hasTmpVolume {
				t.Error("expected /tmp emptyDir volume on pod spec")
			}

			hasTmpMount := false
			for _, vm := range container.VolumeMounts {
				if vm.Name == "tmp" && vm.MountPath == "/tmp" {
					hasTmpMount = true
					break
				}
			}
			if !hasTmpMount {
				t.Error("expected /tmp volumeMount on container")
			}

			hasBoundSATokenVolume := false
			for _, v := range deployment.Spec.Template.Spec.Volumes {
				if v.Name == "bound-sa-token" && v.VolumeSource.Projected != nil {
					hasBoundSATokenVolume = true
					break
				}
			}
			if !hasBoundSATokenVolume {
				t.Error("expected bound-sa-token projected volume on pod spec")
			}

			hasBoundSATokenMount := false
			for _, vm := range container.VolumeMounts {
				if vm.Name == "bound-sa-token" && vm.MountPath == "/var/run/secrets/openshift/serviceaccount" && vm.ReadOnly {
					hasBoundSATokenMount = true
					break
				}
			}
			if !hasBoundSATokenMount {
				t.Error("expected bound-sa-token volumeMount on container")
			}

			// Verify labels
			labels := deployment.GetLabels()
			if labels["control-plane"] != kubevirtDatamoverControlPlaneValue {
				t.Errorf("control-plane label: expected '%s', got %s", kubevirtDatamoverControlPlaneValue, labels["control-plane"])
			}

			// Verify pod annotations include DPA resource version
			podAnnotations := deployment.Spec.Template.GetAnnotations()
			if _, exists := podAnnotations[kdmDpaResourceVersionAnnotation]; !exists {
				t.Error("expected pod annotation with DPA resource version")
			}

			// Verify default resources are applied
			expectedResources := expectedDefaultKubevirtDatamoverResources()
			if !equality.Semantic.DeepEqual(container.Resources, expectedResources) {
				t.Errorf("resources: expected %v, got %v", expectedResources, container.Resources)
			}
			if _, hasCPULimit := container.Resources.Limits[corev1.ResourceCPU]; hasCPULimit {
				t.Errorf("expected no CPU limit, got %s", container.Resources.Limits.Cpu().String())
			}

			// Verify log flags
			for _, expectedArg := range tt.expectedArgs {
				if !slices.Contains(container.Args, expectedArg) {
					t.Errorf("expected arg %s in container args %v", expectedArg, container.Args)
				}
			}
		})
	}
}

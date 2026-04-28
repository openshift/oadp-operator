package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

const (
	defaultKubevirtDatamoverImage = "quay.io/konveyor/kubevirt-datamover-controller:latest"
)

type ReconcileKubevirtDatamoverControllerScenario struct {
	namespace                string
	dpa                      string
	errMessage               string
	eventWords               []string
	kubevirtDatamoverEnabled bool
	deployment               *appsv1.Deployment
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
		ginkgo.Entry("Should update kubevirt-datamover deployment", ReconcileKubevirtDatamoverControllerScenario{
			namespace:                "kdm-test-2",
			dpa:                      "kdm-test-2-dpa",
			eventWords:               []string{"Normal", "KubevirtDatamoverDeploymentReconciled", "updated"},
			kubevirtDatamoverEnabled: true,
			deployment:               createTestKubevirtDatamoverDeployment("kdm-test-2"),
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

func TestGetKubevirtDatamoverResources(t *testing.T) {
	tests := []struct {
		name               string
		expectedCPULimit   string
		expectedMemLimit   string
		expectedCPURequest string
		expectedMemRequest string
	}{
		{
			name:               "Should return default resources",
			expectedCPULimit:   "500m",
			expectedMemLimit:   "128Mi",
			expectedCPURequest: "10m",
			expectedMemRequest: "64Mi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dpa := &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{},
			}

			r := &DataProtectionApplicationReconciler{dpa: dpa}
			result := r.getKubevirtDatamoverResources()

			if result.Limits.Cpu().String() != tt.expectedCPULimit {
				t.Errorf("CPU limit: expected %s, got %s", tt.expectedCPULimit, result.Limits.Cpu().String())
			}
			if result.Limits.Memory().String() != tt.expectedMemLimit {
				t.Errorf("Memory limit: expected %s, got %s", tt.expectedMemLimit, result.Limits.Memory().String())
			}
			if result.Requests.Cpu().String() != tt.expectedCPURequest {
				t.Errorf("CPU request: expected %s, got %s", tt.expectedCPURequest, result.Requests.Cpu().String())
			}
			if result.Requests.Memory().String() != tt.expectedMemRequest {
				t.Errorf("Memory request: expected %s, got %s", tt.expectedMemRequest, result.Requests.Memory().String())
			}
		})
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
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, LOG_LEVEL (empty value)
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
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, LOG_LEVEL (empty value)
			expectError:        false,
		},
		{
			name: "Should include LOG_LEVEL env var when configured",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							LogLevel: "info",
						},
					},
				},
			},
			existingContainers: nil,
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, LOG_LEVEL with value
			expectError:        false,
		},
		{
			name: "Should include LOG_FORMAT env var when configured",
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
			expectedEnvCount:   4, // WATCH_NAMESPACE, DATAMOVER_IMAGE, LOG_LEVEL (empty), LOG_FORMAT
			expectError:        false,
		},
		{
			name: "Should include both LOG_LEVEL and LOG_FORMAT when configured",
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
			expectedEnvCount:   4, // WATCH_NAMESPACE, DATAMOVER_IMAGE, LOG_LEVEL, LOG_FORMAT
			expectError:        false,
		},
		{
			name: "Should include WATCH_NAMESPACE and empty LOG_LEVEL when log level not configured",
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
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, LOG_LEVEL (empty value)
			expectError:        false,
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
			expectedEnvCount:   3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, LOG_LEVEL (empty)
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
			if deployment.Spec.Template.Spec.ServiceAccountName != kubevirtDatamoverObjectName {
				t.Errorf("serviceAccountName: expected %s, got %s", kubevirtDatamoverObjectName, deployment.Spec.Template.Spec.ServiceAccountName)
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

				// Verify probes (only for new containers)
				if container.LivenessProbe == nil {
					t.Error("expected liveness probe to be set")
				} else if container.LivenessProbe.HTTPGet == nil || container.LivenessProbe.HTTPGet.Path != "/healthz" {
					t.Error("expected liveness probe path to be /healthz")
				}

				if container.ReadinessProbe == nil {
					t.Error("expected readiness probe to be set")
				} else if container.ReadinessProbe.HTTPGet == nil || container.ReadinessProbe.HTTPGet.Path != "/readyz" {
					t.Error("expected readiness probe path to be /readyz")
				}

				// Verify ports (only for new containers)
				if len(container.Ports) == 0 {
					t.Error("expected container ports to be set")
				} else if container.Ports[0].Name != "https" || container.Ports[0].ContainerPort != 8443 {
					t.Error("expected https port 8443")
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
			expectedEnvCount: 3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, LOG_LEVEL (empty)
			expectedReplicas: 1,
			expectedSAName:   kubevirtDatamoverObjectName,
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
			expectedEnvCount: 3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, LOG_LEVEL (empty)
			expectedReplicas: 1,
			expectedSAName:   kubevirtDatamoverObjectName,
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
			expectedEnvCount: 3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, LOG_LEVEL (empty)
			expectedReplicas: 1,
			expectedSAName:   kubevirtDatamoverObjectName,
			expectError:      false,
		},
		{
			name: "Should include log level and format env vars",
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
			expectedEnvCount: 4, // WATCH_NAMESPACE, DATAMOVER_IMAGE, LOG_LEVEL, LOG_FORMAT
			expectedReplicas: 1,
			expectedSAName:   kubevirtDatamoverObjectName,
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
			expectedEnvCount: 3, // WATCH_NAMESPACE, DATAMOVER_IMAGE, LOG_LEVEL (empty)
			expectedReplicas: 1,
			expectedSAName:   kubevirtDatamoverObjectName,
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
			expectedCPULimit := resource.MustParse("500m")
			expectedMemLimit := resource.MustParse("128Mi")
			if !container.Resources.Limits.Cpu().Equal(expectedCPULimit) {
				t.Errorf("CPU limit: expected %s, got %s", expectedCPULimit.String(), container.Resources.Limits.Cpu().String())
			}
			if !container.Resources.Limits.Memory().Equal(expectedMemLimit) {
				t.Errorf("Memory limit: expected %s, got %s", expectedMemLimit.String(), container.Resources.Limits.Memory().String())
			}
		})
	}
}

package controller

import (
	"context"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

const (
	defaultKubevirtAnnotationsRemoverImage = "quay.io/migtools/kubevirt-velero-annotations-remover-go:latest"
)

type ReconcileKubevirtAnnotationsRemoverScenario struct {
	namespace  string
	dpa        string
	errMessage string
	eventWords []string
	enabled    bool
	deployment *appsv1.Deployment
}

func createTestKubevirtAnnotationsRemoverDeployment(namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubevirtAnnotationsRemoverName,
			Namespace: namespace,
			Labels: map[string]string{
				"test": "test",
				"app":  "wrong",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(2)),
			Selector: &metav1.LabelSelector{
				MatchLabels: kubevirtAnnotationsRemoverAppLabel,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: kubevirtAnnotationsRemoverAppLabel,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "webhook",
							Image: "wrong",
						},
					},
					ServiceAccountName: "wrong-one",
				},
			},
		},
	}
}

func runReconcileKubevirtAnnotationsRemoverTest(
	scenario ReconcileKubevirtAnnotationsRemoverScenario,
	updateTestScenario func(scenario ReconcileKubevirtAnnotationsRemoverScenario),
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
				Velero: &oadpv1alpha1.VeleroConfig{},
			},
			KubevirtAnnotationsRemover: &oadpv1alpha1.KubevirtAnnotationsRemover{
				Enable: ptr.To(scenario.enabled),
			},
		},
	}
	gomega.Expect(k8sClient.Create(ctx, dpa)).To(gomega.Succeed())

	if scenario.deployment != nil {
		gomega.Expect(k8sClient.Create(ctx, scenario.deployment)).To(gomega.Succeed())
	}

	os.Setenv("RELATED_IMAGE_KUBEVIRT_ANNOTATIONS_REMOVER", imageEnvValue)

	event := record.NewFakeRecorder(10)
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
	result, err := r.ReconcileKubevirtAnnotationsRemover(logr.Discard())

	if len(scenario.errMessage) == 0 {
		gomega.Expect(result).To(gomega.BeTrue())
		gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
	} else {
		gomega.Expect(result).To(gomega.BeFalse())
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(err.Error()).To(gomega.ContainSubstring(scenario.errMessage))
	}

	if scenario.eventWords != nil {
		gomega.Expect(len(event.Events)).To(gomega.BeNumerically(">=", 1))
		message := <-event.Events
		for _, word := range scenario.eventWords {
			gomega.Expect(message).To(gomega.ContainSubstring(word))
		}
	}
}

var _ = ginkgo.Describe("Test ReconcileKubevirtAnnotationsRemover function", func() {
	var (
		ctx                 = context.Background()
		currentTestScenario ReconcileKubevirtAnnotationsRemoverScenario
		updateTestScenario  = func(scenario ReconcileKubevirtAnnotationsRemoverScenario) {
			currentTestScenario = scenario
		}
	)

	ginkgo.AfterEach(func() {
		os.Unsetenv("RELATED_IMAGE_KUBEVIRT_ANNOTATIONS_REMOVER")

		deployment := &appsv1.Deployment{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      kubevirtAnnotationsRemoverName,
				Namespace: currentTestScenario.namespace,
			},
			deployment,
		) == nil {
			gomega.Expect(k8sClient.Delete(ctx, deployment)).To(gomega.Succeed())
		}

		svc := &corev1.Service{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      kubevirtAnnotationsRemoverName,
				Namespace: currentTestScenario.namespace,
			},
			svc,
		) == nil {
			gomega.Expect(k8sClient.Delete(ctx, svc)).To(gomega.Succeed())
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
		func(scenario ReconcileKubevirtAnnotationsRemoverScenario) {
			runReconcileKubevirtAnnotationsRemoverTest(
				scenario,
				updateTestScenario,
				ctx,
				defaultKubevirtAnnotationsRemoverImage,
			)
		},
		ginkgo.Entry("Should create kubevirt annotations remover resources", ReconcileKubevirtAnnotationsRemoverScenario{
			namespace:  "kar-test-1",
			dpa:        "kar-test-1-dpa",
			eventWords: []string{"Normal", "KubevirtAnnotationsRemoverServiceReconciled", "created"},
			enabled:    true,
		}),
		ginkgo.Entry("Should update kubevirt annotations remover deployment", ReconcileKubevirtAnnotationsRemoverScenario{
			namespace:  "kar-test-2",
			dpa:        "kar-test-2-dpa",
			eventWords: []string{"Normal", "KubevirtAnnotationsRemoverServiceReconciled"},
			enabled:    true,
			deployment: createTestKubevirtAnnotationsRemoverDeployment("kar-test-2"),
		}),
		ginkgo.Entry("Should do nothing when disabled and no resources exist", ReconcileKubevirtAnnotationsRemoverScenario{
			namespace: "kar-test-3",
			dpa:       "kar-test-3-dpa",
			enabled:   false,
		}),
		ginkgo.Entry("Should delete resources when disabled", ReconcileKubevirtAnnotationsRemoverScenario{
			namespace:  "kar-test-4",
			dpa:        "kar-test-4-dpa",
			eventWords: []string{"Normal", "KubevirtAnnotationsRemoverDeploymentDeleteSucceed"},
			enabled:    false,
			deployment: createTestKubevirtAnnotationsRemoverDeployment("kar-test-4"),
		}),
	)

	ginkgo.DescribeTable("Reconcile is false",
		func(scenario ReconcileKubevirtAnnotationsRemoverScenario) {
			runReconcileKubevirtAnnotationsRemoverTest(
				scenario,
				updateTestScenario,
				ctx,
				defaultKubevirtAnnotationsRemoverImage,
			)
		},
		ginkgo.Entry("Should error because webhook container was not found in Deployment", ReconcileKubevirtAnnotationsRemoverScenario{
			namespace:  "kar-test-error-1",
			dpa:        "kar-test-error-1-dpa",
			errMessage: "could not find webhook container in kubevirt-velero-annotations-remover Deployment",
			enabled:    true,
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubevirtAnnotationsRemoverName,
					Namespace: "kar-test-error-1",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: kubevirtAnnotationsRemoverAppLabel,
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: kubevirtAnnotationsRemoverAppLabel,
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "wrong-name",
								Image: defaultKubevirtAnnotationsRemoverImage,
							}},
						},
					},
				},
			},
		}),
	)
})

func TestCheckKubevirtAnnotationsRemoverEnabled(t *testing.T) {
	tests := []struct {
		name           string
		config         *oadpv1alpha1.KubevirtAnnotationsRemover
		expectedResult bool
	}{
		{
			name:           "Should return false when KubevirtAnnotationsRemover is nil",
			config:         nil,
			expectedResult: false,
		},
		{
			name: "Should return false when Enable is nil",
			config: &oadpv1alpha1.KubevirtAnnotationsRemover{
				Enable: nil,
			},
			expectedResult: false,
		},
		{
			name: "Should return false when Enable is false",
			config: &oadpv1alpha1.KubevirtAnnotationsRemover{
				Enable: ptr.To(false),
			},
			expectedResult: false,
		},
		{
			name: "Should return true when Enable is true",
			config: &oadpv1alpha1.KubevirtAnnotationsRemover{
				Enable: ptr.To(true),
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dpa := &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					KubevirtAnnotationsRemover: tt.config,
				},
			}

			r := &DataProtectionApplicationReconciler{dpa: dpa}
			result := r.checkKubevirtAnnotationsRemoverEnabled()

			if result != tt.expectedResult {
				t.Errorf("expected %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

func TestGetKubevirtAnnotationsRemoverImage(t *testing.T) {
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
			expectedResult: defaultKubevirtAnnotationsRemoverImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("RELATED_IMAGE_KUBEVIRT_ANNOTATIONS_REMOVER", tt.envValue)
				defer os.Unsetenv("RELATED_IMAGE_KUBEVIRT_ANNOTATIONS_REMOVER")
			} else {
				os.Unsetenv("RELATED_IMAGE_KUBEVIRT_ANNOTATIONS_REMOVER")
			}

			dpa := &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{},
			}

			if tt.overrideValue != "" {
				dpa.Spec.UnsupportedOverrides = map[oadpv1alpha1.UnsupportedImageKey]string{
					oadpv1alpha1.KubevirtAnnotationsRemoverImageKey: tt.overrideValue,
				}
			}

			r := &DataProtectionApplicationReconciler{dpa: dpa}
			result := r.getKubevirtAnnotationsRemoverImage()

			if result != tt.expectedResult {
				t.Errorf("expected %s, got %s", tt.expectedResult, result)
			}
		})
	}
}

func TestBuildKubevirtAnnotationsRemoverDeployment(t *testing.T) {
	tests := []struct {
		name             string
		dpa              *oadpv1alpha1.DataProtectionApplication
		expectedImage    string
		expectedReplicas int32
		expectedSAName   string
		expectError      bool
		errorContains    string
	}{
		{
			name: "Should build deployment with default image",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					KubevirtAnnotationsRemover: &oadpv1alpha1.KubevirtAnnotationsRemover{
						Enable: ptr.To(true),
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			expectedImage:    defaultKubevirtAnnotationsRemoverImage,
			expectedReplicas: 1,
			expectedSAName:   "velero",
			expectError:      false,
		},
		{
			name: "Should use override image from unsupportedOverrides",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					KubevirtAnnotationsRemover: &oadpv1alpha1.KubevirtAnnotationsRemover{
						Enable: ptr.To(true),
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.KubevirtAnnotationsRemoverImageKey: "custom-registry.io/kar:v1.0",
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			expectedImage:    "custom-registry.io/kar:v1.0",
			expectedReplicas: 1,
			expectedSAName:   "velero",
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("RELATED_IMAGE_KUBEVIRT_ANNOTATIONS_REMOVER")

			r := &DataProtectionApplicationReconciler{
				dpa: tt.dpa,
			}

			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubevirtAnnotationsRemoverName,
					Namespace: tt.dpa.Namespace,
				},
			}

			err := r.buildKubevirtAnnotationsRemoverDeployment(deployment)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorContains != "" && !containsStr(err.Error(), tt.errorContains) {
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
			if container.Name != "webhook" {
				t.Errorf("container name: expected 'webhook', got %s", container.Name)
			}

			if container.Image != tt.expectedImage {
				t.Errorf("container image: expected %s, got %s", tt.expectedImage, container.Image)
			}

			// Verify TLS volume mount
			foundTLSMount := false
			for _, vm := range container.VolumeMounts {
				if vm.Name == "tls" && vm.MountPath == "/tls" && vm.ReadOnly {
					foundTLSMount = true
					break
				}
			}
			if !foundTLSMount {
				t.Error("expected TLS volume mount at /tls")
			}

			// Verify TLS volume
			foundTLSVolume := false
			for _, v := range deployment.Spec.Template.Spec.Volumes {
				if v.Name == "tls" && v.Secret != nil && v.Secret.SecretName == kubevirtAnnotationsRemoverTLSSecret {
					foundTLSVolume = true
					break
				}
			}
			if !foundTLSVolume {
				t.Error("expected TLS volume with secret")
			}

			// Verify security context
			if deployment.Spec.Template.Spec.SecurityContext.RunAsNonRoot == nil || !*deployment.Spec.Template.Spec.SecurityContext.RunAsNonRoot {
				t.Error("expected runAsNonRoot to be true")
			}

			if container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
				t.Error("expected allowPrivilegeEscalation to be false")
			}

			// Verify labels
			labels := deployment.GetLabels()
			if labels["app"] != kubevirtAnnotationsRemoverName {
				t.Errorf("app label: expected '%s', got %s", kubevirtAnnotationsRemoverName, labels["app"])
			}

			// Verify port
			if len(container.Ports) == 0 || container.Ports[0].ContainerPort != kubevirtAnnotationsRemoverWebhookPort {
				t.Errorf("expected container port %d", kubevirtAnnotationsRemoverWebhookPort)
			}
		})
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

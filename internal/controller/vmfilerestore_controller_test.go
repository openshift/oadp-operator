package controller

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
)

const (
	defaultVMFileRestoreControllerImage = "quay.io/konveyor/oadp-vm-file-restore:latest"
	defaultVMFileRestoreAccessImage     = "quay.io/konveyor/oadp-vmfr-access:latest"
	defaultVMFileRestoreSSHImage        = "quay.io/konveyor/oadp-vmfr-access-sshd:latest"
	defaultVMFileRestoreBrowserImage    = "quay.io/konveyor/oadp-vmfr-access-filebrowser:latest"
)

type ReconcileVMFileRestoreControllerScenario struct {
	namespace            string
	dpa                  string
	errMessage           string
	eventWords           []string
	vmFileRestoreEnabled bool
	deployment           *appsv1.Deployment
}

func createTestVMFileRestoreDeployment(namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmFileRestoreObjectName,
			Namespace: namespace,
			Labels: map[string]string{
				"test":                       "test",
				"app.kubernetes.io/name":     "wrong",
				vmFileRestoreControlPlaneKey: "super-wrong",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(2)),
			Selector: &metav1.LabelSelector{
				MatchLabels: vmFileRestoreControlPlaneLabel,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: vmFileRestoreControlPlaneLabel,
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

func runReconcileVMFileRestoreControllerTest(
	scenario ReconcileVMFileRestoreControllerScenario,
	updateTestScenario func(scenario ReconcileVMFileRestoreControllerScenario),
	ctx context.Context,
	controllerImageEnvValue string,
	accessImageEnvValue string,
	sshImageEnvValue string,
	browserImageEnvValue string,
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
			VMFileRestore: &oadpv1alpha1.VMFileRestore{
				Enable: ptr.To(scenario.vmFileRestoreEnabled),
			},
		},
	}
	gomega.Expect(k8sClient.Create(ctx, dpa)).To(gomega.Succeed())

	if scenario.deployment != nil {
		gomega.Expect(k8sClient.Create(ctx, scenario.deployment)).To(gomega.Succeed())
	}

	os.Setenv("RELATED_IMAGE_VM_FILE_RESTORE_CONTROLLER", controllerImageEnvValue)
	os.Setenv("RELATED_IMAGE_VM_FILE_RESTORE_ACCESS", accessImageEnvValue)
	os.Setenv("RELATED_IMAGE_VM_FILE_RESTORE_SSH", sshImageEnvValue)
	os.Setenv("RELATED_IMAGE_VM_FILE_RESTORE_BROWSER", browserImageEnvValue)

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
	result, err := r.ReconcileVMFileRestoreController(logr.Discard())

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

var _ = ginkgo.Describe("Test ReconcileVMFileRestoreController function", func() {
	var (
		ctx                 = context.Background()
		currentTestScenario ReconcileVMFileRestoreControllerScenario
		updateTestScenario  = func(scenario ReconcileVMFileRestoreControllerScenario) {
			currentTestScenario = scenario
		}
	)

	ginkgo.AfterEach(func() {
		os.Unsetenv("RELATED_IMAGE_VM_FILE_RESTORE_CONTROLLER")
		os.Unsetenv("RELATED_IMAGE_VM_FILE_RESTORE_ACCESS")
		os.Unsetenv("RELATED_IMAGE_VM_FILE_RESTORE_SSH")
		os.Unsetenv("RELATED_IMAGE_VM_FILE_RESTORE_BROWSER")

		deployment := &appsv1.Deployment{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      vmFileRestoreObjectName,
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
		func(scenario ReconcileVMFileRestoreControllerScenario) {
			runReconcileVMFileRestoreControllerTest(
				scenario,
				updateTestScenario,
				ctx,
				defaultVMFileRestoreControllerImage,
				defaultVMFileRestoreAccessImage,
				defaultVMFileRestoreSSHImage,
				defaultVMFileRestoreBrowserImage,
			)
		},
		ginkgo.Entry("Should create VM file restore deployment", ReconcileVMFileRestoreControllerScenario{
			namespace:            "vmfr-test-1",
			dpa:                  "vmfr-test-1-dpa",
			eventWords:           []string{"Normal", "VMFileRestoreDeploymentReconciled", "created"},
			vmFileRestoreEnabled: true,
		}),
		ginkgo.Entry("Should update VM file restore deployment", ReconcileVMFileRestoreControllerScenario{
			namespace:            "vmfr-test-2",
			dpa:                  "vmfr-test-2-dpa",
			eventWords:           []string{"Normal", "VMFileRestoreDeploymentReconciled", "updated"},
			vmFileRestoreEnabled: true,
			deployment:           createTestVMFileRestoreDeployment("vmfr-test-2"),
		}),
		ginkgo.Entry("Should delete VM file restore deployment", ReconcileVMFileRestoreControllerScenario{
			namespace:            "vmfr-test-3",
			dpa:                  "vmfr-test-3-dpa",
			eventWords:           []string{"Normal", "VMFileRestoreDeploymentDeleteSucceed", "deleted"},
			vmFileRestoreEnabled: false,
			deployment:           createTestVMFileRestoreDeployment("vmfr-test-3"),
		}),
		ginkgo.Entry("Should do nothing when disabled", ReconcileVMFileRestoreControllerScenario{
			namespace:            "vmfr-test-4",
			dpa:                  "vmfr-test-4-dpa",
			vmFileRestoreEnabled: false,
		}),
	)

	ginkgo.DescribeTable("Reconcile is false",
		func(scenario ReconcileVMFileRestoreControllerScenario) {
			runReconcileVMFileRestoreControllerTest(
				scenario,
				updateTestScenario,
				ctx,
				defaultVMFileRestoreControllerImage,
				defaultVMFileRestoreAccessImage,
				defaultVMFileRestoreSSHImage,
				defaultVMFileRestoreBrowserImage,
			)
		},
		ginkgo.Entry("Should error because manager container was not found in Deployment", ReconcileVMFileRestoreControllerScenario{
			namespace:            "vmfr-test-error-1",
			dpa:                  "vmfr-test-error-1-dpa",
			errMessage:           "could not find VM file restore container in Deployment",
			vmFileRestoreEnabled: true,
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vmFileRestoreObjectName,
					Namespace: "vmfr-test-error-1",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: vmFileRestoreControlPlaneLabel,
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: vmFileRestoreControlPlaneLabel,
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "wrong",
								Image: defaultVMFileRestoreControllerImage,
							}},
						},
					},
				},
			},
		}),
	)
})

func TestGetVMFileRestoreControllerImage(t *testing.T) {
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
			expectedResult: defaultVMFileRestoreControllerImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("RELATED_IMAGE_VM_FILE_RESTORE_CONTROLLER", tt.envValue)
				defer os.Unsetenv("RELATED_IMAGE_VM_FILE_RESTORE_CONTROLLER")
			}

			dpa := &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{},
			}

			if tt.overrideValue != "" {
				dpa.Spec.UnsupportedOverrides = map[oadpv1alpha1.UnsupportedImageKey]string{
					oadpv1alpha1.VMFileRestoreControllerImageKey: tt.overrideValue,
				}
			}

			r := &DataProtectionApplicationReconciler{dpa: dpa}
			result := r.getVMFileRestoreControllerImage()

			if result != tt.expectedResult {
				t.Errorf("expected %s, got %s", tt.expectedResult, result)
			}
		})
	}
}

func TestGetVMFileRestoreAccessImage(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		overrideValue  string
		expectedResult string
	}{
		{
			name:           "Should return override value when set",
			envValue:       "env-access-image:v1",
			overrideValue:  "override-access-image:v1",
			expectedResult: "override-access-image:v1",
		},
		{
			name:           "Should return env value when override not set",
			envValue:       "env-access-image:v1",
			overrideValue:  "",
			expectedResult: "env-access-image:v1",
		},
		{
			name:           "Should return default when neither set",
			envValue:       "",
			overrideValue:  "",
			expectedResult: defaultVMFileRestoreAccessImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("RELATED_IMAGE_VM_FILE_RESTORE_ACCESS", tt.envValue)
				defer os.Unsetenv("RELATED_IMAGE_VM_FILE_RESTORE_ACCESS")
			}

			dpa := &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{},
			}

			if tt.overrideValue != "" {
				dpa.Spec.UnsupportedOverrides = map[oadpv1alpha1.UnsupportedImageKey]string{
					oadpv1alpha1.VMFileRestoreAccessImageKey: tt.overrideValue,
				}
			}

			r := &DataProtectionApplicationReconciler{dpa: dpa}
			result := r.getVMFileRestoreAccessImage()

			if result != tt.expectedResult {
				t.Errorf("expected %s, got %s", tt.expectedResult, result)
			}
		})
	}
}

func TestGetVMFileRestoreResources(t *testing.T) {
	tests := []struct {
		name               string
		customResources    *corev1.ResourceRequirements
		expectedCPULimit   string
		expectedMemLimit   string
		expectedCPURequest string
		expectedMemRequest string
	}{
		{
			name:               "Should return default resources when not specified",
			customResources:    nil,
			expectedCPULimit:   "500m",
			expectedMemLimit:   "128Mi",
			expectedCPURequest: "10m",
			expectedMemRequest: "64Mi",
		},
		{
			name: "Should return custom resources when specified",
			customResources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
			expectedCPULimit:   "1",
			expectedMemLimit:   "256Mi",
			expectedCPURequest: "100m",
			expectedMemRequest: "128Mi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dpa := &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					VMFileRestore: &oadpv1alpha1.VMFileRestore{},
				},
			}

			if tt.customResources != nil {
				dpa.Spec.VMFileRestore.Resources = tt.customResources
			}

			r := &DataProtectionApplicationReconciler{dpa: dpa}
			result := r.getVMFileRestoreResources()

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

func TestGetVMFileRestoreSSHImage(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		overrideValue  string
		expectedResult string
	}{
		{
			name:           "Should return override value when set",
			envValue:       "env-ssh-image:v1",
			overrideValue:  "override-ssh-image:v1",
			expectedResult: "override-ssh-image:v1",
		},
		{
			name:           "Should return env value when override not set",
			envValue:       "env-ssh-image:v1",
			overrideValue:  "",
			expectedResult: "env-ssh-image:v1",
		},
		{
			name:           "Should return default when neither set",
			envValue:       "",
			overrideValue:  "",
			expectedResult: defaultVMFileRestoreSSHImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("RELATED_IMAGE_VM_FILE_RESTORE_SSH", tt.envValue)
				defer os.Unsetenv("RELATED_IMAGE_VM_FILE_RESTORE_SSH")
			}

			dpa := &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{},
			}

			if tt.overrideValue != "" {
				dpa.Spec.UnsupportedOverrides = map[oadpv1alpha1.UnsupportedImageKey]string{
					oadpv1alpha1.VMFileRestoreSSHImageKey: tt.overrideValue,
				}
			}

			r := &DataProtectionApplicationReconciler{dpa: dpa}
			result := r.getVMFileRestoreSSHImage()

			if result != tt.expectedResult {
				t.Errorf("expected %s, got %s", tt.expectedResult, result)
			}
		})
	}
}

func TestGetVMFileRestoreBrowserImage(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		overrideValue  string
		expectedResult string
	}{
		{
			name:           "Should return override value when set",
			envValue:       "env-browser-image:v1",
			overrideValue:  "override-browser-image:v1",
			expectedResult: "override-browser-image:v1",
		},
		{
			name:           "Should return env value when override not set",
			envValue:       "env-browser-image:v1",
			overrideValue:  "",
			expectedResult: "env-browser-image:v1",
		},
		{
			name:           "Should return default when neither set",
			envValue:       "",
			overrideValue:  "",
			expectedResult: defaultVMFileRestoreBrowserImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("RELATED_IMAGE_VM_FILE_RESTORE_BROWSER", tt.envValue)
				defer os.Unsetenv("RELATED_IMAGE_VM_FILE_RESTORE_BROWSER")
			}

			dpa := &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{},
			}

			if tt.overrideValue != "" {
				dpa.Spec.UnsupportedOverrides = map[oadpv1alpha1.UnsupportedImageKey]string{
					oadpv1alpha1.VMFileRestoreBrowserImageKey: tt.overrideValue,
				}
			}

			r := &DataProtectionApplicationReconciler{dpa: dpa}
			result := r.getVMFileRestoreBrowserImage()

			if result != tt.expectedResult {
				t.Errorf("expected %s, got %s", tt.expectedResult, result)
			}
		})
	}
}

func TestCheckVMFileRestoreEnabled(t *testing.T) {
	tests := []struct {
		name           string
		vmFileRestore  *oadpv1alpha1.VMFileRestore
		expectedResult bool
	}{
		{
			name:           "Should return false when VMFileRestore is nil",
			vmFileRestore:  nil,
			expectedResult: false,
		},
		{
			name: "Should return false when Enable is nil",
			vmFileRestore: &oadpv1alpha1.VMFileRestore{
				Enable: nil,
			},
			expectedResult: false,
		},
		{
			name: "Should return false when Enable is false",
			vmFileRestore: &oadpv1alpha1.VMFileRestore{
				Enable: ptr.To(false),
			},
			expectedResult: false,
		},
		{
			name: "Should return true when Enable is true",
			vmFileRestore: &oadpv1alpha1.VMFileRestore{
				Enable: ptr.To(true),
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dpa := &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					VMFileRestore: tt.vmFileRestore,
				},
			}

			r := &DataProtectionApplicationReconciler{dpa: dpa}
			result := r.checkVMFileRestoreEnabled()

			if result != tt.expectedResult {
				t.Errorf("expected %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

func TestEnsureVMFileRestoreRequiredLabels(t *testing.T) {
	tests := []struct {
		name           string
		existingLabels map[string]string
		expectedLabels map[string]string
	}{
		{
			name:           "Should set labels when deployment has no labels",
			existingLabels: nil,
			expectedLabels: map[string]string{
				"control-plane":                "oadp-vm-file-restore-controller",
				"app.kubernetes.io/component":  "manager",
				"app.kubernetes.io/created-by": "oadp-operator",
				"app.kubernetes.io/instance":   "oadp-vm-file-restore-controller-manager",
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
				"control-plane":                "oadp-vm-file-restore-controller",
				"app.kubernetes.io/component":  "manager",
				"app.kubernetes.io/created-by": "oadp-operator",
				"app.kubernetes.io/instance":   "oadp-vm-file-restore-controller-manager",
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
				"control-plane":                "oadp-vm-file-restore-controller",
				"app.kubernetes.io/component":  "manager",
				"app.kubernetes.io/created-by": "oadp-operator",
				"app.kubernetes.io/instance":   "oadp-vm-file-restore-controller-manager",
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

			ensureVMFileRestoreRequiredLabels(deployment)

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

func TestBuildVMFileRestoreDeployment(t *testing.T) {
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
			name: "Should build deployment with default resources and images",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					VMFileRestore: &oadpv1alpha1.VMFileRestore{
						Enable: ptr.To(true),
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							LogLevel: "info",
						},
					},
					LogFormat: "text",
				},
			},
			expectedImage:    "quay.io/konveyor/oadp-vm-file-restore:latest",
			expectedEnvCount: 6, // WATCH_NAMESPACE, VMFR_ACCESS_IMAGE, VMFR_SSH_IMAGE, VMFR_BROWSER_IMAGE, LOG_LEVEL, LOG_FORMAT
			expectedReplicas: 1,
			expectedSAName:   "oadp-vm-file-restore-controller-manager",
			expectError:      false,
		},
		{
			name: "Should use custom resources when specified in DPA",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					VMFileRestore: &oadpv1alpha1.VMFileRestore{
						Enable: ptr.To(true),
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			expectedImage:    "quay.io/konveyor/oadp-vm-file-restore:latest",
			expectedEnvCount: 5, // WATCH_NAMESPACE, VMFR_ACCESS_IMAGE, VMFR_SSH_IMAGE, VMFR_BROWSER_IMAGE, LOG_LEVEL
			expectedReplicas: 1,
			expectedSAName:   "oadp-vm-file-restore-controller-manager",
			expectError:      false,
		},
		{
			name: "Should use override images from unsupportedOverrides",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-dpa",
					Namespace:       "test-namespace",
					ResourceVersion: "12345",
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					VMFileRestore: &oadpv1alpha1.VMFileRestore{
						Enable: ptr.To(true),
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.VMFileRestoreControllerImageKey: "custom-registry.io/vmfr:v1.0",
					},
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{},
					},
				},
			},
			expectedImage:    "custom-registry.io/vmfr:v1.0",
			expectedEnvCount: 5, // WATCH_NAMESPACE, VMFR_ACCESS_IMAGE, VMFR_SSH_IMAGE, VMFR_BROWSER_IMAGE, LOG_LEVEL
			expectedReplicas: 1,
			expectedSAName:   "oadp-vm-file-restore-controller-manager",
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
					Name:      vmFileRestoreObjectName,
					Namespace: tt.dpa.Namespace,
				},
			}

			err := r.buildVMFileRestoreDeployment(deployment)

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

			// Verify security context
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
			if labels["control-plane"] != "oadp-vm-file-restore-controller" {
				t.Errorf("control-plane label: expected 'oadp-vm-file-restore-controller', got %s", labels["control-plane"])
			}

			// Verify pod annotations include DPA resource version
			podAnnotations := deployment.Spec.Template.GetAnnotations()
			if _, exists := podAnnotations[vmfrDpaResourceVersionAnnotation]; !exists {
				t.Error("expected pod annotation with DPA resource version")
			}

			// Verify custom resources if specified
			if tt.dpa.Spec.VMFileRestore.Resources != nil {
				expectedCPULimit := tt.dpa.Spec.VMFileRestore.Resources.Limits.Cpu().String()
				actualCPULimit := container.Resources.Limits.Cpu().String()
				if actualCPULimit != expectedCPULimit {
					t.Errorf("CPU limit: expected %s, got %s", expectedCPULimit, actualCPULimit)
				}
			}
		})
	}
}

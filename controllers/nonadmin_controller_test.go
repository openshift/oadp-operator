package controllers

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
	"k8s.io/utils/pointer"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

const defaultNonAdminImage = "quay.io/konveyor/oadp-non-admin:latest"

type ReconcileNonAdminControllerScenario struct {
	namespace       string
	dpa             string
	errMessage      string
	eventWords      []string
	nonAdminEnabled bool
	deployment      *appsv1.Deployment
}

func createTestDeployment(namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nonAdminObjectName,
			Namespace: namespace,
			Labels: map[string]string{
				"test":                   "test",
				"app.kubernetes.io/name": "wrong",
				controlPlaneKey:          "super-wrong",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(2),
			Selector: &metav1.LabelSelector{
				MatchLabels: controlPlaneLabel,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: controlPlaneLabel,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  nonAdminObjectName,
							Image: defaultNonAdminImage,
						},
					},
					ServiceAccountName: "wrong-one",
				},
			},
		},
	}
}

func runReconcileNonAdminControllerTest(
	scenario ReconcileNonAdminControllerScenario,
	updateTestScenario func(scenario ReconcileNonAdminControllerScenario),
	ctx context.Context,
	envVarValue string,
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
			Configuration: &oadpv1alpha1.ApplicationConfig{},
			NonAdmin: &oadpv1alpha1.NonAdmin{
				Enable: pointer.Bool(scenario.nonAdminEnabled),
			},
			UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
				oadpv1alpha1.TechPreviewAck: "true",
			},
		},
	}
	gomega.Expect(k8sClient.Create(ctx, dpa)).To(gomega.Succeed())

	if scenario.deployment != nil {
		gomega.Expect(k8sClient.Create(ctx, scenario.deployment)).To(gomega.Succeed())
	}

	os.Setenv("RELATED_IMAGE_NON_ADMIN_CONTROLLER", envVarValue)
	event := record.NewFakeRecorder(5)
	r := &DPAReconciler{
		Client:  k8sClient,
		Scheme:  testEnv.Scheme,
		Context: ctx,
		NamespacedName: types.NamespacedName{
			Name:      scenario.dpa,
			Namespace: scenario.namespace,
		},
		EventRecorder: event,
	}
	result, err := r.ReconcileNonAdminController(logr.Discard())

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

var _ = ginkgo.Describe("Test ReconcileNonAdminController function", func() {
	var (
		ctx                 = context.Background()
		currentTestScenario ReconcileNonAdminControllerScenario
		updateTestScenario  = func(scenario ReconcileNonAdminControllerScenario) {
			currentTestScenario = scenario
		}
	)

	ginkgo.AfterEach(func() {
		os.Unsetenv("RELATED_IMAGE_NON_ADMIN_CONTROLLER")

		deployment := &appsv1.Deployment{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      nonAdminObjectName,
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
		func(scenario ReconcileNonAdminControllerScenario) {
			runReconcileNonAdminControllerTest(scenario, updateTestScenario, ctx, defaultNonAdminImage)
		},
		ginkgo.Entry("Should create non admin deployment", ReconcileNonAdminControllerScenario{
			namespace:       "test-1",
			dpa:             "test-1-dpa",
			eventWords:      []string{"Normal", "NonAdminDeploymentReconciled", "created"},
			nonAdminEnabled: true,
		}),
		ginkgo.Entry("Should update non admin deployment", ReconcileNonAdminControllerScenario{
			namespace:       "test-2",
			dpa:             "test-2-dpa",
			eventWords:      []string{"Normal", "NonAdminDeploymentReconciled", "updated"},
			nonAdminEnabled: true,
			deployment:      createTestDeployment("test-2"),
		}),
		ginkgo.Entry("Should delete non admin deployment", ReconcileNonAdminControllerScenario{
			namespace:       "test-3",
			dpa:             "test-3-dpa",
			eventWords:      []string{"Normal", "NonAdminDeploymentDeleteSucceed", "deleted"},
			nonAdminEnabled: false,
			deployment:      createTestDeployment("test-3"),
		}),
		ginkgo.Entry("Should do nothing", ReconcileNonAdminControllerScenario{
			namespace:       "test-4",
			dpa:             "test-4-dpa",
			nonAdminEnabled: false,
		}),
	)
})

func TestDPAReconcilerBuildNonAdminDeployment(t *testing.T) {
	r := &DPAReconciler{}
	t.Setenv("RELATED_IMAGE_NON_ADMIN_CONTROLLER", defaultNonAdminImage)
	deployment := createTestDeployment("test-build-deployment")
	r.buildNonAdminDeployment(deployment, &oadpv1alpha1.DataProtectionApplication{})
	labels := deployment.GetLabels()
	if labels["test"] != "test" {
		t.Errorf("Deployment label 'test' has wrong value: %v", labels["test"])
	}
	if labels["app.kubernetes.io/name"] != "deployment" {
		t.Errorf("Deployment label 'app.kubernetes.io/name' has wrong value: %v", labels["app.kubernetes.io/name"])
	}
	if labels[controlPlaneKey] != nonAdminObjectName {
		t.Errorf("Deployment label '%v' has wrong value: %v", controlPlaneKey, labels[controlPlaneKey])
	}
	if *deployment.Spec.Replicas != 1 {
		t.Errorf("Deployment has wrong number of replicas: %v", *deployment.Spec.Replicas)
	}
	if deployment.Spec.Template.Spec.ServiceAccountName != nonAdminObjectName {
		t.Errorf("Deployment has wrong ServiceAccount: %v", deployment.Spec.Template.Spec.ServiceAccountName)
	}
}

func TestEnsureRequiredLabels(t *testing.T) {
	deployment := createTestDeployment("test-ensure-label")
	ensureRequiredLabels(deployment)
	labels := deployment.GetLabels()
	if labels["test"] != "test" {
		t.Errorf("Deployment label 'test' has wrong value: %v", labels["test"])
	}
	if labels["app.kubernetes.io/name"] != "deployment" {
		t.Errorf("Deployment label 'app.kubernetes.io/name' has wrong value: %v", labels["app.kubernetes.io/name"])
	}
	if labels[controlPlaneKey] != nonAdminObjectName {
		t.Errorf("Deployment label '%v' has wrong value: %v", controlPlaneKey, labels[controlPlaneKey])
	}
}

func TestEnsureRequiredSpecs(t *testing.T) {
	deployment := createTestDeployment("test-ensure-spec")
	ensureRequiredSpecs(deployment, defaultNonAdminImage)
	if *deployment.Spec.Replicas != 1 {
		t.Errorf("Deployment has wrong number of replicas: %v", *deployment.Spec.Replicas)
	}
	if deployment.Spec.Template.Spec.ServiceAccountName != nonAdminObjectName {
		t.Errorf("Deployment has wrong ServiceAccount: %v", deployment.Spec.Template.Spec.ServiceAccountName)
	}
}

func TestDPAReconcilerCheckNonAdminEnabled(t *testing.T) {
	r := &DPAReconciler{}
	tests := []struct {
		name   string
		result bool
		dpa    *oadpv1alpha1.DataProtectionApplication
	}{
		{
			name:   "DPA has non admin feature enable: true and tech-preview-ack as true",
			result: true,
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: pointer.Bool(true),
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.TechPreviewAck: "true",
					},
				},
			},
		},
		{
			name:   "DPA has non admin feature enable: false and tech-preview ack as true",
			result: false,
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: pointer.Bool(false),
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.TechPreviewAck: "true",
					},
				},
			},
		},
		{
			name:   "DPA has non admin feature enable: true and tech-preview ack as false",
			result: false,
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: pointer.Bool(true),
					},
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.TechPreviewAck: "false",
					},
				},
			},
		},
		{
			name:   "DPA has non admin feature enable: true and no tech-preview ack unsupported override",
			result: false,
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					NonAdmin: &oadpv1alpha1.NonAdmin{
						Enable: pointer.Bool(true),
					},
				},
			},
		},
		{
			name:   "DPA does not have non admin feature enabled but tech-preview ack is true",
			result: false,
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						oadpv1alpha1.TechPreviewAck: "false",
					},
				},
			},
		},
		{
			name:   "DPA has no non admin feature",
			result: false,
			dpa:    &oadpv1alpha1.DataProtectionApplication{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := r.checkNonAdminEnabled(test.dpa)
			if result != test.result {
				t.Errorf("Results differ: got '%v' but expected '%v'", result, test.result)
			}
		})
	}
}

func TestDPAReconcilerGetNonAdminImage(t *testing.T) {
	r := &DPAReconciler{}
	tests := []struct {
		name  string
		image string
		env   string
		dpa   *oadpv1alpha1.DataProtectionApplication
	}{
		{
			name:  "Get non admin image from environment variable with default value",
			image: defaultNonAdminImage,
			env:   defaultNonAdminImage,
			dpa:   &oadpv1alpha1.DataProtectionApplication{},
		},
		{
			name:  "Get non admin image from environment variable with custom value",
			image: "quay.io/openshift/oadp-non-admin:latest",
			env:   "quay.io/openshift/oadp-non-admin:latest",
			dpa:   &oadpv1alpha1.DataProtectionApplication{},
		},
		{
			name:  "Get non admin image from unsupported overrides",
			image: "quay.io/konveyor/another:latest",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
						"nonAdminControllerImageFqin": "quay.io/konveyor/another:latest",
					},
				},
			},
		},
		{
			name:  "Get non admin image from fallback",
			image: defaultNonAdminImage,
			dpa:   &oadpv1alpha1.DataProtectionApplication{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if len(test.env) > 0 {
				t.Setenv("RELATED_IMAGE_NON_ADMIN_CONTROLLER", test.env)
			}
			image := r.getNonAdminImage(test.dpa)
			if image != test.image {
				t.Errorf("Images differ: got '%v' but expected '%v'", image, test.image)
			}
		})
	}
}

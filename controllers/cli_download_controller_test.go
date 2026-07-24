package controllers

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	consolev1 "github.com/openshift/api/console/v1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// getCLIDownloadTestScheme returns a scheme with the console/route API groups
// registered, suitable for use with a fake client in reconciliation tests.
func getCLIDownloadTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	if err := consolev1.AddToScheme(scheme.Scheme); err != nil {
		t.Fatalf("failed to add consolev1 to scheme: %v", err)
	}
	if err := routev1.AddToScheme(scheme.Scheme); err != nil {
		t.Fatalf("failed to add routev1 to scheme: %v", err)
	}
	return scheme.Scheme
}

// newTestCLIRoute returns a CLI server route with a hostname already
// assigned, so reconcileCLIResources doesn't hit its hostname-assignment
// retry/backoff path.
func newTestCLIRoute(namespace string) *routev1.Route {
	route := buildCLIServerRoute(namespace)
	route.Spec.Host = "cli.example.com"
	return route
}

func TestBuildCLIServerDeployment(t *testing.T) {
	const (
		testNamespace = "openshift-adp"
		testImage     = "quay.io/konveyor/oadp-cli-binaries:oadp-1.3"
	)

	deployment := buildCLIServerDeployment(testNamespace, testImage)

	if deployment.Namespace != testNamespace {
		t.Errorf("expected namespace %q, got %q", testNamespace, deployment.Namespace)
	}
	if deployment.Name != cliServerDeploymentName {
		t.Errorf("expected name %q, got %q", cliServerDeploymentName, deployment.Name)
	}

	podSpec := deployment.Spec.Template.Spec

	if podSpec.ServiceAccountName != cliServerServiceAccountName {
		t.Errorf("expected serviceAccountName %q, got %q", cliServerServiceAccountName, podSpec.ServiceAccountName)
	}
	if podSpec.AutomountServiceAccountToken == nil || *podSpec.AutomountServiceAccountToken {
		t.Error("expected AutomountServiceAccountToken to be false")
	}

	if len(podSpec.Containers) == 0 {
		t.Fatal("expected at least one container")
	}
	container := podSpec.Containers[0]

	if container.Image != testImage {
		t.Errorf("expected image %q, got %q", testImage, container.Image)
	}

	if container.ReadinessProbe == nil {
		t.Fatal("expected ReadinessProbe to be set")
	}
	if container.ReadinessProbe.HTTPGet == nil {
		t.Fatal("expected ReadinessProbe to use HTTPGet")
	}
	if container.ReadinessProbe.HTTPGet.Path != "/" {
		t.Errorf("expected ReadinessProbe path \"/\", got %q", container.ReadinessProbe.HTTPGet.Path)
	}
	if container.ReadinessProbe.HTTPGet.Port != intstr.FromString("http") {
		t.Errorf("expected ReadinessProbe port \"http\", got %v", container.ReadinessProbe.HTTPGet.Port)
	}

	if container.LivenessProbe == nil {
		t.Fatal("expected LivenessProbe to be set")
	}
	if container.LivenessProbe.HTTPGet == nil {
		t.Fatal("expected LivenessProbe to use HTTPGet")
	}
	if container.LivenessProbe.HTTPGet.Path != "/" {
		t.Errorf("expected LivenessProbe path \"/\", got %q", container.LivenessProbe.HTTPGet.Path)
	}
}

func TestBuildCLIServerServiceAccount(t *testing.T) {
	const testNamespace = "openshift-adp"

	sa := buildCLIServerServiceAccount(testNamespace)

	if sa.Name != cliServerServiceAccountName {
		t.Errorf("expected name %q, got %q", cliServerServiceAccountName, sa.Name)
	}
	if sa.Namespace != testNamespace {
		t.Errorf("expected namespace %q, got %q", testNamespace, sa.Namespace)
	}
	if sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken {
		t.Error("expected AutomountServiceAccountToken to be false")
	}
	if sa.Labels[managedByLabel] != operatorName {
		t.Errorf("expected label %q=%q, got %q", managedByLabel, operatorName, sa.Labels[managedByLabel])
	}
}

func TestBuildCLIServerDeployment_SecurityContext(t *testing.T) {
	deployment := buildCLIServerDeployment("openshift-adp", "test-image")
	podSpec := deployment.Spec.Template.Spec

	if podSpec.SecurityContext == nil || podSpec.SecurityContext.RunAsNonRoot == nil || !*podSpec.SecurityContext.RunAsNonRoot {
		t.Error("expected RunAsNonRoot to be true")
	}

	container := podSpec.Containers[0]
	if container.SecurityContext == nil {
		t.Fatal("expected container SecurityContext to be set")
	}
	if container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
		t.Error("expected AllowPrivilegeEscalation to be false")
	}
	if container.SecurityContext.ReadOnlyRootFilesystem == nil || !*container.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("expected ReadOnlyRootFilesystem to be true")
	}

	dropped := false
	for _, cap := range container.SecurityContext.Capabilities.Drop {
		if cap == corev1.Capability("ALL") {
			dropped = true
		}
	}
	if !dropped {
		t.Error("expected ALL capabilities to be dropped")
	}
}

func TestReconcileCLIResources_ReconcilesExistingServiceAccount(t *testing.T) {
	namespace := "openshift-adp"
	testScheme := getCLIDownloadTestScheme(t)

	operatorDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "oadp-operator", Namespace: namespace},
	}

	// Simulate a ServiceAccount that was created before this reconciliation
	// logic existed: token automounting is left at its default (true),
	// required labels are missing, and there's no owner reference.
	automountTrue := true
	existingServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cliServerServiceAccountName,
			Namespace: namespace,
		},
		AutomountServiceAccountToken: &automountTrue,
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(existingServiceAccount, newTestCLIRoute(namespace)).
		Build()

	setup := &CLIDownloadSetup{
		Client:            fakeClient,
		Namespace:         namespace,
		OperatorName:      "oadp-operator",
		OperatorNamespace: namespace,
		Log:               logr.Discard(),
	}

	if err := setup.reconcileCLIResources(context.Background(), operatorDeployment, "test-image"); err != nil {
		t.Fatalf("reconcileCLIResources returned error: %v", err)
	}

	updated := &corev1.ServiceAccount{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: cliServerServiceAccountName, Namespace: namespace}, updated); err != nil {
		t.Fatalf("failed to get updated service account: %v", err)
	}

	if updated.AutomountServiceAccountToken == nil || *updated.AutomountServiceAccountToken {
		t.Error("expected AutomountServiceAccountToken to be reconciled to false")
	}
	if updated.Labels[managedByLabel] != operatorName {
		t.Errorf("expected label %q=%q to be backfilled, got %q", managedByLabel, operatorName, updated.Labels[managedByLabel])
	}
	if len(updated.OwnerReferences) == 0 {
		t.Error("expected an owner reference to be added")
	}
}

func TestReconcileCLIResources_BackfillsMissingStartupProbe(t *testing.T) {
	namespace := "openshift-adp"
	testScheme := getCLIDownloadTestScheme(t)

	operatorDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "oadp-operator", Namespace: namespace},
	}

	// Simulate a Deployment created after readiness/liveness probes existed but
	// before StartupProbe was added: ReadinessProbe/LivenessProbe are already
	// set, so the old backfill gate (keyed only on ReadinessProbe == nil) would
	// never fire and StartupProbe would be stuck missing forever.
	existingDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: cliServerDeploymentName, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: cliServerServiceAccountName,
					Containers: []corev1.Container{
						{
							Name:  "oadp-cli-server",
							Image: "old-image",
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstr.FromString("http")},
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstr.FromString("http")},
								},
							},
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(existingDeployment, newTestCLIRoute(namespace)).
		Build()

	setup := &CLIDownloadSetup{
		Client:            fakeClient,
		Namespace:         namespace,
		OperatorName:      "oadp-operator",
		OperatorNamespace: namespace,
		Log:               logr.Discard(),
	}

	if err := setup.reconcileCLIResources(context.Background(), operatorDeployment, "test-image"); err != nil {
		t.Fatalf("reconcileCLIResources returned error: %v", err)
	}

	updated := &appsv1.Deployment{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: cliServerDeploymentName, Namespace: namespace}, updated); err != nil {
		t.Fatalf("failed to get updated deployment: %v", err)
	}
	container := updated.Spec.Template.Spec.Containers[0]

	if container.StartupProbe == nil {
		t.Error("expected StartupProbe to be backfilled even though ReadinessProbe/LivenessProbe already existed")
	}
	// The image on an existing deployment should not be clobbered by the backfill.
	if container.Image != "old-image" {
		t.Errorf("expected existing image to be preserved, got %q", container.Image)
	}
}

func TestReconcileCLIResources_DoesNotOverwriteExistingStartupProbe(t *testing.T) {
	namespace := "openshift-adp"
	testScheme := getCLIDownloadTestScheme(t)

	operatorDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "oadp-operator", Namespace: namespace},
	}

	customProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Path: "/custom", Port: intstr.FromString("http")},
		},
		InitialDelaySeconds: 99,
		PeriodSeconds:       99,
	}

	existingDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: cliServerDeploymentName, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: cliServerServiceAccountName,
					Containers: []corev1.Container{
						{
							Name:           "oadp-cli-server",
							Image:          "old-image",
							ReadinessProbe: customProbe.DeepCopy(),
							LivenessProbe:  customProbe.DeepCopy(),
							StartupProbe:   customProbe.DeepCopy(),
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(existingDeployment, newTestCLIRoute(namespace)).
		Build()

	setup := &CLIDownloadSetup{
		Client:            fakeClient,
		Namespace:         namespace,
		OperatorName:      "oadp-operator",
		OperatorNamespace: namespace,
		Log:               logr.Discard(),
	}

	if err := setup.reconcileCLIResources(context.Background(), operatorDeployment, "test-image"); err != nil {
		t.Fatalf("reconcileCLIResources returned error: %v", err)
	}

	updated := &appsv1.Deployment{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: cliServerDeploymentName, Namespace: namespace}, updated); err != nil {
		t.Fatalf("failed to get updated deployment: %v", err)
	}
	container := updated.Spec.Template.Spec.Containers[0]

	if container.StartupProbe.InitialDelaySeconds != 99 {
		t.Errorf("expected existing StartupProbe to be preserved, got InitialDelaySeconds=%d", container.StartupProbe.InitialDelaySeconds)
	}
}

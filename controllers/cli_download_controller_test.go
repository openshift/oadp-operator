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

func TestBuildCLIServerDeployment_StartupProbe(t *testing.T) {
	deployment := buildCLIServerDeployment("openshift-adp", "test-image")

	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("expected at least one container")
	}
	container := deployment.Spec.Template.Spec.Containers[0]

	if container.StartupProbe == nil {
		t.Fatal("expected StartupProbe to be set")
	}
	if container.StartupProbe.HTTPGet == nil {
		t.Fatal("expected StartupProbe to use HTTPGet")
	}
	if container.StartupProbe.HTTPGet.Path != "/" {
		t.Errorf("expected StartupProbe path \"/\", got %q", container.StartupProbe.HTTPGet.Path)
	}
	if container.StartupProbe.HTTPGet.Port != intstr.FromString("http") {
		t.Errorf("expected StartupProbe port \"http\", got %v", container.StartupProbe.HTTPGet.Port)
	}
	if container.StartupProbe.FailureThreshold != 12 {
		t.Errorf("expected StartupProbe failureThreshold 12, got %d", container.StartupProbe.FailureThreshold)
	}
	if container.StartupProbe.InitialDelaySeconds != 5 {
		t.Errorf("expected StartupProbe initialDelaySeconds 5, got %d", container.StartupProbe.InitialDelaySeconds)
	}
	if container.StartupProbe.PeriodSeconds != 5 {
		t.Errorf("expected StartupProbe periodSeconds 5, got %d", container.StartupProbe.PeriodSeconds)
	}

	if container.ReadinessProbe == nil {
		t.Fatal("expected ReadinessProbe to remain set")
	}
	if container.LivenessProbe == nil {
		t.Fatal("expected LivenessProbe to remain set")
	}
}

func TestReconcileCLIResources_BackfillsMissingProbes(t *testing.T) {
	namespace := "openshift-adp"
	testScheme := getCLIDownloadTestScheme(t)

	operatorDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "oadp-operator", Namespace: namespace},
	}

	// Simulate a Deployment created by a pre-fix version of the operator: it has
	// the server container, but no probes configured at all.
	existingDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: cliServerDeploymentName, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: cliServerServiceAccountName,
					Containers: []corev1.Container{
						{Name: "oadp-cli-server", Image: "old-image"},
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

	if container.ReadinessProbe == nil {
		t.Error("expected ReadinessProbe to be backfilled")
	}
	if container.LivenessProbe == nil {
		t.Error("expected LivenessProbe to be backfilled")
	}
	if container.StartupProbe == nil {
		t.Error("expected StartupProbe to be backfilled")
	}
	// The image on an existing deployment should not be clobbered by the backfill.
	if container.Image != "old-image" {
		t.Errorf("expected existing image to be preserved, got %q", container.Image)
	}
}

func TestReconcileCLIResources_DoesNotOverwriteExistingProbes(t *testing.T) {
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

	// Simulate a Deployment that already has probes configured (e.g. from a
	// prior reconcile, or customized by a user) to ensure the backfill logic
	// doesn't clobber them.
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

	if container.ReadinessProbe.InitialDelaySeconds != 99 {
		t.Errorf("expected existing ReadinessProbe to be preserved, got InitialDelaySeconds=%d", container.ReadinessProbe.InitialDelaySeconds)
	}
	if container.LivenessProbe.InitialDelaySeconds != 99 {
		t.Errorf("expected existing LivenessProbe to be preserved, got InitialDelaySeconds=%d", container.LivenessProbe.InitialDelaySeconds)
	}
	if container.StartupProbe.InitialDelaySeconds != 99 {
		t.Errorf("expected existing StartupProbe to be preserved, got InitialDelaySeconds=%d", container.StartupProbe.InitialDelaySeconds)
	}
}

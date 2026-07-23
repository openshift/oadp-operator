package controller

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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// getDownloadTestScheme returns a scheme with the console/route API groups
// registered, suitable for use with a fake client in CLI/VMDP reconciliation tests.
func getDownloadTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	if err := consolev1.AddToScheme(scheme.Scheme); err != nil {
		t.Fatalf("failed to add consolev1 to scheme: %v", err)
	}
	if err := routev1.AddToScheme(scheme.Scheme); err != nil {
		t.Fatalf("failed to add routev1 to scheme: %v", err)
	}
	return scheme.Scheme
}

// newTestCLIRoute returns a CLI server route with a hostname already assigned,
// so reconcileCLIResources doesn't hit its hostname-assignment retry/backoff path.
func newTestCLIRoute(namespace string) *routev1.Route {
	route := buildCLIServerRoute(namespace)
	route.Spec.Host = "cli.example.com"
	return route
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
	testScheme := getDownloadTestScheme(t)

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
	testScheme := getDownloadTestScheme(t)

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

func TestBuildCLIServerDeployment_ServiceAccount(t *testing.T) {
	deployment := buildCLIServerDeployment("openshift-adp", "test-image")
	podSpec := deployment.Spec.Template.Spec

	if podSpec.ServiceAccountName != cliServerServiceAccountName {
		t.Errorf("expected serviceAccountName %q, got %q", cliServerServiceAccountName, podSpec.ServiceAccountName)
	}
	if podSpec.AutomountServiceAccountToken == nil || *podSpec.AutomountServiceAccountToken {
		t.Error("expected AutomountServiceAccountToken to be false")
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

// newCLITestScheme registers only corev1 and appsv1 — enough for the
// ServiceAccount and Deployment steps of reconcileCLIResources to succeed,
// but not routev1/consolev1. This means reconcileCLIResources will return an
// error once it reaches the Route step (step 4), which is expected and
// ignored: by that point the ServiceAccount step (step 1) has already run
// and persisted its result, which is what these tests verify.
func newCLITestScheme(t *testing.T) *runtime.Scheme {
	testScheme := runtime.NewScheme()
	if err := corev1.AddToScheme(testScheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(testScheme); err != nil {
		t.Fatal(err)
	}
	return testScheme
}

func newCLITestOperatorDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "oadp-operator",
			Namespace: "openshift-adp",
			UID:       types.UID("test-operator-uid"),
		},
	}
}

// TestReconcileCLIResources_CreatesServiceAccountWhenMissing verifies the
// ServiceAccount is created with the desired state when it doesn't exist.
func TestReconcileCLIResources_CreatesServiceAccountWhenMissing(t *testing.T) {
	const ns = "openshift-adp"
	testScheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(operatorDeploy).Build()

	setup := &CLIDownloadSetup{Client: fakeClient, Namespace: ns, Log: logr.Discard()}
	// Expect an error once reconcileCLIResources reaches the unregistered
	// Route/ConsoleCLIDownload steps — irrelevant to this test.
	if err := setup.reconcileCLIResources(context.Background(), operatorDeploy, "test-image"); err != nil {
		t.Logf("reconcileCLIResources returned expected error (Route/ConsoleCLIDownload step unregistered in test scheme): %v", err)
	}

	got := &corev1.ServiceAccount{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: cliServerServiceAccountName, Namespace: ns}, got); err != nil {
		t.Fatalf("failed to get created SA: %v", err)
	}
	if got.AutomountServiceAccountToken == nil || *got.AutomountServiceAccountToken {
		t.Error("expected AutomountServiceAccountToken=false on created SA")
	}
	if got.Labels[managedByLabel] != operatorName {
		t.Errorf("expected label %q=%q, got %q", managedByLabel, operatorName, got.Labels[managedByLabel])
	}
	if len(got.OwnerReferences) == 0 || got.OwnerReferences[0].UID != operatorDeploy.UID {
		t.Error("expected owner reference to be set on created SA")
	}
}

// TestReconcileCLIResources_FixesExistingServiceAccountDrift verifies that
// when the ServiceAccount already exists with AutomountServiceAccountToken
// unset/true, missing labels, and a missing owner reference, all three are
// corrected by reconcileCLIResources.
func TestReconcileCLIResources_FixesExistingServiceAccountDrift(t *testing.T) {
	const ns = "openshift-adp"
	trueVal := true
	testScheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()

	existing := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cliServerServiceAccountName,
			Namespace: ns,
			// no labels, no owner reference
		},
		AutomountServiceAccountToken: &trueVal, // wrong: should be false
	}

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(operatorDeploy, existing).Build()

	setup := &CLIDownloadSetup{Client: fakeClient, Namespace: ns, Log: logr.Discard()}
	if err := setup.reconcileCLIResources(context.Background(), operatorDeploy, "test-image"); err != nil {
		t.Logf("reconcileCLIResources returned expected error (Route/ConsoleCLIDownload step unregistered in test scheme): %v", err)
	}

	got := &corev1.ServiceAccount{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: cliServerServiceAccountName, Namespace: ns}, got); err != nil {
		t.Fatalf("failed to get SA: %v", err)
	}
	if got.AutomountServiceAccountToken == nil || *got.AutomountServiceAccountToken {
		t.Error("expected AutomountServiceAccountToken to be corrected to false")
	}
	if got.Labels[managedByLabel] != operatorName {
		t.Errorf("expected label %q=%q to be added, got %q", managedByLabel, operatorName, got.Labels[managedByLabel])
	}
	found := false
	for _, ref := range got.OwnerReferences {
		if ref.UID == operatorDeploy.UID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected owner reference to be added")
	}
}

// TestReconcileCLIResources_FixesMissingOwnerReferenceOnly is the key
// regression test: an existing SA that already has the correct automount
// setting and labels, but is MISSING the owner reference, must still be
// updated. This guards against a bug where SetOwnerReference was called
// before checking whether the reference was already present, which always
// found a match (since SetOwnerReference had just added it) and silently
// skipped the Update() call.
func TestReconcileCLIResources_FixesMissingOwnerReferenceOnly(t *testing.T) {
	const ns = "openshift-adp"
	falseVal := false
	testScheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()

	existing := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cliServerServiceAccountName,
			Namespace: ns,
			Labels: map[string]string{
				"app":          "oadp-cli",
				managedByLabel: operatorName,
			},
			// everything else already matches desired state — only the
			// owner reference is missing
		},
		AutomountServiceAccountToken: &falseVal,
	}

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(operatorDeploy, existing).Build()

	setup := &CLIDownloadSetup{Client: fakeClient, Namespace: ns, Log: logr.Discard()}
	if err := setup.reconcileCLIResources(context.Background(), operatorDeploy, "test-image"); err != nil {
		t.Logf("reconcileCLIResources returned expected error (Route/ConsoleCLIDownload step unregistered in test scheme): %v", err)
	}

	got := &corev1.ServiceAccount{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: cliServerServiceAccountName, Namespace: ns}, got); err != nil {
		t.Fatalf("failed to get SA: %v", err)
	}

	found := false
	for _, ref := range got.OwnerReferences {
		if ref.UID == operatorDeploy.UID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected owner reference to operatorDeployment to be added, but it was missing after reconcile")
	}
}

// TestReconcileCLIResources_NoopWhenServiceAccountAlreadyCorrect verifies a
// ServiceAccount already matching the desired state is not unnecessarily
// updated (no ResourceVersion bump).
func TestReconcileCLIResources_NoopWhenServiceAccountAlreadyCorrect(t *testing.T) {
	const ns = "openshift-adp"
	testScheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()

	desired := buildCLIServerServiceAccount(ns)
	desired.OwnerReferences = []metav1.OwnerReference{
		{UID: operatorDeploy.UID, Name: operatorDeploy.Name, Kind: "Deployment", APIVersion: "apps/v1"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(operatorDeploy, desired.DeepCopy()).Build()

	before := &corev1.ServiceAccount{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: cliServerServiceAccountName, Namespace: ns}, before); err != nil {
		t.Fatalf("failed to get SA: %v", err)
	}

	setup := &CLIDownloadSetup{Client: fakeClient, Namespace: ns, Log: logr.Discard()}
	if err := setup.reconcileCLIResources(context.Background(), operatorDeploy, "test-image"); err != nil {
		t.Logf("reconcileCLIResources returned expected error (Route/ConsoleCLIDownload step unregistered in test scheme): %v", err)
	}

	after := &corev1.ServiceAccount{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: cliServerServiceAccountName, Namespace: ns}, after); err != nil {
		t.Fatalf("failed to get SA: %v", err)
	}

	if before.ResourceVersion != after.ResourceVersion {
		t.Errorf("expected no update (ResourceVersion unchanged), got before=%s after=%s", before.ResourceVersion, after.ResourceVersion)
	}
}

package controller

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newTestVMDPRoute returns a VMDP server route with a hostname already
// assigned, so reconcileVMDPResources doesn't hit its hostname-assignment
// retry/backoff path.
func newTestVMDPRoute(namespace string) *routev1.Route {
	route := buildVMDPServerRoute(namespace)
	route.Spec.Host = "vmdp.example.com"
	return route
}

func TestBuildVMDPServerDeployment_StartupProbe(t *testing.T) {
	deployment := buildVMDPServerDeployment("openshift-adp", "test-image")

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

func TestReconcileVMDPResources_BackfillsMissingProbes(t *testing.T) {
	namespace := "openshift-adp"
	testScheme := getDownloadTestScheme(t)

	operatorDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "oadp-operator", Namespace: namespace},
	}

	// Simulate a Deployment created by a pre-fix version of the operator: it has
	// the server container, but no probes configured at all.
	existingDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: vmdpServerDeploymentName, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "oadp-vmdp-server", Image: "old-image"},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(existingDeployment, newTestVMDPRoute(namespace)).
		Build()

	setup := &VMDPDownloadSetup{
		Client:            fakeClient,
		Namespace:         namespace,
		OperatorName:      "oadp-operator",
		OperatorNamespace: namespace,
		Log:               logr.Discard(),
	}

	if err := setup.reconcileVMDPResources(context.Background(), operatorDeployment, "test-image"); err != nil {
		t.Fatalf("reconcileVMDPResources returned error: %v", err)
	}

	updated := &appsv1.Deployment{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: vmdpServerDeploymentName, Namespace: namespace}, updated); err != nil {
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

func TestReconcileVMDPResources_DoesNotOverwriteExistingProbes(t *testing.T) {
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
		ObjectMeta: metav1.ObjectMeta{Name: vmdpServerDeploymentName, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:           "oadp-vmdp-server",
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
		WithObjects(existingDeployment, newTestVMDPRoute(namespace)).
		Build()

	setup := &VMDPDownloadSetup{
		Client:            fakeClient,
		Namespace:         namespace,
		OperatorName:      "oadp-operator",
		OperatorNamespace: namespace,
		Log:               logr.Discard(),
	}

	if err := setup.reconcileVMDPResources(context.Background(), operatorDeployment, "test-image"); err != nil {
		t.Fatalf("reconcileVMDPResources returned error: %v", err)
	}

	updated := &appsv1.Deployment{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: vmdpServerDeploymentName, Namespace: namespace}, updated); err != nil {
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

func TestBuildVMDPServerDeployment_ServiceAccount(t *testing.T) {
	deployment := buildVMDPServerDeployment("openshift-adp", "test-image")
	podSpec := deployment.Spec.Template.Spec

	if podSpec.ServiceAccountName != vmdpServerServiceAccountName {
		t.Errorf("expected serviceAccountName %q, got %q", vmdpServerServiceAccountName, podSpec.ServiceAccountName)
	}
	if podSpec.AutomountServiceAccountToken == nil || *podSpec.AutomountServiceAccountToken {
		t.Error("expected AutomountServiceAccountToken to be false")
	}
}

func TestBuildVMDPServerServiceAccount(t *testing.T) {
	const testNamespace = "openshift-adp"

	sa := buildVMDPServerServiceAccount(testNamespace)

	if sa.Name != vmdpServerServiceAccountName {
		t.Errorf("expected name %q, got %q", vmdpServerServiceAccountName, sa.Name)
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

// TestReconcileVMDPResources_CreatesServiceAccountWhenMissing verifies the
// ServiceAccount is created with the desired state when it doesn't exist.
// Uses newCLITestScheme/newCLITestOperatorDeployment (defined in
// cli_download_controller_test.go) since the same scheme applies here:
// corev1+appsv1 registered, routev1/consolev1 deliberately omitted so
// reconcileVMDPResources errors out cleanly at the Route step, after the
// ServiceAccount step under test has already run.
func TestReconcileVMDPResources_CreatesServiceAccountWhenMissing(t *testing.T) {
	const ns = "openshift-adp"
	testScheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(operatorDeploy).Build()

	setup := &VMDPDownloadSetup{Client: fakeClient, Namespace: ns, Log: logr.Discard()}
	if err := setup.reconcileVMDPResources(context.Background(), operatorDeploy, "test-image"); err != nil {
		t.Logf("reconcileVMDPResources returned expected error (Route/ConsoleCLIDownload step unregistered in test scheme): %v", err)
	}

	got := &corev1.ServiceAccount{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: vmdpServerServiceAccountName, Namespace: ns}, got); err != nil {
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

// TestReconcileVMDPResources_FixesExistingServiceAccountDrift verifies that
// when the ServiceAccount already exists with AutomountServiceAccountToken
// unset/true, missing labels, and a missing owner reference, all three are
// corrected by reconcileVMDPResources.
func TestReconcileVMDPResources_FixesExistingServiceAccountDrift(t *testing.T) {
	const ns = "openshift-adp"
	trueVal := true
	testScheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()

	existing := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmdpServerServiceAccountName,
			Namespace: ns,
			// no labels, no owner reference
		},
		AutomountServiceAccountToken: &trueVal, // wrong: should be false
	}

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(operatorDeploy, existing).Build()

	setup := &VMDPDownloadSetup{Client: fakeClient, Namespace: ns, Log: logr.Discard()}
	if err := setup.reconcileVMDPResources(context.Background(), operatorDeploy, "test-image"); err != nil {
		t.Logf("reconcileVMDPResources returned expected error (Route/ConsoleCLIDownload step unregistered in test scheme): %v", err)
	}

	got := &corev1.ServiceAccount{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: vmdpServerServiceAccountName, Namespace: ns}, got); err != nil {
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

// TestReconcileVMDPResources_FixesMissingOwnerReferenceOnly is the key
// regression test: an existing SA that already has the correct automount
// setting and labels, but is MISSING the owner reference, must still be
// updated. This guards against a bug where SetOwnerReference was called
// before checking whether the reference was already present, which always
// found a match (since SetOwnerReference had just added it) and silently
// skipped the Update() call.
func TestReconcileVMDPResources_FixesMissingOwnerReferenceOnly(t *testing.T) {
	const ns = "openshift-adp"
	falseVal := false
	testScheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()

	existing := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmdpServerServiceAccountName,
			Namespace: ns,
			Labels: map[string]string{
				"app":          "oadp-vmdp",
				managedByLabel: operatorName,
			},
			// everything else already matches desired state — only the
			// owner reference is missing
		},
		AutomountServiceAccountToken: &falseVal,
	}

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(operatorDeploy, existing).Build()

	setup := &VMDPDownloadSetup{Client: fakeClient, Namespace: ns, Log: logr.Discard()}
	if err := setup.reconcileVMDPResources(context.Background(), operatorDeploy, "test-image"); err != nil {
		t.Logf("reconcileVMDPResources returned expected error (Route/ConsoleCLIDownload step unregistered in test scheme): %v", err)
	}

	got := &corev1.ServiceAccount{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: vmdpServerServiceAccountName, Namespace: ns}, got); err != nil {
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

// TestReconcileVMDPResources_NoopWhenServiceAccountAlreadyCorrect verifies a
// ServiceAccount already matching the desired state is not unnecessarily
// updated (no ResourceVersion bump).
func TestReconcileVMDPResources_NoopWhenServiceAccountAlreadyCorrect(t *testing.T) {
	const ns = "openshift-adp"
	testScheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()

	desired := buildVMDPServerServiceAccount(ns)
	desired.OwnerReferences = []metav1.OwnerReference{
		{UID: operatorDeploy.UID, Name: operatorDeploy.Name, Kind: "Deployment", APIVersion: "apps/v1"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(operatorDeploy, desired.DeepCopy()).Build()

	before := &corev1.ServiceAccount{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: vmdpServerServiceAccountName, Namespace: ns}, before); err != nil {
		t.Fatalf("failed to get SA: %v", err)
	}

	setup := &VMDPDownloadSetup{Client: fakeClient, Namespace: ns, Log: logr.Discard()}
	if err := setup.reconcileVMDPResources(context.Background(), operatorDeploy, "test-image"); err != nil {
		t.Logf("reconcileVMDPResources returned expected error (Route/ConsoleCLIDownload step unregistered in test scheme): %v", err)
	}

	after := &corev1.ServiceAccount{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: vmdpServerServiceAccountName, Namespace: ns}, after); err != nil {
		t.Fatalf("failed to get SA: %v", err)
	}

	if before.ResourceVersion != after.ResourceVersion {
		t.Errorf("expected no update (ResourceVersion unchanged), got before=%s after=%s", before.ResourceVersion, after.ResourceVersion)
	}
}

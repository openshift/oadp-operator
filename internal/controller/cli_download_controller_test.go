package controller

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
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
	scheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(operatorDeploy).Build()

	setup := &CLIDownloadSetup{Client: fakeClient, Namespace: ns, Log: logr.Discard()}
	// Expect an error once reconcileCLIResources reaches the unregistered
	// Route/ConsoleCLIDownload steps — irrelevant to this test.
	_ = setup.reconcileCLIResources(context.Background(), operatorDeploy, "test-image")

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
	scheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()

	existing := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cliServerServiceAccountName,
			Namespace: ns,
			// no labels, no owner reference
		},
		AutomountServiceAccountToken: &trueVal, // wrong: should be false
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(operatorDeploy, existing).Build()

	setup := &CLIDownloadSetup{Client: fakeClient, Namespace: ns, Log: logr.Discard()}
	_ = setup.reconcileCLIResources(context.Background(), operatorDeploy, "test-image")

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
	scheme := newCLITestScheme(t)
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

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(operatorDeploy, existing).Build()

	setup := &CLIDownloadSetup{Client: fakeClient, Namespace: ns, Log: logr.Discard()}
	_ = setup.reconcileCLIResources(context.Background(), operatorDeploy, "test-image")

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
	scheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()

	desired := buildCLIServerServiceAccount(ns)
	desired.OwnerReferences = []metav1.OwnerReference{
		{UID: operatorDeploy.UID, Name: operatorDeploy.Name, Kind: "Deployment", APIVersion: "apps/v1"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(operatorDeploy, desired.DeepCopy()).Build()

	before := &corev1.ServiceAccount{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: cliServerServiceAccountName, Namespace: ns}, before); err != nil {
		t.Fatalf("failed to get SA: %v", err)
	}

	setup := &CLIDownloadSetup{Client: fakeClient, Namespace: ns, Log: logr.Discard()}
	_ = setup.reconcileCLIResources(context.Background(), operatorDeploy, "test-image")

	after := &corev1.ServiceAccount{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: cliServerServiceAccountName, Namespace: ns}, after); err != nil {
		t.Fatalf("failed to get SA: %v", err)
	}

	if before.ResourceVersion != after.ResourceVersion {
		t.Errorf("expected no update (ResourceVersion unchanged), got before=%s after=%s", before.ResourceVersion, after.ResourceVersion)
	}
}

package controller

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
	scheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(operatorDeploy).Build()

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
	scheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()

	existing := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmdpServerServiceAccountName,
			Namespace: ns,
			// no labels, no owner reference
		},
		AutomountServiceAccountToken: &trueVal, // wrong: should be false
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(operatorDeploy, existing).Build()

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
	scheme := newCLITestScheme(t)
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

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(operatorDeploy, existing).Build()

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
	scheme := newCLITestScheme(t)
	operatorDeploy := newCLITestOperatorDeployment()

	desired := buildVMDPServerServiceAccount(ns)
	desired.OwnerReferences = []metav1.OwnerReference{
		{UID: operatorDeploy.UID, Name: operatorDeploy.Name, Kind: "Deployment", APIVersion: "apps/v1"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(operatorDeploy, desired.DeepCopy()).Build()

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

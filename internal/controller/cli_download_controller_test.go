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

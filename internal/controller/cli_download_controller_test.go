package controller

import "testing"

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

package controller

import "testing"

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

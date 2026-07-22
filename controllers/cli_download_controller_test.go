package controllers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

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

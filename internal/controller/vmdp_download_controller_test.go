package controller

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"
)

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

	if container.ReadinessProbe == nil {
		t.Fatal("expected ReadinessProbe to remain set")
	}
	if container.LivenessProbe == nil {
		t.Fatal("expected LivenessProbe to remain set")
	}
}

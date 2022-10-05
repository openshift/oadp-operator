package controllers

import (
	"github.com/openshift/oadp-operator/pkg/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

// setting defaults to avoid emitting update events
func setContainerDefaults(container *corev1.Container) {
	// setting defaults to avoid emitting update events
	if container.TerminationMessagePath == "" {
		container.TerminationMessagePath = "/dev/termination-log"
	}
	if container.TerminationMessagePolicy == "" {
		container.TerminationMessagePolicy = corev1.TerminationMessageReadFile
	}
	for i, _ := range container.Ports {
		if container.Ports[i].Protocol == "" {
			container.Ports[i].Protocol = corev1.ProtocolTCP
		}
	}
	for i, _ := range container.Env {
		if container.Env[i].ValueFrom != nil && container.Env[i].ValueFrom.FieldRef != nil && container.Env[i].ValueFrom.FieldRef.APIVersion == "" {
			container.Env[i].ValueFrom.FieldRef.APIVersion = "v1"
		}
	}
}

func setPodTemplateSpecDefaults(template *corev1.PodTemplateSpec) {
	if template.Spec.RestartPolicy == "" {
		template.Spec.RestartPolicy = corev1.RestartPolicyAlways
	}
	if template.Spec.TerminationGracePeriodSeconds == nil {
		template.Spec.TerminationGracePeriodSeconds = pointer.Int64(30)
	}
	if template.Spec.DNSPolicy == "" {
		template.Spec.DNSPolicy = corev1.DNSClusterFirst
	}
	if template.Spec.DeprecatedServiceAccount == "" {
		template.Spec.DeprecatedServiceAccount = common.Velero
	}
	if template.Spec.SecurityContext == nil {
		template.Spec.SecurityContext = &corev1.PodSecurityContext{}
	}
	if template.Spec.SchedulerName == "" {
		template.Spec.SchedulerName = "default-scheduler"
	}
	// for each volumes, if volumeSource is Projected or SecretVolumeSource, set default mode
	for i, _ := range template.Spec.Volumes {
		if template.Spec.Volumes[i].Projected != nil {
			if template.Spec.Volumes[i].Projected != nil {
				template.Spec.Volumes[i].Projected.DefaultMode = common.DefaultModePtr()
			}
		} else if template.Spec.Volumes[i].Secret != nil {
			template.Spec.Volumes[i].Secret.DefaultMode = common.DefaultModePtr()
		} else if template.Spec.Volumes[i].HostPath != nil {
			if template.Spec.Volumes[i].HostPath.Type == nil {
				defaultHostPathType := corev1.HostPathType("")
				template.Spec.Volumes[i].HostPath.Type = &defaultHostPathType
			}
		}
	}
}

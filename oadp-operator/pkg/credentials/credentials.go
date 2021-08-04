package credentials

import (
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type CloudProviderFields struct {
	secretName         string
	mountPath          string
	envCredentialsFile string
}

const (
	veleroSAName     = "velero"
	resticPvHostPath = "/var/lib/kubelet/pods"
	cloudFieldPath   = "cloud"
)

var (
	mountPropagationToHostContainer = corev1.MountPropagationHostToContainer
	cloudProviderConst              = map[oadpv1alpha1.DefaultPlugin]CloudProviderFields{
		oadpv1alpha1.DefaultPluginAWS: {
			secretName:         "cloud-credentials",
			mountPath:          "/credentials",
			envCredentialsFile: "AWS_SHARED_CREDENTIALS_FILE",
		},
		oadpv1alpha1.DefaultPluginGCP: {
			secretName:         "cloud-credentials-gcp",
			mountPath:          "/credentials-gcp",
			envCredentialsFile: "GOOGLE_APPLICATION_CREDENTIALS",
		},
		oadpv1alpha1.DefaultPluginMicrosoftAzure: {
			secretName:         "cloud-credentials-azure",
			mountPath:          "/credentials-azure",
			envCredentialsFile: "AZURE_CREDENTIALS_FILE",
		},
	}
)

func AppendCloudProviderVolumes(velero *oadpv1alpha1.Velero, ds *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	var veleroContainer *corev1.Container
	// Find Velero container
	for _, container := range ds.Spec.Template.Spec.Containers {
		if container.Name == "velero" {
			veleroContainer = &container
		}
	}
	for provider, cloudProviderMap := range cloudProviderConst {
		if contains(provider, velero.Spec.DefaultVeleroPlugins) {
			ds.Spec.Template.Spec.Volumes = append(
				ds.Spec.Template.Spec.Volumes,
				corev1.Volume{
					Name: string(provider),
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: cloudProviderMap.secretName,
						},
					},
				},
			)
			veleroContainer.VolumeMounts = append(
				veleroContainer.VolumeMounts,
				corev1.VolumeMount{
					Name:             cloudProviderMap.secretName,
					MountPath:        cloudProviderMap.mountPath,
					MountPropagation: &mountPropagationToHostContainer,
				},
			)
			veleroContainer.Env = append(
				veleroContainer.Env,
				corev1.EnvVar{
					Name:  cloudProviderMap.envCredentialsFile,
					Value: cloudProviderMap.mountPath + "/" + cloudFieldPath,
				},
			)
		}
	}
	return ds, nil
}

func contains(thisString oadpv1alpha1.DefaultPlugin, thisArray []oadpv1alpha1.DefaultPlugin) bool {
	for _, thisOne := range thisArray {
		if thisOne == thisString {
			return true
		}
	}
	return false
}

package credentials

import (
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type CloudProviderFields struct {
	secretName         string
	mountPath          string
	envCredentialsFile string
}

const (
	cloudFieldPath = "cloud"
)

var (
	mountPropagationToHostContainer = corev1.MountPropagationHostToContainer
	cloudProvidersPluginFields      = map[oadpv1alpha1.DefaultPlugin]CloudProviderFields{
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

func AppendCloudProviderVolumes(velero *oadpv1alpha1.Velero, ds *appsv1.DaemonSet) error {
	var veleroContainer *corev1.Container
	// Find Velero container
	for _, container := range ds.Spec.Template.Spec.Containers {
		if container.Name == common.Velero {
			veleroContainer = &container
		}
	}
	for _, plugin := range velero.Spec.DefaultVeleroPlugins {
		// Check that this is a cloud provider plugin in the cloud provider map
		// ok is boolean that will be true if `plugin` is a valid key in `cloudProvidersPluginFields` map
		// pattern from https://golang.org/doc/effective_go#maps
		// this replaces the need to iterate through the `cloudProvidersPluginFields` O(n) -> O(1)
		if cloudProviderMap, ok := cloudProvidersPluginFields[plugin]; ok {
			ds.Spec.Template.Spec.Volumes = append(
				ds.Spec.Template.Spec.Volumes,
				corev1.Volume{
					Name: string(plugin),
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
	return nil
}

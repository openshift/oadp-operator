package credentials

import (
	"fmt"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"os"
)

type DefaultPluginFields struct {
	IsCloudProvider    bool
	SecretName         string
	MountPath          string
	EnvCredentialsFile string
	PluginImage        string
	PluginSecretKey    string
}

const (
	cloudFieldPath = "cloud"
)

var (
	mountPropagationToHostContainer = corev1.MountPropagationHostToContainer
	PluginSpecificFields            = map[oadpv1alpha1.DefaultPlugin]DefaultPluginFields{
		oadpv1alpha1.DefaultPluginAWS: {
			IsCloudProvider:    true,
			SecretName:         "cloud-credentials",
			MountPath:          "/credentials",
			EnvCredentialsFile: "AWS_SHARED_CREDENTIALS_FILE",
			PluginImage:        fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_AWS_PLUGIN_REPO"), os.Getenv("VELERO_AWS_PLUGIN_TAG")),
			PluginSecretKey:    "cloud",
		},
		oadpv1alpha1.DefaultPluginGCP: {
			IsCloudProvider:    true,
			SecretName:         "cloud-credentials-gcp",
			MountPath:          "/credentials-gcp",
			EnvCredentialsFile: "GOOGLE_APPLICATION_CREDENTIALS",
			PluginImage:        fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_GCP_PLUGIN_REPO"), os.Getenv("VELERO_GCP_PLUGIN_TAG")),
		},
		oadpv1alpha1.DefaultPluginMicrosoftAzure: {
			IsCloudProvider:    true,
			SecretName:         "cloud-credentials-azure",
			MountPath:          "/credentials-azure",
			EnvCredentialsFile: "AZURE_CREDENTIALS_FILE",
			PluginImage:        fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_AZURE_PLUGIN_REPO"), os.Getenv("VELERO_AZURE_PLUGIN_TAG")),
		},
		oadpv1alpha1.DefaultPluginOpenShift: {
			IsCloudProvider: false,
			PluginImage:     fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_OPENSHIFT_PLUGIN_REPO"), os.Getenv("VELERO_OPENSHIFT_PLUGIN_TAG")),
		},
		oadpv1alpha1.DefaultPluginCSI: {
			IsCloudProvider: false,
			//TODO: Check if the Registry needs to an upstream one from CSI
			PluginImage: fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_CSI_PLUGIN_REPO"), os.Getenv("VELERO_CSI_PLUGIN_TAG")),
		},
	}
)

func AppendCloudProviderVolumes(velero *oadpv1alpha1.Velero, ds *appsv1.DaemonSet) error {
	var veleroContainer *corev1.Container
	// Find Velero container
	for i, container := range ds.Spec.Template.Spec.Containers {
		if container.Name == common.Velero {
			veleroContainer = &ds.Spec.Template.Spec.Containers[i]
		}
	}
	for _, plugin := range velero.Spec.DefaultVeleroPlugins {
		// Check that this is a cloud provider plugin in the cloud provider map
		// ok is boolean that will be true if `plugin` is a valid key in `PluginSpecificFields` map
		// pattern from https://golang.org/doc/effective_go#maps
		// this replaces the need to iterate through the `pluginSpecificFields` O(n) -> O(1)
		if cloudProviderMap, ok := PluginSpecificFields[plugin]; ok {
			if !cloudProviderMap.IsCloudProvider {
				continue
			}
			ds.Spec.Template.Spec.Volumes = append(
				ds.Spec.Template.Spec.Volumes,
				corev1.Volume{
					Name: cloudProviderMap.SecretName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: cloudProviderMap.SecretName,
						},
					},
				},
			)
			veleroContainer.VolumeMounts = append(
				veleroContainer.VolumeMounts,
				corev1.VolumeMount{
					Name:      cloudProviderMap.SecretName,
					MountPath: cloudProviderMap.MountPath,
					//TODO: Check if MountPropagation is needed for plugin specific volume mounts
					MountPropagation: &mountPropagationToHostContainer,
				},
			)
			veleroContainer.Env = append(
				veleroContainer.Env,
				corev1.EnvVar{
					Name:  cloudProviderMap.EnvCredentialsFile,
					Value: cloudProviderMap.MountPath + "/" + cloudFieldPath,
				},
			)
		}
	}
	return nil
}

// add plugin specific specs to velero deployment
func AppendPluginSpecficSpecs(velero *oadpv1alpha1.Velero, veleroDeployment *appsv1.Deployment) error {
	var veleroContainer *corev1.Container

	for i, container := range veleroDeployment.Spec.Template.Spec.Containers {
		if container.Name == common.Velero {
			veleroContainer = &veleroDeployment.Spec.Template.Spec.Containers[i]
		}
	}

	for _, plugin := range velero.Spec.DefaultVeleroPlugins {
		if pluginSpecificMap, ok := PluginSpecificFields[plugin]; ok {
			if !pluginSpecificMap.IsCloudProvider {
				continue
			}
			// append plugin specific volume mounts
			if veleroContainer != nil {
				veleroContainer.VolumeMounts = append(
					veleroContainer.VolumeMounts,
					corev1.VolumeMount{
						Name:      pluginSpecificMap.SecretName,
						MountPath: pluginSpecificMap.MountPath,
					})

				// append plugin specific env vars
				veleroContainer.Env = append(
					veleroContainer.Env,
					corev1.EnvVar{
						Name:  pluginSpecificMap.EnvCredentialsFile,
						Value: pluginSpecificMap.MountPath + "/" + cloudFieldPath,
					})
			}

			// append plugin specific volumes
			veleroDeployment.Spec.Template.Spec.Volumes = append(
				veleroDeployment.Spec.Template.Spec.Volumes,
				corev1.Volume{
					Name: pluginSpecificMap.SecretName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: pluginSpecificMap.SecretName,
						},
					},
				})

			// append plugin specifc init containers
			veleroDeployment.Spec.Template.Spec.InitContainers = append(
				veleroDeployment.Spec.Template.Spec.InitContainers,
				corev1.Container{
					Image:                    pluginSpecificMap.PluginImage,
					Name:                     string(plugin),
					ImagePullPolicy:          corev1.PullAlways,
					Resources:                corev1.ResourceRequirements{},
					TerminationMessagePath:   "/dev/termination-log",
					TerminationMessagePolicy: "File",
					VolumeMounts: []corev1.VolumeMount{
						{
							MountPath: "/target",
							Name:      "plugins",
						},
					},
				})

		}
	}
	// append custom plugin init containers
	if velero.Spec.CustomVeleroPlugins != nil {
		for _, plugin := range velero.Spec.CustomVeleroPlugins {
			veleroDeployment.Spec.Template.Spec.InitContainers = append(
				veleroDeployment.Spec.Template.Spec.InitContainers,
				corev1.Container{
					Image:                    plugin.Image,
					Name:                     plugin.Image,
					ImagePullPolicy:          corev1.PullAlways,
					Resources:                corev1.ResourceRequirements{},
					TerminationMessagePath:   "/dev/termination-log",
					TerminationMessagePolicy: "File",
					VolumeMounts: []corev1.VolumeMount{
						{
							MountPath: "/target",
							Name:      "plugins",
						},
					},
				})
		}
	}
	return nil
}

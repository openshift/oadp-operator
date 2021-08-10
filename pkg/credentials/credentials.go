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
	isCloudProvider    bool
	secretName         string
	mountPath          string
	envCredentialsFile string
	pluginImage        string
}

const (
	cloudFieldPath = "cloud"
)

var (
	mountPropagationToHostContainer = corev1.MountPropagationHostToContainer
	pluginSpecificFields            = map[oadpv1alpha1.DefaultPlugin]DefaultPluginFields{
		oadpv1alpha1.DefaultPluginAWS: {
			isCloudProvider:    true,
			secretName:         "cloud-credentials",
			mountPath:          "/credentials",
			envCredentialsFile: "AWS_SHARED_CREDENTIALS_FILE",
			pluginImage:        fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_AWS_PLUGIN_REPO"), os.Getenv("VELERO_AWS_PLUGIN_TAG")),
		},
		oadpv1alpha1.DefaultPluginGCP: {
			isCloudProvider:    true,
			secretName:         "cloud-credentials-gcp",
			mountPath:          "/credentials-gcp",
			envCredentialsFile: "GOOGLE_APPLICATION_CREDENTIALS",
			pluginImage:        fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_GCP_PLUGIN_REPO"), os.Getenv("VELERO_GCP_PLUGIN_TAG")),
		},
		oadpv1alpha1.DefaultPluginMicrosoftAzure: {
			isCloudProvider:    true,
			secretName:         "cloud-credentials-azure",
			mountPath:          "/credentials-azure",
			envCredentialsFile: "AZURE_CREDENTIALS_FILE",
			pluginImage:        fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_AZURE_PLUGIN_REPO"), os.Getenv("VELERO_AZURE_PLUGIN_TAG")),
		},
		oadpv1alpha1.DefaultPluginOpenShift: {
			isCloudProvider: false,
			pluginImage:     fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_OPENSHIFT_PLUGIN_REPO"), os.Getenv("VELERO_OPENSHIFT_PLUGIN_TAG")),
		},
		oadpv1alpha1.DefaultPluginCSI: {
			isCloudProvider: false,
			//TODO: Check if the Registry needs to an upstream one from CSI
			pluginImage: fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_CSI_PLUGIN_REPO"), os.Getenv("VELERO_CSI_PLUGIN_TAG")),
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
		// ok is boolean that will be true if `plugin` is a valid key in `pluginSpecificFields` map
		// pattern from https://golang.org/doc/effective_go#maps
		// this replaces the need to iterate through the `pluginSpecificFields` O(n) -> O(1)
		if cloudProviderMap, ok := pluginSpecificFields[plugin]; ok {
			if !cloudProviderMap.isCloudProvider {
				continue
			}
			ds.Spec.Template.Spec.Volumes = append(
				ds.Spec.Template.Spec.Volumes,
				corev1.Volume{
					Name: cloudProviderMap.secretName,
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
					Name:      cloudProviderMap.secretName,
					MountPath: cloudProviderMap.mountPath,
					//TODO: Check if MountPropagation is needed for plugin specific volume mounts
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

// add plugin specific specs to velero deployment
func AppendPluginSpecficSpecs(velero *oadpv1alpha1.Velero, veleroDeployment *appsv1.Deployment) error {
	var veleroContainer *corev1.Container

	for i, container := range veleroDeployment.Spec.Template.Spec.Containers {
		if container.Name == common.Velero {
			veleroContainer = &veleroDeployment.Spec.Template.Spec.Containers[i]
		}
	}

	for _, plugin := range velero.Spec.DefaultVeleroPlugins {
		if pluginSpecificMap, ok := pluginSpecificFields[plugin]; ok {
			if !pluginSpecificMap.isCloudProvider {
				continue
			}
			// append plugin specific volume mounts
			if veleroContainer != nil {
				veleroContainer.VolumeMounts = append(
					veleroContainer.VolumeMounts,
					corev1.VolumeMount{
						Name:      pluginSpecificMap.secretName,
						MountPath: pluginSpecificMap.mountPath,
					})

				// append plugin specific env vars
				veleroContainer.Env = append(
					veleroContainer.Env,
					corev1.EnvVar{
						Name:  pluginSpecificMap.envCredentialsFile,
						Value: pluginSpecificMap.mountPath + "/" + cloudFieldPath,
					})
			}

			// append plugin specific volumes
			veleroDeployment.Spec.Template.Spec.Volumes = append(
				veleroDeployment.Spec.Template.Spec.Volumes,
				corev1.Volume{
					Name: pluginSpecificMap.secretName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: pluginSpecificMap.secretName,
						},
					},
				})

			// append plugin specifc init containers
			veleroDeployment.Spec.Template.Spec.InitContainers = append(
				veleroDeployment.Spec.Template.Spec.InitContainers,
				corev1.Container{
					Image:                    pluginSpecificMap.pluginImage,
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

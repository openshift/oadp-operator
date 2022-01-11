package credentials

import (
	"errors"
	"fmt"
	"os"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type DefaultPluginFields struct {
	IsCloudProvider    bool
	SecretName         string
	MountPath          string
	EnvCredentialsFile string
	PluginImage        string
	PluginSecretKey    string
	PluginName         string
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
			EnvCredentialsFile: common.AWSSharedCredentialsFileEnvKey,
			PluginName:         common.VeleroPluginForAWS,
			PluginSecretKey:    "cloud",
		},
		oadpv1alpha1.DefaultPluginGCP: {
			IsCloudProvider:    true,
			SecretName:         "cloud-credentials-gcp",
			MountPath:          "/credentials-gcp",
			EnvCredentialsFile: common.GCPCredentialsEnvKey,
			PluginName:         common.VeleroPluginForGCP,
			PluginSecretKey:    "cloud",
		},
		oadpv1alpha1.DefaultPluginMicrosoftAzure: {
			IsCloudProvider:    true,
			SecretName:         "cloud-credentials-azure",
			MountPath:          "/credentials-azure",
			EnvCredentialsFile: common.AzureCredentialsFileEnvKey,
			PluginName:         common.VeleroPluginForAzure,
			PluginSecretKey:    "cloud",
		},
		oadpv1alpha1.DefaultPluginOpenShift: {
			IsCloudProvider: false,
			PluginName:      common.VeleroPluginForOpenshift,
		},
		oadpv1alpha1.DefaultPluginCSI: {
			IsCloudProvider: false,
			//TODO: Check if the Registry needs to an upstream one from CSI
			PluginName: common.VeleroPluginForCSI,
		},
	}
)

func getAWSPluginImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.AWSPluginImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.AWSPluginImageKey]
	}
	if os.Getenv("VELERO_AWS_PLUGIN_REPO") == "" {
		return common.AWSPluginImage
	}
	return fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_AWS_PLUGIN_REPO"), os.Getenv("VELERO_AWS_PLUGIN_TAG"))
}

func getCSIPluginImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.CSIPluginImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.CSIPluginImageKey]
	}
	if os.Getenv("VELERO_CSI_PLUGIN_REPO") == "" {
		return common.CSIPluginImage
	}
	return fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_CSI_PLUGIN_REPO"), os.Getenv("VELERO_CSI_PLUGIN_TAG"))
}

func getGCPPluginImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.GCPPluginImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.GCPPluginImageKey]
	}
	if os.Getenv("VELERO_GCP_PLUGIN_REPO") == "" {
		return common.GCPPluginImage
	}
	return fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_GCP_PLUGIN_REPO"), os.Getenv("VELERO_GCP_PLUGIN_TAG"))
}

func getOpenshiftPluginImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.OpenShiftPluginImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.OpenShiftPluginImageKey]
	}
	if os.Getenv("VELERO_OPENSHIFT_PLUGIN_REPO") == "" {
		return common.OpenshiftPluginImage
	}
	return fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_OPENSHIFT_PLUGIN_REPO"), os.Getenv("VELERO_OPENSHIFT_PLUGIN_TAG"))
}

func getAzurePluginImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.AzurePluginImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.AzurePluginImageKey]
	}
	if os.Getenv("VELERO_AZURE_PLUGIN_REPO") == "" {
		return common.AzurePluginImage
	}
	return fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_AZURE_PLUGIN_REPO"), os.Getenv("VELERO_AZURE_PLUGIN_TAG"))
}

func getPluginImage(pluginName string, dpa *oadpv1alpha1.DataProtectionApplication) string {
	switch pluginName {

	case common.VeleroPluginForAWS:
		return getAWSPluginImage(dpa)

	case common.VeleroPluginForCSI:
		return getCSIPluginImage(dpa)

	case common.VeleroPluginForGCP:
		return getGCPPluginImage(dpa)

	case common.VeleroPluginForOpenshift:
		return getOpenshiftPluginImage(dpa)

	case common.VeleroPluginForAzure:
		return getAzurePluginImage(dpa)
	}
	return ""
}

func AppendCloudProviderVolumes(dpa *oadpv1alpha1.DataProtectionApplication, ds *appsv1.DaemonSet, providerNeedsDefaultCreds map[string]bool, hasCloudStorage bool) error {
	if dpa.Spec.Configuration.Velero == nil {
		return errors.New("velero configuration not found")
	}
	var resticContainer *corev1.Container
	// Find Velero container
	for i, container := range ds.Spec.Template.Spec.Containers {
		if container.Name == common.Restic {
			resticContainer = &ds.Spec.Template.Spec.Containers[i]
		}
	}
	for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
		// Check that this is a cloud provider plugin in the cloud provider map
		// ok is boolean that will be true if `plugin` is a valid key in `PluginSpecificFields` map
		// pattern from https://golang.org/doc/effective_go#maps
		// this replaces the need to iterate through the `pluginSpecificFields` O(n) -> O(1)
		if cloudProviderMap, ok := PluginSpecificFields[plugin]; ok {
			if !cloudProviderMap.IsCloudProvider {
				continue
			}

			pluginNeedsCheck, foundInBSLorVSL := providerNeedsDefaultCreds[string(plugin)]

			if !foundInBSLorVSL && !hasCloudStorage {
				pluginNeedsCheck = true
			}

			if !cloudProviderMap.IsCloudProvider || !pluginNeedsCheck {
				continue
			}

			// default secret name
			secretName := cloudProviderMap.SecretName

			ds.Spec.Template.Spec.Volumes = append(
				ds.Spec.Template.Spec.Volumes,
				corev1.Volume{
					Name: secretName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: secretName,
						},
					},
				},
			)
			if resticContainer != nil {
				resticContainer.VolumeMounts = append(
					resticContainer.VolumeMounts,
					corev1.VolumeMount{
						Name:      secretName,
						MountPath: cloudProviderMap.MountPath,
					},
				)
				resticContainer.Env = append(
					resticContainer.Env,
					corev1.EnvVar{
						Name:  cloudProviderMap.EnvCredentialsFile,
						Value: cloudProviderMap.MountPath + "/" + cloudFieldPath,
					},
				)
			}

		}
	}
	return nil
}

// add plugin specific specs to velero deployment
func AppendPluginSpecificSpecs(dpa *oadpv1alpha1.DataProtectionApplication, veleroDeployment *appsv1.Deployment, veleroContainer *corev1.Container, providerNeedsDefaultCreds map[string]bool, hasCloudStorage bool) error {
	for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
		if pluginSpecificMap, ok := PluginSpecificFields[plugin]; ok {
			veleroDeployment.Spec.Template.Spec.InitContainers = append(
				veleroDeployment.Spec.Template.Spec.InitContainers,
				corev1.Container{
					Image:                    getPluginImage(pluginSpecificMap.PluginName, dpa),
					Name:                     pluginSpecificMap.PluginName,
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

			pluginNeedsCheck, foundInBSLorVSL := providerNeedsDefaultCreds[string(plugin)]

			if !foundInBSLorVSL && !hasCloudStorage {
				pluginNeedsCheck = true
			}

			if !pluginSpecificMap.IsCloudProvider || !pluginNeedsCheck {
				continue
			}
			// set default secret name to use
			secretName := pluginSpecificMap.SecretName
			// append plugin specific volume mounts
			if veleroContainer != nil {
				veleroContainer.VolumeMounts = append(
					veleroContainer.VolumeMounts,
					corev1.VolumeMount{
						Name:      secretName,
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
					Name: secretName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: secretName,
						},
					},
				})

		}
	}
	// append custom plugin init containers
	if dpa.Spec.Configuration.Velero.CustomPlugins != nil {
		for _, plugin := range dpa.Spec.Configuration.Velero.CustomPlugins {
			veleroDeployment.Spec.Template.Spec.InitContainers = append(
				veleroDeployment.Spec.Template.Spec.InitContainers,
				corev1.Container{
					Image:                    plugin.Image,
					Name:                     plugin.Name,
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

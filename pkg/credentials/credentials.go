package credentials

import (
	"context"
	"errors"
	"os"
	"strings"

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/client"
	"github.com/openshift/oadp-operator/pkg/common"
)

type DefaultPluginFields struct {
	IsCloudProvider    bool
	SecretName         string
	MountPath          string
	EnvCredentialsFile string
	PluginImage        string
	PluginSecretKey    string
	PluginName         string
	ProviderName       string
}

const (
	CloudFieldPath = "cloud"
)

var (
	PluginSpecificFields = map[oadpv1alpha1.DefaultPlugin]DefaultPluginFields{
		oadpv1alpha1.DefaultPluginAWS: {
			IsCloudProvider:    true,
			SecretName:         "cloud-credentials",
			MountPath:          "/credentials",
			EnvCredentialsFile: common.AWSSharedCredentialsFileEnvKey,
			PluginName:         common.VeleroPluginForAWS,
			ProviderName:       string(oadpv1alpha1.DefaultPluginAWS),
			PluginSecretKey:    "cloud",
		},
		oadpv1alpha1.DefaultPluginLegacyAWS: {
			IsCloudProvider:    true,
			SecretName:         "cloud-credentials",
			MountPath:          "/credentials",
			EnvCredentialsFile: common.AWSSharedCredentialsFileEnvKey,
			PluginName:         common.VeleroPluginForLegacyAWS,
			ProviderName:       string(oadpv1alpha1.DefaultPluginAWS),
			PluginSecretKey:    "cloud",
		},
		oadpv1alpha1.DefaultPluginGCP: {
			IsCloudProvider:    true,
			SecretName:         "cloud-credentials-gcp",
			MountPath:          "/credentials-gcp",
			EnvCredentialsFile: common.GCPCredentialsEnvKey,
			PluginName:         common.VeleroPluginForGCP,
			ProviderName:       string(oadpv1alpha1.DefaultPluginGCP),
			PluginSecretKey:    "cloud",
		},
		oadpv1alpha1.DefaultPluginMicrosoftAzure: {
			IsCloudProvider:    true,
			SecretName:         "cloud-credentials-azure",
			MountPath:          "/credentials-azure",
			EnvCredentialsFile: common.AzureCredentialsFileEnvKey,
			PluginName:         common.VeleroPluginForAzure,
			ProviderName:       string(oadpv1alpha1.DefaultPluginMicrosoftAzure),
			PluginSecretKey:    "cloud",
		},
		oadpv1alpha1.DefaultPluginOpenShift: {
			IsCloudProvider: false,
			PluginName:      common.VeleroPluginForOpenshift,
		},
		oadpv1alpha1.DefaultPluginKubeVirt: {
			IsCloudProvider: false,
			PluginName:      common.KubeVirtPlugin,
		},
	}
)

// Get secretName and secretKey from "secretName/secretKey"
func GetSecretNameKeyFromCredentialsFileConfigString(credentialsFile string) (string, string, error) {
	credentialsFile = strings.TrimSpace(credentialsFile)
	if credentialsFile == "" {
		return "", "", nil
	}
	nameKeyArray := strings.Split(credentialsFile, "/")
	if len(nameKeyArray) != 2 {
		return "", "", errors.New("credentials file is not supported")
	}
	return nameKeyArray[0], nameKeyArray[1], nil
}

func GetSecretNameFromCredentialsFileConfigString(credentialsFile string) (string, error) {
	name, _, err := GetSecretNameKeyFromCredentialsFileConfigString(credentialsFile)
	return name, err
}

func getAWSPluginImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.AWSPluginImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.AWSPluginImageKey]
	}
	if os.Getenv("RELATED_IMAGE_VELERO_PLUGIN_FOR_AWS") == "" {
		return common.AWSPluginImage
	}
	return os.Getenv("RELATED_IMAGE_VELERO_PLUGIN_FOR_AWS")
}

func getLegacyAWSPluginImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.LegacyAWSPluginImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.LegacyAWSPluginImageKey]
	}
	if os.Getenv("RELATED_IMAGE_VELERO_PLUGIN_FOR_LEGACY_AWS") == "" {
		return common.LegacyAWSPluginImage
	}
	return os.Getenv("RELATED_IMAGE_VELERO_PLUGIN_FOR_LEGACY_AWS")
}

func getGCPPluginImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.GCPPluginImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.GCPPluginImageKey]
	}
	if os.Getenv("RELATED_IMAGE_VELERO_PLUGIN_FOR_GCP") == "" {
		return common.GCPPluginImage
	}
	return os.Getenv("RELATED_IMAGE_VELERO_PLUGIN_FOR_GCP")
}

func getOpenshiftPluginImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.OpenShiftPluginImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.OpenShiftPluginImageKey]
	}
	if os.Getenv("RELATED_IMAGE_OPENSHIFT_VELERO_PLUGIN") == "" {
		return common.OpenshiftPluginImage
	}
	return os.Getenv("RELATED_IMAGE_OPENSHIFT_VELERO_PLUGIN")
}

func getAzurePluginImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.AzurePluginImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.AzurePluginImageKey]
	}
	if os.Getenv("RELATED_IMAGE_VELERO_PLUGIN_FOR_MICROSOFT_AZURE") == "" {
		return common.AzurePluginImage
	}
	return os.Getenv("RELATED_IMAGE_VELERO_PLUGIN_FOR_MICROSOFT_AZURE")
}

func getKubeVirtPluginImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.KubeVirtPluginImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.KubeVirtPluginImageKey]
	}
	if os.Getenv("RELATED_IMAGE_KUBEVIRT_VELERO_PLUGIN") == "" {
		return common.KubeVirtPluginImage
	}
	return os.Getenv("RELATED_IMAGE_KUBEVIRT_VELERO_PLUGIN")
}

func GetPluginImage(defaultPlugin oadpv1alpha1.DefaultPlugin, dpa *oadpv1alpha1.DataProtectionApplication) string {
	switch defaultPlugin {

	case oadpv1alpha1.DefaultPluginAWS:
		return getAWSPluginImage(dpa)

	case oadpv1alpha1.DefaultPluginLegacyAWS:
		return getLegacyAWSPluginImage(dpa)

	case oadpv1alpha1.DefaultPluginGCP:
		return getGCPPluginImage(dpa)

	case oadpv1alpha1.DefaultPluginOpenShift:
		return getOpenshiftPluginImage(dpa)

	case oadpv1alpha1.DefaultPluginMicrosoftAzure:
		return getAzurePluginImage(dpa)

	case oadpv1alpha1.DefaultPluginKubeVirt:
		return getKubeVirtPluginImage(dpa)
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
		if container.Name == common.NodeAgent {
			resticContainer = &ds.Spec.Template.Spec.Containers[i]
		}
	}
	for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
		// Check that this is a cloud provider plugin in the cloud provider map
		// ok is boolean that will be true if `plugin` is a valid key in `PluginSpecificFields` map
		// pattern from https://golang.org/doc/effective_go#maps
		// this replaces the need to iterate through the `pluginSpecificFields` O(n) -> O(1)
		if cloudProviderMap, ok := PluginSpecificFields[plugin]; ok &&
			cloudProviderMap.IsCloudProvider && //if plugin is a cloud provider plugin, and one of the following condition is true
			(!dpa.Spec.Configuration.Velero.NoDefaultBackupLocation || // it has a backup location in OADP/velero context OR
				dpa.Spec.UnsupportedOverrides[oadpv1alpha1.OperatorTypeKey] == oadpv1alpha1.OperatorTypeMTC) { // OADP is installed via MTC

			pluginNeedsCheck, foundProviderPlugin := providerNeedsDefaultCreds[cloudProviderMap.ProviderName]
			// duplication with controllers/validator.go
			if !foundProviderPlugin && !hasCloudStorage {
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
						Value: cloudProviderMap.MountPath + "/" + CloudFieldPath,
					},
				)
			}

		}
	}
	for _, bslSpec := range dpa.Spec.BackupLocations {
		if bslSpec.Velero != nil {
			if _, ok := bslSpec.Velero.Config["credentialsFile"]; ok {
				if secretName, err := GetSecretNameFromCredentialsFileConfigString(bslSpec.Velero.Config["credentialsFile"]); err == nil {
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
				}
			}
		}

	}
	return nil
}

// TODO: remove duplicate func in registry.go - refactoring away registry.go later
func GetSecretNameAndKey(bslSpec *velerov1.BackupStorageLocationSpec, plugin oadpv1alpha1.DefaultPlugin) (string, string) {
	// Assume default values unless user has overriden them
	secretName := PluginSpecificFields[plugin].SecretName
	secretKey := PluginSpecificFields[plugin].PluginSecretKey
	if _, ok := bslSpec.Config["credentialsFile"]; ok {
		if secretName, secretKey, err :=
			GetSecretNameKeyFromCredentialsFileConfigString(bslSpec.Config["credentialsFile"]); err == nil {
			return secretName, secretKey
		}
	}
	// check if user specified the Credential Name and Key
	credential := bslSpec.Credential
	if credential != nil {
		if len(credential.Name) > 0 {
			secretName = credential.Name
		}
		if len(credential.Key) > 0 {
			secretKey = credential.Key
		}
	}

	return secretName, secretKey
}

// TODO: remove duplicate func in registry.go - refactoring away registry.go later
// Get for a given backup location
// - secret name
// - key
// - bsl config
// - provider
// - error
func GetSecretNameKeyConfigProviderForBackupLocation(blspec oadpv1alpha1.BackupLocation, namespace string) (string, string, string, map[string]string, error) {
	if blspec.Velero != nil {
		name, key := GetSecretNameAndKey(blspec.Velero, oadpv1alpha1.DefaultPlugin(blspec.Velero.Provider))
		return name, key, blspec.Velero.Provider, blspec.Velero.Config, nil
	}
	if blspec.CloudStorage != nil {
		if blspec.CloudStorage.Credential != nil {
			// Get CloudStorageRef provider
			cs := oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      blspec.CloudStorage.CloudStorageRef.Name,
					Namespace: namespace,
				},
			}
			err := client.GetClient().Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: blspec.CloudStorage.CloudStorageRef.Name}, &cs)
			if err != nil {
				return "", "", "", nil, err
			}
			return blspec.CloudStorage.Credential.Name, blspec.CloudStorage.Credential.Key, string(cs.Spec.Provider), blspec.CloudStorage.Config, nil
		}
	}
	return "", "", "", nil, nil
}

// Iterate through all backup locations and return true if any of them use short lived credentials
func BslUsesShortLivedCredential(bls []oadpv1alpha1.BackupLocation, namespace string) (ret bool, err error) {
	for _, blspec := range bls {
		if blspec.CloudStorage != nil && blspec.CloudStorage.Credential != nil {
			// Get CloudStorageRef provider
			cs := oadpv1alpha1.CloudStorage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      blspec.CloudStorage.CloudStorageRef.Name,
					Namespace: namespace,
				},
			}
			err = client.GetClient().Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: blspec.CloudStorage.CloudStorageRef.Name}, &cs)
			if err != nil {
				return false, err
			}
			if cs.Spec.EnableSharedConfig != nil && *cs.Spec.EnableSharedConfig {
				return true, nil
			}
		}
		secretName, secretKey, provider, config, err := GetSecretNameKeyConfigProviderForBackupLocation(blspec, namespace)
		if err != nil {
			return false, err
		}
		ret, err = SecretContainsShortLivedCredential(secretName, secretKey, provider, namespace, config)
		if err != nil {
			return false, err
		}
		if ret {
			return true, nil
		}
	}
	return ret, err
}

func SecretContainsShortLivedCredential(secretName, secretKey, provider, namespace string, config map[string]string) (bool, error) {
	switch provider {
	case "aws":
		// AWS credentials short lived are determined by enableSharedConfig
		// if enableSharedConfig is not set, then we assume it is not short lived
		// if enableSharedConfig is set, then we assume it is short lived
		// Alternatively, we can check if the secret contains a session token
		// TODO: check if secret contains session token
		return false, nil
	case "gcp":
		return gcpSecretAccountTypeIsShortLived(secretName, secretKey, namespace)
	case "azure":
		// TODO: check if secret contains session token
		return false, nil
	}
	return false, nil
}

func GetDecodedSecret(secretName, secretKey, namespace string) (s string, err error) {
	bytes, err := GetDecodedSecretAsByte(secretName, secretKey, namespace)
	return string(bytes), err
}

func GetDecodedSecretAsByte(secretName, secretKey, namespace string) ([]byte, error) {
	if secretName != "" && secretKey != "" {
		var secret corev1.Secret
		err := client.GetClient().Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: secretName}, &secret)
		if err != nil {
			return []byte{}, errors.Join(errors.New("error getting provider secret"+secretName), err)
		}
		if secret.Data[secretKey] != nil {
			return secret.Data[secretKey], nil
		}
	}
	return []byte{}, nil
}

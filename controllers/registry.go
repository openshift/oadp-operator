package controllers

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/openshift/oadp-operator/pkg/credentials"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Registry Env var keys
const (
	// AWS registry env vars
	RegistryStorageEnvVarKey                 = "REGISTRY_STORAGE"
	RegistryStorageS3AccesskeyEnvVarKey      = "REGISTRY_STORAGE_S3_ACCESSKEY"
	RegistryStorageS3BucketEnvVarKey         = "REGISTRY_STORAGE_S3_BUCKET"
	RegistryStorageS3RegionEnvVarKey         = "REGISTRY_STORAGE_S3_REGION"
	RegistryStorageS3SecretkeyEnvVarKey      = "REGISTRY_STORAGE_S3_SECRETKEY"
	RegistryStorageS3RegionendpointEnvVarKey = "REGISTRY_STORAGE_S3_REGIONENDPOINT"
	RegistryStorageS3RootdirectoryEnvVarKey  = "REGISTRY_STORAGE_S3_ROOTDIRECTORY"
	RegistryStorageS3SkipverifyEnvVarKey     = "REGISTRY_STORAGE_S3_SKIPVERIFY"
	// Azure registry env vars
	RegistryStorageAzureContainerEnvVarKey       = "REGISTRY_STORAGE_AZURE_CONTAINER"
	RegistryStorageAzureAccountnameEnvVarKey     = "REGISTRY_STORAGE_AZURE_ACCOUNTNAME"
	RegistryStorageAzureAccountkeyEnvVarKey      = "REGISTRY_STORAGE_AZURE_ACCOUNTKEY"
	RegistryStorageAzureSPNClientIDEnvVarKey     = "REGISTRY_STORAGE_AZURE_SPN_CLIENT_ID"
	RegistryStorageAzureSPNClientSecretEnvVarKey = "REGISTRY_STORAGE_AZURE_SPN_CLIENT_SECRET"
	RegistryStorageAzureSPNTenantIDEnvVarKey     = "REGISTRY_STORAGE_AZURE_SPN_TENANT_ID"
	RegistryStorageAzureAADEndpointEnvVarKey     = "REGISTRY_STORAGE_AZURE_AAD_ENDPOINT"
	// GCP registry env vars
	RegistryStorageGCSBucket        = "REGISTRY_STORAGE_GCS_BUCKET"
	RegistryStorageGCSKeyfile       = "REGISTRY_STORAGE_GCS_KEYFILE"
	RegistryStorageGCSRootdirectory = "REGISTRY_STORAGE_GCS_ROOTDIRECTORY"
)

// provider specific object storage
const (
	S3                    = "s3"
	Azure                 = "azure"
	GCS                   = "gcs"
	AWSProvider           = "aws"
	AzureProvider         = "azure"
	GCPProvider           = "gcp"
	Region                = "region"
	Profile               = "profile"
	S3URL                 = "s3Url"
	S3ForcePathStyle      = "s3ForcePathStyle"
	InsecureSkipTLSVerify = "insecureSkipTLSVerify"
	StorageAccount        = "storageAccount"
	ResourceGroup         = "resourceGroup"
)

// creating skeleton for provider based env var map
var cloudProviderEnvVarMap = map[string][]corev1.EnvVar{
	"aws": {
		{
			Name:  RegistryStorageEnvVarKey,
			Value: S3,
		},
		{
			Name:  RegistryStorageS3AccesskeyEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageS3BucketEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageS3RegionEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageS3SecretkeyEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageS3RegionendpointEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageS3SkipverifyEnvVarKey,
			Value: "",
		},
	},
	"azure": {
		{
			Name:  RegistryStorageEnvVarKey,
			Value: Azure,
		},
		{
			Name:  RegistryStorageAzureContainerEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageAzureAccountnameEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageAzureAccountkeyEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageAzureAADEndpointEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageAzureSPNClientIDEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageAzureSPNClientSecretEnvVarKey,
			Value: "",
		},
		{
			Name:  RegistryStorageAzureSPNTenantIDEnvVarKey,
			Value: "",
		},
	},
	"gcp": {
		{
			Name:  RegistryStorageEnvVarKey,
			Value: GCS,
		},
		{
			Name:  RegistryStorageGCSBucket,
			Value: "",
		},
		{
			Name:  RegistryStorageGCSKeyfile,
			Value: "",
		},
	},
}

type azureCredentials struct {
	subscriptionID     string
	tenantID           string
	clientID           string
	clientSecret       string
	resourceGroup      string
	strorageAccountKey string
}

func (r *DPAReconciler) ReconcileRegistries(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	bslLabels := map[string]string{
		"app.kubernetes.io/name":             common.OADPOperatorVelero,
		"app.kubernetes.io/managed-by":       common.OADPOperator,
		"app.kubernetes.io/component":        "bsl",
		oadpv1alpha1.RegistryDeploymentLabel: "True",
	}
	bslListOptions := client.MatchingLabels(bslLabels)
	backupStorageLocationList := velerov1.BackupStorageLocationList{}

	// Fetch the configured backupstoragelocations
	if err := r.List(r.Context, &backupStorageLocationList, bslListOptions); err != nil {
		return false, err
	}

	// Loop through all the configured BSLs and create registry for each of them
	for _, bsl := range backupStorageLocationList.Items {
		registryDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      registryName(&bsl),
				Namespace: bsl.Namespace,
			},
		}
		deleteContext := context.Background()
		if err := r.Get(deleteContext, types.NamespacedName{
			Name:      registryDeployment.Name,
			Namespace: r.NamespacedName.Namespace,
		}, registryDeployment); err != nil {
			if k8serror.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}

		deleteOptionPropagationForeground := metav1.DeletePropagationForeground
		if err := r.Delete(deleteContext, registryDeployment, &client.DeleteOptions{PropagationPolicy: &deleteOptionPropagationForeground}); err != nil {
			r.EventRecorder.Event(registryDeployment, corev1.EventTypeNormal, "DeleteRegistryDeploymentFailed", fmt.Sprint("Could not delete registry deployment %s from %s:"+err.Error(), registryDeployment.Name, registryDeployment.Namespace))
			return false, err
		}
		r.EventRecorder.Event(registryDeployment, corev1.EventTypeNormal, "DeletedRegistryDeployment", fmt.Sprintf("Registry Deployment %s deleted from %s", registryDeployment.Name, registryDeployment.Namespace))
	}

	return true, nil
}

func registryName(bsl *velerov1.BackupStorageLocation) string {
	return "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry"
}

func (r *DPAReconciler) getProviderSecret(secretName string) (corev1.Secret, error) {

	secret := corev1.Secret{}
	key := types.NamespacedName{
		Name:      secretName,
		Namespace: r.NamespacedName.Namespace,
	}
	err := r.Get(r.Context, key, &secret)

	if err != nil {
		return secret, err
	}
	originalSecret := secret.DeepCopy()
	// replace carriage return with new line
	secret.Data = replaceCarriageReturn(secret.Data, r.Log)
	r.Client.Patch(r.Context, &secret, client.MergeFrom(originalSecret))
	return secret, nil
}

func replaceCarriageReturn(data map[string][]byte, logger logr.Logger) map[string][]byte {
	for k, v := range data {
		// report if carriage return is found
		if strings.Contains(string(v), "\r\n") {
			logger.Info("carriage return replaced")
			data[k] = []byte(strings.ReplaceAll(string(v), "\r\n", "\n"))
		}
	}
	return data
}

func (r *DPAReconciler) getSecretNameAndKeyforBackupLocation(bslspec oadpv1alpha1.BackupLocation) (string, string) {

	if bslspec.CloudStorage != nil {
		if bslspec.CloudStorage.Credential != nil {
			return bslspec.CloudStorage.Credential.Name, bslspec.CloudStorage.Credential.Key
		}
	}
	if bslspec.Velero != nil {
		return r.getSecretNameAndKey(bslspec.Velero, oadpv1alpha1.DefaultPlugin(bslspec.Velero.Provider))
	}

	return "", ""
}

func (r *DPAReconciler) getSecretNameAndKey(bslSpec *velerov1.BackupStorageLocationSpec, plugin oadpv1alpha1.DefaultPlugin) (string, string) {
	// Assume default values unless user has overriden them
	secretName := credentials.PluginSpecificFields[plugin].SecretName
	secretKey := credentials.PluginSpecificFields[plugin].PluginSecretKey
	if _, ok := bslSpec.Config["credentialsFile"]; ok {
		if secretName, secretKey, err :=
			credentials.GetSecretNameKeyFromCredentialsFileConfigString(bslSpec.Config["credentialsFile"]); err == nil {
			r.Log.Info(fmt.Sprintf("credentialsFile secret: %s, key: %s", secretName, secretKey))
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

func (r *DPAReconciler) parseAWSSecret(secret corev1.Secret, secretKey string, matchProfile string) (string, string, error) {

	AWSAccessKey, AWSSecretKey, profile := "", "", ""
	splitString := strings.Split(string(secret.Data[secretKey]), "\n")
	keyNameRegex, err := regexp.Compile(`\[.*\]`)
	const (
		accessKeyKey = "aws_access_key_id"
		secretKeyKey = "aws_secret_access_key"
	)
	if err != nil {
		return AWSAccessKey, AWSSecretKey, errors.New("parseAWSSecret faulty regex: keyNameRegex")
	}
	awsAccessKeyRegex, err := regexp.Compile(`\b` + accessKeyKey + `\b`)
	if err != nil {
		return AWSAccessKey, AWSSecretKey, errors.New("parseAWSSecret faulty regex: awsAccessKeyRegex")
	}
	awsSecretKeyRegex, err := regexp.Compile(`\b` + secretKeyKey + `\b`)
	if err != nil {
		return AWSAccessKey, AWSSecretKey, errors.New("parseAWSSecret faulty regex: awsSecretKeyRegex")
	}
	for index, line := range splitString {
		if line == "" {
			continue
		}
		if keyNameRegex.MatchString(line) {
			awsProfileRegex, err := regexp.Compile(`\[|\]`)
			if err != nil {
				return AWSAccessKey, AWSSecretKey, errors.New("parseAWSSecret faulty regex: keyNameRegex")
			}
			cleanedLine := strings.ReplaceAll(line, " ", "")
			parsedProfile := awsProfileRegex.ReplaceAllString(cleanedLine, "")
			if parsedProfile == matchProfile {
				profile = matchProfile
				// check for end of arr
				if index+1 >= len(splitString) {
					break
				}
				for _, profLine := range splitString[index+1:] {
					if profLine == "" {
						continue
					}
					matchedAccessKey := awsAccessKeyRegex.MatchString(profLine)
					matchedSecretKey := awsSecretKeyRegex.MatchString(profLine)

					if err != nil {
						r.Log.Info("Error finding access key id for the supplied AWS credential")
						return AWSAccessKey, AWSSecretKey, err
					}
					if matchedAccessKey { // check for access key
						AWSAccessKey, err = r.getMatchedKeyValue(accessKeyKey, profLine)
						if err != nil {
							r.Log.Info("Error processing access key id for the supplied AWS credential")
							return AWSAccessKey, AWSSecretKey, err
						}
						continue
					} else if matchedSecretKey { // check for secret key
						AWSSecretKey, err = r.getMatchedKeyValue(secretKeyKey, profLine)
						if err != nil {
							r.Log.Info("Error processing secret key id for the supplied AWS credential")
							return AWSAccessKey, AWSSecretKey, err
						}
						continue
					} else {
						break // aws credentials file is only allowed to have profile followed by aws_access_key_id, aws_secret_access_key
					}
				}
			}
		}
	}
	if profile == "" {
		r.Log.Info("Error finding AWS Profile for the supplied AWS credential")
		return AWSAccessKey, AWSSecretKey, errors.New("error finding AWS Profile for the supplied AWS credential")
	}
	if AWSAccessKey == "" {
		r.Log.Info("Error finding access key id for the supplied AWS credential")
		return AWSAccessKey, AWSSecretKey, errors.New("error finding access key id for the supplied AWS credential")
	}
	if AWSSecretKey == "" {
		r.Log.Info("Error finding secret access key for the supplied AWS credential")
		return AWSAccessKey, AWSSecretKey, errors.New("error finding secret access key for the supplied AWS credential")
	}

	return AWSAccessKey, AWSSecretKey, nil
}

func (r *DPAReconciler) parseAzureSecret(secret corev1.Secret, secretKey string) (azureCredentials, error) {

	azcreds := azureCredentials{}

	splitString := strings.Split(string(secret.Data[secretKey]), "\n")
	keyNameRegex, err := regexp.Compile(`\[.*\]`) //ignore lines such as [default]
	if err != nil {
		return azcreds, errors.New("parseAzureSecret faulty regex: keyNameRegex")
	}
	azureStorageKeyRegex, err := regexp.Compile(`\bAZURE_STORAGE_ACCOUNT_ACCESS_KEY\b`)
	if err != nil {
		return azcreds, errors.New("parseAzureSecret faulty regex: azureStorageKeyRegex")
	}
	azureTenantIdRegex, err := regexp.Compile(`\bAZURE_TENANT_ID\b`)
	if err != nil {
		return azcreds, errors.New("parseAzureSecret faulty regex: azureTenantIdRegex")
	}
	azureClientIdRegex, err := regexp.Compile(`\bAZURE_CLIENT_ID\b`)
	if err != nil {
		return azcreds, errors.New("parseAzureSecret faulty regex: azureClientIdRegex")
	}
	azureClientSecretRegex, err := regexp.Compile(`\bAZURE_CLIENT_SECRET\b`)
	if err != nil {
		return azcreds, errors.New("parseAzureSecret faulty regex: azureClientSecretRegex")
	}
	azureResourceGroupRegex, err := regexp.Compile(`\bAZURE_RESOURCE_GROUP\b`)
	if err != nil {
		return azcreds, errors.New("parseAzureSecret faulty regex: azureResourceGroupRegex")
	}
	azureSubscriptionIdRegex, err := regexp.Compile(`\bAZURE_SUBSCRIPTION_ID\b`)
	if err != nil {
		return azcreds, errors.New("parseAzureSecret faulty regex: azureSubscriptionIdRegex")
	}
	for _, line := range splitString {
		if line == "" {
			continue
		}
		if keyNameRegex.MatchString(line) {
			continue
		}
		// check for storage key
		matchedStorageKey := azureStorageKeyRegex.MatchString(line)
		matchedSubscriptionId := azureSubscriptionIdRegex.MatchString(line)
		matchedTenantId := azureTenantIdRegex.MatchString(line)
		matchedCliendId := azureClientIdRegex.MatchString(line)
		matchedClientsecret := azureClientSecretRegex.MatchString(line)
		matchedResourceGroup := azureResourceGroupRegex.MatchString(line)

		switch {
		case matchedStorageKey:
			storageKeyValue, err := r.getMatchedKeyValue("AZURE_STORAGE_ACCOUNT_ACCESS_KEY", line)
			if err != nil {
				return azcreds, err
			}
			azcreds.strorageAccountKey = storageKeyValue
		case matchedSubscriptionId:
			subscriptionIdValue, err := r.getMatchedKeyValue("AZURE_SUBSCRIPTION_ID", line)
			if err != nil {
				return azcreds, err
			}
			azcreds.subscriptionID = subscriptionIdValue
		case matchedCliendId:
			clientIdValue, err := r.getMatchedKeyValue("AZURE_CLIENT_ID", line)
			if err != nil {
				return azcreds, err
			}
			azcreds.clientID = clientIdValue
		case matchedClientsecret:
			clientSecretValue, err := r.getMatchedKeyValue("AZURE_CLIENT_SECRET", line)
			if err != nil {
				return azcreds, err
			}
			azcreds.clientSecret = clientSecretValue
		case matchedResourceGroup:
			resourceGroupValue, err := r.getMatchedKeyValue("AZURE_RESOURCE_GROUP", line)
			if err != nil {
				return azcreds, err
			}
			azcreds.resourceGroup = resourceGroupValue
		case matchedTenantId:
			tenantIdValue, err := r.getMatchedKeyValue("AZURE_TENANT_ID", line)
			if err != nil {
				return azcreds, err
			}
			azcreds.tenantID = tenantIdValue
		}
	}
	return azcreds, nil
}

// Return value to the right of = sign with quotations and spaces removed.
func (r *DPAReconciler) getMatchedKeyValue(key string, s string) (string, error) {
	for _, removeChar := range []string{"\"", "'", " "} {
		s = strings.ReplaceAll(s, removeChar, "")
	}
	for _, prefix := range []string{key, "="} {
		s = strings.TrimPrefix(s, prefix)
	}
	if len(s) == 0 {
		r.Log.Info("Could not parse secret for %s", key)
		return s, errors.New(key + " secret parsing error")
	}
	return s, nil
}

func (r *DPAReconciler) ReconcileRegistrySVCs(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	// fetch the bsl instances
	bslList := velerov1.BackupStorageLocationList{}
	if err := r.List(r.Context, &bslList, &client.ListOptions{
		Namespace: r.NamespacedName.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app.kubernetes.io/component": "bsl",
		}),
	}); err != nil {
		return false, err
	}

	// Now for each of these bsl instances, delete any existing service
	if len(bslList.Items) > 0 {
		for _, bsl := range bslList.Items {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry-svc",
					Namespace: r.NamespacedName.Namespace,
				},
			}
			deleteContext := context.Background()
			if err := r.Get(deleteContext, types.NamespacedName{
				Name:      svc.Name,
				Namespace: r.NamespacedName.Namespace,
			}, &svc); err != nil {
				if k8serror.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}
			deleteOptionPropagationForeground := metav1.DeletePropagationForeground
			if err := r.Delete(deleteContext, &svc, &client.DeleteOptions{PropagationPolicy: &deleteOptionPropagationForeground}); err != nil {
				r.EventRecorder.Event(&svc, corev1.EventTypeNormal, "DeleteRegistryServiceFailed", fmt.Sprintf("Could not delete registry service %s from %s:"+err.Error(), svc.Name, svc.Namespace))
				return false, err
			}
			r.EventRecorder.Event(&svc, corev1.EventTypeNormal, "DeletedRegistryService", fmt.Sprintf("Registry service %s deleted from %s", svc.Name, svc.Namespace))
		}
	}

	return true, nil
}

func (r *DPAReconciler) ReconcileRegistryRoutes(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	// fetch the bsl instances
	bslList := velerov1.BackupStorageLocationList{}
	if err := r.List(r.Context, &bslList, &client.ListOptions{
		Namespace: r.NamespacedName.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app.kubernetes.io/component": "bsl",
		}),
	}); err != nil {
		return false, err
	}

	// Now for each of these bsl instances, create a route
	if len(bslList.Items) > 0 {
		for _, bsl := range bslList.Items {
			route := routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry-route",
					Namespace: r.NamespacedName.Namespace,
				},
				Spec: routev1.RouteSpec{
					To: routev1.RouteTargetReference{
						Kind: "Service",
						Name: "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry-svc",
					},
				},
			}

			deleteContext := context.Background()
			if err := r.Get(deleteContext, types.NamespacedName{
				Name:      route.Name,
				Namespace: r.NamespacedName.Namespace,
			}, &route); err != nil {
				if k8serror.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}

			deleteOptionPropagationForeground := metav1.DeletePropagationForeground
			if err := r.Delete(deleteContext, &route, &client.DeleteOptions{PropagationPolicy: &deleteOptionPropagationForeground}); err != nil {
				r.EventRecorder.Event(&route, corev1.EventTypeNormal, "DeleteRegistryRouteFailed", fmt.Sprintf("Could not delete registry route %s from %s:"+err.Error(), route.Name, route.Namespace))
				return false, err
			}
			r.EventRecorder.Event(&route, corev1.EventTypeNormal, "DeletedRegistryRoute", fmt.Sprintf("Registry route %s deleted from %s", route.Name, route.Namespace))
		}
	}

	return true, nil
}

func (r *DPAReconciler) ReconcileRegistryRouteConfigs(log logr.Logger) (bool, error) {

	// Now for each of these bsl instances, create a registry route cm for each of them
	registryRouteCM := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "oadp-registry-config",
			Namespace: r.NamespacedName.Namespace,
		},
	}

	deleteContext := context.Background()
	if err := r.Get(deleteContext, types.NamespacedName{
		Name:      registryRouteCM.Name,
		Namespace: r.NamespacedName.Namespace,
	}, &registryRouteCM); err != nil {
		if k8serror.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}

	deleteOptionPropagationForeground := metav1.DeletePropagationForeground
	if err := r.Delete(deleteContext, &registryRouteCM, &client.DeleteOptions{PropagationPolicy: &deleteOptionPropagationForeground}); err != nil {
		r.EventRecorder.Event(&registryRouteCM, corev1.EventTypeNormal, "DeleteRegistryConfigMapFailed", fmt.Sprintf("Could not delete registry configmap %s from %s:"+err.Error(), registryRouteCM.Name, registryRouteCM.Namespace))
		return false, err
	}
	r.EventRecorder.Event(&registryRouteCM, corev1.EventTypeNormal, "DeletedRegistryConfigMap", fmt.Sprintf("Registry configmap %s deleted from %s", registryRouteCM.Name, registryRouteCM.Namespace))

	return true, nil
}

// Create secret for registry to be parsed by openshift-velero-plugin
func (r *DPAReconciler) ReconcileRegistrySecrets(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	// fetch the bsl instances
	bslList := velerov1.BackupStorageLocationList{}
	if err := r.List(r.Context, &bslList, &client.ListOptions{
		Namespace: r.NamespacedName.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app.kubernetes.io/component": "bsl",
		}),
	}); err != nil {
		return false, err
	}

	// Now for each of these bsl instances, create a registry secret
	for _, bsl := range bslList.Items {
		// skip for GCP as nothing is directly exposed in env vars
		if bsl.Spec.Provider == GCPProvider {
			continue
		}
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry-secret",
				Namespace: r.NamespacedName.Namespace,
				Labels: map[string]string{
					oadpv1alpha1.OadpOperatorLabel: "True",
				},
			},
		}

		if !dpa.BackupImages() {
			deleteContext := context.Background()
			if err := r.Get(deleteContext, types.NamespacedName{
				Name:      secret.Name,
				Namespace: r.NamespacedName.Namespace,
			}, &secret); err != nil {
				if k8serror.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}

			deleteOptionPropagationForeground := metav1.DeletePropagationForeground
			if err := r.Delete(deleteContext, &secret, &client.DeleteOptions{PropagationPolicy: &deleteOptionPropagationForeground}); err != nil {
				r.EventRecorder.Event(&secret, corev1.EventTypeNormal, "DeleteRegistrySecretFailed", fmt.Sprintf("Could not delete registry secret %s from %s:"+err.Error(), secret.Name, secret.Namespace))
				return false, err
			}
			r.EventRecorder.Event(&secret, corev1.EventTypeNormal, "DeletedRegistrySecret", fmt.Sprintf("Registry secret %s deleted from %s", secret.Name, secret.Namespace))

			return true, nil
		}

		// Create Secret
		op, err := controllerutil.CreateOrPatch(r.Context, r.Client, &secret, func() error {
			// TODO: check for secret status condition errors and respond here
			err := r.patchRegistrySecret(&secret, &bsl, &dpa)

			return err
		})
		if err != nil {
			return false, err
		}
		if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
			// Trigger event to indicate Secret was created or updated
			r.EventRecorder.Event(&secret,
				corev1.EventTypeNormal,
				"RegistrySecretsReconciled",
				fmt.Sprintf("performed %s on secret %s/%s", op, secret.Namespace, secret.Name),
			)
		}
	}

	return true, nil
}

func (r *DPAReconciler) patchRegistrySecret(secret *corev1.Secret, bsl *velerov1.BackupStorageLocation, dpa *oadpv1alpha1.DataProtectionApplication) error {
	// Setting controller owner reference on the registry secret
	err := controllerutil.SetControllerReference(dpa, secret, r.Scheme)
	if err != nil {
		return err
	}

	// when updating the spec fields we update each field individually
	// to get around the immutable fields
	provider := bsl.Spec.Provider
	switch provider {
	case AWSProvider:
		err = r.populateAWSRegistrySecret(bsl, secret)
	case AzureProvider:
		err = r.populateAzureRegistrySecret(bsl, secret)
	}

	if err != nil {
		return err
	}

	return nil
}

func (r *DPAReconciler) populateAWSRegistrySecret(bsl *velerov1.BackupStorageLocation, registrySecret *corev1.Secret) error {
	// Check for secret name
	secretName, secretKey := r.getSecretNameAndKey(&bsl.Spec, oadpv1alpha1.DefaultPluginAWS)

	// fetch secret and error
	secret, err := r.getProviderSecret(secretName)
	if err != nil {
		r.Log.Info(fmt.Sprintf("Error fetching provider secret %s for backupstoragelocation %s/%s", secretName, bsl.Namespace, bsl.Name))
		return err
	}
	awsProfile := "default"
	if value, exists := bsl.Spec.Config[Profile]; exists {
		awsProfile = value
	}
	// parse the secret and get aws access_key and aws secret_key
	AWSAccessKey, AWSSecretKey, err := r.parseAWSSecret(secret, secretKey, awsProfile)
	if err != nil {
		r.Log.Info(fmt.Sprintf("Error parsing provider secret %s for backupstoragelocation %s/%s", secretName, bsl.Namespace, bsl.Name))
		return err
	}

	registrySecret.Data = map[string][]byte{
		"access_key": []byte(AWSAccessKey),
		"secret_key": []byte(AWSSecretKey),
	}

	return nil
}

func (r *DPAReconciler) populateAzureRegistrySecret(bsl *velerov1.BackupStorageLocation, registrySecret *corev1.Secret) error {
	// Check for secret name
	secretName, secretKey := r.getSecretNameAndKey(&bsl.Spec, oadpv1alpha1.DefaultPluginMicrosoftAzure)

	// fetch secret and error
	secret, err := r.getProviderSecret(secretName)
	if err != nil {
		r.Log.Info(fmt.Sprintf("Error fetching provider secret %s for backupstoragelocation %s/%s", secretName, bsl.Namespace, bsl.Name))
		return err
	}

	// parse the secret and get azure storage account key
	azcreds, err := r.parseAzureSecret(secret, secretKey)
	if err != nil {
		r.Log.Info(fmt.Sprintf("Error parsing provider secret %s for backupstoragelocation %s/%s", secretName, bsl.Namespace, bsl.Name))
		return err
	}

	if len(bsl.Spec.Config["storageAccountKeyEnvVar"]) != 0 {
		if azcreds.strorageAccountKey == "" {
			r.Log.Info("Expecting storageAccountKeyEnvVar value set present in the credentials")
			return errors.New("no strorageAccountKey value present in credentials file")
		}
	} else {
		if len(azcreds.subscriptionID) == 0 &&
			len(azcreds.tenantID) == 0 &&
			len(azcreds.clientID) == 0 &&
			len(azcreds.clientSecret) == 0 &&
			len(azcreds.resourceGroup) == 0 {
			return errors.New("error finding service principal parameters for the supplied Azure credential")
		}
	}

	registrySecret.Data = map[string][]byte{
		"storage_account_key": []byte(azcreds.strorageAccountKey),
		"subscription_id_key": []byte(azcreds.subscriptionID),
		"tenant_id_key":       []byte(azcreds.tenantID),
		"client_id_key":       []byte(azcreds.clientID),
		"client_secret_key":   []byte(azcreds.clientSecret),
		"resource_group_key":  []byte(azcreds.resourceGroup),
	}

	return nil
}

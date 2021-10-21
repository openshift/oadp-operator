package controllers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

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
	RegistryStorageAzureContainerEnvVarKey   = "REGISTRY_STORAGE_AZURE_CONTAINER"
	RegistryStorageAzureAccountnameEnvVarKey = "REGISTRY_STORAGE_AZURE_ACCOUNTNAME"
	RegistryStorageAzureAccountkeyEnvVarKey  = "REGISTRY_STORAGE_AZURE_ACCOUNTKEY"
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
	S3URL                 = "s3Url"
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

func (r *VeleroReconciler) ReconcileRegistries(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	bslLabels := map[string]string{
		"app.kubernetes.io/name":       "oadp-operator-velero",
		"app.kubernetes.io/managed-by": "oadp-operator",
		"app.kubernetes.io/component":  "bsl",
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

		if velero.Spec.BackupImages != nil && !*velero.Spec.BackupImages {
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
				r.EventRecorder.Event(registryDeployment, corev1.EventTypeNormal, "DeleteRegistryDeploymentFailed", "Could not delete registry deployment:"+err.Error())
				return false, err
			}
			r.EventRecorder.Event(registryDeployment, corev1.EventTypeNormal, "DeletedRegistryDeployment", "Registry Deployment deleted")

			return true, nil
		}

		op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, registryDeployment, func() error {

			// Setting Registry Deployment selector if a new object is created as it is immutable
			if registryDeployment.ObjectMeta.CreationTimestamp.IsZero() {
				registryDeployment.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"component": registryName(&bsl),
					},
				}
			}

			err := controllerutil.SetControllerReference(&velero, registryDeployment, r.Scheme)
			if err != nil {
				return err
			}
			// update the Registry Deployment template
			err = r.buildRegistryDeployment(registryDeployment, &bsl, &velero)
			return err
		})

		if err != nil {
			return false, err
		}

		//TODO: Review registry deployment status and report errors and conditions

		if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
			// Trigger event to indicate registry deployment was created or updated
			r.EventRecorder.Event(registryDeployment,
				corev1.EventTypeNormal,
				"RegistryDeploymentReconciled",
				fmt.Sprintf("performed %s on registry deployment %s/%s", op, registryDeployment.Namespace, registryDeployment.Name),
			)
		}

	}

	return true, nil
}

// Construct and update the registry deployment for a bsl
func (r *VeleroReconciler) buildRegistryDeployment(registryDeployment *appsv1.Deployment, bsl *velerov1.BackupStorageLocation, velero *oadpv1alpha1.Velero) error {

	// Build registry container
	registryContainer, err := r.buildRegistryContainer(bsl, velero)
	if err != nil {
		return err
	}
	// Setting controller owner reference on the registry deployment
	registryDeployment.Labels = r.getRegistryBSLLabels(bsl)

	registryDeployment.Spec = appsv1.DeploymentSpec{
		Selector: registryDeployment.Spec.Selector,
		Replicas: pointer.Int32(1),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"component": registryName(bsl),
				},
				Annotations: velero.Spec.PodAnnotations,
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyAlways,
				Containers:    registryContainer,
			},
		},
	}

	// attach gcp secret volume if provider is gcp
	if bsl.Spec.Provider == GCPProvider {
		registryDeployment.Spec.Template.Spec.Volumes = []corev1.Volume{
			{
				Name: credentials.PluginSpecificFields[oadpv1alpha1.DefaultPluginGCP].SecretName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: credentials.PluginSpecificFields[oadpv1alpha1.DefaultPluginGCP].SecretName,
					},
				},
			},
		}
	}

	// attach DNS policy and config if enabled
	registryDeployment.Spec.Template.Spec.DNSPolicy = velero.Spec.PodDnsPolicy
	if !reflect.DeepEqual(velero.Spec.PodDnsConfig, corev1.PodDNSConfig{}) {
		registryDeployment.Spec.Template.Spec.DNSConfig = &velero.Spec.PodDnsConfig
	}

	return nil
}

func (r *VeleroReconciler) getRegistryBSLLabels(bsl *velerov1.BackupStorageLocation) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name":       common.OADPOperatorVelero,
		"app.kubernetes.io/instance":   registryName(bsl),
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  Registry,
		oadpv1alpha1.OadpOperatorLabel: "True",
	}
	return labels
}

func registryName(bsl *velerov1.BackupStorageLocation) string {
	return "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry"
}

func getRegistryImage(velero *oadpv1alpha1.Velero) string {
	if velero.Spec.UnsupportedOverrides[oadpv1alpha1.RegistryImageKey] != "" {
		return velero.Spec.UnsupportedOverrides[oadpv1alpha1.RegistryImageKey]
	}
	if os.Getenv("VELERO_REGISTRY_REPO") == "" {
		return common.RegistryImage
	}
	return fmt.Sprintf("%v/%v/%v:%v", os.Getenv("REGISTRY"), os.Getenv("PROJECT"), os.Getenv("VELERO_REGISTRY_REPO"), os.Getenv("VELERO_REGISTRY_TAG"))
}

func (r *VeleroReconciler) buildRegistryContainer(bsl *velerov1.BackupStorageLocation, velero *oadpv1alpha1.Velero) ([]corev1.Container, error) {
	envVars, err := r.getRegistryEnvVars(bsl)
	if err != nil {
		r.Log.Info(fmt.Sprintf("Error building registry container for backupstoragelocation %s/%s, could not fetch registry env vars", bsl.Namespace, bsl.Name))
		return nil, err
	}
	containers := []corev1.Container{
		{
			Image: getRegistryImage(velero),
			Name:  registryName(bsl) + "-container",
			Ports: []corev1.ContainerPort{
				{
					ContainerPort: 5000,
					Protocol:      corev1.ProtocolTCP,
				},
			},
			Env: envVars,
			LivenessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v2/_catalog?n=5",
						Port: intstr.IntOrString{IntVal: 5000},
					},
				},
				PeriodSeconds:       5,
				TimeoutSeconds:      3,
				InitialDelaySeconds: 15,
			},
			ReadinessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v2/_catalog?n=5",
						Port: intstr.IntOrString{IntVal: 5000},
					},
				},
				PeriodSeconds:       5,
				TimeoutSeconds:      3,
				InitialDelaySeconds: 15,
			},
		},
	}

	// append secret volumes if the BSL provider is GCP
	if bsl.Spec.Provider == GCPProvider {
		containers[0].VolumeMounts = []corev1.VolumeMount{
			{
				Name:      credentials.PluginSpecificFields[oadpv1alpha1.DefaultPluginGCP].SecretName,
				MountPath: credentials.PluginSpecificFields[oadpv1alpha1.DefaultPluginGCP].MountPath,
			},
		}
	}

	return containers, nil
}

func (r *VeleroReconciler) getRegistryEnvVars(bsl *velerov1.BackupStorageLocation) ([]corev1.EnvVar, error) {
	envVar := []corev1.EnvVar{}
	provider := bsl.Spec.Provider
	var err error
	switch provider {
	case AWSProvider:
		envVar, err = r.getAWSRegistryEnvVars(bsl, cloudProviderEnvVarMap[AWSProvider])

	case AzureProvider:
		envVar, err = r.getAzureRegistryEnvVars(bsl, cloudProviderEnvVarMap[AzureProvider])

	case GCPProvider:
		envVar, err = r.getGCPRegistryEnvVars(bsl, cloudProviderEnvVarMap[GCPProvider])
	}
	if err != nil {
		return nil, err
	}
	return envVar, nil
}

func (r *VeleroReconciler) getAWSRegistryEnvVars(bsl *velerov1.BackupStorageLocation, awsEnvVars []corev1.EnvVar) ([]corev1.EnvVar, error) {
	// Check for secret name
	secretName, secretKey := r.getSecretNameAndKey(bsl.Spec.Credential, oadpv1alpha1.DefaultPluginAWS)

	// fetch secret and error
	secret, err := r.getProviderSecret(secretName)
	if err != nil {
		r.Log.Info(fmt.Sprintf("Error fetching provider secret %s for backupstoragelocation %s/%s", secretName, bsl.Namespace, bsl.Name))
		return nil, err
	}

	// parse the secret and get aws access_key and aws secret_key
	AWSAccessKey, AWSSecretKey, err := r.parseAWSSecret(secret, secretKey)
	if err != nil {
		r.Log.Info(fmt.Sprintf("Error parsing provider secret %s for backupstoragelocation %s/%s", secretName, bsl.Namespace, bsl.Name))
		return nil, err
	}

	for i := range awsEnvVars {
		if awsEnvVars[i].Name == RegistryStorageS3AccesskeyEnvVarKey {
			awsEnvVars[i].Value = AWSAccessKey
		}

		if awsEnvVars[i].Name == RegistryStorageS3BucketEnvVarKey {
			awsEnvVars[i].Value = bsl.Spec.StorageType.ObjectStorage.Bucket
		}

		if awsEnvVars[i].Name == RegistryStorageS3RegionEnvVarKey {
			awsEnvVars[i].Value = bsl.Spec.Config[Region]
		}

		if awsEnvVars[i].Name == RegistryStorageS3SecretkeyEnvVarKey {
			awsEnvVars[i].Value = AWSSecretKey
		}

		if awsEnvVars[i].Name == RegistryStorageS3RegionendpointEnvVarKey && bsl.Spec.Config[S3URL] != "" {
			awsEnvVars[i].Value = bsl.Spec.Config[S3URL]
		}

		if awsEnvVars[i].Name == RegistryStorageS3SkipverifyEnvVarKey && bsl.Spec.Config[InsecureSkipTLSVerify] != "" {
			awsEnvVars[i].Value = bsl.Spec.Config[InsecureSkipTLSVerify]
		}
	}
	return awsEnvVars, nil
}

func (r *VeleroReconciler) getAzureRegistryEnvVars(bsl *velerov1.BackupStorageLocation, azureEnvVars []corev1.EnvVar) ([]corev1.EnvVar, error) {
	// Check for secret name
	secretName, secretKey := r.getSecretNameAndKey(bsl.Spec.Credential, oadpv1alpha1.DefaultPluginMicrosoftAzure)
	r.Log.Info(fmt.Sprintf("Azure secret name: %s and secret key: %s", secretName, secretKey))

	// fetch secret and error
	secret, err := r.getProviderSecret(secretName)
	if err != nil {
		r.Log.Info(fmt.Sprintf("Error fetching provider secret %s for backupstoragelocation %s/%s", secretName, bsl.Namespace, bsl.Name))
		return nil, err
	}

	// parse the secret and get azure storage account key
	AzureStorageKey, err := r.parseAzureSecret(secret, secretKey)
	if err != nil {
		r.Log.Info(fmt.Sprintf("Error parsing provider secret %s for backupstoragelocation %s/%s", secretName, bsl.Namespace, bsl.Name))
		return nil, err
	}

	for i := range azureEnvVars {
		if azureEnvVars[i].Name == RegistryStorageAzureContainerEnvVarKey {
			azureEnvVars[i].Value = bsl.Spec.StorageType.ObjectStorage.Bucket
		}

		if azureEnvVars[i].Name == RegistryStorageAzureAccountnameEnvVarKey {
			azureEnvVars[i].Value = bsl.Spec.Config[StorageAccount]
		}

		if azureEnvVars[i].Name == RegistryStorageAzureAccountkeyEnvVarKey {
			azureEnvVars[i].Value = AzureStorageKey
		}

	}
	return azureEnvVars, nil
}

func (r *VeleroReconciler) getGCPRegistryEnvVars(bsl *velerov1.BackupStorageLocation, gcpEnvVars []corev1.EnvVar) ([]corev1.EnvVar, error) {
	for i := range gcpEnvVars {
		if gcpEnvVars[i].Name == RegistryStorageGCSBucket {
			gcpEnvVars[i].Value = bsl.Spec.StorageType.ObjectStorage.Bucket
		}

		if gcpEnvVars[i].Name == RegistryStorageGCSKeyfile {
			gcpEnvVars[i].Value = credentials.PluginSpecificFields[oadpv1alpha1.DefaultPluginGCP].MountPath + "/" + credentials.PluginSpecificFields[oadpv1alpha1.DefaultPluginGCP].PluginSecretKey
		}
	}
	return gcpEnvVars, nil
}

func (r *VeleroReconciler) getProviderSecret(secretName string) (corev1.Secret, error) {

	secret := corev1.Secret{}
	key := types.NamespacedName{
		Name:      secretName,
		Namespace: r.NamespacedName.Namespace,
	}
	err := r.Get(r.Context, key, &secret)

	if err != nil {
		return secret, err
	}
	r.Log.Info(fmt.Sprintf("got provider secret name: %s", secret.Name))
	return secret, nil
}

func (r *VeleroReconciler) getSecretNameAndKey(credential *corev1.SecretKeySelector, plugin oadpv1alpha1.DefaultPlugin) (string, string) {
	// Assume default values unless user has overriden them
	secretName := credentials.PluginSpecificFields[plugin].SecretName
	secretKey := credentials.PluginSpecificFields[plugin].PluginSecretKey

	// check if user specified the Credential Name and Key
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

func (r *VeleroReconciler) parseAWSSecret(secret corev1.Secret, secretKey string) (string, string, error) {

	AWSAccessKey, AWSSecretKey := "", ""
	// this logic only supports single profile presence in the aws credentials file
	splitString := strings.Split(string(secret.Data[secretKey]), "\n")
	keyNameRegex, err := regexp.Compile(`\[.*\]`) //ignore lines such as [default]
	if err != nil {
		return AWSAccessKey, AWSSecretKey, errors.New("parseAWSSecret faulty regex: keyNameRegex")
	}
	awsAccessKeyRegex, err := regexp.Compile(`\baws_access_key_id\b`)
	if err != nil {
		return AWSAccessKey, AWSSecretKey, errors.New("parseAWSSecret faulty regex: awsAccessKeyRegex")
	}
	awsSecretKeyRegex, err := regexp.Compile(`\baws_secret_access_key\b`)
	if err != nil {
		return AWSAccessKey, AWSSecretKey, errors.New("parseAWSSecret faulty regex: awsSecretKeyRegex")
	}
	for _, line := range splitString {
		if line == "" {
			continue
		}
		if keyNameRegex.MatchString(line) {
			continue
		}
		// check for access key
		matchedAccessKey := awsAccessKeyRegex.MatchString(line)

		if err != nil {
			r.Log.Info("Error finding access key id for the supplied AWS credential")
			return AWSAccessKey, AWSSecretKey, err
		}

		if matchedAccessKey {
			cleanedLine := strings.ReplaceAll(line, " ", "")
			splitLine := strings.Split(cleanedLine, "=")
			if len(splitLine) != 2 {
				r.Log.Info("Could not parse secret for AWS Access key")
				return AWSAccessKey, AWSSecretKey, errors.New("secret parsing error")
			}
			AWSAccessKey = splitLine[1]
			continue
		}

		// check for secret key
		matchedSecretKey := awsSecretKeyRegex.MatchString(line)

		if matchedSecretKey {
			cleanedLine := strings.ReplaceAll(line, " ", "")
			splitLine := strings.Split(cleanedLine, "=")
			if len(splitLine) != 2 {
				r.Log.Info("Could not parse secret for AWS Secret key")
				return AWSAccessKey, AWSSecretKey, errors.New("secret parsing error")
			}
			AWSSecretKey = splitLine[1]
			continue
		}
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

func (r *VeleroReconciler) parseAzureSecret(secret corev1.Secret, secretKey string) (string, error) {

	AzureStorageKey := ""
	// this logic only supports single profile presence in the azure credentials file
	// current support for only usage of storage account access key in credentials file, need to add logic for other options
	splitString := strings.Split(string(secret.Data[secretKey]), "\n")
	keyNameRegex, err := regexp.Compile(`\[.*\]`) //ignore lines such as [default]
	if err != nil {
		return AzureStorageKey, errors.New("parseAzureSecret faulty regex: keyNameRegex")
	}
	azureStorageKeyRegex, err := regexp.Compile(`\bAZURE_STORAGE_ACCOUNT_ACCESS_KEY\b`)
	if err != nil {
		return AzureStorageKey, errors.New("parseAzureSecret faulty regex: azureStorageKeyRegex")
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

		if err != nil {
			r.Log.Info("Error finding storage key for the supplied Azure credential")
			return AzureStorageKey, err
		}

		if matchedStorageKey {
			cleanedLine := strings.ReplaceAll(line, " ", "")
			storageKeyValue := strings.Replace(cleanedLine, "AZURE_STORAGE_ACCOUNT_ACCESS_KEY=", "", -1)
			if len(storageKeyValue) == 0 {
				r.Log.Info("Could not parse secret for Azure Storage key")
				return AzureStorageKey, errors.New("azure secret parsing error")
			}
			AzureStorageKey = storageKeyValue
			r.Log.Info(fmt.Sprintf("Azure storage key value after parsing: %s", AzureStorageKey))
			continue
		}
	}
	if AzureStorageKey == "" {
		r.Log.Info("Error finding storage key for the supplied Azure credential")
		return AzureStorageKey, errors.New("error finding storage key for the supplied Azure credential")
	}

	return AzureStorageKey, nil
}

func (r *VeleroReconciler) ReconcileRegistrySVCs(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
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

	// Now for each of these bsl instances, create a service
	if len(bslList.Items) > 0 {
		for _, bsl := range bslList.Items {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry-svc",
					Namespace: r.NamespacedName.Namespace,
				},
			}

			if velero.Spec.BackupImages != nil && !*velero.Spec.BackupImages {
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
					r.EventRecorder.Event(&svc, corev1.EventTypeNormal, "DeleteRegistryServiceFailed", "Could not delete registry service:"+err.Error())
					return false, err
				}
				r.EventRecorder.Event(&svc, corev1.EventTypeNormal, "DeletedRegistryService", "Registry service deleted")

				return true, nil
			}

			// Create SVC
			op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &svc, func() error {
				// TODO: check for svc status condition errors and respond here
				err := r.updateRegistrySVC(&svc, &bsl, &velero)

				return err
			})
			if err != nil {
				return false, err
			}
			if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
				// Trigger event to indicate SVC was created or updated
				r.EventRecorder.Event(&bsl,
					corev1.EventTypeNormal,
					"RegistryServicesReconciled",
					fmt.Sprintf("performed %s on service %s/%s", op, svc.Namespace, svc.Name),
				)
			}
		}
	}

	return true, nil
}

func (r *VeleroReconciler) updateRegistrySVC(svc *corev1.Service, bsl *velerov1.BackupStorageLocation, velero *oadpv1alpha1.Velero) error {
	// Setting controller owner reference on the registry svc
	err := controllerutil.SetControllerReference(velero, svc, r.Scheme)
	if err != nil {
		return err
	}

	// when updating the spec fields we update each field individually
	// to get around the immutable fields
	svc.Spec.Selector = map[string]string{
		"component": "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry",
	}
	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name: "5000-tcp",
			Port: int32(5000),
			TargetPort: intstr.IntOrString{
				IntVal: int32(5000),
			},
			Protocol: corev1.ProtocolTCP,
		},
	}
	svc.Labels = map[string]string{
		"component": "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry",
	}
	return nil
}

func (r *VeleroReconciler) ReconcileRegistryRoutes(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
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

			if velero.Spec.BackupImages != nil && !*velero.Spec.BackupImages {
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
					r.EventRecorder.Event(&route, corev1.EventTypeNormal, "DeleteRegistryRouteFailed", "Could not delete registry route:"+err.Error())
					return false, err
				}
				r.EventRecorder.Event(&route, corev1.EventTypeNormal, "DeletedRegistryRoute", "Registry route deleted")

				return true, nil
			}

			// Create Route
			op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &route, func() error {

				// TODO: check for svc status condition errors and respond here

				err := r.updateRegistryRoute(&route, &bsl, &velero)

				return err
			})
			if err != nil {
				return false, err
			}
			if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
				// Trigger event to indicate route was created or updated
				r.EventRecorder.Event(&bsl,
					corev1.EventTypeNormal,
					"RegistryroutesReconciled",
					fmt.Sprintf("performed %s on route %s/%s", op, route.Namespace, route.Name),
				)
			}
		}
	}

	return true, nil
}

func (r *VeleroReconciler) updateRegistryRoute(route *routev1.Route, bsl *velerov1.BackupStorageLocation, velero *oadpv1alpha1.Velero) error {
	// Setting controller owner reference on the registry route
	err := controllerutil.SetControllerReference(velero, route, r.Scheme)
	if err != nil {
		return err
	}

	route.Labels = map[string]string{
		"component": "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry",
		"service":   "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry-svc",
		"track":     "registry-routes",
	}

	return nil
}

func (r *VeleroReconciler) ReconcileRegistryRouteConfigs(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
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

	// Now for each of these bsl instances, create a registry route cm for each of them
	if len(bslList.Items) > 0 {
		for _, bsl := range bslList.Items {
			registryRouteCM := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-registry-config",
					Namespace: r.NamespacedName.Namespace,
				},
			}

			if velero.Spec.BackupImages != nil && !*velero.Spec.BackupImages {
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
					r.EventRecorder.Event(&registryRouteCM, corev1.EventTypeNormal, "DeleteRegistryConfigMapFailed", "Could not delete registry configmap:"+err.Error())
					return false, err
				}
				r.EventRecorder.Event(&registryRouteCM, corev1.EventTypeNormal, "DeletedRegistryConfigMap", "Registry configmap deleted")

				return true, nil
			}

			op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &registryRouteCM, func() error {

				// update the Config Map
				err := r.updateRegistryConfigMap(&registryRouteCM, &bsl, &velero)
				return err
			})

			if err != nil {
				return false, err
			}

			//TODO: Review Registry Route CM status and report errors and conditions

			if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
				// Trigger event to indicate Registry Route CM was created or updated
				r.EventRecorder.Event(&registryRouteCM,
					corev1.EventTypeNormal,
					"ReconcileRegistryRouteConfigReconciled",
					fmt.Sprintf("performed %s on registry route config map %s/%s", op, registryRouteCM.Namespace, registryRouteCM.Name),
				)
			}
		}
	}
	return true, nil
}

func (r *VeleroReconciler) updateRegistryConfigMap(registryRouteCM *corev1.ConfigMap, bsl *velerov1.BackupStorageLocation, velero *oadpv1alpha1.Velero) error {

	// Setting controller owner reference on the restic restore helper CM
	err := controllerutil.SetControllerReference(velero, registryRouteCM, r.Scheme)
	if err != nil {
		return err
	}

	registryRouteCM.Data = map[string]string{
		bsl.Name: "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry-route",
	}

	return nil
}

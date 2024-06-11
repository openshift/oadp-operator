package controllers

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	"github.com/openshift/oadp-operator/pkg/storage/aws"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *DPAReconciler) ValidateBackupStorageLocations(dpa oadpv1alpha1.DataProtectionApplication) (bool, error) {
	// Ensure BSL is a valid configuration
	// First, check for provider and then call functions based on the cloud provider for each backupstoragelocation configured
	numDefaultLocations := 0
	for _, bslSpec := range dpa.Spec.BackupLocations {

		if err := r.ensureBackupLocationHasVeleroOrCloudStorage(&bslSpec); err != nil {
			return false, err
		}

		if err := r.ensurePrefixWhenBackupImages(&dpa, &bslSpec); err != nil {
			return false, err
		}

		if err := r.ensureSecretDataExists(&dpa, &bslSpec); err != nil {
			return false, err
		}

		if bslSpec.Velero != nil {
			if bslSpec.Velero.Default {
				numDefaultLocations++
			} else if bslSpec.Name == "default" {
				return false, fmt.Errorf("Storage location named 'default' must be set as default")
			}
			provider := bslSpec.Velero.Provider
			if len(provider) == 0 {
				return false, fmt.Errorf("no provider specified for one of the backupstoragelocations configured")
			}

			// TODO: cases might need some updates for IBM/Minio/noobaa
			switch provider {
			case AWSProvider, "velero.io/aws":
				err := r.validateAWSBackupStorageLocation(*bslSpec.Velero, &dpa)
				if err != nil {
					return false, err
				}
			case AzureProvider, "velero.io/azure":
				err := r.validateAzureBackupStorageLocation(*bslSpec.Velero, &dpa)
				if err != nil {
					return false, err
				}
			case GCPProvider, "velero.io/gcp":
				err := r.validateGCPBackupStorageLocation(*bslSpec.Velero, &dpa)
				if err != nil {
					return false, err
				}
			default:
				return false, fmt.Errorf("invalid provider")
			}
		}
		if bslSpec.CloudStorage != nil {
			// Make sure credentials are specified.
			if bslSpec.CloudStorage.Credential == nil {
				return false, fmt.Errorf("must provide a valid credential secret")
			}
			if bslSpec.CloudStorage.Credential.LocalObjectReference.Name == "" {
				return false, fmt.Errorf("must provide a valid credential secret name")
			}
			if bslSpec.CloudStorage.Default {
				numDefaultLocations++
			} else if bslSpec.Name == "default" {
				return false, fmt.Errorf("Storage location named 'default' must be set as default")
			}
		}
		if bslSpec.CloudStorage != nil && bslSpec.Velero != nil {
			return false, fmt.Errorf("must choose one of bucket or velero")
		}
	}
	if numDefaultLocations > 1 {
		return false, fmt.Errorf("Only one Storage Location be set as default")
	}
	if numDefaultLocations == 0 && !dpa.Spec.Configuration.Velero.NoDefaultBackupLocation {
		return false, errors.New("no default backupstoragelocations configured, ensure that one backupstoragelocation has been configured as the default location")
	}
	// TODO: Discuss If multiple BSLs exist, ensure we have multiple credentials

	return true, nil
}

func (r *DPAReconciler) ReconcileBackupStorageLocations(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}
	// Loop through all configured BSLs
	for i, bslSpec := range dpa.Spec.BackupLocations {
		// Create BSL as is, we can safely assume they are valid from
		// ValidateBackupStorageLocations

		// check if BSL name is specified in DPA spec
		bslName := fmt.Sprintf("%s-%d", r.NamespacedName.Name, i+1)
		if bslSpec.Name != "" {
			bslName = bslSpec.Name
		}

		bsl := velerov1.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bslName,
				Namespace: r.NamespacedName.Namespace,
			},
		}
		// Add the following labels to the bsl secret,
		//	 1. oadpApi.OadpOperatorLabel: "True"
		// 	 2. <namespace>.dataprotectionapplication: <name>
		// which in turn will be used in th elabel handler to trigger the reconciliation loop

		secretName, _ := r.getSecretNameAndKeyforBackupLocation(bslSpec)
		_, err := r.UpdateCredentialsSecretLabels(secretName, dpa.Namespace, dpa.Name)
		if err != nil {
			return false, err
		}

		// Create BSL
		op, err := controllerutil.CreateOrPatch(r.Context, r.Client, &bsl, func() error {
			// TODO: Velero may be setting controllerReference as
			// well and taking ownership. If so move this to
			// SetOwnerReference instead

			// TODO: check for BSL status condition errors and respond here
			if bslSpec.Velero != nil {
				err := r.updateBSLFromSpec(&bsl, &dpa, *bslSpec.Velero)

				return err
			}
			if bslSpec.CloudStorage != nil {
				bucket := &oadpv1alpha1.CloudStorage{}
				err := r.Get(r.Context, client.ObjectKey{Namespace: dpa.Namespace, Name: bslSpec.CloudStorage.CloudStorageRef.Name}, bucket)
				if err != nil {
					return err
				}
				err = controllerutil.SetControllerReference(&dpa, &bsl, r.Scheme)
				if err != nil {
					return err
				}
				bsl.Spec.BackupSyncPeriod = bslSpec.CloudStorage.BackupSyncPeriod
				bsl.Spec.Config = bslSpec.CloudStorage.Config
				if bucket.Spec.EnableSharedConfig != nil && *bucket.Spec.EnableSharedConfig {
					if bsl.Spec.Config == nil {
						bsl.Spec.Config = map[string]string{}
					}
					bsl.Spec.Config["enableSharedConfig"] = "true"
				}
				bsl.Spec.Credential = bslSpec.CloudStorage.Credential
				bsl.Spec.Default = bslSpec.CloudStorage.Default
				bsl.Spec.ObjectStorage = &velerov1.ObjectStorageLocation{
					Bucket: bucket.Spec.Name,
					Prefix: bslSpec.CloudStorage.Prefix,
					CACert: bslSpec.CloudStorage.CACert,
				}
				switch bucket.Spec.Provider {
				case oadpv1alpha1.AWSBucketProvider:
					bsl.Spec.Provider = AWSProvider
				case oadpv1alpha1.AzureBucketProvider:
					return fmt.Errorf("azure provider not yet supported")
				case oadpv1alpha1.GCPBucketProvider:
					return fmt.Errorf("gcp provider not yet supported")
				default:
					return fmt.Errorf("invalid provider")
				}
			}
			return nil
		})
		if err != nil {
			return false, err
		}
		if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
			// Trigger event to indicate BSL was created or updated
			r.EventRecorder.Event(&bsl,
				corev1.EventTypeNormal,
				"BackupStorageLocationReconciled",
				fmt.Sprintf("performed %s on backupstoragelocation %s/%s", op, bsl.Namespace, bsl.Name),
			)
		}
	}
	return true, nil
}

func (r *DPAReconciler) UpdateCredentialsSecretLabels(secretName string, namespace string, dpaName string) (bool, error) {
	var secret corev1.Secret
	secret, err := r.getProviderSecret(secretName)
	if err != nil {
		return false, err
	}
	if secret.Name == "" {
		return false, errors.New("secret not found")
	}
	needPatch := false
	originalSecret := secret.DeepCopy()
	if secret.Labels == nil {
		secret.Labels = make(map[string]string)
	}
	if secret.Labels[oadpv1alpha1.OadpOperatorLabel] != "True" {
		secret.Labels[oadpv1alpha1.OadpOperatorLabel] = "True"
		needPatch = true
	}
	if secret.Labels[namespace+".dataprotectionapplication"] != dpaName {
		secret.Labels[namespace+".dataprotectionapplication"] = dpaName
		needPatch = true
	}
	if needPatch {
		err = r.Client.Patch(r.Context, &secret, client.MergeFrom(originalSecret))
		if err != nil {
			return false, err
		}

		r.EventRecorder.Event(&secret, corev1.EventTypeNormal, "SecretLabelled", fmt.Sprintf("Secret %s has been labelled", secretName))
	}
	return true, nil
}

func (r *DPAReconciler) updateBSLFromSpec(bsl *velerov1.BackupStorageLocation, dpa *oadpv1alpha1.DataProtectionApplication, bslSpec velerov1.BackupStorageLocationSpec) error {
	// Set controller reference to Velero controller
	err := controllerutil.SetControllerReference(dpa, bsl, r.Scheme)
	if err != nil {
		return err
	}
	// While using Service Principal as Azure credentials, `storageAccountKeyEnvVar` value is not required to be set.
	// However, the registry deployment fails without a valid storage account key.
	// This logic prevents the registry pods from being deployed if Azure SP is used as an auth mechanism.
	registryDeployment := "True"
	if bslSpec.Provider == "azure" {
		if len(bslSpec.Config["storageAccountKeyEnvVar"]) == 0 {
			registryDeployment = "False"
		}
	}
	// The AWS SDK expects the server providing S3 blobs to remove default ports
	// (80 for HTTP and 443 for HTTPS) before calculating a signature, and not
	// all S3-compatible services do this. Remove the ports here to avoid 403
	// errors from mismatched signatures.
	if bslSpec.Provider == "aws" {
		s3Url := bslSpec.Config["s3Url"]
		if len(s3Url) > 0 {
			if s3Url, err = common.StripDefaultPorts(s3Url); err == nil {
				bslSpec.Config["s3Url"] = s3Url
			}
		}

		// Since the AWS SDK upgrade in velero-plugin-for-aws, data transfer to BSL bucket fails
		// if the chosen checksumAlgorithm doesn't work for the provider. Velero sets this to CRC32 if not
		// chosen by the user. We will set it empty string if checksumAlgorithm is not specified by the user
		// to bypass checksum calculation entirely. If your s3 provider supports checksum calculation,
		// then you should specify this value in the config.
		if _, exists := bslSpec.Config[checksumAlgorithm]; !exists {
			bslSpec.Config[checksumAlgorithm] = ""
		}

	}
	bsl.Labels = map[string]string{
		"app.kubernetes.io/name":     common.OADPOperatorVelero,
		"app.kubernetes.io/instance": bsl.Name,
		//"app.kubernetes.io/version":    "x.y.z",
		"app.kubernetes.io/managed-by":       common.OADPOperator,
		"app.kubernetes.io/component":        "bsl",
		oadpv1alpha1.OadpOperatorLabel:       "True",
		oadpv1alpha1.RegistryDeploymentLabel: registryDeployment,
	}
	bsl.Spec = bslSpec

	return nil
}

func (r *DPAReconciler) validateAWSBackupStorageLocation(bslSpec velerov1.BackupStorageLocationSpec, dpa *oadpv1alpha1.DataProtectionApplication) error {
	// validate provider plugin and secret
	err := r.validateProviderPluginAndSecret(bslSpec, dpa)
	if err != nil {
		return err
	}

	// check for bsl non-optional bsl configs and object storage
	if bslSpec.ObjectStorage == nil {
		return fmt.Errorf("object storage configuration for AWS backupstoragelocation cannot be nil")
	}

	if len(bslSpec.ObjectStorage.Bucket) == 0 {
		return fmt.Errorf("bucket name for AWS backupstoragelocation cannot be empty")
	}

	if len(bslSpec.StorageType.ObjectStorage.Prefix) == 0 && dpa.BackupImages() {
		return fmt.Errorf("prefix for AWS backupstoragelocation object storage cannot be empty. It is required for backing up images")
	}

	// BSL region is required when
	// - s3ForcePathStyle is true, because some velero processes requires region to be set and is not auto-discoverable when s3ForcePathStyle is true
	//   imagestream backup in openshift-velero-plugin now uses the same method to discover region as the rest of the velero codebase
	// - even when s3ForcePathStyle is false, some aws bucket regions may not be discoverable and the user has to set it manually
	if (bslSpec.Config == nil || len(bslSpec.Config[Region]) == 0) &&
		(bslSpec.Config != nil && bslSpec.Config[S3ForcePathStyle] == "true" || !aws.BucketRegionIsDiscoverable(bslSpec.ObjectStorage.Bucket)) {
		return fmt.Errorf("region for AWS backupstoragelocation not automatically discoverable. Please set the region in the backupstoragelocation config")
	}

	//TODO: Add minio, noobaa, local storage validations

	return nil
}

func (r *DPAReconciler) validateAzureBackupStorageLocation(bslSpec velerov1.BackupStorageLocationSpec, dpa *oadpv1alpha1.DataProtectionApplication) error {
	// validate provider plugin and secret
	err := r.validateProviderPluginAndSecret(bslSpec, dpa)
	if err != nil {
		return err
	}

	// check for bsl non-optional bsl configs and object storage
	if bslSpec.ObjectStorage == nil {
		return fmt.Errorf("object storage configuration for Azure backupstoragelocation cannot be nil")
	}

	if len(bslSpec.ObjectStorage.Bucket) == 0 {
		return fmt.Errorf("bucket name for Azure backupstoragelocation cannot be empty")
	}

	if len(bslSpec.Config[ResourceGroup]) == 0 {
		return fmt.Errorf("resourceGroup for Azure backupstoragelocation config cannot be empty")
	}

	if len(bslSpec.Config[StorageAccount]) == 0 {
		return fmt.Errorf("storageAccount for Azure backupstoragelocation config cannot be empty")
	}

	if len(bslSpec.StorageType.ObjectStorage.Prefix) == 0 && dpa.BackupImages() {
		return fmt.Errorf("prefix for Azure backupstoragelocation object storage cannot be empty. it is required for backing up images")
	}

	return nil
}

func (r *DPAReconciler) validateGCPBackupStorageLocation(bslSpec velerov1.BackupStorageLocationSpec, dpa *oadpv1alpha1.DataProtectionApplication) error {
	// validate provider plugin and secret
	err := r.validateProviderPluginAndSecret(bslSpec, dpa)
	if err != nil {
		return err
	}

	// check for bsl non-optional bsl configs and object storage
	if bslSpec.ObjectStorage == nil {
		return fmt.Errorf("object storage configuration for GCP backupstoragelocation cannot be nil")
	}

	if len(bslSpec.ObjectStorage.Bucket) == 0 {
		return fmt.Errorf("bucket name for GCP backupstoragelocation cannot be empty")
	}
	if len(bslSpec.StorageType.ObjectStorage.Prefix) == 0 && dpa.BackupImages() {
		return fmt.Errorf("prefix for GCP backupstoragelocation object storage cannot be empty. it is required for backing up images")
	}

	return nil
}

func pluginExistsInVeleroCR(configuredPlugins []oadpv1alpha1.DefaultPlugin, expectedPlugin oadpv1alpha1.DefaultPlugin) bool {
	for _, plugin := range configuredPlugins {
		if plugin == expectedPlugin {
			return true
		}
	}
	return false
}

func (r *DPAReconciler) validateProviderPluginAndSecret(bslSpec velerov1.BackupStorageLocationSpec, dpa *oadpv1alpha1.DataProtectionApplication) error {
	if dpa.Spec.Configuration.Velero.HasFeatureFlag("no-secret") {
		return nil
	}
	// check for existence of provider plugin and warn if the plugin is absent
	if !pluginExistsInVeleroCR(dpa.Spec.Configuration.Velero.DefaultPlugins, oadpv1alpha1.DefaultPlugin(bslSpec.Provider)) {
		r.Log.Info(fmt.Sprintf("%s backupstoragelocation is configured but velero plugin for %s is not present", bslSpec.Provider, bslSpec.Provider))
		//TODO: set warning condition on Velero CR
	}
	secretName, _ := r.getSecretNameAndKey(&bslSpec, oadpv1alpha1.DefaultPlugin(bslSpec.Provider))

	_, err := r.getProviderSecret(secretName)

	if err != nil {
		r.Log.Info(fmt.Sprintf("error validating %s provider secret:  %s/%s", bslSpec.Provider, r.NamespacedName.Namespace, secretName))
		return err
	}
	return nil
}

func (r *DPAReconciler) ensureBackupLocationHasVeleroOrCloudStorage(bsl *oadpv1alpha1.BackupLocation) error {
	if bsl.CloudStorage == nil && bsl.Velero == nil {
		return fmt.Errorf("BackupLocation must have velero or bucket configuration")
	}

	if bsl.CloudStorage != nil && bsl.Velero != nil {
		return fmt.Errorf("cannot have both backupstoragelocations and bucket provided for a single StorageLocation")
	}
	return nil
}

func (r *DPAReconciler) ensurePrefixWhenBackupImages(dpa *oadpv1alpha1.DataProtectionApplication, bsl *oadpv1alpha1.BackupLocation) error {

	if bsl.Velero != nil && bsl.Velero.ObjectStorage != nil && bsl.Velero.ObjectStorage.Prefix == "" && dpa.BackupImages() {
		return fmt.Errorf("BackupLocation must have velero prefix when backupImages is not set to false")
	}

	if bsl.CloudStorage != nil && bsl.CloudStorage.Prefix == "" && dpa.BackupImages() {
		return fmt.Errorf("BackupLocation must have cloud storage prefix when backupImages is not set to false")
	}

	return nil
}

func (r *DPAReconciler) ensureSecretDataExists(dpa *oadpv1alpha1.DataProtectionApplication, bsl *oadpv1alpha1.BackupLocation) error {
	// Check if the Velero feature flag 'no-secret' is not set
	if !(dpa.Spec.Configuration.Velero.HasFeatureFlag("no-secret")) {
		// Check if the user specified credential under velero
		if bsl.Velero != nil && bsl.Velero.Credential != nil {
			// Check if user specified empty credential key
			if bsl.Velero.Credential.Key == "" {
				return fmt.Errorf("Secret key specified in BackupLocation %s cannot be empty", bsl.Name)
			}
			// Check if user specified empty credential name
			if bsl.Velero.Credential.Name == "" {
				return fmt.Errorf("Secret name specified in BackupLocation %s cannot be empty", bsl.Name)
			}
		}
		// Check if the BSL secret key configured in the DPA exists with a secret data
		secretName, secretKey := r.getSecretNameAndKeyforBackupLocation(*bsl)
		bslSecret, err := r.getProviderSecret(secretName)
		if err != nil {
			return err
		}
		data, foundKey := bslSecret.Data[secretKey]
		if !foundKey || len(data) == 0 {
			return fmt.Errorf("Secret name %s is missing data for key %s", secretName, secretKey)
		}
	}
	return nil
}

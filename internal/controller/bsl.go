package controller

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	"github.com/openshift/oadp-operator/pkg/credentials"
	"github.com/openshift/oadp-operator/pkg/storage/aws"
)

func (r *DataProtectionApplicationReconciler) ValidateBackupStorageLocations() (bool, error) {
	// Ensure BSL is a valid configuration
	// First, check for provider and then call functions based on the cloud provider for each backupstoragelocation configured
	dpa := r.dpa
	numDefaultLocations := 0
	for _, bslSpec := range dpa.Spec.BackupLocations {
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
		if err := r.ensureBackupLocationHasVeleroOrCloudStorage(&bslSpec); err != nil {
			return false, err
		}

		if err := r.ensurePrefixWhenBackupImages(&bslSpec); err != nil {
			return false, err
		}

		if err := r.ensureSecretDataExists(&bslSpec); err != nil {
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
				err := r.validateAWSBackupStorageLocation(*bslSpec.Velero)
				if err != nil {
					return false, err
				}
			case AzureProvider, "velero.io/azure":
				err := r.validateAzureBackupStorageLocation(*bslSpec.Velero)
				if err != nil {
					return false, err
				}
			case GCPProvider, "velero.io/gcp":
				err := r.validateGCPBackupStorageLocation(*bslSpec.Velero)
				if err != nil {
					return false, err
				}
			default:
				return false, fmt.Errorf("invalid provider")
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

func (r *DataProtectionApplicationReconciler) ReconcileBackupStorageLocations(log logr.Logger) (bool, error) {
	dpa := r.dpa
	dpaBSLNames := []string{}

	// Loop through all configured BSLs
	for i, bslSpec := range dpa.Spec.BackupLocations {
		// Create BSL as is, we can safely assume they are valid from
		// ValidateBackupStorageLocations

		// check if BSL name is specified in DPA spec
		bslName := fmt.Sprintf("%s-%d", r.NamespacedName.Name, i+1)
		if bslSpec.Name != "" {
			bslName = bslSpec.Name
		}
		dpaBSLNames = append(dpaBSLNames, bslName)

		bsl := velerov1.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bslName,
				Namespace: r.NamespacedName.Namespace,
			},
		}
		// Add the following labels to the bsl secret,
		//	 1. oadpApi.OadpOperatorLabel: "True"
		// 	 2. dataprotectionapplication.name: <name>
		// which in turn will be used in the label handler to trigger the reconciliation loop
		var secretName string
		if bslSpec.CloudStorage != nil {
			secretName, _, _ = r.getSecretNameAndKeyFromCloudStorage(bslSpec.CloudStorage)
		}

		if bslSpec.Velero != nil {
			secretName, _, _ = r.getSecretNameAndKey(bslSpec.Velero.Config, bslSpec.Velero.Credential, oadpv1alpha1.DefaultPlugin(bslSpec.Velero.Provider))
		}
		err := r.UpdateCredentialsSecretLabels(secretName, dpa.Name)
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
				err := r.updateBSLFromSpec(&bsl, *bslSpec.Velero)

				return err
			}
			if bslSpec.CloudStorage != nil {
				bucket := &oadpv1alpha1.CloudStorage{}
				err := r.Get(r.Context, client.ObjectKey{Namespace: dpa.Namespace, Name: bslSpec.CloudStorage.CloudStorageRef.Name}, bucket)
				if err != nil {
					return err
				}
				err = controllerutil.SetControllerReference(dpa, &bsl, r.Scheme)
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

	dpaBSLs := velerov1.BackupStorageLocationList{}
	dpaBSLLabels := map[string]string{
		"app.kubernetes.io/name":       common.OADPOperatorVelero,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  "bsl",
	}
	err := r.List(r.Context, &dpaBSLs, client.InNamespace(r.NamespacedName.Namespace), client.MatchingLabels(dpaBSLLabels))
	if err != nil {
		return false, err
	}
	// If current BSLs do not match the spec, delete extra BSLs
	if len(dpaBSLNames) != len(dpaBSLs.Items) {
		for _, bsl := range dpaBSLs.Items {
			if !slices.Contains(dpaBSLNames, bsl.Name) {
				if err := r.Delete(r.Context, &bsl); err != nil {
					return false, err
				}
				// Record event for BSL deletion
				r.EventRecorder.Event(&bsl,
					corev1.EventTypeNormal,
					"BackupStorageLocationDeleted",
					fmt.Sprintf("BackupStorageLocation %s created by OADP in namespace %s was deleted as it was not in DPA spec.", bsl.Name, bsl.Namespace))
			}
		}
	}

	return true, nil
}

func (r *DataProtectionApplicationReconciler) UpdateCredentialsSecretLabels(secretName string, dpaName string) error {

	var secret corev1.Secret
	secret, err := r.getProviderSecret(secretName)
	if err != nil {
		return err
	}
	if secret.Name == "" {
		return errors.New("secret not found")
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
	if secret.Labels["dataprotectionapplication.name"] != dpaName {
		secret.Labels["dataprotectionapplication.name"] = dpaName
		needPatch = true
	}
	if needPatch {
		err = r.Client.Patch(r.Context, &secret, client.MergeFrom(originalSecret))
		if err != nil {
			return err
		}

		r.EventRecorder.Event(&secret, corev1.EventTypeNormal, "SecretLabelled", fmt.Sprintf("Secret %s has been labelled", secretName))
	}
	return nil
}

func (r *DataProtectionApplicationReconciler) updateBSLFromSpec(bsl *velerov1.BackupStorageLocation, bslSpec velerov1.BackupStorageLocationSpec) error {
	// Set controller reference to Velero controller
	if err := controllerutil.SetControllerReference(r.dpa, bsl, r.Scheme); err != nil {
		return err
	}

	// Update BSL spec and registry-deployment label
	if err := common.UpdateBackupStorageLocation(bsl, bslSpec); err != nil {
		return err
	}

	// Assign labels individually
	// bsl.Labels was initialized in the common.UpdateBackupStorageLocation function
	bsl.Labels["app.kubernetes.io/name"] = common.OADPOperatorVelero
	bsl.Labels["app.kubernetes.io/instance"] = bsl.Name
	bsl.Labels["app.kubernetes.io/managed-by"] = common.OADPOperator
	bsl.Labels["app.kubernetes.io/component"] = "bsl"
	bsl.Labels[oadpv1alpha1.OadpOperatorLabel] = "True"

	return nil
}

func (r *DataProtectionApplicationReconciler) validateAWSBackupStorageLocation(bslSpec velerov1.BackupStorageLocationSpec) error {
	// validate provider plugin and secret
	err := r.validateProviderPluginAndSecret(bslSpec)
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

	if len(bslSpec.StorageType.ObjectStorage.Prefix) == 0 && r.dpa.BackupImages() {
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

func (r *DataProtectionApplicationReconciler) validateAzureBackupStorageLocation(bslSpec velerov1.BackupStorageLocationSpec) error {
	// validate provider plugin and secret
	err := r.validateProviderPluginAndSecret(bslSpec)
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

	if len(bslSpec.StorageType.ObjectStorage.Prefix) == 0 && r.dpa.BackupImages() {
		return fmt.Errorf("prefix for Azure backupstoragelocation object storage cannot be empty. it is required for backing up images")
	}

	return nil
}

func (r *DataProtectionApplicationReconciler) validateGCPBackupStorageLocation(bslSpec velerov1.BackupStorageLocationSpec) error {
	// validate provider plugin and secret
	err := r.validateProviderPluginAndSecret(bslSpec)
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
	if len(bslSpec.StorageType.ObjectStorage.Prefix) == 0 && r.dpa.BackupImages() {
		return fmt.Errorf("prefix for GCP backupstoragelocation object storage cannot be empty. it is required for backing up images")
	}

	return nil
}

func pluginExistsInVeleroCR(configuredPlugins []oadpv1alpha1.DefaultPlugin, expectedProvider string) bool {
	for _, plugin := range configuredPlugins {
		if credentials.PluginSpecificFields[plugin].ProviderName == expectedProvider {
			return true
		}
	}
	return false
}

func (r *DataProtectionApplicationReconciler) validateProviderPluginAndSecret(bslSpec velerov1.BackupStorageLocationSpec) error {
	if r.dpa.Spec.Configuration.Velero.HasFeatureFlag("no-secret") {
		return nil
	}
	// check for existence of provider plugin and warn if the plugin is absent
	if !pluginExistsInVeleroCR(r.dpa.Spec.Configuration.Velero.DefaultPlugins, bslSpec.Provider) {
		r.Log.Info(fmt.Sprintf("%s backupstoragelocation is configured but velero plugin for %s is not present", bslSpec.Provider, bslSpec.Provider))
		//TODO: set warning condition on Velero CR
	}
	secretName, _, _ := r.getSecretNameAndKey(bslSpec.Config, bslSpec.Credential, oadpv1alpha1.DefaultPlugin(bslSpec.Provider))

	_, err := r.getProviderSecret(secretName)

	if err != nil {
		r.Log.Info(fmt.Sprintf("error validating %s provider secret:  %s/%s", bslSpec.Provider, r.NamespacedName.Namespace, secretName))
		return err
	}
	return nil
}

func (r *DataProtectionApplicationReconciler) ensureBackupLocationHasVeleroOrCloudStorage(bsl *oadpv1alpha1.BackupLocation) error {
	if bsl.CloudStorage == nil && bsl.Velero == nil {
		return fmt.Errorf("BackupLocation must have velero or bucket configuration")
	}

	if bsl.CloudStorage != nil && bsl.Velero != nil {
		return fmt.Errorf("cannot have both backupstoragelocations and bucket provided for a single StorageLocation")
	}
	return nil
}

func (r *DataProtectionApplicationReconciler) ensurePrefixWhenBackupImages(bsl *oadpv1alpha1.BackupLocation) error {

	if bsl.Velero != nil && bsl.Velero.ObjectStorage != nil && bsl.Velero.ObjectStorage.Prefix == "" && r.dpa.BackupImages() {
		return fmt.Errorf("BackupLocation must have velero prefix when backupImages is not set to false")
	}

	if bsl.CloudStorage != nil && bsl.CloudStorage.Prefix == "" && r.dpa.BackupImages() {
		return fmt.Errorf("BackupLocation must have cloud storage prefix when backupImages is not set to false")
	}

	return nil
}

func (r *DataProtectionApplicationReconciler) ensureSecretDataExists(bsl *oadpv1alpha1.BackupLocation) error {
	// Don't check if the Velero feature flag 'no-secret' is set
	if r.dpa.Spec.Configuration.Velero.HasFeatureFlag("no-secret") {
		return nil
	}

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

	// Extract secret name, key, provider and profile outside the if blocks
	var secretName, secretKey, provider string
	var err error
	awsProfile := "default"

	// Get secret details from either CloudStorage or Velero
	if bsl.CloudStorage != nil {
		secretName, secretKey, err = r.getSecretNameAndKeyFromCloudStorage(bsl.CloudStorage)
		if err != nil {
			return err
		}

		// Get provider type from CloudStorage
		if bsl.CloudStorage.CloudStorageRef.Name != "" {
			bucket := &oadpv1alpha1.CloudStorage{}
			err := r.Get(r.Context, client.ObjectKey{Namespace: r.dpa.Namespace, Name: bsl.CloudStorage.CloudStorageRef.Name}, bucket)
			if err != nil {
				return err
			} else {
				provider = string(bucket.Spec.Provider)
			}
		}

		// Get AWS profile if specified
		if bsl.CloudStorage.Config != nil {
			if value, exists := bsl.CloudStorage.Config[Profile]; exists {
				awsProfile = value
			}
		}
	} else if bsl.Velero != nil {
		secretName, secretKey, err = r.getSecretNameAndKey(bsl.Velero.Config, bsl.Velero.Credential, oadpv1alpha1.DefaultPlugin(bsl.Velero.Provider))
		if err != nil {
			return err
		}

		// Get provider type from Velero
		provider = bsl.Velero.Provider

		// Get AWS profile if specified
		if bsl.Velero.Config != nil {
			if value, exists := bsl.Velero.Config[Profile]; exists {
				awsProfile = value
			}
		}
	}

	// Skip secret parsing if no secret name is provided
	if secretName == "" {
		return nil
	}

	// Get the secret, this also ensure secret referenced exists.
	secret, err := r.getProviderSecret(secretName)
	if err != nil {
		return err
	}
	// Only parse secrets when backupImages is true
	if !r.dpa.BackupImages() {
		return nil
	}
	// Parse the secret based on provider type
	switch {
	case provider == AWSProvider || strings.Contains(provider, "aws"):
		_, _, err = r.parseAWSSecret(secret, secretKey, awsProfile)
		if err != nil {
			return fmt.Errorf("error parsing AWS secret %s: %v", secretName, err)
		}
	case provider == AzureProvider || strings.Contains(provider, "azure"):
		_, err = r.parseAzureSecret(secret, secretKey)
		if err != nil {
			return fmt.Errorf("error parsing Azure secret %s: %v", secretName, err)
		}
	}

	return nil
}

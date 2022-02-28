package controllers

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *DPAReconciler) ValidateBackupStorageLocations(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}
	if dpa.Spec.Configuration == nil || dpa.Spec.Configuration.Velero == nil {
		return false, errors.New("DPA CR Velero configuration cannot be nil")
	}
	// Ensure we have a BSL or user has specified NoDefaultBackupLocation install
	if len(dpa.Spec.BackupLocations) == 0 && !dpa.Spec.Configuration.Velero.NoDefaultBackupLocation {
		return false, errors.New("no backupstoragelocations configured, ensure a backupstoragelocation has been configured")
	}

	// Ensure BSL:Provider has a 1:1 mapping
	if err := r.ensureBSLProviderMapping(&dpa); err != nil {
		return false, err
	}

	// Ensure BSL is a valid configuration
	// First, check for provider and then call functions based on the cloud provider for each backupstoragelocation configured
	for _, bslSpec := range dpa.Spec.BackupLocations {
		if bslSpec.Velero != nil {
			provider := bslSpec.Velero.Provider
			if len(provider) == 0 {
				return false, fmt.Errorf("no provider specified for one of the backupstoragelocations configured")
			}

			// TODO: cases might need some updates for IBM/Minio/noobaa
			switch common.TrimVeleroPrefix(provider) {
			case AWSProvider:
				err := r.validateAWSBackupStorageLocation(*bslSpec.Velero, &dpa)
				if err != nil {
					return false, err
				}
			case AzureProvider:
				err := r.validateAzureBackupStorageLocation(*bslSpec.Velero, &dpa)
				if err != nil {
					return false, err
				}
			case GCPProvider:
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
		}
		if bslSpec.CloudStorage != nil && bslSpec.Velero != nil {
			return false, fmt.Errorf("must choose one of bucket or velero")
		}
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
		bsl := velerov1.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{
				// TODO: Use a hash instead of i
				Name:      fmt.Sprintf("%s-%d", r.NamespacedName.Name, i+1),
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
		op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &bsl, func() error {
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
				}
				switch bucket.Spec.Provider {
				case oadpv1alpha1.AWSBucketProvider:
					bsl.Spec.Provider = AWSProvider
				case oadpv1alpha1.AzureBucketProvider:
					return fmt.Errorf("azure provider not yet supported")
				case oadpv1alpha1.GCPBucketProvider:
					bsl.Spec.Provider = GCPProvider
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
	secret.SetLabels(map[string]string{oadpv1alpha1.OadpOperatorLabel: "True", namespace + ".dataprotectionapplication": dpaName})
	err = r.Client.Update(r.Context, &secret, &client.UpdateOptions{})
	if err != nil {
		return false, err
	}

	r.EventRecorder.Event(&secret, corev1.EventTypeNormal, "SecretLabelled", fmt.Sprintf("Secret %s has been labelled", secretName))
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
	bsl.Labels = map[string]string{
		"app.kubernetes.io/name":     "oadp-operator-velero",
		"app.kubernetes.io/instance": bsl.Name,
		//"app.kubernetes.io/version":    "x.y.z",
		"app.kubernetes.io/managed-by":       "oadp-operator",
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
	// BSL region is required when s3ForcePathStyle is true AND BackupImages is false
	if (bslSpec.Config == nil || len(bslSpec.Config[Region]) == 0 && bslSpec.Config[S3ForcePathStyle] == "true") && dpa.BackupImages() {
		return fmt.Errorf("region for AWS backupstoragelocation cannot be empty when s3ForcePathStyle is true or when backing up images")
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

func (r *DPAReconciler) ensureBSLProviderMapping(dpa *oadpv1alpha1.DataProtectionApplication) error {

	providerBSLMap := map[string]int{}
	for _, bsl := range dpa.Spec.BackupLocations {
		if bsl.CloudStorage == nil && bsl.Velero == nil {
			return fmt.Errorf("no bucket or BSL provided for backupstoragelocations")
		}
		if bsl.Velero != nil {
			// Only check the default providers here, if there are extra credentials passed then we can have more than one.
			if bsl.Velero.Credential == nil {
				provider := bsl.Velero.Provider

				providerBSLMap[provider]++

				if providerBSLMap[provider] > 1 {
					return fmt.Errorf("more than one backupstoragelocations configured for provider %s ", provider)
				}
			}
		}
		if bsl.CloudStorage != nil && bsl.Velero != nil {
			return fmt.Errorf("more than one of backupstoragelocations and bucket provided for a single StorageLocation")
		}
	}
	return nil
}

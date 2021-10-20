package controllers

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *VeleroReconciler) ValidateBackupStorageLocations(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}
	// Ensure we have a BSL or user has specified NoDefaultBackupLocation install
	if len(velero.Spec.BackupStorageLocations) == 0 && !velero.Spec.NoDefaultBackupLocation {
		return false, errors.New("no backupstoragelocations configured, ensure a backupstoragelocation has been configured")
	}

	// Ensure BSL:Provider has a 1:1 mapping
	if err := r.ensureBSLProviderMapping(&velero); err != nil {
		return false, err
	}

	// Ensure BSL is a valid configuration
	// First, check for provider and then call functions based on the cloud provider for each backupstoragelocation configured
	for _, bslSpec := range velero.Spec.BackupStorageLocations {
		provider := bslSpec.Provider
		if len(provider) == 0 {
			return false, fmt.Errorf("no provider specified for one of the backupstoragelocations configured")
		}

		// TODO: cases might need some updates for IBM/Minio/noobaa
		switch provider {
		case AWSProvider, "velero.io/aws":
			err := r.validateAWSBackupStorageLocation(bslSpec, &velero)
			if err != nil {
				return false, err
			}
		case AzureProvider, "velero.io/azure":
			err := r.validateAzureBackupStorageLocation(bslSpec, &velero)
			if err != nil {
				return false, err
			}
		case GCPProvider, "velero.io/gcp":
			err := r.validateGCPBackupStorageLocation(bslSpec, &velero)
			if err != nil {
				return false, err
			}
		// case when no supporting cloud provider is configured in bsl
		default:
			return false, fmt.Errorf("no valid provider configured for one of the backupstoragelocations")
		}
	}

	// TODO: Discuss If multiple BSLs exist, ensure we have multiple credentials

	return true, nil
}

func (r *VeleroReconciler) ReconcileBackupStorageLocations(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}
	// Loop through all configured BSLs
	for i, bslSpec := range velero.Spec.BackupStorageLocations {
		// Create BSL as is, we can safely assume they are valid from
		// ValidateBackupStorageLocations
		bsl := velerov1.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{
				// TODO: Use a hash instead of i
				Name:      fmt.Sprintf("%s-%d", r.NamespacedName.Name, i+1),
				Namespace: r.NamespacedName.Namespace,
			},
		}
		// Create BSL
		op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &bsl, func() error {
			// TODO: Velero may be setting controllerReference as
			// well and taking ownership. If so move this to
			// SetOwnerReference instead

			// TODO: check for BSL status condition errors and respond here

			err := r.updateBSLFromSpec(&bsl, &velero, bslSpec)

			return err
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

func (r *VeleroReconciler) updateBSLFromSpec(bsl *velerov1.BackupStorageLocation, velero *oadpv1alpha1.Velero, bslSpec velerov1.BackupStorageLocationSpec) error {
	// Set controller reference to Velero controller
	err := controllerutil.SetControllerReference(velero, bsl, r.Scheme)
	if err != nil {
		return err
	}

	bsl.Labels = map[string]string{
		"app.kubernetes.io/name":     "oadp-operator-velero",
		"app.kubernetes.io/instance": bsl.Name,
		//"app.kubernetes.io/version":    "x.y.z",
		"app.kubernetes.io/managed-by": "oadp-operator",
		"app.kubernetes.io/component":  "bsl",
		oadpv1alpha1.OadpOperatorLabel: "True",
	}
	bsl.Spec = bslSpec

	return nil
}

func (r *VeleroReconciler) validateAWSBackupStorageLocation(bslSpec velerov1.BackupStorageLocationSpec, velero *oadpv1alpha1.Velero) error {
	// validate provider plugin and secret
	err := r.validateProviderPluginAndSecret(bslSpec, velero)
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

	if len(bslSpec.StorageType.ObjectStorage.Prefix) == 0 || len(bslSpec.Config[Region]) == 0 &&
		(velero.Spec.BackupImages == nil || *velero.Spec.BackupImages) {
		return fmt.Errorf("prefix and region for AWS backupstoragelocation object storage cannot be empty. It is required for backing up images")
	}

	//TODO: Add minio, noobaa, local storage validations

	return nil
}

func (r *VeleroReconciler) validateAzureBackupStorageLocation(bslSpec velerov1.BackupStorageLocationSpec, velero *oadpv1alpha1.Velero) error {
	// validate provider plugin and secret
	err := r.validateProviderPluginAndSecret(bslSpec, velero)
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

	if len(bslSpec.StorageType.ObjectStorage.Prefix) == 0 && (velero.Spec.BackupImages == nil || *velero.Spec.BackupImages) {
		return fmt.Errorf("prefix for Azure backupstoragelocation object storage cannot be empty. it is required for backing up images")
	}

	return nil
}

func (r *VeleroReconciler) validateGCPBackupStorageLocation(bslSpec velerov1.BackupStorageLocationSpec, velero *oadpv1alpha1.Velero) error {
	// validate provider plugin and secret
	err := r.validateProviderPluginAndSecret(bslSpec, velero)
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

	if len(bslSpec.StorageType.ObjectStorage.Prefix) == 0 && (velero.Spec.BackupImages == nil || *velero.Spec.BackupImages) {
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

func (r *VeleroReconciler) validateProviderPluginAndSecret(bslSpec velerov1.BackupStorageLocationSpec, velero *oadpv1alpha1.Velero) error {
	// check for existence of provider plugin and warn if the plugin is absent
	if !pluginExistsInVeleroCR(velero.Spec.DefaultVeleroPlugins, oadpv1alpha1.DefaultPlugin(bslSpec.Provider)) {
		r.Log.Info(fmt.Sprintf("%s backupstoragelocation is configured but velero plugin for %s is not present", bslSpec.Provider, bslSpec.Provider))
		//TODO: set warning condition on Velero CR
	}
	secretName, _ := r.getSecretNameAndKey(bslSpec.Credential, oadpv1alpha1.DefaultPlugin(bslSpec.Provider))

	_, err := r.getProviderSecret(secretName)

	if err != nil {
		r.Log.Info(fmt.Sprintf("error validating %s provider secret:  %s/%s", bslSpec.Provider, r.NamespacedName.Namespace, secretName))
		return err
	}
	return nil
}

func (r *VeleroReconciler) ensureBSLProviderMapping(velero *oadpv1alpha1.Velero) error {

	providerBSLMap := map[string]int{}
	for _, bsl := range velero.Spec.BackupStorageLocations {
		provider := bsl.Provider

		providerBSLMap[provider]++

		if providerBSLMap[provider] > 1 {
			return fmt.Errorf("more than one backupstoragelocations configured for provider %s ", provider)
		}

	}
	return nil
}

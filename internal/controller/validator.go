package controller

import (
	"errors"
	"fmt"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials"
)

const NACNonEnforceableErr = "DPA %s is non-enforceable by admins"

// ValidateDataProtectionCR function validates the DPA CR, returns true if valid, false otherwise
// it calls other validation functions to validate the DPA CR
func (r *DataProtectionApplicationReconciler) ValidateDataProtectionCR(log logr.Logger) (bool, error) {
	dpaList := &oadpv1alpha1.DataProtectionApplicationList{}
	err := r.List(r.Context, dpaList, &client.ListOptions{Namespace: r.NamespacedName.Namespace})
	if err != nil {
		return false, err
	}
	if len(dpaList.Items) > 1 {
		return false, errors.New("only one DPA CR can exist per OADP installation namespace")
	}

	if r.dpa.Spec.Configuration == nil || r.dpa.Spec.Configuration.Velero == nil {
		return false, errors.New("DPA CR Velero configuration cannot be nil")
	}

	if r.dpa.Spec.Configuration.Restic != nil && r.dpa.Spec.Configuration.NodeAgent != nil {
		return false, errors.New("DPA CR cannot have restic (deprecated in OADP 1.3) as well as nodeAgent options at the same time")
	}

	if r.dpa.Spec.Configuration.Velero.NoDefaultBackupLocation {
		if len(r.dpa.Spec.BackupLocations) != 0 {
			return false, errors.New("DPA CR Velero configuration cannot have backup locations if noDefaultBackupLocation is set")
		}
		if r.dpa.BackupImages() {
			return false, errors.New("backupImages needs to be set to false when noDefaultBackupLocation is set")
		}
	} else {
		if len(r.dpa.Spec.BackupLocations) == 0 {
			return false, errors.New("no backupstoragelocations configured, ensure a backupstoragelocation has been configured or use the noDefaultBackupLocation flag")
		}
	}

	if validBsl, err := r.ValidateBackupStorageLocations(); !validBsl || err != nil {
		return validBsl, err
	}
	if validVsl, err := r.ValidateVolumeSnapshotLocations(); !validVsl || err != nil {
		return validVsl, err
	}

	// check for VSM/Volsync DataMover (OADP 1.2 or below) syntax
	if r.dpa.Spec.Features != nil && r.dpa.Spec.Features.DataMover != nil {
		return false, errors.New("Delete vsm from spec.configuration.velero.defaultPlugins and dataMover object from spec.features. Use Velero Built-in Data Mover instead")
	}

	if val, found := r.dpa.Spec.UnsupportedOverrides[oadpv1alpha1.OperatorTypeKey]; found && val != oadpv1alpha1.OperatorTypeMTC {
		return false, errors.New("only mtc operator type override is supported")
	}

	if _, err := r.ValidateVeleroPlugins(); err != nil {
		return false, err
	}

	// TODO refactor to call functions only once
	// they are called here to check error, and then after to get value
	if _, err := r.getVeleroResourceReqs(); err != nil {
		return false, err
	}

	if _, err := getResticResourceReqs(r.dpa); err != nil {
		return false, err
	}
	if _, err := getNodeAgentResourceReqs(r.dpa); err != nil {
		return false, err
	}

	// validate non-admin enable
	if r.dpa.Spec.NonAdmin != nil {
		if r.dpa.Spec.NonAdmin.Enable != nil {

			dpaList := &oadpv1alpha1.DataProtectionApplicationList{}
			err = r.ClusterWideClient.List(r.Context, dpaList)
			if err != nil {
				return false, err
			}
			for _, dpa := range dpaList.Items {
				if dpa.Namespace != r.NamespacedName.Namespace && (&DataProtectionApplicationReconciler{dpa: &dpa}).checkNonAdminEnabled() {
					nonAdminDeployment := &appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nonAdminObjectName,
							Namespace: dpa.Namespace,
						},
					}
					if err := r.ClusterWideClient.Get(
						r.Context,
						types.NamespacedName{
							Name:      nonAdminDeployment.Name,
							Namespace: nonAdminDeployment.Namespace,
						},
						nonAdminDeployment,
					); err == nil {
						return false, fmt.Errorf("only a single instance of Non-Admin Controller can be installed across the entire cluster. Non-Admin controller is already configured and installed in %s namespace", dpa.Namespace)
					}
				}
			}
		}

		garbageCollectionPeriod := r.dpa.Spec.NonAdmin.GarbageCollectionPeriod
		appliedGarbageCollectionPeriod := oadpv1alpha1.DefaultGarbageCollectionPeriod
		if garbageCollectionPeriod != nil {
			if garbageCollectionPeriod.Duration < 0 {
				return false, fmt.Errorf("DPA spec.nonAdmin.garbageCollectionPeriod can not be negative")
			}
			appliedGarbageCollectionPeriod = garbageCollectionPeriod.Duration
		}

		backupSyncPeriod := r.dpa.Spec.NonAdmin.BackupSyncPeriod
		appliedBackupSyncPeriod := oadpv1alpha1.DefaultBackupSyncPeriod
		if backupSyncPeriod != nil {
			if backupSyncPeriod.Duration < 0 {
				return false, fmt.Errorf("DPA spec.nonAdmin.backupSyncPeriod can not be negative")
			}
			appliedBackupSyncPeriod = backupSyncPeriod.Duration
		}

		if appliedGarbageCollectionPeriod <= appliedBackupSyncPeriod {
			return false, fmt.Errorf(
				"DPA spec.nonAdmin.backupSyncPeriod (%v) can not be greater or equal spec.nonAdmin.garbageCollectionPeriod (%v)",
				appliedBackupSyncPeriod, appliedGarbageCollectionPeriod,
			)
		}
		// TODO should also validate that BSL backupSyncPeriod is not greater or equal to nonAdmin.backupSyncPeriod
		// but BSL can not exist yet when we validate the value

		enforcedBackupSpec := r.dpa.Spec.NonAdmin.EnforceBackupSpec

		if enforcedBackupSpec != nil {
			// check if BSL name is enforced by the admin
			// We do not support this, we restrict enforcing BSL name
			if enforcedBackupSpec.StorageLocation != "" {
				return false, fmt.Errorf(NACNonEnforceableErr, "spec.nonAdmin.enforcedBackupSpec.storageLocation")
			}

			if enforcedBackupSpec.VolumeSnapshotLocations != nil {
				return false, fmt.Errorf(NACNonEnforceableErr, "spec.nonAdmin.enforcedBackupSpec.volumeSnapshotLocations")
			}

			if enforcedBackupSpec.IncludedNamespaces != nil {
				return false, fmt.Errorf(NACNonEnforceableErr, "spec.nonAdmin.enforcedBackupSpec.includedNamespaces")
			}

			if enforcedBackupSpec.ExcludedNamespaces != nil {
				return false, fmt.Errorf(NACNonEnforceableErr, "spec.nonAdmin.enforcedBackupSpec.excludedNamespaces")
			}

			if enforcedBackupSpec.IncludeClusterResources != nil && *enforcedBackupSpec.IncludeClusterResources {
				return false, fmt.Errorf(NACNonEnforceableErr+" as true, must be set to false if enforced by admins", "spec.nonAdmin.enforcedBackupSpec.includeClusterResources")
			}

			if len(enforcedBackupSpec.IncludedClusterScopedResources) > 0 {
				return false, fmt.Errorf(NACNonEnforceableErr+" and must remain empty", "spec.nonAdmin.enforcedBackupSpec.includedClusterScopedResources")
			}

		}

		enforcedRestoreSpec := r.dpa.Spec.NonAdmin.EnforceRestoreSpec

		if enforcedRestoreSpec != nil {
			if len(enforcedRestoreSpec.ScheduleName) > 0 {
				return false, fmt.Errorf(NACNonEnforceableErr, "spec.nonAdmin.enforcedRestoreSpec.scheduleName")
			}

			if enforcedRestoreSpec.IncludedNamespaces != nil {
				return false, fmt.Errorf(NACNonEnforceableErr, "spec.nonAdmin.enforcedRestoreSpec.includedNamespaces")
			}

			if enforcedRestoreSpec.ExcludedNamespaces != nil {
				return false, fmt.Errorf(NACNonEnforceableErr, "spec.nonAdmin.enforcedRestoreSpec.excludedNamespaces")
			}

			if enforcedRestoreSpec.NamespaceMapping != nil {
				return false, fmt.Errorf(NACNonEnforceableErr, "spec.nonAdmin.enforcedRestoreSpec.namespaceMapping")
			}
		}

	}

	return true, nil
}

// For later: Move this code into validator.go when more need for validation arises
// TODO: if multiple default plugins exist, ensure we validate all of them.
// Right now its sequential validation
func (r *DataProtectionApplicationReconciler) ValidateVeleroPlugins() (bool, error) {

	dpa := r.dpa

	providerNeedsDefaultCreds, hasCloudStorage, err := r.noDefaultCredentials()
	if err != nil {
		return false, err
	}

	snapshotLocationsProviders := make(map[string]bool)
	for _, location := range dpa.Spec.SnapshotLocations {
		if location.Velero != nil {
			provider := strings.TrimPrefix(location.Velero.Provider, veleroIOPrefix)
			snapshotLocationsProviders[provider] = true
		}
	}

	foundAWSPlugin := false
	foundLegacyAWSPlugin := false
	for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
		pluginSpecificMap, ok := credentials.PluginSpecificFields[plugin]
		pluginNeedsCheck, foundInBSLorVSL := providerNeedsDefaultCreds[pluginSpecificMap.ProviderName]

		// "aws" and "legacy-aws" cannot both be specified
		if plugin == oadpv1alpha1.DefaultPluginAWS {
			foundAWSPlugin = true
		}
		if plugin == oadpv1alpha1.DefaultPluginLegacyAWS {
			foundLegacyAWSPlugin = true
		}

		// check for VSM/Volsync DataMover (OADP 1.2 or below) syntax
		if plugin == oadpv1alpha1.DefaultPluginVSM {
			return false, errors.New("Delete vsm from spec.configuration.velero.defaultPlugins and dataMover object from spec.features. Use Velero Built-in Data Mover instead")
		}
		if foundInVSL := snapshotLocationsProviders[string(plugin)]; foundInVSL {
			pluginNeedsCheck = true
		}
		if !foundInBSLorVSL && !hasCloudStorage {
			pluginNeedsCheck = true
		}
		if ok && pluginSpecificMap.IsCloudProvider && pluginNeedsCheck && !dpa.Spec.Configuration.Velero.NoDefaultBackupLocation && !dpa.Spec.Configuration.Velero.HasFeatureFlag("no-secret") {
			secretNamesToValidate := mapset.NewSet[string]()
			// check specified credentials in backup locations exists in the cluster
			for _, location := range dpa.Spec.BackupLocations {
				if location.Velero != nil {
					provider := strings.TrimPrefix(location.Velero.Provider, veleroIOPrefix)
					if provider == string(plugin) && location.Velero != nil {
						if location.Velero.Credential != nil {
							secretNamesToValidate.Add(location.Velero.Credential.Name)
						} else {
							secretNamesToValidate.Add(pluginSpecificMap.SecretName)
						}
					}
				}
			}
			// check specified credentials in snapshot locations exists in the cluster
			for _, location := range dpa.Spec.SnapshotLocations {
				if location.Velero != nil {
					provider := strings.TrimPrefix(location.Velero.Provider, veleroIOPrefix)
					if provider == string(plugin) && location.Velero != nil {
						if location.Velero.Credential != nil {
							secretNamesToValidate.Add(location.Velero.Credential.Name)
						} else {
							secretNamesToValidate.Add(pluginSpecificMap.SecretName)
						}
					}
				}
			}
			for _, secretName := range secretNamesToValidate.ToSlice() {
				_, err := r.getProviderSecret(secretName)
				if err != nil {
					r.Log.Info(fmt.Sprintf("error validating %s provider secret:  %s/%s", string(plugin), r.NamespacedName.Namespace, secretName))
					return false, err
				}
			}
		}
	}

	if foundAWSPlugin && foundLegacyAWSPlugin {
		return false, fmt.Errorf("%s and %s can not be both specified in DPA spec.configuration.velero.defaultPlugins", oadpv1alpha1.DefaultPluginAWS, oadpv1alpha1.DefaultPluginLegacyAWS)
	}

	return true, nil
}

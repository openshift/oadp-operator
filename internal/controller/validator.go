package controller

import (
	"errors"
	"fmt"
	"strings"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/go-logr/logr"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	"github.com/openshift/oadp-operator/pkg/credentials"
)

const NACNonEnforceableErr = "DPA %s is non-enforceable by admins"

var wasRestic bool

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

	// Ensure DPA spec.configuration.nodeAgent.PodConfig is not different from spec.configuration.nodeAgent.LoadAffinityConfig
	// If LoadAffinityConfig is set, it will be used instead of PodConfig; however, if both are set, they must be identical.
	if r.dpa.Spec.Configuration.NodeAgent != nil &&
		r.dpa.Spec.Configuration.NodeAgent.PodConfig != nil &&
		r.dpa.Spec.Configuration.NodeAgent.LoadAffinityConfig != nil {

		if len(r.dpa.Spec.Configuration.NodeAgent.LoadAffinityConfig) > 1 {
			return false, errors.New("when spec.configuration.nodeAgent.PodConfig is set, spec.configuration.nodeAgent.LoadAffinityConfig must contain no more than one entry")
		}

		// podConfig is set !
		if len(r.dpa.Spec.Configuration.NodeAgent.LoadAffinityConfig) == 1 {
			podConfigSelector := r.dpa.Spec.Configuration.NodeAgent.PodConfig.NodeSelector
			affinitySelector := r.dpa.Spec.Configuration.NodeAgent.LoadAffinityConfig[0].NodeSelector

			// Ensure MatchLabels is set and MatchExpressions is not used
			if affinitySelector.MatchLabels == nil {
				return false, errors.New("when spec.configuration.nodeAgent.PodConfig is set, spec.configuration.nodeAgent.LoadAffinityConfig must define matchLabels")
			}
			if affinitySelector.MatchExpressions != nil {
				return false, errors.New("when spec.configuration.nodeAgent.PodConfig is set, spec.configuration.nodeAgent.LoadAffinityConfig must not define matchExpressions")
			}

			// Ensure all labels in LoadAffinityConfig match those in PodConfig
			for key, valA := range affinitySelector.MatchLabels {
				if valB, exists := podConfigSelector[key]; !exists || valA != valB {
					return false, errors.New("when spec.configuration.nodeAgent.PodConfig is set, all labels from the spec.configuration.nodeAgent.LoadAffinityConfig must be present in the spec.configuration.nodeAgent.PodConfig")
				}
			}
		}
	}

	// ENSURE UPGRADES --------------------------------------------------------
	// check for VSM/Volsync DataMover (OADP 1.2 or below) syntax
	if r.dpa.Spec.Features != nil && r.dpa.Spec.Features.DataMover != nil {
		return false, errors.New("Delete vsm from spec.configuration.velero.defaultPlugins and dataMover object from spec.features. Use Velero Built-in Data Mover instead")
	}

	// check for ResticConfig (OADP 1.4 or below) syntax
	if r.dpa.Spec.Configuration.Restic != nil {
		return false, errors.New("Delete restic object from spec.configuration, use spec.configuration.nodeAgent instead")
	}
	// ENSURE UPGRADES --------------------------------------------------------

	// DEPRECATIONS -----------------------------------------------------------
	if r.dpa.Spec.Configuration.NodeAgent != nil && r.dpa.Spec.Configuration.NodeAgent.UploaderType == "restic" {
		if !wasRestic {
			deprecationWarning := "(Deprecation Warning) Use kopia instead of restic in spec.configuration.nodeAgent.uploaderType, which is deprecated and will be removed in the future"
			// V(-1) corresponds to the warn level
			log.V(-1).Info(deprecationWarning)
			r.EventRecorder.Event(r.dpa, corev1.EventTypeWarning, "DeprecationResticFileSystemBackup", deprecationWarning)
		}
		wasRestic = true
	} else {
		wasRestic = false
	}
	// DEPRECATIONS -----------------------------------------------------------

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

		defaultBSLIndex := -1
		bslsInOADPNamespace := &velerov1.BackupStorageLocationList{}
		r.List(r.Context, bslsInOADPNamespace, client.InNamespace(r.NamespacedName.Namespace))
		for index, bsl := range bslsInOADPNamespace.Items {
			if bsl.Spec.Default {
				defaultBSLIndex = index
				break
			}
		}
		if defaultBSLIndex >= 0 {
			defaultBSL := bslsInOADPNamespace.Items[defaultBSLIndex]
			defaultBSLSpec := defaultBSL.Spec

			if defaultBSL.Labels != nil {
				if value, ok := defaultBSL.Labels["app.kubernetes.io/managed-by"]; ok && value == common.OADPOperator {
					for index, bsl := range r.dpa.Spec.BackupLocations {
						if bsl.Velero.Default {
							defaultBSLIndex = index
							break
						}
					}
					defaultBSLSpec = *r.dpa.Spec.BackupLocations[defaultBSLIndex].Velero
				}
			}

			defaultBSLSyncPeriodErrorMessage := "default BSL spec.backupSyncPeriod (%v) can not be greater or equal spec.nonAdmin.backupSyncPeriod (%v)"
			if defaultBSLSpec.BackupSyncPeriod != nil {
				if appliedBackupSyncPeriod <= defaultBSLSpec.BackupSyncPeriod.Duration {
					return false, fmt.Errorf(
						defaultBSLSyncPeriodErrorMessage,
						defaultBSLSpec.BackupSyncPeriod.Duration, appliedBackupSyncPeriod,
					)
				}
			} else {
				if r.dpa.Spec.Configuration.Velero.Args != nil && r.dpa.Spec.Configuration.Velero.Args.BackupSyncPeriod != nil {
					if appliedBackupSyncPeriod <= *r.dpa.Spec.Configuration.Velero.Args.BackupSyncPeriod {
						return false, fmt.Errorf(
							defaultBSLSyncPeriodErrorMessage,
							r.dpa.Spec.Configuration.Velero.Args.BackupSyncPeriod, appliedBackupSyncPeriod,
						)
					}
				} else {
					// https://github.com/vmware-tanzu/velero/blob/9295be4cc061038b91b7bfaf55d99e9bc9dcf0af/pkg/cmd/server/config/config.go#L24
					if appliedBackupSyncPeriod <= time.Minute {
						return false, fmt.Errorf(
							defaultBSLSyncPeriodErrorMessage,
							time.Minute, appliedBackupSyncPeriod,
						)
					}
				}
			}
		}

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

		enforcedBSLSpec := r.dpa.Spec.NonAdmin.EnforceBSLSpec

		if enforcedBSLSpec != nil {
			if enforcedBSLSpec.BackupSyncPeriod != nil && enforcedBSLSpec.BackupSyncPeriod.Duration >= appliedBackupSyncPeriod {
				return false, fmt.Errorf(
					"DPA spec.nonAdmin.enforcedBSLSpec.backupSyncPeriod (%v) can not be greater or equal DPA spec.nonAdmin.backupSyncPeriod (%v)",
					enforcedBSLSpec.BackupSyncPeriod.Duration, appliedBackupSyncPeriod,
				)

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

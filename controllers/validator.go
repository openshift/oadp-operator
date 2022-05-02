package controllers

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials"
)

func (r *DPAReconciler) ValidateDataProtectionCR(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}
	if dpa.Spec.Configuration == nil || dpa.Spec.Configuration.Velero == nil {
		return false, errors.New("DPA CR Velero configuration cannot be nil")
	}

	if dpa.Spec.Configuration.Velero.NoDefaultBackupLocation {
		if len(dpa.Spec.BackupLocations) != 0 {
			return false, errors.New("DPA CR Velero configuration cannot have backup locations if noDefaultBackupLocation is set")
		}
	} else {
		if len(dpa.Spec.BackupLocations) == 0 {
			return false, errors.New("no backupstoragelocations configured, ensure a backupstoragelocation has been configured or use the noDefaultBackupLocation flag")
		}
	}

	if dpa.Spec.Configuration.Velero.NoDefaultBackupLocation && dpa.BackupImages() {
		return false, errors.New("backupImages needs to be set to false when noDefaultBackupLocation is set")
	}

	if len(dpa.Spec.BackupLocations) > 0 {
		for _, location := range dpa.Spec.BackupLocations {
			// check for velero BSL config or cloud storage config
			if location.Velero == nil && location.CloudStorage == nil {
				return false, errors.New("BackupLocation must have velero or bucket configuration")
			}
		}
	}

	if len(dpa.Spec.SnapshotLocations) > 0 {
		for _, location := range dpa.Spec.SnapshotLocations {
			if location.Velero == nil {
				return false, errors.New("snapshotLocation velero configuration cannot be nil")
			}
		}
	}

	if _, err := r.ValidateVeleroPlugins(r.Log); err != nil {
		return false, err
	}

	return true, nil
}

// For later: Move this code into validator.go when more need for validation arises
// TODO: if multiple default plugins exist, ensure we validate all of them.
// Right now its sequential validation
func (r *DPAReconciler) ValidateVeleroPlugins(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	providerNeedsDefaultCreds, hasCloudStorage, err := r.noDefaultCredentials(dpa)
	if err != nil {
		return false, err
	}

	var defaultPlugin oadpv1alpha1.DefaultPlugin
	for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {

		pluginSpecificMap, ok := credentials.PluginSpecificFields[plugin]
		pluginNeedsCheck, foundInBSLorVSL := providerNeedsDefaultCreds[string(plugin)]

		if !foundInBSLorVSL && !hasCloudStorage {
			pluginNeedsCheck = true
		}

		if ok && pluginSpecificMap.IsCloudProvider && pluginNeedsCheck && !dpa.Spec.Configuration.Velero.NoDefaultBackupLocation {
			secretName := pluginSpecificMap.SecretName
			_, err := r.getProviderSecret(secretName)
			if err != nil {
				r.Log.Info(fmt.Sprintf("error validating %s provider secret:  %s/%s", defaultPlugin, r.NamespacedName.Namespace, secretName))
				return false, err
			}
		}
	}
	return true, nil
}

package controllers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials"
	"k8s.io/apimachinery/pkg/types"
)

func (r *DPAReconciler) ValidateDataProtectionCR(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}
	if dpa.Spec.Configuration == nil || dpa.Spec.Configuration.Velero == nil {
		return false, errors.New("DPA CR Velero configuration cannot be nil")
	}

	if len(dpa.Spec.BackupLocations) == 0 && !dpa.Spec.Configuration.Velero.NoDefaultBackupLocation {
		return false, errors.New("no backupstoragelocations configured, ensure a backupstoragelocation has been configured or use the noDefaultLocationBackupLocation flag")
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

	providerNeedsDefaultCreds := map[string]bool{}

	for _, bsl := range dpa.Spec.BackupLocations {
		if bsl.Velero != nil && bsl.Velero.Credential == nil {
			providerNeedsDefaultCreds[strings.TrimPrefix(bsl.Velero.Provider, "velero.io/")] = true
		}
		if bsl.CloudStorage != nil && bsl.CloudStorage.Credential == nil {
			cloudStroage := oadpv1alpha1.CloudStorage{}
			err := r.Get(r.Context, types.NamespacedName{Name: bsl.CloudStorage.CloudStorageRef.Name, Namespace: dpa.Namespace}, &cloudStroage)
			if err != nil {
				return false, err
			}
			providerNeedsDefaultCreds[string(cloudStroage.Spec.Provider)] = true
		}
	}

	for _, vsl := range dpa.Spec.SnapshotLocations {
		if vsl.Velero != nil {
			providerNeedsDefaultCreds[vsl.Velero.Provider] = true
		}
	}

	var defaultPlugin oadpv1alpha1.DefaultPlugin
	for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {

		pluginSpecificMap, ok := credentials.PluginSpecificFields[plugin]
		if ok && pluginSpecificMap.IsCloudProvider && providerNeedsDefaultCreds[string(plugin)] {
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

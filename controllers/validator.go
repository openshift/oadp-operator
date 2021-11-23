package controllers

import (
	"errors"
	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
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
			if location.Velero == nil {
				return false, errors.New("BackupLocation velero configuration cannot be nil")
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

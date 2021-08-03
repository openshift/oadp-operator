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
	// Ensure we have a BSL or user has specified noobaa install
	if len(velero.Spec.BackupStorageLocations) == 0 && !velero.Spec.Noobaa {
		return false, errors.New("no backupstoragelocations configured, ensure a backupstoragelocation or noobaa has been configured")
	}
	// Ensure BSL is a valid configuration
	// If multiple BSLs exist, ensure we have multiple credentials
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
			Spec: bslSpec,
		}
		// Create BSL
		op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &bsl, func() error {
			// TODO: Velero may be setting controllerReference as
			// well and taking ownership. If so move this to
			// SetOwnerReference instead

			// TODO: check for BSL status condition errors and respond here

			err := r.updateBSLFromSpec(&bsl, &velero)

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

func (r *VeleroReconciler) updateBSLFromSpec(bsl *velerov1.BackupStorageLocation, velero *oadpv1alpha1.Velero) error {
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
	}

	return nil
}

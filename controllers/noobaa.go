package controllers

import (
	// uncomment to use
	// "github.com/noobaa/noobaa-operator/v2/pkg/apis/noobaa/v1alpha1"
	"github.com/go-logr/logr"
)

func (r *VeleroReconciler) ReconcileNoobaa(log logr.Logger) (bool, error) {
	// velero := oadpv1alpha1.Velero{}

	// op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, ds, func() error {

	// })

	// if err != nil {
	// 	return false, err
	// }

	// if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
	// 	// Trigger event to indicate restic was created or updated
	// 	r.EventRecorder.Event(ds,
	// 		v1.EventTypeNormal,
	// 		"ResticDaemonsetReconciled",
	// 		fmt.Sprintf("performed %s on restic deployment %s/%s", op, ds.Namespace, ds.Name),
	// 	)
	// }

	return true, nil
}

package controllers

import (
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *DPAReconciler) ReconcileDataMoverController(log logr.Logger) (bool, error) {
	// When updating from OADP version 1.2.x to 1.3.x, delete
	//   volume-snapshot-mover deployment
	// because OADP now uses Velero builtin DataMover instead of VSM/Volsync DataMover
	if err := r.Delete(r.Context, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      "volume-snapshot-mover",
		Namespace: r.NamespacedName.Namespace,
		Labels:    map[string]string{"openshift.io/oadp-data-mover": "True"},
	}}); err != nil && !errors.IsNotFound(err) {
		return false, err
	}

	return true, nil
}

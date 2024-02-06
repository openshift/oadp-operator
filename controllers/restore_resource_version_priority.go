package controllers

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	enableApiGroupVersionsConfigMapName    = "enableapigroupversions"
	restoreResourcesVersionPriorityDataKey = "restoreResourcesVersionPriority"
)

// If RestoreResourcesVersionPriority is defined, configmap is created or updated and feature flag for EnableAPIGroupVersions is added to velero
func (r *DPAReconciler) ReconcileRestoreResourcesVersionPriority() (bool, error) {
	dpa := r.dpa
	if len(dpa.Spec.Configuration.Velero.RestoreResourcesVersionPriority) == 0 {
		return true, nil
	}
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      enableApiGroupVersionsConfigMapName,
			Namespace: dpa.Namespace,
		},
	}
	// Create ConfigMap
	op, err := controllerutil.CreateOrPatch(r.Context, r.Client, &configMap, func() error {
		if err := controllerutil.SetControllerReference(dpa, &configMap, r.Scheme); err != nil {
			return err
		}
		configMap.Data = make(map[string]string, 1)
		configMap.Data[restoreResourcesVersionPriorityDataKey] = dpa.Spec.Configuration.Velero.RestoreResourcesVersionPriority
		return nil
	})
	if err != nil {
		return false, err
	}
	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate ConfigMap was created or updated
		r.EventRecorder.Event(&configMap,
			corev1.EventTypeNormal,
			"RestoreResourcesVersionPriorityReconciled",
			fmt.Sprintf("performed %s on RestoreResourcesVersionPriority %s/%s", op, configMap.Namespace, configMap.Name),
		)
	}
	return true, nil
}

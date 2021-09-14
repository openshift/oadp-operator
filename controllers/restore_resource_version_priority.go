package controllers

import (
	"fmt"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	enableApiGroupVersionsFeatureFlag      = "EnableAPIGroupVersions"
	enableApiGroupVersionsConfigMapName    = "enableapigroupversions"
	restoreResourcesVersionPriorityDataKey = "restoreResourcesVersionPriority"
)

// If RestoreResourcesVersionPriority is defined, configmap is created or updated and feature flag for EnableAPIGroupVersions is added to velero
func (r *VeleroReconciler) ReconcileRestoreResourcesVersionPriority(velero *oadpv1alpha1.Velero) (bool, error) {
	if len(velero.Spec.RestoreResourcesVersionPriority) == 0 {
		return true, nil
	}
	// if the RestoreResourcesVersionPriority is specified then ensure feature flag is enabled for enableApiGroupVersions
	// duplicate feature flag checks are done in ReconcileVeleroDeployment
	velero.Spec.VeleroFeatureFlags = append(velero.Spec.VeleroFeatureFlags, enableApiGroupVersionsFeatureFlag)
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      enableApiGroupVersionsConfigMapName,
			Namespace: velero.Namespace,
		},
	}
	// Create ConfigMap
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &configMap, func() error {
		if err := controllerutil.SetControllerReference(velero, &configMap, r.Scheme); err != nil {
			return err
		}
		configMap.Data = make(map[string]string, 1)
		configMap.Data[restoreResourcesVersionPriorityDataKey] = velero.Spec.RestoreResourcesVersionPriority
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

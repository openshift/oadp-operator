package controllers

import (
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *DPAReconciler) ReconcileDataMoverController(log logr.Logger) (bool, error) {
	// When updating from OADP version 1.2.x to 1.3.x, delete
	//   volume-snapshot-mover deployment
	//   1.2 controllers/datamover.go generated secrets
	//   1.2 controllers/datamover.go generated configMaps
	// because OADP now uses Velero builtin DataMover instead of VSM/Volsync DataMover
	if err := r.Delete(r.Context, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      "volume-snapshot-mover",
		Namespace: r.NamespacedName.Namespace,
		Labels:    map[string]string{"openshift.io/oadp-data-mover": "True"},
	}}); err != nil && !errors.IsNotFound(err) {
		return false, err
	}

	secretResources := &corev1.SecretList{}
	r.List(r.Context, secretResources, client.HasLabels{
		"openshift.io/oadp", "openshift.io/oadp-bsl-name", "openshift.io/oadp-bsl-provider",
	}, &client.ListOptions{Namespace: r.NamespacedName.Namespace})
	for _, secretResource := range secretResources.Items {
		if err := r.Delete(r.Context, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name:      secretResource.Name,
			Namespace: r.NamespacedName.Namespace,
		}}); err != nil && !errors.IsNotFound(err) {
			return false, err
		}
	}

	configMapResources := &corev1.ConfigMapList{}
	r.List(r.Context, configMapResources, client.HasLabels{
		"openshift.io/oadp", "openshift.io/volume-snapshot-mover", "openshift.io/vsm-storageclass",
	}, &client.ListOptions{Namespace: r.NamespacedName.Namespace})
	for _, configMapResource := range configMapResources.Items {
		if err := r.Delete(r.Context, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name:      configMapResource.Name,
			Namespace: r.NamespacedName.Namespace,
		}}); err != nil && !errors.IsNotFound(err) {
			return false, err
		}
	}

	return true, nil
}

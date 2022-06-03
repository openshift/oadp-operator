package controllers

import (
	"fmt"
	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *DPAReconciler) ReconcileDataMoverController(log logr.Logger) (bool, error) {

	// fetch latest DPA instance
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	dataMoverDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-mover",
			Namespace: dpa.Namespace,
		},
	}

	//TODO: deploy basedon the existence of csi datamover plugin
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, dataMoverDeployment, func() error {

		// Setting Deployment selector if a new object is created as it is immutable
		if dataMoverDeployment.ObjectMeta.CreationTimestamp.IsZero() {
			dataMoverDeployment.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"component": "datamover-controller",
				},
			}
		}

		// update the Deployment template
		err := r.builddataMoverDeployment(dataMoverDeployment, &dpa)
		if err != nil {
			return err
		}

		// Setting controller owner reference on the dataMover deployment
		return controllerutil.SetControllerReference(&dpa, dataMoverDeployment, r.Scheme)
	})

	if err != nil {
		if errors.IsInvalid(err) {
			cause, isStatusCause := errors.StatusCause(err, metav1.CauseTypeFieldValueInvalid)
			if isStatusCause && cause.Field == "spec.selector" {
				// recreate deployment
				// TODO: check for in-progress backup/restore to wait for it to finish
				log.Info("Found immutable selector from previous deployment, recreating DataMover Deployment")
				err := r.Delete(r.Context, dataMoverDeployment)
				if err != nil {
					return false, err
				}
				return r.ReconcileDataMoverController(log)
			}
		}

		return false, err
	}

	//TODO: Review DataMover deployment status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate dataMover deployment was created or updated
		r.EventRecorder.Event(dataMoverDeployment,
			corev1.EventTypeNormal,
			"DataMoverDeploymentReconciled",
			fmt.Sprintf("performed %s on datamover deployment %s/%s", op, dataMoverDeployment.Namespace, dataMoverDeployment.Name),
		)
	}
	return true, nil
}

// Build DataMover Deployment
func (r *DPAReconciler) builddataMoverDeployment(dataMoverDeployment *appsv1.Deployment, dpa *oadpv1alpha1.DataProtectionApplication) error {

	if dpa == nil {
		return fmt.Errorf("DPA CR cannot be nil")
	}
	if dataMoverDeployment == nil {
		return fmt.Errorf("datamover deployment cannot be nil")
	}

	//TODO: Add unsupportedoverrides support for datamover deployment image
	datamoverContainer := []corev1.Container{
		{
			Image:           "quay.io/konveyor/volume-snapshot-mover:latest",
			Name:            "datamover-controller-container",
			ImagePullPolicy: corev1.PullAlways,
		},
	}

	dataMoverDeployment.Spec = appsv1.DeploymentSpec{
		Selector: dataMoverDeployment.Spec.Selector,
		Replicas: pointer.Int32(1),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"component": "datamover-controller",
				},
			},
			Spec: corev1.PodSpec{
				RestartPolicy:      corev1.RestartPolicyAlways,
				Containers:         datamoverContainer,
				ServiceAccountName: "openshift-adp-controller-manager",
			},
		},
	}

	return nil
}

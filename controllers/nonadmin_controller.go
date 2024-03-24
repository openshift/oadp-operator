package controllers

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

const (
	nonAdminController               = "non-admin-controller"
	nonAdminControllerDeployment     = nonAdminController + "-deployment"
	nonAdminControllerContainer      = nonAdminController + "-container"
	nonAdminControllerServiceAccount = "openshift-adp-non-admin-controller"
)

var nonAdminControllerDeploymentLabel = map[string]string{
	"component": nonAdminController,
}

func (r *DPAReconciler) ReconcileNonAdminController(log logr.Logger) (bool, error) {
	// TODO https://github.com/openshift/oadp-operator/pull/1316
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	nonAdminDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nonAdminControllerDeployment,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Delete (possible) previously deployment
	if !r.checkNonAdminEnabled(&dpa) {
		if err := r.Get(
			context.Background(),
			types.NamespacedName{
				Name:      nonAdminDeployment.Name,
				Namespace: nonAdminDeployment.Namespace,
			},
			nonAdminDeployment,
		); err != nil {
			if k8serror.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}

		deleteOptionPropagationForeground := metav1.DeletePropagationForeground
		if err := r.Delete(
			context.Background(),
			nonAdminDeployment,
			&client.DeleteOptions{PropagationPolicy: &deleteOptionPropagationForeground},
		); err != nil {
			r.EventRecorder.Event(
				nonAdminDeployment,
				corev1.EventTypeWarning,
				"DeleteNonAdminDeploymentFailed",
				fmt.Sprintf("Could not delete non admin controller deployment %s/%s: %s", nonAdminDeployment.Namespace, nonAdminDeployment.Name, err),
			)
			return false, err
		}
		r.EventRecorder.Event(
			nonAdminDeployment,
			corev1.EventTypeNormal,
			"DeletedNonAdminDeploymentDeployment",
			fmt.Sprintf("Non admin controller deployment %s/%s deleted", nonAdminDeployment.Namespace, nonAdminDeployment.Name),
		)
		return true, nil
	}

	operation, err := controllerutil.CreateOrUpdate(
		r.Context,
		r.Client,
		nonAdminDeployment,
		func() error {
			// Setting Deployment selector if a new object is created, as it is immutable
			if nonAdminDeployment.ObjectMeta.CreationTimestamp.IsZero() {
				nonAdminDeployment.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: nonAdminControllerDeploymentLabel,
				}
			}

			nonAdminImage := r.getNonAdminImage(&dpa)
			err := r.buildNonAdminDeployment(nonAdminDeployment, nonAdminImage)
			if err != nil {
				return err
			}

			// Setting controller owner reference on the non admin controller deployment
			return controllerutil.SetControllerReference(&dpa, nonAdminDeployment, r.Scheme)
		},
	)

	if err != nil {
		// TODO needed?
		if k8serror.IsInvalid(err) {
			cause, isStatusCause := k8serror.StatusCause(err, metav1.CauseTypeFieldValueInvalid)
			if isStatusCause && cause.Field == "spec.selector" {
				log.Info("Found immutable selector from previous deployment, recreating non admin controller deployment")
				err := r.Delete(r.Context, nonAdminDeployment)
				if err != nil {
					return false, err
				}
				return r.ReconcileNonAdminController(log)
			}
		}

		return false, err
	}

	if operation != controllerutil.OperationResultNone {
		r.EventRecorder.Event(
			nonAdminDeployment,
			corev1.EventTypeNormal,
			"NonAdminDeploymentReconciled",
			fmt.Sprintf("Non admin controller deployment %s/%s was %s", nonAdminDeployment.Namespace, nonAdminDeployment.Name, operation),
		)
	}
	return true, nil
}

func (r *DPAReconciler) buildNonAdminDeployment(deploymentObject *appsv1.Deployment, image string) error {
	nonAdminContainer := corev1.Container{
		Image:           image,
		Name:            nonAdminControllerContainer,
		ImagePullPolicy: corev1.PullAlways,
	}

	deploymentObject.Spec = appsv1.DeploymentSpec{
		Selector: deploymentObject.Spec.Selector,
		Replicas: pointer.Int32(1),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: nonAdminControllerDeploymentLabel,
			},
			Spec: corev1.PodSpec{
				RestartPolicy:      corev1.RestartPolicyAlways,
				Containers:         []corev1.Container{nonAdminContainer},
				ServiceAccountName: nonAdminControllerServiceAccount,
			},
		},
	}

	return nil
}

func (r *DPAReconciler) checkNonAdminEnabled(dpa *oadpv1alpha1.DataProtectionApplication) bool {
	if dpa.Spec.Features != nil && dpa.Spec.Features.EnableNonAdminMode != nil {
		return *dpa.Spec.Features.EnableNonAdminMode
	}

	return false
}

func (r *DPAReconciler) getNonAdminImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	// TODO is this needed?
	unsupportedOverride := dpa.Spec.UnsupportedOverrides[oadpv1alpha1.NonAdminControllerImageKey]
	if unsupportedOverride != "" {
		return unsupportedOverride
	}

	environmentVariable := os.Getenv("RELATED_IMAGE_NON_ADMIN_CONTROLLER")
	if environmentVariable != "" {
		return environmentVariable
	}

	// TODO change
	return "quay.io/msouzaol/non-admin-controller:latest"
}

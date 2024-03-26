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
	"github.com/openshift/oadp-operator/pkg/common"
)

const (
	nonAdminPrefix                      = "non-admin-"
	controllerManager                   = "controller-manager"
	nonAdminControllerContainer         = nonAdminPrefix + "manager"
	nonAdminControllerControllerManager = common.OADPOperatorPrefix + nonAdminPrefix + controllerManager
)

var (
	nonAdminControlPlaneLabel = map[string]string{
		"control-plane": nonAdminPrefix + controllerManager,
	}
	nonAdminDeploymentLabels = map[string]string{
		"app.kubernetes.io/component":  "manager",
		"app.kubernetes.io/created-by": common.OADPOperator,
		"app.kubernetes.io/instance":   nonAdminPrefix + controllerManager,
		"app.kubernetes.io/managed-by": "kustomize",
		"app.kubernetes.io/name":       "deployment",
		"app.kubernetes.io/part-of":    common.OADPOperator,
		"control-plane":                nonAdminPrefix + controllerManager,
	}
)

func (r *DPAReconciler) ReconcileNonAdminController(log logr.Logger) (bool, error) {
	// TODO https://github.com/openshift/oadp-operator/pull/1316
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	nonAdminDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nonAdminControllerControllerManager,
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
					MatchLabels: nonAdminControlPlaneLabel,
				}
			}

			nonAdminImage := r.getNonAdminImage(&dpa)
			// TODO remove, just for tests
			log.Info(fmt.Sprintf("NON ADMIN IMAGE: %v", nonAdminImage))
			r.buildNonAdminDeployment(nonAdminDeployment, nonAdminImage)

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

func (r *DPAReconciler) buildNonAdminDeployment(deploymentObject *appsv1.Deployment, image string) {
	deploymentObject.ObjectMeta.Labels = nonAdminDeploymentLabels

	deploymentObject.Spec = appsv1.DeploymentSpec{
		Replicas: pointer.Int32(1),
		Selector: deploymentObject.Spec.Selector,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: nonAdminControlPlaneLabel,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Image:           image,
						ImagePullPolicy: corev1.PullAlways,
						Name:            nonAdminControllerContainer,
					},
				},
				RestartPolicy:      corev1.RestartPolicyAlways,
				ServiceAccountName: nonAdminControllerControllerManager,
			},
		},
	}
}

func (r *DPAReconciler) checkNonAdminEnabled(dpa *oadpv1alpha1.DataProtectionApplication) bool {
	if dpa.Spec.Features != nil && dpa.Spec.Features.EnableNonAdmin != nil {
		return *dpa.Spec.Features.EnableNonAdmin
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

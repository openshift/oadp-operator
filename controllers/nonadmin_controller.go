package controllers

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"golang.org/x/exp/maps"
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
	nonAdminPrefix            = "non-admin-"
	controllerManager         = "controller-manager"
	containerName             = nonAdminPrefix + "manager"
	nonAdminControllerManager = common.OADPOperatorPrefix + nonAdminPrefix + controllerManager
	controlPlaneKey           = "control-plane"
)

var (
	controlPlaneLabel = map[string]string{
		controlPlaneKey: nonAdminPrefix + controllerManager,
	}
	deploymentLabels = map[string]string{
		"app.kubernetes.io/component":  "manager",
		"app.kubernetes.io/created-by": common.OADPOperator,
		"app.kubernetes.io/instance":   nonAdminPrefix + controllerManager,
		"app.kubernetes.io/managed-by": "kustomize",
		"app.kubernetes.io/name":       "deployment",
		"app.kubernetes.io/part-of":    common.OADPOperator,
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
			Name:      nonAdminControllerManager,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Delete (possible) previously deployment
	if !r.checkNonAdminEnabled(&dpa) {
		if err := r.Get(
			// TODO use r.Context?
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
			// TODO use r.Context?
			context.Background(),
			nonAdminDeployment,
			&client.DeleteOptions{PropagationPolicy: &deleteOptionPropagationForeground},
		); err != nil {
			r.EventRecorder.Event(
				nonAdminDeployment,
				corev1.EventTypeWarning,
				"NonAdminDeploymentDeleteFailed",
				fmt.Sprintf("Could not delete non admin controller deployment %s/%s: %s", nonAdminDeployment.Namespace, nonAdminDeployment.Name, err),
			)
			return false, err
		}
		r.EventRecorder.Event(
			nonAdminDeployment,
			corev1.EventTypeNormal,
			"NonAdminDeploymentDeleteSucceed",
			fmt.Sprintf("Non admin controller deployment %s/%s deleted", nonAdminDeployment.Namespace, nonAdminDeployment.Name),
		)
		return true, nil
	}

	operation, err := controllerutil.CreateOrUpdate(
		r.Context,
		r.Client,
		nonAdminDeployment,
		func() error {
			nonAdminImage := r.getNonAdminImage(&dpa)
			if len(nonAdminImage) == 0 {
				return fmt.Errorf("no Non Admin Controller image found in RELATED_IMAGE_NON_ADMIN_CONTROLLER environment variable or unsupportedOverrides")
			}
			ensureRequiredLabels(nonAdminDeployment)
			ensureRequiredSpecs(nonAdminDeployment, nonAdminImage)

			// Setting controller owner reference on the non admin controller deployment
			return controllerutil.SetControllerReference(&dpa, nonAdminDeployment, r.Scheme)
		},
	)
	if err != nil {
		return false, err
	}

	if operation != controllerutil.OperationResultNone {
		r.EventRecorder.Event(
			nonAdminDeployment,
			corev1.EventTypeNormal,
			"NonAdminDeploymentReconciled",
			fmt.Sprintf("Non admin controller deployment %s/%s %s", nonAdminDeployment.Namespace, nonAdminDeployment.Name, operation),
		)
	}
	return true, nil
}

func ensureRequiredLabels(deploymentObject *appsv1.Deployment) {
	maps.Copy(deploymentLabels, controlPlaneLabel)
	deploymentObjectLabels := deploymentObject.GetLabels()
	if deploymentObjectLabels == nil {
		deploymentObject.SetLabels(deploymentLabels)
	} else {
		for key, value := range deploymentLabels {
			deploymentObjectLabels[key] = value
		}
		deploymentObject.SetLabels(deploymentObjectLabels)
	}
}

func ensureRequiredSpecs(deploymentObject *appsv1.Deployment, image string) {
	deploymentObject.Spec.Replicas = pointer.Int32(1)
	deploymentObject.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: controlPlaneLabel,
	}
	templateObjectLabels := deploymentObject.Spec.Template.GetLabels()
	if templateObjectLabels == nil {
		deploymentObject.Spec.Template.SetLabels(controlPlaneLabel)
	} else {
		templateObjectLabels[controlPlaneKey] = controlPlaneLabel[controlPlaneKey]
		deploymentObject.Spec.Template.SetLabels(templateObjectLabels)
	}
	if len(deploymentObject.Spec.Template.Spec.Containers) == 0 {
		deploymentObject.Spec.Template.Spec.Containers = []corev1.Container{{
			Name:            containerName,
			Image:           image,
			ImagePullPolicy: corev1.PullAlways,
		}}
	} else {
		for _, container := range deploymentObject.Spec.Template.Spec.Containers {
			if container.Name == containerName {
				container.Image = image
				container.ImagePullPolicy = corev1.PullAlways
				break
			}
		}
	}
	deploymentObject.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways
	deploymentObject.Spec.Template.Spec.ServiceAccountName = nonAdminControllerManager
}

func (r *DPAReconciler) checkNonAdminEnabled(dpa *oadpv1alpha1.DataProtectionApplication) bool {
	// TODO https://github.com/openshift/oadp-operator/pull/1316
	if dpa.Spec.Features != nil && dpa.Spec.Features.EnableNonAdmin != nil {
		return *dpa.Spec.Features.EnableNonAdmin
	}

	return false
}

func (r *DPAReconciler) getNonAdminImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	// TODO https://github.com/openshift/oadp-operator/pull/1316
	unsupportedOverride := dpa.Spec.UnsupportedOverrides[oadpv1alpha1.NonAdminControllerImageKey]
	if unsupportedOverride != "" {
		return unsupportedOverride
	}

	environmentVariable := os.Getenv("RELATED_IMAGE_NON_ADMIN_CONTROLLER")
	return environmentVariable
}

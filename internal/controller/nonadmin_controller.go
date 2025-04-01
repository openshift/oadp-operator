package controller

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
)

const (
	nonAdminObjectName = "non-admin-controller"
	controlPlaneKey    = "control-plane"

	dpaResourceVersionAnnotation = oadpv1alpha1.OadpOperatorLabel + "-dpa-resource-version"
)

var (
	controlPlaneLabel = map[string]string{
		controlPlaneKey: nonAdminObjectName,
	}
	deploymentLabels = map[string]string{
		"app.kubernetes.io/component":  "manager",
		"app.kubernetes.io/created-by": common.OADPOperator,
		"app.kubernetes.io/instance":   nonAdminObjectName,
		"app.kubernetes.io/managed-by": "kustomize",
		"app.kubernetes.io/name":       "deployment",
		"app.kubernetes.io/part-of":    common.OADPOperator,
	}

	dpaResourceVersion                                   = ""
	previousNonAdminConfiguration *oadpv1alpha1.NonAdmin = nil
	previousDefaultBSLSyncPeriod  *time.Duration         = nil
)

func (r *DataProtectionApplicationReconciler) ReconcileNonAdminController(log logr.Logger) (bool, error) {
	nonAdminDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nonAdminObjectName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Delete (possible) previously deployment
	if !r.checkNonAdminEnabled() {
		if err := r.Get(
			r.Context,
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

		if err := r.Delete(
			r.Context,
			nonAdminDeployment,
			&client.DeleteOptions{PropagationPolicy: ptr.To(metav1.DeletePropagationForeground)},
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
			err := r.buildNonAdminDeployment(nonAdminDeployment)
			if err != nil {
				return err
			}

			// Setting controller owner reference on the non admin controller deployment
			return controllerutil.SetControllerReference(r.dpa, nonAdminDeployment, r.Scheme)
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

func (r *DataProtectionApplicationReconciler) buildNonAdminDeployment(deploymentObject *appsv1.Deployment) error {
	nonAdminImage := r.getNonAdminImage()
	imagePullPolicy, err := common.GetImagePullPolicy(r.dpa.Spec.ImagePullPolicy, nonAdminImage)
	if err != nil {
		r.Log.Error(err, "imagePullPolicy regex failed")
	}
	ensureRequiredLabels(deploymentObject)
	err = ensureRequiredSpecs(deploymentObject, r.dpa, nonAdminImage, imagePullPolicy)
	if err != nil {
		return err
	}
	return nil
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

func ensureRequiredSpecs(deploymentObject *appsv1.Deployment, dpa *oadpv1alpha1.DataProtectionApplication, image string, imagePullPolicy corev1.PullPolicy) error {
	envVars := []corev1.EnvVar{
		{
			Name:  "WATCH_NAMESPACE", // TODO: fix, this is only used to indicate oadp ns, and is not actually used for watching ns.
			Value: deploymentObject.Namespace,
		},
	}
	if dpa.Spec.Configuration != nil && dpa.Spec.Configuration.Velero != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: common.LogLevelEnvVar,
			Value: func() string {
				// these levels are already validated in another controller.
				level, err := logrus.ParseLevel(dpa.Spec.Configuration.Velero.LogLevel)
				if err != nil {
					return ""
				}
				return strconv.FormatUint(uint64(level), 10)
			}(),
		})
	}

	if len(dpa.Spec.LogFormat) > 0 {
		envVars = append(envVars, corev1.EnvVar{
			Name:  common.LogFormatEnvVar,
			Value: string(dpa.Spec.LogFormat),
		})
	}

	if len(dpaResourceVersion) == 0 ||
		!reflect.DeepEqual(dpa.Spec.NonAdmin, previousNonAdminConfiguration) ||
		(dpa.Spec.Configuration.Velero.Args != nil &&
			!reflect.DeepEqual(dpa.Spec.Configuration.Velero.Args.BackupSyncPeriod, previousDefaultBSLSyncPeriod)) {
		dpaResourceVersion = dpa.GetResourceVersion()
		previousNonAdminConfiguration = dpa.Spec.NonAdmin
		if dpa.Spec.Configuration.Velero.Args != nil {
			previousDefaultBSLSyncPeriod = dpa.Spec.Configuration.Velero.Args.BackupSyncPeriod
		}
	}
	podAnnotations := map[string]string{
		dpaResourceVersionAnnotation: dpaResourceVersion,
	}

	deploymentObject.Spec.Replicas = ptr.To(int32(1))
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

	templateObjectAnnotations := deploymentObject.Spec.Template.GetAnnotations()
	if templateObjectAnnotations == nil {
		deploymentObject.Spec.Template.SetAnnotations(podAnnotations)
	} else {
		templateObjectAnnotations[dpaResourceVersionAnnotation] = podAnnotations[dpaResourceVersionAnnotation]
		deploymentObject.Spec.Template.SetAnnotations(templateObjectAnnotations)
	}

	nonAdminContainerFound := false
	if len(deploymentObject.Spec.Template.Spec.Containers) == 0 {
		deploymentObject.Spec.Template.Spec.Containers = []corev1.Container{{
			Name:                     nonAdminObjectName,
			Image:                    image,
			ImagePullPolicy:          imagePullPolicy,
			Env:                      envVars,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		}}
		nonAdminContainerFound = true
	} else {
		for index, container := range deploymentObject.Spec.Template.Spec.Containers {
			if container.Name == nonAdminObjectName {
				nonAdminContainer := &deploymentObject.Spec.Template.Spec.Containers[index]
				nonAdminContainer.Image = image
				nonAdminContainer.ImagePullPolicy = imagePullPolicy
				nonAdminContainer.Env = envVars
				nonAdminContainer.TerminationMessagePolicy = corev1.TerminationMessageFallbackToLogsOnError
				nonAdminContainerFound = true
				break
			}
		}
	}
	if !nonAdminContainerFound {
		return fmt.Errorf("could not find Non admin container in Deployment")
	}
	deploymentObject.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways
	deploymentObject.Spec.Template.Spec.ServiceAccountName = nonAdminObjectName
	return nil
}

func (r *DataProtectionApplicationReconciler) checkNonAdminEnabled() bool {
	if r.dpa.Spec.NonAdmin != nil && r.dpa.Spec.NonAdmin.Enable != nil {
		return *r.dpa.Spec.NonAdmin.Enable
	}
	return false
}

func (r *DataProtectionApplicationReconciler) getNonAdminImage() string {
	dpa := r.dpa
	unsupportedOverride := dpa.Spec.UnsupportedOverrides[oadpv1alpha1.NonAdminControllerImageKey]
	if unsupportedOverride != "" {
		return unsupportedOverride
	}

	environmentVariable := os.Getenv("RELATED_IMAGE_NON_ADMIN_CONTROLLER")
	if environmentVariable != "" {
		return environmentVariable
	}

	// TODO https://github.com/openshift/oadp-operator/issues/1379
	return "quay.io/konveyor/oadp-non-admin:latest"
}

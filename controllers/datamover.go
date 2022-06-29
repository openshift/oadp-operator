package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	ResticPassword   = "RESTIC_PASSWORD"
	ResticRepository = "RESTIC_REPOSITORY"
)

func (r *DPAReconciler) ReconcileDataMoverController(log logr.Logger) (bool, error) {

	// fetch latest DPA instance
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	dataMoverDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.DataMover,
			Namespace: dpa.Namespace,
		},
	}

	if (dpa.Spec.Features == nil) || (dpa.Spec.Features != nil && !dpa.Spec.Features.EnableDataMover) {
		deleteContext := context.Background()
		if err := r.Get(deleteContext, types.NamespacedName{
			Name:      dataMoverDeployment.Name,
			Namespace: r.NamespacedName.Namespace,
		}, dataMoverDeployment); err != nil {
			if k8serror.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}

		deleteOptionPropagationForeground := metav1.DeletePropagationForeground
		if err := r.Delete(deleteContext, dataMoverDeployment, &client.DeleteOptions{PropagationPolicy: &deleteOptionPropagationForeground}); err != nil {
			r.EventRecorder.Event(dataMoverDeployment, corev1.EventTypeNormal, "DeleteDataMoverDeploymentFailed", "Could not delete DataMover deployment:"+err.Error())
			return false, err
		}
		r.EventRecorder.Event(dataMoverDeployment, corev1.EventTypeNormal, "DeletedDataMoverDeploymentDeployment", "DataMover Deployment deleted")

		return true, nil
	}

	resticsecretname := "restic-secret"
	cred := dpa.Spec.Features.DataMoverCredential
	if cred != nil {
		resticsecretname = cred.Name
	}

	// Create restic secrets
	for _, bslSpec := range dpa.Spec.BackupLocations {

		rsecret, err := r.createResticSecretsPerBSL(bslSpec, resticsecretname)

		if err == nil {
			//add an event accordingly
			r.EventRecorder.Event(rsecret, corev1.EventTypeNormal, "CreatedResticSecret", "Restic Secret Created")

		}

	}
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, dataMoverDeployment, func() error {

		// Setting Deployment selector if a new object is created as it is immutable
		if dataMoverDeployment.ObjectMeta.CreationTimestamp.IsZero() {
			dataMoverDeployment.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"component": common.DataMoverController,
				},
			}
		}

		// update the Deployment template
		err := r.buildDataMoverDeployment(dataMoverDeployment, &dpa)
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
func (r *DPAReconciler) buildDataMoverDeployment(dataMoverDeployment *appsv1.Deployment, dpa *oadpv1alpha1.DataProtectionApplication) error {

	if dpa == nil {
		return fmt.Errorf("DPA CR cannot be nil")
	}
	if dataMoverDeployment == nil {
		return fmt.Errorf("datamover deployment cannot be nil")
	}

	//TODO: Add unsupportedoverrides support for datamover deployment image
	datamoverContainer := []corev1.Container{
		{
			Image:           r.getDataMoverImage(dpa),
			Name:            common.DataMoverControllerContainer,
			ImagePullPolicy: corev1.PullAlways,
		},
	}

	dataMoverDeployment.Labels = r.getDataMoverLabels()
	dataMoverDeployment.Spec = appsv1.DeploymentSpec{
		Selector: dataMoverDeployment.Spec.Selector,
		Replicas: pointer.Int32(1),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"component": common.DataMoverController,
				},
			},
			Spec: corev1.PodSpec{
				RestartPolicy:      corev1.RestartPolicyAlways,
				Containers:         datamoverContainer,
				ServiceAccountName: common.OADPOperatorServiceAccount,
			},
		},
	}

	return nil
}

func (r *DPAReconciler) getDataMoverImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.DataMoverImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.DataMoverImageKey]
	}
	return common.DataMoverImage
}

func (r *DPAReconciler) getDataMoverLabels() map[string]string {
	labels := getAppLabels(common.DataMover)
	labels["app.kubernetes.io/name"] = common.OADPOperatorVelero
	labels["app.kubernetes.io/component"] = common.DataMover
	labels[oadpv1alpha1.DataMoverDeploymentLabel] = "True"
	return labels
}

// Check if there is a valid user supplied restic secret
func (r *DPAReconciler) validateDataMoverCredential(resticsecret *corev1.Secret) bool {
	if resticsecret == nil {
		return false
	}
	for key, val := range resticsecret.Data {
		if key == ResticPassword {
			if len(val) != 0 {
				return false
			}
		}
	}
	return true
}

func (r *DPAReconciler) createResticSecretsPerBSL(bsl oadpv1alpha1.BackupLocation, resticsecretname string) (*corev1.Secret, error) {
	secretName, secretKey := r.getSecretNameAndKeyforBackupLocation(bsl)
	bslSecret, err := r.getProviderSecret(secretName)
	if err != nil {
		return nil, err
	}

	switch bsl.Velero.Provider {
	case AWSProvider:
		awsProfile := "default"

		if value, exists := bsl.Velero.Config[Profile]; exists {
			awsProfile = value
		}

		key, secret, err := r.parseAWSSecret(bslSecret, secretKey, awsProfile)
		if err != nil {
			r.Log.Info(fmt.Sprintf("Error parsing provider secret %s for backupstoragelocation", secretName))
			return nil, err
		}
		rsecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-secret", <name>),
				Namespace: ,
				Labels: map[string]string{
					label: name,
				},
			},
			Type: corev1.SecretTypeOpaque,
		}

	}

	rsecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-secret"),
			Namespace: namespace,
			Labels: map[string]string{
				label: name,
			},
		},
		Type: corev1.SecretTypeOpaque,
	}

	return nil, nil
}

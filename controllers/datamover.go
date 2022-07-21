package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	ResticPassword   = "RESTIC_PASSWORD"
	ResticRepository = "RESTIC_REPOSITORY"
	ResticsecretName = "dm-credential"

	// AWS vars
	AWSAccessKey     = "AWS_ACCESS_KEY_ID"
	AWSSecretKey     = "AWS_SECRET_ACCESS_KEY"
	AWSDefaultRegion = "AWS_DEFAULT_REGION"

	// TODO: GCP and Azure
)

func (r *DPAReconciler) ReconcileDataMoverController(log logr.Logger) (bool, error) {

	// fetch latest DPA instance
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	// check volSync is installed/deployment exists to use data mover
	if dpa.Spec.Features.DataMover != nil && dpa.Spec.Features.DataMover.Enable {

		// create new client for deployments outside of adp namespace
		kubeConf := config.GetConfigOrDie()

		clientset, err := kubernetes.NewForConfig(kubeConf)
		if err != nil {
			return false, err
		}

		_, err = clientset.AppsV1().Deployments(common.VolSyncDeploymentNamespace).Get(context.TODO(), common.VolSyncDeploymentName, metav1.GetOptions{})
		if err != nil {

			if k8serror.IsNotFound(err) {

				return false, fmt.Errorf("volSync operator not found. Please install")
			}

			return false, err
		}
	}

	dataMoverDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.DataMover,
			Namespace: dpa.Namespace,
		},
	}

	if (dpa.Spec.Features == nil) || (dpa.Spec.Features != nil && dpa.Spec.Features.DataMover != nil && !dpa.Spec.Features.DataMover.Enable) {
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

	_, err = r.createResticSecret(&dpa)
	if err != nil {
		return false, err
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
			if len(val) == 0 {
				return false
			}
		}
	}
	return true
}

func (r *DPAReconciler) createResticSecretsPerBSL(dpa *oadpv1alpha1.DataProtectionApplication, bsl velerov1.BackupStorageLocation, dmresticsecretname string, pass []byte) (*corev1.Secret, error) {

	switch bsl.Spec.Provider {
	case AWSProvider:
		{
			secretName, secretKey := r.getSecretNameAndKey(&bsl.Spec, oadpv1alpha1.DefaultPluginAWS)
			bslSecret, err := r.getProviderSecret(secretName)
			if err != nil {
				return nil, err
			}

			awsProfile := "default"

			if value, exists := bsl.Spec.Config[Profile]; exists {
				awsProfile = value
			}

			key, secret, err := r.parseAWSSecret(bslSecret, secretKey, awsProfile)
			if err != nil {
				r.Log.Info(fmt.Sprintf("Error parsing provider secret %s for backupstoragelocation", secretName))
				return nil, err
			}
			repobase := "s3:s3.amazonaws.com/"
			if bsl.Spec.Config["s3Url"] != "" {
				repobase = bsl.Spec.Config["s3Url"]
			}

			repo := repobase + bsl.Spec.ObjectStorage.Bucket
			rsecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-volsync-restic", bsl.Name),
					Namespace: bsl.Namespace,
					Labels: map[string]string{
						oadpv1alpha1.OadpBSLnameLabel: bsl.Name,
					},
				},
			}

			op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, rsecret, func() error {

				err := controllerutil.SetControllerReference(dpa, rsecret, r.Scheme)
				if err != nil {
					return err
				}
				// TODO: move to a separate fn & add gcp, azure support
				rData := &corev1.Secret{
					Data: map[string][]byte{
						AWSAccessKey:     []byte(key),
						AWSSecretKey:     []byte(secret),
						AWSDefaultRegion: []byte(bsl.Spec.Config[Region]),
						ResticPassword:   pass,
						ResticRepository: []byte(repo),
					},
				}
				rsecret.Data = rData.Data
				return nil
			})

			if err != nil {
				return nil, err
			}
			if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
				r.EventRecorder.Event(rsecret,
					corev1.EventTypeNormal,
					"ResticSecretReconciled",
					fmt.Sprintf("%s restic secret %s", op, rsecret.Name),
				)
			}
		}
		// TODO: Azure & GCP
	}

	return nil, nil
}

func (r *DPAReconciler) createResticSecret(dpa *oadpv1alpha1.DataProtectionApplication) (bool, error) {

	// obtain restic secret name from the config
	// if no name is mentioned in the config, then assume default restic secret name
	dmresticsecretname := ResticsecretName
	name := dpa.Spec.Features.DataMover.CredentialName
	if name != "" {
		if len(name) > 0 {
			dmresticsecretname = name
		}

	}

	// fetch restic secret from the cluster
	resticSecret := corev1.Secret{}
	if err := r.Get(r.Context, types.NamespacedName{Namespace: r.NamespacedName.Namespace, Name: dmresticsecretname}, &resticSecret); err != nil {
		r.Log.Error(err, "unable to fetch Restic Secret")
		return false, err
	}

	// validate restic secret
	val := r.validateDataMoverCredential(&resticSecret)
	if !val {
		return false, fmt.Errorf("no password supplied in the restic secret")
	}

	// obtain the password from user supllied restic secret
	var res_pass []byte
	for key, val := range resticSecret.Data {

		if key == ResticPassword {
			res_pass = val
		}
	}
	// Filter bsl based on the labels and dpa name
	// For each bsl in the list, create a restic secret
	// Label each restic secret with bsl name
	bslLabels := map[string]string{
		"app.kubernetes.io/name":       common.OADPOperatorVelero,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  "bsl",
		"openshift.io/oadp":            "True",
	}
	bslListOptions := client.MatchingLabels(bslLabels)
	backupStorageLocationList := velerov1.BackupStorageLocationList{}

	// Fetch the configured backupstoragelocations
	if err := r.List(r.Context, &backupStorageLocationList, bslListOptions); err != nil {
		return false, err
	}

	for _, bsl := range backupStorageLocationList.Items {
		if strings.Contains(bsl.Name, dpa.Name) {
			_, err := r.createResticSecretsPerBSL(dpa, bsl, dmresticsecretname, res_pass)

			if err != nil {
				return false, err
			}
		}
	}

	return true, nil
}

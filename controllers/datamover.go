package controllers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

	// Azure vars
	AzureAccountName = "AZURE_ACCOUNT_NAME"
	AzureAccountKey  = "AZURE_ACCOUNT_KEY"
	// GCP vars
	GoogleApplicationCredentials = "GOOGLE_APPLICATION_CREDENTIALS"
)

type gcpCredentials struct {
	googleApplicationCredentials string
}

func (r *DPAReconciler) ReconcileDataMoverController(log logr.Logger) (bool, error) {

	// fetch latest DPA instance
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	// check volSync is installed/deployment exists to use data mover
	if r.checkIfDataMoverIsEnabled(&dpa) {

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

	if (dpa.Spec.Features == nil) || (dpa.Spec.Features.DataMover == nil) ||
		(dpa.Spec.Features != nil && dpa.Spec.Features.DataMover != nil && !dpa.Spec.Features.DataMover.Enable) {
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

	op, err := controllerutil.CreateOrPatch(r.Context, r.Client, dataMoverDeployment, func() error {

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
		if k8serror.IsInvalid(err) {
			cause, isStatusCause := k8serror.StatusCause(err, metav1.CauseTypeFieldValueInvalid)
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
	if os.Getenv("RELATED_IMAGE_VOLUME_SNAPSHOT_MOVER") == "" {
		return common.DataMoverImage
	}
	return os.Getenv("RELATED_IMAGE_VOLUME_SNAPSHOT_MOVER")
}

func (r *DPAReconciler) getDataMoverLabels() map[string]string {
	labels := getAppLabels(common.DataMover)
	labels["app.kubernetes.io/name"] = common.OADPOperatorVelero
	labels["app.kubernetes.io/component"] = common.DataMover
	labels[oadpv1alpha1.DataMoverDeploymentLabel] = "True"
	return labels
}

// Check if there is a valid user supplied restic secret
func (r *DPAReconciler) validateDataMoverCredential(resticsecret *corev1.Secret) (bool, error) {
	if resticsecret == nil {
		return false, fmt.Errorf("restic secret not present")
	}
	if resticsecret.Data == nil {
		return false, fmt.Errorf("restic secret data is empty")
	}
	found := false
	for key, val := range resticsecret.Data {

		if key == ResticPassword {
			found = true
			if len(val) == 0 {
				return false, fmt.Errorf("malformed restic secret password")
			}
		}
	}
	if !found {
		return false, fmt.Errorf("restic secret password key is missing")
	}
	return true, nil
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
			repobase := "s3.amazonaws.com"

			if bsl.Spec.Config["s3Url"] != "" {
				repobase = bsl.Spec.Config["s3Url"]
				repobase = strings.TrimPrefix(repobase, "s3:")
			}
			repobase = strings.TrimSuffix(repobase, "/")
			repo := "s3:" + repobase + "/" + bsl.Spec.ObjectStorage.Bucket
			rsecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-volsync-restic", bsl.Name),
					Namespace: bsl.Namespace,
					Labels: map[string]string{
						oadpv1alpha1.OadpBSLnameLabel:     bsl.Name,
						oadpv1alpha1.OadpOperatorLabel:    "True",
						oadpv1alpha1.OadpBSLProviderLabel: bsl.Spec.Provider,
					},
				},
			}

			op, err := controllerutil.CreateOrPatch(r.Context, r.Client, rsecret, func() error {

				err := controllerutil.SetControllerReference(dpa, rsecret, r.Scheme)
				if err != nil {
					return err
				}

				return r.buildDataMoverResticSecretForAWS(rsecret, key, secret, bsl.Spec.Config[Region], pass, repo)
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
	case AzureProvider:
		{
			secretName, secretKey := r.getSecretNameAndKey(&bsl.Spec, oadpv1alpha1.DefaultPluginMicrosoftAzure)
			bslSecret, err := r.getProviderSecret(secretName)
			if err != nil {
				return nil, err
			}

			// parse the secret and get azure storage account key
			azcreds, err := r.parseAzureSecret(bslSecret, secretKey)
			if err != nil {
				r.Log.Info(fmt.Sprintf("Error parsing provider secret %s for backupstoragelocation %s/%s", secretName, bsl.Namespace, bsl.Name))
				return nil, err
			}

			// check for AZURE_ACCOUNT_NAME from BSL
			if len(bsl.Spec.Config["storageAccount"]) == 0 {
				return nil, errors.New("no storageAccount value present in backupstoragelocation config")
			}

			// check for AZURE_STORAGE_ACCOUNT_ACCESS_KEY value
			if len(bsl.Spec.Config["storageAccountKeyEnvVar"]) != 0 {
				if azcreds.strorageAccountKey == "" {
					r.Log.Info("Expecting storageAccountKeyEnvVar value set present in the credentials")
					return nil, errors.New("no strorageAccountKey value present in credentials file")
				}
			}

			accountName := bsl.Spec.Config["storageAccount"]
			accountKey := azcreds.strorageAccountKey

			// lets construct the repo URL
			repo := "azure:" + bsl.Spec.ObjectStorage.Bucket + ":"

			// We are done with checks no lets create the azure dm secret
			rsecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-volsync-restic", bsl.Name),
					Namespace: bsl.Namespace,
					Labels: map[string]string{
						oadpv1alpha1.OadpBSLnameLabel:     bsl.Name,
						oadpv1alpha1.OadpOperatorLabel:    "True",
						oadpv1alpha1.OadpBSLProviderLabel: bsl.Spec.Provider,
					},
				},
			}

			op, err := controllerutil.CreateOrPatch(r.Context, r.Client, rsecret, func() error {

				err := controllerutil.SetControllerReference(dpa, rsecret, r.Scheme)
				if err != nil {
					return err
				}

				return r.buildDataMoverResticSecretForAzure(rsecret, accountName, accountKey, pass, repo)
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
	case GCPProvider:
		{
			secretName, secretKey := r.getSecretNameAndKey(&bsl.Spec, oadpv1alpha1.DefaultPluginGCP)
			bslSecret, err := r.getProviderSecret(secretName)
			if err != nil {
				return nil, err
			}

			// parse the secret and get google application credentials json
			gcpcreds, err := r.parseGCPSecret(bslSecret, secretKey)
			if err != nil {
				r.Log.Info(fmt.Sprintf("Error parsing provider secret %s for backupstoragelocation %s/%s", secretName, bsl.Namespace, bsl.Name))
				return nil, err
			}

			// let's construct the repo URL
			repo := "gs:" + bsl.Spec.ObjectStorage.Bucket + ":"

			// We are done with checks no lets create the gcp dm secret
			rsecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-volsync-restic", bsl.Name),
					Namespace: bsl.Namespace,
					Labels: map[string]string{
						oadpv1alpha1.OadpBSLnameLabel:     bsl.Name,
						oadpv1alpha1.OadpOperatorLabel:    "True",
						oadpv1alpha1.OadpBSLProviderLabel: bsl.Spec.Provider,
					},
				},
			}

			op, err := controllerutil.CreateOrPatch(r.Context, r.Client, rsecret, func() error {

				err := controllerutil.SetControllerReference(dpa, rsecret, r.Scheme)
				if err != nil {
					return err
				}

				return r.buildDataMoverResticSecretForGCP(rsecret, gcpcreds.googleApplicationCredentials, pass, repo)
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
	}

	return nil, nil
}

// build data mover restic secret for given aws bsl
func (r *DPAReconciler) buildDataMoverResticSecretForAWS(rsecret *corev1.Secret, key string, secret string, region string, pass []byte, repo string) error {

	// TODO: add gcp, azure support
	rData := &corev1.Secret{
		Data: map[string][]byte{
			AWSAccessKey:     []byte(key),
			AWSSecretKey:     []byte(secret),
			AWSDefaultRegion: []byte(region),
			ResticPassword:   pass,
			ResticRepository: []byte(repo),
		},
	}
	rsecret.Data = rData.Data
	return nil
}

// build data mover restic secret for given bsl
func (r *DPAReconciler) buildDataMoverResticSecretForAzure(rsecret *corev1.Secret, accountName string, accountKey string, pass []byte, repo string) error {

	rData := &corev1.Secret{
		Data: map[string][]byte{
			AzureAccountName: []byte(accountName),
			AzureAccountKey:  []byte(accountKey),
			ResticPassword:   pass,
			ResticRepository: []byte(repo),
		},
	}
	rsecret.Data = rData.Data
	return nil
}

// build data mover restic secret for given gcp bsl
func (r *DPAReconciler) buildDataMoverResticSecretForGCP(rsecret *corev1.Secret, googleApplicationCredentials string, pass []byte, repo string) error {

	rData := &corev1.Secret{
		Data: map[string][]byte{
			GoogleApplicationCredentials: []byte(googleApplicationCredentials),
			ResticPassword:               pass,
			ResticRepository:             []byte(repo),
		},
	}
	rsecret.Data = rData.Data
	return nil
}

func (r *DPAReconciler) ReconcileDataMoverResticSecret(log logr.Logger) (bool, error) {

	// fetch latest DPA instance
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	// create secret only if data mover is enabled
	if r.checkIfDataMoverIsEnabled(&dpa) {
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
		val, err := r.validateDataMoverCredential(&resticSecret)
		if !val {
			return false, err
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
				_, err := r.createResticSecretsPerBSL(&dpa, bsl, dmresticsecretname, res_pass)

				if err != nil {
					return false, err
				}
			}
		}

	}

	return true, nil
}

// Check if Data Mover feature is enable in the DPA config or not
func (r *DPAReconciler) checkIfDataMoverIsEnabled(dpa *oadpv1alpha1.DataProtectionApplication) bool {

	if dpa.Spec.Features != nil && dpa.Spec.Features.DataMover != nil &&
		dpa.Spec.Features.DataMover.Enable {
		return true
	}

	return false
}

func (r *DPAReconciler) parseGCPSecret(secret corev1.Secret, secretKey string) (gcpCredentials, error) {

	gcpcreds := gcpCredentials{}

	gcpcreds.googleApplicationCredentials = string(secret.Data[secretKey])

	return gcpcreds, nil
}

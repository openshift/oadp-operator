package controllers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// provider specific object storage
const (
	AWSProfile            = "profile"
	AWSRegion             = "region"
	CredentialsFileKey    = "credentialsFile"
	EnableSharedConfigKey = "enableSharedConfig"
	GCPSnapshotLocation   = "snapshotLocation"
	GCPProject            = "project"
	AzureApiTimeout       = "apiTimeout"
	AzureSubscriptionId   = "subscriptionId"
	AzureIncremental      = "incremental"
	AzureResourceGroup    = "resourceGroup"
)

var validAWSKeys = map[string]bool{
	AWSProfile:            true,
	AWSRegion:             true,
	CredentialsFileKey:    true,
	EnableSharedConfigKey: true,
}

var validGCPKeys = map[string]bool{
	GCPProject:          true,
	GCPSnapshotLocation: true,
}

var validAzureKeys = map[string]bool{
	AzureApiTimeout:     true,
	AzureIncremental:    true,
	AzureSubscriptionId: true,
	AzureResourceGroup:  true,
}

func (r *DPAReconciler) LabelVSLSecrets(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	for _, vsl := range dpa.Spec.SnapshotLocations {
		provider := strings.TrimPrefix(vsl.Velero.Provider, "velero.io")
		switch provider {
		case "aws":
			secretName := credentials.PluginSpecificFields[oadpv1alpha1.DefaultPluginAWS].SecretName
			_, err := r.UpdateCredentialsSecretLabels(secretName, dpa.Namespace, dpa.Name)
			if err != nil {
				return false, err
			}
		case "gcp":
			secretName := credentials.PluginSpecificFields[oadpv1alpha1.DefaultPluginGCP].SecretName
			_, err := r.UpdateCredentialsSecretLabels(secretName, dpa.Namespace, dpa.Name)
			if err != nil {
				return false, err
			}
		case "azure":
			secretName := credentials.PluginSpecificFields[oadpv1alpha1.DefaultPluginMicrosoftAzure].SecretName
			_, err := r.UpdateCredentialsSecretLabels(secretName, dpa.Namespace, dpa.Name)
			if err != nil {
				return false, err
			}
		}

	}
	return true, nil
}

func (r *DPAReconciler) ValidateVolumeSnapshotLocations(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}
	if dpa.Spec.Configuration == nil {
		return false, errors.New("application configuration not found")
	}
	if dpa.Spec.Configuration.Velero == nil {
		return false, errors.New("velero configuration not found")
	}
	for i, vslSpec := range dpa.Spec.SnapshotLocations {
		vsl := velerov1.VolumeSnapshotLocation{
			ObjectMeta: metav1.ObjectMeta{
				// TODO: Use a hash instead of i
				Name:      fmt.Sprintf("%s-%d", r.NamespacedName.Name, i+1),
				Namespace: r.NamespacedName.Namespace,
			},
			Spec: *vslSpec.Velero,
		}

		// check for valid provider
		if vslSpec.Velero.Provider != AWSProvider && vslSpec.Velero.Provider != GCPProvider &&
			vslSpec.Velero.Provider != Azure {
			r.Log.Info("Non-supported provider specified, might be a misconfiguration")

			r.EventRecorder.Event(&vsl,
				corev1.EventTypeWarning,
				"VSL provider is invalid",
				fmt.Sprintf("VSL provider %s is invalid, might be a misconfiguration", vslSpec.Velero.Provider),
			)
		}

		//AWS
		if vslSpec.Velero.Provider == AWSProvider {
			//in AWS, region is a required field
			if len(vslSpec.Velero.Config[AWSRegion]) == 0 {
				return false, errors.New("region for AWS VSL is not configured, please ensure a region is configured")
			}

			// check for invalid config key
			for key := range vslSpec.Velero.Config {
				valid := validAWSKeys[key]
				if !valid {
					return false, fmt.Errorf("%s is not a valid AWS config value", key)
				}
			}
			//checking the aws plugin, if not present, throw warning message
			if !containsPlugin(dpa.Spec.Configuration.Velero.DefaultPlugins, AWSProvider) {
				r.Log.Info("VSL for AWS specified, but AWS plugin not present, might be a misconfiguration")

				r.EventRecorder.Event(&vsl,
					corev1.EventTypeWarning,
					"VolumeSnapshotLocation is invalid",
					fmt.Sprintf("could not validate vsl for AWS plugin on: %s/%s", vsl.Namespace, vsl.Name),
				)
			}
		}

		//GCP
		if vslSpec.Velero.Provider == GCPProvider {

			// check for invalid config key
			for key := range vslSpec.Velero.Config {
				valid := validGCPKeys[key]
				if !valid {
					return false, fmt.Errorf("%s is not a valid GCP config value", key)
				}
			}
			//checking the gcp plugin, if not present, throw warning message
			if !containsPlugin(dpa.Spec.Configuration.Velero.DefaultPlugins, "gcp") {
				r.Log.Info("VSL for GCP specified, but GCP plugin not present, might be a misconfiguration")

				r.EventRecorder.Event(&vsl,
					corev1.EventTypeWarning,
					"VolumeSnapshotLocation is invalid",
					fmt.Sprintf("could not validate vsl for GCP plugin on: %s/%s", vsl.Namespace, vsl.Name),
				)
			}
		}

		//Azure
		if vslSpec.Velero.Provider == Azure {

			// check for invalid config key
			for key := range vslSpec.Velero.Config {
				valid := validAzureKeys[key]
				if !valid {
					return false, fmt.Errorf("%s is not a valid Azure config value", key)
				}
			}
			//checking the azure plugin, if not present, throw warning message
			if !containsPlugin(dpa.Spec.Configuration.Velero.DefaultPlugins, "azure") {
				r.Log.Info("VSL for Azure specified, but Azure plugin not present, might be a misconfiguration")

				r.EventRecorder.Event(&vsl,
					corev1.EventTypeWarning,
					"VolumeSnapshotLocation is invalid",
					fmt.Sprintf("could not validate vsl for Azure plugin on: %s/%s", vsl.Namespace, vsl.Name),
				)
			}
		}
	}
	return true, nil
}

func (r *DPAReconciler) ReconcileVolumeSnapshotLocations(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	// Loop through all configured VSLs
	for i, vslSpec := range dpa.Spec.SnapshotLocations {
		// Create VSL as is, we can safely assume they are valid from
		// ValidateVolumeSnapshotLocations
		vsl := velerov1.VolumeSnapshotLocation{
			ObjectMeta: metav1.ObjectMeta{
				// TODO: Use a hash instead of i
				Name:      fmt.Sprintf("%s-%d", r.NamespacedName.Name, i+1),
				Namespace: r.NamespacedName.Namespace,
			},
			Spec: *vslSpec.Velero,
		}
		// Create VSL
		op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &vsl, func() error {
			// TODO: Velero may be setting controllerReference as
			// well and taking ownership. If so move this to
			// SetOwnerReference instead

			// Set controller reference to Velero controller
			err := controllerutil.SetControllerReference(&dpa, &vsl, r.Scheme)
			if err != nil {
				return err
			}
			// TODO: check for VSL status condition errors and respond here

			vsl.Spec = *vslSpec.Velero
			return nil
		})
		if err != nil {
			return false, err
		}
		if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
			// Trigger event to indicate VSL was created or updated
			r.EventRecorder.Event(&vsl,
				corev1.EventTypeNormal,
				"VolumeSnapshotLocationReconciled",
				fmt.Sprintf("performed %s on volumesnapshotlocation %s/%s", op, vsl.Namespace, vsl.Name),
			)
		}

	}
	return true, nil
}

func containsPlugin(d []oadpv1alpha1.DefaultPlugin, value string) bool {
	for _, elem := range d {
		if string(elem) == value {
			return true
		}
	}
	return false
}

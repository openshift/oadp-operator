package controllers

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	"github.com/openshift/oadp-operator/pkg/credentials"
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
	dpa := r.dpa
	for _, vsl := range dpa.Spec.SnapshotLocations {
		provider := strings.TrimPrefix(vsl.Velero.Provider, veleroIOPrefix)
		switch provider {
		case "aws":
			if vsl.Velero.Credential != nil {
				secretName := vsl.Velero.Credential.Name
				_, err := r.UpdateCredentialsSecretLabels(secretName, dpa.Namespace, dpa.Name)
				if err != nil {
					return false, err
				}
			} else {
				secretName := credentials.PluginSpecificFields[oadpv1alpha1.DefaultPluginAWS].SecretName
				_, err := r.UpdateCredentialsSecretLabels(secretName, dpa.Namespace, dpa.Name)
				if err != nil {
					return false, err
				}
			}
		case "gcp":
			if vsl.Velero.Credential != nil {
				secretName := vsl.Velero.Credential.Name
				_, err := r.UpdateCredentialsSecretLabels(secretName, dpa.Namespace, dpa.Name)
				if err != nil {
					return false, err
				}
			} else {
				secretName := credentials.PluginSpecificFields[oadpv1alpha1.DefaultPluginGCP].SecretName
				_, err := r.UpdateCredentialsSecretLabels(secretName, dpa.Namespace, dpa.Name)
				if err != nil {
					return false, err
				}
			}
		case "azure":
			if vsl.Velero.Credential != nil {
				secretName := vsl.Velero.Credential.Name
				_, err := r.UpdateCredentialsSecretLabels(secretName, dpa.Namespace, dpa.Name)
				if err != nil {
					return false, err
				}
			} else {
				secretName := credentials.PluginSpecificFields[oadpv1alpha1.DefaultPluginMicrosoftAzure].SecretName
				_, err := r.UpdateCredentialsSecretLabels(secretName, dpa.Namespace, dpa.Name)
				if err != nil {
					return false, err
				}
			}
		}

	}
	return true, nil
}

func (r *DPAReconciler) ValidateVolumeSnapshotLocations() (bool, error) {
	dpa := r.dpa
	for i, vslSpec := range dpa.Spec.SnapshotLocations {
		vslYAMLPath := fmt.Sprintf("spec.snapshotLocations[%v]", i)
		veleroVSLYAMLPath := vslYAMLPath + ".velero"
		veleroConfigYAMLPath := "spec.configuration.velero"

		if vslSpec.Velero == nil {
			return false, errors.New("snapshotLocation velero configuration cannot be nil")
		}

		// check for valid provider
		if vslSpec.Velero.Provider != AWSProvider && vslSpec.Velero.Provider != GCPProvider &&
			vslSpec.Velero.Provider != AzureProvider {
			return false, fmt.Errorf("DPA %s.provider %s is invalid: only %s, %s and %s are supported", veleroVSLYAMLPath, vslSpec.Velero.Provider, AWSProvider, GCPProvider, AzureProvider)
		}

		//AWS
		if vslSpec.Velero.Provider == AWSProvider {
			//in AWS, region is a required field
			if len(vslSpec.Velero.Config[AWSRegion]) == 0 {
				return false, fmt.Errorf("region for %s VSL in DPA %s.config is not configured, please ensure a region is configured", AWSProvider, veleroVSLYAMLPath)
			}

			// check for invalid config key
			for key := range vslSpec.Velero.Config {
				valid := validAWSKeys[key]
				if !valid {
					return false, fmt.Errorf("DPA %s.config key %s is not a valid %s config key", veleroVSLYAMLPath, key, AWSProvider)
				}
			}
			//checking the aws plugin, if not present, throw warning message
			if !containsPlugin(dpa.Spec.Configuration.Velero.DefaultPlugins, AWSProvider) {
				return false, fmt.Errorf("to use VSL for %s specified in DPA %s, %s plugin must be present in %s.defaultPlugins", AWSProvider, vslYAMLPath, AWSProvider, veleroConfigYAMLPath)
			}
		}

		//GCP
		if vslSpec.Velero.Provider == GCPProvider {

			// check for invalid config key
			for key := range vslSpec.Velero.Config {
				valid := validGCPKeys[key]
				if !valid {
					return false, fmt.Errorf("DPA %s.config key %s is not a valid %s config key", veleroVSLYAMLPath, key, GCPProvider)
				}
			}
			//checking the gcp plugin, if not present, throw warning message
			if !containsPlugin(dpa.Spec.Configuration.Velero.DefaultPlugins, "gcp") {

				return false, fmt.Errorf("to use VSL for %s specified in DPA %s, %s plugin must be present in %s.defaultPlugins", GCPProvider, vslYAMLPath, GCPProvider, veleroConfigYAMLPath)
			}
		}

		//Azure
		if vslSpec.Velero.Provider == AzureProvider {

			// check for invalid config key
			for key := range vslSpec.Velero.Config {
				valid := validAzureKeys[key]
				if !valid {
					return false, fmt.Errorf("DPA %s.config key %s is not a valid %s config key", veleroVSLYAMLPath, key, AzureProvider)
				}
			}
			//checking the azure plugin, if not present, throw warning message
			if !containsPlugin(dpa.Spec.Configuration.Velero.DefaultPlugins, "azure") {

				return false, fmt.Errorf("to use VSL for %s specified in DPA %s, %s plugin must be present in %s.defaultPlugins", AzureProvider, vslYAMLPath, AzureProvider, veleroConfigYAMLPath)
			}
		}

		if err := r.ensureVslSecretDataExists(&vslSpec); err != nil {
			return false, err
		}

	}
	return true, nil
}

func (r *DPAReconciler) ReconcileVolumeSnapshotLocations(log logr.Logger) (bool, error) {
	dpa := r.dpa
	dpaVSLNames := []string{}
	// Loop through all configured VSLs
	for i, vslSpec := range dpa.Spec.SnapshotLocations {
		// Create VSL as is, we can safely assume they are valid from
		// ValidateVolumeSnapshotLocations

		// check if VSL name is specified in DPA spec
		vslName := fmt.Sprintf("%s-%d", r.NamespacedName.Name, i+1)
		if vslSpec.Name != "" {
			vslName = vslSpec.Name
		}
		dpaVSLNames = append(dpaVSLNames, vslName)

		vsl := velerov1.VolumeSnapshotLocation{
			ObjectMeta: metav1.ObjectMeta{
				// TODO: Use a hash instead of i
				Name:      vslName,
				Namespace: r.NamespacedName.Namespace,
			},
			Spec: *vslSpec.Velero,
		}
		// Create VSL
		op, err := controllerutil.CreateOrPatch(r.Context, r.Client, &vsl, func() error {
			// TODO: Velero may be setting controllerReference as
			// well and taking ownership. If so move this to
			// SetOwnerReference instead

			// Set controller reference to Velero controller
			err := controllerutil.SetControllerReference(dpa, &vsl, r.Scheme)
			if err != nil {
				return err
			}
			// TODO: check for VSL status condition errors and respond here

			vsl.Labels = map[string]string{
				"app.kubernetes.io/name":       common.OADPOperatorVelero,
				"app.kubernetes.io/instance":   vslName,
				"app.kubernetes.io/managed-by": common.OADPOperator,
				"app.kubernetes.io/component":  "vsl",
				oadpv1alpha1.OadpOperatorLabel: "True",
			}

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

	dpaVSLs := velerov1.VolumeSnapshotLocationList{}
	dpaVslLabels := map[string]string{
		"app.kubernetes.io/name":       common.OADPOperatorVelero,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  "vsl",
	}
	err := r.List(r.Context, &dpaVSLs, client.InNamespace(r.NamespacedName.Namespace), client.MatchingLabels(dpaVslLabels))
	if err != nil {
		return false, err
	}

	// If current VSLs do not match the spec, delete extra VSLs
	if len(dpaVSLNames) != len(dpaVSLs.Items) {
		for _, vsl := range dpaVSLs.Items {
			if !slices.Contains(dpaVSLNames, vsl.Name) {
				if err := r.Delete(r.Context, &vsl); err != nil {
					return false, err
				}
				// Record event for VSL deletion
				r.EventRecorder.Event(&vsl,
					corev1.EventTypeNormal,
					"VolumeSnapshotLocationDeleted",
					fmt.Sprintf("VolumeSnapshotLocation %s created by OADP in namespace %s was deleted as it was not in DPA spec.", vsl.Name, vsl.Namespace))
			}
		}
	}

	return true, nil
}

func containsPlugin(d []oadpv1alpha1.DefaultPlugin, value string) bool {
	for _, elem := range d {
		if credentials.PluginSpecificFields[elem].ProviderName == value {
			return true
		}
	}
	return false
}

func (r *DPAReconciler) ensureVslSecretDataExists(vsl *oadpv1alpha1.SnapshotLocation) error {
	// Check if the Velero feature flag 'no-secret' is not set
	if !(r.dpa.Spec.Configuration.Velero.HasFeatureFlag("no-secret")) {
		// Check if the user specified credential under velero
		if vsl.Velero != nil && vsl.Velero.Credential != nil {
			// Check if user specified empty credential key
			if vsl.Velero.Credential.Key == "" {
				return fmt.Errorf("Secret key specified in SnapshotLocation cannot be empty")
			}
			// Check if user specified empty credential name
			if vsl.Velero.Credential.Name == "" {
				return fmt.Errorf("Secret name specified in SnapshotLocation cannot be empty")
			}

		}
		// Check if the VSL secret key configured in the DPA exists with a secret data
		if vsl.Velero != nil {
			_, _, err := r.getSecretNameAndKey(vsl.Velero.Config, vsl.Velero.Credential, oadpv1alpha1.DefaultPlugin(vsl.Velero.Provider))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

package controllers

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// provider specific object storage
const (
	Profile          = "profile"
	SnapshotLocation = "shapshotLocation"
	Project          = "project"
	ApiTimeout       = "apiTimeout"
	SubscriptionId   = "subscriptionId"
	Incremental      = "incremental"
)

func (r *VeleroReconciler) ValidateVolumeSnapshotLocations(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}
	// TODO: For each VSL, confirm for each selected provider, we have the
	// needed config values

	for i, vslSpec := range velero.Spec.VolumeSnapshotLocations {
		vsl := velerov1.VolumeSnapshotLocation{
			ObjectMeta: metav1.ObjectMeta{
				// TODO: Use a hash instead of i
				Name:      fmt.Sprintf("%s-%d", r.NamespacedName.Name, i+1),
				Namespace: r.NamespacedName.Namespace,
			},
			Spec: vslSpec,
		}

		// check for valid provider
		if vslSpec.Provider != AWSProvider && vslSpec.Provider != GCPProvider &&
			vslSpec.Provider != Azure {
			return false, errors.New("invalid provider for VSL")
		}

		//AWS
		if vslSpec.Provider == AWSProvider {

			//in AWS, region is a required field
			if len(vslSpec.Config[Region]) == 0 {
				return false, errors.New("region for AWS VSL is not configured, please ensure a region is configured")
			}
			// check for invalid config key
			for key := range vslSpec.Config {
				if key != Region && key != Profile {
					return false, errors.New("invalid AWS config value")
				}
			}
			//checking the aws plugin, if not present, throw warning message
			if !contains(velero.Spec.DefaultVeleroPlugins, AWSProvider) {
				r.Log.Info("VSL for AWS specified, but AWS plugin not present, might be a misconfiguration")

				r.EventRecorder.Event(&vsl,
					corev1.EventTypeWarning,
					"VolumeSnapshotLocation is invalid",
					fmt.Sprintf("could not validate vsl for AWS plugin on: %s/%s", vsl.Namespace, vsl.Name),
				)
			}
		}

		//GCP
		if vslSpec.Provider == GCPProvider {

			// no other required fields for gcp
			// check for invalid config key
			for key := range vslSpec.Config {
				if key != SnapshotLocation && key != Project {
					return false, errors.New("invalid GCP config value")
				}
			}
			//checking the gcp plugin, if not present, throw warning message
			if !contains(velero.Spec.DefaultVeleroPlugins, "gcp") {
				r.Log.Info("VSL for GCP specified, but GCP plugin not present, might be a misconfiguration")

				r.EventRecorder.Event(&vsl,
					corev1.EventTypeWarning,
					"VolumeSnapshotLocation is invalid",
					fmt.Sprintf("could not validate vsl for GCP plugin on: %s/%s", vsl.Namespace, vsl.Name),
				)
			}
		}

		//Azure
		if vslSpec.Provider == Azure {
			// no other required fields for gcp
			// check for invalid config key
			for key := range vslSpec.Config {
				if key != ApiTimeout && key != ResourceGroup &&
					key != SubscriptionId && key != Incremental {
					return false, errors.New("invalid Azure config value")
				}
			}
			//checking the azure plugin, if not present, throw warning message
			if !contains(velero.Spec.DefaultVeleroPlugins, "azure") {
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

func (r *VeleroReconciler) ReconcileVolumeSnapshotLocations(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	// Loop through all configured VSLs
	for i, vslSpec := range velero.Spec.VolumeSnapshotLocations {
		// Create VSL as is, we can safely assume they are valid from
		// ValidateVolumeSnapshotLocations
		vsl := velerov1.VolumeSnapshotLocation{
			ObjectMeta: metav1.ObjectMeta{
				// TODO: Use a hash instead of i
				Name:      fmt.Sprintf("%s-%d", r.NamespacedName.Name, i+1),
				Namespace: r.NamespacedName.Namespace,
			},
			Spec: vslSpec,
		}
		// Create VSL
		op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &vsl, func() error {
			// TODO: Velero may be setting controllerReference as
			// well and taking ownership. If so move this to
			// SetOwnerReference instead

			// Set controller reference to Velero controller
			err := controllerutil.SetControllerReference(&velero, &vsl, r.Scheme)
			if err != nil {
				return err
			}
			// TODO: check for VSL status condition errors and respond here

			vsl.Spec = vslSpec
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

func contains(d []oadpv1alpha1.DefaultPlugin, value string) bool {
	for _, elem := range d {
		if string(elem) == value {
			return true
		}
	}
	return false
}

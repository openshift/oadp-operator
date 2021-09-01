package controllers

import (
	//"errors"
	"fmt"
	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *VeleroReconciler) ValidateVolumeSnapshotLocations(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}
	// TODO: For each VSL, confirm for each selected provider, we have the
	// needed config values

	/*
		for _, vslSpec := range velero.Spec.VolumeSnapshotLocations {

			//AWS
			if vslSpec.Provider == "aws" {

				//validation for AWS
				//in AWS, region is a required field
				if len(vslSpec.Config["region"]) == 0 {
					return false, errors.New("region for AWS VSL is not configured, please ensure a region is configured")
				}

				//checking the aws plugin, if not present, throw warning message
				if !contains(velero.Spec.DefaultVeleroPlugins, "aws") {
					r.Log.Info("VSL for AWS specified, but AWS plugin not present, might be a misconfiguration")
				}

				//TODO: Add warn/error messages to Velero CR status field
			}

			//GCP
			if vslSpec.Provider == "gcp" {

				//validation for GCP
				if len(vslSpec.Config["region"]) == 0 {
					r.Log.Info("region for GCP VSL is not configured, please check if a region might be needed")
				}

				//checking the gcp plugin, if not present, throw warning message
				if !contains(velero.Spec.DefaultVeleroPlugins, "gcp") {
					r.Log.Info("VSL for GCP specified, but GCP plugin not present, might be a misconfiguration")
				}

				//TODO: Add warn/error messages to Velero CR status field

			}

			//Azure
			if vslSpec.Provider == "azure" {

			//validation for Azure
			if len(vslSpec.Config["region"]) == 0 {
				r.Log.Info("region for Azure VSL is not configured, please check if a region might be needed")
			}

				//checking the azure plugin, if not present, throw warning message
				if !contains(velero.Spec.DefaultVeleroPlugins, "azure") {
					r.Log.Info("VSL for Azure specified, but Azure plugin not present, might be a misconfiguration")
				}

				//TODO: Add warn/error messages to Velero CR status field
			}

		}*/

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
		// Create BSL
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

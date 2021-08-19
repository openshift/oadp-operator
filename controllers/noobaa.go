package controllers

import (
	"fmt"
	noobaa "github.com/noobaa/noobaa-operator/v2/pkg/apis/noobaa/v1alpha1"
	"github.com/go-logr/logr"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *VeleroReconciler) ValidateNoobaa(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	//Validation logic for noobaa
	//check if noobaa:true is present, if present proceed
	if velero.Spec.Noobaa {

		fmt.Println("Noobaa is true, perform validation below.")

		//check no vsl or bsl are present

		//default plugins should only consist of aws plugin	

		//enable_restic:true flag present

		//check if ocs and noobaa is up and running 
	}


	return true, nil
}

func (r *VeleroReconciler) ReconcileNoobaa(log logr.Logger) (bool, error) {
	velero := oadpv1alpha1.Velero{}
	noobaa := noobaa.NooBaa{}

	if err := r.Get(r.Context, r.NamespacedName, &velero); err != nil {
		return false, err
	}

	if err := r.Get(r.Context, r.NamespacedName, &noobaa); err != nil {
		return false, err
	}


	//Reconcile logic for Noobaa

	//check if noobaa:true flag is present, if present proceed
	if velero.Spec.Noobaa {

		//OADP creates a BackupStorageLocation that points to this bucket
		bsl := velerov1.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{
				// TODO: Use a hash instead of i
				Name:      fmt.Sprintf("%s-%d", r.NamespacedName.Name, i+1),
				Namespace: r.NamespacedName.Namespace,
			},
			Spec: velerov1.BackupStorageLocationSpec{
				Provider: "aws",
				StorageType: velerov1.StorageType{
					&velerov1.ObjectStorageLocation{
						Bucket: noobaa.NooBaaStatus.ServicesStatus.ServiceS3.ExternalDNS[0],
						Prefix: "velero" ,
					},
				},

				
				
			},
		}

		// Create BSL
		op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &bsl, func() error {
			// TODO: Velero may be setting controllerReference as
			// well and taking ownership. If so move this to
			// SetOwnerReference instead

			// TODO: check for BSL status condition errors and respond here

			err := r.updateBSLFromSpec(&bsl, &velero)

			return err
		})
		if err != nil {
			return false, err
		}

		if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
			// Trigger event to indicate BSL was created or updated
			r.EventRecorder.Event(&bsl,
				corev1.EventTypeNormal,
				"BackupStorageLocationReconciled",
				fmt.Sprintf("performed %s on backupstoragelocation %s/%s", op, bsl.Namespace, bsl.Name),
			)
		}
		
		
		//OADP creates cloud-credentials secret that points to this bucket

	}



	// op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, ds, func() error {

	// })

	// if err != nil {
	// 	return false, err
	// }

	// if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
	// 	// Trigger event to indicate restic was created or updated
	// 	r.EventRecorder.Event(ds,
	// 		v1.EventTypeNormal,
	// 		"ResticDaemonsetReconciled",
	// 		fmt.Sprintf("performed %s on restic deployment %s/%s", op, ds.Namespace, ds.Name),
	// 	)
	// }
		

	return true, nil
}

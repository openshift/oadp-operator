/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"

	routev1 "github.com/openshift/api/route/v1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"

	security "github.com/openshift/api/security/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

// VeleroReconciler reconciles a Velero object
type VeleroReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Log            logr.Logger
	Context        context.Context
	NamespacedName types.NamespacedName
	EventRecorder  record.EventRecorder
}

//TODO!!! FIX THIS!!!!

//+kubebuilder:rbac:groups=*,resources=*,verbs=*
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=veleroes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=use,resourceNames=privileged;velero-privileged
//+kubebuilder:rbac:groups=velero.io,resources=backups;restores;backupstoragelocations;volumesnapshotlocations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=veleroes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=veleroes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Velero object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *VeleroReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)
	log := r.Log.WithValues("velero", req.NamespacedName)
	result := ctrl.Result{}
	// Set reconciler context + name
	r.Context = ctx
	r.NamespacedName = req.NamespacedName
	velero := oadpv1alpha1.Velero{}

	if err := r.Get(ctx, req.NamespacedName, &velero); err != nil {
		log.Error(err, "unable to fetch velero CR")
		return result, client.IgnoreNotFound(err)
	}

	_, err := ReconcileBatch(r.Log,
		r.ValidateVeleroPlugins,
		r.ReconcileVeleroSecurityContextConstraint,
		r.ReconcileResticRestoreHelperConfig,
		r.ValidateBackupStorageLocations,
		r.ReconcileBackupStorageLocations,
		r.ReconcileRegistries,
		r.ReconcileRegistrySVCs,
		r.ReconcileRegistryRoutes,
		r.ReconcileRegistryRouteConfigs,
		r.ValidateVolumeSnapshotLocations,
		r.ReconcileVolumeSnapshotLocations,
		r.ReconcileVeleroDeployment,
		r.ReconcileResticDaemonset,
	)

	if err != nil {
		apimeta.SetStatusCondition(&velero.Status.Conditions,
			metav1.Condition{
				Type:    oadpv1alpha1.ConditionReconciled,
				Status:  metav1.ConditionFalse,
				Reason:  oadpv1alpha1.ReconciledReasonError,
				Message: err.Error(),
			},
		)

	} else {
		apimeta.SetStatusCondition(&velero.Status.Conditions,
			metav1.Condition{
				Type:    oadpv1alpha1.ConditionReconciled,
				Status:  metav1.ConditionTrue,
				Reason:  oadpv1alpha1.ReconciledReasonComplete,
				Message: oadpv1alpha1.ReconcileCompleteMessage,
			},
		)
	}
	statusErr := r.Client.Status().Update(ctx, &velero)
	if err == nil { // Don't mask previous error
		err = statusErr
	}

	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *VeleroReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&oadpv1alpha1.Velero{}).
		Owns(&appsv1.Deployment{}).
		Owns(&velerov1.BackupStorageLocation{}).
		Owns(&velerov1.VolumeSnapshotLocation{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&security.SecurityContextConstraints{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&routev1.Route{}).
		Owns(&corev1.ConfigMap{}).
		WithEventFilter(veleroPredicate(r.Scheme)).
		Complete(r)
}

type ReconcileFunc func(logr.Logger) (bool, error)

// reconcileBatch steps through a list of reconcile functions until one returns
// false or an error.
func ReconcileBatch(l logr.Logger, reconcileFuncs ...ReconcileFunc) (bool, error) {
	for _, f := range reconcileFuncs {
		if cont, err := f(l); !cont || err != nil {
			return cont, err
		}
	}
	return true, nil
}

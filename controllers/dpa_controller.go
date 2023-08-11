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
	"os"

	routev1 "github.com/openshift/api/route/v1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"

	security "github.com/openshift/api/security/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// DPAReconciler reconciles a Velero object
type DPAReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Log            logr.Logger
	Context        context.Context
	NamespacedName types.NamespacedName
	EventRecorder  record.EventRecorder
}

var debugMode = os.Getenv("DEBUG") == "true"

//TODO!!! FIX THIS!!!!

//+kubebuilder:rbac:groups=*,resources=*,verbs=*
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=dataprotectionapplications,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=use,resourceNames=privileged;velero-privileged
//+kubebuilder:rbac:groups=velero.io,resources=backups;restores;backupstoragelocations;volumesnapshotlocations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=dataprotectionapplications/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=dataprotectionapplications/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DataProtectionApplciation object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *DPAReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)
	log := r.Log.WithValues("dpa", req.NamespacedName)
	result := ctrl.Result{}
	// Set reconciler context + name
	r.Context = ctx
	r.NamespacedName = req.NamespacedName
	dpa := oadpv1alpha1.DataProtectionApplication{}

	if err := r.Get(ctx, req.NamespacedName, &dpa); err != nil {
		log.Error(err, "unable to fetch DataProtectionApplication CR")
		return result, nil
	}

	_, err := ReconcileBatch(r.Log,
		r.ValidateDataProtectionCR,
		r.ReconcileResticRestoreHelperConfig,
		r.ReconcileBackupStorageLocations,
		r.ReconcileRegistrySecrets,
		r.ReconcileRegistries,
		r.ReconcileRegistrySVCs,
		r.ReconcileRegistryRoutes,
		r.ReconcileRegistryRouteConfigs,
		r.LabelVSLSecrets,
		r.ReconcileVolumeSnapshotLocations,
		r.ReconcileVeleroDeployment,
		r.ReconcileResticDaemonset,
		r.ReconcileVeleroMetricsSVC,
		r.ReconcileDataMoverController,
		r.ReconcileDataMoverResticSecret,
		r.ReconcileDataMoverVolumeOptions,
	)

	if err != nil {
		apimeta.SetStatusCondition(&dpa.Status.Conditions,
			metav1.Condition{
				Type:    oadpv1alpha1.ConditionReconciled,
				Status:  metav1.ConditionFalse,
				Reason:  oadpv1alpha1.ReconciledReasonError,
				Message: err.Error(),
			},
		)

	} else {
		apimeta.SetStatusCondition(&dpa.Status.Conditions,
			metav1.Condition{
				Type:    oadpv1alpha1.ConditionReconciled,
				Status:  metav1.ConditionTrue,
				Reason:  oadpv1alpha1.ReconciledReasonComplete,
				Message: oadpv1alpha1.ReconcileCompleteMessage,
			},
		)
	}
	statusErr := r.Client.Status().Update(ctx, &dpa)
	if err == nil { // Don't mask previous error
		err = statusErr
	}

	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *DPAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&oadpv1alpha1.DataProtectionApplication{}).
		Owns(&appsv1.Deployment{}).
		Owns(&velerov1.BackupStorageLocation{}).
		Owns(&velerov1.VolumeSnapshotLocation{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&security.SecurityContextConstraints{}).
		Owns(&corev1.Service{}).
		Owns(&routev1.Route{}).
		Owns(&corev1.ConfigMap{}).
		Watches(&source.Kind{Type: &corev1.Secret{}}, &labelHandler{}).
		WithEventFilter(veleroPredicate(r.Scheme)).
		Complete(r)
}

type labelHandler struct {
}

func (l *labelHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	// check for the label & add it to the queue
	namespace := evt.Object.GetNamespace()
	dpaname := evt.Object.GetLabels()[namespace+".dataprotectionapplication"]
	if evt.Object.GetLabels()[oadpv1alpha1.OadpOperatorLabel] == "" || dpaname == "" {
		return
	}

	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      dpaname,
		Namespace: namespace,
	}})

}
func (l *labelHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {

	namespace := evt.Object.GetNamespace()
	dpaname := evt.Object.GetLabels()[namespace+".dataprotectionapplication"]
	if evt.Object.GetLabels()[oadpv1alpha1.OadpOperatorLabel] == "" || dpaname == "" {
		return
	}
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      dpaname,
		Namespace: namespace,
	}})

}
func (l *labelHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	namespace := evt.ObjectNew.GetNamespace()
	dpaname := evt.ObjectNew.GetLabels()[namespace+".dataprotectionapplication"]
	if evt.ObjectNew.GetLabels()[oadpv1alpha1.OadpOperatorLabel] == "" || dpaname == "" {
		return
	}
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      dpaname,
		Namespace: namespace,
	}})

}
func (l *labelHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {

	namespace := evt.Object.GetNamespace()
	dpaname := evt.Object.GetLabels()[namespace+".dataprotectionapplication"]
	if evt.Object.GetLabels()[oadpv1alpha1.OadpOperatorLabel] == "" || dpaname == "" {
		return
	}
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      dpaname,
		Namespace: namespace,
	}})

}

type ReconcileFunc func(logr.Logger) (bool, error)

// reconcileBatch steps through a list of reconcile functions until one returns
// false or an error.
func ReconcileBatch(l logr.Logger, reconcileFuncs ...ReconcileFunc) (bool, error) {
	// TODO: #1127 DPAReconciler already have a logger, use it instead of passing to each reconcile functions
	// TODO: #1128 Right now each reconcile functions call get for DPA, we can do it once and pass it to each function
	for _, f := range reconcileFuncs {
		if cont, err := f(l); !cont || err != nil {
			return cont, err
		}
	}
	return true, nil
}

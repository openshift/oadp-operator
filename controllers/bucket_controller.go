package controllers

import (
	"context"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	bucketpkg "github.com/openshift/oadp-operator/pkg/bucket"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// VeleroReconciler reconciles a Velero object
type BucketReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Log            logr.Logger
	Context        context.Context
	NamespacedName types.NamespacedName
	EventRecorder  record.EventRecorder
}

//TODO!!! FIX THIS!!!!

//+kubebuilder:rbac:groups=oadp.openshift.io,resources=buckets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=corev1,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=buckets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=buckets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Velero object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (b BucketReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	b.Log = log.FromContext(ctx)
	log := b.Log.WithValues("velero", req.NamespacedName)
	result := ctrl.Result{}
	// Set reconciler context + name
	b.Context = ctx
	b.NamespacedName = req.NamespacedName

	bucket := oadpv1alpha1.Bucket{}
	if err := b.Get(ctx, req.NamespacedName, &bucket); err != nil {
		log.Error(err, "unable to fetch velero CR")
		return result, client.IgnoreNotFound(err)
	}

	_, _ = bucketpkg.NewClient(bucket, b.Client)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (b *BucketReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&oadpv1alpha1.Bucket{}).
		Owns(&corev1.Secret{}).
		WithEventFilter(bucketPredicate()).
		Complete(b)
}

func bucketPredicate() predicate.Predicate {
	return predicate.Funcs{
		// Update returns true if the Update event should be processed
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
		// Create returns true if the Create event should be processed
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		// Delete returns true if the Delete event should be processed
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown
		},
	}
}

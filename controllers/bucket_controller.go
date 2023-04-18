package controllers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	bucketpkg "github.com/openshift/oadp-operator/pkg/bucket"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	oadpFinalizerBucket              = "oadp.openshift.io/bucket-protection"
	oadpCloudStorageDeleteAnnotation = "oadp.openshift.io/cloudstorage-delete"
)

// VeleroReconciler reconciles a Velero object
type BucketReconciler struct {
	Client        client.Client
	Scheme        *runtime.Scheme
	Log           logr.Logger
	EventRecorder record.EventRecorder
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
	log := b.Log.WithValues("bucket", req.NamespacedName)
	result := ctrl.Result{}
	// Set reconciler context + name

	bucket := oadpv1alpha1.CloudStorage{}

	if err := b.Client.Get(ctx, req.NamespacedName, &bucket); err != nil {
		log.Error(err, "unable to fetch bucket CR")
		return result, nil
	}

	// Add finalizer if none exists and object is not being deleted.
	if bucket.DeletionTimestamp == nil && !containFinalizer(bucket.Finalizers, oadpFinalizerBucket) {
		bucket.Finalizers = append(bucket.Finalizers, oadpFinalizerBucket)
		err := b.Client.Update(ctx, &bucket, &client.UpdateOptions{})
		if err != nil {
			b.EventRecorder.Event(&bucket, corev1.EventTypeWarning, "UnableToAddFinalizer", fmt.Sprintf("unable to add finalizer: %v", err))
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{Requeue: true}, nil
	}

	clnt, err := bucketpkg.NewClient(bucket, b.Client)
	if err != nil {
		return result, err
	}
	annotation, annotationExists := bucket.Annotations[oadpCloudStorageDeleteAnnotation]
	shouldDelete := false
	if annotationExists {
		shouldDelete, err = strconv.ParseBool(annotation)
		if err != nil {
			// delete annotation should have values of "1", "t", "T", "true", "TRUE", "True" or "0", "f", "F", "false", "FALSE", "False"
			b.EventRecorder.Event(&bucket, corev1.EventTypeWarning, "UnableToParseAnnotation", fmt.Sprintf("unable to parse annotation: %v, use \"1\", \"t\", \"T\", \"true\", \"TRUE\", \"True\" or \"0\", \"f\", \"F\", \"false\", \"FALSE\", \"False\"", err))
			return ctrl.Result{Requeue: true}, nil
		}
		if shouldDelete && bucket.DeletionTimestamp != nil {
			deleted, err := clnt.Delete()
			if err != nil {
				log.Error(err, "unable to delete bucket")
				b.EventRecorder.Event(&bucket, corev1.EventTypeWarning, "UnableToDeleteBucket", fmt.Sprintf("unable to delete bucket: %v", bucket.Spec.Name))
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			if !deleted {
				log.Info("unable to delete bucket for unknown reason")
				b.EventRecorder.Event(&bucket, corev1.EventTypeWarning, "UnableToDeleteBucketUnknown", fmt.Sprintf("unable to delete bucket: %v", bucket.Spec.Name))
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			log.Info("bucket deleted")
			b.EventRecorder.Event(&bucket, corev1.EventTypeNormal, "BucketDeleted", fmt.Sprintf("bucket %v deleted", bucket.Spec.Name))

			//Removing oadpFinalizerBucket from bucket.Finalizers
			bucket.Finalizers = removeKey(bucket.Finalizers, oadpFinalizerBucket)
			err = b.Client.Update(ctx, &bucket, &client.UpdateOptions{})
			if err != nil {
				b.EventRecorder.Event(&bucket, corev1.EventTypeWarning, "UnableToRemoveFinalizer", fmt.Sprintf("unable to remove finalizer: %v", err))
			}
			return ctrl.Result{Requeue: true}, nil
		}
	}
	var ok bool
	if ok, err = clnt.Exists(); !ok && err == nil {
		// Handle Creation if not exist.
		created, err := clnt.Create()
		if !created {
			log.Info("unable to create object bucket")
			b.EventRecorder.Event(&bucket, corev1.EventTypeWarning, "BucketNotCreated", fmt.Sprintf("unable to create bucket: %v", err))
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		if err != nil {
			//TODO: LOG/EVENT THE MESSAGE
			log.Error(err, "Error while creating event")
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}
		b.EventRecorder.Event(&bucket, corev1.EventTypeNormal, "BucketCreated", fmt.Sprintf("bucket %v has been created", bucket.Spec.Name))
	}
	if err != nil {
		// Bucket may be created but something else went wrong.
		log.Error(err, "unable to determine if bucket exists.")
		b.EventRecorder.Event(&bucket, corev1.EventTypeWarning, "BucketNotFound", fmt.Sprintf("unable to find bucket: %v", err))
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// Update status with updated value
	bucket.Status.LastSynced = &v1.Time{Time: time.Now()}
	bucket.Status.Name = bucket.Spec.Name

	b.Client.Status().Update(ctx, &bucket, &client.SubResourceUpdateOptions{})
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (b *BucketReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&oadpv1alpha1.CloudStorage{}).
		WithEventFilter(bucketPredicate()).
		Complete(b)

}

func bucketPredicate() predicate.Predicate {
	return predicate.Funcs{
		// Update returns true if the Update event should be processed
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetDeletionTimestamp() != nil {
				return true
			}
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
		// Create returns true if the Create event should be processed
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		// Delete returns true if the Delete event should be processed
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}
}

func containFinalizer(finalizers []string, f string) bool {
	for _, finalizer := range finalizers {
		if finalizer == f {
			return true
		}
	}
	return false
}

func removeKey(slice []string, s string) []string {
	for i, v := range slice {
		if v == s {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

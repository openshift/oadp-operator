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

package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	bucketpkg "github.com/openshift/oadp-operator/pkg/bucket"
	"github.com/openshift/oadp-operator/pkg/credentials/stsflow"
)

const (
	oadpFinalizerBucket              = "oadp.openshift.io/bucket-protection"
	oadpCloudStorageDeleteAnnotation = "oadp.openshift.io/cloudstorage-delete"
)

// CloudStorageReconciler reconciles a CloudStorage object
type CloudStorageReconciler struct {
	Client        client.Client
	Scheme        *runtime.Scheme
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

//+kubebuilder:rbac:groups=oadp.openshift.io,resources=cloudstorages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=cloudstorages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=cloudstorages/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (b CloudStorageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	b.Log = log.FromContext(ctx)
	logger := b.Log.WithValues("bucket", req.NamespacedName)
	result := ctrl.Result{}
	// Set reconciler context + name

	bucket := oadpv1alpha1.CloudStorage{}

	if err := b.Client.Get(ctx, req.NamespacedName, &bucket); err != nil {
		logger.Error(err, "unable to fetch bucket CR")
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
				logger.Error(err, "unable to delete bucket")
				b.EventRecorder.Event(&bucket, corev1.EventTypeWarning, "UnableToDeleteBucket", fmt.Sprintf("unable to delete bucket: %v", bucket.Spec.Name))
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			if !deleted {
				logger.Info("unable to delete bucket for unknown reason")
				b.EventRecorder.Event(&bucket, corev1.EventTypeWarning, "UnableToDeleteBucketUnknown", fmt.Sprintf("unable to delete bucket: %v", bucket.Spec.Name))
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			logger.Info("bucket deleted")
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
	var (
		ok         bool
		secretName string
	)
	// check if STSStandardizedFlow was successful
	if secretName, err = stsflow.STSStandardizedFlow(); err != nil {
		logger.Error(err, "unable to get STS Secret")
		b.EventRecorder.Event(&bucket, corev1.EventTypeWarning, "UnableToSTSSecret", fmt.Sprintf("unable to delete bucket: %v", bucket.Spec.Name))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	if secretName != "" {
		// Secret was created successfully by STSStandardizedFlow
		logger.Info(fmt.Sprintf("Following standardized STS workflow, secret %s created successfully", secretName))
	}
	// Now continue with bucket creation as secret exists and we are good to go !!!
	if ok, err = clnt.Exists(); !ok && err == nil {
		// Handle Creation if not exist.
		created, err := clnt.Create()
		if !created {
			logger.Info("unable to create object bucket")
			b.EventRecorder.Event(&bucket, corev1.EventTypeWarning, "BucketNotCreated", fmt.Sprintf("unable to create bucket: %v", err))
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		if err != nil {
			//TODO: LOG/EVENT THE MESSAGE
			logger.Error(err, "Error while creating event")
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}
		b.EventRecorder.Event(&bucket, corev1.EventTypeNormal, "BucketCreated", fmt.Sprintf("bucket %v has been created", bucket.Spec.Name))
	}
	if err != nil {
		// Bucket may be created but something else went wrong.
		logger.Error(err, "unable to determine if bucket exists.")
		b.EventRecorder.Event(&bucket, corev1.EventTypeWarning, "BucketNotFound", fmt.Sprintf("unable to find bucket: %v", err))
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// Update status with updated value
	bucket.Status.LastSynced = &metav1.Time{Time: time.Now()}
	bucket.Status.Name = bucket.Spec.Name

	b.Client.Status().Update(ctx, &bucket)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (b *CloudStorageReconciler) SetupWithManager(mgr ctrl.Manager) error {
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

func (b *CloudStorageReconciler) WaitForSecret(namespace, name string) (*corev1.Secret, error) {
	// set a timeout of 10 minutes
	timeout := 10 * time.Minute

	secret := corev1.Secret{}
	key := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}

	err := wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {

		err := b.Client.Get(context.Background(), key, &secret)
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
		}
		return true, nil
	})

	if err != nil {
		return nil, err
	}

	return &secret, nil
}

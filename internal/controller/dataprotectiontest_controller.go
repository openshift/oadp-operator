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
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"net/http"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

// DataProtectionTestReconciler reconciles a DataProtectionTest object
type DataProtectionTestReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Log           logr.Logger
	Context       context.Context
	EventRecorder record.EventRecorder
}

//+kubebuilder:rbac:groups=oadp.openshift.io,resources=dataprotectiontests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=dataprotectiontests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=dataprotectiontests/finalizers,verbs=update

func (r *DataProtectionTestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)
	logger := r.Log.WithValues("dpt", req.NamespacedName)

	var dpt oadpv1alpha1.DataProtectionTest
	if err := r.Get(ctx, req.NamespacedName, &dpt); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("DPT not found; skipping")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info(fmt.Sprintf("DPT found, DPT name is: %v", dpt.Name))

	// Determine S3-compatible vendor if applicable
	if dpt.Spec.BackupLocationSpec != nil && dpt.Spec.BackupLocationSpec.Provider == AWSProvider {
		if err := r.determineVendor(ctx, &dpt); err != nil {
			logger.Error(err, "failed to determine S3 vendor")
		}
	}
	//
	//// 2. Initialize provider (e.g., S3)
	//var cp cloudprovider.CloudProvider
	//var err error
	//if dpt.Spec.UploadSpeedTestConfig != nil {
	//	cp, err = r.initializeProvider(&dpt)
	//	if err != nil {
	//		log.Error(err, "failed to initialize provider")
	//	}
	//}
	//
	//// 3. Run Upload Test
	//if cp != nil && dpt.Spec.UploadSpeedTestConfig != nil {
	//	if err := r.runUploadTest(ctx, &dpt, cp); err != nil {
	//		log.Error(err, "upload test failed")
	//	}
	//}
	//
	//// 4. Run Snapshot Test(s)
	//if len(dpt.Spec.CSIVolumeSnapshotTestConfigs) > 0 {
	//	if err := r.runSnapshotTests(ctx, &dpt); err != nil {
	//		log.Error(err, "snapshot tests failed")
	//	}
	//}
	//
	// Update status
	dpt.Status.LastTested = metav1.NewTime(time.Now())
	if err := r.Status().Update(ctx, &dpt); err != nil {
		logger.Error(err, "failed to update DPT status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DataProtectionTestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&oadpv1alpha1.DataProtectionTest{}).
		Complete(r)
}

// determineVendor sends a HEAD request to the provided s3Url in the BackupLocationSpec config,
// extracts the Server header, and sets the detected vendor (e.g., AWS, MinIO, Ceph) in the DPT status.
// only applicable for aws provider BSL objects
func (r *DataProtectionTestReconciler) determineVendor(ctx context.Context, dpt *oadpv1alpha1.DataProtectionTest) error {
	s3Url := dpt.Spec.BackupLocationSpec.Config["s3Url"]
	if s3Url == "" {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, s3Url, nil)
	if err != nil {
		return fmt.Errorf("failed to create HEAD request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("HEAD request to %s failed: %w", s3Url, err)
	}
	defer resp.Body.Close()

	server := strings.ToLower(resp.Header.Get("Server"))
	switch {
	case strings.Contains(server, "amazon"):
		dpt.Status.S3Vendor = "AWS"
	case strings.Contains(server, "minio"):
		dpt.Status.S3Vendor = "MinIO"
	case strings.Contains(server, "ceph"):
		dpt.Status.S3Vendor = "Ceph"
	default:
		dpt.Status.S3Vendor = server
	}

	r.Log.Info("Detected S3 vendor", "vendor", dpt.Status.S3Vendor)
	return nil
}

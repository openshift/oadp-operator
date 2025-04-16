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
	"github.com/openshift/oadp-operator/pkg/cloudprovider"
	"github.com/openshift/oadp-operator/pkg/utils"

	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
)

// DataProtectionTestReconciler reconciles a DataProtectionTest object
type DataProtectionTestReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Log            logr.Logger
	Context        context.Context
	EventRecorder  record.EventRecorder
	NamespacedName types.NamespacedName
	dpt            *oadpv1alpha1.DataProtectionTest
}

//+kubebuilder:rbac:groups=oadp.openshift.io,resources=dataprotectiontests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=dataprotectiontests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=dataprotectiontests/finalizers,verbs=update

func (r *DataProtectionTestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)
	logger := r.Log.WithValues("dpt", req.NamespacedName)
	r.NamespacedName = req.NamespacedName
	r.Context = ctx
	
	r.dpt = &oadpv1alpha1.DataProtectionTest{}

	if err := r.Get(ctx, req.NamespacedName, r.dpt); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("DPT not found; skipping")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info(fmt.Sprintf("DPT found, DPT name is: %v", r.dpt.Name))

	// Short-circuit if already completed
	if r.dpt.Status.Phase == "Complete" {
		logger.Info("DPT already completed; skipping reprocessing")
		return ctrl.Result{}, nil
	}	

	// Resolve the backup location from spec or by fetching BSL
	resolvedBackupLocationSpec, err := r.resolveBackupLocation(r.Context, r.dpt)
	if err != nil {
		logger.Error(err, "failed to resolve backup location spec")
		return ctrl.Result{}, err
	}

	// Determine S3-compatible vendor (if applicable)
	if r.dpt.Spec.BackupLocationSpec != nil && r.dpt.Spec.BackupLocationSpec.Provider == AWSProvider {
		if err := r.determineVendor(ctx, r.dpt, resolvedBackupLocationSpec); err != nil {
			logger.Error(err, "failed to determine S3 vendor")
		}
	}

	// Initialize cloud provider
	var cp cloudprovider.CloudProvider
	if r.dpt.Spec.UploadSpeedTestConfig != nil {
		var err error
		cp, err = r.initializeProvider(r.dpt, resolvedBackupLocationSpec)
		if err != nil {
			logger.Error(err, "failed to initialize provider")
			return ctrl.Result{}, err
		}
	}

	// Run Upload Test
	if cp != nil && r.dpt.Spec.UploadSpeedTestConfig != nil {
		if err := r.runUploadTest(r.Context, r.dpt, resolvedBackupLocationSpec,cp); err != nil {
			logger.Error(err, "upload test failed")
			return ctrl.Result{}, err
		}
	}

	// Fetch Bucket Metadata
	if cp != nil {
		meta, err := cp.GetBucketMetadata(ctx, resolvedBackupLocationSpec.ObjectStorage.Bucket)
		if err != nil {
			logger.Error(err, "bucket metadata collection failed")
		}
		r.dpt.Status.BucketMetadata = meta
	}
	
	//
	//// 4. Run Snapshot Test(s)
	//if len(dpt.Spec.CSIVolumeSnapshotTestConfigs) > 0 {
	//	if err := r.runSnapshotTests(ctx, &dpt); err != nil {
	//		log.Error(err, "snapshot tests failed")
	//	}
	//}
	//
	// Update status
	r.dpt.Status.LastTested = metav1.NewTime(time.Now())
	r.dpt.Status.Phase = "Complete"
	if err := r.Status().Update(ctx, r.dpt); err != nil {
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
func (r *DataProtectionTestReconciler) determineVendor(ctx context.Context, dpt *oadpv1alpha1.DataProtectionTest, backupLocationSpec *velerov1.BackupStorageLocationSpec) error {
	s3Url := backupLocationSpec.Config["s3Url"]
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

// initializeProvider reads the BackupLocationSpec from the DPT CR,
// retrieves the associated credentials from a Secret, and returns an initialized
// CloudProvider
func (r *DataProtectionTestReconciler) initializeProvider(dpt *oadpv1alpha1.DataProtectionTest, backupLocationSpec *velerov1.BackupStorageLocationSpec) (cloudprovider.CloudProvider, error) {

	providerName := strings.ToLower(backupLocationSpec.Provider)
	cfg := backupLocationSpec.Config
	cred := backupLocationSpec.Credential
	s3Url := cfg["s3Url"]
	region := cfg["region"]

	// Ignore s3Url if it's aws-native
	if strings.Contains(s3Url, "amazonaws.com") {
		s3Url = ""
	}
	
	switch providerName {
	case AWSProvider:
		secret, err := utils.GetProviderSecret(cred.Name, r.NamespacedName.Namespace, r.Client, r.Context, r.Log)
		if err != nil {
			return nil, fmt.Errorf("failed to get AWS secret: %w", err)
		}

		AWSProfile := "default"
		if backupLocationSpec.Config != nil {
			if value, exists := backupLocationSpec.Config[Profile]; exists {
				AWSProfile = value
			}
		}

		accessKey, secretKey, err := utils.ParseAWSSecret(secret, cred.Key, AWSProfile, r.Log)
		if err != nil {
			return nil, fmt.Errorf("failed to parse AWS secret: %w", err)
		}

		return cloudprovider.NewAWSProvider(region, s3Url, accessKey, secretKey), nil
	case GCPProvider:
		return nil, fmt.Errorf("GCP provider support not implemented yet")
	case AzureProvider:
		return nil, fmt.Errorf("azure provider support not implemented yet")

	default:
		return nil, fmt.Errorf("unsupported cloud provider: %s", providerName)
	}
}

// runUploadTest performs an upload speed test using the provided CloudProvider implementation.
// It uploads test data of the specified size to the configured bucket and measures speed and duration.
// The results are written into the DataProtectionTest's UploadTestStatus field.
func (r *DataProtectionTestReconciler) runUploadTest(ctx context.Context, dpt *oadpv1alpha1.DataProtectionTest, backupLocationSpec *velerov1.BackupStorageLocationSpec,cp cloudprovider.CloudProvider) error {
	cfg := dpt.Spec.UploadSpeedTestConfig
	bucket := backupLocationSpec.ObjectStorage.Bucket

	if cfg == nil {
		return fmt.Errorf("uploadSpeedTestConfig is nil")
	}
	if bucket == "" {
		return fmt.Errorf("bucket name is empty")
	}
	
	speed, duration, err := cp.UploadTest(ctx, *cfg, bucket)

	dpt.Status.UploadTest = oadpv1alpha1.UploadTestStatus{
		Duration: duration.Truncate(time.Millisecond).String(),
		Success:  err == nil,
	}

	if err != nil {
		dpt.Status.UploadTest.ErrorMessage = err.Error()
		dpt.Status.UploadTest.SpeedMbps = 0
		return err
	}

	dpt.Status.UploadTest.SpeedMbps = speed

	return nil
}

// resolveBackupLocation resolves the effective BackupStorageLocationSpec to use,
// either inline from the DPT CR or by fetching a named BSL from the cluster.
func (r *DataProtectionTestReconciler) resolveBackupLocation(
	ctx context.Context,
	dpt *oadpv1alpha1.DataProtectionTest,
) (*velerov1.BackupStorageLocationSpec, error) {

	if dpt.Spec.BackupLocationSpec != nil && dpt.Spec.BackupLocationName != "" {
		return nil, fmt.Errorf("both backupLocationSpec and backupLocationName cannot be set")
	}

	if dpt.Spec.BackupLocationSpec == nil && dpt.Spec.BackupLocationName == "" {
		return nil, fmt.Errorf("one of backupLocationSpec or backupLocationName must be set")
	}

	// Return user-specified inline config
	if dpt.Spec.BackupLocationSpec != nil {
		return dpt.Spec.BackupLocationSpec, nil
	}

	// Otherwise fetch BSL and return its spec
	bsl := &velerov1.BackupStorageLocation{}
	key := types.NamespacedName{
		Name:      dpt.Spec.BackupLocationName,
		Namespace: dpt.Namespace,
	}

	if err := r.Get(ctx, key, bsl); err != nil {
		return nil, fmt.Errorf("failed to get BackupStorageLocation %q: %w", key.Name, err)
	}

	return &bsl.Spec, nil
}


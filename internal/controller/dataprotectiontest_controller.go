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
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/cloudprovider"
	"github.com/openshift/oadp-operator/pkg/utils"
)

// DataProtectionTestReconciler reconciles a DataProtectionTest object
type DataProtectionTestReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	Log               logr.Logger
	Context           context.Context
	EventRecorder     record.EventRecorder
	NamespacedName    types.NamespacedName
	dpt               *oadpv1alpha1.DataProtectionTest
	ClusterWideClient client.Client
}

// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots,verbs=get;list;create;watch;delete;update
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshotcontents,verbs=get;list;watch;delete;update
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshotclasses,verbs=get;list;watch;delete;update
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
			logger.Info("DPT not found; skipping reconciliation")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get DPT")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling DataProtectionTest", "name", r.dpt.Name)

	// Short-circuit if already completed
	if (r.dpt.Status.Phase == "Complete" || r.dpt.Status.Phase == "Failed") && !r.dpt.Spec.ForceRun {
		logger.Info("DPT already completed or failed and forceRun not set; skipping")
		return ctrl.Result{}, nil
	}

	// Always reset forceRun after reconciliation attempt (whether successful or not)
	if r.dpt.Spec.ForceRun {
		defer func() {
			original := r.dpt.DeepCopy()
			r.dpt.Spec.ForceRun = false
			if !original.Spec.ForceRun {
				return
			}
			logger.Info("Resetting forceRun flag")
			_ = r.Patch(ctx, r.dpt, client.MergeFrom(original))
		}()
	}

	// Mark as InProgress
	if r.dpt.Status.Phase != "InProgress" {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			latest := &oadpv1alpha1.DataProtectionTest{}
			if err := r.Get(ctx, r.NamespacedName, latest); err != nil {
				return err
			}
			// Skip if it’s already done and forceRun is not set
			if (latest.Status.Phase == "Complete" || latest.Status.Phase == "Failed") && !latest.Spec.ForceRun {
				logger.Info("Skipping setting InProgress, current phase:", "phase", latest.Status.Phase)
				return nil
			}
			latest.Status.Phase = "InProgress"
			latest.Status.LastTested = metav1.Now()
			return r.Status().Update(ctx, latest)
		})
		if err != nil {
			logger.Error(err, "failed to set DPT phase to InProgress")
			return ctrl.Result{}, err
		}

		// Let the status update trigger the next reconcile
		logger.Info("Successfully set phase to InProgress; waiting for next reconcile")
		return ctrl.Result{}, nil
	}

	// Resolve the backup location from spec or by fetching BSL
	resolvedBackupLocationSpec, err := r.resolveBackupLocation(r.Context, r.dpt)
	if err != nil {
		logger.Error(err, "failed to resolve BackupLocation")
		r.updateDPTErrorStatus(ctx, fmt.Sprintf("failed to resolve BackupLocation: %v", err))
		return ctrl.Result{}, err
	}

	if resolvedBackupLocationSpec == nil {
		msg := "BackupLocation is nil after resolution"
		logger.Info(msg)
		r.updateDPTErrorStatus(ctx, msg)
		return ctrl.Result{}, fmt.Errorf("resolved BackupLocationSpec is nil")
	}

	// Determine S3-compatible vendor (if applicable)
	if strings.EqualFold(resolvedBackupLocationSpec.Provider, AWSProvider) {
		if err := r.determineVendor(ctx, r.dpt, resolvedBackupLocationSpec); err != nil {
			logger.Error(err, "S3 vendor detection failed")
		}
	}

	// Handle Upload Speed Test + Bucket Metadata (if UploadSpeedTestConfig is provided)
	if cfg := r.dpt.Spec.UploadSpeedTestConfig; cfg != nil {
		logger.Info("Initializing cloud provider for upload test...")

		cp, err := r.initializeProvider(resolvedBackupLocationSpec)
		if err != nil {
			logger.Error(err, "failed to initialize cloud provider")
			r.updateDPTErrorStatus(ctx, fmt.Sprintf("cloud provider init failed: %v", err))
			return ctrl.Result{}, err
		}

		// Upload speed test
		logger.Info("Executing upload test...")
		if err := r.runUploadTest(ctx, r.dpt, resolvedBackupLocationSpec, cp); err != nil {
			logger.Error(err, "upload test failed")
			// handled in UploadTestStatus.ErrorMessage
		}

		// Bucket metadata
		logger.Info("Fetching Bucket metadata...")
		meta, err := cp.GetBucketMetadata(ctx, resolvedBackupLocationSpec.ObjectStorage.Bucket, r.Log)
		if err != nil {
			logger.Error(err, "bucket metadata collection failed")
			r.dpt.Status.BucketMetadata = &oadpv1alpha1.BucketMetadata{
				ErrorMessage: err.Error(),
			}
		} else {
			r.dpt.Status.BucketMetadata = meta
		}
	} else {
		logger.Info("Skipping upload test because no spec.uploadSpeed config found")
	}

	//Run Snapshot Test(s)
	if len(r.dpt.Spec.CSIVolumeSnapshotTestConfigs) > 0 {
		logger.Info("Running snapshot tests", "count", len(r.dpt.Spec.CSIVolumeSnapshotTestConfigs))
		if err := r.runSnapshotTests(ctx, r.dpt); err != nil {
			logger.Error(err, "snapshot test execution failed")
			// handled in SnapshotTestStatus.ErrorMessage
		}
	} else {
		logger.Info("Skipping snapshot test because no spec.csiVolumeSnapshotTestConfigs found")
	}

	// Final status update: mark as Complete
	if err := r.updateDPTStatusToComplete(ctx); err != nil {
		logger.Error(err, "failed to update DPT status to Complete")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciliation completed successfully", "finalPhase", "Complete")
	return ctrl.Result{}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *DataProtectionTestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&oadpv1alpha1.DataProtectionTest{}).
		Complete(r)
}

// determineVendor sends a HEAD request to the provided s3Url in the BackupLocationSpec config,
// extracts the Server header and known fallback headers to set the detected vendor (e.g., AWS, MinIO, Ceph) in the DPT status.
// Only applicable for aws-compatible BSLs.
func (r *DataProtectionTestReconciler) determineVendor(ctx context.Context, dpt *oadpv1alpha1.DataProtectionTest, backupLocationSpec *velerov1.BackupStorageLocationSpec) error {
	s3Url := backupLocationSpec.Config["s3Url"]

	// Fallback to AWS default endpoint if missing
	if s3Url == "" && strings.EqualFold(backupLocationSpec.Provider, "aws") {
		region := backupLocationSpec.Config["region"]
		if region == "" {
			region = "us-east-1"
		}
		s3Url = fmt.Sprintf("https://s3.%s.amazonaws.com", region)
	}

	if s3Url == "" {
		r.Log.Info("No s3Url available; skipping vendor detection")
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
	xAmzReqID := resp.Header.Get("x-amz-request-id")
	minioRegion := resp.Header.Get("x-minio-region")
	rgwReqID := resp.Header.Get("x-rgw-request-id")

	switch {
	case strings.Contains(server, "amazon") || (xAmzReqID != "" && minioRegion == ""):
		dpt.Status.S3Vendor = "AWS"
	case strings.Contains(server, "minio") || minioRegion != "":
		dpt.Status.S3Vendor = "MinIO"
	case strings.Contains(server, "ceph") || rgwReqID != "":
		dpt.Status.S3Vendor = "Ceph"
	default:
		if server != "" {
			dpt.Status.S3Vendor = server
		} else {
			dpt.Status.S3Vendor = "Unknown"
		}
	}

	r.Log.Info("Detected S3 vendor", "vendor", dpt.Status.S3Vendor)
	return nil
}

// initializeProvider reads the BackupLocationSpec from the DPT CR,
// retrieves the associated credentials from a Secret, and returns an initialized
// CloudProvider
func (r *DataProtectionTestReconciler) initializeProvider(backupLocationSpec *velerov1.BackupStorageLocationSpec) (cloudprovider.CloudProvider, error) {

	if backupLocationSpec == nil {
		return nil, fmt.Errorf("backupLocationSpec is nil")
	}

	providerName := strings.ToLower(backupLocationSpec.Provider)
	cfg := backupLocationSpec.Config

	if cfg == nil {
		return nil, fmt.Errorf("backupLocationSpec.Config is nil")
	}

	//TODO handle credential when not specified
	cred := backupLocationSpec.Credential

	s3Url := cfg[S3URL]
	region := cfg[Region]

	if region == "" {
		r.Log.Info("Region not specified in backupLocationSpec.Config; using default 'us-east-1'")
		region = "us-east-1"
	}

	// Ignore s3Url if it's aws-native
	if strings.Contains(s3Url, "amazonaws.com") {
		r.Log.Info("Detected AWS-native endpoint; ignoring s3Url")
		s3Url = ""
	}

	switch providerName {
	case AWSProvider:
		r.Log.Info("Fetching AWS provider secret", "secretName", cred.Name, "namespace", r.NamespacedName.Namespace)
		secret, err := utils.GetProviderSecret(cred.Name, r.NamespacedName.Namespace, r.Client, r.Context)
		if err != nil {
			return nil, fmt.Errorf("failed to get AWS secret: %w", err)
		}

		AWSProfile := "default"
		if backupLocationSpec.Config != nil {
			if value, exists := backupLocationSpec.Config[Profile]; exists {
				AWSProfile = value
			}
		}

		r.Log.Info("Parsing AWS credentials", "profile", AWSProfile)
		accessKey, secretKey, err := utils.ParseAWSSecret(secret, cred.Key, AWSProfile)
		if err != nil {
			return nil, fmt.Errorf("failed to parse AWS secret: %w", err)
		}

		r.Log.Info("Successfully initialized AWS provider")
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
func (r *DataProtectionTestReconciler) runUploadTest(ctx context.Context, dpt *oadpv1alpha1.DataProtectionTest, backupLocationSpec *velerov1.BackupStorageLocationSpec, cp cloudprovider.CloudProvider) error {
	if dpt.Spec.UploadSpeedTestConfig == nil {
		return fmt.Errorf("uploadSpeedTestConfig is is nil")
	}

	if backupLocationSpec == nil || backupLocationSpec.ObjectStorage == nil {
		return fmt.Errorf("objectStorage config is missing in backupLocationSpec")
	}

	bucket := backupLocationSpec.ObjectStorage.Bucket
	if bucket == "" {
		return fmt.Errorf("bucket name is empty")
	}

	cfg := dpt.Spec.UploadSpeedTestConfig
	r.Log.Info("Starting upload test", "bucket", bucket, "fileSize", cfg.FileSize, "timeout", cfg.Timeout)
	speed, duration, err := cp.UploadTest(ctx, *cfg, bucket, r.Log)

	dpt.Status.UploadTest = oadpv1alpha1.UploadTestStatus{
		Duration: duration.Truncate(time.Millisecond).String(),
		Success:  err == nil,
	}

	if err != nil {
		r.Log.Error(err, "Upload test failed")
		dpt.Status.UploadTest.ErrorMessage = err.Error()
		dpt.Status.UploadTest.SpeedMbps = 0
		return fmt.Errorf("upload test failed: %w", err)
	}

	dpt.Status.UploadTest.SpeedMbps = speed
	r.Log.Info("Upload test succeeded", "speedMbps", speed, "duration", duration.Truncate(time.Millisecond).String())

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

// runSnapshotTests creates CSI VolumeSnapshots for each provided test configuration in parallel,
// and measures how long each snapshot takes to become ReadyToUse. The results are added to the DPT status.
func (r *DataProtectionTestReconciler) runSnapshotTests(ctx context.Context, dpt *oadpv1alpha1.DataProtectionTest) error {
	r.Log.Info("Starting CSI VolumeSnapshot tests")

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errMu sync.Mutex
	var results []oadpv1alpha1.SnapshotTestStatus
	var combinedErr error

	for _, cfg := range dpt.Spec.CSIVolumeSnapshotTestConfigs {

		if cfg.VolumeSnapshotSource.PersistentVolumeClaimName == "" ||
			cfg.VolumeSnapshotSource.PersistentVolumeClaimNamespace == "" ||
			cfg.SnapshotClassName == "" {
			r.Log.Info("Skipping snapshot test due to missing required fields", "config", cfg)
			continue
		}

		wg.Add(1)

		// launch a goroutine for each snapshot test config
		go func(cfg oadpv1alpha1.CSIVolumeSnapshotTestConfig) {
			defer wg.Done()

			start := time.Now()
			logger := r.Log.WithValues(
				"PVC", cfg.VolumeSnapshotSource.PersistentVolumeClaimName,
				"Namespace", cfg.VolumeSnapshotSource.PersistentVolumeClaimNamespace,
			)

			status := oadpv1alpha1.SnapshotTestStatus{
				PersistentVolumeClaimName:      cfg.VolumeSnapshotSource.PersistentVolumeClaimName,
				PersistentVolumeClaimNamespace: cfg.VolumeSnapshotSource.PersistentVolumeClaimNamespace,
			}

			// Create VS
			logger.Info("Creating VolumeSnapshot")
			vs, err := r.createVolumeSnapshot(ctx, dpt, cfg)
			if err != nil {
				logger.Error(err, "Failed to create VolumeSnapshot")
				status.Status = "Failed"
				status.ErrorMessage = err.Error()

				errMu.Lock()
				combinedErr = multierror.Append(combinedErr, err)
				errMu.Unlock()

			} else {
				// Wait for VS to be ready
				logger.Info("Waiting for VolumeSnapshot to become ReadyToUse")
				err := r.waitForSnapshotReady(ctx, vs, cfg.Timeout.Duration)
				if err != nil {
					logger.Error(err, "Snapshot did not become ready in time")
					status.Status = "Failed"
					status.ErrorMessage = err.Error()

					errMu.Lock()
					combinedErr = multierror.Append(combinedErr, err)
					errMu.Unlock()

				} else {
					duration := time.Since(start).Truncate(time.Second)
					logger.Info("Snapshot is ReadyToUse", "duration", duration)
					status.Status = "Ready"
					status.ReadyDuration = duration.String()
				}
			}

			// append the results
			mu.Lock()
			results = append(results, status)
			mu.Unlock()

		}(cfg)
	}

	wg.Wait()

	r.Log.Info("All snapshot tests completed", "count", len(results))

	dpt.Status.SnapshotTests = results

	// Summarize results
	passed := 0
	total := len(dpt.Status.SnapshotTests)
	for _, s := range dpt.Status.SnapshotTests {
		if s.Status == "Ready" {
			passed++
		}
	}
	dpt.Status.SnapshotSummary = fmt.Sprintf("%d/%d passed", passed, total)

	return combinedErr

}

// createVolumeSnapshot constructs and creates a CSI VolumeSnapshot for the specified PVC.
func (r *DataProtectionTestReconciler) createVolumeSnapshot(ctx context.Context, dpt *oadpv1alpha1.DataProtectionTest, cfg oadpv1alpha1.CSIVolumeSnapshotTestConfig) (*snapshotv1api.VolumeSnapshot, error) {

	vs := &snapshotv1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "dpt-snap-",
			Namespace:    cfg.VolumeSnapshotSource.PersistentVolumeClaimNamespace,
			Labels: map[string]string{
				"oadp.openshift.io/dpt": dpt.Name,
			},
		},
		Spec: snapshotv1api.VolumeSnapshotSpec{
			Source: snapshotv1api.VolumeSnapshotSource{
				PersistentVolumeClaimName: &cfg.VolumeSnapshotSource.PersistentVolumeClaimName,
			},
			VolumeSnapshotClassName: &cfg.SnapshotClassName,
		},
	}

	if err := r.Create(ctx, vs); err != nil {
		r.Log.Error(err, "Failed to create VolumeSnapshot object")
		return nil, fmt.Errorf("failed to create VolumeSnapshot: %w", err)
	}
	r.Log.Info("VolumeSnapshot created", "name", vs.Name, "namespace", vs.Namespace)
	return vs, nil
}

// waitForSnapshotReady polls the VolumeSnapshot resource until it's marked ReadyToUse or timeout expires.
func (r *DataProtectionTestReconciler) waitForSnapshotReady(ctx context.Context, vs *snapshotv1api.VolumeSnapshot, timeout time.Duration) error {

	timeoutDuration := timeout
	if timeoutDuration == 0 {
		timeoutDuration = 2 * time.Minute
	}

	key := types.NamespacedName{
		Name:      vs.Name,
		Namespace: vs.Namespace,
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeoutChan := time.After(timeoutDuration)

	for {
		select {
		case <-ticker.C:
			r.Log.Info("Waiting for VolumeSnapshot to be ready", "snapshot", vs.Name, "timeout", timeoutDuration)
			current := &snapshotv1api.VolumeSnapshot{}
			if err := r.ClusterWideClient.Get(ctx, key, current); err != nil {
				r.Log.Error(err, "Failed to get VolumeSnapshot during readiness check", "name", vs.Name)
				return fmt.Errorf("failed to get VolumeSnapshot %q: %w", vs.Name, err)
			}

			if current.Status != vs.Status && current.Status.ReadyToUse != nil && *current.Status.ReadyToUse {
				r.Log.Info("VolumeSnapshot is ready", "name", vs.Name)
				return nil
			}

		case <-timeoutChan:
			r.Log.Error(nil, "Timed out waiting for VolumeSnapshot to become ready", "name", vs.Name)
			return fmt.Errorf("timed out waiting for VolumeSnapshot %q to be ready", vs.Name)
		}
	}
}

// updateDPTErrorStatus sets the DPT status.phase to "Failed" and updates the error message.
// It handles conflict retries gracefully.
func (r *DataProtectionTestReconciler) updateDPTErrorStatus(ctx context.Context, msg string) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &oadpv1alpha1.DataProtectionTest{}
		if getErr := r.Get(ctx, r.NamespacedName, latest); getErr != nil {
			return getErr
		}
		latest.Status.Phase = "Failed"
		latest.Status.ErrorMessage = msg
		return r.Status().Update(ctx, latest)
	})

	if err != nil {
		r.Log.Error(err, "failed to update DPT error status", "message", msg)
	}
}

func (r *DataProtectionTestReconciler) updateDPTStatusToComplete(ctx context.Context) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &oadpv1alpha1.DataProtectionTest{}
		if err := r.Get(ctx, r.NamespacedName, latest); err != nil {
			return err
		}

		latest.Status.Phase = "Complete"
		latest.Status.ErrorMessage = ""
		latest.Status.UploadTest = r.dpt.Status.UploadTest
		latest.Status.SnapshotTests = r.dpt.Status.SnapshotTests
		latest.Status.SnapshotSummary = r.dpt.Status.SnapshotSummary
		latest.Status.BucketMetadata = r.dpt.Status.BucketMetadata
		latest.Status.S3Vendor = r.dpt.Status.S3Vendor

		return r.Status().Update(ctx, latest)
	})
}

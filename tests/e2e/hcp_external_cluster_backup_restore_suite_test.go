package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
	libhcp "github.com/openshift/oadp-operator/tests/e2e/lib/hcp"
)

const (
	testNamespace = "test"
	// testPodName is the name of the test pod created on the guest cluster to
	// exercise the velero-plugin GetRegistryInfo() code path (OADP-8924).
	testPodName = "test-pod-oadp-8924"
)

// External cluster backup and restore tests will skip creating HostedCluster resource. They expect the cluster
// to already have HostedCluster with a data plane.
// The tests are skipped unless hc_backup_restore_mode flag is properly configured.
var _ = ginkgo.Describe("HCP external cluster Backup and Restore tests", ginkgo.Ordered, func() {
	var (
		lastInstallTime time.Time
		lastBRCase      HCPBackupRestoreCase
		h               *libhcp.HCHandler
	)

	updateLastBRcase := func(brCase HCPBackupRestoreCase) {
		lastBRCase = brCase
	}

	updateLastInstallTime := func() {
		lastInstallTime = time.Now()
	}

	var _ = ginkgo.BeforeAll(func() {
		if HCBackupRestoreMode(hcBackupRestoreMode) != HCModeExternal &&
			HCBackupRestoreMode(hcBackupRestoreMode) != HCModeExternalROSA {
			ginkgo.Skip("Skipping HCP full backup and restore test for non-existent HCP")
		}

		h = &libhcp.HCHandler{
			Ctx:                  context.Background(),
			Client:               runTimeClientForSuiteRun,
			ClientServiceCluster: crClientForServiceCluster,
			HCOCPTestImage:       libhcp.HCOCPTestImage,
		}
	})

	// After Each
	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		gatherLogs(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
		oadpDeploymentOperation := NewOADPDeploymentOperationDefault()
		if HCBackupRestoreMode(hcBackupRestoreMode) == HCModeExternalROSA {
			oadpDeploymentOperation = NewOADPDeploymentOperationROSA()
		}
		oadpDeploymentOperation.Undeploy(lastBRCase.BackupRestoreCase.BackupRestoreType)
	})

	ginkgo.It("HCP external cluster backup and restore test", ginkgo.Label("hcp_external"), func() {
		if ginkgo.CurrentSpecReport().NumAttempts > 1 && !knownFlake {
			ginkgo.Fail("No known FLAKE found in a previous run, marking test as failed.")
		}

		runHCPBackupAndRestore(HCPBackupRestoreCase{
			Mode:                   HCBackupRestoreMode(hcBackupRestoreMode),
			PreBackupVerifyGuest:   preBackupVerifyGuest(),
			PostRestoreVerifyGuest: postBackupVerifyGuest(),
			BackupRestoreCase: BackupRestoreCase{
				Name:              hcName,
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(hcName, hcNamespace)),
				PostRestoreVerify: libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(hcName, hcNamespace)),
				BackupTimeout:     libhcp.HCPBackupTimeout,
			},
		}, updateLastBRcase, updateLastInstallTime, h)
	})

	// OADP-8924: Verify that backing up a guest cluster pod succeeds when the
	// openshift-apiserver/config ConfigMap is absent. On HCP guest clusters the
	// openshift-apiserver runs on the management cluster, so the ConfigMap
	// "config" does not exist in the guest cluster's openshift-apiserver
	// namespace. The openshift-velero-plugin's GetRegistryInfo() used to fail
	// with "configmaps \"config\" not found" for every pod backup. The fix
	// (openshift/openshift-velero-plugin#489) returns ("", nil) on 404.
	// This test deploys a workload pod on the guest cluster and includes the
	// guest namespace in the backup to exercise that code path.
	ginkgo.It("HCP external cluster guest pod backup test (OADP-8924)", ginkgo.Label("hcp_external"), func() {
		if ginkgo.CurrentSpecReport().NumAttempts > 1 && !knownFlake {
			ginkgo.Fail("No known FLAKE found in a previous run, marking test as failed.")
		}

		runHCPBackupAndRestore(HCPBackupRestoreCase{
			Mode:                       HCBackupRestoreMode(hcBackupRestoreMode),
			PreBackupVerifyGuest:       preBackupVerifyGuestPod(),
			PostRestoreVerifyGuest:     postRestoreVerifyGuestPod(),
			AdditionalBackupNamespaces: []string{testNamespace},
			BackupRestoreCase: BackupRestoreCase{
				Name:              hcName,
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(hcName, hcNamespace)),
				PostRestoreVerify: libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(hcName, hcNamespace)),
				BackupTimeout:     libhcp.HCPBackupTimeout,
			},
		}, updateLastBRcase, updateLastInstallTime, h)
	})
})

func preBackupVerifyGuest() VerificationFunctionGuest {
	return func(crClientGuest client.Client, _ string) error {
		var errs []error
		errs = append(errs, createTestNamespace(crClientGuest))
		// Add more verifications here if needed
		return errors.Join(errs...)
	}
}

func postBackupVerifyGuest() VerificationFunctionGuest {
	return func(crClientGuest client.Client, _ string) error {
		var errs []error
		errs = append(errs, validateTestNamespace(crClientGuest))
		// Add more verifications here if needed
		return errors.Join(errs...)
	}
}

func createTestNamespace(crClientGuest client.Client) error {
	ns := &corev1.Namespace{}
	ns.Name = testNamespace
	err := crClientGuest.Create(context.Background(), ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func validateTestNamespace(crClientGuest client.Client) error {
	ns := &corev1.Namespace{}
	err := crClientGuest.Get(context.Background(), client.ObjectKey{Name: testNamespace}, ns)
	if err != nil {
		return err
	}
	return nil
}

// preBackupVerifyGuestPod creates a test namespace and a workload pod on the
// guest cluster. The pod exercises the velero-plugin GetRegistryInfo() code
// path during backup (OADP-8924).
func preBackupVerifyGuestPod() VerificationFunctionGuest {
	return func(crClientGuest client.Client, _ string) error {
		var errs []error
		errs = append(errs, createTestNamespace(crClientGuest))
		errs = append(errs, createTestPod(crClientGuest))
		return errors.Join(errs...)
	}
}

// postRestoreVerifyGuestPod validates that the test namespace and workload pod
// exist on the guest cluster after the HCP backup/restore cycle.
func postRestoreVerifyGuestPod() VerificationFunctionGuest {
	return func(crClientGuest client.Client, _ string) error {
		var errs []error
		errs = append(errs, validateTestNamespace(crClientGuest))
		errs = append(errs, validateTestPod(crClientGuest))
		return errors.Join(errs...)
	}
}

// createTestPod creates a simple workload pod in the test namespace on the
// guest cluster. When this pod is included in a Velero backup, the
// openshift-velero-plugin backup action calls GetRegistryInfo(), which reads the
// ConfigMap "config" from the openshift-apiserver namespace. On HCP guest
// clusters that ConfigMap is absent, triggering the bug fixed in OADP-8924.
func createTestPod(crClientGuest client.Client) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNamespace,
			Labels: map[string]string{
				"app":       "oadp-e2e-test",
				"test-case": "oadp-8924",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "sleep",
					Image:   "registry.access.redhat.com/ubi9/ubi-minimal:latest",
					Command: []string{"/bin/sh", "-c", "sleep infinity"},
				},
			},
		},
	}
	err := crClientGuest.Create(context.Background(), pod)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create test pod %s/%s: %w", testNamespace, testPodName, err)
	}

	// Wait for the pod to be running so the backup encounters a live pod.
	gomega.Eventually(func() bool {
		p := &corev1.Pod{}
		if getErr := crClientGuest.Get(context.Background(), client.ObjectKey{
			Name:      testPodName,
			Namespace: testNamespace,
		}, p); getErr != nil {
			return false
		}
		return p.Status.Phase == corev1.PodRunning
	}, 5*time.Minute, 10*time.Second).Should(gomega.BeTrue(),
		"test pod %s/%s did not reach Running phase", testNamespace, testPodName)

	return nil
}

// validateTestPod verifies the test pod exists on the guest cluster after
// restore.
func validateTestPod(crClientGuest client.Client) error {
	pod := &corev1.Pod{}
	err := crClientGuest.Get(context.Background(), client.ObjectKey{
		Name:      testPodName,
		Namespace: testNamespace,
	}, pod)
	if err != nil {
		return fmt.Errorf("failed to validate test pod %s/%s after restore: %w", testNamespace, testPodName, err)
	}
	return nil
}

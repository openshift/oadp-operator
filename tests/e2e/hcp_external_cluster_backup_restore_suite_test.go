package e2e_test

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
	libhcp "github.com/openshift/oadp-operator/tests/e2e/lib/hcp"
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
		if hcBackupRestoreMode != string(HCModeExternal) {
			ginkgo.Skip("Skipping HCP full backup and restore test for non-existent HCP")
		}

		h = &libhcp.HCHandler{
			Ctx:            context.Background(),
			Client:         runTimeClientForSuiteRun,
			HCOCPTestImage: libhcp.HCOCPTestImage,
		}
	})

	// After Each
	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		gatherLogs(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
		tearDownDPAResources(lastBRCase.BackupRestoreCase)
	})

	ginkgo.It("HCP external cluster backup and restore test", ginkgo.Label("hcp_external"), func() {
		if ginkgo.CurrentSpecReport().NumAttempts > 1 && !knownFlake {
			ginkgo.Fail("No known FLAKE found in a previous run, marking test as failed.")
		}

		runHCPBackupAndRestore(HCPBackupRestoreCase{
			Mode:                   HCModeExternal,
			PreBackupVerifyGuest:   preBackupVerifyGuest(),
			PostRestoreVerifyGuest: postBackupVerifyGuest(),
			BackupRestoreCase: BackupRestoreCase{
				Name:              hcName,
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(hcName, libhcp.ClustersNamespace)),
				PostRestoreVerify: libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(hcName, libhcp.ClustersNamespace)),
				BackupTimeout:     libhcp.HCPBackupTimeout,
			},
		}, updateLastBRcase, updateLastInstallTime, h)
	})
})

func preBackupVerifyGuest() VerificationFunctionGuest {
	return func(crClientGuest client.Client, namespace string) error {
		ns := &corev1.Namespace{}
		ns.Name = "test"
		err := crClientGuest.Create(context.Background(), ns)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		return nil
	}
}

func postBackupVerifyGuest() VerificationFunctionGuest {
	return func(crClientGuest client.Client, namespace string) error {
		ns := &corev1.Namespace{}
		err := crClientGuest.Get(context.Background(), client.ObjectKey{Name: "test"}, ns)
		if err != nil {
			return err
		}
		return nil
	}
}

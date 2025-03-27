package e2e_test

import (
	"context"
	"log"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	hypershiftv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

type HCPBackupRestoreCase struct {
	BackupRestoreCase
	HostedCluster *hypershiftv1.HostedCluster
}

func runHCPBackupAndRestore(brCase HCPBackupRestoreCase, updateLastBRcase func(brCase HCPBackupRestoreCase), h *lib.HCHandler) {
	updateLastBRcase(brCase)

	log.Printf("Preparing backup and restore")
	backupName, restoreName := prepareBackupAndRestore(brCase.BackupRestoreCase, func() {})

	err := lib.AddHCPPluginToDPA(h, dpaCR.Namespace, dpaCR.Name, false)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to add HCP plugin to DPA: %v", err)
	// TODO: move the wait for HC just after the DPA modification to allow reconciliation to go ahead without waiting for the HC to be created

	//Wait for HCP plugin to be added
	gomega.Eventually(lib.IsHCPPluginAdded(h.Client, brCase.BackupRestoreCase.Namespace, brCase.BackupRestoreCase.Name), 3*time.Minute, 1*time.Second).Should(gomega.BeTrue())

	log.Printf("Running pre-backup verification")
	if brCase.PreBackupVerify != nil {
		err := brCase.PreBackupVerify(runTimeClientForSuiteRun, brCase.Namespace)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run HCP pre-backup verification: %v", err)
	}

	// Backup HCP & HC
	log.Printf("Backing up HC")
	includedResources := lib.HCPIncludedResources
	excludedResources := lib.HCPExcludedResources
	includedNamespaces := lib.HCPIncludedNamespaces

	nsRequiresResticDCWorkaround := runHCPBackup(brCase.BackupRestoreCase, backupName, h, includedNamespaces, includedResources, excludedResources)

	// Delete everything in HCP namespace
	log.Printf("Deleting HCP & HC")
	err = lib.RemoveHCP(h, brCase.HostedCluster)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to remove HCP: %v", err)

	// Wait for HC to be deleted
	log.Printf("Waiting for HC to be deleted")
	gomega.Eventually(lib.IsHCDeleted(h, brCase.HostedCluster), 10*time.Minute, 1*time.Second).Should(gomega.BeTrue())

	// Restore HC
	log.Printf("Restoring HC")
	runRestore(brCase.BackupRestoreCase, backupName, restoreName, nsRequiresResticDCWorkaround)

	// Wait for HCP to be restored
	log.Printf("Validating HC")
	err = lib.ValidateHCP()(h.Client, lib.ClustersNamespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run HCP post-restore verification: %v", err)
}

var _ = ginkgo.Describe("HCP Backup and Restore tests", ginkgo.Ordered, func() {
	var (
		lastInstallTime time.Time
		lastBRCase      HCPBackupRestoreCase
		h               *lib.HCHandler
		ctx             context.Context
		err             error
		hc              *hypershiftv1.HostedCluster
	)

	updateLastBRcase := func(brCase HCPBackupRestoreCase) {
		lastBRCase = brCase
	}

	// Before All
	var _ = ginkgo.BeforeAll(func() {
		reqOperators := []lib.RequiredOperator{
			{
				Name:          lib.MCEName,
				Namespace:     lib.MCENamespace,
				OperatorGroup: lib.MCEOperatorGroup,
			},
		}
		ctx = context.Background()

		// Install MCE and Hypershift operators
		h, err = lib.InstallRequiredOperators(ctx, runTimeClientForSuiteRun, reqOperators)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(h).ToNot(gomega.BeNil())

		// Deploy the MCE manifest
		err = h.DeployMCEManifest()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		// Deploy the MCE and wait for it to be ready
		err = h.ValidateMCE()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		// Deploy MCE manifest
		err = h.DeployMCEManifest()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		// Validate the Hypershift operator
		err = h.ValidateHO()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		// Create the HostedCluster for the test
		hc, err = h.DeployHCManifest()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		lastBRCase.HostedCluster = hc
	})

	// After All
	var _ = ginkgo.AfterAll(func() {
		err := lib.RemoveHCP(h, hc)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to remove HCP: %v", err)
		gomega.Eventually(lib.IsHCDeleted(h, hc), 10*time.Minute, 1*time.Second).Should(gomega.BeTrue(), "HCP was not deleted")
		lib.RemoveHCPPluginFromDPA(h, dpaCR.Namespace, dpaCR.Name)
	})

	// After Each
	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		tearDownBackupAndRestore(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	ginkgo.DescribeTable("Basic HCP backup and restore test",
		func(brCase BackupRestoreCase, expectedErr error) {
			if ginkgo.CurrentSpecReport().NumAttempts > 1 && !knownFlake {
				ginkgo.Fail("No known FLAKE found in a previous run, marking test as failed.")
			}
			runHCPBackupAndRestore(HCPBackupRestoreCase{
				BackupRestoreCase: brCase,
			}, updateLastBRcase, h)
		},

		// Test Cases
		ginkgo.Entry("None HostedCluster backup and restore", BackupRestoreCase{
			Namespace:         lib.HCPNamespace,
			Name:              lib.HostedClusterName,
			BackupRestoreType: lib.CSIDataMover,
			PreBackupVerify:   lib.ValidateHCP(),
			PostRestoreVerify: lib.ValidateHCP(),
			BackupTimeout:     10 * time.Minute,
		}, nil),
	)
})

func runHCPBackup(brCase BackupRestoreCase, backupName string, h *lib.HCHandler, namespaces []string, includedResources, excludedResources []string) bool {
	nsRequiresResticDCWorkaround, err := lib.NamespaceRequiresResticDCWorkaround(h.Client, brCase.Namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// create backup
	log.Printf("Creating backup %s for case %s", backupName, brCase.Name)
	err = lib.CreateCustomBackupForNamespaces(h.Client, namespace, backupName, namespaces, includedResources, excludedResources, brCase.BackupRestoreType == lib.RESTIC || brCase.BackupRestoreType == lib.KOPIA, brCase.BackupRestoreType == lib.CSIDataMover)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// wait for backup to not be running
	gomega.Eventually(lib.IsBackupDone(h.Client, namespace, backupName), brCase.BackupTimeout, time.Second*10).Should(gomega.BeTrue())
	// TODO only log on fail?
	describeBackup := lib.DescribeBackup(h.Client, namespace, backupName)
	ginkgo.GinkgoWriter.Println(describeBackup)

	backupLogs := lib.BackupLogs(kubernetesClientForSuiteRun, h.Client, namespace, backupName)
	backupErrorLogs := lib.BackupErrorLogs(kubernetesClientForSuiteRun, h.Client, namespace, backupName)
	accumulatedTestLogs = append(accumulatedTestLogs, describeBackup, backupLogs)

	if !brCase.SkipVerifyLogs {
		gomega.Expect(backupErrorLogs).Should(gomega.Equal([]string{}))
	}

	// check if backup succeeded
	succeeded, err := lib.IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, h.Client, namespace, backupName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(succeeded).To(gomega.Equal(true))
	log.Printf("Backup for case %s succeeded", brCase.Name)

	if brCase.BackupRestoreType == lib.CSI {
		// wait for volume snapshot to be Ready
		gomega.Eventually(lib.AreVolumeSnapshotsReady(h.Client, backupName), time.Minute*4, time.Second*10).Should(gomega.BeTrue())
	}

	return nsRequiresResticDCWorkaround
}

package e2e_test

import (
	"context"
	"log"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

type HCPBackupRestoreCase struct {
	BackupRestoreCase
	Template string
	Provider string
}

func runHCPBackupAndRestore(brCase HCPBackupRestoreCase, updateLastBRcase func(brCase HCPBackupRestoreCase), h *lib.HCHandler) {
	updateLastBRcase(brCase)

	log.Printf("Preparing backup and restore")
	backupName, restoreName := prepareBackupAndRestore(brCase.BackupRestoreCase, func() {})

	err := lib.AddHCPPluginToDPA(h, dpaCR.Namespace, dpaCR.Name, false)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to add HCP plugin to DPA: %v", err)
	// TODO: move the wait for HC just after the DPA modification to allow reconciliation to go ahead without waiting for the HC to be created

	//Wait for HCP plugin to be added
	gomega.Eventually(lib.IsHCPPluginAdded(h.Client, dpaCR.Namespace, dpaCR.Name), 3*time.Minute, 1*time.Second).Should(gomega.BeTrue())

	// Create the HostedCluster for the test
	h.HostedCluster, err = h.DeployHCManifest(brCase.Template, brCase.Provider)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

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
	err = lib.RemoveHCP(h)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to remove HCP: %v", err)

	// Restore HC
	log.Printf("Restoring HC")
	runHCPRestore(brCase.BackupRestoreCase, backupName, restoreName, nsRequiresResticDCWorkaround)

	// Wait for HCP to be restored
	log.Printf("Validating HC")
	err = lib.ValidateHCP(8*time.Minute, []string{})(h.Client, lib.ClustersNamespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run HCP post-restore verification: %v", err)
}

var _ = ginkgo.Describe("HCP Backup and Restore tests", ginkgo.Ordered, func() {
	var (
		lastInstallTime time.Time
		lastBRCase      HCPBackupRestoreCase
		h               *lib.HCHandler
		ctx             context.Context
		err             error
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
		gomega.Eventually(lib.IsDeploymentReady(h.Client, lib.MCENamespace, lib.MCEOperatorName), time.Minute*5, time.Second*5).Should(gomega.BeTrue())
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		// Deploy MCE manifest
		err = h.DeployMCEManifest()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		// Validate the Hypershift operator
		gomega.Eventually(lib.IsDeploymentReady(h.Client, lib.HONamespace, lib.HypershiftOperatorName), time.Minute*5, time.Second*5).Should(gomega.BeTrue())
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	// After All
	var _ = ginkgo.AfterAll(func() {
		err := lib.RemoveHCP(h)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to remove HCP: %v", err)
	})

	// After Each
	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		tearDownBackupAndRestore(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	ginkgo.DescribeTable("Basic HCP backup and restore test",
		func(brCase HCPBackupRestoreCase, expectedErr error) {
			if ginkgo.CurrentSpecReport().NumAttempts > 1 && !knownFlake {
				ginkgo.Fail("No known FLAKE found in a previous run, marking test as failed.")
			}
			runHCPBackupAndRestore(brCase, updateLastBRcase, h)
		},

		// Test Cases
		ginkgo.Entry("None HostedCluster backup and restore", ginkgo.Label("hcp"), HCPBackupRestoreCase{
			Template: lib.HCPNoneManifest,
			Provider: "None",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         lib.HCPNamespace,
				Name:              lib.HostedClusterName,
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   lib.ValidateHCP(25*time.Minute, []string{}),
				PostRestoreVerify: lib.ValidateHCP(25*time.Minute, []string{}),
				BackupTimeout:     30 * time.Minute,
			},
		}, nil),

		ginkgo.Entry("Agent HostedCluster backup and restore", ginkgo.Label("hcp"), HCPBackupRestoreCase{
			Template: lib.HCPAgentManifest,
			Provider: "Agent",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         lib.HCPNamespace,
				Name:              lib.HostedClusterName,
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   lib.ValidateHCP(25*time.Minute, []string{}),
				PostRestoreVerify: lib.ValidateHCP(25*time.Minute, []string{}),
				BackupTimeout:     30 * time.Minute,
			},
		}, nil),
	)
})

// TODO: Modify the runBackup function to inject the filtered error logs to avoid repeating code with this
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

	// Check error logs for non-relevant errors
	filteredBackupErrorLogs := lib.FilterErrorLogs(backupErrorLogs)

	if !brCase.SkipVerifyLogs {
		gomega.Expect(filteredBackupErrorLogs).Should(gomega.Equal([]string{}))
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

// TODO: Modify the runRestore function to inject the filtered error logs to avoid repeating code with this
func runHCPRestore(brCase BackupRestoreCase, backupName string, restoreName string, nsRequiresResticDCWorkaround bool) {
	log.Printf("Creating restore %s for case %s", restoreName, brCase.Name)
	err := lib.CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Eventually(lib.IsRestoreDone(dpaCR.Client, namespace, restoreName), time.Minute*60, time.Second*10).Should(gomega.BeTrue())
	// TODO only log on fail?
	describeRestore := lib.DescribeRestore(dpaCR.Client, namespace, restoreName)
	ginkgo.GinkgoWriter.Println(describeRestore)

	restoreLogs := lib.RestoreLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	restoreErrorLogs := lib.RestoreErrorLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	accumulatedTestLogs = append(accumulatedTestLogs, describeRestore, restoreLogs)

	// Check error logs for non-relevant errors
	filteredRestoreErrorLogs := lib.FilterErrorLogs(restoreErrorLogs)

	if !brCase.SkipVerifyLogs {
		gomega.Expect(filteredRestoreErrorLogs).Should(gomega.Equal([]string{}))
	}

	// Check if restore succeeded
	succeeded, err := lib.IsRestoreCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(succeeded).To(gomega.Equal(true))

	if nsRequiresResticDCWorkaround {
		// We run the dc-post-restore.sh script for both restic and
		// kopia backups and for any DCs with attached volumes,
		// regardless of whether it was restic or kopia backup.
		// The script is designed to work with labels set by the
		// openshift-velero-plugin and can be run without pre-conditions.
		log.Printf("Running dc-post-restore.sh script.")
		err = lib.RunDcPostRestoreScript(restoreName)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
}

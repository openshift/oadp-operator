package e2e_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
	libhcp "github.com/openshift/oadp-operator/tests/e2e/lib/hcp"
)

type HCPBackupRestoreCase struct {
	BackupRestoreCase
	Template string
	Provider string
}

func runHCPBackupAndRestore(brCase HCPBackupRestoreCase, updateLastBRcase func(brCase HCPBackupRestoreCase), h *libhcp.HCHandler) {
	updateLastBRcase(brCase)

	log.Printf("Preparing backup and restore")
	backupName, restoreName := prepareBackupAndRestore(brCase.BackupRestoreCase, func() {})

	err := h.AddHCPPluginToDPA(dpaCR.Namespace, dpaCR.Name, false)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to add HCP plugin to DPA: %v", err)
	// TODO: move the wait for HC just after the DPA modification to allow reconciliation to go ahead without waiting for the HC to be created

	//Wait for HCP plugin to be added
	gomega.Eventually(libhcp.IsHCPPluginAdded(h.Client, dpaCR.Namespace, dpaCR.Name), 3*time.Minute, 1*time.Second).Should(gomega.BeTrue())

	// Create the HostedCluster for the test
	h.HCPNamespace = libhcp.GetHCPNamespace(brCase.BackupRestoreCase.Name, libhcp.ClustersNamespace)
	h.HostedCluster, err = h.DeployHCManifest(brCase.Template, brCase.Provider, brCase.BackupRestoreCase.Name)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	if brCase.PreBackupVerify != nil {
		err := brCase.PreBackupVerify(runTimeClientForSuiteRun, brCase.Namespace)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run HCP pre-backup verification: %v", err)
	}

	// Backup HCP & HC
	log.Printf("Backing up HC")
	includedResources := libhcp.HCPIncludedResources
	excludedResources := libhcp.HCPExcludedResources
	includedNamespaces := append(libhcp.HCPIncludedNamespaces, libhcp.GetHCPNamespace(h.HostedCluster.Name, libhcp.ClustersNamespace))

	nsRequiresResticDCWorkaround := runHCPBackup(brCase.BackupRestoreCase, backupName, h, includedNamespaces, includedResources, excludedResources)

	// Delete everything in HCP namespace
	log.Printf("Deleting HCP & HC")
	err = h.RemoveHCP(libhcp.Wait10Min)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to remove HCP: %v", err)

	// Restore HC
	log.Printf("Restoring HC")
	runHCPRestore(brCase.BackupRestoreCase, backupName, restoreName, nsRequiresResticDCWorkaround)

	// Wait for HCP to be restored
	log.Printf("Validating HC")
	err = libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, h.HCPNamespace)(h.Client, libhcp.ClustersNamespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run HCP post-restore verification: %v", err)
}

var _ = ginkgo.Describe("HCP Backup and Restore tests", ginkgo.Ordered, func() {
	var (
		lastInstallTime time.Time
		lastBRCase      HCPBackupRestoreCase
		h               *libhcp.HCHandler
		err             error
		ctx             = context.Background()
	)

	updateLastBRcase := func(brCase HCPBackupRestoreCase) {
		lastBRCase = brCase
	}

	// Before All
	var _ = ginkgo.BeforeAll(func() {
		reqOperators := []libhcp.RequiredOperator{
			{
				Name:          libhcp.MCEName,
				Namespace:     libhcp.MCENamespace,
				OperatorGroup: libhcp.MCEOperatorGroup,
			},
		}

		// Install MCE and Hypershift operators
		h, err = libhcp.InstallRequiredOperators(ctx, runTimeClientForSuiteRun, reqOperators)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(h).ToNot(gomega.BeNil())
		gomega.Eventually(lib.IsDeploymentReady(h.Client, libhcp.MCENamespace, libhcp.MCEOperatorName), libhcp.Wait10Min, time.Second*5).Should(gomega.BeTrue())

		// Deploy the MCE manifest
		err = h.DeployMCEManifest()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		// Deploy the MCE and wait for it to be ready
		gomega.Eventually(lib.IsDeploymentReady(h.Client, libhcp.MCENamespace, libhcp.MCEOperatorName), libhcp.Wait10Min, time.Second*5).Should(gomega.BeTrue())
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		// Validate the Hypershift operator
		gomega.Eventually(lib.IsDeploymentReady(h.Client, libhcp.HONamespace, libhcp.HypershiftOperatorName), libhcp.Wait10Min, time.Second*5).Should(gomega.BeTrue())
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	// After All
	var _ = ginkgo.AfterAll(func() {
		err := h.RemoveHCP(libhcp.Wait10Min)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to remove HCP: %v", err)
	})

	// After Each
	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		h.RemoveHCP(libhcp.Wait10Min)
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
			Template: libhcp.HCPNoneManifest,
			Provider: "None",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         libhcp.GetHCPNamespace(fmt.Sprintf("%s-none", libhcp.HostedClusterPrefix), libhcp.ClustersNamespace),
				Name:              fmt.Sprintf("%s-none", libhcp.HostedClusterPrefix),
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(fmt.Sprintf("%s-none", libhcp.HostedClusterPrefix), libhcp.ClustersNamespace)),
				PostRestoreVerify: libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(fmt.Sprintf("%s-none", libhcp.HostedClusterPrefix), libhcp.ClustersNamespace)),
				BackupTimeout:     libhcp.HCPBackupTimeout,
			},
		}, nil),

		ginkgo.Entry("Agent HostedCluster backup and restore", ginkgo.Label("hcp"), HCPBackupRestoreCase{
			Template: libhcp.HCPAgentManifest,
			Provider: "Agent",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         libhcp.GetHCPNamespace(fmt.Sprintf("%s-agent", libhcp.HostedClusterPrefix), libhcp.ClustersNamespace),
				Name:              fmt.Sprintf("%s-agent", libhcp.HostedClusterPrefix),
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(fmt.Sprintf("%s-agent", libhcp.HostedClusterPrefix), libhcp.ClustersNamespace)),
				PostRestoreVerify: libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(fmt.Sprintf("%s-agent", libhcp.HostedClusterPrefix), libhcp.ClustersNamespace)),
				BackupTimeout:     libhcp.HCPBackupTimeout,
			},
		}, nil),
	)
})

// TODO: Modify the runBackup function to inject the filtered error logs to avoid repeating code with this
func runHCPBackup(brCase BackupRestoreCase, backupName string, h *libhcp.HCHandler, namespaces []string, includedResources, excludedResources []string) bool {
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
	filteredBackupErrorLogs := libhcp.FilterErrorLogs(backupErrorLogs)

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
	filteredRestoreErrorLogs := libhcp.FilterErrorLogs(restoreErrorLogs)

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

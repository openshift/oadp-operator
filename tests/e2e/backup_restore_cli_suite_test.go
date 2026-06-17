package e2e_test

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

// CLI-specific backup execution
func runBackupViaCLI(brCase BackupRestoreCase, backupName string) bool {
	nsRequiresResticDCWorkaround, err := lib.NamespaceRequiresResticDCWorkaround(dpaCR.Client, brCase.Namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	if strings.Contains(brCase.Name, "twovol") {
		volumeSyncDelay := 30 * time.Second
		log.Printf("Sleeping for %v to allow volume to be in sync with /tmp/log/ for case %s", volumeSyncDelay, brCase.Name)
		time.Sleep(volumeSyncDelay)
	}

	// Create backup via CLI
	log.Printf("Creating backup %s for case %s via CLI", backupName, brCase.Name)
	err = lib.CreateBackupForNamespacesViaCLI(backupName, []string{brCase.Namespace}, brCase.BackupRestoreType == lib.RESTIC || brCase.BackupRestoreType == lib.KOPIA, brCase.BackupRestoreType == lib.CSIDataMover)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// Wait for backup via CLI
	gomega.Eventually(lib.IsBackupDoneViaCLI(backupName), brCase.BackupTimeout, time.Second*10).Should(gomega.BeTrue())

	// Get backup details via CLI
	describeBackup := lib.DescribeBackupViaCLI(backupName)
	ginkgo.GinkgoWriter.Println(describeBackup)

	backupLogs, err := lib.BackupLogsViaCLI(backupName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	backupErrorLogs := lib.BackupErrorLogsViaCLI(backupName)
	accumulatedTestLogs = append(accumulatedTestLogs, describeBackup, backupLogs)

	if !brCase.SkipVerifyLogs {
		gomega.Expect(backupErrorLogs).Should(gomega.Equal([]string{}))
	}

	// Check if backup succeeded
	succeeded, err := lib.IsBackupCompletedSuccessfullyViaCLI(backupName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(succeeded).To(gomega.Equal(true))

	log.Printf("Backup for case %s succeeded via CLI", brCase.Name)

	return nsRequiresResticDCWorkaround
}

// CLI-specific restore execution
func runRestoreViaCLI(brCase BackupRestoreCase, backupName, restoreName string, nsRequiresResticDCWorkaround bool) {
	log.Printf("Creating restore %s for case %s via CLI", restoreName, brCase.Name)
	err := lib.CreateRestoreFromBackupViaCLI(backupName, restoreName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	gomega.Eventually(lib.IsRestoreDoneViaCLI(restoreName), time.Minute*60, time.Second*10).Should(gomega.BeTrue())

	// Get restore details via CLI
	describeRestore := lib.DescribeRestoreViaCLI(restoreName)
	ginkgo.GinkgoWriter.Println(describeRestore)

	restoreLogs, err := lib.RestoreLogsViaCLI(restoreName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	restoreErrorLogs := lib.RestoreErrorLogsViaCLI(restoreName)
	accumulatedTestLogs = append(accumulatedTestLogs, describeRestore, restoreLogs)

	if !brCase.SkipVerifyLogs {
		gomega.Expect(restoreErrorLogs).Should(gomega.Equal([]string{}))
	}

	// Check if restore succeeded
	succeeded, err := lib.IsRestoreCompletedSuccessfullyViaCLI(restoreName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(succeeded).To(gomega.Equal(true))

	if nsRequiresResticDCWorkaround {
		log.Printf("Running dc-post-restore.sh script.")
		err = lib.RunDcPostRestoreScript(restoreName)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
}

// CLI-specific application backup and restore
func runApplicationBackupAndRestoreViaCLI(brCase ApplicationBackupRestoreCase, updateLastBRcase func(brCase ApplicationBackupRestoreCase), updateLastInstallTime func()) {
	updateLastBRcase(brCase)

	// create DPA (still using K8s client for setup)
	backupName, restoreName := prepareBackupAndRestore(brCase.BackupRestoreCase, updateLastInstallTime)

	// Ensure that an existing backup repository is deleted (still using K8s client)
	brerr := lib.DeleteBackupRepositories(runTimeClientForSuiteRun, namespace)
	gomega.Expect(brerr).ToNot(gomega.HaveOccurred())

	// install app (still using K8s client for setup)
	updateLastInstallTime()
	log.Printf("Installing application for case %s", brCase.Name)
	err := lib.InstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	if brCase.BackupRestoreType == lib.CSI || brCase.BackupRestoreType == lib.CSIDataMover {
		log.Printf("Creating pvc for case %s", brCase.Name)
		var pvcName string
		var pvcPath string

		pvcName = provider
		if brCase.PvcSuffixName != "" {
			pvcName += brCase.PvcSuffixName
		}

		pvcPathFormat := "./sample-applications/%s/pvc/%s.yaml"
		if strings.Contains(brCase.Name, "twovol") {
			pvcPathFormat = "./sample-applications/%s/pvc-twoVol/%s.yaml"
		}

		pvcPath = fmt.Sprintf(pvcPathFormat, brCase.Namespace, pvcName)

		err = lib.InstallApplication(dpaCR.Client, pvcPath)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}

	// Run optional custom verification
	if brCase.PreBackupVerify != nil {
		log.Printf("Running pre-backup custom function for case %s", brCase.Name)
		err := brCase.PreBackupVerify(dpaCR.Client, brCase.Namespace)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}

	// do the backup via CLI
	nsRequiredResticDCWorkaround := runBackupViaCLI(brCase.BackupRestoreCase, backupName)

	// uninstall app (still using K8s client)
	log.Printf("Uninstalling app for case %s", brCase.Name)
	err = lib.UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// Wait for namespace to be deleted
	gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), time.Minute*4, time.Second*5).Should(gomega.BeTrue())

	updateLastInstallTime()

	// run restore via CLI
	runRestoreViaCLI(brCase.BackupRestoreCase, backupName, restoreName, nsRequiredResticDCWorkaround)

	// Run optional custom verification
	if brCase.PostRestoreVerify != nil {
		log.Printf("Running post-restore custom function for case %s", brCase.Name)
		err = brCase.PostRestoreVerify(dpaCR.Client, brCase.Namespace)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
}

// CLI teardown function
func tearDownBackupAndRestoreViaCLI(brCase BackupRestoreCase, installTime time.Time, report ginkgo.SpecReport) {
	log.Println("Post backup and restore state (CLI): ", report.State.String())

	if report.Failed() {
		knownFlake = lib.CheckIfFlakeOccurred(accumulatedTestLogs)
		accumulatedTestLogs = nil
		getFailedTestLogs(namespace, brCase.Namespace, installTime, report)
	}

	if brCase.BackupRestoreType == lib.CSI || brCase.BackupRestoreType == lib.CSIDataMover {
		log.Printf("Deleting VolumeSnapshot for CSI backuprestore of %s", brCase.Name)
		snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", provider)
		err := lib.UninstallApplication(dpaCR.Client, snapshotClassPath)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}

	err := dpaCR.Delete()
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	err = lib.DeleteNamespace(kubernetesClientForSuiteRun, brCase.Namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), time.Minute*5, time.Second*5).Should(gomega.BeTrue())
}

// CLI Test Suite
var _ = ginkgo.Describe("Backup and restore tests via OADP CLI", ginkgo.Label("cli"), ginkgo.Ordered, func() {
	var lastBRCase ApplicationBackupRestoreCase
	var lastInstallTime time.Time
	updateLastBRcase := func(brCase ApplicationBackupRestoreCase) {
		lastBRCase = brCase
	}
	updateLastInstallTime := func() {
		lastInstallTime = time.Now()
	}

	ginkgo.BeforeAll(func() {

		cliSetup := lib.NewOADPCLISetup()
		if err := cliSetup.Install(); err != nil {
			ginkgo.Fail(fmt.Sprintf("OADP CLI setup failed: %v", err))
		}
	})

	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		tearDownBackupAndRestoreViaCLI(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	var _ = ginkgo.AfterAll(func() {
		// Same cleanup as original
		waitOADPReadiness(lib.KOPIA)

		log.Printf("Creating real DataProtectionTest before must-gather")
		bsls, err := dpaCR.ListBSLs()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		bslName := bsls.Items[0].Name
		err = lib.CreateUploadTestOnlyDPT(dpaCR.Client, dpaCR.Namespace, bslName)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		log.Printf("skipMustGather: %v", skipMustGather)
		if !skipMustGather {
			log.Printf("Running OADP must-gather")
			err = lib.RunMustGather(artifact_dir, dpaCR.Client)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}

		err = dpaCR.Delete()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	ginkgo.DescribeTable("Backup and restore applications via OADP CLI",
		func(brCase ApplicationBackupRestoreCase, expectedErr error) {
			if ginkgo.CurrentSpecReport().NumAttempts > 1 && !knownFlake {
				ginkgo.Fail("No known FLAKE found in a previous run, marking test as failed.")
			}
			runApplicationBackupAndRestoreViaCLI(brCase, updateLastBRcase, updateLastInstallTime)
		},
		ginkgo.Entry("MySQL application CSI via CLI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-csi-cli-e2e",
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application CSI via CLI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-csi-cli-e2e",
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application two Vol CSI via CLI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-twovol-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-twovol-csi-cli-e2e",
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   todoListReady(true, true, "mysql"),
				PostRestoreVerify: todoListReady(false, true, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application RESTIC via CLI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-restic-cli-e2e",
				BackupRestoreType: lib.RESTIC,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application RESTIC via CLI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-restic-cli-e2e",
				BackupRestoreType: lib.RESTIC,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application KOPIA via CLI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-kopia-cli-e2e",
				BackupRestoreType: lib.KOPIA,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application KOPIA via CLI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-kopia-cli-e2e",
				BackupRestoreType: lib.KOPIA,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application DATAMOVER via CLI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-datamover-cli-e2e",
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application DATAMOVER via CLI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-datamover-cli-e2e",
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application BlockDevice DATAMOVER via CLI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-block.yaml",
			PvcSuffixName:       "-block-mode",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-blockdevice-cli-e2e",
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application Native-Snapshots via CLI", ginkgo.FlakeAttempts(flakeAttempts), ginkgo.Label("aws", "azure", "gcp"), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-native-snapshots-cli-e2e",
				BackupRestoreType: lib.NativeSnapshots,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application Native-Snapshots via CLI", ginkgo.FlakeAttempts(flakeAttempts), ginkgo.Label("aws", "azure", "gcp"), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-native-snapshots-cli-e2e",
				BackupRestoreType: lib.NativeSnapshots,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
	)
})

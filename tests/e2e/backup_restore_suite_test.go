package e2e_test

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VerificationFunction func(client.Client, string) error

type BackupRestoreCase struct {
	Namespace         string
	Name              string
	BackupRestoreType BackupRestoreType
	PreBackupVerify   VerificationFunction
	PostRestoreVerify VerificationFunction
	SkipVerifyLogs    bool // TODO remove
	BackupTimeout     time.Duration
}

type ApplicationBackupRestoreCase struct {
	BackupRestoreCase
	ApplicationTemplate string
	PvcSuffixName       string
}

func todoListReady(preBackupState bool, twoVol bool, database string) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		log.Printf("checking for the NAMESPACE: %s", namespace)
		Eventually(IsDeploymentReady(ocClient, namespace, database), time.Minute*10, time.Second*10).Should(BeTrue())
		Eventually(IsDCReady(ocClient, namespace, "todolist"), time.Minute*10, time.Second*10).Should(BeTrue())
		Eventually(AreApplicationPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*9, time.Second*5).Should(BeTrue())
		// This test confirms that SCC restore logic in our plugin is working
		err := DoesSCCExist(ocClient, database+"-persistent-scc")
		if err != nil {
			return err
		}
		err = VerifyBackupRestoreData(runTimeClientForSuiteRun, artifact_dir, namespace, "todolist-route", "todolist", preBackupState, twoVol)
		return err
	})
}

func waitOADPReadiness(backupRestoreType BackupRestoreType) {
	err := dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, dpaCR.Build(backupRestoreType))
	Expect(err).NotTo(HaveOccurred())

	log.Print("Checking if DPA is reconciled")
	Eventually(dpaCR.IsReconciledTrue(), time.Minute*3, time.Second*5).Should(BeTrue())

	log.Print("Checking if velero Pod is running")
	Eventually(VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(BeTrue())

	if backupRestoreType == RESTIC || backupRestoreType == KOPIA || backupRestoreType == CSIDataMover {
		log.Printf("Waiting for Node Agent pods to be running")
		Eventually(AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(BeTrue())
	}

	log.Print("Checking if BSL is available")
	Eventually(dpaCR.BSLsAreAvailable(), time.Minute*3, time.Second*5).Should(BeTrue())
}

func prepareBackupAndRestore(brCase BackupRestoreCase, updateLastInstallTime func()) (string, string) {
	updateLastInstallTime()

	waitOADPReadiness(brCase.BackupRestoreType)

	if brCase.BackupRestoreType == CSI || brCase.BackupRestoreType == CSIDataMover {
		if provider == "aws" || provider == "ibmcloud" || provider == "gcp" || provider == "azure" {
			log.Printf("Creating VolumeSnapshotClass for CSI backuprestore of %s", brCase.Name)
			snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", provider)
			err := InstallApplication(dpaCR.Client, snapshotClassPath)
			Expect(err).ToNot(HaveOccurred())
		}
	}

	// TODO: check registry deployments are deleted
	// TODO: check S3 for images

	backupUid, _ := uuid.NewUUID()
	restoreUid, _ := uuid.NewUUID()
	backupName := fmt.Sprintf("%s-%s", brCase.Name, backupUid.String())
	restoreName := fmt.Sprintf("%s-%s", brCase.Name, restoreUid.String())

	return backupName, restoreName
}

func runApplicationBackupAndRestore(brCase ApplicationBackupRestoreCase, expectedErr error, updateLastBRcase func(brCase ApplicationBackupRestoreCase), updateLastInstallTime func()) {
	updateLastBRcase(brCase)

	// create DPA
	backupName, restoreName := prepareBackupAndRestore(brCase.BackupRestoreCase, updateLastInstallTime)

	// install app
	updateLastInstallTime()
	log.Printf("Installing application for case %s", brCase.Name)
	err := InstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
	Expect(err).ToNot(HaveOccurred())
	if brCase.BackupRestoreType == CSI || brCase.BackupRestoreType == CSIDataMover {
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

		err = InstallApplication(dpaCR.Client, pvcPath)
		Expect(err).ToNot(HaveOccurred())
	}

	// Run optional custom verification
	if brCase.PreBackupVerify != nil {
		log.Printf("Running pre-backup custom function for case %s", brCase.Name)
		err := brCase.PreBackupVerify(dpaCR.Client, brCase.Namespace)
		Expect(err).ToNot(HaveOccurred())
	}

	// do the backup for real
	nsRequiredResticDCWorkaround := runBackup(brCase.BackupRestoreCase, backupName)

	// uninstall app
	log.Printf("Uninstalling app for case %s", brCase.Name)
	err = UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
	Expect(err).ToNot(HaveOccurred())

	// Wait for namespace to be deleted
	Eventually(IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), time.Minute*4, time.Second*5).Should(BeTrue())

	updateLastInstallTime()

	// run restore
	runRestore(brCase.BackupRestoreCase, backupName, restoreName, nsRequiredResticDCWorkaround)

	// Run optional custom verification
	if brCase.PostRestoreVerify != nil {
		log.Printf("Running post-restore custom function for case %s", brCase.Name)
		err = brCase.PostRestoreVerify(dpaCR.Client, brCase.Namespace)
		Expect(err).ToNot(HaveOccurred())
	}
}

func runBackup(brCase BackupRestoreCase, backupName string) bool {
	nsRequiresResticDCWorkaround, err := NamespaceRequiresResticDCWorkaround(dpaCR.Client, brCase.Namespace)
	Expect(err).ToNot(HaveOccurred())

	if strings.Contains(brCase.Name, "twovol") {
		volumeSyncDelay := 30 * time.Second
		log.Printf("Sleeping for %v to allow volume to be in sync with /tmp/log/ for case %s", volumeSyncDelay, brCase.Name)
		// TODO this should be a function, not an arbitrary sleep
		time.Sleep(volumeSyncDelay)
	}

	// create backup
	log.Printf("Creating backup %s for case %s", backupName, brCase.Name)
	err = CreateBackupForNamespaces(dpaCR.Client, namespace, backupName, []string{brCase.Namespace}, brCase.BackupRestoreType == RESTIC || brCase.BackupRestoreType == KOPIA, brCase.BackupRestoreType == CSIDataMover)
	Expect(err).ToNot(HaveOccurred())

	// wait for backup to not be running
	Eventually(IsBackupDone(dpaCR.Client, namespace, backupName), brCase.BackupTimeout, time.Second*10).Should(BeTrue())
	// TODO only log on fail?
	describeBackup := DescribeBackup(veleroClientForSuiteRun, dpaCR.Client, namespace, backupName)
	GinkgoWriter.Println(describeBackup)

	backupLogs := BackupLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
	backupErrorLogs := BackupErrorLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
	accumulatedTestLogs = append(accumulatedTestLogs, describeBackup, backupLogs)

	if !brCase.SkipVerifyLogs {
		Expect(backupErrorLogs).Should(Equal([]string{}))
	}

	// check if backup succeeded
	succeeded, err := IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
	Expect(err).ToNot(HaveOccurred())
	Expect(succeeded).To(Equal(true))
	log.Printf("Backup for case %s succeeded", brCase.Name)

	if brCase.BackupRestoreType == CSI {
		// wait for volume snapshot to be Ready
		Eventually(AreVolumeSnapshotsReady(dpaCR.Client, backupName), time.Minute*4, time.Second*10).Should(BeTrue())
	}

	return nsRequiresResticDCWorkaround
}

func runRestore(brCase BackupRestoreCase, backupName, restoreName string, nsRequiresResticDCWorkaround bool) {
	log.Printf("Creating restore %s for case %s", restoreName, brCase.Name)
	err := CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
	Expect(err).ToNot(HaveOccurred())
	Eventually(IsRestoreDone(dpaCR.Client, namespace, restoreName), time.Minute*60, time.Second*10).Should(BeTrue())
	// TODO only log on fail?
	describeRestore := DescribeRestore(veleroClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	GinkgoWriter.Println(describeRestore)

	restoreLogs := RestoreLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	restoreErrorLogs := RestoreErrorLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	accumulatedTestLogs = append(accumulatedTestLogs, describeRestore, restoreLogs)

	if !brCase.SkipVerifyLogs {
		Expect(restoreErrorLogs).Should(Equal([]string{}))
	}

	// Check if restore succeeded
	succeeded, err := IsRestoreCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	Expect(err).ToNot(HaveOccurred())
	Expect(succeeded).To(Equal(true))

	if nsRequiresResticDCWorkaround {
		// We run the dc-post-restore.sh script for both restic and
		// kopia backups and for any DCs with attached volumes,
		// regardless of whether it was restic or kopia backup.
		// The script is designed to work with labels set by the
		// openshift-velero-plugin and can be run without pre-conditions.
		log.Printf("Running dc-post-restore.sh script.")
		err = RunDcPostRestoreScript(restoreName)
		Expect(err).ToNot(HaveOccurred())
	}
}

func getFailedTestLogs(oadpNamespace string, appNamespace string, installTime time.Time, report SpecReport) {
	baseReportDir := artifact_dir + "/" + report.LeafNodeText
	err := os.MkdirAll(baseReportDir, 0755)
	Expect(err).NotTo(HaveOccurred())

	log.Println("Printing OADP namespace events")
	PrintNamespaceEventsAfterTime(kubernetesClientForSuiteRun, oadpNamespace, installTime)
	err = SavePodLogs(kubernetesClientForSuiteRun, oadpNamespace, baseReportDir)
	Expect(err).NotTo(HaveOccurred())

	if appNamespace != "" {
		log.Println("Printing app namespace events")
		PrintNamespaceEventsAfterTime(kubernetesClientForSuiteRun, appNamespace, installTime)
		err = SavePodLogs(kubernetesClientForSuiteRun, appNamespace, baseReportDir)
		Expect(err).NotTo(HaveOccurred())
	}
}

func tearDownBackupAndRestore(brCase BackupRestoreCase, installTime time.Time, report SpecReport) {
	log.Println("Post backup and restore state: ", report.State.String())

	if report.Failed() {
		knownFlake = CheckIfFlakeOccurred(accumulatedTestLogs)
		accumulatedTestLogs = nil
		getFailedTestLogs(namespace, brCase.Namespace, installTime, report)
	}
	if brCase.BackupRestoreType == CSI || brCase.BackupRestoreType == CSIDataMover {
		log.Printf("Deleting VolumeSnapshot for CSI backuprestore of %s", brCase.Name)
		snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", provider)
		err := UninstallApplication(dpaCR.Client, snapshotClassPath)
		Expect(err).ToNot(HaveOccurred())
	}

	err := dpaCR.Delete()
	Expect(err).ToNot(HaveOccurred())

	err = DeleteNamespace(kubernetesClientForSuiteRun, brCase.Namespace)
	Expect(err).ToNot(HaveOccurred())
	Eventually(IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), time.Minute*5, time.Second*5).Should(BeTrue())
}

var _ = Describe("Backup and restore tests", Ordered, func() {
	var lastBRCase ApplicationBackupRestoreCase
	var lastInstallTime time.Time
	updateLastBRcase := func(brCase ApplicationBackupRestoreCase) {
		lastBRCase = brCase
	}
	updateLastInstallTime := func() {
		lastInstallTime = time.Now()
	}

	var _ = AfterEach(func(ctx SpecContext) {
		tearDownBackupAndRestore(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	var _ = AfterAll(func() {
		// DPA just needs to have BSL so gathering of backups/restores logs/describe work
		// using kopia to collect more info (DaemonSet)
		waitOADPReadiness(KOPIA)

		log.Printf("Running OADP must-gather")
		err := RunMustGather(artifact_dir, dpaCR.Client)
		Expect(err).ToNot(HaveOccurred())

		err = dpaCR.Delete()
		Expect(err).ToNot(HaveOccurred())
	})

	DescribeTable("Backup and restore applications",
		func(brCase ApplicationBackupRestoreCase, expectedErr error) {
			if CurrentSpecReport().NumAttempts > 1 && !knownFlake {
				Fail("No known FLAKE found in a previous run, marking test as failed.")
			}
			runApplicationBackupAndRestore(brCase, expectedErr, updateLastBRcase, updateLastInstallTime)
		},
		Entry("MySQL application CSI", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-csi-e2e",
				BackupRestoreType: CSI,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("Mongo application CSI", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-csi-e2e",
				BackupRestoreType: CSI,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("MySQL application two Vol CSI", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-twovol-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-twovol-csi-e2e",
				BackupRestoreType: CSI,
				PreBackupVerify:   todoListReady(true, true, "mysql"),
				PostRestoreVerify: todoListReady(false, true, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("Mongo application RESTIC", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-restic-e2e",
				BackupRestoreType: RESTIC,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("MySQL application RESTIC", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-restic-e2e",
				BackupRestoreType: RESTIC,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("Mongo application KOPIA", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-kopia-e2e",
				BackupRestoreType: KOPIA,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("MySQL application KOPIA", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-kopia-e2e",
				BackupRestoreType: KOPIA,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("Mongo application DATAMOVER", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-datamover-e2e",
				BackupRestoreType: CSIDataMover,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("MySQL application DATAMOVER", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-datamover-e2e",
				BackupRestoreType: CSIDataMover,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("Mongo application BlockDevice DATAMOVER", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-block.yaml",
			PvcSuffixName:       "-block-mode",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-blockdevice-e2e",
				BackupRestoreType: CSIDataMover,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
	)
})

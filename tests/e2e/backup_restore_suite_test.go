package e2e_test

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

type VerificationFunction func(client.Client, string) error

type BackupRestoreCase struct {
	Namespace         string
	Name              string
	BackupRestoreType lib.BackupRestoreType
	PreBackupVerify   VerificationFunction
	PostRestoreVerify VerificationFunction
	SkipVerifyLogs    bool // TODO remove
	BackupTimeout     time.Duration
}

type ApplicationBackupRestoreCase struct {
	BackupRestoreCase
	ApplicationTemplate          string
	PvcSuffixName                string
	MustGatherFiles              []string            // list of files expected in must-gather under quay.io.../clusters/clustername/... ie. "namespaces/openshift-adp/oadp.openshift.io/dpa-ts-example-velero/ts-example-velero.yml"
	MustGatherValidationFunction *func(string) error // validation function for must-gather where string parameter is the path to "quay.io.../clusters/clustername/"
}

func todoListReady(preBackupState bool, twoVol bool, database string) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		log.Printf("checking for the NAMESPACE: %s", namespace)
		gomega.Eventually(lib.IsDeploymentReady(ocClient, namespace, database), time.Minute*10, time.Second*10).Should(gomega.BeTrue())
		gomega.Eventually(lib.IsDCReady(ocClient, namespace, "todolist"), time.Minute*10, time.Second*10).Should(gomega.BeTrue())
		gomega.Eventually(lib.AreApplicationPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*9, time.Second*5).Should(gomega.BeTrue())
		// This test confirms that SCC restore logic in our plugin is working
		err := lib.DoesSCCExist(ocClient, database+"-persistent-scc")
		if err != nil {
			return err
		}
		err = lib.VerifyBackupRestoreData(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, kubeConfig, artifact_dir, namespace, "todolist-route", "todolist", "todolist", preBackupState, twoVol)
		return err
	})
}

func prepareBackupAndRestore(brCase BackupRestoreCase, updateLastInstallTime func()) (string, string) {
	updateLastInstallTime()

	err := dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, dpaCR.Build(brCase.BackupRestoreType))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Print("Checking if DPA is reconciled")
	gomega.Eventually(dpaCR.IsReconciledTrue(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

	log.Printf("Waiting for Velero Pod to be running")
	gomega.Eventually(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

	if brCase.BackupRestoreType == lib.RESTIC || brCase.BackupRestoreType == lib.KOPIA || brCase.BackupRestoreType == lib.CSIDataMover {
		log.Printf("Waiting for Node Agent pods to be running")
		gomega.Eventually(lib.AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
	}

	// Velero does not change status of VSL objects. Users can only confirm if VSLs are correct configured when running a native snapshot backup/restore

	log.Print("Checking if BSL is available")
	gomega.Eventually(dpaCR.BSLsAreAvailable(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

	if brCase.BackupRestoreType == lib.CSI || brCase.BackupRestoreType == lib.CSIDataMover {
		if provider == "aws" || provider == "ibmcloud" || provider == "gcp" || provider == "azure" || provider == "openstack" {
			log.Printf("Creating VolumeSnapshotClass for CSI backuprestore of %s", brCase.Name)
			snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", provider)
			err = lib.InstallApplication(dpaCR.Client, snapshotClassPath)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
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

	// Ensure that an existing backup repository is deleted
	brerr := lib.DeleteBackupRepositories(runTimeClientForSuiteRun, namespace)
	gomega.Expect(brerr).ToNot(gomega.HaveOccurred())

	// install app
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

	// do the backup for real
	nsRequiredResticDCWorkaround := runBackup(brCase.BackupRestoreCase, backupName)

	// uninstall app
	log.Printf("Uninstalling app for case %s", brCase.Name)
	err = lib.UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// Wait for namespace to be deleted
	gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), time.Minute*4, time.Second*5).Should(gomega.BeTrue())

	updateLastInstallTime()

	// run restore
	runRestore(brCase.BackupRestoreCase, backupName, restoreName, nsRequiredResticDCWorkaround)

	// Run optional custom verification
	if brCase.PostRestoreVerify != nil {
		log.Printf("Running post-restore custom function for case %s", brCase.Name)
		err = brCase.PostRestoreVerify(dpaCR.Client, brCase.Namespace)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
}

func runBackup(brCase BackupRestoreCase, backupName string) bool {
	nsRequiresResticDCWorkaround, err := lib.NamespaceRequiresResticDCWorkaround(dpaCR.Client, brCase.Namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	if strings.Contains(brCase.Name, "twovol") {
		volumeSyncDelay := 30 * time.Second
		log.Printf("Sleeping for %v to allow volume to be in sync with /tmp/log/ for case %s", volumeSyncDelay, brCase.Name)
		// TODO this should be a function, not an arbitrary sleep
		time.Sleep(volumeSyncDelay)
	}

	// create backup
	log.Printf("Creating backup %s for case %s", backupName, brCase.Name)
	err = lib.CreateBackupForNamespaces(dpaCR.Client, namespace, backupName, []string{brCase.Namespace}, brCase.BackupRestoreType == lib.RESTIC || brCase.BackupRestoreType == lib.KOPIA, brCase.BackupRestoreType == lib.CSIDataMover)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// wait for backup to not be running
	gomega.Eventually(lib.IsBackupDone(dpaCR.Client, namespace, backupName), brCase.BackupTimeout, time.Second*10).Should(gomega.BeTrue())
	// TODO only log on fail?
	describeBackup := lib.DescribeBackup(dpaCR.Client, namespace, backupName)
	ginkgo.GinkgoWriter.Println(describeBackup)

	backupLogs := lib.BackupLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
	backupErrorLogs := lib.BackupErrorLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
	accumulatedTestLogs = append(accumulatedTestLogs, describeBackup, backupLogs)

	if !brCase.SkipVerifyLogs {
		gomega.Expect(backupErrorLogs).Should(gomega.Equal([]string{}))
	}

	// check if backup succeeded
	succeeded, err := lib.IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(succeeded).To(gomega.Equal(true))
	log.Printf("Backup for case %s succeeded", brCase.Name)

	if brCase.BackupRestoreType == lib.CSI {
		// wait for volume snapshot to be Ready
		gomega.Eventually(lib.AreVolumeSnapshotsReady(dpaCR.Client, backupName), time.Minute*4, time.Second*10).Should(gomega.BeTrue())
	}

	return nsRequiresResticDCWorkaround
}

func runRestore(brCase BackupRestoreCase, backupName, restoreName string, nsRequiresResticDCWorkaround bool) {
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

	if !brCase.SkipVerifyLogs {
		gomega.Expect(restoreErrorLogs).Should(gomega.Equal([]string{}))
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

func getFailedTestLogs(oadpNamespace string, appNamespace string, installTime time.Time, report ginkgo.SpecReport) {
	baseReportDir := artifact_dir + "/" + report.LeafNodeText
	err := os.MkdirAll(baseReportDir, 0755)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Println("Printing OADP namespace events")
	lib.PrintNamespaceEventsAfterTime(kubernetesClientForSuiteRun, oadpNamespace, installTime)
	err = lib.SavePodLogs(kubernetesClientForSuiteRun, oadpNamespace, baseReportDir)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	if appNamespace != "" {
		log.Println("Printing app namespace events")
		lib.PrintNamespaceEventsAfterTime(kubernetesClientForSuiteRun, appNamespace, installTime)
		err = lib.SavePodLogs(kubernetesClientForSuiteRun, appNamespace, baseReportDir)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
}

func tearDownBackupAndRestore(brCase BackupRestoreCase, installTime time.Time, report ginkgo.SpecReport) {
	log.Println("Post backup and restore state: ", report.State.String())

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

var _ = ginkgo.Describe("Backup and restore tests", func() {
	var lastBRCase ApplicationBackupRestoreCase
	var lastInstallTime time.Time
	updateLastBRcase := func(brCase ApplicationBackupRestoreCase) {
		lastBRCase = brCase
	}
	updateLastInstallTime := func() {
		lastInstallTime = time.Now()
	}

	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		tearDownBackupAndRestore(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	ginkgo.DescribeTable("Backup and restore applications",
		func(brCase ApplicationBackupRestoreCase, expectedErr error) {
			if ginkgo.CurrentSpecReport().NumAttempts > 1 && !knownFlake {
				ginkgo.Fail("No known FLAKE found in a previous run, marking test as failed.")
			}
			runApplicationBackupAndRestore(brCase, expectedErr, updateLastBRcase, updateLastInstallTime)
		},
		ginkgo.Entry("MySQL application CSI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-csi-e2e",
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application CSI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-csi-e2e",
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application two Vol CSI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-twovol-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-twovol-csi-e2e",
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   todoListReady(true, true, "mysql"),
				PostRestoreVerify: todoListReady(false, true, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application RESTIC", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-restic-e2e",
				BackupRestoreType: lib.RESTIC,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application RESTIC", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-restic-e2e",
				BackupRestoreType: lib.RESTIC,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application KOPIA", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-kopia-e2e",
				BackupRestoreType: lib.KOPIA,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application KOPIA", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-kopia-e2e",
				BackupRestoreType: lib.KOPIA,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application DATAMOVER", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-datamover-e2e",
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application DATAMOVER", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-datamover-e2e",
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application BlockDevice DATAMOVER", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-block.yaml",
			PvcSuffixName:       "-block-mode",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-blockdevice-e2e",
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application Native-Snapshots", ginkgo.FlakeAttempts(flakeAttempts), ginkgo.Label("aws", "azure", "gcp"), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-native-snapshots-e2e",
				BackupRestoreType: lib.NativeSnapshots,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application Native-Snapshots", ginkgo.FlakeAttempts(flakeAttempts), ginkgo.Label("aws", "azure", "gcp"), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-native-snapshots-e2e",
				BackupRestoreType: lib.NativeSnapshots,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
	)
})

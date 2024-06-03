package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VerificationFunction func(client.Client, string) error

type appVerificationFunction func(bool, bool, BackupRestoreType) VerificationFunction

// TODO duplications with mongoready
func mongoready(preBackupState bool, twoVol bool, backupRestoreType BackupRestoreType) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		Eventually(IsDCReady(ocClient, namespace, "todolist"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		exists, err := DoesSCCExist(ocClient, "mongo-persistent-scc")
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("did not find Mongo scc")
		}
		err = VerifyBackupRestoreData(runTimeClientForSuiteRun, artifact_dir, namespace, "todolist-route", "todolist", preBackupState, false, backupRestoreType)
		return err
	})
}

func mysqlReady(preBackupState bool, twoVol bool, backupRestoreType BackupRestoreType) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		log.Printf("checking for the NAMESPACE: %s", namespace)
		// This test confirms that SCC restore logic in our plugin is working
		Eventually(IsDeploymentReady(ocClient, namespace, "mysql"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		exists, err := DoesSCCExist(ocClient, "mysql-persistent-scc")
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("did not find MYSQL scc")
		}
		err = VerifyBackupRestoreData(runTimeClientForSuiteRun, artifact_dir, namespace, "todolist-route", "todolist", preBackupState, twoVol, backupRestoreType)
		return err
	})
}

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
	ApplicationTemplate          string
	PvcSuffixName                string
	MustGatherFiles              []string            // list of files expected in must-gather under quay.io.../clusters/clustername/... ie. "namespaces/openshift-adp/oadp.openshift.io/dpa-ts-example-velero/ts-example-velero.yml"
	MustGatherValidationFunction *func(string) error // validation function for must-gather where string parameter is the path to "quay.io.../clusters/clustername/"
}

func prepareBackupAndRestore(brCase BackupRestoreCase, updateLastInstallTime func()) (string, string) {
	err := dpaCR.Build(brCase.BackupRestoreType)
	Expect(err).NotTo(HaveOccurred())

	//updateLastInstallingNamespace(dpaCR.Namespace)
	updateLastInstallTime()

	err = dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, &dpaCR.CustomResource.Spec)
	Expect(err).NotTo(HaveOccurred())

	log.Printf("Waiting for velero pod to be running")
	Eventually(AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())

	if brCase.BackupRestoreType == RESTIC || brCase.BackupRestoreType == KOPIA || brCase.BackupRestoreType == CSIDataMover {
		log.Printf("Waiting for Node Agent pods to be running")
		Eventually(AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
	}
	if brCase.BackupRestoreType == CSI || brCase.BackupRestoreType == CSIDataMover {
		if provider == "aws" || provider == "ibmcloud" || provider == "gcp" || provider == "azure" {
			log.Printf("Creating VolumeSnapshotClass for CSI backuprestore of %s", brCase.Name)
			snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", provider)
			err = InstallApplication(dpaCR.Client, snapshotClassPath)
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

	// wait for pods to be running
	Eventually(AreAppBuildsReady(dpaCR.Client, brCase.Namespace), timeoutMultiplier*time.Minute*5, time.Second*5).Should(BeTrue())
	Eventually(AreApplicationPodsRunning(kubernetesClientForSuiteRun, brCase.Namespace), timeoutMultiplier*time.Minute*9, time.Second*5).Should(BeTrue())

	// do the backup for real
	nsRequiredResticDCWorkaround := runBackup(brCase.BackupRestoreCase, backupName)

	// uninstall app
	log.Printf("Uninstalling app for case %s", brCase.Name)
	err = UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
	Expect(err).ToNot(HaveOccurred())

	// Wait for namespace to be deleted
	Eventually(IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), timeoutMultiplier*time.Minute*4, time.Second*5).Should(BeTrue())

	updateLastInstallTime()

	// run restore
	runRestore(brCase.BackupRestoreCase, backupName, restoreName, nsRequiredResticDCWorkaround)

	// verify app is running
	Eventually(AreAppBuildsReady(dpaCR.Client, brCase.Namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
	Eventually(AreApplicationPodsRunning(kubernetesClientForSuiteRun, brCase.Namespace), timeoutMultiplier*time.Minute*9, time.Second*5).Should(BeTrue())

	// Run optional custom verification
	if brCase.PostRestoreVerify != nil {
		log.Printf("Running post-restore function for case %s", brCase.Name)
		err = brCase.PostRestoreVerify(dpaCR.Client, brCase.Namespace)
		Expect(err).ToNot(HaveOccurred())
	}
}

func runBackup(brCase BackupRestoreCase, backupName string) bool {
	// Run optional custom verification
	if brCase.PreBackupVerify != nil {
		log.Printf("Running pre-backup function for case %s", brCase.Name)
		err := brCase.PreBackupVerify(dpaCR.Client, brCase.Namespace)
		Expect(err).ToNot(HaveOccurred())
	}

	nsRequiresResticDCWorkaround, err := NamespaceRequiresResticDCWorkaround(dpaCR.Client, brCase.Namespace)
	Expect(err).ToNot(HaveOccurred())

	// create backup
	log.Printf("Creating backup %s for case %s", backupName, brCase.Name)
	backup, err := CreateBackupForNamespaces(dpaCR.Client, namespace, backupName, []string{brCase.Namespace}, brCase.BackupRestoreType == RESTIC || brCase.BackupRestoreType == KOPIA, brCase.BackupRestoreType == CSIDataMover)
	Expect(err).ToNot(HaveOccurred())

	// wait for backup to not be running
	Eventually(IsBackupDone(dpaCR.Client, namespace, backupName), timeoutMultiplier*brCase.BackupTimeout, time.Second*10).Should(BeTrue())
	// TODO only log on fail?
	describeBackup := DescribeBackup(veleroClientForSuiteRun, csiClientForSuiteRun, dpaCR.Client, backup)
	GinkgoWriter.Println(describeBackup)

	backupLogs := BackupLogs(kubernetesClientForSuiteRun, dpaCR.Client, backup)
	backupErrorLogs := BackupErrorLogs(kubernetesClientForSuiteRun, dpaCR.Client, backup)
	accumulatedTestLogs = append(accumulatedTestLogs, describeBackup, backupLogs)

	if !brCase.SkipVerifyLogs {
		Expect(backupErrorLogs).Should(Equal([]string{}))
	}

	// check if backup succeeded
	succeeded, err := IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, backup)
	Expect(err).ToNot(HaveOccurred())
	Expect(succeeded).To(Equal(true))
	log.Printf("Backup for case %s succeeded", brCase.Name)

	if brCase.BackupRestoreType == CSI {
		// wait for volume snapshot to be Ready
		Eventually(AreVolumeSnapshotsReady(dpaCR.Client, backupName), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
	}

	return nsRequiresResticDCWorkaround
}

func runRestore(brCase BackupRestoreCase, backupName, restoreName string, nsRequiresResticDCWorkaround bool) {
	log.Printf("Creating restore %s for case %s", restoreName, brCase.Name)
	restore, err := CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
	Expect(err).ToNot(HaveOccurred())
	Eventually(IsRestoreDone(dpaCR.Client, namespace, restoreName), timeoutMultiplier*time.Minute*60, time.Second*10).Should(BeTrue())
	// TODO only log on fail?
	describeRestore := DescribeRestore(veleroClientForSuiteRun, dpaCR.Client, restore)
	GinkgoWriter.Println(describeRestore)

	restoreLogs := RestoreLogs(kubernetesClientForSuiteRun, dpaCR.Client, restore)
	restoreErrorLogs := RestoreErrorLogs(kubernetesClientForSuiteRun, dpaCR.Client, restore)
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

func tearDownBackupAndRestore(brCase BackupRestoreCase, installTime time.Time, report SpecReport) {
	log.Println("Post backup and restore state: ", report.State.String())
	knownFlake = false
	logString := strings.Join(accumulatedTestLogs, "\n")
	CheckIfFlakeOccured(logString, &knownFlake)
	accumulatedTestLogs = nil

	if report.Failed() {
		// print namespace error events for app namespace
		if brCase.Namespace != "" {
			GinkgoWriter.Println("Printing app namespace events")
			PrintNamespaceEventsAfterTime(kubernetesClientForSuiteRun, brCase.Namespace, installTime)
		}
		GinkgoWriter.Println("Printing oadp namespace events")
		PrintNamespaceEventsAfterTime(kubernetesClientForSuiteRun, namespace, installTime)
		baseReportDir := artifact_dir + "/" + report.LeafNodeText
		err := os.MkdirAll(baseReportDir, 0755)
		Expect(err).NotTo(HaveOccurred())
		err = SavePodLogs(kubernetesClientForSuiteRun, namespace, baseReportDir)
		Expect(err).NotTo(HaveOccurred())
		err = SavePodLogs(kubernetesClientForSuiteRun, brCase.Namespace, baseReportDir)
		Expect(err).NotTo(HaveOccurred())
	}
	if brCase.BackupRestoreType == CSI || brCase.BackupRestoreType == CSIDataMover {
		log.Printf("Deleting VolumeSnapshot for CSI backuprestore of %s", brCase.Name)
		snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", provider)
		err := UninstallApplication(dpaCR.Client, snapshotClassPath)
		Expect(err).ToNot(HaveOccurred())
	}
	err := dpaCR.Client.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:      brCase.Namespace,
		Namespace: brCase.Namespace,
	}}, &client.DeleteOptions{})
	if k8serror.IsNotFound(err) {
		err = nil
	}
	Expect(err).ToNot(HaveOccurred())

	err = dpaCR.Delete(runTimeClientForSuiteRun)
	Expect(err).ToNot(HaveOccurred())
	Eventually(IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), timeoutMultiplier*time.Minute*5, time.Second*5).Should(BeTrue())
}

var _ = Describe("Backup and restore tests", func() {
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
				PreBackupVerify:   mysqlReady(true, false, CSI),
				PostRestoreVerify: mysqlReady(false, false, CSI),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("Mongo application CSI", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-csi-e2e",
				BackupRestoreType: CSI,
				PreBackupVerify:   mongoready(true, false, CSI),
				PostRestoreVerify: mongoready(false, false, CSI),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("MySQL application two Vol CSI", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-twovol-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-twovol-csi-e2e",
				BackupRestoreType: CSI,
				PreBackupVerify:   mysqlReady(true, true, CSI),
				PostRestoreVerify: mysqlReady(false, true, CSI),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("Mongo application RESTIC", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-restic-e2e",
				BackupRestoreType: RESTIC,
				PreBackupVerify:   mongoready(true, false, RESTIC),
				PostRestoreVerify: mongoready(false, false, RESTIC),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("MySQL application RESTIC", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-restic-e2e",
				BackupRestoreType: RESTIC,
				PreBackupVerify:   mysqlReady(true, false, RESTIC),
				PostRestoreVerify: mysqlReady(false, false, RESTIC),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("Mongo application KOPIA", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-kopia-e2e",
				BackupRestoreType: KOPIA,
				PreBackupVerify:   mongoready(true, false, KOPIA),
				PostRestoreVerify: mongoready(false, false, KOPIA),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("MySQL application KOPIA", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-kopia-e2e",
				BackupRestoreType: KOPIA,
				PreBackupVerify:   mysqlReady(true, false, KOPIA),
				PostRestoreVerify: mysqlReady(false, false, KOPIA),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("Mongo application DATAMOVER", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-datamover-e2e",
				BackupRestoreType: CSIDataMover,
				PreBackupVerify:   mongoready(true, false, CSIDataMover),
				PostRestoreVerify: mongoready(false, false, CSIDataMover),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		Entry("MySQL application DATAMOVER", FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-datamover-e2e",
				BackupRestoreType: CSIDataMover,
				PreBackupVerify:   mysqlReady(true, false, CSIDataMover),
				PostRestoreVerify: mysqlReady(false, false, CSIDataMover),
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
				PreBackupVerify:   mongoready(true, false, CSIDataMover),
				PostRestoreVerify: mongoready(false, false, CSIDataMover),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
	)
})

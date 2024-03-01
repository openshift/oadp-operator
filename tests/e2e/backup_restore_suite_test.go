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
	ginkgov2 "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

type VerificationFunction func(client.Client, string) error

type appVerificationFunction func(bool, bool, lib.BackupRestoreType) VerificationFunction

// TODO duplications with mongoready
func mongoready(preBackupState bool, twoVol bool) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		gomega.Eventually(lib.IsDCReady(ocClient, namespace, "todolist"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(gomega.BeTrue())
		exists, err := lib.DoesSCCExist(ocClient, "mongo-persistent-scc")
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("did not find Mongo scc")
		}
		err = lib.VerifyBackupRestoreData(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, kubeConfig, artifact_dir, namespace, "todolist-route", "todolist", "todolist", preBackupState, false)
		return err
	})
}

func mysqlReady(preBackupState bool, twoVol bool) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		log.Printf("checking for the NAMESPACE: %s", namespace)
		// This test confirms that SCC restore logic in our plugin is working
		gomega.Eventually(lib.IsDeploymentReady(ocClient, namespace, "mysql"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(gomega.BeTrue())
		exists, err := lib.DoesSCCExist(ocClient, "mysql-persistent-scc")
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("did not find MYSQL scc")
		}
		err = lib.VerifyBackupRestoreData(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, kubeConfig, artifact_dir, namespace, "todolist-route", "todolist", "todolist", preBackupState, twoVol)
		return err
	})
}

type BackupRestoreCase struct {
	Namespace         string
	Name              string
	BackupRestoreType lib.BackupRestoreType
	PreBackupVerify   VerificationFunction
	PostRestoreVerify VerificationFunction
}

type ApplicationBackupRestoreCase struct {
	BackupRestoreCase
	ApplicationTemplate          string
	PvcSuffixName                string
	AppReadyDelay                time.Duration
	MustGatherFiles              []string            // list of files expected in must-gather under quay.io.../clusters/clustername/... ie. "namespaces/openshift-adp/oadp.openshift.io/dpa-ts-example-velero/ts-example-velero.yml"
	MustGatherValidationFunction *func(string) error // validation function for must-gather where string parameter is the path to "quay.io.../clusters/clustername/"
}

func prepareBackupAndRestore(brCase BackupRestoreCase, updateLastInstallTime func()) (string, string) {
	err := dpaCR.Build(brCase.BackupRestoreType)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	//updateLastInstallingNamespace(dpaCR.Namespace)
	updateLastInstallTime()

	err = dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, &dpaCR.CustomResource.Spec)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Printf("Waiting for velero pod to be running")
	gomega.Eventually(lib.AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(gomega.BeTrue())

	if brCase.BackupRestoreType == lib.RESTIC || brCase.BackupRestoreType == lib.KOPIA || brCase.BackupRestoreType == lib.CSIDataMover {
		log.Printf("Waiting for Node Agent pods to be running")
		gomega.Eventually(lib.AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(gomega.BeTrue())
	}
	if brCase.BackupRestoreType == lib.CSI || brCase.BackupRestoreType == lib.CSIDataMover {
		if provider == "aws" || provider == "ibmcloud" || provider == "gcp" || provider == "azure" {
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

	// wait for pods to be running
	gomega.Eventually(lib.AreAppBuildsReady(dpaCR.Client, brCase.Namespace), timeoutMultiplier*time.Minute*5, time.Second*5).Should(gomega.BeTrue())
	gomega.Eventually(lib.AreApplicationPodsRunning(kubernetesClientForSuiteRun, brCase.Namespace), timeoutMultiplier*time.Minute*9, time.Second*5).Should(gomega.BeTrue())

	// do the backup for real
	nsRequiredResticDCWorkaround := runBackup(brCase.BackupRestoreCase, backupName, brCase.AppReadyDelay)

	// uninstall app
	log.Printf("Uninstalling app for case %s", brCase.Name)
	err = lib.UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// Wait for namespace to be deleted
	gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), timeoutMultiplier*time.Minute*4, time.Second*5).Should(gomega.BeTrue())

	updateLastInstallTime()

	// run restore
	runRestore(brCase.BackupRestoreCase, backupName, restoreName, nsRequiredResticDCWorkaround)

	// verify app is running
	gomega.Eventually(lib.AreAppBuildsReady(dpaCR.Client, brCase.Namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(gomega.BeTrue())
	gomega.Eventually(lib.AreApplicationPodsRunning(kubernetesClientForSuiteRun, brCase.Namespace), timeoutMultiplier*time.Minute*9, time.Second*5).Should(gomega.BeTrue())

	// Run optional custom verification
	log.Printf("Running post-restore function for case %s", brCase.Name)
	err = brCase.PostRestoreVerify(dpaCR.Client, brCase.Namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
}

func runBackup(brCase BackupRestoreCase, backupName string, delay time.Duration) bool {
	// Run optional custom verification
	log.Printf("Running pre-backup function for case %s", brCase.Name)
	err := brCase.PreBackupVerify(dpaCR.Client, brCase.Namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	nsRequiresResticDCWorkaround, err := lib.NamespaceRequiresResticDCWorkaround(dpaCR.Client, brCase.Namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// TODO this should be a function, not an arbitrary sleep
	log.Printf("Sleeping for %v to allow application to be ready for case %s", delay, brCase.Name)
	time.Sleep(delay)
	// create backup
	log.Printf("Creating backup %s for case %s", backupName, brCase.Name)
	backup, err := lib.CreateBackupForNamespaces(dpaCR.Client, namespace, backupName, []string{brCase.Namespace}, brCase.BackupRestoreType == lib.RESTIC || brCase.BackupRestoreType == lib.KOPIA, brCase.BackupRestoreType == lib.CSIDataMover)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// wait for backup to not be running
	gomega.Eventually(lib.IsBackupDone(dpaCR.Client, namespace, backupName), timeoutMultiplier*time.Minute*20, time.Second*10).Should(gomega.BeTrue())
	// TODO only log on fail?
	describeBackup := lib.DescribeBackup(veleroClientForSuiteRun, csiClientForSuiteRun, dpaCR.Client, backup)
	ginkgov2.GinkgoWriter.Println(describeBackup)

	backupLogs := lib.BackupLogs(kubernetesClientForSuiteRun, dpaCR.Client, backup)
	backupErrorLogs := lib.BackupErrorLogs(kubernetesClientForSuiteRun, dpaCR.Client, backup)
	accumulatedTestLogs = append(accumulatedTestLogs, describeBackup, backupLogs)

	gomega.Expect(backupErrorLogs).Should(gomega.Equal([]string{}))

	// check if backup succeeded
	succeeded, err := lib.IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, backup)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(succeeded).To(gomega.Equal(true))
	log.Printf("Backup for case %s succeeded", brCase.Name)

	if brCase.BackupRestoreType == lib.CSI {
		// wait for volume snapshot to be Ready
		gomega.Eventually(lib.AreVolumeSnapshotsReady(dpaCR.Client, backupName), timeoutMultiplier*time.Minute*4, time.Second*10).Should(gomega.BeTrue())
	}

	return nsRequiresResticDCWorkaround
}

func runRestore(brCase BackupRestoreCase, backupName, restoreName string, nsRequiresResticDCWorkaround bool) {
	log.Printf("Creating restore %s for case %s", restoreName, brCase.Name)
	restore, err := lib.CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Eventually(lib.IsRestoreDone(dpaCR.Client, namespace, restoreName), timeoutMultiplier*time.Minute*60, time.Second*10).Should(gomega.BeTrue())
	// TODO only log on fail?
	describeRestore := lib.DescribeRestore(veleroClientForSuiteRun, dpaCR.Client, restore)
	ginkgov2.GinkgoWriter.Println(describeRestore)

	restoreLogs := lib.RestoreLogs(kubernetesClientForSuiteRun, dpaCR.Client, restore)
	restoreErrorLogs := lib.RestoreErrorLogs(kubernetesClientForSuiteRun, dpaCR.Client, restore)
	accumulatedTestLogs = append(accumulatedTestLogs, describeRestore, restoreLogs)

	gomega.Expect(restoreErrorLogs).Should(gomega.Equal([]string{}))

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

func tearDownBackupAndRestore(brCase BackupRestoreCase, installTime time.Time, report ginkgov2.SpecReport) {
	log.Println("Post backup and restore state: ", report.State.String())
	knownFlake = false
	logString := strings.Join(accumulatedTestLogs, "\n")
	lib.CheckIfFlakeOccured(logString, &knownFlake)
	accumulatedTestLogs = nil

	if report.Failed() {
		// print namespace error events for app namespace
		if brCase.Namespace != "" {
			ginkgov2.GinkgoWriter.Println("Printing app namespace events")
			lib.PrintNamespaceEventsAfterTime(kubernetesClientForSuiteRun, brCase.Namespace, installTime)
		}
		ginkgov2.GinkgoWriter.Println("Printing oadp namespace events")
		lib.PrintNamespaceEventsAfterTime(kubernetesClientForSuiteRun, namespace, installTime)
		baseReportDir := artifact_dir + "/" + report.LeafNodeText
		err := os.MkdirAll(baseReportDir, 0755)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		err = lib.SavePodLogs(kubernetesClientForSuiteRun, namespace, baseReportDir)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		err = lib.SavePodLogs(kubernetesClientForSuiteRun, brCase.Namespace, baseReportDir)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
	if brCase.BackupRestoreType == lib.CSI || brCase.BackupRestoreType == lib.CSIDataMover {
		log.Printf("Deleting VolumeSnapshot for CSI backuprestore of %s", brCase.Name)
		snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", provider)
		err := lib.UninstallApplication(dpaCR.Client, snapshotClassPath)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
	err := dpaCR.Client.Delete(context.Background(), &corev1.Namespace{ObjectMeta: v1.ObjectMeta{
		Name:      brCase.Namespace,
		Namespace: brCase.Namespace,
	}}, &client.DeleteOptions{})
	if k8serror.IsNotFound(err) {
		err = nil
	}
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	err = dpaCR.Delete(runTimeClientForSuiteRun)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), timeoutMultiplier*time.Minute*2, time.Second*5).Should(gomega.BeTrue())
}

var _ = ginkgov2.Describe("Backup and restore tests", func() {
	var lastBRCase ApplicationBackupRestoreCase
	var lastInstallTime time.Time
	updateLastBRcase := func(brCase ApplicationBackupRestoreCase) {
		lastBRCase = brCase
	}
	updateLastInstallTime := func() {
		lastInstallTime = time.Now()
	}

	var _ = ginkgov2.AfterEach(func(ctx ginkgov2.SpecContext) {
		tearDownBackupAndRestore(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	ginkgov2.DescribeTable("Backup and restore applications",
		func(brCase ApplicationBackupRestoreCase, expectedErr error) {
			if ginkgov2.CurrentSpecReport().NumAttempts > 1 && !knownFlake {
				ginkgov2.Fail("No known FLAKE found in a previous run, marking test as failed.")
			}
			runApplicationBackupAndRestore(brCase, expectedErr, updateLastBRcase, updateLastInstallTime)
		},
		ginkgov2.Entry("MySQL application CSI", ginkgov2.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-csi-e2e",
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   mysqlReady(true, false),
				PostRestoreVerify: mysqlReady(false, false),
			},
		}, nil),
		ginkgov2.Entry("Mongo application CSI", ginkgov2.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-csi-e2e",
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   mongoready(true, false),
				PostRestoreVerify: mongoready(false, false),
			},
		}, nil),
		ginkgov2.Entry("MySQL application two Vol CSI", ginkgov2.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-twovol-csi.yaml",
			AppReadyDelay:       30 * time.Second,
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-twovol-csi-e2e",
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   mysqlReady(true, true),
				PostRestoreVerify: mysqlReady(false, true),
			},
		}, nil),
		ginkgov2.Entry("Mongo application RESTIC", ginkgov2.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-restic-e2e",
				BackupRestoreType: lib.RESTIC,
				PreBackupVerify:   mongoready(true, false),
				PostRestoreVerify: mongoready(false, false),
			},
		}, nil),
		ginkgov2.Entry("MySQL application RESTIC", ginkgov2.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-restic-e2e",
				BackupRestoreType: lib.RESTIC,
				PreBackupVerify:   mysqlReady(true, false),
				PostRestoreVerify: mysqlReady(false, false),
			},
		}, nil),
		ginkgov2.Entry("Mongo application KOPIA", ginkgov2.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-kopia-e2e",
				BackupRestoreType: lib.KOPIA,
				PreBackupVerify:   mongoready(true, false),
				PostRestoreVerify: mongoready(false, false),
			},
		}, nil),
		ginkgov2.Entry("MySQL application KOPIA", ginkgov2.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-kopia-e2e",
				BackupRestoreType: lib.KOPIA,
				PreBackupVerify:   mysqlReady(true, false),
				PostRestoreVerify: mysqlReady(false, false),
			},
		}, nil),
		ginkgov2.Entry("Mongo application DATAMOVER", ginkgov2.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-datamover-e2e",
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   mongoready(true, false),
				PostRestoreVerify: mongoready(false, false),
			},
		}, nil),
		ginkgov2.Entry("MySQL application DATAMOVER", ginkgov2.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-datamover-e2e",
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   mysqlReady(true, false),
				PostRestoreVerify: mysqlReady(false, false),
			},
		}, nil),
		ginkgov2.Entry("Mongo application BlockDevice DATAMOVER", ginkgov2.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-block.yaml",
			PvcSuffixName:       "-block-mode",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-blockdevice-e2e",
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   mongoready(true, false),
				PostRestoreVerify: mongoready(false, false),
			},
		}, nil),
	)
})

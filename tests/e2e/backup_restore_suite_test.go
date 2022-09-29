package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VerificationFunction func(client.Client, string) error

func mongoready(preBackupState bool, backupRestoreType BackupRestoreType) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		Eventually(IsDCReady(ocClient, namespace, "todolist"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		exists, err := DoesSCCExist(ocClient, "mongo-persistent-scc")
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("did not find Mongo scc")
		}
		err = VerifyBackupRestoreData(artifact_dir, namespace, "todolist-route", "todolist", preBackupState, backupRestoreType) // TODO: VERIFY PARKS APP DATA
		return err
	})
}
func mysqlReady(preBackupState bool, backupRestoreType BackupRestoreType) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		fmt.Printf("checking for the NAMESPACE: %s\n ", namespace)
		// This test confirms that SCC restore logic in our plugin is working
		//Eventually(IsDCReady(ocClient, "mssql-persistent", "mysql"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		Eventually(IsDeploymentReady(ocClient, namespace, "mysql"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		exists, err := DoesSCCExist(ocClient, "mysql-persistent-scc")
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("did not find MYSQL scc")
		}
		err = VerifyBackupRestoreData(artifact_dir, namespace, "todolist-route", "todolist", preBackupState, backupRestoreType)
		return err
	})
}

var _ = Describe("AWS backup restore tests", func() {

	var _ = BeforeEach(func() {
		testSuiteInstanceName := "ts-" + instanceName
		dpaCR.Name = testSuiteInstanceName
	})

	var lastInstallingApplicationNamespace string
	var lastInstallTime time.Time
	var _ = ReportAfterEach(func(report SpecReport) {
		if report.State == types.SpecStateSkipped {
			// do not run if the test is skipped
			return
		}
		GinkgoWriter.Println("Report after each: state: ", report.State.String())
		if report.Failed() {
			// print namespace error events for app namespace
			if lastInstallingApplicationNamespace != "" {
				PrintNamespaceEventsAfterTime(lastInstallingApplicationNamespace, lastInstallTime)
			}
			GinkgoWriter.Println("Printing velero deployment pod logs")
			logs, err := GetVeleroContainerLogs(namespace)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(logs)
			GinkgoWriter.Println("End of velero deployment pod logs")
		}
		// remove app namespace if leftover (likely previously failed before reaching uninstall applications) to clear items such as PVCs which are immutable so that next test can create new ones
		err := dpaCR.Client.Delete(context.Background(), &corev1.Namespace{ObjectMeta: v1.ObjectMeta{
			Name:      lastInstallingApplicationNamespace,
			Namespace: lastInstallingApplicationNamespace,
		}}, &client.DeleteOptions{})
		if k8serror.IsNotFound(err) {
			err = nil
		}
		Expect(err).ToNot(HaveOccurred())
		err = dpaCR.Delete()
		Expect(err).ToNot(HaveOccurred())
	})

	type BackupRestoreCase struct {
		ApplicationTemplate  string
		ApplicationNamespace string
		Name                 string
		BackupRestoreType    BackupRestoreType
		PreBackupVerify      VerificationFunction
		PostRestoreVerify    VerificationFunction
		MaxK8SVersion        *K8sVersion
		MinK8SVersion        *K8sVersion
	}

	updateLastInstallingNamespace := func(namespace string) {
		lastInstallingApplicationNamespace = namespace
		lastInstallTime = time.Now()
	}

	DescribeTable("backup and restore applications",
		func(brCase BackupRestoreCase, expectedErr error) {
			if notVersionTarget, reason := NotServerVersionTarget(brCase.MinK8SVersion, brCase.MaxK8SVersion); notVersionTarget {
				Skip(reason)
			}

			if provider == "azure" && brCase.BackupRestoreType == CSI {
				if brCase.MinK8SVersion == nil {
					brCase.MinK8SVersion = &K8sVersion{Major: "1", Minor: "23"}
				}
			}
			if notVersionTarget, reason := NotServerVersionTarget(brCase.MinK8SVersion, brCase.MaxK8SVersion); notVersionTarget {
				Skip(reason)
			}

			err := dpaCR.Build(brCase.BackupRestoreType)
			Expect(err).NotTo(HaveOccurred())

			updateLastInstallingNamespace(dpaCR.Namespace)

			err = dpaCR.CreateOrUpdate(&dpaCR.CustomResource.Spec)
			Expect(err).NotTo(HaveOccurred())

			fmt.Printf("Cluster type: %s \n", provider)

			log.Printf("Waiting for velero pod to be running")
			Eventually(AreVeleroPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())

			if brCase.BackupRestoreType == RESTIC {
				log.Printf("Waiting for restic pods to be running")
				Eventually(AreResticPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			if brCase.BackupRestoreType == CSI {
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

			// install app
			updateLastInstallingNamespace(brCase.ApplicationNamespace)
			log.Printf("Installing application for case %s", brCase.Name)
			err = InstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())
			if brCase.BackupRestoreType == CSI {
				log.Printf("Creating pvc for case %s", brCase.Name)
				pvcPath := fmt.Sprintf("./sample-applications/%s/pvc/%s.yaml", brCase.ApplicationNamespace, provider)
				err = InstallApplication(dpaCR.Client, pvcPath)
				Expect(err).ToNot(HaveOccurred())
			}

			// wait for pods to be running
			Eventually(AreAppBuildsReady(dpaCR.Client, brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*5, time.Second*5).Should(BeTrue())
			Eventually(AreApplicationPodsRunning(brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*9, time.Second*5).Should(BeTrue())

			// Run optional custom verification
			log.Printf("Running pre-backup function for case %s", brCase.Name)
			err = brCase.PreBackupVerify(dpaCR.Client, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())

			nsRequiresResticDCWorkaround, err := NamespaceRequiresResticDCWorkaround(dpaCR.Client, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())
			// create backup
			log.Printf("Creating backup %s for case %s", backupName, brCase.Name)
			backup, err := CreateBackupForNamespaces(dpaCR.Client, namespace, backupName, []string{brCase.ApplicationNamespace}, brCase.BackupRestoreType == RESTIC)
			Expect(err).ToNot(HaveOccurred())

			// wait for backup to not be running
			Eventually(IsBackupDone(dpaCR.Client, namespace, backupName), timeoutMultiplier*time.Minute*12, time.Second*10).Should(BeTrue())
			GinkgoWriter.Println(DescribeBackup(dpaCR.Client, backup))
			Expect(BackupErrorLogs(dpaCR.Client, backup)).To(Equal([]string{}))

			// check if backup succeeded
			succeeded, err := IsBackupCompletedSuccessfully(dpaCR.Client, backup)
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))
			log.Printf("Backup for case %s succeeded", brCase.Name)

			if brCase.BackupRestoreType == CSI {
				// wait for volume snapshot to be Ready
				Eventually(AreVolumeSnapshotsReady(dpaCR.Client, backupName), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
			}

			// uninstall app
			log.Printf("Uninstalling app for case %s", brCase.Name)
			err = UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())

			// Wait for namespace to be deleted
			Eventually(IsNamespaceDeleted(brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())

			updateLastInstallingNamespace(brCase.ApplicationNamespace)
			// run restore
			log.Printf("Creating restore %s for case %s", restoreName, brCase.Name)
			restore, err := CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
			Expect(err).ToNot(HaveOccurred())
			Eventually(IsRestoreDone(dpaCR.Client, namespace, restoreName), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
			GinkgoWriter.Println(DescribeRestore(dpaCR.Client, restore))
			Expect(RestoreErrorLogs(dpaCR.Client, restore)).To(Equal([]string{}))

			// Check if restore succeeded
			succeeded, err = IsRestoreCompletedSuccessfully(dpaCR.Client, namespace, restoreName)
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))

			if brCase.BackupRestoreType == RESTIC && nsRequiresResticDCWorkaround {
				// run the restic post restore script if restore type is RESTIC
				log.Printf("Running restic post restore script for case %s", brCase.Name)
				err = RunResticPostRestoreScript(restoreName)
				Expect(err).ToNot(HaveOccurred())
			}

			// verify app is running
			Eventually(AreAppBuildsReady(dpaCR.Client, brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			Eventually(AreApplicationPodsRunning(brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*9, time.Second*5).Should(BeTrue())

			// Run optional custom verification
			log.Printf("Running post-restore function for case %s", brCase.Name)
			err = brCase.PostRestoreVerify(dpaCR.Client, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())

			// Test is successful, clean up everything
			log.Printf("Uninstalling application for case %s", brCase.Name)
			err = UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())

			// Wait for namespace to be deleted
			Eventually(IsNamespaceDeleted(brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())

			if brCase.BackupRestoreType == CSI {
				log.Printf("Deleting VolumeSnapshot for CSI backuprestore of %s", brCase.Name)
				snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", provider)
				err = UninstallApplication(dpaCR.Client, snapshotClassPath)
				Expect(err).ToNot(HaveOccurred())
			}

		},
		Entry("MySQL application CSI", Label("ibmcloud", "aws", "gcp", "azure"), BackupRestoreCase{
			ApplicationTemplate:  fmt.Sprintf("./sample-applications/mysql-persistent/mysql-persistent-csi.yaml"),
			ApplicationNamespace: "mysql-persistent",
			Name:                 "mysql-csi-e2e",
			BackupRestoreType:    CSI,
			PreBackupVerify:      mysqlReady(true, CSI),
			PostRestoreVerify:    mysqlReady(false, CSI),
		}, nil),
		Entry("Mongo application CSI", Label("ibmcloud", "aws", "gcp", "azure"), BackupRestoreCase{
			ApplicationTemplate:  fmt.Sprintf("./sample-applications/mongo-persistent/mongo-persistent-csi.yaml"),
			ApplicationNamespace: "mongo-persistent",
			Name:                 "mongo-csi-e2e",
			BackupRestoreType:    CSI,
			PreBackupVerify:      mongoready(true, CSI),
			PostRestoreVerify:    mongoready(false, CSI),
		}, nil),
		Entry("Mongo application RESTIC", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			ApplicationNamespace: "mongo-persistent",
			Name:                 "mongo-restic-e2e",
			BackupRestoreType:    RESTIC,
			PreBackupVerify:      mongoready(true, RESTIC),
			PostRestoreVerify:    mongoready(false, RESTIC),
		}, nil),
		Entry("MySQL application RESTIC", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			ApplicationNamespace: "mysql-persistent",
			Name:                 "mysql-restic-e2e",
			BackupRestoreType:    RESTIC,
			PreBackupVerify:      mysqlReady(true, RESTIC),
			PostRestoreVerify:    mysqlReady(false, RESTIC),
		}, nil),
	)
})

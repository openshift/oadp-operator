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
	utils "github.com/openshift/oadp-operator/tests/e2e/utils"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VerificationFunction func(client.Client, string) error

var _ = Describe("AWS backup restore tests", func() {

	var _ = BeforeEach(func() {
		testSuiteInstanceName := "ts-" + instanceName
		dpaCR.Name = testSuiteInstanceName

		credData, err := utils.ReadFile(credFile)
		Expect(err).NotTo(HaveOccurred())
		err = CreateCredentialsSecret(credData, namespace, GetSecretRef(credSecretRef))
		Expect(err).NotTo(HaveOccurred())
	})

	var _ = AfterEach(func() {
		err := dpaCR.Delete()
		Expect(err).ToNot(HaveOccurred())

	})
	var lastInstallingApplicationNamespace string
	var lastInstallTime time.Time
	var _ = ReportAfterEach(func(report SpecReport) {
		if report.State == types.SpecStateSkipped || report.State == types.SpecStatePending {
			// do not run if the test is skipped
			return
		}
		if report.Failed() {
			// print namespace error events for app namespace
			if lastInstallingApplicationNamespace != "" {
				PrintNamespaceEventsAfterTime(lastInstallingApplicationNamespace, lastInstallTime)
			}
		}
		// remove app namespace if leftover (likely previously failed before reaching uninstall applications) to clear items such as PVCs which are immutable so that next test can create new ones
		if lastInstallingApplicationNamespace != "" {
			err := dpaCR.Client.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name:      lastInstallingApplicationNamespace,
				Namespace: lastInstallingApplicationNamespace,
			}}, &client.DeleteOptions{})
			if k8serrors.IsNotFound(err) {
				err = nil
			}
			Expect(err).ToNot(HaveOccurred())
		}
		err := dpaCR.Delete()
		if k8serrors.IsNotFound(err) {
			err = nil
		}
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

	mongoReady := VerificationFunction(func(ocClient client.Client, namespace string) error {
		Eventually(IsDCReady(ocClient, "mongo-persistent", "todolist"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		// err := VerifyBackupRestoreData(artifact_dir, namespace, "restify", "parks-app") // TODO: VERIFY PARKS APP DATA
		return nil
	})
	mysqlReady := VerificationFunction(func(ocClient client.Client, namespace string) error {
		// This test confirms that SCC restore logic in our plugin is working
		//Eventually(IsDCReady(ocClient, "mssql-persistent", "mysql"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		Eventually(IsDeploymentReady(ocClient, "mysql-persistent", "mysql"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		exists, err := DoesSCCExist(ocClient, "mysql-persistent-scc")
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("did not find MYSQL scc")
		}
		err = VerifyBackupRestoreData(artifact_dir, namespace, "todolist-route", "todolist")
		return err
	})

	updateLastInstallingNamespace := func(namespace string) {
		lastInstallingApplicationNamespace = namespace
		lastInstallTime = time.Now()
	}

	DescribeTable("backup and restore applications",
		func(brCase BackupRestoreCase, expectedErr error) {
			if notVersionTarget, reason := NotServerVersionTarget(brCase.MinK8SVersion, brCase.MaxK8SVersion); notVersionTarget {
				Skip(reason)
			}

			err := dpaCR.Build(brCase.BackupRestoreType)
			Expect(err).NotTo(HaveOccurred())

			updateLastInstallingNamespace(dpaCR.Namespace)
			err = dpaCR.CreateOrUpdate(&dpaCR.CustomResource.Spec)
			Expect(err).NotTo(HaveOccurred())

			log.Printf("Waiting for velero pod to be running")
			Eventually(AreVeleroPodsRunning(namespace), timeoutMultiplier*time.Minute*4, time.Second*5).Should(BeTrue())

			if brCase.BackupRestoreType == RESTIC {
				log.Printf("Waiting for restic pods to be running")
				Eventually(AreResticPodsRunning(namespace), timeoutMultiplier*time.Minute*4, time.Second*5).Should(BeTrue())
			}
			if brCase.BackupRestoreType == CSI {
				log.Printf("Creating VolumeSnapshotClass for CSI backuprestore of %s", brCase.Name)
				err = InstallApplication(dpaCR.Client, "./sample-applications/gp2-csi/volumeSnapshotClass.yaml")
				Expect(err).ToNot(HaveOccurred())
			}

			if dpaCR.CustomResource.BackupImages() {
				log.Printf("Waiting for registry pods to be running")
				Eventually(AreRegistryDeploymentsAvailable(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			backupUid, _ := uuid.NewUUID()
			restoreUid, _ := uuid.NewUUID()
			backupName := fmt.Sprintf("%s-%s", brCase.Name, backupUid.String())
			restoreName := fmt.Sprintf("%s-%s", brCase.Name, restoreUid.String())

			// install app
			updateLastInstallingNamespace(brCase.ApplicationNamespace)
			log.Printf("Installing application for case %s", brCase.Name)
			err = InstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())
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
			backup, err := CreateBackupForNamespaces(dpaCR.Client, namespace, backupName, []string{brCase.ApplicationNamespace})
			Expect(err).ToNot(HaveOccurred())

			// wait for backup to not be running
			Eventually(IsBackupDone(dpaCR.Client, namespace, backupName), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
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
			// Check if backup needs restic deploymentconfig workaround. https://github.com/openshift/oadp-operator/blob/master/docs/TROUBLESHOOTING.md#deployconfig
			if brCase.BackupRestoreType == RESTIC && nsRequiresResticDCWorkaround {
				log.Printf("DC found in backup namespace, using DC restic workaround")
				var dcWorkaroundResources = []string{"replicationcontroller", "deploymentconfig", "templateinstances.template.openshift.io"}
				// run restore
				log.Printf("Creating restore %s excluding DC workaround resources for case %s", restoreName, brCase.Name)
				noDcDrestoreName := fmt.Sprintf("%s-no-dc-workaround", restoreName)
				restore, err := CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, noDcDrestoreName, WithExcludedResources(dcWorkaroundResources))
				Expect(err).ToNot(HaveOccurred())
				Eventually(IsRestoreDone(dpaCR.Client, namespace, noDcDrestoreName), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
				GinkgoWriter.Println(DescribeRestore(dpaCR.Client, restore))
				Expect(RestoreErrorLogs(dpaCR.Client, restore)).To(Equal([]string{}))

				// Check if restore succeeded
				succeeded, err = IsRestoreCompletedSuccessfully(dpaCR.Client, namespace, noDcDrestoreName)
				Expect(err).ToNot(HaveOccurred())
				Expect(succeeded).To(Equal(true))
				Eventually(AreAppBuildsReady(dpaCR.Client, brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())

				// run restore
				log.Printf("Creating restore %s including DC workaround resources for case %s", restoreName, brCase.Name)
				withDcRestoreName := fmt.Sprintf("%s-with-dc-workaround", restoreName)
				restore, err = CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, withDcRestoreName, WithIncludedResources(dcWorkaroundResources))
				Expect(err).ToNot(HaveOccurred())
				Eventually(IsRestoreDone(dpaCR.Client, namespace, withDcRestoreName), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
				GinkgoWriter.Println(DescribeRestore(dpaCR.Client, restore))
				Expect(RestoreErrorLogs(dpaCR.Client, restore)).To(Equal([]string{}))

				// Check if restore succeeded
				succeeded, err = IsRestoreCompletedSuccessfully(dpaCR.Client, namespace, withDcRestoreName)
				Expect(err).ToNot(HaveOccurred())
				Expect(succeeded).To(Equal(true))

			} else {
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
				err = UninstallApplication(dpaCR.Client, "./sample-applications/gp2-csi/volumeSnapshotClass.yaml")
				Expect(err).ToNot(HaveOccurred())
			}

		},
		Entry("MySQL application CSI", Label("aws"), BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mysql-persistent/mysql-persistent-csi-template.yaml",
			ApplicationNamespace: "mysql-persistent",
			Name:                 "mysql-e2e",
			BackupRestoreType:    CSI,
			PreBackupVerify:      mysqlReady,
			PostRestoreVerify:    mysqlReady,
		}, nil),
		Entry("Mongo application", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			ApplicationNamespace: "mongo-persistent",
			Name:                 "mongo-e2e",
			BackupRestoreType:    RESTIC,
			PreBackupVerify:      mongoReady,
			PostRestoreVerify:    mongoReady,
		}, nil),
		Entry("MySQL application", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mysql-persistent/mysql-persistent-template.yaml",
			ApplicationNamespace: "mysql-persistent",
			Name:                 "mysql-e2e",
			BackupRestoreType:    RESTIC,
			PreBackupVerify:      mysqlReady,
			PostRestoreVerify:    mysqlReady,
		}, nil),
	)
})

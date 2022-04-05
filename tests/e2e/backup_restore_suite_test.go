package e2e_test

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	utils "github.com/openshift/oadp-operator/tests/e2e/utils"
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

	parksAppReady := VerificationFunction(func(ocClient client.Client, namespace string) error {
		Eventually(IsDCReady(ocClient, "parks-app", "restify"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
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
		return nil
	})

	DescribeTable("backup and restore applications",
		func(brCase BackupRestoreCase, expectedErr error) {

			err := dpaCR.Build(brCase.BackupRestoreType)
			Expect(err).NotTo(HaveOccurred())

			err = dpaCR.CreateOrUpdate(&dpaCR.CustomResource.Spec)
			Expect(err).NotTo(HaveOccurred())

			log.Printf("Waiting for velero pod to be running")
			Eventually(AreVeleroPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())

			if brCase.BackupRestoreType == RESTIC {
				log.Printf("Waiting for restic pods to be running")
				Eventually(AreResticPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			if brCase.BackupRestoreType == CSI {
				log.Printf("Creating VolumeSnapshot for CSI backuprestore of %s", brCase.Name)
				err = InstallApplication(dpaCR.Client, "./sample-applications/gp2-csi/volumeSnapshotClass.yaml")
				Expect(err).ToNot(HaveOccurred())
			}

			if dpaCR.CustomResource.Spec.BackupImages == nil || *dpaCR.CustomResource.Spec.BackupImages {
				log.Printf("Waiting for registry pods to be running")
				Eventually(AreRegistryDeploymentsAvailable(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			if notVersionTarget, reason := NotServerVersionTarget(brCase.MinK8SVersion, brCase.MaxK8SVersion); notVersionTarget {
				Skip(reason)
			}
			backupUid, _ := uuid.NewUUID()
			restoreUid, _ := uuid.NewUUID()
			backupName := fmt.Sprintf("%s-%s", brCase.Name, backupUid.String())
			restoreName := fmt.Sprintf("%s-%s", brCase.Name, restoreUid.String())

			// install app
			log.Printf("Installing application for case %s", brCase.Name)
			err = InstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())
			// wait for pods to be running
			Eventually(AreApplicationPodsRunning(brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*9, time.Second*5).Should(BeTrue())

			// Run optional custom verification
			log.Printf("Running pre-backup function for case %s", brCase.Name)
			err = brCase.PreBackupVerify(dpaCR.Client, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())

			// create backup
			log.Printf("Creating backup %s for case %s", backupName, brCase.Name)
			err = CreateBackupForNamespaces(dpaCR.Client, namespace, backupName, []string{brCase.ApplicationNamespace})
			Expect(err).ToNot(HaveOccurred())

			// wait for backup to not be running
			Eventually(IsBackupDone(dpaCR.Client, namespace, backupName), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
			Expect(GetVeleroContainerFailureLogs(dpaCR.Namespace)).To(Equal([]string{}))

			// check if backup succeeded
			succeeded, err := IsBackupCompletedSuccessfully(dpaCR.Client, namespace, backupName)
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))
			log.Printf("Backup for case %s succeeded", brCase.Name)

			// uninstall app
			log.Printf("Uninstalling app for case %s", brCase.Name)
			err = UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())

			// Wait for namespace to be deleted
			Eventually(IsNamespaceDeleted(brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())

			// run restore
			log.Printf("Creating restore %s for case %s", restoreName, brCase.Name)
			err = CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
			Expect(err).ToNot(HaveOccurred())
			Eventually(IsRestoreDone(dpaCR.Client, namespace, restoreName), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
			Expect(GetVeleroContainerFailureLogs(dpaCR.Namespace)).To(Equal([]string{}))

			// Check if restore succeeded
			succeeded, err = IsRestoreCompletedSuccessfully(dpaCR.Client, namespace, restoreName)
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))

			// verify app is running
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
		Entry("Parks application <4.8.0", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/parks-app/manifest.yaml",
			ApplicationNamespace: "parks-app",
			Name:                 "parks-e2e",
			BackupRestoreType:    RESTIC,
			PreBackupVerify:      parksAppReady,
			PostRestoreVerify:    parksAppReady,
			MaxK8SVersion:        &K8sVersionOcp47,
		}, nil),
		Entry("MySQL application", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mysql-persistent/mysql-persistent-template.yaml",
			ApplicationNamespace: "mysql-persistent",
			Name:                 "mysql-e2e",
			BackupRestoreType:    RESTIC,
			PreBackupVerify:      mysqlReady,
			PostRestoreVerify:    mysqlReady,
		}, nil),
		Entry("Parks application >=4.8.0", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/parks-app/manifest4.8.yaml",
			ApplicationNamespace: "parks-app",
			Name:                 "parks-e2e",
			BackupRestoreType:    RESTIC,
			PreBackupVerify:      parksAppReady,
			PostRestoreVerify:    parksAppReady,
			MinK8SVersion:        &K8sVersionOcp48,
		}, nil),
	)
})

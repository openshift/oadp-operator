package e2e_test

import (
	"errors"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	utils "github.com/openshift/oadp-operator/tests/e2e/utils"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VerificationFunction func(client.Client, string) error

var _ = Describe("AWS backup restore tests", func() {
	var currentBackup BackupInterface

	var _ = BeforeEach(func() {
		testSuiteInstanceName := "ts-" + instanceName

		dpaCR.Name = testSuiteInstanceName
		var err error

		credData, err := utils.ReadFile(credFile)
		Expect(err).NotTo(HaveOccurred())
		err = CreateCredentialsSecret(credData, namespace, GetSecretRef(credSecretRef))
		Expect(err).NotTo(HaveOccurred())
	})

	var _ = AfterEach(func() {
		err := dpaCR.Delete()
		Expect(err).ToNot(HaveOccurred())
		if currentBackup != nil {
			err = currentBackup.CleanBackup()
			Expect(err).NotTo(HaveOccurred())
		}

	})

	type BackupRestoreCase struct {
		ApplicationTemplate string
		BackupSpec          velero.BackupSpec
		Name                string
		PreBackupVerify     VerificationFunction
		PostRestoreVerify   VerificationFunction
		MaxK8SVersion       *K8sVersion
		MinK8SVersion       *K8sVersion
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
		func(brCase BackupRestoreCase, backup BackupInterface, expectedErr error) {

			err := dpaCR.Build(backup.GetType())
			Expect(err).NotTo(HaveOccurred())

			err = dpaCR.CreateOrUpdate(&dpaCR.CustomResource.Spec)
			Expect(err).NotTo(HaveOccurred())

			log.Printf("Waiting for velero pod to be running")
			Eventually(AreVeleroPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())

			if dpaCR.CustomResource.Spec.BackupImages == nil || *dpaCR.CustomResource.Spec.BackupImages {
				log.Printf("Waiting for registry pods to be running")
				Eventually(AreRegistryDeploymentsAvailable(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			if notVersionTarget, reason := NotServerVersionTarget(brCase.MinK8SVersion, brCase.MaxK8SVersion); notVersionTarget {
				Skip(reason)
			}

			brCaseName := brCase.Name
			backup.NewBackup(dpaCR.Client, brCaseName, &brCase.BackupSpec)
			backupRestoreName := backup.GetBackupSpec().Name
			err = backup.PrepareBackup()
			Expect(err).ToNot(HaveOccurred())
			currentBackup = backup

			// install app
			log.Printf("Installing application for case %s", brCaseName)
			err = InstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())
			// wait for pods to be running
			Eventually(AreApplicationPodsRunning(brCaseName), timeoutMultiplier*time.Minute*9, time.Second*5).Should(BeTrue())

			// Run optional custom verification
			log.Printf("Running pre-backup function for case %s", brCaseName)
			err = brCase.PreBackupVerify(dpaCR.Client, brCaseName)
			Expect(err).ToNot(HaveOccurred())

			// create backup
			log.Printf("Creating backup %s for case %s", backupRestoreName, brCaseName)
			err = backup.CreateBackup()
			Expect(err).ToNot(HaveOccurred())

			// wait for backup to not be running
			Eventually(backup.IsBackupDone(), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
			Expect(GetVeleroContainerFailureLogs(dpaCR.Namespace)).To(Equal([]string{}))

			// check if backup succeeded
			succeeded, err := backup.IsBackupCompletedSuccessfully()
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))
			log.Printf("Backup for case %s succeeded", brCaseName)

			// uninstall app
			log.Printf("Uninstalling app for case %s", brCaseName)
			err = UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())

			// Wait for namespace to be deleted
			Eventually(IsNamespaceDeleted(brCaseName), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())

			// run restore
			log.Printf("Creating restore %s for case %s", backupRestoreName, brCaseName)
			err = CreateRestoreFromBackup(dpaCR.Client, namespace, backupRestoreName, backupRestoreName)
			Expect(err).ToNot(HaveOccurred())
			Eventually(IsRestoreDone(dpaCR.Client, namespace, backupRestoreName), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
			Expect(GetVeleroContainerFailureLogs(dpaCR.Namespace)).To(Equal([]string{}))

			// Check if restore succeeded
			succeeded, err = IsRestoreCompletedSuccessfully(dpaCR.Client, namespace, backupRestoreName)
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))

			// verify app is running
			Eventually(AreApplicationPodsRunning(brCaseName), timeoutMultiplier*time.Minute*9, time.Second*5).Should(BeTrue())

			// Run optional custom verification
			log.Printf("Running post-restore function for case %s", brCaseName)
			err = brCase.PostRestoreVerify(dpaCR.Client, brCaseName)
			Expect(err).ToNot(HaveOccurred())

			// Test is successful, clean up everything
			log.Printf("Uninstalling application for case %s", brCaseName)
			err = UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())

			// Wait for namespace to be deleted
			Eventually(IsNamespaceDeleted(brCaseName), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())

		},
		Entry("MySQL application CSI", Label("aws"), BackupRestoreCase{

			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-template.yaml",
			Name:                "mysql-persistent",
			BackupSpec: velero.BackupSpec{
				IncludedNamespaces: []string{"mysql-persistent"},
			},
			PreBackupVerify:   mysqlReady,
			PostRestoreVerify: mysqlReady,
		}, &BackupCsi{
			DriverName: csi_driver,
		}, nil),
		Entry("MySQL application", BackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-template.yaml",
			Name:                "mysql-persistent",
			PreBackupVerify:     mysqlReady,
			PostRestoreVerify:   mysqlReady,
			BackupSpec: velero.BackupSpec{
				IncludedNamespaces: []string{"mysql-persistent"},
			},
		}, &BackupRestic{}, nil),
		Entry("Parks application <4.8.0", BackupRestoreCase{
			ApplicationTemplate: "./sample-applications/parks-app/manifest.yaml",
			Name:                "parks-app",
			BackupSpec: velero.BackupSpec{
				IncludedNamespaces: []string{"parks-app"},
			},
			PreBackupVerify:   parksAppReady,
			PostRestoreVerify: parksAppReady,
			MaxK8SVersion:     &K8sVersionOcp47,
		}, &BackupVsl{CreateFromDpa: false},
			nil),
		Entry("Parks application >=4.8.0", BackupRestoreCase{
			ApplicationTemplate: "./sample-applications/parks-app/manifest4.8.yaml",
			Name:                "parks-app",
			BackupSpec: velero.BackupSpec{
				IncludedNamespaces: []string{"parks-app"},
			},
			PreBackupVerify:   parksAppReady,
			PostRestoreVerify: parksAppReady,
			MinK8SVersion:     &K8sVersionOcp48,
		}, &BackupVsl{
			CreateFromDpa: true,
		}, nil),
	)
})

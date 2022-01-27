package e2e

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VerificationFunction func(client.Client, string) error

var _ = Describe("AWS backup restore tests", func() {
	var _ = BeforeEach(func() {
		testSuiteInstanceName := "ts-" + instanceName
		dpaCR.Name = testSuiteInstanceName

		credData, err := readFile(cloud)
		Expect(err).NotTo(HaveOccurred())
		err = createCredentialsSecret(credData, namespace, getSecretRef(credSecretRef))
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
		MaxK8SVersion        *k8sVersion
		MinK8SVersion        *k8sVersion
	}

	parksAppReady := VerificationFunction(func(ocClient client.Client, namespace string) error {
		Eventually(isDCReady(ocClient, "parks-app", "restify"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		return nil
	})
	mssqlReady := VerificationFunction(func(ocClient client.Client, namespace string) error {
		// This test confirms that SCC restore logic in our plugin is working
		Eventually(isDCReady(ocClient, "mssql-persistent", "mssql-deployment"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		Eventually(isDeploymentReady(ocClient, "mssql-persistent", "mssql-app-deployment"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		exists, err := doesSCCExist(ocClient, "mssql-persistent-scc")
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("did not find MSSQL scc")
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
			Eventually(areVeleroPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())

			if brCase.BackupRestoreType == restic {
				log.Printf("Waiting for restic pods to be running")
				Eventually(areResticPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			if brCase.BackupRestoreType == csi {
				if clusterProfile == "aws" {
					log.Printf("Creating VolumeSnapshot for CSI backuprestore of %s", brCase.Name)
					err = installApplication(dpaCR.Client, "./sample-applications/gp2-csi/volumeSnapshotClass.yaml")
					Expect(err).ToNot(HaveOccurred())
				} else {
					Skip("CSI testing is not provided for this cluster provider.")
				}
			}

			if dpaCR.CustomResource.Spec.BackupImages == nil || *dpaCR.CustomResource.Spec.BackupImages {
				log.Printf("Waiting for registry pods to be running")
				Eventually(areRegistryDeploymentsAvailable(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
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
			err = installApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())
			// wait for pods to be running
			Eventually(areApplicationPodsRunning(brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*9, time.Second*5).Should(BeTrue())

			// Run optional custom verification
			log.Printf("Running pre-backup function for case %s", brCase.Name)
			err = brCase.PreBackupVerify(dpaCR.Client, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())

			// create backup
			log.Printf("Creating backup %s for case %s", backupName, brCase.Name)
			err = createBackupForNamespaces(dpaCR.Client, namespace, backupName, []string{brCase.ApplicationNamespace})
			Expect(err).ToNot(HaveOccurred())

			// wait for backup to not be running
			Eventually(isBackupDone(dpaCR.Client, namespace, backupName), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
			Expect(getVeleroContainerFailureLogs(dpaCR.Namespace)).To(Equal([]string{}))

			// check if backup succeeded
			succeeded, err := isBackupCompletedSuccessfully(dpaCR.Client, namespace, backupName)
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))
			log.Printf("Backup for case %s succeeded", brCase.Name)

			// uninstall app
			log.Printf("Uninstalling app for case %s", brCase.Name)
			err = uninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())

			// Wait for namespace to be deleted
			Eventually(isNamespaceDeleted(brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())

			// run restore
			log.Printf("Creating restore %s for case %s", restoreName, brCase.Name)
			err = createRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
			Expect(err).ToNot(HaveOccurred())
			Eventually(isRestoreDone(dpaCR.Client, namespace, restoreName), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
			Expect(getVeleroContainerFailureLogs(dpaCR.Namespace)).To(Equal([]string{}))

			// Check if restore succeeded
			succeeded, err = isRestoreCompletedSuccessfully(dpaCR.Client, namespace, restoreName)
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))

			// verify app is running
			Eventually(areApplicationPodsRunning(brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*9, time.Second*5).Should(BeTrue())

			// Run optional custom verification
			log.Printf("Running post-restore function for case %s", brCase.Name)
			err = brCase.PostRestoreVerify(dpaCR.Client, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())

			// Test is successful, clean up everything
			log.Printf("Uninstalling application for case %s", brCase.Name)
			err = uninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())

			// Wait for namespace to be deleted
			Eventually(isNamespaceDeleted(brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())

			if brCase.BackupRestoreType == csi {
				log.Printf("Deleting VolumeSnapshot for CSI backuprestore of %s", brCase.Name)
				err = uninstallApplication(dpaCR.Client, "./sample-applications/gp2-csi/volumeSnapshotClass.yaml")
				Expect(err).ToNot(HaveOccurred())
			}

		},
		Entry("MSSQL application CSI", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mssql-persistent/mssql-persistent-csi-template.yaml",
			ApplicationNamespace: "mssql-persistent",
			Name:                 "mssql-e2e",
			BackupRestoreType:    csi,
			PreBackupVerify:      mssqlReady,
			PostRestoreVerify:    mssqlReady,
		}, nil),
		Entry("Parks application <4.8.0", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/parks-app/manifest.yaml",
			ApplicationNamespace: "parks-app",
			Name:                 "parks-e2e",
			BackupRestoreType:    restic,
			PreBackupVerify:      parksAppReady,
			PostRestoreVerify:    parksAppReady,
			MaxK8SVersion:        &k8sVersionOcp47,
		}, nil),
		Entry("MSSQL application", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mssql-persistent/mssql-persistent-template.yaml",
			ApplicationNamespace: "mssql-persistent",
			Name:                 "mssql-e2e",
			BackupRestoreType:    restic,
			PreBackupVerify:      mssqlReady,
			PostRestoreVerify:    mssqlReady,
		}, nil),
		Entry("Parks application >=4.8.0", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/parks-app/manifest4.8.yaml",
			ApplicationNamespace: "parks-app",
			Name:                 "parks-e2e",
			BackupRestoreType:    restic,
			PreBackupVerify:      parksAppReady,
			PostRestoreVerify:    parksAppReady,
			MinK8SVersion:        &k8sVersionOcp48,
		}, nil),
	)
})

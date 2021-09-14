package e2e

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var _ = Describe("AWS backup restore tests", func() {
	var _ = BeforeEach(func() {
		testSuiteInstanceName := "ts-" + instanceName
		vel.Name = testSuiteInstanceName

		credData, err := getCredsData(cloud)
		Expect(err).NotTo(HaveOccurred())

		err = createCredentialsSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())

		err = vel.Build()
		Expect(err).NotTo(HaveOccurred())

		err = vel.CreateOrUpdate(&vel.CustomResource.Spec)
		Expect(err).NotTo(HaveOccurred())
	})

	var _ = AfterEach(func() {
		err := vel.Delete()
		Expect(err).ToNot(HaveOccurred())

	})

	type VerificationFunction func(client.Client, string) error

	type BackupRestoreCase struct {
		ApplicationTemplate  string
		ApplicationNamespace string
		Name                 string
		PreBackupVerify      VerificationFunction
		PostRestoreVerify    VerificationFunction
	}

	DescribeTable("backup and restore applications",
		func(brCase BackupRestoreCase, expectedErr error) {
			backupUid, _ := uuid.NewUUID()
			restoreUid, _ := uuid.NewUUID()
			backupName := fmt.Sprintf("%s-%s", brCase.Name, backupUid.String())
			restoreName := fmt.Sprintf("%s-%s", brCase.Name, restoreUid.String())

			// install app
			log.Printf("Installing application for case %s", brCase.Name)
			err := installApplication(vel.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())
			// wait for pods to be running
			Eventually(areApplicationPodsRunning(brCase.ApplicationNamespace), time.Minute*2, time.Second*5).Should(BeTrue())

			// Run optional custom verification
			log.Printf("Running pre-backup function for case %s", brCase.Name)
			err = brCase.PreBackupVerify(vel.Client, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())

			// create backup
			log.Printf("Creating backup %s for case %s", backupName, brCase.Name)
			err = createBackupForNamespaces(vel.Client, namespace, backupName, []string{brCase.ApplicationNamespace})
			Expect(err).ToNot(HaveOccurred())

			// wait for backup to not be running
			Eventually(isBackupDone(vel.Client, namespace, backupName), time.Minute*4, time.Second*10).Should(BeTrue())

			// check if backup succeeded
			succeeded, err := isBackupCompletedSuccessfully(vel.Client, namespace, backupName)
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))
			log.Printf("Backup for case %s succeeded", brCase.Name)

			// uninstall app
			log.Printf("Uninstalling app for case %s", brCase.Name)
			err = uninstallApplication(vel.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())

			// Wait for namespace to be deleted
			Eventually(isNamespaceDeleted(brCase.ApplicationNamespace), time.Minute*2, time.Second*5).Should(BeTrue())

			// run restore
			log.Printf("Creating restore %s for case %s", restoreName, brCase.Name)
			err = createRestoreFromBackup(vel.Client, namespace, backupName, restoreName)
			Expect(err).ToNot(HaveOccurred())
			Eventually(isRestoreDone(vel.Client, namespace, restoreName), time.Minute*4, time.Second*10).Should(BeTrue())

			// Check if restore succeeded
			succeeded, err = isRestoreCompletedSuccessfully(vel.Client, namespace, restoreName)
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))

			// verify app is running
			Eventually(areApplicationPodsRunning(brCase.ApplicationNamespace), time.Minute*2, time.Second*5).Should(BeTrue())

			// Run optional custom verification
			log.Printf("Running post-restore function for case %s", brCase.Name)
			err = brCase.PostRestoreVerify(vel.Client, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())

			// Test is successful, clean up everything
			log.Printf("Uninstalling application for case %s", brCase.Name)
			err = uninstallApplication(vel.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())
		},
		Entry("MSSQL application", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mssql-persistent/mssql-persistent-template.yaml",
			ApplicationNamespace: "mssql-persistent",
			Name:                 "mssql-e2e",
			PreBackupVerify: VerificationFunction(func(ocClient client.Client, namespace string) error {
				return nil
			}),
			PostRestoreVerify: VerificationFunction(func(ocClient client.Client, namespace string) error {
				// This test confirms that SCC restore logic in our plugin is working
				exists, err := doesSCCExist(ocClient, "mssql-persistent-scc")
				if err != nil {
					return err
				}
				if !exists {
					return errors.New("did not find MSSQL scc after restore")
				}
				return nil
			}),
		}, nil),
		Entry("Parks application", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/parks-app/manifest.yaml",
			ApplicationNamespace: "parks-app",
			Name:                 "parks-e2e",
			PreBackupVerify: VerificationFunction(func(ocClient client.Client, namespace string) error {
				Eventually(isDCReady(ocClient, "parks-app", "restify"), time.Minute*5, time.Second*10).Should(BeTrue())
				return nil
			}),
			PostRestoreVerify: VerificationFunction(func(ocClient client.Client, namespace string) error {
				return nil
			}),
		}, nil),
	)
})

package e2e

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"
)

var _ = FDescribe("AWS backup tests", func() {
	var _ = BeforeEach(func() {
		testSuiteInstanceName := "ts-" + instanceName
		vel.Name = testSuiteInstanceName

		credData, err := getCredsData(cloud)
		Expect(err).NotTo(HaveOccurred())

		err = createCredentialsSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())

		err = vel.Build()
		Expect(err).NotTo(HaveOccurred())

		err = vel.Create()
		Expect(err).NotTo(HaveOccurred())
	})

	var _ = AfterEach(func() {
		err := vel.Delete()
		Expect(err).ToNot(HaveOccurred())

		errs := deleteSecret(namespace, credSecretRef)
		Expect(errs).ToNot(HaveOccurred())
	})

	Context("When we install MSSQL and take a backup", func() {
		It("Should succeed in `Completed` state", func() {
			// Install app
			err := installApplication(vel.Client, "./sample-applications/mssql-persistent/mssql-persistent-template.yaml")
			Expect(err).ToNot(HaveOccurred())
			// Wait for pods to be running
			Eventually(areApplicationPodsRunning("mssql-persistent"), time.Minute*2, time.Second*5).Should(BeTrue())

			backupName := "mssql-e2e-backup"
			// Create backup
			err = createBackupForNamespaces(vel.Client, backupName, []string{"mssql-persistent"})
			Expect(err).ToNot(HaveOccurred())

			// Wait for backup to not be running
			Eventually(isBackupDone(vel.Client, backupName), time.Minute*4, time.Second*10).Should(BeTrue())

			// Check if backup succeeded
			succeeded, err := isBackupCompletedSuccessfully(vel.Client, backupName)
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))

			// Uninstall app
			err = uninstallApplication(vel.Client, "./sample-applications/mssql-persistent/mssql-persistent-template.yaml")
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

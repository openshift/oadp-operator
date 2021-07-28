package e2e

import (
	"flag"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var testSuiteInstanceName string
var resticName string

var _ = Describe("The Velero Restic spec", func() {
	var _ = BeforeEach(func() {
		flag.Parse()
		s3Buffer, err := getJsonData(s3BucketFilePath)
		Expect(err).NotTo(HaveOccurred())
		s3Data, err := decodeJson(s3Buffer) // Might need to change this later on to create s3 for each tests
		Expect(err).NotTo(HaveOccurred())
		s3Bucket = s3Data["velero-bucket-name"].(string)

		testSuiteInstanceName = "rs-" + instanceName

		resticName = "restic"

		credData, err := getCredsData(cloud)
		Expect(err).NotTo(HaveOccurred())

		err = createCredentialsSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())
	})

	var _ = AfterEach(func() {
		testSuiteInstanceName := "rs-" + instanceName
		err := uninstallVelero(namespace, testSuiteInstanceName)
		Expect(err).ToNot(HaveOccurred())

		errs := deleteSecret(namespace, credSecretRef)
		Expect(errs).ToNot(HaveOccurred())
	})

	Context("When the value of 'enable_restic' is changed to false", func() {
		It("Should delete the Restic daemonset", func() {
			// Check that OADP operator is installed in test namespace
			err := installDefaultVelero(namespace, s3Bucket, credSecretRef, testSuiteInstanceName)
			Expect(err).ToNot(HaveOccurred())

			// wait for daemonSet to initialize
			Eventually(doesDaemonSetExists(namespace, "restic"), time.Minute*2, time.Second*5).Should(BeTrue())

			err = disableRestic(namespace, testSuiteInstanceName)
			Expect(err).ToNot(HaveOccurred())

			Eventually(isResticDaemonsetDeleted(namespace, testSuiteInstanceName, resticName), time.Minute*2, time.Second*5).Should(BeTrue())
		})
	})

	Context("When 'restic_node_selector' is added to the Velero CR spec", func() {
		It("Should update the Restic daemonSet to include a nodeSelector", func() {
			err := enableResticNodeSelector(namespace, s3Bucket, credSecretRef, testSuiteInstanceName)
			Expect(err).ToNot(HaveOccurred())
			Eventually(resticDaemonSetHasNodeSelector(namespace, s3Bucket, credSecretRef, testSuiteInstanceName, resticName), time.Minute*1, time.Second*5).Should(BeTrue())
		})
	})
})

package e2e

import (
	"flag"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var testSuiteInstanceName string

var _ = Describe("The Velero Restic spec", func() {
	var _ = BeforeEach(func() {
		flag.Parse()
		s3Buffer, err := getJsonData(s3BucketFilePath)
		Expect(err).NotTo(HaveOccurred())
		s3Data, err := decodeJson(s3Buffer) // Might need to change this later on to create s3 for each tests
		Expect(err).NotTo(HaveOccurred())
		s3Bucket = s3Data["velero-bucket-name"].(string)
		testSuiteInstanceName := "rs-" + instanceName
		credData, err := getCredsData(cloud)
		Expect(err).NotTo(HaveOccurred())
		err = createSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())
		// Check that OADP operator is installed in test namespace
		err = installDefaultVelero(namespace, s3Bucket, credSecretRef, testSuiteInstanceName)
		Expect(err).ToNot(HaveOccurred())
	})

	var _ = AfterEach(func() {
		testSuiteInstanceName := "rs-" + instanceName
		err := uninstallVelero(namespace, testSuiteInstanceName)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("When the value of 'enable_restic' is changed to false", func() {
		It("Should delete the Restic daemonset", func() {
			err := waitForDeletedRestic(namespace, testSuiteInstanceName, "restic")
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

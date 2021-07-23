package e2e

import (
	"flag"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var testSuiteInstanceName string

var _ = Describe("The Velero Restic spec", func() {
	var _ = BeforeEach(func() {
		flag.Parse()
		s3Data := decodeJson(getJsonData(s3BucketFilePath)) // Might need to change this later on to create s3 for each tests
		s3Bucket = s3Data["velero-bucket-name"].(string)
		testSuiteInstanceName = "rs-" + instanceName

		err := createOADPTestNamespace(namespace)
		Expect(err).NotTo(HaveOccurred())
		credData := getCredsData(cloud)
		err = createSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())
		// Check that OADP operator is installed in test namespace
		err = installDefaultVelero(namespace, s3Bucket, credSecretRef, testSuiteInstanceName)
		Expect(err).ToNot(HaveOccurred())
	})

	var _ = AfterEach(func() {
		testSuiteInstanceName = "rs-" + instanceName
		err := uninstallVelero(namespace, testSuiteInstanceName)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("When 'restic_node_selector' is added to the Velero CR spec", func() {
		It("Should update the Restic daemonSet to include a nodeSelector", func() {
			fmt.Printf("Hello")
		})
	})

	// Context("When the value of 'enable_restic' is changed to false", func() {
	// 	It("Should delete the Restic daemonset", func() {
	// 		err := waitForDeletedRestic(namespace, testSuiteInstanceName, "restic")
	// 		Expect(err).NotTo(HaveOccurred())
	// 	})
	// })
})

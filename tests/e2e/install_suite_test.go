package e2e

import (
	"flag"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// Check Velero is deleted
// TODO: Check test namespace is deleted
// TODO: Check secret is deleted
// })

var _ = BeforeSuite(func() {
	Expect(isNamespaceExists(namespace)).Should(BeNil())
})

var _ = Describe("The default Velero custom resource", func() {
	var _ = BeforeEach(func() {
		flag.Parse()
		s3Buffer, err := getJsonData(s3BucketFilePath)
		Expect(err).NotTo(HaveOccurred())
		s3Data, err := decodeJson(s3Buffer) // Might need to change this later on to create s3 for each tests
		Expect(err).NotTo(HaveOccurred())
		s3Bucket = s3Data["velero-bucket-name"].(string)
		testSuiteInstanceName := "ts-" + instanceName
		credData, err := getCredsData(cloud)
		Expect(err).NotTo(HaveOccurred())
		err = createSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())
		// Check that OADP operator is installed in test namespace
		err = installDefaultVelero(namespace, s3Bucket, credSecretRef, testSuiteInstanceName)
		Expect(err).ToNot(HaveOccurred())
	})

	var _ = AfterEach(func() {
		testSuiteInstanceName := "ts-" + instanceName
		err := uninstallVelero(namespace, testSuiteInstanceName)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("When the default valid Velero CR is created", func() {
		It("Should create a Velero pod in the cluster", func() {
			Eventually(isVeleroPodRunning(namespace), time.Minute*2, time.Second*5).Should(BeTrue())
		})
		It("Should create a Restic daemonset in the cluster", func() {
			Eventually(areResticPodsRunning(namespace), time.Minute*2, time.Second*5).Should(BeTrue())
		})
	})
})

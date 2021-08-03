package e2e

import (
	"flag"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = BeforeSuite(func() {
	Expect(doesNamespaceExists(namespace)).Should(BeTrue())
})

var _ = AfterSuite(func() {
	// Check Velero is deleted
	Eventually(isVeleroDeleted(namespace, testSuiteInstanceName), time.Minute*2, time.Second*5).Should(BeTrue())

	// Check Restic daemonSet is deleted
	Eventually(isResticDaemonsetDeleted(namespace, testSuiteInstanceName, resticName), time.Minute*2, time.Second*5).Should(BeTrue())

	// Check secret is deleted
	Eventually(isCredentialsSecretDeleted(namespace, credSecretRef), time.Minute*2, time.Second*5).Should(BeTrue())

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

		err = createCredentialsSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())

		// Check that OADP operator is installed in test namespace
		err = installDefaultVelero(namespace, s3Bucket, credSecretRef, testSuiteInstanceName)
		Expect(err).ToNot(HaveOccurred())
	})

	var _ = AfterEach(func() {
		testSuiteInstanceName := "ts-" + instanceName
		err := uninstallVelero(namespace, testSuiteInstanceName)
		Expect(err).ToNot(HaveOccurred())

		errs := deleteSecret(namespace, credSecretRef)
		Expect(errs).ToNot(HaveOccurred())
	})

	Context("When the default valid Velero CR is created", func() {
		It("Should create a Velero pod in the cluster", func() {
			Eventually(isVeleroPodRunning(namespace), time.Minute*2, time.Second*5).Should(BeTrue())
		})
		It("Should create a Restic daemonset in the cluster", func() {
			Eventually(areResticPodsRunning(namespace), time.Minute*2, time.Second*5).Should(BeTrue())
		})
		It("Should install the aws plugin", func() {
			Eventually(doesPluginExist(namespace, "velero", "velero-plugin-for-aws"), time.Minute*2, time.Second*5).Should(BeTrue())
		})
		It("Should install the openshift plugin", func() {
			Eventually(doesPluginExist(namespace, "velero", "openshift-velero-plugin"), time.Minute*2, time.Second*5).Should(BeTrue())
		})
		It("Should install the csi plugin", func() {
			Eventually(doesPluginExist(namespace, "velero", "velero-plugin-for-csi"), time.Minute*2, time.Second*5).Should(BeTrue())
		})
	})
})

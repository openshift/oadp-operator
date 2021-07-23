package e2e

import (
	"flag"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// var _ = BeforeSuite(func() {
// 	prefix := "ts-" // To create individual instance per spec
// 	flag.Parse()
// 	s3Data := decodeJson(getJsonData(s3BuckerFilePath)) // Might need to change this later on to create s3 for each tests
// 	s3Bucket = s3Data["velero-bucket-name"].(string)
// 	credData := getCredsData(cloud)
// 	err := createSecret(credData, namespace, credSecretRef)
// 	Expect(err).NotTo(HaveOccurred())

// 	namespace = prefix + namespace
// 	s3Bucket = prefix + s3Bucket
// 	credSecretRef = prefix + credSecretRef
// 	instanceName = prefix + instanceName
// 	err = createOADPTestNamespace(namespace)
// 	Expect(err).NotTo(HaveOccurred())
// 	// Check that OADP operator is installed in test namespace
// })

// var _ = AfterSuite(func() {
// 	err := deleteSecret(namespace, credSecretRef)
// 	Expect(err).NotTo(HaveOccurred())
// 	err = deleteOADPTestNamespace(namespace)
// 	Expect(err).NotTo(HaveOccurred())
// })

// Check Velero is deleted
// TODO: Check test namespace is deleted
// TODO: Check secret is deleted
// })

var _ = Describe("The default Velero custom resource", func() {
	var _ = BeforeEach(func() {
		prefix := "ts-" // To create individual instance per spec
		flag.Parse()
		s3Data := decodeJson(getJsonData(s3BuckerFilePath)) // Might need to change this later on to create s3 for each tests
		s3Bucket = s3Data["velero-bucket-name"].(string)
		namespace = prefix + namespace
		s3Bucket = prefix + s3Bucket
		credSecretRef = prefix + credSecretRef
		instanceName = prefix + instanceName

		err := createOADPTestNamespace(namespace)
		Expect(err).NotTo(HaveOccurred())
		credData := getCredsData(cloud)
		err = createSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())
		// Check that OADP operator is installed in test namespace
		err = installDefaultVelero(namespace, s3Bucket, credSecretRef, instanceName)
		Expect(err).ToNot(HaveOccurred())
	})

	var _ = AfterEach(func() {
		err := deleteOADPTestNamespace(namespace)
		Expect(err).NotTo(HaveOccurred())
	})

	// Context("When the default valid Velero CR is created, but no credential secret is present", func() {
	// 	It("Should print an error to Velero CR status", func() {
	// 		err := waitForFailedVeleroCR()
	// 		Expect(err).NotTo(HaveOccurred())
	// 	})
	// })

	Context("When the default valid Velero CR is created", func() {
		It("Should create a Velero pod in the cluster", func() {
			veleroResult := waitForVeleroPodRunning(namespace)
			Expect(veleroResult).To(BeNil())
		})
		It("Should create a Restic daemonset in the cluster", func() {
			resticResult := waitForResticPods(namespace)
			Expect(resticResult).To(BeNil())
		})
	})
})

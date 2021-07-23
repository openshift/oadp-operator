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

var _ = Describe("The Velero Restic spec", func() {
	BeforeEach(func() {
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
	})

	AfterEach(func() {
		err := deleteOADPTestNamespace(namespace)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("When the value of 'enable_restic' is changed to false", func() {
		It("Should delete the Restic daemonset", func() {
			errs := installDefaultVelero(namespace, s3Bucket, credSecretRef, instanceName)
			Expect(errs).NotTo(HaveOccurred())

			err := waitForDeletedRestic(namespace, instanceName, "restic")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// Context("When 'restic_node_selector' is added to the Velero CR spec", func() {
	// 	It("Should update the Restic daemonSet to include a nodeSelector", func() {
	// 		err := waitForResticNodeSelector()
	// 		Expect(err).NotTo(HaveOccurred())
	// 	})
	// })
})

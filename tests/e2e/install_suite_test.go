package e2e

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// var _ = BeforeSuite(func() {
// 	namespace := "oadp-operator"
// 	s3Bucket := "myBucket"
// 	credSecretRef := "cloud-credentials"
// 	instanceName := "example-velero"

// 	credData := getCredsData()
// 	err := createSecret(credData)
// 	Expect(err).NotTo(HaveOccurred())

// 	if flag.Lookup("velero_namespace") != nil {
// 		namespace = flag.Lookup("kubeconfig").Value.(flag.Getter).Get().(string)
// 	}
// 	if flag.Lookup("s3_bucket") != nil {
// 		s3Bucket = flag.Lookup("s3_bucket").Value.(flag.Getter).Get().(string)
// 	}
// 	if flag.Lookup("creds_secret_ref") != nil {
// 		credSecretRef = flag.Lookup("creds_secret_ref").Value.(flag.Getter).Get().(string)
// 	}
// 	if flag.Lookup("velero_instance_name") != nil {
// 		instanceName = flag.Lookup("velero_instance_name").Value.(flag.Getter).Get().(string)
// 	}

// 	err = installDefaultVelero(namespace, s3Bucket, credSecretRef, instanceName)
// 	Expect(err).ToNot(HaveOccurred())

// 	//err := createOADPTestNamespace()
// 	//Expect(err).NotTo(HaveOccurred())
// 	// Check that OADP operator is installed in test namespace
// })

// var _ = AfterSuite(func() {
// 	err := uninstallVelero(namespace, instanceName)
// 	Expect(err).ToNot(HaveOccurred())
// })

// Check Velero is deleted
// TODO: Check test namespace is deleted
// TODO: Check secret is deleted
// })

var _ = Describe("The default Velero custom resource", func() {
	var _ = BeforeEach(func() {
		prefix := "ts-"

		credData := getCredsData()
		err := createSecret(credData)
		Expect(err).NotTo(HaveOccurred())

		namespace = prefix + namespace
		s3Bucket = prefix + s3Bucket
		credSecretRef = prefix + credSecretRef
		instanceName = prefix + instanceName

		err = installDefaultVelero(namespace, s3Bucket, credSecretRef, instanceName)
		Expect(err).ToNot(HaveOccurred())

	})

	var _ = AfterEach(func() {
		err := uninstallVelero(namespace, instanceName)
		Expect(err).ToNot(HaveOccurred())
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

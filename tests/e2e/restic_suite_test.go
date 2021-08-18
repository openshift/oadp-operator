package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var testSuiteInstanceName string
var resticName string

var _ = Describe("The Velero Restic spec", func() {
	var _ = BeforeEach(func() {
		testSuiteInstanceName = "rs-" + instanceName
		resticName = "restic"

		credData, err := getCredsData(cloud)
		Expect(err).NotTo(HaveOccurred())

		err = createCredentialsSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())
	})

	var _ = AfterEach(func() {
		err := vel.Delete()
		Expect(err).ToNot(HaveOccurred())

		errs := deleteSecret(namespace, credSecretRef)
		Expect(errs).ToNot(HaveOccurred())
	})

	Context("When the value of 'enable_restic' is changed to false", func() {
		It("Should delete the Restic daemonset", func() {
			err := vel.Create()
			Expect(err).ToNot(HaveOccurred())

			// wait for daemonSet to initialize
			Eventually(doesDaemonSetExists(namespace, resticName), time.Minute*2, time.Second*5).Should(BeTrue())

			err = disableRestic(namespace, testSuiteInstanceName)
			Expect(err).ToNot(HaveOccurred())

			// wait for daemonSet to update
			Eventually(isResticDaemonsetDeleted(namespace, testSuiteInstanceName, resticName), time.Minute*2, time.Second*5).Should(BeTrue())
		})
	})

	// Context("When 'restic_node_selector' is added to the Velero CR spec", func() {
	// 	It("Should update the Restic daemonSet to include a nodeSelector", func() {

	// 		// also installs Velero CR
	// 		err := enableResticNodeSelector(namespace, s3Bucket, credSecretRef, testSuiteInstanceName)
	// 		Expect(err).ToNot(HaveOccurred())

	// 		// wait for daemonSet to initialize
	// 		Eventually(doesDaemonSetExists(namespace, resticName), time.Minute*2, time.Second*5).Should(BeTrue())

	// 		// wait for daemonSet to update
	// 		Eventually(resticDaemonSetHasNodeSelector(namespace, s3Bucket, credSecretRef, testSuiteInstanceName, resticName), time.Minute*1, time.Second*5).Should(BeTrue())
	// 	})
	// })
})

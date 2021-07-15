package e2e

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// var _ = BeforeSuite(func() {
// 	err := installDefaultVelero()
// 	Expect(err).ToNot(HaveOccurred())

// 	//err := createOADPTestNamespace()
// 	//Expect(err).NotTo(HaveOccurred())
// 	// Check that OADP operator is installed in test namespace
// })

var _ = AfterSuite(func() {
	err := uninstallVelero()
	Expect(err).ToNot(HaveOccurred())
})

// Check Velero is deleted
// TODO: Check test namespace is deleted
// TODO: Check secret is deleted
//})

var _ = Describe("The default Velero custom resource", func() {
	var _ = BeforeEach(func() {
		err := installDefaultVelero()
		Expect(err).NotTo(HaveOccurred())
	})

	var _ = AfterEach(func() {
		err := uninstallVelero()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("When the default valid Velero CR is created, but no credential secret is present", func() {
		It("Should print an error to Velero CR status", func() {
			err := waitForFailedVeleroCR()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When the default valid Velero CR is created", func() {
		It("Should create a Velero pod and Restic daemonset in the cluster", func() {
			veleroResult := waitForVeleroPodRunning()
			Expect(veleroResult).To(BeNil())
		})
		It("Should create running restic pods", func() {
			resticResult := waitForResticPods()
			Expect(resticResult).To(BeNil())
		})
	})
})

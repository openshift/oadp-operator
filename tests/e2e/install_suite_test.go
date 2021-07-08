package e2e

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = BeforeSuite(func() {
	err := createOADPTestNamespace()
	Expect(err).NotTo(HaveOccurred())
	// Check that OADP operator is installed in test namespace
})

var _ = Describe("Creating Default Velero Custom Resource", func() {
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
			result := waitForVeleroPodRunning()
			Expect(result).To(BeNil())
			resticPodsResult := waitForResticPods()
			Expect(resticPodsResult).To(BeNil())
		})
	})
})

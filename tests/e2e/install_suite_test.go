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

var _ = Describe("Creating Velero Custom Resource", func() {
	Context("When the default valid Velero CR is created, but no credential secret is present", func() {
		It("Should print an error to Velero CR status", func() {
			err := installVelero()
			Expect(err).NotTo(HaveOccurred())
			result := waitForPodRunning()
			Expect(result).To(BeNil())
		})
	})
	Context("When the default valid Velero CR is created", func() {
		It("Should create a Velero pod in the cluster", func() {
			err := installVelero()
			Expect(err).NotTo(HaveOccurred())
			result := waitForPodRunning()
			Expect(result).To(BeNil())
		})
	})
})

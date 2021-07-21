package e2e

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// var _ = BeforeSuite(func() {
// 	err := installDefaultVelero()
// 	Expect(err).ToNot(HaveOccurred())

// })

// var _ = AfterSuite(func() {
// 	err := uninstallVelero()
// 	Expect(err).ToNot(HaveOccurred())
// })

var _ = Describe("The Velero Restic spec", func() {

	var _ = AfterEach(func() {
		err := uninstallVelero()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("When the value of 'enable_restic' is changed to false", func() {
		It("Should delete the Restic daemonset", func() {
			errs := installDefaultVelero()
			Expect(errs).NotTo(HaveOccurred())

			err := waitForDeletedRestic()
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

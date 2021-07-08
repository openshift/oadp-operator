package e2e

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Installing Velero", func() {
	Context("When the default Velero CR is created", func() {
		It("Should create a Velero pod in the cluster", func() {
			result := waitForPodRunning()
			Expect(result).To(BeNil())
		})
	})
})

package main

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestVeleroE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Velero E2E Suite")
}

var _ = BeforeSuite(func() {
	installVelero()
})

var _ = Describe("Installing Velero", func() {
	Context("When the default Velero CR is created", func() {
		It("Should create a Velero pod in the cluster", func() {
			result := waitForPodRunning()
			Expect(result).To(BeNil())
		})
	})
})

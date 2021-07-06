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

var _ = Describe("Installing Velero", func() {
	Context("When the default Velero CR is created", func() {
		It("Should create a Velero pod in the cluster", func() {
			//result := getPodStatus()
			//expected := "Running"
			//Expect(expected).To(Equal(result))
		})
	})
})

// to run test, run command 'ginkgo'
// var _ = Describe("Adding two nums", func() {
// 	Context("When  3 is added to 6", func() {
// 		It("Should return 9", func() {
// 			result := add_nums(3, 6)
// 			expected := 9
// 			Expect(expected).To(Equal(result))
// 		})
// 	})
// })

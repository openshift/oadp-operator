package e2e

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAdd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Add Nums Suite")
}

// to run test, run command 'ginkgo'
var _ = Describe("Adding two nums", func() {
	Context("When  3 is added to 6", func() {
		It("Should return 9", func() {
			result := add_nums(3, 6)
			expected := 9
			Expect(expected).To(Equal(result))
		})
	})
})

package e2e_test

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// This test is intentionally designed to fail for testing Claude failure analysis
// It should be removed after verifying the Claude analysis script works correctly
var _ = ginkgo.Describe("Claude Analysis Test Failure", ginkgo.Label("aws"), func() {
	ginkgo.It("should fail to test Claude analysis (TEMPORARY TEST)", func() {
		// This test intentionally fails to verify Claude analysis is working
		gomega.Expect(true).To(gomega.BeFalse(), "This is an intentional failure to test Claude analysis script")
	})
})

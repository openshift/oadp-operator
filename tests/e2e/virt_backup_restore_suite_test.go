package e2e_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
)

var _ = Describe("VM backup and restore tests", Ordered, func() {
	var v *VirtOperator
	var err error

	var _ = BeforeAll(func() {
		Expect(runTimeClientForSuiteRun).ToNot(BeNil())
		Expect(kubernetesClientForSuiteRun).ToNot(BeNil())
		v, err = GetVirtOperator(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, "down")
		Expect(err).To(BeNil())
		Expect(v).ToNot(BeNil())

		err = v.EnsureVirtInstallation(5 * time.Minute)
		Expect(err).To(BeNil())
	})

	It("should verify virt installation", func() {
		installed := v.IsVirtInstalled()
		Expect(installed).To(BeTrue())
	})
})

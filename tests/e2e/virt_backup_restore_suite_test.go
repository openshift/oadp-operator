package e2e_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	"k8s.io/apimachinery/pkg/util/version"
)

var _ = Describe("VM backup and restore tests", Ordered, func() {
	var v *VirtOperator
	var err error
	wasInstalledFromTest := false

	var _ = BeforeAll(func() {
		if !virtTestingEnabled {
			Skip("Virtualization testing is disabled, skipping tests")
		}

		v, err = GetVirtOperator(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, dynamicClientForSuiteRun)
		Expect(err).To(BeNil())
		Expect(v).ToNot(BeNil())

		minimum, err := version.ParseGeneric("4.13")
		Expect(err).To(BeNil())
		Expect(v.Version).ToNot(BeNil())
		if !v.Version.AtLeast(minimum) {
			Skip("Skipping virtualization testing on cluster version " + v.Version.String() + ", minimum required is " + minimum.String())
		}

		err = v.EnsureVirtInstallation(5 * time.Minute)
		Expect(err).To(BeNil())
		wasInstalledFromTest = true
	})

	var _ = AfterAll(func() {
		if !virtTestingEnabled {
			Skip("Virtualization testing is disabled, skipping test cleanup")
		}

		if v != nil && wasInstalledFromTest {
			err := v.EnsureVirtRemoval(5 * time.Minute)
			Expect(err).To(BeNil())
		}
	})

	It("should verify virt installation", func() {
		installed := v.IsVirtInstalled()
		Expect(installed).To(BeTrue())
	})
})

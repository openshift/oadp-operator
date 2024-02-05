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

		if !v.IsVirtInstalled() {
			err = v.EnsureVirtInstallation(5 * time.Minute)
			Expect(err).To(BeNil())
			wasInstalledFromTest = true
		}
	})

	var _ = AfterAll(func() {
		if !virtTestingEnabled {
			Skip("Virtualization testing is disabled, skipping test cleanup")
		}

		if v != nil && wasInstalledFromTest {
			v.EnsureVirtRemoval(6 * time.Minute)
		}
	})

	It("should verify virt installation", func() {
		installed := v.IsVirtInstalled()
		Expect(installed).To(BeTrue())
	})

	It("should upload a data volume successfully", func() {
		err := v.EnsureDataVolume("openshift-cnv", "cirros", "https://download.cirros-cloud.net/0.6.2/cirros-0.6.2-x86_64-disk.img", "128Mi", 5*time.Minute)
		Expect(err).To(BeNil())
		v.EnsureDataVolumeRemoval("openshift-cnv", "cirros", 2*time.Minute)
	})
})

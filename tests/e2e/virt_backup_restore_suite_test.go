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
	wasInstalledFromTest := false

	var _ = BeforeAll(func() {
		dpaCR.CustomResource.Name = "dummyDPA"
		dpaCR.CustomResource.Namespace = "openshift-adp"

		v, err = GetVirtOperator(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, dynamicClientForSuiteRun)
		Expect(err).To(BeNil())
		Expect(v).ToNot(BeNil())

		if !v.IsVirtInstalled() {
			err = v.EnsureVirtInstallation()
			Expect(err).To(BeNil())
			wasInstalledFromTest = true
		}

		err = v.EnsureDataVolume("openshift-cnv", "cirros-dv", "https://download.cirros-cloud.net/0.6.2/cirros-0.6.2-x86_64-disk.img", "128Mi", 5*time.Minute)
		Expect(err).To(BeNil())
	})

	var _ = AfterAll(func() {
		if v != nil && wasInstalledFromTest {
			v.EnsureVirtRemoval()
			v.EnsureDataVolumeRemoval("openshift-cnv", "cirros-dv", 2*time.Minute)
		}
	})

	It("should verify virt installation", Label("virt"), func() {
		installed := v.IsVirtInstalled()
		Expect(installed).To(BeTrue())
	})

	It("should create and boot a virtual machine", Label("virt"), func() {
		err := v.CreateVM("openshift-cnv", "cirros-vm", "cirros-dv")
		Expect(err).To(BeNil())
	})
})

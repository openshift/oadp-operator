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

		err = v.EnsureEmulation(10 * time.Second)
		Expect(err).To(BeNil())

		err = v.EnsureDataVolume("openshift-cnv", "cirros-dv", "https://download.cirros-cloud.net/0.6.2/cirros-0.6.2-x86_64-disk.img", "128Mi", 5*time.Minute)
		Expect(err).To(BeNil())
	})

	var _ = AfterAll(func() {
		if v != nil && wasInstalledFromTest {
			v.EnsureVirtRemoval()
		}
	})

	It("should verify virt installation", Label("virt"), func() {
		installed := v.IsVirtInstalled()
		Expect(installed).To(BeTrue())
	})

	It("should create and boot a virtual machine", Label("virt"), func() {
		namespace := "openshift-cnv"
		source := "cirros-dv"
		name := "cirros-vm"

		err := v.CloneDisk(namespace, source, name, 5*time.Minute)
		Expect(err).To(BeNil())

		err = v.CreateVm(namespace, name, source, 5*time.Minute)
		Expect(err).To(BeNil())

		v.RemoveVm(namespace, name, 2*time.Minute)
		v.RemoveDataVolume(namespace, name, 2*time.Minute)
	})
})

package e2e_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
)

func getLatestCirrosImageURL() (string, error) {
	cirrosVersionURL := "https://download.cirros-cloud.net/version/released"

	resp, err := http.Get(cirrosVersionURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	latestCirrosVersion := strings.TrimSpace(string(body))

	imageURL := fmt.Sprintf("https://download.cirros-cloud.net/%s/cirros-%s-x86_64-disk.img", latestCirrosVersion, latestCirrosVersion)

	return imageURL, nil
}

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

		url, err := getLatestCirrosImageURL()
		Expect(err).To(BeNil())
		err = v.EnsureDataVolumeFromUrl("openshift-cnv", "cirros-dv", url, "128Mi", 5*time.Minute)
		Expect(err).To(BeNil())
	})

	var _ = AfterAll(func() {
		v.RemoveDataVolume("openshift-cnv", "cirros-dv", 2*time.Minute)

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

package e2e_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	ginkgov2 "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
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

var _ = ginkgov2.Describe("VM backup and restore tests", ginkgov2.Ordered, func() {
	var v *lib.VirtOperator
	var err error
	wasInstalledFromTest := false

	var _ = ginkgov2.BeforeAll(func() {
		dpaCR.CustomResource.Name = "dummyDPA"
		dpaCR.CustomResource.Namespace = "openshift-adp"

		v, err = lib.GetVirtOperator(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, dynamicClientForSuiteRun)
		gomega.Expect(err).To(gomega.BeNil())
		gomega.Expect(v).ToNot(gomega.BeNil())

		if !v.IsVirtInstalled() {
			err = v.EnsureVirtInstallation()
			gomega.Expect(err).To(gomega.BeNil())
			wasInstalledFromTest = true
		}

		err = v.EnsureEmulation(10 * time.Second)
		gomega.Expect(err).To(gomega.BeNil())

		url, err := getLatestCirrosImageURL()
		gomega.Expect(err).To(gomega.BeNil())
		err = v.EnsureDataVolumeFromUrl("openshift-cnv", "cirros-dv", url, "128Mi", 5*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())
	})

	var _ = ginkgov2.AfterAll(func() {
		v.RemoveDataVolume("openshift-cnv", "cirros-dv", 2*time.Minute)

		if v != nil && wasInstalledFromTest {
			v.EnsureVirtRemoval()
		}
	})

	ginkgov2.It("should verify virt installation", ginkgov2.Label("virt"), func() {
		installed := v.IsVirtInstalled()
		gomega.Expect(installed).To(gomega.BeTrue())
	})

	ginkgov2.It("should create and boot a virtual machine", ginkgov2.Label("virt"), func() {
		namespace := "openshift-cnv"
		source := "cirros-dv"
		name := "cirros-vm"

		err := v.CloneDisk(namespace, source, name, 5*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())

		err = v.CreateVm(namespace, name, source, 5*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())

		v.RemoveVm(namespace, name, 2*time.Minute)
		v.RemoveDataVolume(namespace, name, 2*time.Minute)
	})
})

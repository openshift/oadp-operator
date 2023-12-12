package e2e_test

import (
	"log"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
)

// TODO: check HCO
func IsVirtInstalled() (bool, error) {
	return DoesNamespaceExist(kubernetesClientForSuiteRun, "openshift-cnv")
}

func ensureVirtInstallation() {
	log.Println("Checking virt operator status...")
	exists, err := IsVirtInstalled()
	Expect(err).To(BeNil())

	if !exists {
		log.Println("Installing virt operator...")
		command := exec.Command("bash", "scripts/virt_install.sh")
		output, err := command.CombinedOutput()
		log.Printf("Installation script output:\n%s", output)
		Expect(err).To(BeNil())
		Expect(command.ProcessState.ExitCode()).To(Equal(0))

		exists, err = DoesNamespaceExist(kubernetesClientForSuiteRun, "openshift-cnv")
		Expect(err).To(BeNil())
		Expect(exists).To(BeTrue())
	} else {
		log.Println("Virt operator already installed, continuing with tests...")
	}
}

var _ = Describe("VM backup and restore tests", Ordered, func() {
	var _ = BeforeAll(func() {
		ensureVirtInstallation()
	})
	It("should verify virt installation", func() {
		installed, err := IsVirtInstalled()
		Expect(err).To(BeNil())
		Expect(installed).To(BeTrue())
	})
})

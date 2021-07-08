package e2e

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestOADPE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OADP E2E Suite")
}

var _ = BeforeSuite(func() {
	installVelero()
})

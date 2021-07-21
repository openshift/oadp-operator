package e2e

import (
	"flag"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var cloud string

func init() {
	flag.StringVar(&cloud, "cloud", "", "Credentials file path location")
}

func TestOADPE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OADP E2E Suite")
}

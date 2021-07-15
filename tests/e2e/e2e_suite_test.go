package e2e

import (
<<<<<<< HEAD
	"flag"
=======
>>>>>>> 5b7b367fde5a09a9d62b421ea671bf50d28ecd34
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

<<<<<<< HEAD
var cloud string

func init() {
	flag.StringVar(&cloud, "cloud", "", "Credentials file path location")
}

=======
>>>>>>> 5b7b367fde5a09a9d62b421ea671bf50d28ecd34
func TestOADPE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OADP E2E Suite")
}

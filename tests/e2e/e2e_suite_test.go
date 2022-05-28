package e2e_test

import (
	"flag"
	"log"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	i "github.com/openshift/oadp-operator/tests/e2e/lib/init"
	"github.com/openshift/oadp-operator/tests/e2e/utils"
)

func TestOADPE2E(t *testing.T) {
	flag.Parse()
	errString := LoadDpaSettingsFromJson(i.GetSettings())
	if errString != "" {
		t.Fatalf(errString)
	}
	dpaCR = &DpaCustomResource{
		Name:      i.GetTestSuiteInstanceName(),
		Namespace:     i.GetNamespace(),
		Credentials:   i.GetCredfile(), // secret data for BSL
		CredSecretRef: i.GetCredsecretref(),
		Provider:      i.GetProvider(),
	}
	log.Println("Using velero prefix: " + GetVeleroPrefix())
	RegisterFailHandler(Fail)
	RunSpecs(t, "OADP E2E Suite")
}

var dpaCR *DpaCustomResource

var _ = BeforeSuite(func() {
	dpaCopy := GetDpa()
	dpaCR.CustomResource = &dpaCopy

	cloudCredData, err := utils.ReadFile(dpaCR.Credentials) // get secret data for BSL
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(utils.ReplaceSecretDataNewLineWithCarriageReturn(cloudCredData), i.GetNamespace(), i.GetBslSecretName()) // secret with data for BSL
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(utils.ReplaceSecretDataNewLineWithCarriageReturn(cloudCredData), i.GetNamespace(), "credential-with-carriage-return") // secret with data for BSL with carriage return
	Expect(err).NotTo(HaveOccurred())
	
	vslCredData, err := utils.ReadFile(i.GetCi_Cred_File()) // get secret data for vsl
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(vslCredData, i.GetNamespace(), i.GetCredsecretref()) // secret with data for vsl with name oadp expects https://github.com/openshift/oadp-operator/blob/b64e96e4432d266fe5a7680aa5406e160cded824/pkg/credentials/credentials.go#L34
	Expect(err).NotTo(HaveOccurred())
	dpaCR.SetClient()
	Expect(DoesNamespaceExist(i.GetNamespace())).Should(BeTrue())
})

var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		log.Printf("Running must gather for failed test - " + report.LeafNodeText)
		err := RunMustGather(i.GetOc_Cli(), i.GetArtifact_Dir()+"/must-gather-"+report.LeafNodeText)
		if err != nil {
			log.Printf("Failed to run must gather: " + err.Error())
		}
	}
})

var _ = AfterSuite(func() {
	log.Printf("Deleting Velero CR")
	errs := DeleteSecret(i.GetNamespace(), i.GetCredsecretref())
	Expect(errs).ToNot(HaveOccurred())
	errs = DeleteSecret(i.GetNamespace(), i.GetBslSecretName())
	Expect(errs).ToNot(HaveOccurred())
	errs = DeleteSecret(i.GetNamespace(), "credential-with-carriage-return")
	Expect(errs).ToNot(HaveOccurred())
	err := dpaCR.Delete()
	Expect(err).ToNot(HaveOccurred())
	Eventually(dpaCR.IsDeleted(), i.GetTimeoutMultiplier()*time.Minute*2, time.Second*5).Should(BeTrue())
})

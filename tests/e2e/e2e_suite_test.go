package e2e_test

import (
	"errors"
	"flag"
	"log"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	"github.com/openshift/oadp-operator/tests/e2e/utils"
)

// Common vars obtained from flags passed in ginkgo.
var credFile, namespace, credSecretRef, instanceName, provider, ci_cred_file, settings, artifact_dir, oc_cli string
var timeoutMultiplier time.Duration

func init() {
	flag.StringVar(&credFile, "credentials", "", "Cloud Credentials file path location")
	flag.StringVar(&namespace, "velero_namespace", "velero", "Velero Namespace")
	flag.StringVar(&settings, "settings", "./templates/default_settings.json", "Settings of the velero instance")
	flag.StringVar(&instanceName, "velero_instance_name", "example-velero", "Velero Instance Name")
	flag.StringVar(&credSecretRef, "creds_secret_ref", "cloud-credentials", "Credential secret ref for backup storage location")
	flag.StringVar(&provider, "provider", "aws", "Cloud provider")
	flag.StringVar(&ci_cred_file, "ci_cred_file", credFile, "CI Cloud Cred File")
	flag.StringVar(&artifact_dir, "artifact_dir", "/tmp", "Directory for storing must gather")
	flag.StringVar(&oc_cli, "oc_cli", "oc", "OC CLI Client")

	timeoutMultiplierInput := flag.Int64("timeout_multiplier", 1, "Customize timeout multiplier from default (1)")
	timeoutMultiplier = 1
	if timeoutMultiplierInput != nil && *timeoutMultiplierInput >= 1 {
		timeoutMultiplier = time.Duration(*timeoutMultiplierInput)
	}
}

func TestOADPE2E(t *testing.T) {
	flag.Parse()
	errString := LoadDpaSettingsFromJson(settings)
	if errString != "" {
		t.Fatalf(errString)
	}

	log.Println("Using velero prefix: " + VeleroPrefix)
	RegisterFailHandler(Fail)
	RunSpecs(t, "OADP E2E Suite")
}

var dpaCR *DpaCustomResource

var _ = BeforeSuite(func() {
	flag.Parse()
	errString := LoadDpaSettingsFromJson(settings)
	if errString != "" {
		Expect(errors.New(errString)).NotTo(HaveOccurred())
	}

	dpaCR = &DpaCustomResource{
		Namespace:     namespace,
		Credentials:   credFile,
		CredSecretRef: credSecretRef,
		Provider:      provider,
	}
	dpaCR.CustomResource = Dpa
	testSuiteInstanceName := "ts-" + instanceName
	dpaCR.Name = testSuiteInstanceName

	cloudCredData, err := utils.ReadFile(dpaCR.Credentials)
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(cloudCredData, namespace, "bsl-cloud-credentials-"+provider)
	Expect(err).NotTo(HaveOccurred())
	dpaCR.Credentials = ci_cred_file

	credData, err := utils.ReadFile(dpaCR.Credentials)
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(credData, namespace, credSecretRef)
	Expect(err).NotTo(HaveOccurred())
	dpaCR.SetClient()
	Expect(DoesNamespaceExist(namespace)).Should(BeTrue())
})

var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		log.Printf("Running must gather for failed test - " + report.LeafNodeText)
		err := RunMustGather(oc_cli, artifact_dir+"/must-gather-"+report.LeafNodeText)
		if err != nil {
			log.Printf("Failed to run must gather: " + err.Error())
		}
	}
})

var _ = AfterSuite(func() {
	log.Printf("Deleting Velero CR")
	errs := DeleteSecret(namespace, GetSecretRef(credSecretRef))
	Expect(errs).ToNot(HaveOccurred())
	err := dpaCR.Delete()
	Expect(err).ToNot(HaveOccurred())
	Eventually(dpaCR.IsDeleted(), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())
})

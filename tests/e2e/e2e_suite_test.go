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
	utils "github.com/openshift/oadp-operator/tests/e2e/utils"
)

// Common vars obtained from flags passed in ginkgo.
var namespace, instanceName, settings, cloud, clusterProfile, credSecretRef string
var timeoutMultiplier time.Duration

func init() {
	flag.StringVar(&cloud, "cloud", "", "Cloud Credentials file path location")
	flag.StringVar(&namespace, "velero_namespace", "velero", "Velero Namespace")
	flag.StringVar(&settings, "settings", "./templates/default_settings.json", "Settings of the velero instance")
	flag.StringVar(&instanceName, "velero_instance_name", "example-velero", "Velero Instance Name")
	flag.StringVar(&clusterProfile, "cluster_profile", "aws", "Cluster profile")
	flag.StringVar(&credSecretRef, "creds_secret_ref", "cloud-credentials", "Credential secret ref for backup storage location")

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

	credData, err := utils.ReadFile(cloud)
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(credData, namespace, GetSecretRef(credSecretRef))
	Expect(err).NotTo(HaveOccurred())

	dpaCR = &DpaCustomResource{
		Namespace: namespace,
	}
	dpaCR.CustomResource = Dpa
	testSuiteInstanceName := "ts-" + instanceName
	dpaCR.Name = testSuiteInstanceName

	dpaCR.SetClient()
	Expect(DoesNamespaceExist(namespace)).Should(BeTrue())
})

var _ = AfterSuite(func() {
	log.Printf("Deleting Velero CR")
	errs := DeleteSecret(namespace, GetSecretRef(credSecretRef))
	Expect(errs).ToNot(HaveOccurred())
	err := dpaCR.Delete()
	Expect(err).ToNot(HaveOccurred())
	Eventually(dpaCR.IsDeleted(), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())
})

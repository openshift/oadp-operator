package e2e_test

import (
	"errors"
	"flag"
	"log"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	"github.com/openshift/oadp-operator/tests/e2e/utils"
)

// Common vars obtained from flags passed in ginkgo.
var bslCredFile, namespace, credSecretRef, instanceName, provider, vslCredFile, settings, artifact_dir, oc_cli string
var timeoutMultiplier time.Duration

func init() {
	flag.StringVar(&bslCredFile, "credentials", "", "Credentials path for BackupStorageLocation")
	flag.StringVar(&namespace, "velero_namespace", "openshift-adp", "Velero Namespace")
	flag.StringVar(&settings, "settings", "./templates/default_settings.json", "Settings of the velero instance")
	flag.StringVar(&instanceName, "velero_instance_name", "example-velero", "Velero Instance Name")
	flag.StringVar(&credSecretRef, "creds_secret_ref", "cloud-credentials", "Credential secret ref (name) for volume storage location")
	flag.StringVar(&provider, "provider", "aws", "Cloud provider")
	// TODO: change flag in makefile to --vsl-credentials
	flag.StringVar(&vslCredFile, "ci_cred_file", bslCredFile, "Credentials path for for VolumeSnapshotLocation, this credential would have access to cluster volume snapshots (for CI this is not OADP owned credential)")
	flag.StringVar(&artifact_dir, "artifact_dir", "/tmp", "Directory for storing must gather")
	flag.StringVar(&oc_cli, "oc_cli", "oc", "OC CLI Client")

	// helps with launching debug sessions from IDE
	if os.Getenv("E2E_USE_ENV_FLAGS") == "true" {
		if os.Getenv("CLOUD_CREDENTIALS") != "" {
			bslCredFile = os.Getenv("CLOUD_CREDENTIALS")
		}
		if os.Getenv("VELERO_NAMESPACE") != "" {
			namespace = os.Getenv("VELERO_NAMESPACE")
		}
		if os.Getenv("SETTINGS") != "" {
			settings = os.Getenv("SETTINGS")
		}
		if os.Getenv("VELERO_INSTANCE_NAME") != "" {
			instanceName = os.Getenv("VELERO_INSTANCE_NAME")
		}
		if os.Getenv("CREDS_SECRET_REF") != "" {
			credSecretRef = os.Getenv("CREDS_SECRET_REF")
		}
		if os.Getenv("PROVIDER") != "" {
			provider = os.Getenv("PROVIDER")
		}
		if os.Getenv("CI_CRED_FILE") != "" {
			vslCredFile = os.Getenv("CI_CRED_FILE")
		} else {
			vslCredFile = bslCredFile
		}
		if os.Getenv("ARTIFACT_DIR") != "" {
			artifact_dir = os.Getenv("ARTIFACT_DIR")
		}
		if os.Getenv("OC_CLI") != "" {
			oc_cli = os.Getenv("OC_CLI")
		}
	}

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
		Namespace: namespace,
		Provider:  provider,
	}
	dpaCR.CustomResource = Dpa
	testSuiteInstanceName := "ts-" + instanceName
	dpaCR.Name = testSuiteInstanceName

	bslCredFileData, err := utils.ReadFile(bslCredFile)
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(bslCredFileData, namespace, "bsl-cloud-credentials-"+provider)
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(utils.ReplaceSecretDataNewLineWithCarriageReturn(bslCredFileData), namespace, "bsl-cloud-credentials-"+provider+"-with-carriage-return")
	Expect(err).NotTo(HaveOccurred())

	vslCredFileData, err := utils.ReadFile(vslCredFile)
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(vslCredFileData, namespace, credSecretRef)
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
	err := DeleteSecret(namespace, credSecretRef)
	Expect(err).ToNot(HaveOccurred())
	err = DeleteSecret(namespace, "bsl-cloud-credentials-"+provider)
	Expect(err).ToNot(HaveOccurred())
	err = DeleteSecret(namespace, "bsl-cloud-credentials-"+provider+"-with-carriage-return")
	Expect(err).ToNot(HaveOccurred())
	err = dpaCR.Delete()
	Expect(err).ToNot(HaveOccurred())
	Eventually(dpaCR.IsDeleted(), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())
})

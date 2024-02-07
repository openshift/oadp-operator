package e2e_test

import (
	"errors"
	"flag"
	"log"
	"os"
	"strconv"
	"testing"
	"time"

	snapshotv1client "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	"github.com/openshift/oadp-operator/tests/e2e/utils"
	veleroClientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// Common vars obtained from flags passed in ginkgo.
var bslCredFile, namespace, credSecretRef, instanceName, provider, vslCredFile, settings, artifact_dir, oc_cli, stream string
var timeoutMultiplierInput, flakeAttempts int64
var timeoutMultiplier time.Duration

func init() {
	// TODO better descriptions to flags
	flag.StringVar(&bslCredFile, "credentials", "", "Credentials path for BackupStorageLocation")
	flag.StringVar(&namespace, "velero_namespace", "velero", "Velero Namespace")
	flag.StringVar(&settings, "settings", "./templates/default_settings.json", "Settings of the velero instance")
	flag.StringVar(&instanceName, "velero_instance_name", "example-velero", "Velero Instance Name")
	flag.StringVar(&credSecretRef, "creds_secret_ref", "cloud-credentials", "Credential secret ref (name) for volume storage location")
	flag.StringVar(&provider, "provider", "aws", "Cloud provider")
	// TODO: change flag in makefile to --vsl-credentials
	flag.StringVar(&vslCredFile, "ci_cred_file", bslCredFile, "Credentials path for for VolumeSnapshotLocation, this credential would have access to cluster volume snapshots (for CI this is not OADP owned credential)")
	flag.StringVar(&artifact_dir, "artifact_dir", "/tmp", "Directory for storing must gather")
	flag.StringVar(&oc_cli, "oc_cli", "oc", "OC CLI Client")
	flag.StringVar(&stream, "stream", "up", "[up, down] upstream or downstream")
	flag.Int64Var(&timeoutMultiplierInput, "timeout_multiplier", 1, "Customize timeout multiplier from default (1)")
	timeoutMultiplier = time.Duration(timeoutMultiplierInput)
	flag.Int64Var(&flakeAttempts, "flakeAttempts", 3, "Customize the number of flake retries (3)")

	// helps with launching debug sessions from IDE
	if os.Getenv("E2E_USE_ENV_FLAGS") == "true" {
		if os.Getenv("CLOUD_CREDENTIALS") != "" {
			bslCredFile = os.Getenv("CLOUD_CREDENTIALS")
		}
		if os.Getenv("VELERO_NAMESPACE") != "" {
			namespace = os.Getenv("VELERO_NAMESPACE")
		}
		if os.Getenv("OADP_STREAM") != "" {
			stream = os.Getenv("OADP_STREAM")
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
		if envValue := os.Getenv("FLAKE_ATTEMPTS"); envValue != "" {
			// Parse the environment variable as int64
			parsedValue, err := strconv.ParseInt(envValue, 10, 64)
			if err != nil {
				log.Println("Error parsing FLAKE_ATTEMPTS, default flake number will be used:", err)
			} else {
				flakeAttempts = parsedValue
			}
		}
	}
}

func TestOADPE2E(t *testing.T) {
	flag.Parse()
	errString := LoadDpaSettingsFromJson(settings)
	if errString != "" {
		t.Fatalf(errString)
	}

	RegisterFailHandler(Fail)
	RunSpecs(t, "OADP E2E using velero prefix: "+VeleroPrefix)
}

var kubernetesClientForSuiteRun *kubernetes.Clientset
var runTimeClientForSuiteRun client.Client
var veleroClientForSuiteRun veleroClientset.Interface
var csiClientForSuiteRun *snapshotv1client.Clientset
var dynamicClientForSuiteRun dynamic.Interface
var dpaCR *DpaCustomResource
var knownFlake bool
var accumulatedTestLogs []string

var _ = BeforeSuite(func() {
	// TODO create logger (hh:mm:ss message) to be used by all functions
	flag.Parse()
	errString := LoadDpaSettingsFromJson(settings)
	if errString != "" {
		Expect(errors.New(errString)).NotTo(HaveOccurred())
	}

	var err error
	kubeConf := config.GetConfigOrDie()
	kubeConf.QPS = 50
	kubeConf.Burst = 100

	kubernetesClientForSuiteRun, err = kubernetes.NewForConfig(kubeConf)
	Expect(err).NotTo(HaveOccurred())

	runTimeClientForSuiteRun, err = client.New(kubeConf, client.Options{})
	Expect(err).NotTo(HaveOccurred())

	veleroClientForSuiteRun, err = veleroClientset.NewForConfig(kubeConf)
	Expect(err).NotTo(HaveOccurred())

	csiClientForSuiteRun, err = snapshotv1client.NewForConfig(kubeConf)
	Expect(err).NotTo(HaveOccurred())

	dynamicClientForSuiteRun, err = dynamic.NewForConfig(kubeConf)
	Expect(err).NotTo(HaveOccurred())

	dpaCR = &DpaCustomResource{
		Namespace: namespace,
		Provider:  provider,
	}
	dpaCR.CustomResource = Dpa
	dpaCR.Name = "ts-" + instanceName

	bslCredFileData, err := utils.ReadFile(bslCredFile)
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(kubernetesClientForSuiteRun, bslCredFileData, namespace, "bsl-cloud-credentials-"+provider)
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(
		kubernetesClientForSuiteRun,
		utils.ReplaceSecretDataNewLineWithCarriageReturn(bslCredFileData),
		namespace, "bsl-cloud-credentials-"+provider+"-with-carriage-return",
	)
	Expect(err).NotTo(HaveOccurred())

	vslCredFileData, err := utils.ReadFile(vslCredFile)
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(kubernetesClientForSuiteRun, vslCredFileData, namespace, credSecretRef)
	Expect(err).NotTo(HaveOccurred())
	dpaCR.SetClient(runTimeClientForSuiteRun)
	Expect(DoesNamespaceExist(kubernetesClientForSuiteRun, namespace)).Should(BeTrue())
})

var _ = AfterSuite(func() {
	log.Printf("Deleting Velero CR")
	err := DeleteSecret(kubernetesClientForSuiteRun, namespace, credSecretRef)
	Expect(err).ToNot(HaveOccurred())
	err = DeleteSecret(kubernetesClientForSuiteRun, namespace, "bsl-cloud-credentials-"+provider)
	Expect(err).ToNot(HaveOccurred())
	err = DeleteSecret(kubernetesClientForSuiteRun, namespace, "bsl-cloud-credentials-"+provider+"-with-carriage-return")
	Expect(err).ToNot(HaveOccurred())
	err = dpaCR.Delete(runTimeClientForSuiteRun)
	Expect(err).ToNot(HaveOccurred())
	Eventually(dpaCR.IsDeleted(runTimeClientForSuiteRun), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())
})

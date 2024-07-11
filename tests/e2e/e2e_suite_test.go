package e2e_test

import (
	"flag"
	"log"
	"os"
	"strconv"
	"testing"
	"time"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	openshiftappsv1 "github.com/openshift/api/apps/v1"
	openshiftbuildv1 "github.com/openshift/api/build/v1"
	openshiftsecurityv1 "github.com/openshift/api/security/v1"
	openshifttemplatev1 "github.com/openshift/api/template/v1"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	"github.com/openshift/oadp-operator/tests/e2e/utils"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroClientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

var (
	// Common vars obtained from flags passed in ginkgo.
	bslCredFile, namespace, instanceName, provider, vslCredFile, settings, artifact_dir, oc_cli, stream string
	flakeAttempts                                                                                       int64

	kubernetesClientForSuiteRun *kubernetes.Clientset
	runTimeClientForSuiteRun    client.Client
	veleroClientForSuiteRun     veleroClientset.Interface
	dynamicClientForSuiteRun    dynamic.Interface

	dpaCR                           *DpaCustomResource
	bslSecretName                   string
	bslSecretNameWithCarriageReturn string
	vslSecretName                   string

	knownFlake          bool
	accumulatedTestLogs []string
)

func init() {
	// TODO better descriptions to flags
	flag.StringVar(&bslCredFile, "credentials", "", "Credentials path for BackupStorageLocation")
	// TODO: change flag in makefile to --vsl-credentials
	flag.StringVar(&vslCredFile, "ci_cred_file", bslCredFile, "Credentials path for for VolumeSnapshotLocation, this credential would have access to cluster volume snapshots (for CI this is not OADP owned credential)")
	flag.StringVar(&namespace, "velero_namespace", "velero", "Velero Namespace")
	flag.StringVar(&settings, "settings", "./templates/default_settings.json", "Settings of the velero instance")
	flag.StringVar(&instanceName, "velero_instance_name", "example-velero", "Velero Instance Name")
	flag.StringVar(&provider, "provider", "aws", "Cloud provider")
	flag.StringVar(&artifact_dir, "artifact_dir", "/tmp", "Directory for storing must gather")
	flag.StringVar(&oc_cli, "oc_cli", "oc", "OC CLI Client")
	flag.StringVar(&stream, "stream", "up", "[up, down] upstream or downstream")
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

	var err error
	kubeConf := config.GetConfigOrDie()
	kubeConf.QPS = 50
	kubeConf.Burst = 100

	RegisterFailHandler(Fail)

	kubernetesClientForSuiteRun, err = kubernetes.NewForConfig(kubeConf)
	Expect(err).NotTo(HaveOccurred())

	runTimeClientForSuiteRun, err = client.New(kubeConf, client.Options{})
	Expect(err).NotTo(HaveOccurred())

	oadpv1alpha1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	velerov1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	openshiftappsv1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	openshiftbuildv1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	openshiftsecurityv1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	openshifttemplatev1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	corev1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	volumesnapshotv1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	operatorsv1alpha1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	operatorsv1.AddToScheme(runTimeClientForSuiteRun.Scheme())

	veleroClientForSuiteRun, err = veleroClientset.NewForConfig(kubeConf)
	Expect(err).NotTo(HaveOccurred())

	dynamicClientForSuiteRun, err = dynamic.NewForConfig(kubeConf)
	Expect(err).NotTo(HaveOccurred())

	err = CreateNamespace(kubernetesClientForSuiteRun, namespace)
	Expect(err).To(BeNil())
	Expect(DoesNamespaceExist(kubernetesClientForSuiteRun, namespace)).Should(BeTrue())

	dpa, err := LoadDpaSettingsFromJson(settings)
	Expect(err).NotTo(HaveOccurred())

	bslSecretName = "bsl-cloud-credentials-" + provider
	bslSecretNameWithCarriageReturn = "bsl-cloud-credentials-" + provider + "-with-carriage-return"
	vslSecretName = "vsl-cloud-credentials-" + provider

	veleroPrefix := "velero-e2e-" + string(uuid.NewUUID())

	dpaCR = &DpaCustomResource{
		Name:                 "ts-" + instanceName,
		Namespace:            namespace,
		Provider:             provider,
		Client:               runTimeClientForSuiteRun,
		BSLSecretName:        bslSecretName,
		BSLConfig:            dpa.DeepCopy().Spec.BackupLocations[0].Velero.Config,
		BSLProvider:          dpa.DeepCopy().Spec.BackupLocations[0].Velero.Provider,
		BSLBucket:            dpa.DeepCopy().Spec.BackupLocations[0].Velero.ObjectStorage.Bucket,
		BSLBucketPrefix:      veleroPrefix,
		VeleroDefaultPlugins: dpa.DeepCopy().Spec.Configuration.Velero.DefaultPlugins,
		SnapshotLocations:    dpa.DeepCopy().Spec.SnapshotLocations,
	}

	RunSpecs(t, "OADP E2E using velero prefix: "+veleroPrefix)
}

var _ = BeforeSuite(func() {
	// TODO create logger (hh:mm:ss message) to be used by all functions
	log.Printf("Creating Secrets")
	bslCredFileData, err := utils.ReadFile(bslCredFile)
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(kubernetesClientForSuiteRun, bslCredFileData, namespace, bslSecretName)
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(
		kubernetesClientForSuiteRun,
		utils.ReplaceSecretDataNewLineWithCarriageReturn(bslCredFileData),
		namespace, bslSecretNameWithCarriageReturn,
	)
	Expect(err).NotTo(HaveOccurred())

	vslCredFileData, err := utils.ReadFile(vslCredFile)
	Expect(err).NotTo(HaveOccurred())
	err = CreateCredentialsSecret(kubernetesClientForSuiteRun, vslCredFileData, namespace, vslSecretName)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	log.Printf("Deleting Secrets")
	err := DeleteSecret(kubernetesClientForSuiteRun, namespace, vslSecretName)
	Expect(err).ToNot(HaveOccurred())
	err = DeleteSecret(kubernetesClientForSuiteRun, namespace, bslSecretName)
	Expect(err).ToNot(HaveOccurred())
	err = DeleteSecret(kubernetesClientForSuiteRun, namespace, bslSecretNameWithCarriageReturn)
	Expect(err).ToNot(HaveOccurred())

	log.Printf("Deleting DPA")
	err = dpaCR.Delete()
	Expect(err).ToNot(HaveOccurred())
	Eventually(dpaCR.IsDeleted(), time.Minute*2, time.Second*5).Should(BeTrue())
})

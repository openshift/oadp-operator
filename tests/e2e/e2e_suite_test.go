package e2e_test

import (
	"flag"
	"log"
	"os"
	"strconv"
	"testing"
	"time"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	openshiftappsv1 "github.com/openshift/api/apps/v1"
	openshiftbuildv1 "github.com/openshift/api/build/v1"
	openshiftroutev1 "github.com/openshift/api/route/v1"
	openshiftsecurityv1 "github.com/openshift/api/security/v1"
	openshifttemplatev1 "github.com/openshift/api/template/v1"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

var (
	// Common vars obtained from flags passed in ginkgo.
	bslCredFile, namespace, instanceName, provider, vslCredFile, settings, artifact_dir, oc_cli string
	flakeAttempts                                                                               int64

	kubernetesClientForSuiteRun *kubernetes.Clientset
	runTimeClientForSuiteRun    client.Client
	dynamicClientForSuiteRun    dynamic.Interface

	dpaCR                           *lib.DpaCustomResource
	bslSecretName                   string
	bslSecretNameWithCarriageReturn string
	vslSecretName                   string

	kubeConfig          *rest.Config
	knownFlake          bool
	accumulatedTestLogs []string

	kvmEmulation   bool
	useUpstreamHco bool
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
	flag.Int64Var(&flakeAttempts, "flakeAttempts", 3, "Customize the number of flake retries (3)")
	flag.BoolVar(&kvmEmulation, "kvm_emulation", true, "Enable or disable KVM emulation for virtualization testing")
	flag.BoolVar(&useUpstreamHco, "hco_upstream", false, "Force use of upstream virtualization operator")

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
		if envValue := os.Getenv("KVM_EMULATION"); envValue != "" {
			if parsedValue, err := strconv.ParseBool(envValue); err == nil {
				kvmEmulation = parsedValue
			} else {
				log.Println("Error parsing KVM_EMULATION, it will be enabled by default: ", err)
			}
		}
		if envValue := os.Getenv("HCO_UPSTREAM"); envValue != "" {
			if parsedValue, err := strconv.ParseBool(envValue); err == nil {
				useUpstreamHco = parsedValue
			} else {
				log.Println("Error parsing HCO_UPSTREAM, it will be disabled by default: ", err)
			}
		}
	}

}

func TestOADPE2E(t *testing.T) {
	flag.Parse()

	var err error
	kubeConfig = config.GetConfigOrDie()
	kubeConfig.QPS = 50
	kubeConfig.Burst = 100

	gomega.RegisterFailHandler(ginkgo.Fail)

	kubernetesClientForSuiteRun, err = kubernetes.NewForConfig(kubeConfig)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	runTimeClientForSuiteRun, err = client.New(kubeConfig, client.Options{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	oadpv1alpha1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	velerov1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	openshiftappsv1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	openshiftbuildv1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	openshiftsecurityv1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	openshifttemplatev1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	openshiftroutev1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	corev1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	volumesnapshotv1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	operatorsv1alpha1.AddToScheme(runTimeClientForSuiteRun.Scheme())
	operatorsv1.AddToScheme(runTimeClientForSuiteRun.Scheme())

	dynamicClientForSuiteRun, err = dynamic.NewForConfig(kubeConfig)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = lib.CreateNamespace(kubernetesClientForSuiteRun, namespace)
	gomega.Expect(err).To(gomega.BeNil())
	gomega.Expect(lib.DoesNamespaceExist(kubernetesClientForSuiteRun, namespace)).Should(gomega.BeTrue())

	dpa, err := lib.LoadDpaSettingsFromJson(settings)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	bslSecretName = "bsl-cloud-credentials-" + provider
	bslSecretNameWithCarriageReturn = "bsl-cloud-credentials-" + provider + "-with-carriage-return"
	vslSecretName = "vsl-cloud-credentials-" + provider

	veleroPrefix := "velero-e2e-" + string(uuid.NewUUID())

	dpaCR = &lib.DpaCustomResource{
		Name:                 "ts-" + instanceName,
		Namespace:            namespace,
		Client:               runTimeClientForSuiteRun,
		VSLSecretName:        vslSecretName,
		BSLSecretName:        bslSecretName,
		BSLConfig:            dpa.DeepCopy().Spec.BackupLocations[0].Velero.Config,
		BSLProvider:          dpa.DeepCopy().Spec.BackupLocations[0].Velero.Provider,
		BSLBucket:            dpa.DeepCopy().Spec.BackupLocations[0].Velero.ObjectStorage.Bucket,
		BSLBucketPrefix:      veleroPrefix,
		VeleroDefaultPlugins: dpa.DeepCopy().Spec.Configuration.Velero.DefaultPlugins,
		SnapshotLocations:    dpa.DeepCopy().Spec.SnapshotLocations,
		UnsupportedOverrides: dpa.DeepCopy().Spec.UnsupportedOverrides,
	}

	ginkgo.RunSpecs(t, "OADP E2E using velero prefix: "+veleroPrefix)
}

var _ = ginkgo.BeforeSuite(func() {
	// TODO create logger (hh:mm:ss message) to be used by all functions
	log.Printf("Creating Secrets")
	bslCredFileData, err := lib.ReadFile(bslCredFile)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	err = lib.CreateCredentialsSecret(kubernetesClientForSuiteRun, bslCredFileData, namespace, bslSecretName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	err = lib.CreateCredentialsSecret(
		kubernetesClientForSuiteRun,
		lib.ReplaceSecretDataNewLineWithCarriageReturn(bslCredFileData),
		namespace, bslSecretNameWithCarriageReturn,
	)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	vslCredFileData, err := lib.ReadFile(vslCredFile)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	err = lib.CreateCredentialsSecret(kubernetesClientForSuiteRun, vslCredFileData, namespace, vslSecretName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
})

var _ = ginkgo.AfterSuite(func() {
	log.Printf("Deleting Secrets")
	err := lib.DeleteSecret(kubernetesClientForSuiteRun, namespace, vslSecretName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	err = lib.DeleteSecret(kubernetesClientForSuiteRun, namespace, bslSecretName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	err = lib.DeleteSecret(kubernetesClientForSuiteRun, namespace, bslSecretNameWithCarriageReturn)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	log.Printf("Deleting DPA")
	err = dpaCR.Delete()
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Eventually(dpaCR.IsDeleted(), time.Minute*2, time.Second*5).Should(gomega.BeTrue())
})

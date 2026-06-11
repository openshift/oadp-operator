package e2e_test

import (
	"flag"
	"log"
	"os"
	"strconv"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
	libhcp "github.com/openshift/oadp-operator/tests/e2e/lib/hcp"
)

var (
	// Common vars obtained from flags passed in ginkgo.
	bslCredFile, namespace, instanceName, provider, vslCredFile, settings, artifact_dir, scKubeconfig string
	flakeAttempts                                                                                     int64

	kubernetesClientForSuiteRun *kubernetes.Clientset
	crClientForServiceCluster   client.Client
	runTimeClientForSuiteRun    client.Client
	dynamicClientForSuiteRun    dynamic.Interface

	dpaCR                           *lib.DpaCustomResource
	bslSecretName                   string
	bslSecretNameWithCarriageReturn string
	vslSecretName                   string

	kubeConfig          *rest.Config
	kubeConfigForSC     *rest.Config
	knownFlake          bool
	accumulatedTestLogs []string

	kvmEmulation        bool
	useUpstreamHco      bool
	useCommunityHco     bool
	hcoIndexTag         string
	skipMustGather      bool
	hcBackupRestoreMode string
	hcName              string
	hcNamespace         string
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
	flag.Int64Var(&flakeAttempts, "flakeAttempts", 3, "Customize the number of flake retries (3)")
	flag.BoolVar(&kvmEmulation, "kvm_emulation", true, "Enable or disable KVM emulation for virtualization testing")
	flag.BoolVar(&useUpstreamHco, "hco_upstream", false, "Force use of upstream virtualization operator")
	flag.BoolVar(&useCommunityHco, "hco_community", false, "Install community HCO from custom CatalogSource (mutually exclusive with -hco_upstream)")
	flag.StringVar(&hcoIndexTag, "hco_index_tag", "1.17.1", "HCO index image tag for community CatalogSource (used with -hco_community)")
	flag.BoolVar(&skipMustGather, "skipMustGather", false, "avoid errors with local execution and cluster architecture")
	flag.StringVar(&hcBackupRestoreMode, "hc_backup_restore_mode", string(HCModeCreate), "Type of HC test to run")
	flag.StringVar(&hcName, "hc_name", "", "Name of the HostedCluster to use for HCP tests")
	flag.StringVar(&hcNamespace, "hc_namespace", libhcp.ClustersNamespace, "Namespace for HostedClusters")
	flag.StringVar(&scKubeconfig, "sc_kubeconfig", "", "Path to kubeconfig file for Service Cluster. Only used for HCP tests and ROSA.")

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
		if envValue := os.Getenv("TEST_VIRT"); envValue != "" {
			if parsedValue, err := strconv.ParseBool(envValue); err == nil {
				useCommunityHco = parsedValue
			} else {
				log.Println("Error parsing TEST_VIRT, it will be disabled by default: ", err)
			}
		}
		if os.Getenv("HCO_INDEX_TAG") != "" {
			hcoIndexTag = os.Getenv("HCO_INDEX_TAG")
		}
		if envValue := os.Getenv("SKIP_MUST_GATHER"); envValue != "" {
			if parsedValue, err := strconv.ParseBool(envValue); err == nil {
				skipMustGather = parsedValue
			} else {
				log.Println("Error parsing SKIP_MUST_GATHER, must-gather will be enabled by default: ", err)
			}
		}
		if os.Getenv("HC_BACKUP_RESTORE_MODE") != "" {
			hcBackupRestoreMode = os.Getenv("HC_BACKUP_RESTORE_MODE")
		} else {
			hcBackupRestoreMode = string(HCModeCreate)
		}
		if os.Getenv("HC_NAME") != "" {
			hcName = os.Getenv("HC_NAME")
		}
		if os.Getenv("SC_KUBECONFIG") != "" {
			scKubeconfig = os.Getenv("SC_KUBECONFIG")
		}
	}
}

func TestOADPE2E(t *testing.T) {
	flag.Parse()

	if os.Getenv("OPENSHIFT_CI") != "true" {
		log.Println("OPENSHIFT_CI is not set to true, skipping must-gather")
		skipMustGather = true
	}

	if useUpstreamHco && useCommunityHco {
		t.Fatal("Cannot use both -hco_upstream and -hco_community at the same time")
	}

	var err error

	kubeConfig = config.GetConfigOrDie()
	kubeConfig.QPS = 50
	kubeConfig.Burst = 100

	gomega.RegisterFailHandler(ginkgo.Fail)

	kubernetesClientForSuiteRun, err = kubernetes.NewForConfig(kubeConfig)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Set up kubeConfigForSC if sc_kubeconfig flag is provided
	if scKubeconfig != "" {
		kubeConfigForSC, err = clientcmd.BuildConfigFromFlags("", scKubeconfig)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		kubeConfigForSC.QPS = kubeConfig.QPS
		kubeConfigForSC.Burst = kubeConfig.Burst

		scheme := lib.Scheme
		workv1.Install(scheme)
		crClientForServiceCluster, err = client.New(kubeConfigForSC, client.Options{Scheme: scheme})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}

	runTimeClientForSuiteRun, err = client.New(kubeConfig, client.Options{Scheme: lib.Scheme})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

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
		BSLCacert:            dpa.DeepCopy().Spec.BackupLocations[0].Velero.ObjectStorage.CACert,
		BSLBucketPrefix:      veleroPrefix,
		VeleroDefaultPlugins: dpa.DeepCopy().Spec.Configuration.Velero.DefaultPlugins,
		SnapshotLocations:    dpa.DeepCopy().Spec.SnapshotLocations,
		UnsupportedOverrides: dpa.DeepCopy().Spec.UnsupportedOverrides,
	}

	ginkgo.RunSpecs(t, "OADP E2E using velero prefix: "+veleroPrefix)
}

var _ = ginkgo.BeforeSuite(func() {
	// Initialize controller-runtime logger
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

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
	oadpDeploymentOperation := NewOADPDeploymentOperationDefault()
	if HCBackupRestoreMode(hcBackupRestoreMode) == HCModeExternalROSA {
		oadpDeploymentOperation = NewOADPDeploymentOperationROSA()
	}
	oadpDeploymentOperation.Undeploy(lib.KOPIA)
})

package e2e_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

var (
	cacertDpaCR *lib.DpaCustomResource
	noCACertDPA *lib.DpaCustomResource
)

var _ = ginkgo.Describe("BSL cacert with in-cluster minio", ginkgo.Ordered, ginkgo.Label("aws"), func() {
	const (
		minioBSLSecretName = "minio-bsl-creds"
		testBackupName     = "cacert-minio-backup"
		testNamespace      = "cacert-minio-test-app"
	)

	var caPEM []byte

	// ── Setup ──────────────────────────────────────────────────────────────────

	ginkgo.BeforeAll(func(ctx ginkgo.SpecContext) {
		// Initialise early so AfterEach can safely call Delete() even if BeforeAll fails.
		cacertDpaCR = &lib.DpaCustomResource{
			Name:      "ts-cacert-minio",
			Namespace: namespace,
			Client:    runTimeClientForSuiteRun,
		}

		// Clean up any resources left by a previously interrupted run.
		_ = lib.DeleteBackup(runTimeClientForSuiteRun, namespace, testBackupName)

		log.Println("cacert: generating self-signed CA and server certificate")
		var caKeyPEM []byte
		var err error
		caPEM, caKeyPEM, err = lib.GenerateSelfSignedCA()
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "generating self-signed CA")

		dnsNames := []string{
			lib.MinioServiceName,
			fmt.Sprintf("%s.%s", lib.MinioServiceName, namespace),
			fmt.Sprintf("%s.%s.svc", lib.MinioServiceName, namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", lib.MinioServiceName, namespace),
		}
		certPEM, keyPEM, err := lib.GenerateServerCert(caPEM, caKeyPEM, dnsNames)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "generating minio server certificate")

		log.Println("cacert: deploying minio with TLS")
		minioURL, err := lib.DeployMinioWithTLS(ctx, kubernetesClientForSuiteRun, namespace, certPEM, keyPEM)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "deploying minio with TLS in namespace %s", namespace)
		log.Printf("cacert: minio available at %s", minioURL)

		log.Printf("cacert: creating bucket %s", lib.MinioBucketName)
		err = lib.CreateMinioBucket(ctx, kubernetesClientForSuiteRun, kubeConfig, namespace, lib.MinioBucketName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "creating minio bucket %s", lib.MinioBucketName)

		log.Println("cacert: creating BSL credentials secret")
		credsData := fmt.Sprintf("[default]\naws_access_key_id = %s\naws_secret_access_key = %s\n",
			lib.MinioAccessKey, lib.MinioSecretKey)
		_, err = kubernetesClientForSuiteRun.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: minioBSLSecretName, Namespace: namespace},
			Data:       map[string][]byte{"cloud": []byte(credsData)},
		}, metav1.CreateOptions{})
		// Tolerate AlreadyExists: AfterAll may not have run if a previous run was killed.
		if err != nil && !apierrors.IsAlreadyExists(err) {
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "creating BSL credentials secret %s", minioBSLSecretName)
		}

		// kubevirt/hypershift plugins lack arm64-compatible images in some environments.
		cacertDpaCR.BSLSecretName = minioBSLSecretName
		cacertDpaCR.BSLProvider = dpaCR.BSLProvider
		cacertDpaCR.BSLBucket = lib.MinioBucketName
		cacertDpaCR.BSLBucketPrefix = "e2e"
		cacertDpaCR.BSLCacert = caPEM
		cacertDpaCR.BSLConfig = map[string]string{
			"s3Url":            minioURL,
			"s3ForcePathStyle": "true",
			"region":           "us-east-1",
		}
		cacertDpaCR.VeleroDefaultPlugins = []oadpv1alpha1.DefaultPlugin{
			oadpv1alpha1.DefaultPluginOpenShift,
			oadpv1alpha1.DefaultPluginAWS,
		}
		cacertDpaCR.UnsupportedOverrides = dpaCR.UnsupportedOverrides
	})

	// ── Teardown ───────────────────────────────────────────────────────────────

	ginkgo.AfterAll(func(ctx ginkgo.SpecContext) {
		lib.DeleteMinioResources(ctx, kubernetesClientForSuiteRun, namespace)
		_ = lib.DeleteSecret(kubernetesClientForSuiteRun, namespace, minioBSLSecretName)
	})

	ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		if !skipMustGather && ctx.SpecReport().Failed() {
			_ = lib.RunMustGather(artifact_dir, cacertDpaCR.Client)
		}
		// Happy path: the positive It block exercised deletion via DeleteBackupRequest.
		// Fallback: direct CR delete when the test failed before reaching that step.
		if ctx.SpecReport().Failed() {
			if err := lib.DeleteBackup(runTimeClientForSuiteRun, namespace, testBackupName); err != nil {
				log.Printf("cacert: warning: could not delete backup CR %s: %v", testBackupName, err)
			}
		}
		// Clean up whichever DPA was created this run (positive or negative test).
		if noCACertDPA != nil {
			if err := noCACertDPA.Delete(); err != nil {
				log.Printf("cacert: warning: could not delete DPA %s: %v", noCACertDPA.Name, err)
			}
			noCACertDPA = nil
		}
		gomega.Expect(cacertDpaCR.Delete()).NotTo(gomega.HaveOccurred())
		gomega.Eventually(lib.VeleroIsDeleted(kubernetesClientForSuiteRun, namespace), 5*time.Minute, 5*time.Second).Should(gomega.BeTrue())
	})

	// ── Tests ──────────────────────────────────────────────────────────────────

	ginkgo.It("BSL is Available and Velero gets AWS_CA_BUNDLE when using minio with custom TLS", func(ctx ginkgo.SpecContext) {
		gomega.Expect(cacertDpaCR.CreateOrUpdate(cacertDpaCR.Build(lib.CSI))).NotTo(gomega.HaveOccurred())
		gomega.Eventually(cacertDpaCR.IsReconciledTrue(), 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())
		gomega.Eventually(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())

		// BSL Available means Velero successfully TLS-connected to minio using our CA.
		log.Println("cacert: waiting for BSL to become Available")
		gomega.Eventually(cacertDpaCR.BSLsAreAvailable(), 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())

		gomega.Expect(awsCABundleIsSet(ctx, "/etc/velero/ca-certs/ca-bundle.pem")).NotTo(gomega.HaveOccurred())

		cm, err := kubernetesClientForSuiteRun.CoreV1().ConfigMaps(namespace).Get(ctx, "velero-ca-bundle", metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(cm.Data).To(gomega.HaveKey("ca-bundle.pem"))

		log.Println("cacert: running backup via minio BSL")
		gomega.Expect(lib.CreateNamespace(kubernetesClientForSuiteRun, testNamespace)).NotTo(gomega.HaveOccurred())
		defer func() { _ = lib.DeleteNamespace(kubernetesClientForSuiteRun, testNamespace) }()

		_, err = kubernetesClientForSuiteRun.CoreV1().ConfigMaps(testNamespace).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: testNamespace},
			Data:       map[string]string{"key": "value"},
		}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		gomega.Expect(lib.CreateBackupForNamespaces(runTimeClientForSuiteRun, namespace, testBackupName, []string{testNamespace}, false, false)).
			NotTo(gomega.HaveOccurred())
		gomega.Eventually(lib.IsBackupDone(runTimeClientForSuiteRun, namespace, testBackupName), 10*time.Minute, 10*time.Second).
			Should(gomega.BeTrue())

		succeeded, err := lib.IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, runTimeClientForSuiteRun, namespace, testBackupName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(succeeded).To(gomega.BeTrue(), "backup to minio with custom CA cert should complete successfully")

		// Deletion path: Velero must connect to minio via the custom CA to remove
		// backup objects. The DPA stays up so Velero is still running — its controller
		// must clear the backup finalizer, otherwise the CR gets stuck Terminating.
		log.Println("cacert: deleting backup via minio BSL")
		gomega.Expect(lib.DeleteVeleroBackupAndRestore(
			runTimeClientForSuiteRun, kubernetesClientForSuiteRun, kubeConfig,
			namespace, testBackupName, "",
		)).NotTo(gomega.HaveOccurred())
	})

	ginkgo.It("BSL without CACert does not become Available against minio with self-signed TLS", func(ctx ginkgo.SpecContext) {
		// Same minio, same bucket — but no CA cert supplied to the BSL.
		// Velero cannot verify minio's self-signed cert, so the BSL must never become Available.
		// AfterEach cleans up noCACertDPA alongside cacertDpaCR.
		noCACertDPA = &lib.DpaCustomResource{
			Name:                 "ts-cacert-minio-nocert",
			Namespace:            namespace,
			Client:               runTimeClientForSuiteRun,
			BSLSecretName:        minioBSLSecretName,
			BSLProvider:          cacertDpaCR.BSLProvider,
			BSLBucket:            lib.MinioBucketName,
			BSLBucketPrefix:      "e2e-nocert",
			BSLConfig:            cacertDpaCR.BSLConfig,
			VeleroDefaultPlugins: cacertDpaCR.VeleroDefaultPlugins,
			UnsupportedOverrides: cacertDpaCR.UnsupportedOverrides,
			// BSLCacert intentionally omitted
		}

		gomega.Expect(noCACertDPA.CreateOrUpdate(noCACertDPA.Build(lib.CSI))).NotTo(gomega.HaveOccurred())
		gomega.Eventually(noCACertDPA.IsReconciledTrue(), 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())
		gomega.Eventually(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())

		gomega.Consistently(noCACertDPA.BSLsAreAvailable(), 90*time.Second, 10*time.Second).Should(gomega.BeFalse(),
			"BSL without CA cert should not become Available when minio uses a self-signed certificate")
	})
})

// awsCABundleIsSet polls the Velero deployment until AWS_CA_BUNDLE equals wantPath.
func awsCABundleIsSet(ctx context.Context, wantPath string) error {
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
		dep, err := lib.GetVeleroDeployment(kubernetesClientForSuiteRun, namespace)
		if err != nil {
			return false, err // surface permanent errors (RBAC, missing CRD) immediately
		}
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.Name != "velero" {
				continue
			}
			for _, env := range c.Env {
				if env.Name == "AWS_CA_BUNDLE" && env.Value == wantPath {
					return true, nil
				}
			}
		}
		return false, nil
	})
}

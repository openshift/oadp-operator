package e2e_test

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

var (
	minioBRDpaCR *lib.DpaCustomResource
)

var _ = ginkgo.Describe("MinIO Backup and Restore with TLS", ginkgo.Ordered, ginkgo.Label("aws"), func() {
	const (
		minioBSLSecretName = "minio-br-bsl-creds"
		testBackupName     = "minio-br-backup"
		testRestoreName    = "minio-br-restore"
		testAppNamespace   = "minio-br-test-app"
		testConfigMapName  = "test-data-cm"
		testSecretName     = "test-secret"
	)

	var caPEM []byte
	var minioURL string

	// ── Setup Phase ────────────────────────────────────────────────────────────

	ginkgo.BeforeAll(func(ctx ginkgo.SpecContext) {
		// Initialize DPA CR early for cleanup in AfterEach
		minioBRDpaCR = &lib.DpaCustomResource{
			Name:      "ts-minio-br",
			Namespace: namespace,
			Client:    runTimeClientForSuiteRun,
		}
		if err := lib.DeleteNamespace(kubernetesClientForSuiteRun, testAppNamespace); err != nil {
			log.Printf("minio-br: warning: cleanup failed for namespace %s: %v", testAppNamespace, err)
		} else {
			gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, testAppNamespace), 2*time.Minute, 5*time.Second).
				Should(gomega.BeTrue())
		}

		// ── Generate TLS Certificates ──

		log.Println("minio-br: generating self-signed CA and server certificate")
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

		// ── Deploy MinIO with TLS ──

		log.Println("minio-br: deploying minio with TLS")
		minioURL, err = lib.DeployMinioWithTLS(ctx, kubernetesClientForSuiteRun, namespace, certPEM, keyPEM)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "deploying minio with TLS in namespace %s", namespace)
		log.Println("minio-br: minio deployed and available")

		// ── Create MinIO Bucket ──

		log.Printf("minio-br: creating bucket %s", lib.MinioBucketName)
		err = lib.CreateMinioBucket(ctx, kubernetesClientForSuiteRun, kubeConfig, namespace, lib.MinioBucketName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "creating minio bucket %s", lib.MinioBucketName)

		// ── Create BSL Credentials Secret ──

		log.Println("minio-br: creating BSL credentials secret")
		credsData := fmt.Sprintf("[default]\naws_access_key_id = %s\naws_secret_access_key = %s\n",
			lib.MinioAccessKey, lib.MinioSecretKey)

		// Delete any existing secret first to ensure fresh credentials
		if err = lib.DeleteSecret(kubernetesClientForSuiteRun, namespace, minioBSLSecretName); err != nil && !apierrors.IsNotFound(err) {
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "deleting existing BSL credentials secret %s", minioBSLSecretName)
		}

		_, err = kubernetesClientForSuiteRun.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: minioBSLSecretName, Namespace: namespace},
			Data:       map[string][]byte{"cloud": []byte(credsData)},
		}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "creating BSL credentials secret %s", minioBSLSecretName)

		// ── Configure DPA ──

		log.Println("minio-br: configuring DPA with TLS")
		minioBRDpaCR.BSLSecretName = minioBSLSecretName
		minioBRDpaCR.BSLProvider = "aws"
		minioBRDpaCR.BSLBucket = lib.MinioBucketName
		minioBRDpaCR.BSLBucketPrefix = "e2e-backup-restore"
		minioBRDpaCR.BSLCacert = caPEM
		minioBRDpaCR.BSLConfig = map[string]string{
			"s3Url":            minioURL,
			"s3ForcePathStyle": "true",
			"region":           "minio",
		}
		minioBRDpaCR.VeleroDefaultPlugins = []oadpv1alpha1.DefaultPlugin{
			oadpv1alpha1.DefaultPluginOpenShift,
			oadpv1alpha1.DefaultPluginAWS,
		}

		// Use common overrides if available
		if dpaCR != nil {
			minioBRDpaCR.UnsupportedOverrides = dpaCR.UnsupportedOverrides
		}
	})

	// ── Teardown Phase ─────────────────────────────────────────────────────────

	ginkgo.AfterAll(func(ctx ginkgo.SpecContext) {
		log.Println("minio-br: cleaning up minio and resources")
		lib.DeleteMinioResources(ctx, kubernetesClientForSuiteRun, namespace)
		if err := lib.DeleteSecret(kubernetesClientForSuiteRun, namespace, minioBSLSecretName); err != nil {
			log.Printf("minio-br: warning: cleanup failed for secret %s: %v", minioBSLSecretName, err)
		}
	})

	ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		// Gather logs on failure
		if ctx.SpecReport().Failed() {
			if !skipMustGather {
				if err := lib.RunMustGather(artifact_dir, minioBRDpaCR.Client); err != nil {
					fmt.Fprintf(ginkgo.GinkgoWriter, "minio-br: warning: must-gather failed: %v\n", err)
				}
			}
			// Clean up backup/restore CRs if test failed
			if err := lib.DeleteVeleroBackupAndRestore(
				runTimeClientForSuiteRun, kubernetesClientForSuiteRun, kubeConfig,
				namespace, testBackupName, testRestoreName,
			); err != nil {
				fmt.Fprintf(ginkgo.GinkgoWriter, "minio-br: warning: cleanup failed for backup/restore: %v\n", err)
			}
		}

		// Clean up DPA
		if err := minioBRDpaCR.Delete(); err != nil {
			log.Printf("minio-br: warning: could not delete DPA %s: %v", minioBRDpaCR.Name, err)
		} else {
			// Only wait for Velero deletion if DPA deletion succeeded
			gomega.Eventually(lib.VeleroIsDeleted(kubernetesClientForSuiteRun, namespace), 5*time.Minute, 5*time.Second).
				Should(gomega.BeTrue())
		}

		// Clean up test application namespace
		if err := lib.DeleteNamespace(kubernetesClientForSuiteRun, testAppNamespace); err != nil {
			log.Printf("minio-br: warning: cleanup failed for namespace %s: %v", testAppNamespace, err)
		} else {
			gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, testAppNamespace), 2*time.Minute, 5*time.Second).
				Should(gomega.BeTrue())
		}
	})

	// ── Tests ──────────────────────────────────────────────────────────────────

	ginkgo.It("should complete full backup and restore cycle with MinIO TLS", func(ctx ginkgo.SpecContext) {
		// ── Create and verify DPA ──

		log.Println("minio-br: creating DPA with MinIO TLS")
		gomega.Expect(minioBRDpaCR.CreateOrUpdate(minioBRDpaCR.Build(lib.CSI))).NotTo(gomega.HaveOccurred())

		log.Println("minio-br: waiting for DPA to be reconciled")
		gomega.Eventually(minioBRDpaCR.IsReconciledTrue(), 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())

		log.Println("minio-br: waiting for Velero pod to be running")
		gomega.Eventually(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())

		log.Println("minio-br: waiting for BSL to become Available")
		gomega.Eventually(minioBRDpaCR.BSLsAreAvailable(), 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())

		// ── Verify AWS_CA_BUNDLE is set ──

		log.Println("minio-br: verifying AWS_CA_BUNDLE is set in Velero pod")
		gomega.Expect(awsCABundleIsSet(ctx, "/etc/velero/ca-certs/ca-bundle.pem")).NotTo(gomega.HaveOccurred())

		// ── Create test application ──

		log.Printf("minio-br: creating test application in namespace %s", testAppNamespace)
		gomega.Expect(lib.CreateNamespace(kubernetesClientForSuiteRun, testAppNamespace)).NotTo(gomega.HaveOccurred())

		// Create ConfigMap with test data
		log.Printf("minio-br: creating ConfigMap with test data")
		_, err := kubernetesClientForSuiteRun.CoreV1().ConfigMaps(testAppNamespace).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: testConfigMapName, Namespace: testAppNamespace},
			Data: map[string]string{
				"config.yaml": "app:\n  name: test-app\n  version: 1.0.0",
				"data.json":   "{\"key\": \"value\", \"timestamp\": \"2024-01-01T00:00:00Z\"}",
			},
		}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Create Secret with test data
		log.Printf("minio-br: creating Secret with test data")
		_, err = kubernetesClientForSuiteRun.CoreV1().Secrets(testAppNamespace).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: testAppNamespace},
			Type:       corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"username": []byte("admin"),
				"password": []byte("secretpassword123"),
			},
		}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// ── Backup Phase ──

		log.Printf("minio-br: creating backup for namespace %s", testAppNamespace)
		gomega.Expect(lib.CreateBackupForNamespaces(runTimeClientForSuiteRun, namespace, testBackupName, []string{testAppNamespace}, false, false)).
			NotTo(gomega.HaveOccurred())

		log.Printf("minio-br: waiting for backup %s to complete", testBackupName)
		gomega.Eventually(lib.IsBackupDone(runTimeClientForSuiteRun, namespace, testBackupName), 10*time.Minute, 10*time.Second).
			Should(gomega.BeTrue())

		log.Printf("minio-br: verifying backup %s completed successfully", testBackupName)
		succeeded, err := lib.IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, runTimeClientForSuiteRun, namespace, testBackupName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(succeeded).To(gomega.BeTrue(), "backup with MinIO TLS should complete successfully")

		// Log backup details
		describeBackup := lib.DescribeBackup(runTimeClientForSuiteRun, namespace, testBackupName)
		log.Printf("minio-br: backup details:\n%s", describeBackup)

		// ── Simulate Disaster: Delete application ──

		log.Printf("minio-br: simulating disaster - deleting application namespace %s", testAppNamespace)
		gomega.Expect(lib.DeleteNamespace(kubernetesClientForSuiteRun, testAppNamespace)).NotTo(gomega.HaveOccurred())

		// Verify namespace is gone
		log.Printf("minio-br: waiting for namespace %s to be fully deleted", testAppNamespace)
		gomega.Eventually(func() bool {
			_, err := kubernetesClientForSuiteRun.CoreV1().Namespaces().Get(ctx, testAppNamespace, metav1.GetOptions{})
			return apierrors.IsNotFound(err)
		}, 2*time.Minute, 5*time.Second).Should(gomega.BeTrue(), "test namespace should be deleted before restore")

		// ── Restore Phase ──

		log.Printf("minio-br: creating restore from backup %s", testBackupName)
		gomega.Expect(lib.CreateRestoreFromBackup(runTimeClientForSuiteRun, namespace, testBackupName, testRestoreName)).
			NotTo(gomega.HaveOccurred())

		log.Printf("minio-br: waiting for restore %s to complete", testRestoreName)
		gomega.Eventually(lib.IsRestoreDone(runTimeClientForSuiteRun, namespace, testRestoreName), 10*time.Minute, 10*time.Second).
			Should(gomega.BeTrue())

		log.Printf("minio-br: verifying restore %s completed successfully", testRestoreName)
		restoreSucceeded, err := lib.IsRestoreCompletedSuccessfully(kubernetesClientForSuiteRun, runTimeClientForSuiteRun, namespace, testRestoreName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(restoreSucceeded).To(gomega.BeTrue(), "restore from MinIO with TLS should complete successfully")

		// Log restore details
		describeRestore := lib.DescribeRestore(runTimeClientForSuiteRun, namespace, testRestoreName)
		log.Printf("minio-br: restore details:\n%s", describeRestore)

		// ── Verification Phase: Data Integrity ──

		log.Printf("minio-br: verifying restored application data")
		verifyRestoredData(ctx, kubernetesClientForSuiteRun, testAppNamespace, testConfigMapName, testSecretName)

		// ── Cleanup Phase ──

		log.Printf("minio-br: cleaning up backup and restore CRs")
		gomega.Expect(lib.DeleteVeleroBackupAndRestore(
			runTimeClientForSuiteRun, kubernetesClientForSuiteRun, kubeConfig,
			namespace, testBackupName, testRestoreName,
		)).NotTo(gomega.HaveOccurred())
	})

	ginkgo.It("should handle restore with resource filters", func(ctx ginkgo.SpecContext) {
		// ── Setup DPA and MinIO ──

		log.Println("minio-br-filters: creating DPA with MinIO TLS")
		gomega.Expect(minioBRDpaCR.CreateOrUpdate(minioBRDpaCR.Build(lib.CSI))).NotTo(gomega.HaveOccurred())

		gomega.Eventually(minioBRDpaCR.IsReconciledTrue(), 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())
		gomega.Eventually(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())
		gomega.Eventually(minioBRDpaCR.BSLsAreAvailable(), 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())

		// ── Create test application ──

		log.Printf("minio-br-filters: creating test application")
		gomega.Expect(lib.CreateNamespace(kubernetesClientForSuiteRun, testAppNamespace)).NotTo(gomega.HaveOccurred())

		// Create ConfigMap
		_, err := kubernetesClientForSuiteRun.CoreV1().ConfigMaps(testAppNamespace).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: testConfigMapName, Namespace: testAppNamespace},
			Data:       map[string]string{"key": "value"},
		}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Create Secret
		_, err = kubernetesClientForSuiteRun.CoreV1().Secrets(testAppNamespace).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: testAppNamespace},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{"password": []byte("secret")},
		}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// ── Backup ──

		log.Printf("minio-br-filters: creating backup")
		gomega.Expect(lib.CreateBackupForNamespaces(runTimeClientForSuiteRun, namespace, testBackupName, []string{testAppNamespace}, false, false)).
			NotTo(gomega.HaveOccurred())

		gomega.Eventually(lib.IsBackupDone(runTimeClientForSuiteRun, namespace, testBackupName), 10*time.Minute, 10*time.Second).
			Should(gomega.BeTrue())

		succeeded, err := lib.IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, runTimeClientForSuiteRun, namespace, testBackupName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(succeeded).To(gomega.BeTrue())

		// ── Delete application ──

		log.Printf("minio-br-filters: deleting application namespace %s", testAppNamespace)
		gomega.Expect(lib.DeleteNamespace(kubernetesClientForSuiteRun, testAppNamespace)).NotTo(gomega.HaveOccurred())

		log.Printf("minio-br-filters: waiting for namespace %s to be fully deleted", testAppNamespace)
		gomega.Eventually(func() bool {
			_, err := kubernetesClientForSuiteRun.CoreV1().Namespaces().Get(ctx, testAppNamespace, metav1.GetOptions{})
			return apierrors.IsNotFound(err)
		}, 2*time.Minute, 5*time.Second).Should(gomega.BeTrue(), "test namespace should be deleted before restore")

		// ── Restore with filter: only ConfigMaps ──

		log.Printf("minio-br-filters: creating restore with resource filter (ConfigMaps only)")
		includedResources := []string{"configmaps"}
		gomega.Expect(lib.CreateCustomRestoreFromBackup(
			runTimeClientForSuiteRun, namespace, testBackupName, testRestoreName,
			includedResources, nil, nil,
		)).NotTo(gomega.HaveOccurred())

		gomega.Eventually(lib.IsRestoreDone(runTimeClientForSuiteRun, namespace, testRestoreName), 10*time.Minute, 10*time.Second).
			Should(gomega.BeTrue())

		restoreSucceeded, err := lib.IsRestoreCompletedSuccessfully(kubernetesClientForSuiteRun, runTimeClientForSuiteRun, namespace, testRestoreName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(restoreSucceeded).To(gomega.BeTrue())

		// ── Verify only ConfigMap was restored ──

		log.Printf("minio-br-filters: verifying only ConfigMap was restored")
		cm, err := kubernetesClientForSuiteRun.CoreV1().ConfigMaps(testAppNamespace).Get(ctx, testConfigMapName, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "ConfigMap should be restored")
		gomega.Expect(cm.Data["key"]).To(gomega.Equal("value"))

		// Secret should NOT be restored
		_, err = kubernetesClientForSuiteRun.CoreV1().Secrets(testAppNamespace).Get(ctx, testSecretName, metav1.GetOptions{})
		gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue(), "Secret should NOT be restored (filtered out)")

		// ── Cleanup ──

		gomega.Expect(lib.DeleteVeleroBackupAndRestore(
			runTimeClientForSuiteRun, kubernetesClientForSuiteRun, kubeConfig,
			namespace, testBackupName, testRestoreName,
		)).NotTo(gomega.HaveOccurred())
	})
})

// ── Helper Functions ───────────────────────────────────────────────────────

// verifyRestoredData checks that backed up data was properly restored
func verifyRestoredData(ctx context.Context, kubeClient *kubernetes.Clientset, namespace, cmName, secretName string) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ctx = timeoutCtx

	// Verify ConfigMap was restored
	log.Printf("minio-br: verifying ConfigMap %s exists in namespace %s", cmName, namespace)
	cm, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(ctx, cmName, metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "ConfigMap should exist after restore")
	gomega.Expect(cm.Data).NotTo(gomega.BeEmpty(), "ConfigMap should have data")
	expectedConfigYAML := "app:\n  name: test-app\n  version: 1.0.0"
	expectedDataJSON := "{\"key\": \"value\", \"timestamp\": \"2024-01-01T00:00:00Z\"}"
	gomega.Expect(cm.Data).To(gomega.HaveKey("config.yaml"), "ConfigMap should contain config.yaml")
	gomega.Expect(cm.Data["config.yaml"]).To(gomega.Equal(expectedConfigYAML), "config.yaml content should match")
	gomega.Expect(cm.Data).To(gomega.HaveKey("data.json"), "ConfigMap should contain data.json")
	gomega.Expect(cm.Data["data.json"]).To(gomega.Equal(expectedDataJSON), "data.json content should match")
	log.Printf("minio-br: ConfigMap verified - has %d keys", len(cm.Data))

	// Verify Secret was restored
	log.Printf("minio-br: verifying Secret %s exists in namespace %s", secretName, namespace)
	secret, err := kubeClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Secret should exist after restore")
	gomega.Expect(secret.Data).NotTo(gomega.BeEmpty(), "Secret should have data")
	gomega.Expect(secret.Data).To(gomega.HaveKey("username"), "Secret should contain username")
	gomega.Expect(secret.Data).To(gomega.HaveKey("password"), "Secret should contain password")
	gomega.Expect(string(secret.Data["username"])).To(gomega.Equal("admin"), "Username should match")
	passwordMatch := subtle.ConstantTimeCompare(secret.Data["password"], []byte("secretpassword123"))
	gomega.Expect(passwordMatch).To(gomega.Equal(1), "Password should match")
	log.Printf("minio-br: Secret verified - has %d keys", len(secret.Data))

	// Verify namespace still exists
	log.Printf("minio-br: verifying namespace %s exists", namespace)
	ns, err := kubeClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(ns.Name).To(gomega.Equal(namespace))
	log.Printf("minio-br: namespace verified")
}

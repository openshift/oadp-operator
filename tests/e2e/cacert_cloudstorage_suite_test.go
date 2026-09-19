package e2e_test

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

// This suite covers OADP-8056: for a CloudStorage-backed BackupLocation, OADP must create an
// owned Secret from an inline `caCert` and point the resulting Velero BSL's `caCertRef` at it,
// instead of copying the CA bytes inline. It only exercises reconciliation and the resulting
// BSL/Secret state — it does not need a working backup, since the CloudStorage-backed BSL here
// points at the suite's real cloud bucket (a self-signed test CA would correctly fail TLS
// verification against that bucket's real public CA, so BSL Available is not asserted).
var _ = ginkgo.Describe("BSL cacert with CloudStorage-backed BSL", ginkgo.Ordered, ginkgo.Label("aws"), func() {
	const (
		cloudStorageName = "ts-cacert-cloudstorage"
		dpaName          = "ts-cacert-cloudstorage"
		bslName          = "cs-cacert-bsl"
	)

	var (
		caPEM             []byte
		cloudStorageDpaCR *lib.DpaCustomResource
	)

	ginkgo.BeforeAll(func(ctx ginkgo.SpecContext) {
		cloudStorageDpaCR = &lib.DpaCustomResource{
			Name:      dpaName,
			Namespace: namespace,
			Client:    runTimeClientForSuiteRun,
		}

		var err error
		caPEM, _, err = lib.GenerateSelfSignedCA()
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "generating self-signed CA")

		cloudStorage := &oadpv1alpha1.CloudStorage{
			ObjectMeta: metav1.ObjectMeta{Name: cloudStorageName, Namespace: namespace},
			Spec: oadpv1alpha1.CloudStorageSpec{
				Name:     dpaCR.BSLBucket,
				Provider: oadpv1alpha1.AWSBucketProvider,
				Region:   dpaCR.BSLConfig["region"],
				CreationSecret: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: bslSecretName},
					Key:                  "cloud",
				},
			},
		}
		err = runTimeClientForSuiteRun.Create(ctx, cloudStorage)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "creating CloudStorage CR %s", cloudStorageName)
		}
	})

	ginkgo.AfterAll(func(ctx ginkgo.SpecContext) {
		cloudStorage := &oadpv1alpha1.CloudStorage{ObjectMeta: metav1.ObjectMeta{Name: cloudStorageName, Namespace: namespace}}
		_ = runTimeClientForSuiteRun.Delete(ctx, cloudStorage)
	})

	ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		if !skipMustGather && ctx.SpecReport().Failed() {
			_ = lib.RunMustGather(artifact_dir, cloudStorageDpaCR.Client)
		}
		gomega.Expect(cloudStorageDpaCR.Delete()).NotTo(gomega.HaveOccurred())
		gomega.Eventually(lib.VeleroIsDeleted(kubernetesClientForSuiteRun, namespace), 5*time.Minute, 5*time.Second).Should(gomega.BeTrue())
	})

	buildCloudStorageDpaSpec := func(caCert []byte) *oadpv1alpha1.DataProtectionApplicationSpec {
		return &oadpv1alpha1.DataProtectionApplicationSpec{
			Configuration: &oadpv1alpha1.ApplicationConfig{
				Velero: &oadpv1alpha1.VeleroConfig{
					DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
						oadpv1alpha1.DefaultPluginOpenShift,
						oadpv1alpha1.DefaultPluginAWS,
					},
				},
				NodeAgent: &oadpv1alpha1.NodeAgentConfig{
					UploaderType: "kopia",
					NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
						Enable: ptr.To(false),
					},
				},
			},
			BackupLocations: []oadpv1alpha1.BackupLocation{
				{
					Name: bslName,
					CloudStorage: &oadpv1alpha1.CloudStorageLocation{
						CloudStorageRef: corev1.LocalObjectReference{Name: cloudStorageName},
						CACert:          caCert,
						// Prefix is required by DPA validation when backupImages is enabled.
						Prefix: "cloudstorage",
					},
				},
			},
		}
	}

	ginkgo.It("creates an owned Secret and sets CACertRef instead of inline CACert for a CloudStorage-backed BSL", func(ctx ginkgo.SpecContext) {
		gomega.Expect(cloudStorageDpaCR.CreateOrUpdate(buildCloudStorageDpaSpec(caPEM))).NotTo(gomega.HaveOccurred())
		gomega.Eventually(cloudStorageDpaCR.IsReconciledTrue(), 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())

		bsl := &velero.BackupStorageLocation{}
		gomega.Expect(runTimeClientForSuiteRun.Get(ctx, types.NamespacedName{Namespace: namespace, Name: bslName}, bsl)).NotTo(gomega.HaveOccurred())
		gomega.Expect(bsl.Spec.ObjectStorage).NotTo(gomega.BeNil())
		gomega.Expect(bsl.Spec.ObjectStorage.CACert).To(gomega.BeEmpty(), "inline CACert should not be set on the Velero BSL")
		gomega.Expect(bsl.Spec.ObjectStorage.CACertRef).NotTo(gomega.BeNil(), "CACertRef should be set on the Velero BSL")

		expectedSecretName := "oadp-" + bslName + "-cacert"
		gomega.Expect(bsl.Spec.ObjectStorage.CACertRef.Name).To(gomega.Equal(expectedSecretName))
		gomega.Expect(bsl.Spec.ObjectStorage.CACertRef.Key).To(gomega.Equal("cacert"))

		secret, err := kubernetesClientForSuiteRun.CoreV1().Secrets(namespace).Get(context.Background(), expectedSecretName, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(secret.Data).To(gomega.HaveKeyWithValue("cacert", caPEM))
		gomega.Expect(secret.Labels[oadpv1alpha1.OadpOperatorLabel]).To(gomega.Equal("True"))

		dpa, err := cloudStorageDpaCR.Get()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ownedByDPA := false
		for _, ref := range secret.OwnerReferences {
			if ref.Kind == "DataProtectionApplication" && ref.Name == dpa.Name {
				ownedByDPA = true
			}
		}
		gomega.Expect(ownedByDPA).To(gomega.BeTrue(), "the generated CACert secret should be owned by the DPA")

		ginkgo.GinkgoWriter.Println("cacert: removing inline CACert should clean up the generated Secret")
		gomega.Expect(cloudStorageDpaCR.CreateOrUpdate(buildCloudStorageDpaSpec(nil))).NotTo(gomega.HaveOccurred())
		gomega.Eventually(cloudStorageDpaCR.IsReconciledTrue(), 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())
		gomega.Eventually(func() bool {
			_, err := kubernetesClientForSuiteRun.CoreV1().Secrets(namespace).Get(context.Background(), expectedSecretName, metav1.GetOptions{})
			return apierrors.IsNotFound(err)
		}, 2*time.Minute, 5*time.Second).Should(gomega.BeTrue(), "generated CACert secret should be deleted once inline CACert is removed")
	})
})

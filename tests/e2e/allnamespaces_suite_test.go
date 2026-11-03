package e2e_test

import (
	"context"
	"log"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	operatorv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

// These specs validate AllNamespaces-specific install behavior. They carry the
// "allnamespaces" label and are excluded from the default ginkgo filter; they
// are meant to run against an operator installed with a global OperatorGroup
// (make deploy-olm-allnamespaces / the -allnamespaces CI jobs), selected via
// TEST_ALLNAMESPACES=true.
var _ = ginkgo.Describe("AllNamespaces install mode", ginkgo.Label("allnamespaces"), ginkgo.Ordered, func() {
	ctx := context.Background()

	ginkgo.It("should run the operator under a global OperatorGroup", func() {
		operatorGroupList := operatorv1.OperatorGroupList{}
		err := runTimeClientForSuiteRun.List(ctx, &operatorGroupList, client.InNamespace(namespace))
		gomega.Expect(err).To(gomega.BeNil())
		gomega.Expect(operatorGroupList.Items).To(gomega.HaveLen(1))
		operatorGroup := operatorGroupList.Items[0]
		// empty targetNamespaces is OLM's all-namespaces signal
		gomega.Expect(operatorGroup.Spec.TargetNamespaces).To(gomega.BeEmpty())
		gomega.Eventually(func() []string {
			updated := operatorv1.OperatorGroup{}
			err := runTimeClientForSuiteRun.Get(ctx, client.ObjectKeyFromObject(&operatorGroup), &updated)
			if err != nil {
				return []string{"error"}
			}
			return updated.Status.Namespaces
		}, time.Minute*2, time.Second*5).Should(gomega.ContainElement(""))
	})

	ginkgo.It("should expose a CSV supporting both AllNamespaces and OwnNamespace install modes", func() {
		csvList := operatorv1alpha1.ClusterServiceVersionList{}
		err := runTimeClientForSuiteRun.List(ctx, &csvList, client.InNamespace(namespace))
		gomega.Expect(err).To(gomega.BeNil())
		var csv *operatorv1alpha1.ClusterServiceVersion
		for i := range csvList.Items {
			if len(csvList.Items[i].Name) > len("oadp-operator.") && csvList.Items[i].Name[:len("oadp-operator.")] == "oadp-operator." {
				csv = &csvList.Items[i]
				break
			}
		}
		gomega.Expect(csv).NotTo(gomega.BeNil(), "expected an oadp-operator CSV in namespace %s", namespace)
		supportedModes := map[operatorv1alpha1.InstallModeType]bool{}
		for _, mode := range csv.Spec.InstallModes {
			supportedModes[mode.Type] = mode.Supported
		}
		gomega.Expect(supportedModes[operatorv1alpha1.InstallModeTypeAllNamespaces]).To(gomega.BeTrue())
		gomega.Expect(supportedModes[operatorv1alpha1.InstallModeTypeOwnNamespace]).To(gomega.BeTrue())
	})

	ginkgo.It("should resolve the operator's own namespace via OPERATOR_NAMESPACE when WATCH_NAMESPACE is empty", func() {
		var operatorPod *corev1.Pod
		gomega.Eventually(func() error {
			pod, err := lib.GetPodWithLabel(kubernetesClientForSuiteRun, namespace, "control-plane=controller-manager")
			operatorPod = pod
			return err
		}, time.Minute*3, time.Second*15).Should(gomega.BeNil())

		// OLM's all-namespaces signal: the annotation the WATCH_NAMESPACE
		// fieldRef resolves from is the empty string.
		gomega.Expect(operatorPod.Annotations).To(gomega.HaveKeyWithValue("olm.targetNamespaces", ""))

		var watchNamespaceEnv, operatorNamespaceEnv *corev1.EnvVar
		for i, container := range operatorPod.Spec.Containers {
			if container.Name != "manager" {
				continue
			}
			for j, env := range operatorPod.Spec.Containers[i].Env {
				switch env.Name {
				case "WATCH_NAMESPACE":
					watchNamespaceEnv = &operatorPod.Spec.Containers[i].Env[j]
				case "OPERATOR_NAMESPACE":
					operatorNamespaceEnv = &operatorPod.Spec.Containers[i].Env[j]
				}
			}
		}
		gomega.Expect(watchNamespaceEnv).NotTo(gomega.BeNil(), "operator pod is missing the WATCH_NAMESPACE env var")
		gomega.Expect(watchNamespaceEnv.ValueFrom).NotTo(gomega.BeNil())
		gomega.Expect(watchNamespaceEnv.ValueFrom.FieldRef).NotTo(gomega.BeNil())
		gomega.Expect(watchNamespaceEnv.ValueFrom.FieldRef.FieldPath).To(gomega.Equal("metadata.annotations['olm.targetNamespaces']"))

		gomega.Expect(operatorNamespaceEnv).NotTo(gomega.BeNil(), "operator pod is missing the OPERATOR_NAMESPACE env var")
		gomega.Expect(operatorNamespaceEnv.ValueFrom).NotTo(gomega.BeNil())
		gomega.Expect(operatorNamespaceEnv.ValueFrom.FieldRef).NotTo(gomega.BeNil())
		gomega.Expect(operatorNamespaceEnv.ValueFrom.FieldRef.FieldPath).To(gomega.Equal("metadata.namespace"))
	})

	ginkgo.It("should reconcile a DPA in a second namespace (multi-tenant)", func() {
		tenantNamespace := namespace + "-tenant-e2e"
		err := lib.CreateNamespace(kubernetesClientForSuiteRun, tenantNamespace)
		gomega.Expect(err).To(gomega.BeNil())

		bslCredFileData, err := lib.ReadFile(bslCredFile)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		err = lib.CreateCredentialsSecret(kubernetesClientForSuiteRun, bslCredFileData, tenantNamespace, bslSecretName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		tenantDpa := &lib.DpaCustomResource{
			Name:                 "ts-" + instanceName + "-tenant",
			Namespace:            tenantNamespace,
			Client:               runTimeClientForSuiteRun,
			BSLSecretName:        bslSecretName,
			BSLConfig:            dpaCR.BSLConfig,
			BSLProvider:          dpaCR.BSLProvider,
			BSLBucket:            dpaCR.BSLBucket,
			BSLBucketPrefix:      dpaCR.BSLBucketPrefix + "-tenant",
			VeleroDefaultPlugins: dpaCR.VeleroDefaultPlugins,
		}
		err = tenantDpa.CreateOrUpdate(tenantDpa.Build(lib.KOPIA))
		gomega.Expect(err).To(gomega.BeNil())

		// The operator (installed once, in the main namespace) must reconcile
		// this DPA in a different namespace: its own Velero deployment must
		// come up and validate the BSL against the bucket.
		gomega.Eventually(tenantDpa.IsReconciledTrue(), time.Minute*5, time.Second*5).Should(gomega.BeTrue())
		gomega.Eventually(tenantDpa.BSLsAreAvailable(), time.Minute*5, time.Second*10).Should(gomega.BeTrue())

		ginkgo.DeferCleanup(func() {
			err := tenantDpa.Delete()
			gomega.Expect(err).To(gomega.BeNil())
			gomega.Eventually(tenantDpa.IsDeleted(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			err = lib.DeleteNamespace(kubernetesClientForSuiteRun, tenantNamespace)
			if err != nil && !apierrors.IsNotFound(err) {
				log.Printf("failed to delete tenant namespace %s: %v", tenantNamespace, err)
			}
		})
	})
})

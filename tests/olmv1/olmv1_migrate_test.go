package olmv1_test

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
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	subscriptionGVR = schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "subscriptions",
	}
	csvGVR = schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "clusterserviceversions",
	}
	operatorGroupGVR = schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1",
		Resource: "operatorgroups",
	}
)

var _ = ginkgo.Describe("OADP OLMv0 to OLMv1 migration", ginkgo.Ordered, ginkgo.Label("olmv1-migrate"), func() {
	ctx := context.Background()

	ginkgo.BeforeAll(func() {
		if !migrate {
			ginkgo.Skip("Migration tests disabled (pass -migrate=true to enable)")
		}

		ginkgo.By("Verifying OLMv0 resources exist before migration")
		subs, err := dynamicClient.Resource(subscriptionGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil || len(subs.Items) == 0 {
			ginkgo.Skip(fmt.Sprintf("No OLMv0 Subscription found in %s — run 'make deploy-olm' first", namespace))
		}
		for _, sub := range subs.Items {
			log.Printf("Found OLMv0 Subscription: %s", sub.GetName())
		}
	})

	ginkgo.It("should remove OLMv0 Subscriptions", func() {
		subs, err := dynamicClient.Resource(subscriptionGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		for _, sub := range subs.Items {
			ginkgo.By(fmt.Sprintf("Deleting Subscription %s", sub.GetName()))
			err := dynamicClient.Resource(subscriptionGVR).Namespace(namespace).Delete(ctx, sub.GetName(), metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}

		gomega.Eventually(func() int {
			list, _ := dynamicClient.Resource(subscriptionGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
			return len(list.Items)
		}, 1*time.Minute, 5*time.Second).Should(gomega.Equal(0))
	})

	ginkgo.It("should remove OLMv0 CSVs", func() {
		csvs, err := dynamicClient.Resource(csvGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		for _, csv := range csvs.Items {
			name := csv.GetName()
			ginkgo.By(fmt.Sprintf("Deleting CSV %s", name))
			err := dynamicClient.Resource(csvGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			}
		}

		gomega.Eventually(func() int {
			list, _ := dynamicClient.Resource(csvGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
			return len(list.Items)
		}, 2*time.Minute, 5*time.Second).Should(gomega.Equal(0))
	})

	ginkgo.It("should remove OLMv0 OperatorGroup", func() {
		ogs, err := dynamicClient.Resource(operatorGroupGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		for _, og := range ogs.Items {
			ginkgo.By(fmt.Sprintf("Deleting OperatorGroup %s", og.GetName()))
			err := dynamicClient.Resource(operatorGroupGVR).Namespace(namespace).Delete(ctx, og.GetName(), metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			}
		}
	})

	ginkgo.It("should clean orphaned OADP/Velero CRDs", func() {
		ginkgo.By("Deleting orphaned CRDs that OLMv1 cannot adopt")
		cleanupOrphanedCRDs(ctx)
	})

	ginkgo.It("should install OADP via OLMv1 ClusterExtension", func() {
		ginkgo.By("Setting up installer ServiceAccount and RBAC")
		ensureNamespace(ctx, namespace)
		ensureServiceAccount(ctx, serviceAccountName, namespace)
		ensureClusterAdminBinding(ctx, serviceAccountName, namespace)

		if catalogImage != "" {
			ginkgo.By(fmt.Sprintf("Creating ClusterCatalog %s", catalogName))
			ensureClusterCatalog(ctx, catalogName, catalogImage)
			waitForClusterCatalogServing(ctx, catalogName)
		}

		ginkgo.By("Creating the ClusterExtension")
		ce := buildClusterExtension(packageName, packageName, namespace, serviceAccountName)
		_, err := dynamicClient.Resource(clusterExtensionGVR).Create(ctx, ce, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Waiting for ClusterExtension to be installed")
		terminalReasons := map[string]bool{
			"InvalidConfiguration": true,
			"Failed":               true,
		}
		gomega.Eventually(func(g gomega.Gomega) {
			obj, err := getClusterExtension(ctx, packageName)
			g.Expect(err).NotTo(gomega.HaveOccurred())

			log.Print("Current conditions:")
			logAllConditions(obj)

			progCond, progFound := getCondition(obj, "Progressing")
			if progFound {
				reason, _ := progCond["reason"].(string)
				message, _ := progCond["message"].(string)
				g.Expect(terminalReasons[reason]).NotTo(gomega.BeTrue(),
					"ClusterExtension has terminal error: reason=%s message=%s", reason, message)
			}

			instCond, instFound := getCondition(obj, "Installed")
			g.Expect(instFound).To(gomega.BeTrue(), "Installed condition should be present")
			status, _ := instCond["status"].(string)
			g.Expect(status).To(gomega.Equal("True"), "Installed condition should be True")
		}, 10*time.Minute, 10*time.Second).Should(gomega.Succeed())

		ginkgo.By("Checking installed bundle info")
		obj, err := getClusterExtension(ctx, packageName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		bundleName, bundleVersion, found := getInstalledBundle(obj)
		gomega.Expect(found).To(gomega.BeTrue())
		log.Printf("Installed bundle: name=%s version=%s", bundleName, bundleVersion)
	})

	ginkgo.It("should have controller-manager pod running after migration", func() {
		gomega.Eventually(func() (bool, error) {
			pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "control-plane=controller-manager",
			})
			if err != nil {
				return false, err
			}
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					log.Printf("Controller-manager pod %s is Running", pod.Name)
					return true, nil
				}
			}
			return false, nil
		}, 5*time.Minute, 10*time.Second).Should(gomega.BeTrue(), "controller-manager pod should be Running")
	})

	ginkgo.AfterAll(func() {
		if !migrate {
			return
		}
		ginkgo.By("Cleaning up migration test resources")
		_ = deleteClusterExtension(ctx, packageName)

		gomega.Eventually(func() bool {
			_, err := getClusterExtension(ctx, packageName)
			return apierrors.IsNotFound(err)
		}, 2*time.Minute, 5*time.Second).Should(gomega.BeTrue())

		if createdCatalog {
			deleteClusterCatalog(ctx, catalogName)
		}
		cleanupClusterRoleBinding(ctx, serviceAccountName)
	})
})

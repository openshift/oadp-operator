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
	"k8s.io/apimachinery/pkg/types"
)

const (
	clusterExtensionName = "oadp-operator"

	oadpCRDName    = "dataprotectionapplications.oadp.openshift.io"
	veleroCRDName  = "backups.velero.io"
	restoreCRDName = "restores.velero.io"

	managerLabelSelector = "control-plane=controller-manager"
)

var _ = ginkgo.Describe("OADP OLMv1 lifecycle", ginkgo.Ordered, ginkgo.Label("olmv1"), func() {
	ctx := context.Background()

	ginkgo.BeforeAll(func() {
		ginkgo.By("Cleaning up orphaned OADP/Velero CRDs from previous installs")
		cleanupOrphanedCRDs(ctx)

		ginkgo.By("Setting up namespace, ServiceAccount, and RBAC")
		ensureNamespace(ctx, namespace)
		ensureServiceAccount(ctx, serviceAccountName, namespace)
		ensureClusterAdminBinding(ctx, serviceAccountName, namespace)

		if catalogImage != "" {
			ginkgo.By(fmt.Sprintf("Creating ClusterCatalog %s from image %s", catalogName, catalogImage))
			ensureClusterCatalog(ctx, catalogName, catalogImage)
			waitForClusterCatalogServing(ctx, catalogName)
		}
	})

	ginkgo.AfterAll(func() {
		ginkgo.By("Cleaning up OLMv1 test resources")
		err := deleteClusterExtension(ctx, clusterExtensionName)
		if err != nil {
			log.Printf("Warning: failed to delete ClusterExtension: %v", err)
		}

		gomega.Eventually(func() bool {
			_, err := getClusterExtension(ctx, clusterExtensionName)
			return apierrors.IsNotFound(err)
		}, 3*time.Minute, 5*time.Second).Should(gomega.BeTrue(), "ClusterExtension should be deleted")

		if createdCatalog {
			ginkgo.By(fmt.Sprintf("Deleting ClusterCatalog %s", catalogName))
			deleteClusterCatalog(ctx, catalogName)
		}

		cleanupClusterRoleBinding(ctx, serviceAccountName)
	})

	ginkgo.It("should install OADP operator via ClusterExtension", func() {
		ginkgo.By("Creating the ClusterExtension")
		ce := buildClusterExtension(clusterExtensionName, packageName, namespace, serviceAccountName)
		_, err := dynamicClient.Resource(clusterExtensionGVR).Create(ctx, ce, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		log.Printf("Created ClusterExtension %s (package=%s, namespace=%s)", clusterExtensionName, packageName, namespace)

		ginkgo.By("Waiting for ClusterExtension to be installed")
		terminalReasons := map[string]bool{
			"InvalidConfiguration": true,
			"Failed":               true,
		}
		gomega.Eventually(func(g gomega.Gomega) {
			obj, err := getClusterExtension(ctx, clusterExtensionName)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "ClusterExtension should exist")

			logAllConditions(obj)

			progCond, progFound := getCondition(obj, "Progressing")
			if progFound {
				reason, _ := progCond["reason"].(string)
				message, _ := progCond["message"].(string)
				g.Expect(terminalReasons[reason]).NotTo(gomega.BeTrue(),
					"ClusterExtension has terminal error on Progressing: reason=%s message=%s", reason, message)
			}

			instCond, instFound := getCondition(obj, "Installed")
			g.Expect(instFound).To(gomega.BeTrue(), "Installed condition should be present")
			status, _ := instCond["status"].(string)
			g.Expect(status).To(gomega.Equal("True"), "Installed condition should be True")
		}, 10*time.Minute, 10*time.Second).Should(gomega.Succeed())

		ginkgo.By("Checking installed bundle info")
		obj, err := getClusterExtension(ctx, clusterExtensionName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		bundleName, bundleVersion, found := getInstalledBundle(obj)
		gomega.Expect(found).To(gomega.BeTrue(), "installed bundle should be present in status")
		log.Printf("Installed bundle: name=%s version=%s", bundleName, bundleVersion)
	})

	ginkgo.It("should have the OADP controller-manager pod running", func() {
		ginkgo.By("Waiting for controller-manager pod to be Running")
		gomega.Eventually(func() (bool, error) {
			pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: managerLabelSelector,
			})
			if err != nil {
				return false, err
			}
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					log.Printf("Controller-manager pod %s is Running", pod.Name)
					return true, nil
				}
				log.Printf("Controller-manager pod %s phase: %s", pod.Name, pod.Status.Phase)
			}
			return false, nil
		}, 5*time.Minute, 10*time.Second).Should(gomega.BeTrue(), "controller-manager pod should be Running")
	})

	ginkgo.It("should have OADP CRDs installed", func() {
		expectedCRDs := []string{
			oadpCRDName,
			veleroCRDName,
			restoreCRDName,
			"schedules.velero.io",
			"backupstoragelocations.velero.io",
			"volumesnapshotlocations.velero.io",
		}

		for _, crdName := range expectedCRDs {
			ginkgo.By(fmt.Sprintf("Checking CRD %s exists", crdName))
			exists, err := crdExists(ctx, crdName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(exists).To(gomega.BeTrue(), fmt.Sprintf("CRD %s should exist", crdName))
			log.Printf("CRD %s exists", crdName)
		}
	})

	ginkgo.It("should not report deprecation warnings", func() {
		obj, err := getClusterExtension(ctx, clusterExtensionName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		for _, condType := range []string{"Deprecated", "PackageDeprecated", "ChannelDeprecated", "BundleDeprecated"} {
			cond, found := getCondition(obj, condType)
			if found {
				status, _ := cond["status"].(string)
				gomega.Expect(status).To(gomega.Equal("False"),
					fmt.Sprintf("%s condition should be False, got %s", condType, status))
			}
		}
	})

	ginkgo.When("upgrading the operator", func() {
		ginkgo.BeforeAll(func() {
			if upgradeVersion == "" {
				ginkgo.Skip("No --upgrade-version specified, skipping upgrade tests")
			}
		})

		ginkgo.It("should upgrade the ClusterExtension to the target version", func() {
			ginkgo.By(fmt.Sprintf("Patching ClusterExtension version to %s", upgradeVersion))
			obj, err := getClusterExtension(ctx, clusterExtensionName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			previousBundleName, previousVersion, _ := getInstalledBundle(obj)
			log.Printf("Current installed bundle: name=%s version=%s", previousBundleName, previousVersion)

			patch := []byte(fmt.Sprintf(`{"spec":{"source":{"catalog":{"version":"%s","upgradeConstraintPolicy":"SelfCertified"}}}}`, upgradeVersion))
			_, err = dynamicClient.Resource(clusterExtensionGVR).Patch(ctx, clusterExtensionName, types.MergePatchType, patch, metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			log.Printf("Patched ClusterExtension version to %s", upgradeVersion)

			ginkgo.By("Waiting for upgrade to complete")
			gomega.Eventually(func() string {
				updated, err := getClusterExtension(ctx, clusterExtensionName)
				if err != nil {
					return ""
				}

				cond, found := getCondition(updated, "Installed")
				if !found {
					return ""
				}
				status, _ := cond["status"].(string)
				if status != "True" {
					reason, _ := cond["reason"].(string)
					message, _ := cond["message"].(string)
					log.Printf("Installed condition: status=%s reason=%s message=%s", status, reason, message)
					return ""
				}

				_, bundleVer, found := getInstalledBundle(updated)
				if !found {
					return ""
				}
				log.Printf("Installed bundle version: %s", bundleVer)
				return bundleVer
			}, 10*time.Minute, 10*time.Second).ShouldNot(gomega.Equal(previousVersion),
				"Installed bundle version should change after upgrade")

			ginkgo.By("Verifying controller-manager pod is running after upgrade")
			gomega.Eventually(func() (bool, error) {
				pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
					LabelSelector: managerLabelSelector,
				})
				if err != nil {
					return false, err
				}
				for _, pod := range pods.Items {
					if pod.Status.Phase == corev1.PodRunning {
						return true, nil
					}
				}
				return false, nil
			}, 5*time.Minute, 10*time.Second).Should(gomega.BeTrue())
		})
	})
})


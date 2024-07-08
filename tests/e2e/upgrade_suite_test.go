package e2e_test

import (
	"context"
	"log"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/operator-framework/api/pkg/operators/v1"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

type channelUpgradeCase struct {
	previous string
	next     string
}

// TODO break in smaller PRs (refactor) and last one being upgrade test one

var _ = ginkgo.Describe("OADP upgrade scenarios", ginkgo.Ordered, func() {
	ginkgo.DescribeTable("Upgrade OADP channel tests",
		func(scenario channelUpgradeCase) {
			// Create operatorGroup and subscription with previous channel stable-1.3
			log.Print("Checking if OperatorGroup needs to be created")
			operatorGroupList := v1.OperatorGroupList{}
			err := runTimeClientForSuiteRun.List(context.Background(), &operatorGroupList, client.InNamespace(namespace))
			gomega.Expect(err).To(gomega.BeNil())
			gomega.Expect(len(operatorGroupList.Items) > 1).To(gomega.BeFalse())
			if len(operatorGroupList.Items) == 0 {
				log.Print("Creating OperatorGroup oadp-operator-group")
				operatorGroup := v1.OperatorGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "oadp-operator-group",
						Namespace: namespace,
					},
					Spec: v1.OperatorGroupSpec{
						TargetNamespaces: []string{namespace},
					},
				}
				err = runTimeClientForSuiteRun.Create(context.Background(), &operatorGroup)
				gomega.Expect(err).To(gomega.BeNil())
			}

			log.Print("Creating Subscription oadp-operator")
			subscription := v1alpha1.Subscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-operator",
					Namespace: namespace,
				},
				Spec: &v1alpha1.SubscriptionSpec{
					CatalogSource:          "oadp-operator-catalog-test-upgrade",
					CatalogSourceNamespace: "openshift-marketplace",
					Package:                "oadp-operator",
					Channel:                scenario.previous,
					InstallPlanApproval:    v1alpha1.ApprovalAutomatic,
				},
			}
			err = runTimeClientForSuiteRun.Create(context.Background(), &subscription)
			gomega.Expect(err).To(gomega.BeNil())

			// Check that after 5 minutes csv oadp-operator.v1.3.0 has status.phase Succeeded
			log.Print("Checking if previous channel CSV has status.phase Succeeded")
			subscriptionHelper := lib.Subscription{Subscription: &subscription}
			gomega.Eventually(subscriptionHelper.CsvIsReady(runTimeClientForSuiteRun), time.Minute*5, time.Second*5).Should(gomega.BeTrue())

			// create DPA after controller-manager Pod is running
			gomega.Eventually(lib.ManagerPodIsUp(kubernetesClientForSuiteRun, namespace), time.Minute*8, time.Second*15).Should(gomega.BeTrue())
			log.Print("Creating DPA")
			dpaCR.CustomResource = &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dpaCR.Name,
					Namespace: dpaCR.Namespace,
				},
				Spec: oadpv1alpha1.DataProtectionApplicationSpec{
					Configuration: &oadpv1alpha1.ApplicationConfig{
						Velero: &oadpv1alpha1.VeleroConfig{
							LogLevel:       "debug",
							DefaultPlugins: append(dpaCR.CustomResource.Spec.Configuration.Velero.DefaultPlugins, oadpv1alpha1.DefaultPluginCSI),
							FeatureFlags:   append(dpaCR.CustomResource.Spec.Configuration.Velero.FeatureFlags, velerov1.CSIFeatureFlag),
						},
						NodeAgent: &oadpv1alpha1.NodeAgentConfig{
							UploaderType: "kopia",
							NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
								PodConfig: &oadpv1alpha1.PodConfig{},
								Enable:    ptr.To(false),
							},
						},
					},
					BackupLocations: []oadpv1alpha1.BackupLocation{
						{
							Velero: &velerov1.BackupStorageLocationSpec{
								Provider: dpaCR.CustomResource.Spec.BackupLocations[0].Velero.Provider,
								Default:  true,
								Config:   dpaCR.CustomResource.Spec.BackupLocations[0].Velero.Config,
								Credential: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: bslSecretName,
									},
									Key: "cloud",
								},
								StorageType: velerov1.StorageType{
									ObjectStorage: &velerov1.ObjectStorageLocation{
										Bucket: dpaCR.CustomResource.Spec.BackupLocations[0].Velero.ObjectStorage.Bucket,
										Prefix: lib.VeleroPrefix,
									},
								},
							},
						},
					},
				},
			}
			err = dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, &dpaCR.CustomResource.Spec)
			gomega.Expect(err).To(gomega.BeNil())

			// check that DPA is reconciled
			log.Print("Checking if DPA is reconciled")
			gomega.Eventually(dpaCR.IsReconciled(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			// check that velero pod is running
			log.Print("Checking if velero pod is running")
			gomega.Eventually(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			// check if BSL is available
			log.Print("Checking if BSL is available")
			gomega.Eventually(dpaCR.BSLsAreAvailable(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			// Velero api changes:
			// check that BSL had checksumAlgorithm not set
			bsls, err := dpaCR.ListBSLs()
			gomega.Expect(err).To(gomega.BeNil())
			_, ok := bsls.Items[0].Spec.Config["checksumAlgorithm"]
			gomega.Expect(ok).To(gomega.BeFalse())
			// check that velero Pod had 3 init containers (aws, openshift, csi)
			velero, err := lib.GetVeleroPod(kubernetesClientForSuiteRun, namespace)
			gomega.Expect(err).To(gomega.BeNil())
			gomega.Expect(len(velero.Spec.InitContainers)).To(gomega.Equal(3))

			// TODO backup/restore

			// Update spec.channel in subscription to stable-1.4
			log.Print("Updating Subscription oadp-operator spec.channel")
			subscription.Spec.Channel = scenario.next
			err = runTimeClientForSuiteRun.Update(context.Background(), &subscription)
			gomega.Expect(err).To(gomega.BeNil())

			// TODO Check that after X minutes csv oadp-operator.v1.3.0 has status.phase Replacing and its deleted

			// Check that after 8 minutes csv oadp-operator.v1.4.0 has status.phase Installing -> Succeeded
			log.Print("Waiting for next channel CSV to be created")
			gomega.Eventually(subscriptionHelper.CsvIsInstalling(runTimeClientForSuiteRun), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			log.Print("Checking if next channel CSV has status.phase Succeeded")
			gomega.Eventually(subscriptionHelper.CsvIsReady(runTimeClientForSuiteRun), time.Minute*5, time.Second*5).Should(gomega.BeTrue())

			timeAfterUpgrade := time.Now()

			// check DPA after controller-manager Pod is running
			gomega.Eventually(lib.ManagerPodIsUp(kubernetesClientForSuiteRun, namespace), time.Minute*8, time.Second*15).Should(gomega.BeTrue())

			// check if updated DPA is reconciled
			log.Print("Checking if DPA was reconciled after update")
			// TODO gomega.Eventually(dpaCR.IsUpdated, time.Minute*3, time.Second*5).WithArguments(timeAfterUpgrade).Should(gomega.BeTrue())
			// TODO gomega.Eventually(dpaCR.IsReconciled(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			gomega.Consistently(dpaCR.IsReconciled(), time.Minute*3, time.Second*15).Should(gomega.BeTrue())

			// check if updated velero pod is running
			log.Print("Checking if velero pod was recreated after update")
			gomega.Eventually(lib.VeleroPodIsUpdated(kubernetesClientForSuiteRun, namespace, timeAfterUpgrade), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			log.Print("Checking if velero pod is running")
			gomega.Eventually(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			timeAfterVeleroIsRunning := time.Now()

			// check if updated BSL is available
			log.Print("Checking if BSL was reconciled after update")
			gomega.Eventually(dpaCR.BSLsAreUpdated(timeAfterVeleroIsRunning), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			log.Print("Checking if BSL is available")
			gomega.Eventually(dpaCR.BSLsAreAvailable(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			// No OADP api changes

			// Velero api changes:
			// check that BSL has checksumAlgorithm set to empty
			bsls, err = dpaCR.ListBSLs()
			gomega.Expect(err).To(gomega.BeNil())
			value, ok := bsls.Items[0].Spec.Config["checksumAlgorithm"]
			gomega.Expect(ok).To(gomega.BeTrue())
			gomega.Expect(value).To(gomega.Equal(""))
			// check that velero Pod has 2 init containers (aws, openshift)
			velero, err = lib.GetVeleroPod(kubernetesClientForSuiteRun, namespace)
			gomega.Expect(err).To(gomega.BeNil())
			gomega.Expect(len(velero.Spec.InitContainers)).To(gomega.Equal(2))
			// TODO check that CSI works after code integration
		},
		ginkgo.Entry("Upgrade from stable-1.3 (oadp-1.3 branch) to stable-1.4 (oadp-1.4 branch) channel", ginkgo.Label("upgrade", "aws", "ibmcloud"), channelUpgradeCase{
			previous: "stable-1.3",
			next:     "stable-1.4",
		}),
	)
})

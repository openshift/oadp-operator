package e2e_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	operatorv1 "github.com/operator-framework/api/pkg/operators/v1"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

type channelUpgradeCase struct {
	previous   string
	next       string
	production bool
}

var _ = ginkgo.Describe("OADP upgrade scenarios", ginkgo.Ordered, func() {
	ginkgo.DescribeTable("Upgrade OADP channel tests",
		func(scenario channelUpgradeCase) {
			// Create OperatorGroup and Subscription with previous channel stable-1.4
			log.Print("Checking if OperatorGroup needs to be created")
			operatorGroupList := operatorv1.OperatorGroupList{}
			err := runTimeClientForSuiteRun.List(context.Background(), &operatorGroupList, client.InNamespace(namespace))
			gomega.Expect(err).To(gomega.BeNil())
			gomega.Expect(len(operatorGroupList.Items) > 1).To(gomega.BeFalse())
			if len(operatorGroupList.Items) == 0 {
				log.Print("Creating OperatorGroup oadp-operator-group")
				operatorGroup := operatorv1.OperatorGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "oadp-operator-group",
						Namespace: namespace,
					},
					Spec: operatorv1.OperatorGroupSpec{
						TargetNamespaces: []string{namespace},
					},
				}
				err = runTimeClientForSuiteRun.Create(context.Background(), &operatorGroup)
				gomega.Expect(err).To(gomega.BeNil())
			}

			subscriptionPackage := "oadp-operator"
			subscriptionSource := "oadp-operator-catalog-test-upgrade"
			if scenario.production {
				subscriptionPackage = "redhat-oadp-operator"
				subscriptionSource = "redhat-operators"
			}

			log.Print("Creating Subscription oadp-operator")
			subscription := v1alpha1.Subscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oadp-operator",
					Namespace: namespace,
				},
				Spec: &v1alpha1.SubscriptionSpec{
					Package:                subscriptionPackage,
					CatalogSource:          subscriptionSource,
					Channel:                scenario.previous,
					CatalogSourceNamespace: "openshift-marketplace",
					InstallPlanApproval:    v1alpha1.ApprovalAutomatic,
				},
			}
			err = runTimeClientForSuiteRun.Create(context.Background(), &subscription)
			gomega.Expect(err).To(gomega.BeNil())

			// Check that after 5 minutes ClusterServiceVersion oadp-operator.v1.4.0 has status.phase Succeeded
			log.Print("Checking if previous channel CSV has status.phase Succeeded")
			subscriptionHelper := lib.Subscription{Subscription: &subscription}
			gomega.Eventually(subscriptionHelper.CsvIsReady(runTimeClientForSuiteRun), time.Minute*5, time.Second*5).Should(gomega.BeTrue())

			// create DPA after controller-manager Pod is running
			gomega.Eventually(lib.ManagerPodIsUp(kubernetesClientForSuiteRun, namespace), time.Minute*8, time.Second*15).Should(gomega.BeTrue())
			log.Print("Creating DPA")
			dpaSpec := &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						LogLevel:       "debug",
						DefaultPlugins: dpaCR.VeleroDefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
							Enable: ptr.To(true),
						},
					},
				},
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velerov1.BackupStorageLocationSpec{
							Provider: dpaCR.BSLProvider,
							Default:  true,
							Config:   dpaCR.BSLConfig,
							Credential: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: bslSecretName,
								},
								Key: "cloud",
							},
							StorageType: velerov1.StorageType{
								ObjectStorage: &velerov1.ObjectStorageLocation{
									Bucket: dpaCR.BSLBucket,
									Prefix: dpaCR.BSLBucketPrefix,
								},
							},
						},
					},
				},
			}
			err = dpaCR.CreateOrUpdate(dpaSpec)
			gomega.Expect(err).To(gomega.BeNil())

			// check that DPA is reconciled true
			log.Print("Checking if DPA is reconciled true")
			gomega.Eventually(dpaCR.IsReconciledTrue(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			// check that Velero Pod is running
			log.Print("Checking if Velero Pod is running")
			gomega.Eventually(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			// check that NodeAgent Pods are running
			log.Printf("Checking if Node Agent Pods are running")
			gomega.Eventually(lib.AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			// check if BSL is available
			log.Print("Checking if BSL is available")
			gomega.Eventually(dpaCR.BSLsAreAvailable(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			// TODO Velero api changes

			// TODO OADP api changes

			// TODO backup/restore

			// Update spec.channel in Subscription to stable
			log.Print("Updating Subscription oadp-operator spec.channel")
			err = runTimeClientForSuiteRun.Get(context.Background(), types.NamespacedName{Namespace: subscription.Namespace, Name: subscription.Name}, &subscription)
			gomega.Expect(err).To(gomega.BeNil())
			subscription.Spec.Channel = scenario.next
			err = runTimeClientForSuiteRun.Update(context.Background(), &subscription)
			gomega.Expect(err).To(gomega.BeNil())

			// Check that after 8 minutes ClusterServiceVersion oadp-operator.v99.0.0 has status.phase Installing -> Succeeded
			log.Print("Waiting for next channel CSV to be created")
			gomega.Eventually(subscriptionHelper.CsvIsInstalling(runTimeClientForSuiteRun), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			log.Print("Checking if next channel CSV has status.phase Succeeded")
			gomega.Eventually(subscriptionHelper.CsvIsReady(runTimeClientForSuiteRun), time.Minute*5, time.Second*5).Should(gomega.BeTrue())

			timeAfterUpgrade := time.Now()

			// check DPA after controller-manager Pod is running
			gomega.Eventually(lib.ManagerPodIsUp(kubernetesClientForSuiteRun, namespace), time.Minute*8, time.Second*15).Should(gomega.BeTrue())

			// check if updated DPA is reconciled
			log.Print("Checking if DPA was reconciled after update")
			gomega.Eventually(dpaCR.IsReconciledFalse("Delete restic object from spec.configuration, use spec.configuration.nodeAgent instead"), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			log.Print("Updating DPA")
			dpaSpec.Configuration.Restic = nil
			dpaSpec.Configuration.NodeAgent = &oadpv1alpha1.NodeAgentConfig{
				UploaderType: "restic",
				NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
					Enable: ptr.To(true),
				},
			}
			err = dpaCR.CreateOrUpdate(dpaSpec)
			gomega.Expect(err).To(gomega.BeNil())

			// check if updated DPA is reconciled
			log.Print("Checking if DPA was reconciled after update")
			// TODO do not use Consistently, using because no field in DPA is updated telling when it was last reconciled
			gomega.Consistently(dpaCR.IsReconciledTrue(), time.Minute*3, time.Second*15).Should(gomega.BeTrue())

			// check if updated Velero Pod is running
			log.Print("Checking if Velero Pod was recreated after update")
			gomega.Eventually(lib.VeleroPodIsUpdated(kubernetesClientForSuiteRun, namespace, timeAfterUpgrade), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			log.Print("Checking if Velero Pod is running")
			gomega.Eventually(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			timeAfterVeleroIsRunning := time.Now()

			// check if updated NodeAgent Pods are running
			log.Print("Checking if Node Agent Pods were recreated after update")
			gomega.Eventually(func() (bool, error) {
				nodeAgentDaemonSet, err := lib.GetNodeAgentDaemonSet(kubernetesClientForSuiteRun, namespace)
				if err != nil {
					return false, err
				}

				numScheduled := nodeAgentDaemonSet.Status.CurrentNumberScheduled
				numDesired := nodeAgentDaemonSet.Status.DesiredNumberScheduled
				// check correct number of NodeAgent Pods are initialized
				if numScheduled != numDesired {
					return false, fmt.Errorf("wrong number of Node Agent Pods")
				}

				podList, err := lib.GetAllPodsWithLabel(kubernetesClientForSuiteRun, namespace, "name=node-agent")
				if err != nil {
					return false, err
				}
				if err != nil {
					return false, err
				}
				for _, pod := range podList.Items {
					if !pod.CreationTimestamp.After(timeAfterUpgrade) {
						return false, fmt.Errorf("not all Node Agent Pods were updated")
					}
				}
				return true, nil
			}, time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			log.Printf("Checking if Node Agent Pods are running")
			gomega.Eventually(lib.AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			// check if updated BSL is available
			log.Print("Checking if BSL was reconciled after update")
			gomega.Eventually(dpaCR.BSLsAreUpdated(timeAfterVeleroIsRunning), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			log.Print("Checking if BSL is available")
			gomega.Eventually(dpaCR.BSLsAreAvailable(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			// TODO Velero api changes

			// TODO OADP api changes

			// TODO backup/restore
		},
		ginkgo.Entry("Upgrade from stable-1.4 (oadp-1.4 branch) to stable (master branch) channel", ginkgo.Label("upgrade"), channelUpgradeCase{
			previous: "stable-1.4",
			next:     "stable",
			// to test production
			// production: true,
		}),
	)
})

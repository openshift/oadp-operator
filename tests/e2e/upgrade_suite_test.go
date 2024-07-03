package e2e_test

import (
	"context"
	"log"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/operator-framework/api/pkg/operators/v1"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

type channelUpgradeCase struct {
	previous string
	next     string
}

var _ = ginkgo.Describe("OADP upgrade scenarios", ginkgo.Ordered, func() {
	// TODO do any clean up?
	// var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
	// 	tearDownCatalog()
	// })

	ginkgo.DescribeTable("Upgrade OADP channel tests",
		func(scenario channelUpgradeCase) {
			// Avoid DPA cleanup error on AfterSuite of upgrade only tests
			dpaCR.CustomResource.Name = "dummyDPA"
			dpaCR.CustomResource.Namespace = namespace

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
			gomega.Eventually(subscriptionHelper.CsvIsReady, time.Minute*5, time.Second*5).WithArguments(runTimeClientForSuiteRun).Should(gomega.BeTrue())

			// TODO check leaderelection then create DPA

			// TODO backup/restore

			// Update spec.channel in subscription to stable-1.4
			log.Print("Updating Subscription oadp-operator spec.channel")
			subscription.Spec.Channel = scenario.next
			err = runTimeClientForSuiteRun.Update(context.Background(), &subscription)
			gomega.Expect(err).To(gomega.BeNil())

			// TODO Check that after X minutes csv oadp-operator.v1.3.0 has status.phase Replacing and its deleted

			// Check that after 8 minutes csv oadp-operator.v1.4.0 has status.phase Installing -> Succeeded
			log.Print("Waiting for next channel CSV to be created")
			gomega.Eventually(subscriptionHelper.CsvIsInstalling, time.Minute*3, time.Second*5).WithArguments(runTimeClientForSuiteRun).Should(gomega.BeTrue())
			log.Print("Checking if next channel CSV has status.phase Succeeded")
			gomega.Eventually(subscriptionHelper.CsvIsReady, time.Minute*5, time.Second*5).WithArguments(runTimeClientForSuiteRun).Should(gomega.BeTrue())

			// TODO check leaderelection then check DPA

			// No OADP api changes

			// TODO Velero api changes:
			// - check that BSL has checksumAlgorithm set to empty
			// - check that CSI works after code integration
		},
		ginkgo.Entry("Upgrade from stable-1.3 (oadp-1.3 branch) to stable-1.4 (oadp-1.4 branch) channel", ginkgo.Label("upgrade", "aws", "ibmcloud"), channelUpgradeCase{
			previous: "stable-1.3",
			next:     "stable-1.4",
		}),
	)
})

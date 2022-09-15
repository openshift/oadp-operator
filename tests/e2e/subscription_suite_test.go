package e2e_test

import (
	"context"
	"fmt"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	utils "github.com/openshift/oadp-operator/tests/e2e/utils"
	operators "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("Subscription Config Suite Test", func() {
	var _ = BeforeEach(func() {
		log.Printf("Building dpaSpec")
		err := dpaCR.Build(CSI)
		Expect(err).NotTo(HaveOccurred())
		//also test restic
		dpaCR.CustomResource.Spec.Configuration.Restic.Enable = pointer.BoolPtr(true)

		err = dpaCR.Delete()
		Expect(err).ToNot(HaveOccurred())
		Eventually(dpaCR.IsDeleted(), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())

		testSuiteInstanceName := "ts-" + instanceName
		dpaCR.Name = testSuiteInstanceName

		credData, err := utils.ReadFile(credFile)
		Expect(err).NotTo(HaveOccurred())

		err = CreateCredentialsSecret(credData, namespace, GetSecretRef(credSecretRef))
		Expect(err).NotTo(HaveOccurred())
	})

	var _ = AfterEach(func() {
		err := dpaCR.Delete()
		Expect(err).ToNot(HaveOccurred())
	})
	type SubscriptionConfigTestCase struct {
		operators.SubscriptionConfig
		failureExpected *bool
		stream          string
	}
	DescribeTable("Proxy test table",
		func(testCase SubscriptionConfigTestCase) {
			log.Printf("Getting Operator Subscription")
			log.Printf(stream)
			s, err := dpaCR.GetOperatorSubscription(stream)
			Expect(err).To(BeNil())
			log.Printf("Setting test case subscription config")
			s.Spec.Config = &testCase.SubscriptionConfig
			log.Printf("Updating Subscription")
			err = dpaCR.Client.Update(context.Background(), s.Subscription)
			Expect(err).To(BeNil())

			// get csv from installplan from subscription
			log.Printf("Wait for CSV to be succeeded")
			if testCase.failureExpected != nil && *testCase.failureExpected {
				Consistently(s.CsvIsReady, time.Minute*2).Should(BeFalse())
			} else {
				Eventually(s.CsvIsReady, time.Minute*9).Should(BeTrue())

				log.Printf("CreatingOrUpdate test Velero")
				err = dpaCR.CreateOrUpdate(&dpaCR.CustomResource.Spec)
				Expect(err).NotTo(HaveOccurred())

				log.Printf("Getting velero object")
				velero, err := dpaCR.Get()
				Expect(err).NotTo(HaveOccurred())
				log.Printf("Waiting for velero pod to be running")
				Eventually(AreVeleroPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				if velero.Spec.Configuration.Restic.Enable != nil && *velero.Spec.Configuration.Restic.Enable {
					log.Printf("Waiting for restic pods to be running")
					Eventually(AreResticPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
				if velero.BackupImages() {
					log.Printf("Waiting for registry pods to be running")
					Eventually(AreRegistryDeploymentsAvailable(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
				if s.Spec.Config != nil && s.Spec.Config.Env != nil {
					// get pod env vars
					log.Printf("Getting velero pods")
					podList, err := GetVeleroPods(namespace)
					Expect(err).NotTo(HaveOccurred())
					log.Printf("Getting pods containers env vars")
					bl := dpaCR.CustomResource.Spec.BackupLocations[0]
					for _, podInfo := range podList.Items {
						// we care about pods that have labels control-plane=controller-manager, component=velero, "component": "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry",
						if podInfo.Labels["control-plane"] == "controller-manager" ||
							podInfo.Labels["app.kubernetes.io/name"] == "velero" ||
							podInfo.Labels["component"] == "oadp-"+fmt.Sprintf("%s-%d", dpaCR.Name, 1)+"-"+bl.Velero.Provider+"-registry" {
							log.Printf("Checking env vars are passed to each container in " + podInfo.Name)
							for _, container := range podInfo.Spec.Containers {
								log.Printf("Checking env vars are passed to container " + container.Name)
								for _, env := range s.Spec.Config.Env {
									Expect(container.Env).To(ContainElement(env))
								}
							}
						}
					}
				}
				log.Printf("Deleting test Velero")
				err = dpaCR.Delete()
				Expect(err).ToNot(HaveOccurred())
			}

		},
		Entry("HTTP_PROXY set", SubscriptionConfigTestCase{
			SubscriptionConfig: operators.SubscriptionConfig{
				Env: []corev1.EnvVar{
					{
						Name:  "HTTP_PROXY",
						Value: "http://proxy.example.com:8080",
					},
				},
			},
		}),
		Entry("NO_PROXY set", SubscriptionConfigTestCase{
			SubscriptionConfig: operators.SubscriptionConfig{
				Env: []corev1.EnvVar{
					{
						Name:  "NO_PROXY",
						Value: "1.1.1.1",
					},
				},
			},
		}),
		Entry("HTTPS_PROXY set", SubscriptionConfigTestCase{
			SubscriptionConfig: operators.SubscriptionConfig{
				Env: []corev1.EnvVar{
					{
						Name:  "HTTPS_PROXY",
						Value: "localhost",
					},
				},
			},
			// Failure is expected because localhost is not a valid https proxy and manager container will fail setup
			failureExpected: pointer.Bool(true),
		}),
		// Leave this as last entry to reset config
		Entry("Config unset", SubscriptionConfigTestCase{
			SubscriptionConfig: operators.SubscriptionConfig{},
		}),
	)
})

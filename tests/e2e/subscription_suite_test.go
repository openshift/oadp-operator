package e2e

import (
	"context"
	"fmt"
	"log"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	operators "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("Subscription Config Suite Test", func() {
	var _ = BeforeEach(func() {
		log.Printf("Building veleroSpec")
		err := vel.Build(csi)
		Expect(err).NotTo(HaveOccurred())
		//also test restic
		vel.CustomResource.Spec.Configuration.Restic.Enable = pointer.BoolPtr(true)

		err = vel.Delete()
		Expect(err).ToNot(HaveOccurred())
		Eventually(vel.IsDeleted(), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())

		testSuiteInstanceName := "ts-" + instanceName
		vel.Name = testSuiteInstanceName

		credData, err := readFile(cloud)
		Expect(err).NotTo(HaveOccurred())

		err = createCredentialsSecret(credData, namespace, getSecretRef(credSecretRef))
		Expect(err).NotTo(HaveOccurred())
	})

	var _ = AfterEach(func() {
		err := vel.Delete()
		Expect(err).ToNot(HaveOccurred())
	})
	type SubscriptionConfigTestCase struct {
		operators.SubscriptionConfig
		failureExpected *bool
	}
	DescribeTable("Proxy test table",
		func(testCase SubscriptionConfigTestCase) {
			log.Printf("Getting Operator Subscription")
			s, err := vel.getOperatorSubscription()
			Expect(err).To(BeNil())
			log.Printf("Setting test case subscription config")
			s.Spec.Config = &testCase.SubscriptionConfig
			log.Printf("Updating Subscription")
			err = vel.Client.Update(context.Background(), s.Subscription)
			Expect(err).To(BeNil())

			// get csv from installplan from subscription
			log.Printf("Wait for CSV to be succeeded")
			if testCase.failureExpected != nil && *testCase.failureExpected {
				Consistently(s.csvIsReady, time.Minute*2).Should(BeFalse())
			} else {
				Eventually(s.csvIsReady, time.Minute*9).Should(BeTrue())

				log.Printf("CreatingOrUpdate test Velero")
				err = vel.CreateOrUpdate(&vel.CustomResource.Spec)
				Expect(err).NotTo(HaveOccurred())

				log.Printf("Getting velero object")
				velero, err := vel.Get()
				Expect(err).NotTo(HaveOccurred())
				log.Printf("Waiting for velero pod to be running")
				Eventually(areVeleroPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				if velero.Spec.Configuration.Restic.Enable != nil && *velero.Spec.Configuration.Restic.Enable {
					log.Printf("Waiting for restic pods to be running")
					Eventually(areResticPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
				if velero.Spec.BackupImages == nil || *velero.Spec.BackupImages {
					log.Printf("Waiting for registry pods to be running")
					Eventually(areRegistryDeploymentsAvailable(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
				if s.Spec.Config != nil && s.Spec.Config.Env != nil {
					// get pod env vars
					log.Printf("Getting velero pods")
					podList, err := getVeleroPods(namespace)
					Expect(err).NotTo(HaveOccurred())
					log.Printf("Getting pods containers env vars")
					bl := vel.CustomResource.Spec.BackupLocations[0]
					for _, podInfo := range podList.Items {
						// we care about pods that have labels control-plane=controller-manager, component=velero, "component": "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry",
						if podInfo.Labels["control-plane"] == "controller-manager" ||
							podInfo.Labels["app.kubernetes.io/name"] == "velero" ||
							podInfo.Labels["component"] == "oadp-"+fmt.Sprintf("%s-%d", vel.Name, 1)+"-"+bl.Velero.Provider+"-registry" {
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
				err = vel.Delete()
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

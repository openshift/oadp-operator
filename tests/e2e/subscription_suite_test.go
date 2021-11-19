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
)

var _ = Describe("Subscription Config Suite Test", func() {
	var _ = BeforeEach(func() {
		testSuiteInstanceName := "ts-" + instanceName
		vel.Name = testSuiteInstanceName

		credData, err := getCredsData(cloud)
		Expect(err).NotTo(HaveOccurred())

		err = createCredentialsSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())
	})

	var _ = AfterEach(func() {
		err := vel.Delete()
		Expect(err).ToNot(HaveOccurred())
	})
	type SubscriptionConfigTestCase struct {
		operators.SubscriptionConfig
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

			// get installplan from subscription

			// get csv from installplan from subscription
			log.Printf("Wait for CSV to be succeeded")
			Eventually(s.csvIsReady, "2m").Should(BeTrue())
			log.Printf("Wait for CSV status.phase to be succeeded")
			log.Printf("Waiting for InstallPlanPending condition to be false")

			log.Printf("Building veleroSpec")
			err = vel.Build(csi)
			Expect(err).NotTo(HaveOccurred())

			log.Printf("CreatingOrUpdate test Velero")
			err = vel.CreateOrUpdate(&vel.CustomResource.Spec)
			Expect(err).NotTo(HaveOccurred())
			
			log.Printf("Checking config environment variables are passed to each container in each pod")

			log.Printf("Getting velero object")
			velero, err := vel.Get()
			Expect(err).NotTo(HaveOccurred())
			log.Printf("Waiting for velero pod to be running")
			Eventually(isVeleroPodRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			if *velero.Spec.EnableRestic {
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
				bsl := vel.CustomResource.Spec.BackupStorageLocations[0]
				for _, podInfo := range podList.Items {
					// we care about pods that have labels control-plane=controller-manager, component=velero, "component": "oadp-" + bsl.Name + "-" + bsl.Spec.Provider + "-registry",
					if podInfo.Labels["control-plane"] == "controller-manager" ||
						podInfo.Labels["component"] == "velero" ||
						podInfo.Labels["component"] == "oadp-"+fmt.Sprintf("%s-%d", vel.Name, 1)+"-"+bsl.Provider+"-registry" {
							log.Printf("Checking env vars are passed to each container in each pod")
							for _, container := range podInfo.Spec.Containers {
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
		Entry("HTTPS_PROXY set", SubscriptionConfigTestCase{
			SubscriptionConfig: operators.SubscriptionConfig{
				Env: []corev1.EnvVar{
					{
						Name:  "HTTPS_PROXY",
						Value: "https://proxy.example.com:8080",
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
		// Leave this as last entry to reset config
		Entry("Config unset", SubscriptionConfigTestCase{
			SubscriptionConfig: operators.SubscriptionConfig{},
		}),
	)
})

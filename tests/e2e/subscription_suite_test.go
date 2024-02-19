package e2e_test

import (
	"context"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operators "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	. "github.com/openshift/oadp-operator/tests/e2e/lib"
)

var _ = Describe("Subscription Config Suite Test", func() {
	type SubscriptionConfigTestCase struct {
		operators.SubscriptionConfig
		failureExpected *bool
		stream          StreamSource
	}

	var _ = AfterEach(func() {
		err := dpaCR.Delete(runTimeClientForSuiteRun)
		Expect(err).ToNot(HaveOccurred())
		Eventually(dpaCR.IsDeleted(runTimeClientForSuiteRun), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())
	})

	DescribeTable("Proxy test table",
		func(testCase SubscriptionConfigTestCase) {
			log.Printf("Getting Operator Subscription")
			s, err := dpaCR.GetOperatorSubscription(runTimeClientForSuiteRun, StreamSource(stream))
			Expect(err).To(BeNil())
			log.Printf("Setting test case subscription config")
			s.Spec.Config = &testCase.SubscriptionConfig
			log.Printf("Updating Subscription")
			err = dpaCR.Client.Update(context.Background(), s.Subscription)
			Expect(err).To(BeNil())
			Eventually(s.CsvIsInstalling, time.Minute*1, time.Second*5).WithArguments(runTimeClientForSuiteRun).Should(BeTrue())

			// get csv from installplan from subscription
			log.Printf("Wait for CSV to be succeeded")
			if testCase.failureExpected != nil && *testCase.failureExpected {
				Consistently(s.CsvIsReady, time.Minute*2, time.Second*5).WithArguments(runTimeClientForSuiteRun).Should(BeFalse())
				// TODO read error message instead?
			} else {
				Eventually(s.CsvIsReady, time.Minute*15, time.Second*5).WithArguments(runTimeClientForSuiteRun).Should(BeTrue())

				log.Printf("Creating test Velero")
				err := dpaCR.Build(CSI)
				Expect(err).NotTo(HaveOccurred())
				//also test restic
				dpaCR.CustomResource.Spec.Configuration.NodeAgent.Enable = pointer.BoolPtr(true)
				dpaCR.CustomResource.Spec.Configuration.NodeAgent.UploaderType = "restic"
				err = dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, &dpaCR.CustomResource.Spec)
				Expect(err).NotTo(HaveOccurred())

				log.Printf("Getting velero object")
				velero, err := dpaCR.Get(runTimeClientForSuiteRun)
				Expect(err).NotTo(HaveOccurred())
				log.Printf("Waiting for velero pod to be running")
				Eventually(AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				if velero.Spec.Configuration.NodeAgent.Enable != nil && *velero.Spec.Configuration.NodeAgent.Enable {
					log.Printf("Waiting for Node Agent pods to be running")
					Eventually(AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
				if s.Spec.Config != nil && s.Spec.Config.Env != nil {
					// get pod env vars
					log.Printf("Getting deployments")
					vd, err := GetVeleroDeploymentList(kubernetesClientForSuiteRun, namespace)
					Expect(err).NotTo(HaveOccurred())
					log.Printf("Getting daemonsets")
					nads, err := GetNodeAgentDaemonsetList(kubernetesClientForSuiteRun, namespace)
					Expect(err).NotTo(HaveOccurred())
					for _, env := range s.Spec.Config.Env {
						for _, deployment := range vd.Items {
							log.Printf("Checking env vars are passed to deployment " + deployment.Name)
							for _, container := range deployment.Spec.Template.Spec.Containers {
								log.Printf("Checking env vars are passed to container " + container.Name)
								Expect(container.Env).To(ContainElement(env))
							}
						}
						for _, daemonset := range nads.Items {
							log.Printf("Checking env vars are passed to daemonset " + daemonset.Name)
							for _, container := range daemonset.Spec.Template.Spec.Containers {
								log.Printf("Checking env vars are passed to container " + container.Name)
								Expect(container.Env).To(ContainElement(env))
							}
						}
					}
				}
				log.Printf("Deleting test Velero")
				err = dpaCR.Delete(runTimeClientForSuiteRun)
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
		// TODO move this to after each
		Entry("Config unset", SubscriptionConfigTestCase{
			SubscriptionConfig: operators.SubscriptionConfig{},
		}),
	)
})

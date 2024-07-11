package e2e_test

import (
	"context"
	"log"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	operators "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("Subscription Config Suite Test", func() {
	type SubscriptionConfigTestCase struct {
		operators.SubscriptionConfig
		failureExpected *bool
	}

	var _ = AfterEach(func() {
		log.Printf("Deleting test DPA")
		err := dpaCR.Delete()
		Expect(err).ToNot(HaveOccurred())
		Eventually(dpaCR.IsDeleted(), time.Minute*2, time.Second*5).Should(BeTrue())
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
			Eventually(s.CsvIsInstalling(runTimeClientForSuiteRun), time.Minute*1, time.Second*5).Should(BeTrue())

			if testCase.failureExpected != nil && *testCase.failureExpected {
				haveErrorMessage := func() bool {
					controllerManagerPods, err := GetAllPodsWithLabel(kubernetesClientForSuiteRun, namespace, "control-plane=controller-manager")
					if err != nil {
						return false
					}
					for _, pod := range controllerManagerPods.Items {
						podLogs, err := GetPodContainerLogs(kubernetesClientForSuiteRun, namespace, pod.Name, "manager")
						if err != nil {
							return false
						}
						if strings.Contains(podLogs, "error setting privileged pod security labels to operator namespace") && strings.Contains(podLogs, "connect: connection refused") {
							log.Printf("found error message in controller manager Pod")
							return true
						}
					}
					return false
				}
				Eventually(haveErrorMessage, time.Minute*1, time.Second*5).Should(BeTrue())
				// TODO can not wait to CSV change to failure, otherwise following tests get stuck
			} else {
				log.Printf("Wait for CSV to be succeeded")
				Eventually(s.CsvIsReady(runTimeClientForSuiteRun), time.Minute*7, time.Second*5).Should(BeTrue())

				Eventually(ManagerPodIsUp(kubernetesClientForSuiteRun, namespace), time.Minute*8, time.Second*5).Should(BeTrue())

				log.Printf("Creating test DPA")
				err = dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, dpaCR.Build(RESTIC))
				Expect(err).NotTo(HaveOccurred())

				log.Print("Checking if DPA is reconciled")
				Eventually(dpaCR.IsReconciledTrue(), time.Minute*1, time.Second*5).Should(BeTrue())

				log.Printf("Getting DPA object")
				dpa, err := dpaCR.Get()
				Expect(err).NotTo(HaveOccurred())

				log.Printf("Waiting for velero Pod to be running")
				Eventually(VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*1, time.Second*5).Should(BeTrue())
				if dpa.Spec.Configuration.NodeAgent.Enable != nil && *dpa.Spec.Configuration.NodeAgent.Enable {
					log.Printf("Waiting for Node Agent Pods to be running")
					Eventually(AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*1, time.Second*5).Should(BeTrue())
				}

				log.Print("Checking if BSL is available")
				Eventually(dpaCR.BSLsAreAvailable(), time.Minute*3, time.Second*5).Should(BeTrue())

				if s.Spec.Config != nil && s.Spec.Config.Env != nil {
					// get pod env vars
					log.Printf("Getting deployments")
					velero, err := GetVeleroDeployment(kubernetesClientForSuiteRun, namespace)
					Expect(err).NotTo(HaveOccurred())
					log.Printf("Getting daemonsets")
					nodeAgent, err := GetNodeAgentDaemonSet(kubernetesClientForSuiteRun, namespace)
					Expect(err).NotTo(HaveOccurred())
					for _, env := range s.Spec.Config.Env {
						log.Printf("Checking env vars are passed to Deployment " + velero.Name)
						for _, container := range velero.Spec.Template.Spec.Containers {
							log.Printf("Checking env vars are passed to Container " + container.Name)
							Expect(container.Env).To(ContainElement(env))
						}
						log.Printf("Checking env vars are passed to DaemonSet " + nodeAgent.Name)
						for _, container := range nodeAgent.Spec.Template.Spec.Containers {
							log.Printf("Checking env vars are passed to Container " + container.Name)
							Expect(container.Env).To(ContainElement(env))
						}
					}
				}
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
			failureExpected: ptr.To(true),
		}),
		// Leave this as last entry to reset config
		// TODO move this to after each
		Entry("Config unset", SubscriptionConfigTestCase{
			SubscriptionConfig: operators.SubscriptionConfig{},
		}),
	)
})

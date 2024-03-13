package e2e_test

import (
	"context"
	"log"
	"strings"
	"time"

	ginkgov2 "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	operators "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

var _ = ginkgov2.Describe("Subscription Config Suite Test", func() {
	type SubscriptionConfigTestCase struct {
		operators.SubscriptionConfig
		failureExpected *bool
	}

	var _ = ginkgov2.AfterEach(func() {
		err := dpaCR.Delete(runTimeClientForSuiteRun)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Eventually(dpaCR.IsDeleted(runTimeClientForSuiteRun), timeoutMultiplier*time.Minute*2, time.Second*5).Should(gomega.BeTrue())
	})

	ginkgov2.DescribeTable("Proxy test table",
		func(testCase SubscriptionConfigTestCase) {
			log.Printf("Getting Operator Subscription")
			s, err := dpaCR.GetOperatorSubscription(runTimeClientForSuiteRun, lib.StreamSource(stream))
			gomega.Expect(err).To(gomega.BeNil())
			log.Printf("Setting test case subscription config")
			s.Spec.Config = &testCase.SubscriptionConfig
			log.Printf("Updating Subscription")
			err = dpaCR.Client.Update(context.Background(), s.Subscription)
			gomega.Expect(err).To(gomega.BeNil())
			gomega.Eventually(s.CsvIsInstalling, time.Minute*1, time.Second*5).WithArguments(runTimeClientForSuiteRun).Should(gomega.BeTrue())

			if testCase.failureExpected != nil && *testCase.failureExpected {
				haveErrorMessage := func() bool {
					controllerManagerPods, err := kubernetesClientForSuiteRun.CoreV1().Pods(namespace).List(context.Background(), v1.ListOptions{LabelSelector: "control-plane=controller-manager"})
					if err != nil {
						return false
					}
					for _, pod := range controllerManagerPods.Items {
						podLogs, err := lib.GetPodContainerLogs(kubernetesClientForSuiteRun, namespace, pod.Name, "manager")
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
				gomega.Eventually(haveErrorMessage, time.Minute*1, time.Second*5).Should(gomega.BeTrue())
			} else {
				log.Printf("Wait for CSV to be succeeded")
				gomega.Eventually(s.CsvIsReady, time.Minute*7, time.Second*5).WithArguments(runTimeClientForSuiteRun).Should(gomega.BeTrue())

				var controllerManagerPodName string
				onlyOneManagerPod := func() bool {
					controllerManagerPod, err := kubernetesClientForSuiteRun.CoreV1().Pods(namespace).List(context.Background(), v1.ListOptions{LabelSelector: "control-plane=controller-manager"})
					if err != nil {
						return false
					}
					log.Printf("waiting for having only one controller manager Pod")
					if len(controllerManagerPod.Items) == 1 {
						controllerManagerPodName = controllerManagerPod.Items[0].Name
						return true
					}
					return false
				}

				gomega.Eventually(onlyOneManagerPod, time.Minute*4, time.Second*5).Should(gomega.BeTrue())

				isLeaseReady := func() bool {
					podLogs, err := lib.GetPodContainerLogs(kubernetesClientForSuiteRun, namespace, controllerManagerPodName, "manager")
					if err != nil {
						return false
					}
					log.Printf("waiting leaderelection")
					return strings.Contains(podLogs, "leaderelection.go:258] successfully acquired lease")
				}
				gomega.Eventually(isLeaseReady, time.Minute*4, time.Second*5).Should(gomega.BeTrue())

				log.Printf("Creating test Velero")
				dpaCR.Build(lib.CSI)
				//also test restic
				dpaCR.CustomResource.Spec.Configuration.NodeAgent.Enable = pointer.BoolPtr(true)
				dpaCR.CustomResource.Spec.Configuration.NodeAgent.UploaderType = "restic"
				err = dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, &dpaCR.CustomResource.Spec)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				log.Printf("Getting velero object")
				velero, err := dpaCR.Get(runTimeClientForSuiteRun)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				log.Printf("Waiting for velero pod to be running")
				gomega.Eventually(lib.AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*1, time.Second*5).Should(gomega.BeTrue())
				if velero.Spec.Configuration.NodeAgent.Enable != nil && *velero.Spec.Configuration.NodeAgent.Enable {
					log.Printf("Waiting for Node Agent pods to be running")
					gomega.Eventually(lib.AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*1, time.Second*5).Should(gomega.BeTrue())
				}
				if s.Spec.Config != nil && s.Spec.Config.Env != nil {
					// get pod env vars
					log.Printf("Getting deployments")
					vd, err := lib.GetVeleroDeploymentList(kubernetesClientForSuiteRun, namespace)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					log.Printf("Getting daemonsets")
					nads, err := lib.GetNodeAgentDaemonsetList(kubernetesClientForSuiteRun, namespace)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					for _, env := range s.Spec.Config.Env {
						for _, deployment := range vd.Items {
							log.Printf("Checking env vars are passed to deployment " + deployment.Name)
							for _, container := range deployment.Spec.Template.Spec.Containers {
								log.Printf("Checking env vars are passed to container " + container.Name)
								gomega.Expect(container.Env).To(gomega.ContainElement(env))
							}
						}
						for _, daemonset := range nads.Items {
							log.Printf("Checking env vars are passed to daemonset " + daemonset.Name)
							for _, container := range daemonset.Spec.Template.Spec.Containers {
								log.Printf("Checking env vars are passed to container " + container.Name)
								gomega.Expect(container.Env).To(gomega.ContainElement(env))
							}
						}
					}
				}
				log.Printf("Deleting test Velero")
				err = dpaCR.Delete(runTimeClientForSuiteRun)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			}

		},
		ginkgov2.Entry("HTTP_PROXY set", SubscriptionConfigTestCase{
			SubscriptionConfig: operators.SubscriptionConfig{
				Env: []corev1.EnvVar{
					{
						Name:  "HTTP_PROXY",
						Value: "http://proxy.example.com:8080",
					},
				},
			},
		}),
		ginkgov2.Entry("NO_PROXY set", SubscriptionConfigTestCase{
			SubscriptionConfig: operators.SubscriptionConfig{
				Env: []corev1.EnvVar{
					{
						Name:  "NO_PROXY",
						Value: "1.1.1.1",
					},
				},
			},
		}),
		ginkgov2.Entry("HTTPS_PROXY set", SubscriptionConfigTestCase{
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
		ginkgov2.Entry("Config unset", SubscriptionConfigTestCase{
			SubscriptionConfig: operators.SubscriptionConfig{},
		}),
	)
})

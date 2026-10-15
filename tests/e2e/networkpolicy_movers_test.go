package e2e_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func expectNodeAgentNetworkPolicy() {
	ginkgo.GinkgoHelper()
	gomega.Eventually(func() error {
		policy, err := kubernetesClientForSuiteRun.NetworkingV1().NetworkPolicies(namespace).Get(context.Background(), "velero-network-policy", metav1.GetOptions{})
		if err != nil {
			return err
		}
		selector, err := metav1.LabelSelectorAsSelector(&policy.Spec.PodSelector)
		if err != nil {
			return err
		}
		pods, err := kubernetesClientForSuiteRun.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "name=node-agent,component=velero"})
		if err != nil {
			return err
		}
		if len(pods.Items) == 0 {
			return fmt.Errorf("no node-agent pods found")
		}
		active := 0
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp != nil {
				continue
			}
			active++
			if !selector.Matches(labels.Set(pod.Labels)) {
				return fmt.Errorf("node-agent pod %s does not match policy %s", pod.Name, policy.Name)
			}
		}
		if active == 0 {
			return fmt.Errorf("no active node-agent pods found")
		}
		return nil
	}, time.Minute, 2*time.Second).Should(gomega.Succeed())
}

// Start watching before the operation creates any movers. Completed transfer
// pods can disappear before the backup/restore finishes, so listing afterward
// alone cannot prove that those pods carried the policy label.
func observeNetworkPolicyMovers(podLabel string) func(string) {
	ginkgo.GinkgoHelper()
	ctx, cancel := context.WithCancel(context.Background())
	events, err := kubernetesClientForSuiteRun.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{LabelSelector: podLabel})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ginkgo.DeferCleanup(func() { cancel(); events.Stop() })
	var mu sync.Mutex
	observed := map[string]map[types.UID]*corev1.Pod{}
	var watchErr error
	go func() {
		for event := range events.ResultChan() {
			mu.Lock()
			if pod, ok := event.Object.(*corev1.Pod); ok {
				name := pod.Labels[podLabel]
				if observed[name] == nil {
					observed[name] = map[types.UID]*corev1.Pod{}
				}
				if _, found := observed[name][pod.UID]; !found {
					observed[name][pod.UID] = pod.DeepCopy()
				}
			} else if event.Type == watch.Error {
				watchErr = fmt.Errorf("mover pod watch failed: %v", event.Object)
			}
			mu.Unlock()
		}
	}()
	return func(operationName string) {
		ginkgo.GinkgoHelper()
		policy, err := kubernetesClientForSuiteRun.NetworkingV1().NetworkPolicies(namespace).Get(ctx, "velero-mover-network-policy", metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(policy.Spec.PolicyTypes).To(gomega.ContainElement(networkingv1.PolicyTypeIngress))
		gomega.Expect(policy.Spec.Ingress).To(gomega.BeEmpty())
		selector, err := metav1.LabelSelectorAsSelector(&policy.Spec.PodSelector)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Eventually(func() error {
			var transferNames []string
			if podLabel == velero.DataUploadLabel {
				transfers := &velerov2alpha1.DataUploadList{}
				if err := dpaCR.Client.List(ctx, transfers, client.InNamespace(namespace), client.MatchingLabels{velero.BackupNameLabel: operationName}); err != nil {
					return err
				}
				for _, transfer := range transfers.Items {
					transferNames = append(transferNames, transfer.Name)
				}
			} else {
				transfers := &velerov2alpha1.DataDownloadList{}
				if err := dpaCR.Client.List(ctx, transfers, client.InNamespace(namespace), client.MatchingLabels{velero.RestoreNameLabel: operationName}); err != nil {
					return err
				}
				for _, transfer := range transfers.Items {
					transferNames = append(transferNames, transfer.Name)
				}
			}
			if len(transferNames) == 0 {
				return fmt.Errorf("no transfers found for %s", operationName)
			}
			mu.Lock()
			defer mu.Unlock()
			if watchErr != nil {
				return watchErr
			}
			for _, name := range transferNames {
				pods := observed[name]
				if len(pods) == 0 {
					return fmt.Errorf("did not observe a mover pod for transfer %s", name)
				}
				for _, pod := range pods {
					if !selector.Matches(labels.Set(pod.Labels)) {
						return fmt.Errorf("mover pod %s for transfer %s does not match policy %s", pod.Name, name, policy.Name)
					}
				}
			}
			return nil
		}, 30*time.Second, time.Second).Should(gomega.Succeed())
	}
}

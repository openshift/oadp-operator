package lib

import (
	"context"
	"fmt"
	"log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift/oadp-operator/pkg/common"
)

func GetNodeAgentDaemonSet(c *kubernetes.Clientset, namespace string) (*appsv1.DaemonSet, error) {
	nodeAgent, err := c.AppsV1().DaemonSets(namespace).Get(context.Background(), common.NodeAgent, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return nodeAgent, nil
}

func AreNodeAgentPodsRunning(c *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	log.Printf("Checking for correct number of running Node Agent Pods...")
	return func() (bool, error) {
		nodeAgentDaemonSet, err := GetNodeAgentDaemonSet(c, namespace)
		if err != nil {
			return false, err
		}

		numScheduled := nodeAgentDaemonSet.Status.CurrentNumberScheduled
		numDesired := nodeAgentDaemonSet.Status.DesiredNumberScheduled
		// check correct number of NodeAgent Pods are initialized
		if numScheduled != numDesired {
			return false, fmt.Errorf("wrong number of Node Agent Pods")
		}
		if numDesired == 0 {
			return true, nil
		}

		podList, err := GetAllPodsWithLabel(c, namespace, "name="+common.NodeAgent)
		if err != nil {
			return false, err
		}

		for _, pod := range podList.Items {
			if pod.Status.Phase != corev1.PodRunning {
				return false, fmt.Errorf("not all Node Agent Pods are running")
			}
		}
		return true, nil
	}
}

// keep for now
func IsNodeAgentDaemonSetDeleted(c *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	log.Printf("Checking if NodeAgent DaemonSet has been deleted...")
	return func() (bool, error) {
		_, err := GetNodeAgentDaemonSet(c, namespace)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
}

func NodeAgentDaemonSetHasNodeSelector(c *kubernetes.Clientset, namespace, key, value string) wait.ConditionFunc {
	return func() (bool, error) {
		ds, err := GetNodeAgentDaemonSet(c, namespace)
		if err != nil {
			return false, err
		}
		// verify DaemonSet has nodeSelector "foo": "bar"
		return ds.Spec.Template.Spec.NodeSelector[key] == value, nil
	}
}

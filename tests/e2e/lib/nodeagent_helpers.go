package lib

import (
	"context"
	"log"
	"time"

	"github.com/openshift/oadp-operator/pkg/common"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

func HasCorrectNumNodeAgentPods(c *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		nodeAgentOptions := metav1.ListOptions{
			FieldSelector: "metadata.name=" + common.NodeAgent,
		}
		nodeAgentDaemeonSet, err := c.AppsV1().DaemonSets(namespace).List(context.Background(), nodeAgentOptions)
		if err != nil {
			return false, err
		}
		var numScheduled int32
		var numDesired int32

		for _, daemonSetInfo := range (*nodeAgentDaemeonSet).Items {
			numScheduled = daemonSetInfo.Status.CurrentNumberScheduled
			numDesired = daemonSetInfo.Status.DesiredNumberScheduled
		}
		// check correct num of NodeAgent pods are initialized
		if numScheduled != 0 && numDesired != 0 {
			if numScheduled == numDesired {
				return true, nil
			}
		}
		if numDesired == 0 {
			return true, nil
		}
		return false, err
	}
}

func waitForDesiredNodeAgentPods(c *kubernetes.Clientset, namespace string) error {
	return wait.PollImmediate(time.Second*5, time.Minute*2, HasCorrectNumNodeAgentPods(c, namespace))
}

func AreNodeAgentPodsRunning(c *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	log.Printf("Checking for correct number of running Node Agent pods...")
	return func() (bool, error) {
		err := waitForDesiredNodeAgentPods(c, namespace)
		if err != nil {
			return false, err
		}
		nodeAgentOptions := metav1.ListOptions{
			LabelSelector: "name=" + common.NodeAgent,
		}
		// get pods in the oadp-operator-e2e namespace with label selector
		podList, err := c.CoreV1().Pods(namespace).List(context.Background(), nodeAgentOptions)
		if err != nil {
			return false, nil
		}
		// loop until pod status is 'Running' or timeout
		for _, podInfo := range (*podList).Items {
			if podInfo.Status.Phase != "Running" {
				return false, err
			}
		}
		return true, err
	}
}

// keep for now
func IsNodeAgentDaemonsetDeleted(c *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	log.Printf("Checking if NodeAgent daemonset has been deleted...")
	return func() (bool, error) {
		// Check for daemonSet
		_, err := c.AppsV1().DaemonSets(namespace).Get(context.Background(), common.NodeAgent, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
}

func NodeAgentDaemonSetHasNodeSelector(c *kubernetes.Clientset, namespace, key, value string) wait.ConditionFunc {
	return func() (bool, error) {
		ds, err := c.AppsV1().DaemonSets(namespace).Get(context.Background(), common.NodeAgent, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		// verify daemonset has nodeSelector "foo": "bar"
		selector := ds.Spec.Template.Spec.NodeSelector[key]

		if selector == value {
			return true, nil
		}
		return false, err
	}
}

func GetNodeAgentDaemonsetList(c *kubernetes.Clientset, namespace string) (*appsv1.DaemonSetList, error) {
	registryListOptions := metav1.ListOptions{
		LabelSelector: "component=velero",
	}
	// get pods in the oadp-operator-e2e namespace with label selector
	deploymentList, err := c.AppsV1().DaemonSets(namespace).List(context.Background(), registryListOptions)
	if err != nil {
		return nil, err
	}
	return deploymentList, nil
}

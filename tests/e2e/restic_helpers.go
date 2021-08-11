package e2e

import (
	"context"
	"log"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// Defining new var for Velero CR to include 'restic_node_selector'
// unable to use setNestedField() to do so as there is currently
// a bug in using this to set a map[string]string with dynamic client
// panic: cannot deep copy []map[string]string

func hasCorrectNumResticPods(namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		client, err := setUpClient()
		if err != nil {
			return false, err
		}
		resticOptions := metav1.ListOptions{
			FieldSelector: "metadata.name=restic",
		}
		// get daemonset in oadp-operator-e2e ns with specified field selector
		resticDaemeonSet, err := client.AppsV1().DaemonSets(namespace).List(context.TODO(), resticOptions)
		if err != nil {
			return false, err
		}
		var numScheduled int32
		var numDesired int32

		for _, daemonSetInfo := range (*resticDaemeonSet).Items {
			numScheduled = daemonSetInfo.Status.CurrentNumberScheduled
			numDesired = daemonSetInfo.Status.DesiredNumberScheduled
		}
		// check correct num of Restic pods are initialized
		if numScheduled != 0 && numDesired != 0 {
			if numScheduled == numDesired {
				return true, nil
			}
		}
		return false, err
	}
}

func waitForDesiredResticPods(namespace string) error {
	return wait.PollImmediate(time.Second*5, time.Minute*2, hasCorrectNumResticPods(namespace))
}

func areResticPodsRunning(namespace string) wait.ConditionFunc {
	log.Printf("Checking for correct number of running Restic pods...")
	return func() (bool, error) {
		er := waitForDesiredResticPods(namespace)
		if er != nil {
			return false, er
		}
		client, err := setUpClient()
		if err != nil {
			return false, err
		}
		// used to select Restic pods
		resticPodOptions := metav1.ListOptions{
			LabelSelector: "name=restic",
		}
		// get pods in the oadp-operator-e2e namespace with label selector
		podList, err := client.CoreV1().Pods(namespace).List(context.TODO(), resticPodOptions)
		if err != nil {
			return false, nil
		}
		// loop until pod status is 'Running' or timeout
		for _, podInfo := range (*podList).Items {
			if podInfo.Status.Phase != "Running" {
				return false, err
			} else {
				return true, nil
			}
		}
		return false, err
	}
}

func doesDaemonSetExists(namespace string, resticName string) wait.ConditionFunc {
	log.Printf("Checking if restic daemonset exists...")
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}
		_, err = clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), resticName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return true, nil
	}
}

// keep for now
func isResticDaemonsetDeleted(namespace string, instanceName string, resticName string) wait.ConditionFunc {
	log.Printf("Checking if Restic daemonset has been deleted...")
	return func() (bool, error) {
		client, err := setUpClient()
		if err != nil {
			return false, nil
		}
		// Check for daemonSet
		_, err = client.AppsV1().DaemonSets(namespace).Get(context.Background(), resticName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Restic daemonSet has been deleted")
			return true, nil
		}
		return false, err
	}
}

func resticDaemonSetHasNodeSelector(namespace string, s3Bucket string, credSecretRef string, instanceName string, resticName string) wait.ConditionFunc {
	log.Printf("Waiting for Restic daemonset to have a nodeSelector...")
	return func() (bool, error) {
		client, err := setUpClient()
		if err != nil {
			return false, nil
		}
		ds, err := client.AppsV1().DaemonSets(namespace).Get(context.TODO(), resticName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		// verify daemonset has nodeSelector "foo": "bar"
		selector := ds.Spec.Template.Spec.NodeSelector["foo"]

		if selector == "bar" {
			return true, nil
		}
		return false, err
	}
}

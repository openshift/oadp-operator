package e2e

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
)

// Defining new var for Velero CR to include 'restic_node_selector'
// unable to use setNestedField() to do so as there is currently
// a bug in using this to set a map[string]string with dynamic client
// panic: cannot deep copy []map[string]string

func getResticVeleroConfig(namespace string, s3Bucket string, credSecretRef string, instanceName string) *unstructured.Unstructured {
	// Default Velero Instance config with backup_storage_locations defaulted to AWS.
	var resticVeleroSpec = unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "konveyor.openshift.io/v1alpha1",
			"kind":       "Velero",
			"metadata": map[string]interface{}{
				"name":      instanceName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"restic_node_selector": map[string]interface{}{
					"foo": "bar",
				},
				"olm_managed": false,
				"default_velero_plugins": []string{
					"aws",
					"csi",
					"openshift",
				},
				"backup_storage_locations": [](map[string]interface{}){
					map[string]interface{}{
						"config": map[string]interface{}{
							"profile": "default",
							"region":  "us-east-1",
						},
						"credentials_secret_ref": map[string]interface{}{
							"name":      credSecretRef,
							"namespace": "oadp-operator",
						},
						"object_storage": map[string]interface{}{
							"bucket": s3Bucket,
							"prefix": "velero",
						},
						"name":     "default",
						"provider": "aws",
					},
				},
				"velero_feature_flags": "EnableCSI",
				"enable_restic":        true,
				"volume_snapshot_locations": [](map[string]interface{}){
					map[string]interface{}{
						"config": map[string]interface{}{
							"profile": "default",
							"region":  "us-west-2",
						},
						"name":     "default",
						"provider": "aws",
					},
				},
			},
		},
	}
	return &resticVeleroSpec
}

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
	fmt.Println("Checking for correct number of running Restic pods...")
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

func disableRestic(namespace string, instanceName string) error {
	veleroClient, err := setUpDynamicVeleroClient(namespace)
	if err != nil {
		return nil
	}
	// get Velero as unstructured type
	veleroResource, err := veleroClient.Get(context.Background(), instanceName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	// update spec 'enable_restic' to be false
	err = unstructured.SetNestedField(veleroResource.Object, false, "spec", "enable_restic")
	if err != nil {
		return err
	}
	_, err = veleroClient.Update(context.Background(), veleroResource, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	fmt.Println("spec 'enable_restic' has been updated to false")
	return nil
}

func doesDaemonSetExists(namespace string, resticName string) wait.ConditionFunc {
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}
		_, err = clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), resticName, metav1.GetOptions{})
		if err != nil {
			fmt.Println("Restic daemonSet does not exist..")
			return false, err
		}
		fmt.Println("Restic daemonSet exists")
		return true, nil
	}
}

// keep for now
func isResticDaemonsetDeleted(namespace string, instanceName string, resticName string) wait.ConditionFunc {
	fmt.Println("Checking Restic daemonSet has been deleted...")
	return func() (bool, error) {
		client, err := setUpClient()
		if err != nil {
			return false, nil
		}
		// Check for daemonSet
		_, err = client.AppsV1().DaemonSets(namespace).Get(context.Background(), resticName, metav1.GetOptions{})
		if err != nil {
			fmt.Println("Restic daemonSet has been deleted")
			return true, nil
		}
		fmt.Println("daemonSet still exists")
		return false, err
	}
}

func enableResticNodeSelector(namespace string, s3Bucket string, credSecretRef string, instanceName string) error {
	veleroClient, err := setUpDynamicVeleroClient(namespace)
	if err != nil {
		return nil
	}
	// get Velero as unstructured type
	veleroResource := getResticVeleroConfig(namespace, s3Bucket, credSecretRef, instanceName)
	_, err = veleroClient.Create(context.Background(), veleroResource, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	fmt.Println("spec 'restic_node_selector' has been updated")
	return nil
}

func resticDaemonSetHasNodeSelector(namespace string, s3Bucket string, credSecretRef string, instanceName string, resticName string) wait.ConditionFunc {
	fmt.Println("Checking Restic daemonSet has a nodeSelector...")
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
			fmt.Println("Restic daemonset has nodeSelector")
			return true, nil
		}
		return false, err
	}
}

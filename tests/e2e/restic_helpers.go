package e2e

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
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
					"csix",
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
				"enable_restic":        false,
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

func areResticPodsRunning(namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		kubeConf := getKubeConfig()
		// create client for daemonset
		client, err := kubernetes.NewForConfig(kubeConf)
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
		// if numScheduled == numDesired, then all restic pods are running
		if numScheduled != 0 && numDesired != 0 {
			if numScheduled == numDesired {
				fmt.Println("All restic pods are running")
				return true, nil
			}
		}
		return false, err
	}
}

func disableRestic(namespace string, instanceName string) error {
	config := getKubeConfig()
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}
	veleroClient, err := createVeleroClient(client, namespace)
	if err != nil {
		return nil
	}
	// get Velero as unstructured type
	veleroResource, err := veleroClient.Get(context.Background(), instanceName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	// update spec enable_restic to be false
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

func isResticDaemonsetDeleted(namespace string, instanceName string, resticName string) wait.ConditionFunc {
	err := disableRestic(namespace, instanceName)
	if err != nil {
		return nil
	}
	return func() (bool, error) {
		config := getKubeConfig()
		client, err := dynamic.NewForConfig(config)
		if err != nil {
			return false, nil
		}
		veleroClient, err := createVeleroClient(client, namespace)
		if err != nil {
			return false, err
		}
		_, errs := veleroClient.Get(context.Background(), resticName, metav1.GetOptions{})
		if apierrors.IsAlreadyExists(errs) {
			return false, errors.New("restic daemonset has not been deleted")
		}
		fmt.Println("Restic daemonset has been deleted")
		return true, nil
	}
}

func decodeResticYaml(resticVeleroConfigYAML string) (*unstructured.Unstructured, error) {
	// set new unstructured type for Velero CR
	unstructVelero := &unstructured.Unstructured{}

	// decode yaml into unstructured type
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode([]byte(resticVeleroConfigYAML), nil, unstructVelero)
	if err != nil {
		return unstructVelero, err
	}
	return unstructVelero, nil
}

func enableResticNodeSelector(namespace string, s3Bucket string, credSecretRef string, instanceName string) error {
	config := getKubeConfig()
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}
	veleroClient, err := createVeleroClient(client, namespace)
	if err != nil {
		return nil
	}
	// get Velero as unstructured type
	veleroResource := getResticVeleroConfig(namespace, s3Bucket, credSecretRef, instanceName) //decodeResticYaml()
	_, err = veleroClient.Create(context.Background(), veleroResource, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	fmt.Println("spec 'restic_node_selector' has been updated")
	return nil
}

func resticDaemonSetHasNodeSelector(namespace string, s3Bucket string, credSecretRef string, instanceName string) wait.ConditionFunc {
	err := enableResticNodeSelector(namespace, s3Bucket, credSecretRef, instanceName)
	if err != nil {
		return nil
	}
	return func() (bool, error) {
		config := getKubeConfig()
		client, err := kubernetes.NewForConfig(config)
		if err != nil {
			return false, nil
		}
		resticOptions := metav1.ListOptions{
			FieldSelector: "spec.template.spec.nodeSelector.foo=bar",
		}
		// get daemonset in oadp-operator-e2e ns with specified field selector
		_, errs := client.AppsV1().DaemonSets(namespace).List(context.TODO(), resticOptions)
		if errs != nil {
			return false, err
		}
		fmt.Println("Restic daemonset has NodeSelector")
		return true, nil
	}
}

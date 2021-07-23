package e2e

import (
	"context"
	"errors"
	"fmt"
	"time"

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
const ResticVeleroConfigYAML = `apiVersion: konveyor.openshift.io/v1alpha1
kind: Velero
metadata:
  name: example-velero
spec:
  restic_node_selector:
    foo: bar
  olm_managed: false
  default_velero_plugins:
  - aws
  - openshift
  - csi
  backup_storage_locations:
  - name: default
    provider: aws
    object_storage:
      bucket: myBucket
      prefix: "velero"
    config:
      region: us-east-1
      profile: "default"
    credentials_secret_ref:
      name: cloud-credentials
      namespace: oadp-operator
  volume_snapshot_locations:
  - name: default
    provider: aws
    config:
      region: us-west-2
      profile: "default"
  enable_restic: true
  velero_feature_flags: EnableCSI`

func areResticPodsRunning() wait.ConditionFunc {
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
		resticDaemeonSet, err := client.AppsV1().DaemonSets("oadp-operator").List(context.TODO(), resticOptions)
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

func waitForResticPods() error {
	// poll pod every 5 secs for 2 mins until it's running or timeout occurs
	return wait.PollImmediate(time.Second*5, time.Minute*2, areResticPodsRunning())
}

func disableRestic() error {
	config := getKubeConfig()
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}
	veleroClient, err := createVeleroClient(client)
	if err != nil {
		return nil
	}
	// get Velero as unstructured type
	veleroResource, err := veleroClient.Get(context.Background(), "example-velero", metav1.GetOptions{})
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

func isResticDaemonsetDeleted() wait.ConditionFunc {
	err := disableRestic()
	if err != nil {
		panic(err)
	}
	return func() (bool, error) {
		config := getKubeConfig()
		client, err := dynamic.NewForConfig(config)
		if err != nil {
			return false, nil
		}
		veleroClient, err := createVeleroClient(client)
		if err != nil {
			return false, err
		}
		_, errs := veleroClient.Get(context.Background(), "restic", metav1.GetOptions{})
		if apierrors.IsAlreadyExists(errs) {
			return false, errors.New("restic daemonset has not been deleted")
		}
		fmt.Println("Restic daemonset has been deleted")
		return true, nil
	}
}

func waitForDeletedRestic() error {
	return wait.PollImmediate(time.Second*5, time.Minute*2, isResticDaemonsetDeleted())
}

func decodeResticYaml() *unstructured.Unstructured {
	// set new unstructured type for Velero CR
	unstructVelero := &unstructured.Unstructured{}

	// decode yaml into unstructured type
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode([]byte(ResticVeleroConfigYAML), nil, unstructVelero)
	if err != nil {
		panic(err)
	}
	return unstructVelero
}

func enableResticNodeSelector() error {
	config := getKubeConfig()
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}
	veleroClient, err := createVeleroClient(client)
	if err != nil {
		return nil
	}
	// get Velero as unstructured type
	veleroResource := decodeResticYaml()
	_, err = veleroClient.Create(context.Background(), veleroResource, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	fmt.Println("spec 'restic_node_selector' has been updated")
	return nil
}

func resticDaemonSetHasNodeSelector() wait.ConditionFunc {

	return func() (bool, error) {
		config := getKubeConfig()
		client, err := kubernetes.NewForConfig(config)
		if err != nil {
			return false, nil
		}
		// get daemonset in oadp-operator-e2e ns with specified field selector
		ds, errs := client.AppsV1().DaemonSets("oadp-operator").Get(context.TODO(), "restic", metav1.GetOptions{})
		if errs != nil {
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

func waitForResticNodeSelector() error {
	return wait.PollImmediate(time.Second*5, time.Minute*1, resticDaemonSetHasNodeSelector())
}

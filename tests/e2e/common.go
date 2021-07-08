package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/wait"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const DefaultVeleroConfigYAML = `apiVersion: konveyor.openshift.io/v1alpha1
kind: Velero
metadata:
  name: example-velero
spec:
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
      namespace: oadp-operator-e2e
  volume_snapshot_locations:
  - name: default
    provider: aws
    config:
      region: us-west-2
      profile: "default"
  enable_restic: true
  velero_feature_flags: EnableCSI`

func decodeYaml() *unstructured.Unstructured {
	// set new unstructured type for Velero CR
	unstructVelero := &unstructured.Unstructured{}

	// decode yaml into unstructured type
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode([]byte(DefaultVeleroConfigYAML), nil, unstructVelero)
	if err != nil {
		panic(err)
	}
	return unstructVelero
}

func createOADPTestNamespace() error {
	kubeConf := getKubeConfig()

	// create client for pod
	clientset, err := kubernetes.NewForConfig(kubeConf)
	if err != nil {
		panic(err)
	}
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "oadp-operator-e2e",
		},
	}
	_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), &ns, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}

	return err
}

func createVeleroClient(client dynamic.Interface) (dynamic.ResourceInterface, error) {

	resourceClient := client.Resource(schema.GroupVersionResource{
		Group:    "konveyor.openshift.io",
		Version:  "v1alpha1",
		Resource: "veleros",
	})
	namespaceResClient := resourceClient.Namespace("oadp-operator-e2e")

	return namespaceResClient, nil
}

func createDefaultVeleroCR(res *unstructured.Unstructured, client dynamic.Interface) (*unstructured.Unstructured, error) {
	veleroClient, err := createVeleroClient(client)
	if err != nil {
		return nil, err
	}
	createdResource, err := veleroClient.Create(context.Background(), res, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil, errors.New("found unexpected existing Velero CR")
	} else if err != nil {
		return nil, err
	}
	return createdResource, nil
}

func deleteVeleroCR(client dynamic.Interface) error {
	veleroClient, err := createVeleroClient(client)
	if err != nil {
		return err
	}
	return veleroClient.Delete(context.Background(), "example-velero", metav1.DeleteOptions{})
}

func getKubeConfig() *rest.Config {
	return config.GetConfigOrDie()
}

func isVeleroPodRunning() wait.ConditionFunc {
	return func() (bool, error) {
		kubeConf := getKubeConfig()

		// create client for pod
		clientset, err := kubernetes.NewForConfig(kubeConf)
		if err != nil {
			panic(err)
		}
		// select Velero pod with this label
		veleroOptions := metav1.ListOptions{
			LabelSelector: "component=velero",
		}
		// get pods in the oadp-operator namespace
		podList, err := clientset.CoreV1().Pods("oadp-operator-e2e").List(context.TODO(), veleroOptions)
		if err != nil {
			panic(err)
		}
		// get pod name and status with specified label selector
		var status string
		for _, podInfo := range (*podList).Items {
			status = string(podInfo.Status.Phase)
		}
		if status == "Running" {
			return true, nil
		}
		return false, err
	}
}

func waitForVeleroPodRunning() error {
	// poll pod every 5 secs for 3 mins until it's running or timeout occurs
	return wait.PollImmediate(time.Second*5, time.Minute*3, isVeleroPodRunning())
}

func areResticPodsRunning() wait.ConditionFunc {
	return func() (bool, error) {
		kubeConf := getKubeConfig()
		// create client for pods
		client, err := kubernetes.NewForConfig(kubeConf)
		if err != nil {
			return false, err
		}
		resticOptions := metav1.ListOptions{
			FieldSelector: "metadata.name=restic",
		}
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
		if numScheduled != 0 && numDesired != 0 {
			if numScheduled == numDesired {
				return true, nil
			}
		}
		return false, err
	}
}

func waitForResticPods() error {
	// poll pod every 5 secs for 3 mins until it's running or timeout occurs
	return wait.PollImmediate(time.Second*5, time.Minute*3, areResticPodsRunning())
}

func waitForFailedVeleroCR() error {
	return wait.PollImmediate(time.Second*5, time.Minute*2, isVeleroCRFailed())
}

func isVeleroCRFailed() wait.ConditionFunc {
	kubeConfig := getKubeConfig()

	// create dynamic client for CR
	client, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil
	}
	veleroClient, err := createVeleroClient(client)
	if err != nil {
		return nil
	}
	return func() (bool, error) {
		// Get velero CR in cluster
		veleroResource, err := veleroClient.Get(context.Background(), "example-velero", metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		// Read status subresource from cluster
		veleroStatus := ansibleOperatorStatus{}
		statusObj := veleroResource.Object["status"]
		// Convert status subresource interface to typed structure
		statusBytes, err := json.Marshal(statusObj)
		if err != nil {
			return false, err
		}
		err = json.Unmarshal(statusBytes, &veleroStatus)
		if err != nil {
			return false, err
		}
		conditions := veleroStatus.Conditions
		for _, condition := range conditions {
			if condition.Type == "Failure" {
				return true, nil
			}
		}
		return false, nil
	}
}

type ansibleOperatorStatus struct {
	Conditions []condition `json:"conditions"`
}
type condition struct {
	AnsibleResult ansibleResult `json:"ansibleResult,omitempty"`
	//LastTransitionTime time.Time     `json:"lastTransitionTime"`
	Message string `json:"message"`
	Reason  string `json:"reason"`
	Status  string `json"status"`
	Type    string `json:"type"`
}

type ansibleResult struct {
	Changed int `json:"changed"`
	//Completion time.Time `json:"completion"`
	Failures int `json:"failures"`
	Ok       int `json:"ok"`
	Skipped  int `json:"skipped"`
}

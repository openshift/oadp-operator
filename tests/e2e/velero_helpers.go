package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
)

func getDefaultVeleroConfig(namespace string, s3Bucket string, credSecretRef string, instanceName string) *unstructured.Unstructured {
	// Velero Instance creation spec with backupstorage location default to AWS. Would need to parameterize this later on to support multiple plugins.
	var veleroSpec = unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "konveyor.openshift.io/v1alpha1",
			"kind":       "Velero",
			"metadata": map[string]interface{}{
				"name":      instanceName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
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
							"namespace": namespace,
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
	return &veleroSpec
}

func createDefaultVeleroCR(res *unstructured.Unstructured, client dynamic.Interface, namespace string) (*unstructured.Unstructured, error) {
	veleroClient, err := createVeleroClient(client, namespace)
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

func deleteVeleroCR(client dynamic.Interface, instanceName string, namespace string) error {
	veleroClient, err := createVeleroClient(client, namespace)
	if err != nil {
		return err
	}
	return veleroClient.Delete(context.Background(), instanceName, metav1.DeleteOptions{})
}

func createVeleroClient(client dynamic.Interface, namespace string) (dynamic.ResourceInterface, error) {
	resourceClient := client.Resource(schema.GroupVersionResource{
		Group:    "konveyor.openshift.io",
		Version:  "v1alpha1",
		Resource: "veleros",
	})
	namespaceResClient := resourceClient.Namespace(namespace)

	return namespaceResClient, nil
}

func setUpDynamicVeleroClient(namespace string) (dynamic.ResourceInterface, error) {
	kubeConfig := getKubeConfig()
	client, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	veleroClient, errs := createVeleroClient(client, namespace)
	if err != nil {
		return nil, errs
	}
	return veleroClient, nil
}

func isVeleroPodRunning(namespace string) wait.ConditionFunc {
	fmt.Println("Checking for running Velero pod...")
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}
		// select Velero pod with this label
		veleroOptions := metav1.ListOptions{
			LabelSelector: "component=velero",
		}
		// get pods in test namespace with labelSelector
		podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), veleroOptions)
		if err != nil {
			return false, nil
		}
		// get pod name and status with specified label selector
		var status string
		for _, podInfo := range (*podList).Items {
			status = string(podInfo.Status.Phase)
		}
		if status == "Running" {
			fmt.Println("Velero pod is running")
			return true, nil
		}
		return false, err
	}
}

func isVeleroCRFailed(namespace string, instanceName string) wait.ConditionFunc {
	veleroClient, err := setUpDynamicVeleroClient(namespace)
	if err != nil {
		return nil
	}
	return func() (bool, error) {
		// Get velero CR
		veleroResource, err := veleroClient.Get(context.Background(), instanceName, metav1.GetOptions{})
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
		var message string

		for _, condition := range conditions {
			message = condition.Message
			if condition.Type == "Failure" {
				fmt.Printf("Velero install failure: %s\n", message)
				return true, nil
			}
		}
		return false, err
	}
}

// Used to read Velero CR fields
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

func isVeleroDeleted(namespace string, instanceName string) wait.ConditionFunc {
	fmt.Println("Checking the Velero CR has been deleted...")
	return func() (bool, error) {
		veleroClient, err := setUpDynamicVeleroClient(namespace)
		if err != nil {
			return false, err
		}
		// Check for velero CR in cluster
		_, err = veleroClient.Get(context.Background(), instanceName, metav1.GetOptions{})
		if err != nil {
			fmt.Println("Velero has been deleted")
			return true, nil
		}
		fmt.Println("Velero CR still exists")
		return false, err
	}
}

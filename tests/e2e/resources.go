package e2e

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func isVeleroPodRunning(namespace string) wait.ConditionFunc {
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
		// get pods in the oadp-operator-e2e namespace
		podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), veleroOptions)
		if err != nil {
			panic(err)
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

func waitForVeleroPodRunning(namespace string) error {
	// poll pod every 5 secs for 2 mins until it's running or timeout occurs
	return wait.PollImmediate(time.Second*5, time.Minute*2, isVeleroPodRunning(namespace))
}

func isVeleroCRFailed(namespace string, instanceName string) wait.ConditionFunc {
	kubeConfig := getKubeConfig()

	// create dynamic client for CR
	client, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil
	}
	veleroClient, err := createVeleroClient(client, namespace)
	if err != nil {
		return nil
	}
	return func() (bool, error) {
		// Get velero CR in cluster
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

func waitForFailedVeleroCR(namespace string, instanceName string) error {
	return wait.PollImmediate(time.Second*5, time.Minute*2, isVeleroCRFailed(namespace, instanceName))
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

func getCredsData() []byte {
	// pass in aws credentials by cli flag
	// from cli:  -cloud=<"filepath">
	// go run main.go -cloud="/Users/emilymcmullan/.aws/credentials"
	cloud := flag.String("cloud", "", "file path for aws credentials")
	flag.Parse()

	// save passed in cred file as []byte
	credsFile, err := ioutil.ReadFile(*cloud)
	if err != nil {
		panic(err)
	}
	return credsFile
}

func createSecret(data []byte) error {
	config := getKubeConfig()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloud-credentials",
			Namespace: "oadp-operator",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: metav1.SchemeGroupVersion.String(),
		},
		Data: map[string][]byte{
			"cloud": data,
		},
		Type: corev1.SecretTypeOpaque,
	}
	_, errors := clientset.CoreV1().Secrets("oadp-operator").Create(context.TODO(), &secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(errors) {
		fmt.Println("Secret already exists in this namespace")
		return nil
	}
	return err
}

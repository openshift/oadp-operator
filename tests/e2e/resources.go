package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

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
		// get pods in the oadp-operator-e2e namespace
		podList, err := clientset.CoreV1().Pods("oadp-operator").List(context.TODO(), veleroOptions)
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

func waitForVeleroPodRunning() error {
	// poll pod every 5 secs for 2 mins until it's running or timeout occurs
	return wait.PollImmediate(time.Second*5, time.Minute*2, isVeleroPodRunning())
}

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

func waitForFailedVeleroCR() error {
	return wait.PollImmediate(time.Second*5, time.Minute*2, isVeleroCRFailed())
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

func addResticNodeSelector() error {
	kubeConf := getKubeConfig()
	// create client for daemonset
	clientset, err := kubernetes.NewForConfig(kubeConf)
	if err != nil {
		return err
	}

	// get daemonset in oadp-operator-e2e ns with specified field selector
	resticDaemeonSet, err := clientset.AppsV1().DaemonSets("oadp-operator").Get(context.Background(), "restic", metav1.GetOptions{})
	if err != nil {
		return err
	}
	// ***** TODO *****
	// update Restic daemonset to have node selector
	resticOptions := metav1.UpdateOptions{}

	_, err = clientset.AppsV1().DaemonSets("oadp-operator").Update(context.Background(), resticDaemeonSet, resticOptions)
	if err != nil {
		return err
	}
	fmt.Println("spec 'enable_restic' has been updated to false")
	return nil
}

type patchStringValue struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

func addNodeSelector() error {
	kubeConf := getKubeConfig()
	// create client for daemonset
	clientset, err := kubernetes.NewForConfig(kubeConf)
	if err != nil {
		return err
	}
	api := clientset.CoreV1()

	// select Velero pod with this label
	veleroOptions := metav1.ListOptions{
		LabelSelector: "component=velero",
	}
	// get pods in the oadp-operator-e2e namespace
	podList, _ := clientset.CoreV1().Pods("oadp-operator").List(context.TODO(), veleroOptions)
	for _, podList := range podList.Items {
		fmt.Printf("%s, Phase: %s, Message: %s", podList.GetName(), string(podList.Status.Phase), string(podList.Status.Message))

		payload := []patchStringValue{{
			Op:    "add",
			Path:  "/spec/nodeSelector/restic_test",
			Value: "test-node-selector",
		}}
		payloadBytes, _ := json.Marshal(payload)

		_, err = api.Pods("oadp-operator").Patch(context.Background(), podList.GetName(), types.JSONPatchType, payloadBytes, metav1.PatchOptions{})
		if err == nil {
			fmt.Printf("Pod %s labelled successfully.", podList.GetName())
		} else {
			return err
		}
	}
	return nil
}

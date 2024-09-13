package lib

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

type ProxyPodParameters struct {
	KubeClient    *kubernetes.Clientset
	KubeConfig    *rest.Config
	Namespace     string
	PodName       string
	ContainerName string
}

func CreateNamespace(clientset *kubernetes.Clientset, namespace string) error {
	ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	_, err := clientset.CoreV1().Namespaces().Create(context.Background(), &ns, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func DoesNamespaceExist(clientset *kubernetes.Clientset, namespace string) (bool, error) {
	_, err := clientset.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return true, nil
}

func DeleteNamespace(clientset *kubernetes.Clientset, namespace string) error {
	err := clientset.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func IsNamespaceDeleted(clientset *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		_, err := clientset.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
}

func CreateCredentialsSecret(clientset *kubernetes.Clientset, data []byte, namespace string, credSecretRef string) error {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      credSecretRef,
			Namespace: namespace,
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
	_, err := clientset.CoreV1().Secrets(namespace).Create(context.Background(), &secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func DeleteSecret(clientset *kubernetes.Clientset, namespace string, credSecretRef string) error {
	err := clientset.CoreV1().Secrets(namespace).Delete(context.Background(), credSecretRef, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// ExecuteCommandInPodsSh executes a command in a Kubernetes pod using the provided parameters.
//
// Parameters:
//   - params: ProxyPodParameters - Parameters specifying Connection to the Kubernetes, the pod, namespace, and container details.
//   - command: string - The command to be executed in the specified pod.
//
// Returns:
//   - string: Standard output of the executed command.
//   - string: Standard error output of the executed command.
//   - error: An error, if any, that occurred during the execution of the command.
//
// The function logs relevant information, such as the provided command, the pod name, container name,
// and the full command URL before initiating the command execution. It streams the command's standard
// output and error output, logging them if available. In case of errors, it returns an error message
// with details about the issue.
func ExecuteCommandInPodsSh(params ProxyPodParameters, command string) (string, string, error) {

	var containerName string

	kubeClient := params.KubeClient
	kubeConfig := params.KubeConfig

	if command == "" {
		return "", "", fmt.Errorf("No command specified")
	}

	if kubeClient == nil {
		return "", "", fmt.Errorf("No valid kubernetes.Clientset provided")
	}

	if kubeConfig == nil {
		return "", "", fmt.Errorf("No valid rest.Config provided")
	}

	if params.PodName == "" {
		return "", "", fmt.Errorf("No proxy pod specified for the command: %s", command)
	}

	if params.Namespace == "" {
		return "", "", fmt.Errorf("No proxy pod namespace specified for the command: %s", command)
	}

	if params.ContainerName != "" {
		containerName = params.ContainerName
	} else {
		containerName = "curl-tool"
	}

	log.Printf("Provided command: %s\n", command)
	log.Printf("Command will run in the pod: %s, container: %s in the namespace: %s\n", params.PodName, containerName, params.Namespace)

	option := &corev1.PodExecOptions{
		Command:   strings.Split(command, " "),
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
		Container: containerName,
	}

	postRequest := kubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(params.PodName).
		Namespace(params.Namespace).
		SubResource("exec").
		VersionedParams(option, scheme.ParameterCodec)

	log.Printf("Full command URL: %s\n", postRequest.URL())

	executor, err := remotecommand.NewSPDYExecutor(kubeConfig, "POST", postRequest.URL())
	if err != nil {
		return "", "", err
	}

	stdOutput := &bytes.Buffer{}
	stdErrOutput := &bytes.Buffer{}

	err = executor.Stream(remotecommand.StreamOptions{Stdout: stdOutput, Stderr: stdErrOutput})

	if stdOutput.Len() > 0 {
		log.Printf("stdOutput: %s\n", stdOutput.String())
	}

	if stdErrOutput.Len() > 0 {
		log.Printf("stdErrOutput: %s\n", stdErrOutput.String())
	}

	if err != nil {
		log.Printf("Error while streaming command output: %v\n", err)
		return stdOutput.String(), stdErrOutput.String(), fmt.Errorf("Error while streaming command output: %v", err)
	}

	return stdOutput.String(), stdErrOutput.String(), nil
}

// GetFirstPodByLabel retrieves a first found pod in the specified namespace based on the given label selector.
// It uses the provided Kubernetes client to interact with the Kubernetes API.
//
// Parameters:
//   - clientset: A pointer to the Kubernetes client (*kubernetes.Clientset).
//   - namespace: The namespace in which to search for the pod.
//   - labelSelector: The label selector to filter pods.
//
// Returns:
//   - (*corev1.Pod, error): A pointer to the first pod matching the label selector, or an error if any.
func GetFirstPodByLabel(clientset *kubernetes.Clientset, namespace string, labelSelector string) (*corev1.Pod, error) {
	podList, err := GetAllPodsWithLabel(clientset, namespace, labelSelector)
	if err != nil {
		return nil, err
	}

	return &podList.Items[0], nil
}

func SavePodLogs(clientset *kubernetes.Clientset, namespace, dir string) error {
	podList, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil
	}
	for _, pod := range podList.Items {
		podDir := fmt.Sprintf("%s/%s/%s", dir, namespace, pod.Name)
		err = os.MkdirAll(podDir, 0755)
		if err != nil {
			log.Printf("Error creating pod directory: %v", err)
		}
		for _, container := range pod.Spec.Containers {
			logs, err := GetPodContainerLogs(clientset, namespace, pod.Name, container.Name)
			if err != nil {
				return nil
			}
			err = os.WriteFile(podDir+"/"+container.Name+".log", []byte(logs), 0644)
			if err != nil {
				log.Printf("Error writing pod logs: %v", err)
			}
		}
	}
	return nil
}

func GetPodContainerLogs(clientset *kubernetes.Clientset, namespace, podname, container string) (string, error) {
	req := clientset.CoreV1().Pods(namespace).GetLogs(podname, &corev1.PodLogOptions{
		Container: container,
	})
	podLogs, err := req.Stream(context.Background())
	if err != nil {
		return "", err
	}
	defer podLogs.Close()
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func GetAllPodsWithLabel(c *kubernetes.Clientset, namespace string, LabelSelector string) (*corev1.PodList, error) {
	podList, err := c.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: LabelSelector})
	if err != nil {
		return nil, err
	}
	if len(podList.Items) == 0 {
		log.Println("no Pod found")
		return nil, fmt.Errorf("no Pod found")
	}
	return podList, nil
}

func GetPodWithLabel(c *kubernetes.Clientset, namespace string, LabelSelector string) (*corev1.Pod, error) {
	podList, err := GetAllPodsWithLabel(c, namespace, LabelSelector)
	if err != nil {
		return nil, err
	}
	if len(podList.Items) > 1 {
		log.Println("more than one Pod found")
		return nil, fmt.Errorf("more than one Pod found")
	}
	return &podList.Items[0], nil
}

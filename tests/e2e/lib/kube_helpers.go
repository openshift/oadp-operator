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
)

func CreateNamespace(clientset *kubernetes.Clientset, namespace string) error {
	ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	_, err := clientset.CoreV1().Namespaces().Create(context.Background(), &ns, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func DeleteNamespace(clientset *kubernetes.Clientset, namespace string) error {
	err := clientset.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
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

func isCredentialsSecretDeleted(clientset *kubernetes.Clientset, namespace string, credSecretRef string) wait.ConditionFunc {
	return func() (bool, error) {
		_, err := clientset.CoreV1().Secrets(namespace).Get(context.Background(), credSecretRef, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			log.Printf("Secret in test namespace has been deleted")
			return true, nil
		}
		log.Printf("Secret still exists in namespace")
		return false, err
	}
}

func GetPodWithPrefixContainerLogs(clientset *kubernetes.Clientset, namespace string, podPrefix string, container string) (string, error) {
	podList, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	for _, pod := range podList.Items {
		if strings.HasPrefix(pod.Name, podPrefix) {
			logs, err := GetPodContainerLogs(clientset, namespace, pod.Name, container)
			if err != nil {
				return "", err
			}
			return logs, nil
		}
	}
	return "", fmt.Errorf("No pod found with prefix %s", podPrefix)
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

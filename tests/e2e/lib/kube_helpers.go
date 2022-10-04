package lib

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type K8sVersion struct {
	Major string
	Minor string
}

var (
	// Version struct representing OCP 4.8.x https://docs.openshift.com/container-platform/4.8/release_notes/ocp-4-8-release-notes.html
	K8sVersionOcp48 = K8sVersion{
		Major: "1",
		Minor: "21",
	}
	// https://docs.openshift.com/container-platform/4.7/release_notes/ocp-4-7-release-notes.html
	K8sVersionOcp47 = K8sVersion{
		Major: "1",
		Minor: "20",
	}
)

func k8sVersionGreater(v1 *K8sVersion, v2 *K8sVersion) bool {
	if v1.Major > v2.Major {
		return true
	}
	if v1.Major == v2.Major {
		return v1.Minor > v2.Minor
	}
	return false
}

func k8sVersionLesser(v1 *K8sVersion, v2 *K8sVersion) bool {
	if v1.Major < v2.Major {
		return true
	}
	if v1.Major == v2.Major {
		return v1.Minor < v2.Minor
	}
	return false
}

func serverK8sVersion() *K8sVersion {
	version, err := serverVersion()
	if err != nil {
		return nil
	}
	return &K8sVersion{Major: version.Major, Minor: version.Minor}
}

func NotServerVersionTarget(minVersion *K8sVersion, maxVersion *K8sVersion) (bool, string) {
	serverVersion := serverK8sVersion()
	if maxVersion != nil && k8sVersionGreater(serverVersion, maxVersion) {
		return true, "Server Version is greater than max target version"
	}
	if minVersion != nil && k8sVersionLesser(serverVersion, minVersion) {
		return true, "Server Version is lesser than min target version"
	}
	return false, ""
}

func setUpClient() (*kubernetes.Clientset, error) {
	kubeConf := getKubeConfig()
	// create client for pod
	clientset, err := kubernetes.NewForConfig(kubeConf)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

// FIXME: Remove
func createOADPTestNamespace(namespace string) error {
	// default OADP Namespace
	kubeConf := getKubeConfig()
	clientset, err := kubernetes.NewForConfig(kubeConf)
	if err != nil {
		return err
	}
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), &ns, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}

	return err
}

// FIXME: Remove
func deleteOADPTestNamespace(namespace string) error {
	// default OADP Namespace
	kubeConf := getKubeConfig()
	clientset, err := kubernetes.NewForConfig(kubeConf)

	if err != nil {
		return err
	}
	err = clientset.CoreV1().Namespaces().Delete(context.TODO(), namespace, metav1.DeleteOptions{})
	return err
}

func getKubeConfig() *rest.Config {
	return config.GetConfigOrDie()
}

// FIXME: Remove
func DoesNamespaceExist(namespace string) (bool, error) {
	clientset, err := setUpClient()
	if err != nil {
		return false, err
	}
	_, err = clientset.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return true, nil
}

// Keeping it for now.
func IsNamespaceDeleted(namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}
		_, err = clientset.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})
		if err != nil {
			return true, nil
		}
		return false, err
	}
}

func serverVersion() (*version.Info, error) {
	clientset, err := setUpClient()
	if err != nil {
		return nil, err
	}
	return clientset.Discovery().ServerVersion()
}

func CreateCredentialsSecret(data []byte, namespace string, credSecretRef string) error {
	clientset, err := setUpClient()
	if err != nil {
		return err
	}
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
	_, err = clientset.CoreV1().Secrets(namespace).Create(context.TODO(), &secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func DeleteSecret(namespace string, credSecretRef string) error {
	clientset, err := setUpClient()
	if err != nil {
		return err
	}
	err = clientset.CoreV1().Secrets(namespace).Delete(context.Background(), credSecretRef, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func isCredentialsSecretDeleted(namespace string, credSecretRef string) wait.ConditionFunc {
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}
		_, err = clientset.CoreV1().Secrets(namespace).Get(context.Background(), credSecretRef, metav1.GetOptions{})
		if err != nil {
			log.Printf("Secret in test namespace has been deleted")
			return true, nil
		}
		log.Printf("Secret still exists in namespace")
		return false, err
	}
}

func GetPodWithPrefixContainerLogs(namespace string, podPrefix string, container string) (string, error) {
	clientset, err := setUpClient()
	if err != nil {
		return "", err
	}
	podList, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	for _, pod := range podList.Items {
		if strings.HasPrefix(pod.Name, podPrefix) {
			logs, err := GetPodContainerLogs(namespace, pod.Name, container)
			if err != nil {
				return "", err
			}
			return logs, nil
		}
	}
	return "", fmt.Errorf("No pod found with prefix %s", podPrefix)
}

func GetPodContainerLogs(namespace, podname, container string) (string, error) {
	clientset, err := setUpClient()
	if err != nil {
		return "", err
	}
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
	str := buf.String()
	return str, nil
}

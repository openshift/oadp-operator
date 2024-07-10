package lib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift/oadp-operator/pkg/common"
)

func recoverFromPanicLogs(c *kubernetes.Clientset, veleroNamespace string, panicReason interface{}, panicFrom string) string {
	log.Printf("Recovered from panic in %s: %v\n", panicFrom, panicReason)
	log.Print("returning container logs instead")
	containerLogs, err := GetVeleroContainerLogs(c, veleroNamespace)
	if err != nil {
		log.Printf("error getting container logs: %v\n", err)
	}
	return containerLogs
}

func errorLogsExcludingIgnored(logs string) []string {
	errorRegex, err := regexp.Compile("error|Error")
	if err != nil {
		return []string{"could not compile regex: ", err.Error()}
	}
	logLines := []string{}
	for _, line := range strings.Split(logs, "\n") {
		if errorRegex.MatchString(line) {
			// ignore some expected errors
			ignoreLine := false
			for _, ignore := range errorIgnorePatterns {
				ignoreLine, _ = regexp.MatchString(ignore, line)
				if ignoreLine {
					break
				}
			}
			if !ignoreLine {
				logLines = append(logLines, line)
			}
		}
	}
	return logLines
}

func GetVeleroDeployment(c *kubernetes.Clientset, namespace string) (*appsv1.Deployment, error) {
	deployment, err := c.AppsV1().Deployments(namespace).Get(context.Background(), common.Velero, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return deployment, nil
}

func GetVeleroPod(c *kubernetes.Clientset, namespace string) (*corev1.Pod, error) {
	pod, err := GetPodWithLabel(c, namespace, "deploy=velero,component=velero,!job-name")
	if err != nil {
		return nil, err
	}
	return pod, nil
}

func VeleroPodIsRunning(c *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := GetVeleroPod(c, namespace)
		if err != nil {
			return false, err
		}
		if pod.Status.Phase != corev1.PodRunning {
			log.Printf("velero Pod phase is %v", pod.Status.Phase)
			return false, nil
		}
		log.Printf("velero Pod phase is %v", corev1.PodRunning)
		return true, nil
	}
}

func VeleroPodIsUpdated(c *kubernetes.Clientset, namespace string, updateTime time.Time) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := GetVeleroPod(c, namespace)
		if err != nil {
			return false, err
		}
		return pod.CreationTimestamp.After(updateTime), nil
	}
}

func VeleroIsDeleted(c *kubernetes.Clientset, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		_, err := GetVeleroPod(c, namespace)
		if err == nil || err.Error() != "no Pod found" {
			return false, err
		}
		_, err = GetVeleroDeployment(c, namespace)
		if err == nil || !apierrors.IsNotFound(err) {
			return false, err
		}
		return true, nil
	}
}

// check velero tolerations
func VerifyVeleroTolerations(c *kubernetes.Clientset, namespace string, t []corev1.Toleration) wait.ConditionFunc {
	return func() (bool, error) {
		velero, err := GetVeleroDeployment(c, namespace)
		if err != nil {
			return false, err
		}

		if !reflect.DeepEqual(t, velero.Spec.Template.Spec.Tolerations) {
			return false, errors.New("given Velero tolerations does not match the deployed velero tolerations")
		}
		return true, nil
	}
}

// check for velero resource requests
func VerifyVeleroResourceRequests(c *kubernetes.Clientset, namespace string, requests corev1.ResourceList) wait.ConditionFunc {
	return func() (bool, error) {
		velero, err := GetVeleroDeployment(c, namespace)
		if err != nil {
			return false, err
		}

		for _, container := range velero.Spec.Template.Spec.Containers {
			if container.Name == common.Velero {
				if !reflect.DeepEqual(requests, container.Resources.Requests) {
					return false, errors.New("given Velero resource requests do not match the deployed velero resource requests")
				}
			}
		}
		return true, nil
	}
}

// check for velero resource limits
func VerifyVeleroResourceLimits(c *kubernetes.Clientset, namespace string, limits corev1.ResourceList) wait.ConditionFunc {
	return func() (bool, error) {
		velero, err := GetVeleroDeployment(c, namespace)
		if err != nil {
			return false, err
		}

		for _, container := range velero.Spec.Template.Spec.Containers {
			if container.Name == common.Velero {
				if !reflect.DeepEqual(limits, container.Resources.Limits) {
					return false, errors.New("given Velero resource limits do not match the deployed velero resource limits")
				}
			}
		}
		return true, nil
	}
}

// Returns logs from velero container on velero pod
func GetVeleroContainerLogs(c *kubernetes.Clientset, namespace string) (string, error) {
	velero, err := GetVeleroPod(c, namespace)
	if err != nil {
		return "", err
	}
	logs, err := GetPodContainerLogs(c, namespace, velero.Name, common.Velero)
	if err != nil {
		return "", err
	}
	return logs, nil
}

func GetVeleroContainerFailureLogs(c *kubernetes.Clientset, namespace string) []string {
	containerLogs, err := GetVeleroContainerLogs(c, namespace)
	if err != nil {
		log.Printf("cannot get velero container logs")
		return nil
	}
	containerLogsArray := strings.Split(containerLogs, "\n")
	var failureArr = []string{}
	for i, line := range containerLogsArray {
		if strings.Contains(line, "level=error") {
			failureArr = append(failureArr, fmt.Sprintf("velero container error line#%d: "+line+"\n", i))
		}
	}
	return failureArr
}

func RunDcPostRestoreScript(dcRestoreName string) error {
	log.Printf("Running post restore script for %s", dcRestoreName)
	currentDir, err := os.Getwd()
	if err != nil {
		return err
	}
	currentDir = strings.TrimSuffix(currentDir, "/tests/e2e")
	var stderrOutput bytes.Buffer
	command := exec.Command("bash", currentDir+"/docs/scripts/dc-post-restore.sh", dcRestoreName)
	command.Stderr = &stderrOutput
	stdOut, err := command.Output()
	log.Printf("command: %s", command.String())
	log.Printf("stdout:\n%s", stdOut)
	log.Printf("stderr:\n%s", stderrOutput.String())
	log.Printf("err: %v", err)
	return err
}

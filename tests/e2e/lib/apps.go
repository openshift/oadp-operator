package lib

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/onsi/ginkgo/v2"
	ocpappsv1 "github.com/openshift/api/apps/v1"
	security "github.com/openshift/api/security/v1"
	templatev1 "github.com/openshift/api/template/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func InstallApplication(ocClient client.Client, file string) error {
	template, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	obj := &unstructured.UnstructuredList{}

	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err = dec.Decode([]byte(template), nil, obj)
	if err != nil {
		return err
	}
	for _, resource := range obj.Items {
		err = ocClient.Create(context.Background(), &resource)
		if apierrors.IsAlreadyExists(err) {
			continue
		} else if err != nil {
			return err
		}
	}
	return nil
}

func DoesSCCExist(ocClient client.Client, sccName string) (bool, error) {
	scc := security.SecurityContextConstraints{}
	err := ocClient.Get(context.Background(), client.ObjectKey{
		Name: sccName,
	}, &scc)
	if err != nil {
		return false, err
	}
	return true, nil

}

func UninstallApplication(ocClient client.Client, file string) error {
	template, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	obj := &unstructured.UnstructuredList{}

	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err = dec.Decode([]byte(template), nil, obj)
	if err != nil {
		return err
	}
	for _, resource := range obj.Items {
		err = ocClient.Delete(context.Background(), &resource)
		if apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return err
		}
	}
	return nil
}

// func AreApplicationPodsRunning(namespace string) wait.ConditionFunc {
// 	return func() (bool, error) {
// 		clientset, err := setUpClient()
// 		if err != nil {
// 			return false, err
// 		}
// 		// select Velero pod with this label
// 		veleroOptions := metav1.ListOptions{
// 			LabelSelector: "e2e-app=true",
// 		}
// 		// get pods in test namespace with labelSelector
// 		podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), veleroOptions)
// 		if err != nil {
// 			return false, nil
// 		}
// 		if len(podList.Items) == 0 {
// 			return false, nil
// 		}
// 		// get pod name and status with specified label selector
// 		for _, podInfo := range podList.Items {
// 			phase := podInfo.Status.Phase
// 			if phase != corev1.PodRunning && phase != corev1.PodSucceeded {
// 				ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("Pod %v not yet succeeded", podInfo.Name)))
// 				ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("status: %v", podInfo.Status)))
// 				return false, nil
// 			}
// 		}
// 		return true, err
// 	}
// }
func HasDCsInNamespace(ocClient client.Client, namespace string) (bool, error) {
	dcList := &ocpappsv1.DeploymentConfigList{}
	err := ocClient.List(context.Background(), dcList, client.InNamespace(namespace))
	if err!= nil {
		return false, err
	}
	if len(dcList.Items) == 0 {
		return false, nil
	}
	return true, nil
}

func HasReplicationControllersInNamespace(ocClient client.Client, namespace string) (bool, error) {
	rcList := &corev1.ReplicationControllerList{}
	err := ocClient.List(context.Background(), rcList, client.InNamespace(namespace))
	if err!= nil {
		return false, err
	}
	if len(rcList.Items) == 0 {
		return false, nil
	}
	return true, nil
}

func HasTemplateInstancesInNamespace(ocClient client.Client, namespace string) (bool, error) {
	tiList := &templatev1.TemplateInstanceList{}
	err := ocClient.List(context.Background(), tiList, client.InNamespace(namespace))
	if err!= nil {
		return false, err
	}
	if len(tiList.Items) == 0 {
		return false, nil
	}
	return true, nil
}

func NamespaceRequiresResticDCWorkaround(ocClient client.Client, namespace string) (bool, error) {
	hasDC ,err := HasDCsInNamespace(ocClient, namespace)
	if err != nil {
		return false, err
	}
	hasRC, err := HasReplicationControllersInNamespace(ocClient, namespace)
	if err!= nil {
		return false, err
	}
	hasTI, err := HasTemplateInstancesInNamespace(ocClient, namespace)
	if err!= nil {
		return false, err
	}
	return hasDC || hasRC || hasTI, nil
}

func IsDCReady(ocClient client.Client, namespace, dcName string) wait.ConditionFunc {
	return func() (bool, error) {
		dc := ocpappsv1.DeploymentConfig{}
		err := ocClient.Get(context.Background(), client.ObjectKey{
			Namespace: namespace,
			Name:      dcName,
		}, &dc)
		if err != nil {
			return false, err
		}
		if dc.Status.AvailableReplicas != dc.Status.Replicas || dc.Status.Replicas == 0 {
			for _, condition := range dc.Status.Conditions {
				if len(condition.Message) > 0 {
					ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("DC not available with condition: %s\n", condition.Message)))
				}
			}
			return false, errors.New("DC is not in a ready state")
		}
		return true, nil
	}
}

func IsDeploymentReady(ocClient client.Client, namespace, dName string) wait.ConditionFunc {
	return func() (bool, error) {
		deployment := appsv1.Deployment{}
		err := ocClient.Get(context.Background(), client.ObjectKey{
			Namespace: namespace,
			Name:      dName,
		}, &deployment)
		if err != nil {
			return false, err
		}
		if deployment.Status.AvailableReplicas != deployment.Status.Replicas || deployment.Status.Replicas == 0 {
			for _, condition := range deployment.Status.Conditions {
				if len(condition.Message) > 0 {
					ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("deployment not available with condition: %s\n", condition.Message)))
				}
			}
			return false, errors.New("deployment is not in a ready state")
		}
		return true, nil
	}
}

func AreApplicationPodsRunning(namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}
		// select Velero pod with this label
		veleroOptions := metav1.ListOptions{
			LabelSelector: "e2e-app=true",
		}
		// get pods in test namespace with labelSelector
		podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), veleroOptions)
		if err != nil {
			return false, nil
		}
		if len(podList.Items) == 0 {
			return false, nil
		}
		// get pod name and status with specified label selector
		for _, podInfo := range podList.Items {
			phase := podInfo.Status.Phase
			if phase != corev1.PodRunning && phase != corev1.PodSucceeded {
				ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("Pod %v not yet succeeded", podInfo.Name)))
				ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("status: %v", podInfo.Status)))
				return false, nil
			}
		}
		return true, err
	}
}

func RunMustGather(oc_cli string, artifact_dir string) error {
	ocClient := oc_cli
	ocAdmin := "adm"
	mustGatherCmd := "must-gather"
	mustGatherImg := "--image=quay.io/konveyor/oadp-must-gather:latest"
	destDir := "--dest-dir=" + artifact_dir
	logCmdPmt := "--"
	logCmd := "gather_logs_without_zip"

	cmd := exec.Command(ocClient, ocAdmin, mustGatherCmd, mustGatherImg, destDir, logCmdPmt, logCmd)
	_, err := cmd.Output()
	return err
}

package e2e

import (
	"context"
	"errors"
	appsv1 "github.com/openshift/api/apps/v1"
	security "github.com/openshift/api/security/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/wait"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func installApplication(ocClient client.Client, file string) error {
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

func doesSCCExist(ocClient client.Client, sccName string) (bool, error) {
	scc := security.SecurityContextConstraints{}
	err := ocClient.Get(context.Background(), client.ObjectKey{
		Name: sccName,
	}, &scc)
	if err != nil {
		return false, err
	}
	return true, nil

}

func uninstallApplication(ocClient client.Client, file string) error {
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

func areApplicationPodsRunning(namespace string) wait.ConditionFunc {
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
			status := podInfo.Status.Phase
			if status != corev1.PodRunning && status != corev1.PodSucceeded {
				return false, nil
			}
		}
		return true, err
	}
}

func isDCReady(ocClient client.Client, namespace, dc string) wait.ConditionFunc {
	return func() (bool, error) {
		dc := appsv1.DeploymentConfig{}
		err := ocClient.Get(context.Background(), client.ObjectKey{
			Namespace: namespace,
			Name:      "restify",
		}, &dc)
		if err != nil {
			return false, err
		}
		if dc.Status.AvailableReplicas != dc.Status.Replicas || dc.Status.Replicas == 0 {
			return false, errors.New("DC is not in a ready state")
		}
		return true, nil
	}
}

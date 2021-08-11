package e2e

import (
	"context"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func doesPluginExist(namespace string, deploymentName string, pluginName string) wait.ConditionFunc {
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}
		veleroDeployment, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		// loop over initContainers and get names
		for _, container := range veleroDeployment.Spec.Template.Spec.InitContainers {
			name := container.Name
			if name == pluginName {
				return true, nil
			}
		}
		return false, err
	}
}

func doesVeleroDeploymentExist(namespace string, deploymentName string) wait.ConditionFunc {
	log.Printf("Waiting for velero deployment to be created...")
	return func() (bool, error) {
		client, err := setUpClient()
		if err != nil {
			return false, err
		}
		// Check for deployment
		_, err = client.AppsV1().Deployments(namespace).Get(context.Background(), deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return true, nil
	}
}

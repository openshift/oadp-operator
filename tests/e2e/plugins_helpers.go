package e2e

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
)

func removeVeleroPlugin(namespace string, instanceName string, pluginValues []string, removedPlugin string) error {
	veleroClient, err := setUpDynamicVeleroClient(namespace)
	if err != nil {
		return nil
	}
	// get Velero as unstructured type
	veleroResource, err := veleroClient.Get(context.Background(), instanceName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	// remove aws from default_plugins
	err = unstructured.SetNestedStringSlice(veleroResource.Object, pluginValues, "spec", "default_velero_plugins")
	if err != nil {
		return err
	}
	_, err = veleroClient.Update(context.Background(), veleroResource, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	fmt.Printf("%s plugin has been removed\n", removedPlugin)
	return nil
}

func doesPluginExist(namespace string, deploymentName string, pluginName string) wait.ConditionFunc {
	fmt.Printf("Checking if %s exists\n", pluginName)
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
				fmt.Printf("%s exists\n", pluginName)
				return true, nil
			}
		}
		fmt.Printf("%s does not exist\n", pluginName)
		return false, err
	}
}

func doesVeleroDeploymentExist(namespace string, deploymentName string) wait.ConditionFunc {
	return func() (bool, error) {
		client, err := setUpClient()
		if err != nil {
			return false, err
		}
		// Check for deployment
		_, err = client.AppsV1().Deployments(namespace).Get(context.Background(), deploymentName, metav1.GetOptions{})
		if err != nil {
			fmt.Println("Velero deployment does not yet exist...")
			return false, err
		}
		fmt.Println("Velero deployment now exists")
		return true, nil
	}
}

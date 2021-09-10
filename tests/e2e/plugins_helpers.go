package e2e

import (
	"context"
	"log"

	"github.com/openshift/oadp-operator/api/v1alpha1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (v *veleroCustomResource) removeVeleroPlugin(namespace string, instanceName string, pluginValues []v1alpha1.DefaultPlugin, removedPlugin string) error {
	err := v.SetClient()
	if err != nil {
		return err
	}
	velero := &oadpv1alpha1.Velero{}
	err = v.Client.Get(context.Background(), client.ObjectKey{
		Namespace: v.Namespace,
		Name:      v.Name,
	}, velero)
	if err != nil {
		return err
	}
	// remove plugin from default_plugins
	velero.Spec.DefaultVeleroPlugins = pluginValues

	err = v.Client.Update(context.Background(), velero)
	if err != nil {
		return err
	}
	log.Printf("%s plugin has been removed\n", removedPlugin)
	return nil
}

func doesPluginExist(namespace string, pluginName string) wait.ConditionFunc {
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}
		veleroDeployment, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), "velero", metav1.GetOptions{})
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

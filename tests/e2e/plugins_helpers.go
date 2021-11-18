package e2e

import (
	"context"
	"log"

	"github.com/openshift/oadp-operator/api/v1alpha1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (v *dpaCustomResource) removeVeleroPlugin(namespace string, instanceName string, pluginValues []v1alpha1.DefaultPlugin, removedPlugin string) error {
	err := v.SetClient()
	if err != nil {
		return err
	}
	dpa := &oadpv1alpha1.DataProtectionApplication{}
	err = v.Client.Get(context.Background(), client.ObjectKey{
		Namespace: v.Namespace,
		Name:      v.Name,
	}, dpa)
	if err != nil {
		return err
	}
	// remove plugin from default_plugins
	dpa.Spec.Configuration.Velero.DefaultPlugins = pluginValues

	err = v.Client.Update(context.Background(), dpa)
	if err != nil {
		return err
	}
	log.Printf("%s plugin has been removed\n", removedPlugin)
	return nil
}

func doesPluginExist(namespace string, plugin oadpv1alpha1.DefaultPlugin) wait.ConditionFunc {
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
			if p, ok := credentials.PluginSpecificFields[plugin]; ok {
				if container.Name == p.PluginName {
					return true, nil
				}
			}
		}
		return false, err
	}
}

func doesCustomPluginExist(namespace string, plugin oadpv1alpha1.CustomPlugin) wait.ConditionFunc {
	return func() (bool, error) {
		clientset, err := setUpClient()
		if err != nil {
			return false, err
		}
		veleroDeployment, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), "velero", metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		// loop over initContainers and check for custom plugins

		for _, container := range veleroDeployment.Spec.Template.Spec.InitContainers {
				if container.Name == plugin.Name && container.Image == plugin.Image {
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

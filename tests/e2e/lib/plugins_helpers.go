package lib

import (
	"context"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials"
)

func (d *DpaCustomResource) RemoveVeleroPlugin(c client.Client, string, instanceName string, pluginValues []oadpv1alpha1.DefaultPlugin, removedPlugin string) error {
	err := d.SetClient(c)
	if err != nil {
		return err
	}
	dpa := &oadpv1alpha1.DataProtectionApplication{}
	err = d.Client.Get(context.Background(), client.ObjectKey{
		Namespace: d.Namespace,
		Name:      d.Name,
	}, dpa)
	if err != nil {
		return err
	}
	// remove plugin from default_plugins
	dpa.Spec.Configuration.Velero.DefaultPlugins = pluginValues

	err = d.Client.Update(context.Background(), dpa)
	if err != nil {
		return err
	}
	log.Printf("%s plugin has been removed\n", removedPlugin)
	return nil
}

func DoesPluginExist(c *kubernetes.Clientset, namespace string, plugin oadpv1alpha1.DefaultPlugin) wait.ConditionFunc {
	return func() (bool, error) {
		veleroDeployment, err := c.AppsV1().Deployments(namespace).Get(context.Background(), "velero", metav1.GetOptions{})
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

func DoesCustomPluginExist(c *kubernetes.Clientset, namespace string, plugin oadpv1alpha1.CustomPlugin) wait.ConditionFunc {
	return func() (bool, error) {
		veleroDeployment, err := c.AppsV1().Deployments(namespace).Get(context.Background(), "velero", metav1.GetOptions{})
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

func DoesVeleroDeploymentExist(c *kubernetes.Clientset, namespace string, deploymentName string) wait.ConditionFunc {
	log.Printf("Waiting for velero deployment to be created...")
	return func() (bool, error) {
		// Check for deployment
		_, err := c.AppsV1().Deployments(namespace).Get(context.Background(), deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return true, nil
	}
}

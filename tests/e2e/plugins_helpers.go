package e2e

import (
	"context"
	"log"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (d *dpaCustomResource) removeVeleroPlugin(namespace string, instanceName string, pluginValues []oadpv1alpha1.DefaultPlugin, removedPlugin string) error {
	err := d.SetClient()
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

func doesPluginExist(namespace string, plugin oadpv1alpha1.DefaultPlugin, v *dpaCustomResource) wait.ConditionFunc {
	return func() (bool, error) {
		err := v.SetClient()
		if err != nil {
			return false, err
		}
		dpa, err := v.Get()
		if err != nil {
			return false, err
		}

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
				if container.Name == p.PluginName && dpa.Status.Conditions[0].Status == "True" {
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

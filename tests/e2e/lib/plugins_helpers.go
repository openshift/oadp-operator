package lib

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials"
)

func DoesPluginExist(c *kubernetes.Clientset, namespace string, plugin oadpv1alpha1.DefaultPlugin) wait.ConditionFunc {
	return func() (bool, error) {
		veleroDeployment, err := GetVeleroDeployment(c, namespace)
		if err != nil {
			return false, err
		}
		// loop over initContainers and get names
		if pluginSpecific, ok := credentials.PluginSpecificFields[plugin]; ok {
			for _, container := range veleroDeployment.Spec.Template.Spec.InitContainers {
				if container.Name == pluginSpecific.PluginName {
					return true, nil
				}
			}
			return false, fmt.Errorf("plugin %s does not exist", plugin)
		}
		return false, fmt.Errorf("plugin %s is not valid", plugin)
	}
}

func DoesCustomPluginExist(c *kubernetes.Clientset, namespace string, plugin oadpv1alpha1.CustomPlugin) wait.ConditionFunc {
	return func() (bool, error) {
		veleroDeployment, err := GetVeleroDeployment(c, namespace)
		if err != nil {
			return false, err
		}
		// loop over initContainers and check for custom plugins
		for _, container := range veleroDeployment.Spec.Template.Spec.InitContainers {
			if container.Name == plugin.Name && container.Image == plugin.Image {
				return true, nil
			}
		}
		return false, fmt.Errorf("custom plugin %s does not exist", plugin.Name)
	}
}

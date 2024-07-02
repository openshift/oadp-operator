package lib

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials"
)

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

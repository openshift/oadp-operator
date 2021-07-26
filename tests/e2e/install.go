package e2e

import (
	"fmt"

	"k8s.io/client-go/dynamic"
)

func installDefaultVelero(namespace string, s3Bucket string, credSecretRef string, instanceName string) error {
	kubeConfig := getKubeConfig()

	// create dynamic client for CR
	client, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}
	// get Velero unstruct type to create Velero CR
	unstrVel := getDefaultVeleroConfig(namespace, s3Bucket, credSecretRef, instanceName) //decodeYaml()
	_, err = createDefaultVeleroCR(unstrVel, client, namespace)
	fmt.Println("Default Velero CR created")
	return err
}

func uninstallVelero(namespace string, instanceName string) error {
	kubeConfig := getKubeConfig()

	// create dynamic client for CR
	client, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}
	fmt.Println("Default Velero CR deleted")
	return deleteVeleroCR(client, instanceName, namespace)
}

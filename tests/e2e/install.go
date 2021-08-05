package e2e

import (
	"log"

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
	unstrVel := getDefaultVeleroConfig(namespace, s3Bucket, credSecretRef, instanceName)
	_, err = createDefaultVeleroCR(unstrVel, client, namespace)
	log.Printf("Velero Custom Resource created")
	return err
}

func uninstallVelero(namespace string, instanceName string) error {
	kubeConfig := getKubeConfig()

	// create dynamic client for CR
	client, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}
	log.Printf("Velero Custom Resource deleted")
	return deleteVeleroCR(client, instanceName, namespace)
}

package e2e

import (
	"k8s.io/client-go/dynamic"
)

func installDefaultVelero() error {
	kubeConfig := getKubeConfig()

	// create dynamic client for CR
	client, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}
	// get Velero unstruct type to create Velero CR
	unstrVel := decodeYaml()
	_, err = createDefaultVeleroCR(unstrVel, client)
	return err
}

func uninstallVelero() error {
	kubeConfig := getKubeConfig()

	// create dynamic client for CR
	client, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}
	return deleteVeleroCR(client)
}

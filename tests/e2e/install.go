package e2e

import (
	"k8s.io/client-go/dynamic"
)

func installVelero() error {
	kubeConfig := getKubeConfig()

	// create dynamic client for CR
	client, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}
	// get Velero unstruct type to create Velero CR
	unstrVel := decodeYaml()
	_, err = createVeleroCR(unstrVel, client)
	return err
}

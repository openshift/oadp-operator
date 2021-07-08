package e2e

import (
	"k8s.io/client-go/dynamic"
)

func installVelero() {
	kubeConfig := getKubeConfig()

	// create dynamic client for CR
	client, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		panic(err)
	}
	// get Velero unstruct type to create Velero CR
	unstrVel := decodeYaml()
	createVeleroCR(unstrVel, client)
}

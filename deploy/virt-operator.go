package main

import (
	"fmt"
	"os"
	"time"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
)

// Wrapper around E2E virtualization library, for convenient installation and
// uninstallation of the OpenShift Virtualization Operator.
func main() {
	if len(os.Args) < 2 {
		fmt.Println("Virtualization operator: expected 'install' or 'remove' command line argument.")
		os.Exit(1)
	}

	commands := make(map[string]func(*lib.VirtOperator))

	commands["install"] = func(v *lib.VirtOperator) {
		if err := v.EnsureVirtInstallation(5 * time.Minute); err != nil {
			fmt.Printf("Failed to install operator: %v", err)
			os.Exit(1)
		}
		fmt.Println("Operator installed.")
		os.Exit(0)
	}

	commands["remove"] = func(v *lib.VirtOperator) {
		if err := v.EnsureVirtRemoval(5 * time.Minute); err != nil {
			fmt.Printf("Failed to remove operator: %v", err)
			os.Exit(1)
		}
		fmt.Println("Operator uninstalled.")
		os.Exit(0)
	}
	commands["uninstall"] = commands["remove"]

	operation, ok := commands[os.Args[1]]
	if !ok {
		fmt.Println("Expected 'install' or 'remove' command line argument.")
		os.Exit(1)
	}

	kubeConf := config.GetConfigOrDie()

	kubeClientSet, err := kubernetes.NewForConfig(kubeConf)
	if err != nil {
		fmt.Printf("Failed to get Kubernetes clientset: %v", err)
		os.Exit(1)
	}

	kubeClient, err := client.New(kubeConf, client.Options{})
	if err != nil {
		fmt.Printf("Failed to get Kubernetes client: %v", err)
		os.Exit(1)
	}

	dynamicClient, err := dynamic.NewForConfig(kubeConf)
	if err != nil {
		fmt.Printf("Failed to get Kubernetes dynamic client: %v", err)
		os.Exit(1)
	}

	operatorsv1alpha1.AddToScheme(kubeClient.Scheme())
	operatorsv1.AddToScheme(kubeClient.Scheme())

	v, err := lib.GetVirtOperator(kubeClient, kubeClientSet, dynamicClient)
	if err != nil {
		fmt.Printf("Failed to get client for operator installation: %v", err)
		os.Exit(1)
	}
	operation(v)
}

package main

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const VeleroYAML = `apiVersion: konveyor.openshift.io/v1alpha1
kind: Velero
metadata:
  name: example-velero
spec:
  olm_managed: false
  default_velero_plugins:
  - aws
  - openshift
  - csi
  backup_storage_locations:
  - name: default
    provider: aws
    object_storage:
      bucket: myBucket
      prefix: "velero"
    config:
      region: us-east-1
      profile: "default"
    credentials_secret_ref:
      name: cloud-credentials
      namespace: oadp-operator
  volume_snapshot_locations:
  - name: default
    provider: aws
    config:
      region: us-west-2
      profile: "default"
  enable_restic: true
  velero_feature_flags: EnableCSI`

func main() {
	installVelero()

	// get Velero pod status
	veleroStatus := getPodStatus()
	fmt.Println(veleroStatus)
}

func decodeYaml() *unstructured.Unstructured {
	// set new unstructured type for Velero CR
	unstructVelero := &unstructured.Unstructured{}

	// decode yaml into unstructured type
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode([]byte(VeleroYAML), nil, unstructVelero)
	if err != nil {
		panic(err)
	}
	return unstructVelero
}

func createVeleroClient(res *unstructured.Unstructured, client dynamic.Interface) (dynamic.ResourceInterface, error) {

	resourceClient := client.Resource(schema.GroupVersionResource{
		Group:    "konveyor.openshift.io",
		Version:  "v1alpha1",
		Resource: "veleros",
	})
	namespaceResClient := resourceClient.Namespace("oadp-operator")

	return namespaceResClient, nil
}

func createVeleroCR(res *unstructured.Unstructured, client dynamic.Interface) (unstructured.Unstructured, error) {
	veleroClient, err := createVeleroClient(res, client)
	if err != nil {
		panic(err)
	}
	createdResource, err := veleroClient.Create(context.Background(), res, v1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		fmt.Println("Resource already exists")
	} else if err != nil {
		fmt.Println("Error creating resource")
	} else {
		fmt.Println("Velero resource successfully created")
	}
	return *createdResource, nil
}

func installVelero() {
	// get cluster config
	config, err := clientcmd.BuildConfigFromFlags("", "/Users/emilymcmullan/.agnosticd/ocp4-emcmulla-06232_kubeconfig")
	if err != nil {
		panic(err)
	}
	// create dynamic client
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	// get Velero unstruct type to create Velero CR
	unstrVel := decodeYaml()
	createVeleroCR(unstrVel, client)
}

func getPodStatus() string {
	// get cluster config
	config, err := clientcmd.BuildConfigFromFlags("", "/Users/emilymcmullan/.agnosticd/ocp4-emcmulla-06232_kubeconfig")
	if err != nil {
		panic(err)
	}
	// create client for pod
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	// select Velero pod with this label
	veleroOptions := v1.ListOptions{
		LabelSelector: "component=velero",
	}
	// get pods in the oadp-operator namespace
	podList, err := clientset.CoreV1().Pods("oadp-operator").List(context.TODO(), veleroOptions)
	if err != nil {
		panic(err)
	}
	// get pod name and status with specified label selector
	var status string
	for _, podInfo := range (*podList).Items {
		status = string(podInfo.Status.Phase)
	}
	return status
}

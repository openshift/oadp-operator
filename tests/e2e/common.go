package e2e

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/wait"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const DefaultVeleroConfigYAML = `apiVersion: konveyor.openshift.io/v1alpha1
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
      namespace: oadp-operator-e2e
  volume_snapshot_locations:
  - name: default
    provider: aws
    config:
      region: us-west-2
      profile: "default"
  enable_restic: true
  velero_feature_flags: EnableCSI`

func decodeYaml() *unstructured.Unstructured {
	// set new unstructured type for Velero CR
	unstructVelero := &unstructured.Unstructured{}

	// decode yaml into unstructured type
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode([]byte(DefaultVeleroConfigYAML), nil, unstructVelero)
	if err != nil {
		panic(err)
	}
	return unstructVelero
}

func createOADPTestNamespace() error {
	kubeConf := getKubeConfig()

	// create client for pod
	clientset, err := kubernetes.NewForConfig(kubeConf)
	if err != nil {
		panic(err)
	}
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "oadp-operator-e2e",
		},
	}
	_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), &ns, metav1.CreateOptions{})

	return err
}

func createVeleroClient(res *unstructured.Unstructured, client dynamic.Interface) (dynamic.ResourceInterface, error) {

	resourceClient := client.Resource(schema.GroupVersionResource{
		Group:    "konveyor.openshift.io",
		Version:  "v1alpha1",
		Resource: "veleros",
	})
	namespaceResClient := resourceClient.Namespace("oadp-operator-e2e")

	return namespaceResClient, nil
}

func createVeleroCR(res *unstructured.Unstructured, client dynamic.Interface) (unstructured.Unstructured, error) {
	veleroClient, err := createVeleroClient(res, client)
	if err != nil {
		panic(err)
	}
	createdResource, err := veleroClient.Create(context.Background(), res, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		fmt.Println("Resource already exists")
	} else if err != nil {
		fmt.Println("Error creating resource")
		panic(err)
	} else {
		fmt.Println("Velero resource successfully created")
	}
	return *createdResource, nil
}

func getKubeConfig() *rest.Config {
	return config.GetConfigOrDie()
}

func isPodRunning() wait.ConditionFunc {
	return func() (bool, error) {
		kubeConf := getKubeConfig()

		// create client for pod
		clientset, err := kubernetes.NewForConfig(kubeConf)
		if err != nil {
			panic(err)
		}
		// select Velero pod with this label
		veleroOptions := metav1.ListOptions{
			LabelSelector: "component=velero",
		}
		// get pods in the oadp-operator namespace
		podList, err := clientset.CoreV1().Pods("oadp-operator-e2e").List(context.TODO(), veleroOptions)
		if err != nil {
			panic(err)
		}
		// get pod name and status with specified label selector
		var status string
		for _, podInfo := range (*podList).Items {
			status = string(podInfo.Status.Phase)
		}
		if status == "Running" {
			fmt.Println("Pod is running")
			return true, nil
		}
		return false, err
	}
}

func waitForPodRunning() error {
	// poll pod every 5 secs for 3 mins until it's running or timeout occurs
	return wait.PollImmediate(time.Second*5, time.Minute*3, isPodRunning())
}

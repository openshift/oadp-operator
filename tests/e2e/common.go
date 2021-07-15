package e2e

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"

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
      namespace: oadp-operator
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
	clientset, err := kubernetes.NewForConfig(kubeConf)
	if err != nil {
		return err
	}
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "oadp-operator-e2e",
		},
	}
	_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), &ns, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func createVeleroClient(client dynamic.Interface) (dynamic.ResourceInterface, error) {

	resourceClient := client.Resource(schema.GroupVersionResource{
		Group:    "konveyor.openshift.io",
		Version:  "v1alpha1",
		Resource: "veleros",
	})
	namespaceResClient := resourceClient.Namespace("oadp-operator")

	return namespaceResClient, nil
}

func getKubeConfig() *rest.Config {
	return config.GetConfigOrDie()
}

func createDefaultVeleroCR(res *unstructured.Unstructured, client dynamic.Interface) (*unstructured.Unstructured, error) {
	veleroClient, err := createVeleroClient(client)
	if err != nil {
		return nil, err
	}
	createdResource, err := veleroClient.Create(context.Background(), res, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil, errors.New("found unexpected existing Velero CR")
	} else if err != nil {
		return nil, err
	}
	return createdResource, nil
}

func deleteVeleroCR(client dynamic.Interface) error {
	veleroClient, err := createVeleroClient(client)
	if err != nil {
		return err
	}
	return veleroClient.Delete(context.Background(), "example-velero", metav1.DeleteOptions{})
}

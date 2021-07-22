package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func getDefaultVeleroConfig(namespace string, s3Bucket string, credSecretRef string, instanceName string) *unstructured.Unstructured {
	var veleroSpec = unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "konveyor.openshift.io/v1alpha1",
			"kind":       "Velero",
			"metadata": map[string]interface{}{
				"name":      instanceName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"olm_managed": false,
				"default_velero_plugins": []string{
					"aws",
					"csix",
					"openshift",
				},
				"backup_storage_locations": [](map[string]interface{}){
					map[string]interface{}{
						"config": map[string]interface{}{
							"profile": "default",
							"region":  "us-east-1",
						},
						"credentials_secret_ref": map[string]interface{}{
							"name":      credSecretRef,
							"namespace": "oadp-operator",
						},
						"object_storage": map[string]interface{}{
							"bucket": s3Bucket,
							"prefix": "velero",
						},
						"name":     "default",
						"provider": "aws",
					},
				},
				"velero_feature_flags": "EnableCSI",
				"enable_restic":        true,
				"volume_snapshot_locations": [](map[string]interface{}){
					map[string]interface{}{
						"config": map[string]interface{}{
							"profile": "default",
							"region":  "us-west-2",
						},
						"name":     "default",
						"provider": "aws",
					},
				},
			},
		},
	}
	return &veleroSpec
}

func decodeYaml(DefaultVeleroConfigYAML string) *unstructured.Unstructured {
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

func getJsonData(path string) []byte {
	jsonData, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return jsonData
}

func decodeJson(data []byte) map[string]interface{} {
	// set new unstructured type for Velero CR
	var jsonData map[string]interface{}

	// decode yaml into unstructured type
	err := json.Unmarshal(data, &jsonData)
	if err != nil {
		panic(err)
	}
	return jsonData
}

func createOADPTestNamespace() error {
	kubeConf := getKubeConfig()
	clientset, err := kubernetes.NewForConfig(kubeConf)
	if err != nil {
		return err
	}
	veleroNamespace := flag.String("velero-namespace", "", "Velero Namespace")
	flag.Parse()
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: *veleroNamespace,
		},
	}
	_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), &ns, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}

	return err
}

func createVeleroClient(client dynamic.Interface, namespace string) (dynamic.ResourceInterface, error) {

	resourceClient := client.Resource(schema.GroupVersionResource{
		Group:    "konveyor.openshift.io",
		Version:  "v1alpha1",
		Resource: "veleros",
	})
	namespaceResClient := resourceClient.Namespace(namespace)

	return namespaceResClient, nil
}

func getKubeConfig() *rest.Config {
	return config.GetConfigOrDie()
}

func createDefaultVeleroCR(res *unstructured.Unstructured, client dynamic.Interface, namespace string) (*unstructured.Unstructured, error) {
	veleroClient, err := createVeleroClient(client, namespace)
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

func deleteVeleroCR(client dynamic.Interface, instanceName string, namespace string) error {
	veleroClient, err := createVeleroClient(client, namespace)
	if err != nil {
		return err
	}
	return veleroClient.Delete(context.Background(), instanceName, metav1.DeleteOptions{})
}

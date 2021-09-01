package e2e

import (
	"context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"log"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func installApplication(ocClient client.Client, file string) error {
	template, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	obj := &unstructured.UnstructuredList{}

	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err = dec.Decode([]byte(template), nil, obj)
	if err != nil {
		return err
	}
	for _, resource := range obj.Items {
		err = ocClient.Create(context.Background(), &resource)
		if apierrors.IsAlreadyExists(err) {
			continue
		} else if err != nil {
			return err
		}
	}
	return nil
}

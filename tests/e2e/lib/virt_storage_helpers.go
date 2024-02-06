package lib

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
)

var dataVolumeGVK = schema.GroupVersionResource{
	Group:    "cdi.kubevirt.io",
	Resource: "datavolumes",
	Version:  "v1beta1",
}

func (v *VirtOperator) getDataVolume(namespace, name string) (*unstructured.Unstructured, error) {
	unstructuredDataVolume, err := v.Dynamic.Resource(dataVolumeGVK).Namespace(namespace).Get(context.Background(), name, v1.GetOptions{})
	return unstructuredDataVolume, err
}

func (v *VirtOperator) deleteDataVolume(namespace, name string) error {
	return v.Dynamic.Resource(dataVolumeGVK).Namespace(namespace).Delete(context.Background(), name, v1.DeleteOptions{})
}

func (v *VirtOperator) checkDataVolumeExists(namespace, name string) bool {
	unstructuredDataVolume, err := v.getDataVolume(namespace, name)
	if err != nil {
		return false
	}
	return unstructuredDataVolume != nil
}

// Check the Status.Phase field of the given DataVolume, and make sure it is
// marked "Succeeded".
func (v *VirtOperator) checkDataVolumeReady(namespace, name string) bool {
	unstructuredDataVolume, err := v.getDataVolume(namespace, name)
	if err != nil {
		log.Printf("Error getting DataVolume %s/%s: %v", namespace, name, err)
		return false
	}
	if unstructuredDataVolume == nil {
		return false
	}
	phase, ok, err := unstructured.NestedString(unstructuredDataVolume.UnstructuredContent(), "status", "phase")
	if err != nil {
		log.Printf("Error getting phase from DataVolume: %v", err)
		return false
	}
	if !ok {
		return false
	}
	return phase == "Succeeded"
}

// Create a DataVolume and ask it to fill itself with the contents of the given
// URL. Also add annotations to immediately create and bind to a PersistentVolume,
// and to avoid deleting the DataVolume after the PVC is all ready.
func (v *VirtOperator) createDataVolumeFromUrl(namespace, name, url, size string) error {
	unstructuredDataVolume := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cdi.kubevirt.io/v1beta1",
			"kind":       "DataVolume",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"annotations": map[string]interface{}{
					"cdi.kubevirt.io/storage.bind.immediate.requested": "",
					"cdi.kubevirt.io/storage.deleteAfterCompletion":    "false",
				},
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"http": map[string]interface{}{
						"url": url,
					},
				},
				"pvc": map[string]interface{}{
					"accessModes": []string{
						"ReadWriteOnce",
					},
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"storage": size,
						},
					},
				},
			},
		},
	}

	_, err := v.Dynamic.Resource(dataVolumeGVK).Namespace(v.Namespace).Create(context.Background(), &unstructuredDataVolume, v1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		if strings.Contains(err.Error(), "already exists") {
			return nil
		}
		log.Printf("Error creating DataVolume: %v", err)
		return err
	}

	return nil
}

// Create a DataVolume and wait for it to be ready.
func (v *VirtOperator) EnsureDataVolume(namespace, name, url, size string, timeout time.Duration) error {
	if !v.checkDataVolumeExists(namespace, name) {
		if err := v.createDataVolumeFromUrl(namespace, name, url, size); err != nil {
			return err
		}
		log.Printf("Created DataVolume %s/%s", namespace, name)
	} else {
		log.Printf("DataVolume %s/%s already created, checking for readiness", namespace, name)
	}

	err := wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		return v.checkDataVolumeReady(namespace, name), nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for DataVolume %s/%s to go ready: %w", namespace, name, err)
	}

	log.Printf("DataVolume %s/%s ready", namespace, name)

	return nil
}

// Delete a DataVolume and wait for it to go away.
func (v *VirtOperator) EnsureDataVolumeRemoval(namespace, name string, timeout time.Duration) error {
	err := v.deleteDataVolume(namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("DataVolume %s/%s already removed", namespace, name)
		} else {
			return err
		}
	}

	err = wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		return !v.checkDataVolumeExists(namespace, name), nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for DataVolume %s/%s to be deleted: %w", namespace, name, err)
	}

	log.Printf("DataVolume %s/%s cleaned up", namespace, name)

	return nil
}

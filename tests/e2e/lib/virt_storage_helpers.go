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

func (v *VirtOperator) getDataVolumeSize(namespace, name string) (string, error) {
	unstructuredDataVolume, err := v.getDataVolume(namespace, name)
	if err != nil {
		log.Printf("Error getting DataVolume %s/%s: %v", namespace, name, err)
		return "", err
	}
	if unstructuredDataVolume == nil {
		return "", err
	}
	size, ok, err := unstructured.NestedString(unstructuredDataVolume.UnstructuredContent(), "spec", "pvc", "resources", "requests", "storage")
	if err != nil {
		log.Printf("Error getting size from DataVolume: %v", err)
		return "", err
	}
	if !ok {
		return "", err
	}
	return size, nil
}

// Create a DataVolume, accepting an unstructured source specification.
// Also add annotations to immediately create and bind to a PersistentVolume,
// and to avoid deleting the DataVolume after the PVC is all ready.
func (v *VirtOperator) createDataVolumeFromSource(namespace, name, size string, source map[string]interface{}) error {
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
				"source": source,
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

// Create a DataVolume and ask it to fill itself with the contents of the given URL.
func (v *VirtOperator) createDataVolumeFromUrl(namespace, name, url, size string) error {
	urlSource := map[string]interface{}{
		"http": map[string]interface{}{
			"url": url,
		},
	}
	return v.createDataVolumeFromSource(namespace, name, size, urlSource)
}

// Create a DataVolume as a clone of an existing PVC.
func (v *VirtOperator) createDataVolumeFromPvc(namespace, sourceName, cloneName, size string) error {
	pvcSource := map[string]interface{}{
		"pvc": map[string]interface{}{
			"name":      sourceName,
			"namespace": namespace,
		},
	}
	return v.createDataVolumeFromSource(namespace, cloneName, size, pvcSource)
}

// Create a DataVolume and wait for it to be ready.
func (v *VirtOperator) EnsureDataVolumeFromUrl(namespace, name, url, size string, timeout time.Duration) error {
	if !v.checkDataVolumeExists(namespace, name) {
		if err := v.createDataVolumeFromUrl(namespace, name, url, size); err != nil {
			return err
		}
		log.Printf("Created DataVolume %s/%s from %s", namespace, name, url)
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
func (v *VirtOperator) RemoveDataVolume(namespace, name string, timeout time.Duration) error {
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

// Clone a DataVolume and wait for the copy to be ready.
func (v *VirtOperator) CloneDisk(namespace, sourceName, cloneName string, timeout time.Duration) error {
	log.Printf("Cloning %s/%s to %s/%s...", namespace, sourceName, namespace, cloneName)
	if !v.checkDataVolumeExists(namespace, sourceName) {
		return fmt.Errorf("source disk does not exist")
	}

	size, err := v.getDataVolumeSize(namespace, sourceName)
	if err != nil {
		return fmt.Errorf("failed to get disk size for clone: %w", err)
	}

	if err := v.createDataVolumeFromPvc(namespace, sourceName, cloneName, size); err != nil {
		return fmt.Errorf("failed to clone disk: %w", err)
	}

	err = wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		return v.checkDataVolumeReady(namespace, cloneName), nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting to clone DataVolume %s/%s to %s/%s: %w", namespace, sourceName, namespace, cloneName, err)
	}

	return nil
}

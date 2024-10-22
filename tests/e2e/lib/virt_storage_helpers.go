package lib

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
)

var dataVolumeGVR = schema.GroupVersionResource{
	Group:    "cdi.kubevirt.io",
	Resource: "datavolumes",
	Version:  "v1beta1",
}

var dataSourceGVR = schema.GroupVersionResource{
	Group:    "cdi.kubevirt.io",
	Resource: "datasources",
	Version:  "v1beta1",
}

func (v *VirtOperator) getDataVolume(namespace, name string) (*unstructured.Unstructured, error) {
	unstructuredDataVolume, err := v.Dynamic.Resource(dataVolumeGVR).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	return unstructuredDataVolume, err
}

func (v *VirtOperator) deleteDataVolume(namespace, name string) error {
	return v.Dynamic.Resource(dataVolumeGVR).Namespace(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
}

func (v *VirtOperator) CheckDataVolumeExists(namespace, name string) bool {
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
	log.Printf("Phase of DataVolume %s/%s: %s", namespace, name, phase)
	return phase == "Succeeded"
}

// Create a DataVolume and ask it to fill itself with the contents of the given URL.
// Also add annotations to immediately create and bind to a PersistentVolume,
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

	_, err := v.Dynamic.Resource(dataVolumeGVR).Namespace(namespace).Create(context.Background(), &unstructuredDataVolume, metav1.CreateOptions{})
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
func (v *VirtOperator) EnsureDataVolumeFromUrl(namespace, name, url, size string, timeout time.Duration) error {
	if !v.CheckDataVolumeExists(namespace, name) {
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
		return !v.CheckDataVolumeExists(namespace, name), nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for DataVolume %s/%s to be deleted: %w", namespace, name, err)
	}

	log.Printf("DataVolume %s/%s cleaned up", namespace, name)

	return nil
}

func (v *VirtOperator) RemoveDataSource(namespace, name string) error {
	return v.Dynamic.Resource(dataSourceGVR).Namespace(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
}

// Create a DataSource from an existing PVC, with the same name and namespace.
// This way, the PVC can be specified as a sourceRef in the VM spec.
func (v *VirtOperator) CreateDataSourceFromPvc(namespace, name string) error {
	return v.CreateTargetDataSourceFromPvc(namespace, namespace, name, name)
}

func (v *VirtOperator) CreateTargetDataSourceFromPvc(sourceNamespace, destinationNamespace, sourcePvcName, destinationDataSourceName string) error {
	unstructuredDataSource := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cdi.kubevirt.io/v1beta1",
			"kind":       "DataSource",
			"metadata": map[string]interface{}{
				"name":      destinationDataSourceName,
				"namespace": destinationNamespace,
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"pvc": map[string]interface{}{
						"name":      sourcePvcName,
						"namespace": sourceNamespace,
					},
				},
			},
		},
	}

	_, err := v.Dynamic.Resource(dataSourceGVR).Namespace(destinationNamespace).Create(context.Background(), &unstructuredDataSource, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		if strings.Contains(err.Error(), "already exists") {
			return nil
		}
		log.Printf("Error creating DataSource: %v", err)
		return err
	}

	return nil
}

// Find the given DataSource, and return the PVC it points to
func (v *VirtOperator) GetDataSourcePvc(ns, name string) (string, string, error) {
	unstructuredDataSource, err := v.Dynamic.Resource(dataSourceGVR).Namespace(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error getting DataSource %s: %v", name, err)
		return "", "", err
	}

	pvcName, ok, err := unstructured.NestedString(unstructuredDataSource.UnstructuredContent(), "status", "source", "pvc", "name")
	if err != nil {
		log.Printf("Error getting PVC from DataSource: %v", err)
		return "", "", err
	}
	if !ok {
		return "", "", errors.New("failed to get PVC from " + name + " DataSource")
	}

	pvcNamespace, ok, err := unstructured.NestedString(unstructuredDataSource.UnstructuredContent(), "status", "source", "pvc", "namespace")
	if err != nil {
		log.Printf("Error getting PVC namespace from DataSource: %v", err)
		return "", "", err
	}
	if !ok {
		return "", "", errors.New("failed to get PVC namespace from " + name + " DataSource")
	}

	return pvcNamespace, pvcName, nil

}

// Find the default storage class
func (v *VirtOperator) GetDefaultStorageClass() (*storagev1.StorageClass, error) {
	storageClasses, err := v.Clientset.StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var defaultStorageClass *storagev1.StorageClass
	for _, storageClass := range storageClasses.Items {
		if storageClass.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			log.Printf("Found default storage class: %s", storageClass.Name)
			defaultStorageClass = storageClass.DeepCopy()
			return defaultStorageClass, nil
		}
	}

	return nil, errors.New("no default storage class found")
}

// Check the VolumeBindingMode of the default storage class, and make an
// Immediate-mode copy if it is set to WaitForFirstConsumer.
func (v *VirtOperator) CreateImmediateModeStorageClass(name string) error {
	defaultStorageClass, err := v.GetDefaultStorageClass()
	if err != nil {
		return err
	}

	immediateStorageClass := defaultStorageClass
	immediateStorageClass.VolumeBindingMode = ptr.To[storagev1.VolumeBindingMode](storagev1.VolumeBindingImmediate)
	immediateStorageClass.Name = name
	immediateStorageClass.ResourceVersion = ""
	immediateStorageClass.Annotations["storageclass.kubernetes.io/is-default-class"] = "false"

	_, err = v.Clientset.StorageV1().StorageClasses().Create(context.Background(), immediateStorageClass, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// Check the VolumeBindingMode of the default storage class, and make a
// WaitForFirstConsumer-mode copy if it is set to Immediate.
func (v *VirtOperator) CreateWaitForFirstConsumerStorageClass(name string) error {
	defaultStorageClass, err := v.GetDefaultStorageClass()
	if err != nil {
		return err
	}

	wffcStorageClass := defaultStorageClass
	wffcStorageClass.VolumeBindingMode = ptr.To[storagev1.VolumeBindingMode](storagev1.VolumeBindingWaitForFirstConsumer)
	wffcStorageClass.Name = name
	wffcStorageClass.ResourceVersion = ""
	wffcStorageClass.Annotations["storageclass.kubernetes.io/is-default-class"] = "false"

	_, err = v.Clientset.StorageV1().StorageClasses().Create(context.Background(), wffcStorageClass, metav1.CreateOptions{})
	return err
}

func (v *VirtOperator) RemoveStorageClass(name string) error {
	err := v.Clientset.StorageV1().StorageClasses().Delete(context.Background(), name, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

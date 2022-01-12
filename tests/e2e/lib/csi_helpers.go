package lib

import (
	"context"
	"log"
	"regexp"

	v1vsc "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned/typed/volumesnapshot/v1"
	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var DEFAULT_CSI_PLGUIN = map[string]string{
	"aws":       "ebs.csi.aws.com",
	"gcp":       "pd.csi.storage.gke.io",
	"openstack": "cinder.csi.openstack.org",
}

func GetVolumesnapshotListByLabel(namespace string, labelselector string) (*v1vsc.VolumeSnapshotList, error) {
	clientset, err := SetUpSnapshotClient()
	if err != nil {
		return nil, err
	}

	volumeSnapshotList, err := clientset.VolumeSnapshots(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labelselector,
	})
	if err != nil {
		return nil, err
	}

	return volumeSnapshotList, nil
}

func IsVolumeSnapshotReadyToUse(vs *v1vsc.VolumeSnapshot) (bool, error) {
	log.Printf("Checking if volumesnapshot is ready to use...")

	clientset, err := SetUpSnapshotClient()
	if err != nil {
		return false, err
	}

	volumeSnapshot, err := clientset.VolumeSnapshots(vs.Namespace).Get(context.Background(), vs.Name, metav1.GetOptions{})
	return *volumeSnapshot.Status.ReadyToUse, err

}

func GetCsiDriversList() (*v1.CSIDriverList, error) {
	clientset, err := setUpClient()
	if err != nil {
		return nil, err
	}

	clientcsi, err := clientset.StorageV1().CSIDrivers().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return clientcsi, nil
}

func SetUpSnapshotClient() (*snapshotv1.SnapshotV1Client, error) {
	kubeConf := getKubeConfig()

	client, err := snapshotv1.NewForConfig(kubeConf)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func GetVolumesnapshotclassByDriver(provisioner string) (*v1vsc.VolumeSnapshotClass, error) {
	clientset, err := SetUpSnapshotClient()
	if err != nil {
		return nil, err
	}

	volumeSnapshotClassList, err := clientset.VolumeSnapshotClasses().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, vsc := range volumeSnapshotClassList.Items {
		match, err := regexp.MatchString(provisioner, vsc.Driver)
		if err != nil {
			return nil, err
		}
		if match {
			return &vsc, nil
		}
	}
	vscgroupresource := schema.GroupResource{
		Group:    "snapshot.storage.k8s.io",
		Resource: "volumesnapshotclasses",
	}
	return nil, errors.NewNotFound(vscgroupresource, provisioner)
}

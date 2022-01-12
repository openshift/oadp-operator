package lib

import (
	"context"
	"fmt"
	"log"
	"time"

	v1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openshift/oadp-operator/tests/e2e/utils"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	v1storage "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BackupInterface interface {
	NewBackup(client.Client, string, *velero.BackupSpec)
	SetBackupStorageClass(storageClassName string) (err error)
	PrepareBackup() error
	CreateBackup() error
	CleanBackup() error
	GetType() BackupRestoreType
	GetBackupSpec() *velero.Backup
	IsBackupCompletedSuccessfully() (bool, error)
	IsBackupDone() wait.ConditionFunc
}

type backup struct {
	BackupInterface
	*velero.Backup
	client.Client
	dsc *v1storage.StorageClass
}

// empty implementation
func (b *backup) CleanBackup() error {
	return nil
}

func (b *backup) GetBackupSpec() *velero.Backup {
	return b.Backup
}

func (b *backup) NewBackup(ocClient client.Client, backupName string, backupSpec *velero.BackupSpec) {
	b.Client = ocClient
	b.Backup = &velero.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenNameUuid(backupName),
			Namespace: Dpa.Namespace,
		},
		Spec: *backupSpec,
	}

}

func (b *backup) SetBackupStorageClass(storageClassName string) (err error) {
	if storageClassName != "" {
		b.dsc, err = GetDefaultStorageClass()
		if err != nil {
			return
		}
		return SetNewDefaultStorageClass(string(storageClassName))
	}
	return
}

func (b *backup) CreateBackup() error {
	err := b.Client.Create(context.Background(), b.Backup)
	return err

}

func (b *backup) IsBackupDone() wait.ConditionFunc {
	return func() (bool, error) {
		backupvar := velero.Backup{}
		err := b.Client.Get(context.Background(), client.ObjectKey{
			Namespace: Dpa.Namespace,
			Name:      b.Backup.Name,
		}, &backupvar)
		if err != nil {
			return false, err
		}
		if len(backupvar.Status.Phase) > 0 {
			ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("backup phase: %s\n", backupvar.Status.Phase)))
		}
		if backupvar.Status.Phase != "" && backupvar.Status.Phase != velero.BackupPhaseNew && backupvar.Status.Phase != velero.BackupPhaseInProgress {
			return true, nil
		}
		return false, nil
	}
}

func (b *backup) IsBackupCompletedSuccessfully() (bool, error) {
	backupvar := velero.Backup{}
	err := b.Client.Get(context.Background(), client.ObjectKey{
		Namespace: Dpa.Namespace,
		Name:      b.Backup.Name,
	}, &backupvar)
	if err != nil {
		return false, err
	}
	if err != nil {
		return false, err
	}
	if backupvar.Status.Phase == velero.BackupPhaseCompleted {
		return true, nil
	}
	return false, fmt.Errorf("backup phase is: %s; expected: %s\nvalidation errors: %v\nvelero failure logs: %v", backupvar.Status.Phase, velero.BackupPhaseCompleted, backupvar.Status.ValidationErrors, GetVeleroContainerFailureLogs(Dpa.Namespace))
}

type BackupCsi struct {
	backup
	vsc        *v1.VolumeSnapshotClass
	DriverName string
}

func (b *BackupCsi) PrepareBackup() error {

	snapshotClient, _ := SetUpSnapshotClient()
	var vsc *v1.VolumeSnapshotClass
	var err error
	if b.DriverName == "" {
		infra, err := GetInfrastructure(b.Client)
		if err != nil {
			return err
		}

		if _, ok := DEFAULT_CSI_PLGUIN[infra]; ok {
			b.DriverName = DEFAULT_CSI_PLGUIN[infra]
		}
	}

	// Get existing VolumeSnapshotClass by the driver
	vsc, err = GetVolumesnapshotclassByDriver(b.DriverName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			csiClient, _ := GetCsiDriversList()
			vsc = &v1.VolumeSnapshotClass{
				Driver:     csiClient.Items[0].ObjectMeta.Name,
				Parameters: map[string]string{},
			}
		} else {
			return err
		}
	}

	vs := v1.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "example-snapclass",
			Annotations: map[string]string{
				"snapshot.storage.kubernetes.io/is-default-class": "true",
			},
			Labels: map[string]string{
				"velero.io/csi-volumesnapshot-class": "true",
			},
		},
		Driver:         vsc.Driver,
		DeletionPolicy: v1.VolumeSnapshotContentRetain,
		Parameters:     vsc.Parameters,
	}

	_, err = snapshotClient.VolumeSnapshotClasses().Create(context.TODO(), &vs, metav1.CreateOptions{})
	if err == nil {
		b.vsc = &vs
	} else {
		return err
	}

	csiStorageClass, err := GetStorageClassByProvisioner(vsc.Driver)
	if err != nil {
		return err
	}

	return b.SetBackupStorageClass(csiStorageClass.Name)
}

func (b *BackupCsi) IsBackupDone() wait.ConditionFunc {
	Eventually(b.backup.IsBackupDone(), 1*time.Minute*4, time.Second*10).Should(BeTrue())
	return func() (bool, error) {
		var err error
		for _, namespace := range b.backup.Backup.Spec.IncludedNamespaces {
			vsList, err := GetVolumesnapshotListByLabel(namespace,
				"velero.io/backup-name="+b.backup.Backup.GetObjectMeta().GetName())
			if err != nil {
				ginkgo.GinkgoWriter.Write([]byte(err.Error()))
			}
			if err != nil {
				return false, err
			}

			result, err := IsVolumeSnapshotReadyToUse(&vsList.Items[0])
			if !result {
				ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("VolumeSnapshot %s readyToUse: %v\n", vsList.Items[0].Name, result)))
			}
			if err != nil || !result {
				return result, err
			}

		}
		return true, err
	}
}

func (b *BackupCsi) CleanBackup() error {
	log.Printf("Deleting VolumeSnapshot for CSI backuprestore of %s", b.Backup.Name)
	SetNewDefaultStorageClass(b.dsc.Name)
	snapshotClient, _ := SetUpSnapshotClient()
	return snapshotClient.VolumeSnapshotClasses().Delete(context.TODO(), b.vsc.Name, metav1.DeleteOptions{})
}

func (b *BackupCsi) GetType() BackupRestoreType {
	return CSI
}

type BackupVsl struct {
	backup
	vsl []*velero.VolumeSnapshotLocation
	*DpaCustomResource
	CreateFromDpa bool
}

func (b *BackupVsl) PrepareBackup() error {
	if !b.CreateFromDpa {
		for _, item := range Dpa.Spec.SnapshotLocations {
			vsl := velero.VolumeSnapshotLocation{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "snapshot-location-",
					Namespace:    Dpa.Namespace,
				},
				Spec: velero.VolumeSnapshotLocationSpec{
					Provider: item.Velero.Provider,
					Config:   item.Velero.Config,
				},
			}
			err := b.backup.Client.Create(context.Background(), &vsl)
			if err != nil {
				return err
			}
			b.vsl = append(b.vsl, &vsl)
			b.Backup.Spec.VolumeSnapshotLocations = append(b.Backup.Spec.VolumeSnapshotLocations, vsl.Name)
		}
	}
	return nil
}

func (b *BackupVsl) CleanBackup() error {
	if !b.CreateFromDpa {
		for _, item := range b.vsl {

			err := b.backup.Client.Delete(context.Background(), item)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *BackupVsl) GetType() BackupRestoreType {
	return VSL
}

type BackupRestic struct {
	backup
}

func (b *BackupRestic) PrepareBackup() error {
	Eventually(AreResticPodsRunning(b.Backup.Namespace), 1*time.Minute*3, time.Second*5).Should(BeTrue())
	// TODO: Uncomment once the permission issues on PR job are figured out
	// if b.Backup != nil {
	// 	b.Backup.Spec.DefaultVolumesToRestic = pointer.Bool(true)
	// }
	return nil
}

func (b *BackupRestic) GetType() BackupRestoreType {
	return RESTIC
}

// TODO: Remove
func CreateBackupForNamespaces(ocClient client.Client, veleroNamespace, backupName string, namespaces []string) error {

	backup := velero.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: veleroNamespace,
		},
		Spec: velero.BackupSpec{
			IncludedNamespaces: namespaces,
		},
	}
	err := ocClient.Create(context.Background(), &backup)
	return err
}

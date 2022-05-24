package lib

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BackupOpts func(*velero.Backup) error

func WithBackupStorageLocation(name string) BackupOpts {
	return func(backup *velero.Backup) error {
		backup.Spec.StorageLocation = name
		return nil
	}
}

func WithDefaultVolumesToRestic(val bool) BackupOpts {
	return func(backup *velero.Backup) error {
		backup.Spec.DefaultVolumesToRestic = pointer.Bool(val)
		return nil
	}
}

func CreateBackupForNamespaces(ocClient client.Client, veleroNamespace, backupName string, namespaces []string, backupOpts ...BackupOpts) (velero.Backup, error) {

	backup := velero.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: veleroNamespace,
		},
		Spec: velero.BackupSpec{
			IncludedNamespaces: namespaces,
		},
	}
	for _, opt := range backupOpts {
		err := opt(&backup)
		if err != nil {
			return velero.Backup{}, err
		}
	}
	err := ocClient.Create(context.Background(), &backup)
	return backup, err
}

func IsBackupDone(ocClient client.Client, veleroNamespace, name string) wait.ConditionFunc {
	return func() (bool, error) {
		backup := velero.Backup{}
		err := ocClient.Get(context.Background(), client.ObjectKey{
			Namespace: veleroNamespace,
			Name:      name,
		}, &backup)
		if err != nil {
			return false, err
		}
		if len(backup.Status.Phase) > 0 {
			ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("backup phase: %s\n", backup.Status.Phase)))
		}
		if backup.Status.Phase != "" && backup.Status.Phase != velero.BackupPhaseNew && backup.Status.Phase != velero.BackupPhaseInProgress {
			return true, nil
		}
		return false, nil
	}
}

func IsBackupCompletedSuccessfully(ocClient client.Client, backup velero.Backup) (bool, error) {
	err := ocClient.Get(context.Background(), client.ObjectKey{
		Namespace: backup.Namespace,
		Name:      backup.Name,
	}, &backup)
	if err != nil {
		return false, err
	}

	if backup.Status.Phase == velero.BackupPhaseCompleted {
		return true, nil
	}
	return false, fmt.Errorf("backup phase is: %s; expected: %s\nvalidation errors: %v\nvelero failure logs: %v", backup.Status.Phase, velero.BackupPhaseCompleted, backup.Status.ValidationErrors, GetVeleroContainerFailureLogs(backup.Namespace))
}

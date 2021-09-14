package e2e

import (
	"context"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func createBackupForNamespaces(ocClient client.Client, veleroNamespace, backupName string, namespaces []string) error {

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

func isBackupDone(ocClient client.Client, veleroNamespace, name string) wait.ConditionFunc {
	return func() (bool, error) {
		backup := velero.Backup{}
		err := ocClient.Get(context.Background(), client.ObjectKey{
			Namespace: veleroNamespace,
			Name:      name,
		}, &backup)
		if err != nil {
			return false, err
		}
		if backup.Status.Phase != "" && backup.Status.Phase != velero.BackupPhaseNew && backup.Status.Phase != velero.BackupPhaseInProgress {
			return true, nil
		}
		return false, nil
	}
}

func isBackupCompletedSuccessfully(ocClient client.Client, veleroNamespace, name string) (bool, error) {
	backup := velero.Backup{}
	err := ocClient.Get(context.Background(), client.ObjectKey{
		Namespace: veleroNamespace,
		Name:      name,
	}, &backup)
	if err != nil {
		return false, err
	}
	if backup.Status.Phase == velero.BackupPhaseCompleted {
		return true, nil
	}
	return false, nil
}

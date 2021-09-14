package e2e

import (
	"context"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func createRestoreFromBackup(ocClient client.Client, veleroNamespace, backupName, restoreName string) error {
	restore := velero.Restore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restoreName,
			Namespace: veleroNamespace,
		},
		Spec: velero.RestoreSpec{
			BackupName: backupName,
		},
	}
	err := ocClient.Create(context.Background(), &restore)
	return err
}

func isRestoreDone(ocClient client.Client, veleroNamespace, name string) wait.ConditionFunc {
	return func() (bool, error) {
		restore := velero.Restore{}
		err := ocClient.Get(context.Background(), client.ObjectKey{
			Namespace: veleroNamespace,
			Name:      name,
		}, &restore)
		if err != nil {
			return false, err
		}
		if restore.Status.Phase != "" && restore.Status.Phase != velero.RestorePhaseNew && restore.Status.Phase != velero.RestorePhaseInProgress {
			return true, nil
		}
		return false, nil
	}
}

func isRestoreCompletedSuccessfully(ocClient client.Client, veleroNamespace, name string) (bool, error) {
	restore := velero.Restore{}
	err := ocClient.Get(context.Background(), client.ObjectKey{
		Namespace: veleroNamespace,
		Name:      name,
	}, &restore)
	if err != nil {
		return false, err
	}
	if restore.Status.Phase == velero.RestorePhaseCompleted {
		return true, nil
	}
	return false, nil
}

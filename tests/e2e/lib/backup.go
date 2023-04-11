package lib

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateBackupForNamespaces(ocClient client.Client, veleroNamespace, backupName string, namespaces []string, defaultVolumesToRestic bool) (velero.Backup, error) {

	backup := velero.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: veleroNamespace,
		},
		Spec: velero.BackupSpec{
			IncludedNamespaces:     namespaces,
			DefaultVolumesToRestic: &defaultVolumesToRestic,
		},
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
		var phasesNotDone = []velero.BackupPhase{
			velero.BackupPhaseNew,
			velero.BackupPhaseInProgress,
			velero.BackupPhaseWaitingForPluginOperations,
			velero.BackupPhaseWaitingForPluginOperationsPartiallyFailed,
			velero.BackupPhaseFinalizing,
			velero.BackupPhaseFinalizingPartiallyFailed,
			"",
		}
		for _, notDonePhase := range phasesNotDone {
			if backup.Status.Phase == notDonePhase {
				return false, nil
			}
		}
		return true, nil
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

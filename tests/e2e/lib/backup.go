package lib

import (
	"context"
	"fmt"
	"log"

	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateBackupForNamespaces(ocClient client.Client, veleroNamespace, backupName string, namespaces []string, defaultVolumesToFsBackup bool, snapshotMoveData bool) (velero.Backup, error) {
	backup := velero.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: veleroNamespace,
		},
		Spec: velero.BackupSpec{
			IncludedNamespaces:       namespaces,
			DefaultVolumesToFsBackup: &defaultVolumesToFsBackup,
			SnapshotMoveData:         &snapshotMoveData,
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
			log.Printf("backup phase: %s", backup.Status.Phase)
		}
		var phasesNotDone = []velero.BackupPhase{
			velero.BackupPhaseNew,
			velero.BackupPhaseInProgress,
			velero.BackupPhaseWaitingForPluginOperations,
			velero.BackupPhaseWaitingForPluginOperationsPartiallyFailed,
			"Finalizing",
			"FinalizingPartiallyFailed",
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

func IsBackupCompletedSuccessfully(c *kubernetes.Clientset, ocClient client.Client, backup velero.Backup) (bool, error) {
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
	return false, fmt.Errorf(
		"backup phase is: %s; expected: %s\nfailure reason: %s\nvalidation errors: %v\nvelero failure logs: %v",
		backup.Status.Phase, velero.BackupPhaseCompleted, backup.Status.FailureReason, backup.Status.ValidationErrors,
		GetVeleroContainerFailureLogs(c, backup.Namespace),
	)
}

// backupPhasesNotDone is the shared list of backup phases that indicate a backup is still in progress.
var backupPhasesNotDone = []velero.BackupPhase{
	velero.BackupPhaseNew,
	"Queued",
	"ReadyToStart",
	velero.BackupPhaseInProgress,
	velero.BackupPhaseWaitingForPluginOperations,
	velero.BackupPhaseWaitingForPluginOperationsPartiallyFailed,
	"Finalizing",
	"FinalizingPartiallyFailed",
	"",
}

// IsBackupPhaseNotDone returns true if the given phase string represents a backup still in progress.
func IsBackupPhaseNotDone(phase string) bool {
	for _, notDonePhase := range backupPhasesNotDone {
		if phase == string(notDonePhase) {
			return true
		}
	}
	return false
}

package lib

import (
	"context"
	"fmt"
	"log"

	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateRestoreFromBackup(ocClient client.Client, veleroNamespace, backupName, restoreName string, restorePVs bool) (velero.Restore, error) {
	restore := velero.Restore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restoreName,
			Namespace: veleroNamespace,
		},
		Spec: velero.RestoreSpec{
			BackupName: backupName,
		},
	}
	if restorePVs {
		restore.Spec.RestorePVs = pointer.Bool(true)
	}
	err := ocClient.Create(context.Background(), &restore)
	return restore, err
}

func IsRestoreDone(ocClient client.Client, veleroNamespace, name string) wait.ConditionFunc {
	return func() (bool, error) {
		restore := velero.Restore{}
		err := ocClient.Get(context.Background(), client.ObjectKey{
			Namespace: veleroNamespace,
			Name:      name,
		}, &restore)
		if err != nil {
			return false, err
		}
		if len(restore.Status.Phase) > 0 {
			log.Printf("restore phase: %s", restore.Status.Phase)
		}
		var phasesNotDone = []velero.RestorePhase{
			velero.RestorePhaseNew,
			velero.RestorePhaseInProgress,
			velero.RestorePhaseWaitingForPluginOperations,
			velero.RestorePhaseWaitingForPluginOperationsPartiallyFailed,
			"",
		}
		for _, notDonePhase := range phasesNotDone {
			if restore.Status.Phase == notDonePhase {
				return false, nil
			}
		}
		return true, nil

	}
}

func IsRestoreCompletedSuccessfully(c *kubernetes.Clientset, ocClient client.Client, veleroNamespace, name string) (bool, error) {
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
	return false, fmt.Errorf(
		"restore phase is: %s; expected: %s\nfailure reason: %s\nvalidation errors: %v\nvelero failure logs: %v",
		restore.Status.Phase, velero.RestorePhaseCompleted, restore.Status.FailureReason, restore.Status.ValidationErrors,
		GetVeleroContainerFailureLogs(c, veleroNamespace),
	)
}

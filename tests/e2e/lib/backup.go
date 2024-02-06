package lib

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ParseDuration parses a string representation of a duration with suffixes (s, m, h)
// and converts it to the k8s.io/apimachinery/pkg/apis/meta/v1.Duration type.
//
// The input string should have the format "<value><suffix>", where:
// - <value> is a positive integer representing the duration value.
// - <suffix> is a character representing the duration unit:
//   - "s" for seconds
//   - "m" for minutes
//   - "h" for hours
//
// Example valid inputs: "10s", "5m", "1h"
//
// Returns the parsed duration as a v1.Duration object, or an error if parsing fails.
func parseDuration(durationStr string) (metav1.Duration, error) {

	value, err := time.ParseDuration(durationStr)
	if err != nil {
		return metav1.Duration{}, errors.New("Invalid duration value: " + durationStr)
	}

	duration := metav1.Duration{Duration: value}
	return duration, nil
}

func CreateBackupForNamespaces(ocClient client.Client, veleroNamespace, backupName string, namespaces []string, defaultVolumesToFsBackup bool, snapshotMoveData bool, csiSnapshotTimeout string) (velero.Backup, error) {

	csiSnapDuration, err := parseDuration(csiSnapshotTimeout)

	if err != nil {
		log.Printf("Error parsing duration: %v", err)
		return velero.Backup{}, err
	}

	log.Printf("csiSnapshotTimeout Parsed duration: %s", csiSnapDuration)

	backup := velero.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: veleroNamespace,
		},
		Spec: velero.BackupSpec{
			IncludedNamespaces:       namespaces,
			DefaultVolumesToFsBackup: &defaultVolumesToFsBackup,
			SnapshotMoveData:         &snapshotMoveData,
			CSISnapshotTimeout:       csiSnapDuration,
		},
	}
	err = ocClient.Create(context.Background(), &backup)
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

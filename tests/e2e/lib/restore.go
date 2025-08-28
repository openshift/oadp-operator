package lib

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/downloadrequest"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/output"
	"github.com/vmware-tanzu/velero/pkg/label"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateRestoreFromBackup(ocClient client.Client, veleroNamespace, backupName, restoreName string) error {
	restore := velero.Restore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restoreName,
			Namespace: veleroNamespace,
		},
		Spec: velero.RestoreSpec{
			BackupName: backupName,
		},
	}
	return ocClient.Create(context.Background(), &restore)
}

func GetRestore(c client.Client, namespace string, name string) (*velero.Restore, error) {
	restore := velero.Restore{}
	err := c.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, &restore)
	if err != nil {
		return nil, err
	}
	return &restore, nil
}

func IsRestoreDone(ocClient client.Client, veleroNamespace, name string) wait.ConditionFunc {
	return func() (bool, error) {
		restore, err := GetRestore(ocClient, veleroNamespace, name)
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
			velero.RestorePhaseFinalizing,
			velero.RestorePhaseFinalizingPartiallyFailed,
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
	restore, err := GetRestore(ocClient, veleroNamespace, name)
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

// https://github.com/vmware-tanzu/velero/blob/11bfe82342c9f54c63f40d3e97313ce763b446f2/pkg/cmd/cli/restore/describe.go#L72-L78
func DescribeRestore(ocClient client.Client, namespace string, name string) string {
	restore, err := GetRestore(ocClient, namespace, name)
	if err != nil {
		return "could not get provided backup: " + err.Error()
	}
	details := true
	insecureSkipTLSVerify := true
	caCertFile := ""
	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", velero.RestoreNameLabel, label.GetValidName(restore.Name))}
	podvolumeRestoreList := &velero.PodVolumeRestoreList{}
	err = ocClient.List(context.Background(), podvolumeRestoreList, client.InNamespace(restore.Namespace), &client.ListOptions{Raw: &opts})
	if err != nil {
		log.Printf("error getting PodVolumeRestores for restore %s: %v\n", restore.Name, err)
	}

	return output.DescribeRestore(context.Background(), ocClient, restore, podvolumeRestoreList.Items, details, insecureSkipTLSVerify, caCertFile)
}

func RestoreLogs(c *kubernetes.Clientset, ocClient client.Client, namespace string, name string) (restoreLogs string) {
	insecureSkipTLSVerify := true
	caCertFile := ""
	// new io.Writer that store the logs in a string
	logs := &bytes.Buffer{}
	// new io.Writer that store the logs in a string
	// if a backup failed, this function may panic. Recover from the panic and return container logs
	defer func() {
		if r := recover(); r != nil {
			restoreLogs = recoverFromPanicLogs(c, namespace, r, "RestoreLogs")
		}
	}()
	downloadrequest.Stream(context.Background(), ocClient, namespace, name, velero.DownloadTargetKindRestoreLog, logs, time.Minute, insecureSkipTLSVerify, caCertFile)

	return logs.String()
}

func RestoreErrorLogs(c *kubernetes.Clientset, ocClient client.Client, namespace string, name string) []string {
	rl := RestoreLogs(c, ocClient, namespace, name)
	return errorLogsExcludingIgnored(rl)
}

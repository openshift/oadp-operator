package lib

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	pkgbackup "github.com/vmware-tanzu/velero/pkg/backup"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/downloadrequest"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/output"
	"github.com/vmware-tanzu/velero/pkg/label"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateBackupForNamespaces(ocClient client.Client, veleroNamespace, backupName string, namespaces []string, defaultVolumesToFsBackup bool, snapshotMoveData bool) error {
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
	return ocClient.Create(context.Background(), &backup)
}

func CreateCustomBackupForNamespaces(ocClient client.Client, veleroNamespace, backupName string, namespaces []string, includedResources, excludedResources []string, defaultVolumesToFsBackup bool, snapshotMoveData bool) error {
	backup := velero.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: veleroNamespace,
		},
		Spec: velero.BackupSpec{
			IncludedNamespaces:       namespaces,
			IncludedResources:        includedResources,
			ExcludedResources:        excludedResources,
			DefaultVolumesToFsBackup: &defaultVolumesToFsBackup,
			SnapshotMoveData:         &snapshotMoveData,
		},
	}
	return ocClient.Create(context.Background(), &backup)
}

const kubevirtVolumePolicyName = "kubevirt-volume-policy"

const kubevirtVolumePolicyData = `version: v1
volumePolicies:
  - conditions: {}
    action:
      type: skip
`

// EnsureKubevirtVolumePolicy creates (or updates) the volume policy ConfigMap
// that tells Velero to skip CSI snapshots for CBT-labeled PVCs, allowing the
// kubevirt-datamover-plugin BackupItemActionV2 to handle them instead.
func EnsureKubevirtVolumePolicy(ocClient client.Client, namespace string) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubevirtVolumePolicyName,
			Namespace: namespace,
		},
		Data: map[string]string{
			"policy.yaml": kubevirtVolumePolicyData,
		},
	}
	err := ocClient.Create(context.Background(), cm)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			log.Printf("Volume policy ConfigMap %s already exists, updating", kubevirtVolumePolicyName)
			existing := &corev1.ConfigMap{}
			if getErr := ocClient.Get(context.Background(), client.ObjectKeyFromObject(cm), existing); getErr != nil {
				return fmt.Errorf("failed to get existing volume policy ConfigMap: %w", getErr)
			}
			existing.Data = cm.Data
			return ocClient.Update(context.Background(), existing)
		}
		return fmt.Errorf("failed to create volume policy ConfigMap: %w", err)
	}
	log.Printf("Created kubevirt volume policy ConfigMap %s/%s", namespace, kubevirtVolumePolicyName)
	return nil
}

// DeleteKubevirtVolumePolicy removes the volume policy ConfigMap.
func DeleteKubevirtVolumePolicy(ocClient client.Client, namespace string) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubevirtVolumePolicyName,
			Namespace: namespace,
		},
	}
	err := ocClient.Delete(context.Background(), cm)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete volume policy ConfigMap: %w", err)
	}
	return nil
}

// CreateBackupWithVolumePolicy creates a backup that references the kubevirt
// volume policy ConfigMap via Spec.ResourcePolicy.
func CreateBackupWithVolumePolicy(ocClient client.Client, veleroNamespace, backupName string, namespaces []string, snapshotMoveData bool) error {
	backup := velero.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: veleroNamespace,
		},
		Spec: velero.BackupSpec{
			IncludedNamespaces:       namespaces,
			DefaultVolumesToFsBackup: boolPtr(false),
			SnapshotMoveData:         &snapshotMoveData,
			ResourcePolicy: &corev1.TypedLocalObjectReference{
				Kind: "ConfigMap",
				Name: kubevirtVolumePolicyName,
			},
		},
	}
	return ocClient.Create(context.Background(), &backup)
}

func boolPtr(b bool) *bool { return &b }

func GetBackup(c client.Client, namespace string, name string) (*velero.Backup, error) {
	backup := velero.Backup{}
	err := c.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, &backup)
	if err != nil {
		return nil, err
	}
	return &backup, nil
}

// backupPhasesNotDone is the shared list of backup phases that indicate a backup is still in progress.
var backupPhasesNotDone = []velero.BackupPhase{
	velero.BackupPhaseNew,
	velero.BackupPhaseQueued,
	velero.BackupPhaseReadyToStart,
	velero.BackupPhaseInProgress,
	velero.BackupPhaseWaitingForPluginOperations,
	velero.BackupPhaseWaitingForPluginOperationsPartiallyFailed,
	velero.BackupPhaseFinalizing,
	velero.BackupPhaseFinalizingPartiallyFailed,
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

func IsBackupDone(ocClient client.Client, veleroNamespace, name string) wait.ConditionFunc {
	return func() (bool, error) {
		backup, err := GetBackup(ocClient, veleroNamespace, name)
		if err != nil {
			return false, err
		}
		if len(backup.Status.Phase) > 0 {
			log.Printf("backup phase: %s", backup.Status.Phase)
		}
		return !IsBackupPhaseNotDone(string(backup.Status.Phase)), nil
	}
}

func IsBackupCompletedSuccessfully(c *kubernetes.Clientset, ocClient client.Client, namespace string, name string) (bool, error) {
	backup, err := GetBackup(ocClient, namespace, name)
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

// https://github.com/vmware-tanzu/velero/blob/11bfe82342c9f54c63f40d3e97313ce763b446f2/pkg/cmd/cli/backup/describe.go#L77-L111
func DescribeBackup(ocClient client.Client, namespace string, name string) (backupDescription string) {
	backup, err := GetBackup(ocClient, namespace, name)
	if err != nil {
		return "could not get provided backup: " + err.Error()
	}
	details := true
	insecureSkipTLSVerify := true
	caCertFile := ""

	deleteRequestListOptions := pkgbackup.NewDeleteBackupRequestListOptions(backup.Name, string(backup.UID))
	deleteRequestList := &velero.DeleteBackupRequestList{}
	err = ocClient.List(context.Background(), deleteRequestList, client.InNamespace(backup.Namespace), &client.ListOptions{Raw: &deleteRequestListOptions})
	if err != nil {
		log.Printf("error getting DeleteBackupRequests for backup %s: %v\n", backup.Name, err)
	}

	opts := label.NewListOptionsForBackup(backup.Name)
	podVolumeBackupList := &velero.PodVolumeBackupList{}
	err = ocClient.List(context.Background(), podVolumeBackupList, client.InNamespace(backup.Namespace), &client.ListOptions{Raw: &opts})
	if err != nil {
		log.Printf("error getting PodVolumeBackups for backup %s: %v\n", backup.Name, err)
	}

	// output.DescribeBackup is a helper function from velero CLI that attempts to download logs for a backup.
	// if a backup failed, this function may panic. Recover from the panic and return string of backup object
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in DescribeBackup: %v\n", r)
			log.Print("returning backup object instead")
			backupDescription = fmt.Sprint(backup)
		}
	}()
	return output.DescribeBackup(context.Background(), ocClient, backup, deleteRequestList.Items, podVolumeBackupList.Items, details, insecureSkipTLSVerify, caCertFile)
}

func BackupLogs(c *kubernetes.Clientset, ocClient client.Client, namespace string, name string) (backupLogs string) {
	insecureSkipTLSVerify := true
	caCertFile := ""
	// new io.Writer that store the logs in a string
	logs := &bytes.Buffer{}
	// new io.Writer that store the logs in a string
	// if a backup failed, this function may panic. Recover from the panic and return container logs
	defer func() {
		if r := recover(); r != nil {
			backupLogs = recoverFromPanicLogs(c, namespace, r, "BackupLogs")
		}
	}()
	downloadrequest.Stream(context.Background(), ocClient, namespace, name, velero.DownloadTargetKindBackupLog, logs, time.Minute, insecureSkipTLSVerify, caCertFile)

	return logs.String()
}

func BackupErrorLogs(c *kubernetes.Clientset, ocClient client.Client, namespace string, name string) []string {
	bl := BackupLogs(c, ocClient, namespace, name)
	return errorLogsExcludingIgnored(bl)
}

func GetBackupRepositoryList(c client.Client, namespace string) (*velero.BackupRepositoryList, error) {
	// initialize an empty list of BackupRepositories
	backupRepositoryList := &velero.BackupRepositoryList{
		Items: []velero.BackupRepository{},
	}
	// get the list of BackupRepositories in the given namespace
	err := c.List(context.Background(), backupRepositoryList, client.InNamespace(namespace))
	if err != nil {
		log.Printf("error getting BackupRepository list: %v", err)
		return nil, err
	}
	return backupRepositoryList, nil
}

func DeleteBackupRepository(c client.Client, namespace string, name string) error {
	backupRepository := &velero.BackupRepository{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
	err := c.Delete(context.Background(), backupRepository)
	if err != nil {
		return err
	}
	return nil
}

// DeleteBackupRepositories deletes all BackupRepositories in the given namespace.
func DeleteBackupRepositories(c client.Client, namespace string) error {
	log.Printf("Checking if backuprepository's exist in %s", namespace)

	backupRepos, err := GetBackupRepositoryList(c, namespace)
	if err != nil {
		return fmt.Errorf("failed to get BackupRepository list: %v", err)
	}
	if len(backupRepos.Items) == 0 {
		log.Printf("No BackupRepositories found in namespace %s", namespace)
		return nil
	}

	// Get a list of the BackupRepositories and delete all of them.
	for _, repo := range backupRepos.Items {
		log.Printf("backuprepository name is %s", repo.Name)
		err := DeleteBackupRepository(c, namespace, repo.Name)
		if err != nil {
			log.Printf("failed to delete BackupRepository %s: ", repo.Name)
			return err
		}
		log.Printf("Successfully deleted BackupRepository: %s", repo.Name)
	}

	return nil
}

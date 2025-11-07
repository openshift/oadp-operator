package lib

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	pkgbackup "github.com/vmware-tanzu/velero/pkg/backup"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/downloadrequest"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/output"
	"github.com/vmware-tanzu/velero/pkg/label"
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

func IsBackupDone(ocClient client.Client, veleroNamespace, name string) wait.ConditionFunc {
	return func() (bool, error) {
		backup, err := GetBackup(ocClient, veleroNamespace, name)
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

// PrintBackupRepositoryDiagnostics prints detailed information about BackupRepositories
// to help diagnose PrepareRepo bugs where local Kopia config files are missing
func PrintBackupRepositoryDiagnostics(c client.Client, namespace string) {
	log.Println("===== BackupRepository Diagnostics (PrepareRepo Bug Detection) =====")

	backupRepos, err := GetBackupRepositoryList(c, namespace)
	if err != nil {
		log.Printf("ERROR: Failed to get BackupRepository list: %v", err)
		return
	}

	if len(backupRepos.Items) == 0 {
		log.Println("INFO: No BackupRepositories found")
		return
	}

	for _, repo := range backupRepos.Items {
		log.Printf("\n--- BackupRepository: %s ---", repo.Name)
		log.Printf("  UID: %s", repo.UID)
		log.Printf("  Created: %s", repo.CreationTimestamp)
		log.Printf("  Repository Type: %s", repo.Spec.RepositoryType)
		log.Printf("  Volume Namespace: %s", repo.Spec.VolumeNamespace)
		log.Printf("  Backup Storage Location: %s", repo.Spec.BackupStorageLocation)
		log.Printf("  Phase: %s", repo.Status.Phase)
		log.Printf("  Message: %s", repo.Status.Message)

		if repo.Status.Phase == velero.BackupRepositoryPhaseReady {
			log.Printf("  ⚠️  EVIDENCE: Repository exists in backend storage")
			log.Printf("  ⚠️  If DataUpload fails with 'config file not found', this confirms PrepareRepo bug")
		}
	}
	log.Println("================================================================")
}

// PrintDataUploadDiagnostics prints detailed information about DataUpload resources
// to help diagnose PrepareRepo bugs by showing config file errors
func PrintDataUploadDiagnostics(c client.Client, namespace string, backupName string) {
	log.Println("===== DataUpload Diagnostics (PrepareRepo Bug Detection) =====")

	dataUploadList := &velerov2alpha1.DataUploadList{}
	err := c.List(context.Background(), dataUploadList, client.InNamespace(namespace))
	if err != nil {
		log.Printf("ERROR: Failed to get DataUpload list: %v", err)
		return
	}

	if len(dataUploadList.Items) == 0 {
		log.Println("INFO: No DataUpload resources found")
		log.Println("  ⚠️  This might indicate volume backup didn't run")
		log.Println("================================================================")
		return
	}

	// Filter for DataUploads related to this backup
	relevantUploads := []velerov2alpha1.DataUpload{}
	for _, du := range dataUploadList.Items {
		if du.Labels != nil && du.Labels[velero.BackupNameLabel] == backupName {
			relevantUploads = append(relevantUploads, du)
		}
	}

	if len(relevantUploads) == 0 {
		log.Printf("INFO: No DataUpload resources found for backup %s", backupName)
		log.Println("  ⚠️  This might indicate volume backup didn't run")
		log.Println("================================================================")
		return
	}

	for _, du := range relevantUploads {
		log.Printf("\n--- DataUpload: %s ---", du.Name)
		log.Printf("  Created: %s", du.CreationTimestamp)
		log.Printf("  Phase: %s", du.Status.Phase)
		log.Printf("  Message: %s", du.Status.Message)
		log.Printf("  Progress: %+v", du.Status.Progress)
		log.Printf("  Node: %s", du.Status.Node)

		// Check for PrepareRepo bug evidence in message
		msg := du.Status.Message
		if msg != "" {
			if (du.Status.Phase == velerov2alpha1.DataUploadPhaseFailed || du.Status.Phase == velerov2alpha1.DataUploadPhaseCanceled) &&
				(containsIgnoreCase(msg, "config") && containsIgnoreCase(msg, "no such file")) {
				log.Println("  🔴 CRITICAL BUG EVIDENCE: Config file not found error detected!")
				log.Println("  🔴 This confirms the PrepareRepo bug hypothesis")
			}
		}
	}
	log.Println("================================================================")
}

// containsIgnoreCase performs case-insensitive substring search
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			indexIgnoreCase(s, substr) >= 0))
}

// indexIgnoreCase finds the index of substr in s, case-insensitive
func indexIgnoreCase(s, substr string) int {
	s = toLower(s)
	substr = toLower(substr)
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// toLower converts string to lowercase
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

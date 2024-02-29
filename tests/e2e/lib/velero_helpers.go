package lib

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	snapshotv1client "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	pkgbackup "github.com/vmware-tanzu/velero/pkg/backup"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/downloadrequest"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/output"
	"github.com/vmware-tanzu/velero/pkg/features"
	veleroclientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	"github.com/vmware-tanzu/velero/pkg/label"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// https://github.com/vmware-tanzu/velero/blob/11bfe82342c9f54c63f40d3e97313ce763b446f2/pkg/cmd/cli/backup/describe.go#L77-L111
func DescribeBackup(veleroClient veleroclientset.Interface, csiClient *snapshotv1client.Clientset, ocClient client.Client, backup velero.Backup) (backupDescription string) {
	err := ocClient.Get(context.Background(), client.ObjectKey{
		Namespace: backup.Namespace,
		Name:      backup.Name,
	}, &backup)
	if err != nil {
		return "could not get provided backup: " + err.Error()
	}
	details := true
	insecureSkipTLSVerify := true
	caCertFile := ""

	deleteRequestListOptions := pkgbackup.NewDeleteBackupRequestListOptions(backup.Name, string(backup.UID))
	deleteRequestList, err := veleroClient.VeleroV1().DeleteBackupRequests(backup.Namespace).List(context.Background(), deleteRequestListOptions)
	if err != nil {
		log.Printf("error getting DeleteBackupRequests for backup %s: %v\n", backup.Name, err)
	}

	opts := label.NewListOptionsForBackup(backup.Name)
	podVolumeBackupList, err := veleroClient.VeleroV1().PodVolumeBackups(backup.Namespace).List(context.Background(), opts)
	if err != nil {
		log.Printf("error getting PodVolumeBackups for backup %s: %v\n", backup.Name, err)
	}

	// declare vscList up here since it may be empty and we'll pass the empty Items field into DescribeBackup
	vscList := new(snapshotv1api.VolumeSnapshotContentList)
	if features.IsEnabled(velero.CSIFeatureFlag) {
		vscList, err = csiClient.SnapshotV1().VolumeSnapshotContents().List(context.Background(), opts)
		if err != nil {
			log.Printf("error getting VolumeSnapshotContent objects for backup %s: %v\n", backup.Name, err)
		}
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
	return output.DescribeBackup(context.Background(), ocClient, &backup, deleteRequestList.Items, podVolumeBackupList.Items, vscList.Items, details, insecureSkipTLSVerify, caCertFile)
}

// https://github.com/vmware-tanzu/velero/blob/11bfe82342c9f54c63f40d3e97313ce763b446f2/pkg/cmd/cli/restore/describe.go#L72-L78
func DescribeRestore(veleroClient veleroclientset.Interface, ocClient client.Client, restore velero.Restore) string {
	err := ocClient.Get(context.Background(), client.ObjectKey{
		Namespace: restore.Namespace,
		Name:      restore.Name,
	}, &restore)
	if err != nil {
		return "could not get provided backup: " + err.Error()
	}
	details := true
	insecureSkipTLSVerify := true
	caCertFile := ""
	opts := newPodVolumeRestoreListOptions(restore.Name)
	podvolumeRestoreList, err := veleroClient.VeleroV1().PodVolumeRestores(restore.Namespace).List(context.Background(), opts)
	if err != nil {
		log.Printf("error getting PodVolumeRestores for restore %s: %v\n", restore.Name, err)
	}

	return output.DescribeRestore(context.Background(), ocClient, &restore, podvolumeRestoreList.Items, details, insecureSkipTLSVerify, caCertFile)
}

// newPodVolumeRestoreListOptions creates a ListOptions with a label selector configured to
// find PodVolumeRestores for the restore identified by name.
func newPodVolumeRestoreListOptions(name string) metav1.ListOptions {
	return metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", velero.RestoreNameLabel, label.GetValidName(name)),
	}
}

func BackupLogs(c *kubernetes.Clientset, ocClient client.Client, backup velero.Backup) (backupLogs string) {
	insecureSkipTLSVerify := true
	caCertFile := ""
	// new io.Writer that store the logs in a string
	logs := &bytes.Buffer{}
	// new io.Writer that store the logs in a string
	// if a backup failed, this function may panic. Recover from the panic and return container logs
	defer func() {
		if r := recover(); r != nil {
			backupLogs = recoverFromPanicLogs(c, backup.Namespace, r, "BackupLogs")
		}
	}()
	downloadrequest.Stream(context.Background(), ocClient, backup.Namespace, backup.Name, velero.DownloadTargetKindBackupLog, logs, time.Minute, insecureSkipTLSVerify, caCertFile)

	return logs.String()
}

func RestoreLogs(c *kubernetes.Clientset, ocClient client.Client, restore velero.Restore) (restoreLogs string) {
	insecureSkipTLSVerify := true
	caCertFile := ""
	// new io.Writer that store the logs in a string
	logs := &bytes.Buffer{}
	// new io.Writer that store the logs in a string
	// if a backup failed, this function may panic. Recover from the panic and return container logs
	defer func() {
		if r := recover(); r != nil {
			restoreLogs = recoverFromPanicLogs(c, restore.Namespace, r, "RestoreLogs")
		}
	}()
	downloadrequest.Stream(context.Background(), ocClient, restore.Namespace, restore.Name, velero.DownloadTargetKindRestoreLog, logs, time.Minute, insecureSkipTLSVerify, caCertFile)

	return logs.String()
}

func recoverFromPanicLogs(c *kubernetes.Clientset, veleroNamespace string, panicReason interface{}, panicFrom string) string {
	log.Printf("Recovered from panic in %s: %v\n", panicFrom, panicReason)
	log.Print("returning container logs instead")
	containerLogs, err := GetVeleroContainerLogs(c, veleroNamespace)
	if err != nil {
		log.Printf("error getting container logs: %v\n", err)
	}
	return containerLogs
}

func BackupErrorLogs(c *kubernetes.Clientset, ocClient client.Client, backup velero.Backup) []string {
	bl := BackupLogs(c, ocClient, backup)
	return errorLogsExcludingIgnored(bl)
}

func RestoreErrorLogs(c *kubernetes.Clientset, ocClient client.Client, restore velero.Restore) []string {
	rl := RestoreLogs(c, ocClient, restore)
	return errorLogsExcludingIgnored(rl)
}

func errorLogsExcludingIgnored(logs string) []string {
	errorRegex, err := regexp.Compile("error|Error")
	if err != nil {
		return []string{"could not compile regex: ", err.Error()}
	}
	logLines := []string{}
	for _, line := range strings.Split(logs, "\n") {
		if errorRegex.MatchString(line) {
			// ignore some expected errors
			ignoreLine := false
			for _, ignore := range errorIgnorePatterns {
				ignoreLine, _ = regexp.MatchString(ignore, line)
				if ignoreLine {
					break
				}
			}
			if !ignoreLine {
				logLines = append(logLines, line)
			}
		}
	}
	return logLines
}

func GetVeleroDeploymentList(c *kubernetes.Clientset, namespace string) (*appsv1.DeploymentList, error) {
	registryListOptions := metav1.ListOptions{
		LabelSelector: "component=velero",
	}
	// get pods in the oadp-operator-e2e namespace with label selector
	deploymentList, err := c.AppsV1().Deployments(namespace).List(context.Background(), registryListOptions)
	if err != nil {
		return nil, err
	}
	return deploymentList, nil
}

func RunDcPostRestoreScript(dcRestoreName string) error {
	log.Printf("Running post restore script for %s", dcRestoreName)
	currentDir, err := os.Getwd()
	if err != nil {
		return err
	}
	currentDir = strings.TrimSuffix(currentDir, "/tests/e2e")
	var stderrOutput bytes.Buffer
	command := exec.Command("bash", currentDir+"/docs/scripts/dc-post-restore.sh", dcRestoreName)
	command.Stderr = &stderrOutput
	stdOut, err := command.Output()
	log.Printf("command: %s", command.String())
	log.Printf("stdout:\n%s", stdOut)
	log.Printf("stderr:\n%s", stderrOutput.String())
	log.Printf("err: %v", err)
	return err
}

package lib

import (
	"bytes"
	"context"
	"log"
	"regexp"
	"strings"
	"time"

	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	snapshotv1client "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	pkgbackup "github.com/vmware-tanzu/velero/pkg/backup"
	"github.com/vmware-tanzu/velero/pkg/cmd"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/downloadrequest"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/output"
	"github.com/vmware-tanzu/velero/pkg/features"
	veleroClientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	"github.com/vmware-tanzu/velero/pkg/label"
	"github.com/vmware-tanzu/velero/pkg/restic"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetVeleroClient() (veleroClientset.Interface, error) {
	if vc, err := veleroClientset.NewForConfig(getKubeConfig()); err == nil {
		return vc, nil
	} else {
		return nil, err
	}
}

// https://github.com/vmware-tanzu/velero/blob/11bfe82342c9f54c63f40d3e97313ce763b446f2/pkg/cmd/cli/backup/describe.go#L77-L111
func DescribeBackup(ocClient client.Client, backup velero.Backup) string {
	err := ocClient.Get(context.Background(), client.ObjectKey{
		Namespace: backup.Namespace,
		Name:      backup.Name,
	}, &backup)
	if err != nil {
		return "could not get provided backup: " + err.Error()
	}
	veleroClient, err := GetVeleroClient()
	if err != nil {
		return err.Error()
	}
	details := true
	insecureSkipTLSVerify := true
	caCertFile := ""

	deleteRequestListOptions := pkgbackup.NewDeleteBackupRequestListOptions(backup.Name, string(backup.UID))
	deleteRequestList, err := veleroClient.VeleroV1().DeleteBackupRequests(backup.Namespace).List(context.TODO(), deleteRequestListOptions)
	if err != nil {
		log.Printf("error getting DeleteBackupRequests for backup %s: %v\n", backup.Name, err)
	}

	opts := label.NewListOptionsForBackup(backup.Name)
	podVolumeBackupList, err := veleroClient.VeleroV1().PodVolumeBackups(backup.Namespace).List(context.TODO(), opts)
	if err != nil {
		log.Printf("error getting PodVolumeBackups for backup %s: %v\n", backup.Name, err)
	}

	var csiClient *snapshotv1client.Clientset
	// declare vscList up here since it may be empty and we'll pass the empty Items field into DescribeBackup
	vscList := new(snapshotv1api.VolumeSnapshotContentList)
	if features.IsEnabled(velero.CSIFeatureFlag) {
		clientConfig := getKubeConfig()

		csiClient, err = snapshotv1client.NewForConfig(clientConfig)
		cmd.CheckError(err)

		vscList, err = csiClient.SnapshotV1().VolumeSnapshotContents().List(context.TODO(), opts)
		if err != nil {
			log.Printf("error getting VolumeSnapshotContent objects for backup %s: %v\n", backup.Name, err)
		}
	}

	return output.DescribeBackup(context.Background(), ocClient, &backup, deleteRequestList.Items, podVolumeBackupList.Items, vscList.Items, details, veleroClient, insecureSkipTLSVerify, caCertFile)
}

// https://github.com/vmware-tanzu/velero/blob/11bfe82342c9f54c63f40d3e97313ce763b446f2/pkg/cmd/cli/restore/describe.go#L72-L78
func DescribeRestore(ocClient client.Client, restore velero.Restore) string {
	err := ocClient.Get(context.Background(), client.ObjectKey{
		Namespace: restore.Namespace,
		Name:      restore.Name,
	}, &restore)
	if err != nil {
		return "could not get provided backup: " + err.Error()
	}
	veleroClient, err := GetVeleroClient()
	if err != nil {
		return err.Error()
	}
	details := true
	insecureSkipTLSVerify := true
	caCertFile := ""
	opts := restic.NewPodVolumeRestoreListOptions(restore.Name)
	podvolumeRestoreList, err := veleroClient.VeleroV1().PodVolumeRestores(restore.Namespace).List(context.TODO(), opts)
	if err != nil {
		log.Printf("error getting PodVolumeRestores for restore %s: %v\n", restore.Name, err)
	}

	return output.DescribeRestore(context.Background(), ocClient, &restore, podvolumeRestoreList.Items, details, veleroClient, insecureSkipTLSVerify, caCertFile)
}

func BackupLogs(ocClient client.Client, backup velero.Backup) string {
	insecureSkipTLSVerify := true
	caCertFile := ""
	// new io.Writer that store the logs in a string
	logs := &bytes.Buffer{}
	// new io.Writer that store the logs in a string

	downloadrequest.Stream(context.Background(), ocClient, backup.Namespace, backup.Name, velero.DownloadTargetKindBackupLog, logs, time.Minute, insecureSkipTLSVerify, caCertFile)

	return logs.String()
}

func RestoreLogs(ocClient client.Client, restore velero.Restore) string {
	insecureSkipTLSVerify := true
	caCertFile := ""
	// new io.Writer that store the logs in a string
	logs := &bytes.Buffer{}
	// new io.Writer that store the logs in a string

	downloadrequest.Stream(context.Background(), ocClient, restore.Namespace, restore.Name, velero.DownloadTargetKindRestoreLog, logs, time.Minute, insecureSkipTLSVerify, caCertFile)

	return logs.String()
}

var errorIgnorePatterns = []string{
	"received EOF, stopping recv loop",
	"Checking for AWS specific error information",
	"awserr.Error contents",
	"Error creating parent directories for blob-info-cache-v1.boltdb",
	"blob unknown",
}

func BackupErrorLogs(ocClient client.Client, backup velero.Backup) []string {
	bl := BackupLogs(ocClient, backup)
	errorRegex, err := regexp.Compile("error|Error")
	if err != nil {
		return []string{"could not compile regex: ", err.Error()}
	}
	logLines := []string{}
	for _, line := range strings.Split(bl, "\n") {
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

func RestoreErrorLogs(ocClient client.Client, restore velero.Restore) []string {
	rl := RestoreLogs(ocClient, restore)
	errorRegex, err := regexp.Compile("error|Error")
	if err != nil {
		return []string{"could not compile regex: ", err.Error()}
	}
	logLines := []string{}
	for _, line := range strings.Split(rl, "\n") {
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

func GetVeleroDeploymentList(namespace string) (*appsv1.DeploymentList, error) {
	client, err := setUpClient()
	if err != nil {
		return nil, err
	}
	registryListOptions := metav1.ListOptions{
		LabelSelector: "component=velero",
	}
	// get pods in the oadp-operator-e2e namespace with label selector
	deploymentList, err := client.AppsV1().Deployments(namespace).List(context.TODO(), registryListOptions)
	if err != nil {
		return nil, err
	}
	return deploymentList, nil
}

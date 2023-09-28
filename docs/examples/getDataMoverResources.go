package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
)

var (
	backup    bool
	restore   bool
	details   bool
	namespace string

	err error

	ClusterConfig    *rest.Config
	KubernetesClient *kubernetes.Clientset
	DynamicClient    dynamic.Interface

	VolumeSnapshotResource = schema.GroupVersionResource{
		Group:    "snapshot.storage.k8s.io",
		Version:  "v1",
		Resource: "volumesnapshots",
	}
	VolumeSnapshotContentResource = schema.GroupVersionResource{
		Group:    "snapshot.storage.k8s.io",
		Version:  "v1",
		Resource: "volumesnapshotcontents",
	}
	VolumeSnapshotBackupResource = schema.GroupVersionResource{
		Group:    "datamover.oadp.openshift.io",
		Version:  "v1alpha1",
		Resource: "volumesnapshotbackups",
	}
	VolumeSnapshotRestoreResource = schema.GroupVersionResource{
		Group:    "datamover.oadp.openshift.io",
		Version:  "v1alpha1",
		Resource: "volumesnapshotrestores",
	}
	ReplicationSourceResource = schema.GroupVersionResource{
		Group:    "volsync.backube",
		Version:  "v1alpha1",
		Resource: "replicationsources",
	}
	ReplicationDestinationResource = schema.GroupVersionResource{
		Group:    "volsync.backube",
		Version:  "v1alpha1",
		Resource: "replicationdestinations",
	}
)

// TODO Does this work with built in DataMover?
var cli = &cobra.Command{
	Use: "go run docs/examples/getDataMoverResources.go",
	Long: `Check the OADP DataMover Custom Resources in real time.

You need to log in to your cluster and deploy OADP with DataMover prior to run this script`,
	Args: cobra.NoArgs,
	Example: `  # Check the OADP DataMover Backup Resources
  go run docs/examples/getDataMoverResources.go -b

  # Check the OADP DataMover Restore Resources with details
  go run docs/examples/getDataMoverResources.go -r -d`,
	Run: func(cmd *cobra.Command, args []string) {
		getClients()
		if backup {
			backupSummary()
		}
		if restore {
			restoreSummary()
		}
	},
}

func init() {
	cli.Flags().BoolVarP(&backup, "backup", "b", false, "Check a backup")
	cli.Flags().BoolVarP(&restore, "restore", "r", false, "Check a restore")
	cli.MarkFlagsMutuallyExclusive("backup", "restore")
	// TODO https://github.com/spf13/cobra/pull/1952
	// cli.MarkFlagsOneRequired("backup", "restore")
	cli.Flags().BoolVarP(&details, "details", "d", false, "Print a list of the relevant backup/restore CR's")
	cli.Flags().StringVarP(&namespace, "namespace", "n", "openshift-adp", "Namespace OADP was deployed")

	cli.SetHelpCommand(&cobra.Command{Hidden: true})
}

func main() {
	err := cli.Execute()
	exitOnError(err)
}

func exitOnError(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func getClients() {
	ClusterConfig, err = clientcmd.BuildConfigFromFlags("", filepath.Join(homedir.HomeDir(), ".kube", "config"))
	exitOnError(err)

	KubernetesClient, err = kubernetes.NewForConfig(ClusterConfig)
	exitOnError(err)

	DynamicClient, err = dynamic.NewForConfig(ClusterConfig)
	exitOnError(err)
}

func getClusterResourceItems(resourceSpecification schema.GroupVersionResource, namespace string) []unstructured.Unstructured {
	resource, err := DynamicClient.Resource(resourceSpecification).Namespace(namespace).List(context.Background(), metaV1.ListOptions{})
	exitOnError(err)
	return resource.Items
}

func getPodForDeployment(deployment *appsV1.Deployment) coreV1.Pod {
	listOptions := metaV1.ListOptions{LabelSelector: labels.Set(deployment.Spec.Selector.MatchLabels).AsSelector().String()}
	pods, err := KubernetesClient.CoreV1().Pods(namespace).List(context.Background(), listOptions)
	exitOnError(err)
	if len(pods.Items) > 1 {
		fmt.Printf("More than one Pod (%v) for deployment %v\n", pods.Items, deployment)
		os.Exit(1)
	}
	return pods.Items[0]
}

func execCommandInVeleroPod(command string) io.Reader {
	deployment, err := KubernetesClient.AppsV1().Deployments(namespace).Get(context.Background(), "velero", metaV1.GetOptions{})
	exitOnError(err)

	option := &coreV1.PodExecOptions{
		Command:   strings.Split(command, " "),
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
		Container: "velero",
	}
	request := KubernetesClient.CoreV1().RESTClient().Post().Resource("pods").
		Name(getPodForDeployment(deployment).Name).Namespace(namespace).SubResource("exec").
		VersionedParams(option, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(ClusterConfig, "POST", request.URL())
	exitOnError(err)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}

	err = exec.Stream(remotecommand.StreamOptions{Stdout: output, Stderr: errOutput})
	if err != nil {
		fmt.Println(output)
		fmt.Println(errOutput)
		fmt.Println(err)
		os.Exit(1)
	}
	return output
}

func backupSummary() {
	backups := execCommandInVeleroPod("./velero get backup")
	fmt.Printf("Get Backups:\n%v\n", backups)

	volumeSnapshots := getClusterResourceItems(VolumeSnapshotResource, "")
	fmt.Printf("Total Snapshots: %v\n", len(volumeSnapshots))
	namespaceVolumeSnapshots := getClusterResourceItems(VolumeSnapshotResource, namespace)
	fmt.Printf("Total OADP Snapshots: %v\n", len(namespaceVolumeSnapshots))
	volumeSnapshotContents := getClusterResourceItems(VolumeSnapshotContentResource, "")
	fmt.Printf("Total SnapshotContents: %v\n\n", len(volumeSnapshotContents))

	volumeSnapshotBackups := getClusterResourceItems(VolumeSnapshotBackupResource, "")
	phaseCompleted := 0
	phaseInProgress := 0
	phaseSnapshotBackupDone := 0

	batchingCompleted := 0
	batchingProcessing := 0
	batchingQueued := 0
	increaseBatching := map[string]func(){
		"Completed":  func() { batchingCompleted += 1 },
		"Processing": func() { batchingProcessing += 1 },
		"Queued":     func() { batchingQueued += 1 },
	}
	for _, volumeSnapshotBackup := range volumeSnapshotBackups {
		status := volumeSnapshotBackup.Object["status"].(map[string]interface{})
		if status["phase"] == "Completed" {
			phaseCompleted += 1
		}
		if status["phase"] == "InProgress" {
			phaseInProgress += 1
		}
		if status["phase"] == "SnapshotBackupDone" {
			phaseSnapshotBackupDone += 1
		}
		increaseBatching[status["batchingStatus"].(string)]()
	}
	fmt.Printf("Total VSB: %v\n", len(volumeSnapshotBackups))
	fmt.Printf("  Completed: %v\n", phaseCompleted)
	fmt.Printf("  InProgress: %v\n", phaseInProgress)
	fmt.Printf("  SnapshotBackupDone: %v\n\n", phaseSnapshotBackupDone)
	fmt.Println("VSB STATUS:")
	fmt.Printf("  Completed: %v\n", batchingCompleted)
	fmt.Printf("  Processing: %v\n", batchingProcessing)
	fmt.Printf("  Queued: %v\n\n", batchingQueued)

	replicationSources := getClusterResourceItems(ReplicationSourceResource, "")
	fmt.Printf("Total ReplicationSources: %v\n", len(replicationSources))
	if details {
		volumeSnapshotContentDetails()
		replicationSourceDetails()
	}
}

func restoreSummary() {
	restores := execCommandInVeleroPod("./velero get restore")
	fmt.Printf("Get Restores:\n%v\n", restores)

	volumeSnapshotRestores := getClusterResourceItems(VolumeSnapshotRestoreResource, "")
	phaseCompleted := 0
	phaseInProgress := 0
	phaseSnapshotRestoreDone := 0

	batchingCompleted := 0
	batchingProcessing := 0
	batchingQueued := 0
	increaseBatching := map[string]func(){
		"Completed":  func() { batchingCompleted += 1 },
		"Processing": func() { batchingProcessing += 1 },
		"Queued":     func() { batchingQueued += 1 },
	}
	for _, volumeSnapshotBackup := range volumeSnapshotRestores {
		status := volumeSnapshotBackup.Object["status"].(map[string]interface{})
		if status["phase"] == "Completed" {
			phaseCompleted += 1
		}
		if status["phase"] == "InProgress" {
			phaseInProgress += 1
		}
		if status["phase"] == "SnapshotRestoreDone" {
			phaseSnapshotRestoreDone += 1
		}
		increaseBatching[status["batchingStatus"].(string)]()
	}
	fmt.Printf("Total VSR: %v\n", len(volumeSnapshotRestores))
	fmt.Printf("  Completed: %v\n", phaseCompleted)
	fmt.Printf("  InProgress: %v\n", phaseInProgress)
	fmt.Printf("  SnapshotRestoreDone: %v\n\n", phaseSnapshotRestoreDone)
	fmt.Println("VSR STATUS:")
	fmt.Printf("  Completed: %v\n", batchingCompleted)
	fmt.Printf("  Processing: %v\n", batchingProcessing)
	fmt.Printf("  Queued: %v\n\n", batchingQueued)

	replicationDestinations := getClusterResourceItems(ReplicationDestinationResource, "")
	fmt.Printf("Total ReplicationDestinations: %v\n", len(replicationDestinations))
	if details {
		volumeSnapshotContentDetails()
		replicationDestinationDetails()
	}
}

func volumeSnapshotContentDetails() {
	fmt.Println("\n***** VOLUME SNAPSHOT CONTENT ******")
	volumeSnapshotContents := getClusterResourceItems(VolumeSnapshotContentResource, "")
	for _, volumeSnapshotContent := range volumeSnapshotContents {
		fmt.Printf(
			"Name:  %v  ReadyToUse:  %v  creationTime:  %v\n",
			volumeSnapshotContent.GetName(),
			volumeSnapshotContent.Object["status"].(map[string]interface{})["readyToUse"],
			volumeSnapshotContent.GetCreationTimestamp().UTC(),
		)
	}
}

func replicationSourceDetails() {
	fmt.Println("\n***** REPLICATION SOURCE ******")
	replicationSources := getClusterResourceItems(ReplicationSourceResource, "")
	for _, replicationSource := range replicationSources {
		fmt.Printf(
			"Name:  %v  SyncDuration:  %v  sourcePVC:  %v\n",
			replicationSource.GetName(),
			replicationSource.Object["status"].(map[string]interface{})["lastSyncDuration"],
			replicationSource.Object["spec"].(map[string]interface{})["sourcePVC"],
		)
	}
}

func replicationDestinationDetails() {
	fmt.Println("\n***** REPLICATION DESTINATION ******")
	replicationDestinations := getClusterResourceItems(ReplicationDestinationResource, "")
	for _, replicationDestination := range replicationDestinations {
		fmt.Printf(
			"Name:  %v  SyncDuration:  %v  sourcePVC:  %v\n",
			replicationDestination.GetName(),
			replicationDestination.Object["status"].(map[string]interface{})["lastSyncDuration"],
			replicationDestination.Object["spec"].(map[string]interface{})["sourcePVC"],
		)
	}
}

package e2e_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

type VerificationFunction func(client.Client, string) error

type BackupRestoreCase struct {
	Namespace                string
	Name                     string
	BackupRestoreType        lib.BackupRestoreType
	PreBackupVerify          VerificationFunction
	PostRestoreVerify        VerificationFunction
	SkipVerifyLogs           bool // TODO remove
	BackupTimeout            time.Duration
	ExcludedResources        []string             // Resources to exclude from backup spec
	RestoreHooks             *velero.RestoreHooks // Pre/post restore hooks (optional)
	IncludedResources        []string             // Resources to include in restore (optional)
	RestoreExcludedResources []string             // Resources to exclude from restore (optional)
}

type ApplicationBackupRestoreCase struct {
	BackupRestoreCase
	ApplicationTemplate string
	PvcSuffixName       string
}

func todoListReady(preBackupState bool, twoVol bool, database string) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		log.Printf("checking for the NAMESPACE: %s", namespace)
		gomega.Eventually(lib.IsDeploymentReady(ocClient, namespace, database), time.Minute*10, time.Second*10).Should(gomega.BeTrue())
		// perhaps we phase out deploymentConfigs?
		//gomega.Eventually(lib.IsDCReady(ocClient, namespace, "todolist"), time.Minute*10, time.Second*10).Should(gomega.BeTrue())
		gomega.Eventually(lib.AreApplicationPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*9, time.Second*5).Should(gomega.BeTrue())
		// This test confirms that SCC restore logic in our plugin is working
		err := lib.DoesSCCExist(ocClient, database+"-persistent-scc")
		if err != nil {
			return err
		}
		err = lib.VerifyBackupRestoreData(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, kubeConfig, artifact_dir, namespace, "todolist-route", "todolist", "todolist", preBackupState, twoVol)
		return err
	})
}

func parksAppReady(preBackupState bool, twoVol bool, DCReadyCheck bool) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		log.Printf("checking parksapp for the NAMESPACE: %s", namespace)
		if DCReadyCheck {
			gomega.Eventually(lib.IsDCReady(ocClient, namespace, "restify"), time.Minute*10, time.Second*10).Should(gomega.BeTrue())
		}
		gomega.Eventually(lib.AreApplicationPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*9, time.Second*5).Should(gomega.BeTrue())
		err := lib.VerifyBackupRestoreData(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, kubeConfig, artifact_dir, namespace, "restify", "restify", "restify", preBackupState, twoVol)
		return err
	})
}

func waitOADPReadiness(backupRestoreType lib.BackupRestoreType) {
	err := dpaCR.CreateOrUpdate(dpaCR.Build(backupRestoreType))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Print("Checking if DPA is reconciled")
	gomega.Eventually(dpaCR.IsReconciledTrue(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

	log.Printf("Waiting for Velero Pod to be running")
	gomega.Eventually(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

	if backupRestoreType == lib.KOPIA || backupRestoreType == lib.CSIDataMover {
		log.Printf("Waiting for Node Agent pods to be running")
		gomega.Eventually(lib.AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
	}

	// Velero does not change status of VSL objects. Users can only confirm if VSLs are correct configured when running a native snapshot backup/restore

	log.Print("Checking if BSL is available")
	gomega.Eventually(dpaCR.BSLsAreAvailable(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
}

func prepareBackupAndRestore(brCase BackupRestoreCase, updateLastInstallTime func()) (string, string) {
	updateLastInstallTime()

	waitOADPReadiness(brCase.BackupRestoreType)

	if brCase.BackupRestoreType == lib.CSI || brCase.BackupRestoreType == lib.CSIDataMover {
		if provider == "aws" || provider == "ibmcloud" || provider == "gcp" || provider == "azure" || provider == "openstack" {
			log.Printf("Creating VolumeSnapshotClass for CSI backuprestore of %s", brCase.Name)
			snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", provider)
			err := lib.InstallApplication(dpaCR.Client, snapshotClassPath)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	}

	// TODO: check registry deployments are deleted
	// TODO: check S3 for images

	backupUid, _ := uuid.NewUUID()
	restoreUid, _ := uuid.NewUUID()
	backupName := fmt.Sprintf("%s-%s", brCase.Name, backupUid.String())
	restoreName := fmt.Sprintf("%s-%s", brCase.Name, restoreUid.String())

	return backupName, restoreName
}

func runApplicationBackupAndRestore(brCase ApplicationBackupRestoreCase, updateLastBRcase func(brCase ApplicationBackupRestoreCase), updateLastInstallTime func()) {
	updateLastBRcase(brCase)

	// create DPA
	backupName, restoreName := prepareBackupAndRestore(brCase.BackupRestoreCase, updateLastInstallTime)

	// Ensure that an existing backup repository is deleted
	brerr := lib.DeleteBackupRepositories(runTimeClientForSuiteRun, namespace)
	gomega.Expect(brerr).ToNot(gomega.HaveOccurred())

	// install app
	updateLastInstallTime()
	log.Printf("Installing application for case %s", brCase.Name)
	err := lib.InstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	var pvcPath string
	if brCase.BackupRestoreType == lib.CSI || brCase.BackupRestoreType == lib.CSIDataMover {
		log.Printf("Creating pvc for case %s", brCase.Name)
		var pvcName string

		pvcName = provider
		if brCase.PvcSuffixName != "" {
			pvcName += brCase.PvcSuffixName
		}

		pvcPathFormat := "./sample-applications/%s/pvc/%s.yaml"
		if strings.Contains(brCase.Name, "twovol") {
			pvcPathFormat = "./sample-applications/%s/pvc-twoVol/%s.yaml"
		}

		pvcPath = fmt.Sprintf(pvcPathFormat, brCase.Namespace, pvcName)

		err = lib.InstallApplication(dpaCR.Client, pvcPath)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}

	// Run optional custom verification
	if brCase.PreBackupVerify != nil {
		log.Printf("Running pre-backup custom function for case %s", brCase.Name)
		err := brCase.PreBackupVerify(dpaCR.Client, brCase.Namespace)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}

	// do the backup for real
	nsRequiredResticDCWorkaround := runBackup(brCase.BackupRestoreCase, backupName)

	// uninstall app
	log.Printf("Uninstalling app for case %s", brCase.Name)
	err = lib.UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// Ensure that we remove the pvc(s) before waiting for namespace deletion
	if brCase.BackupRestoreType == lib.CSI || brCase.BackupRestoreType == lib.CSIDataMover {
		// For CSI/CSIDataMover tests, delete the explicitly created PVC
		log.Printf("Deleting PVC from %s for case %s", pvcPath, brCase.Name)
		err = lib.UninstallApplication(dpaCR.Client, pvcPath)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}

	// Delete all PVCs in the namespace to ensure clean state
	log.Printf("Deleting all PVCs in namespace %s", brCase.Namespace)
	err = lib.DeleteAllPVCsInNamespace(kubernetesClientForSuiteRun, brCase.Namespace)
	if err != nil {
		log.Printf("Warning: Failed to delete PVCs in namespace %s: %v", brCase.Namespace, err)
	}

	// Wait for namespace to be deleted
	log.Printf("Waiting for namespace %s to be deleted", brCase.Namespace)
	gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), time.Minute*4, time.Second*5).Should(gomega.BeTrue())

	updateLastInstallTime()

	// run restore
	runRestore(brCase.BackupRestoreCase, backupName, restoreName, nsRequiredResticDCWorkaround)

	// Run optional custom verification
	if brCase.PostRestoreVerify != nil {
		log.Printf("Running post-restore custom function for case %s", brCase.Name)
		err = brCase.PostRestoreVerify(dpaCR.Client, brCase.Namespace)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
}

func runBackup(brCase BackupRestoreCase, backupName string) bool {
	nsRequiresResticDCWorkaround, err := lib.NamespaceRequiresResticDCWorkaround(dpaCR.Client, brCase.Namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	if strings.Contains(brCase.Name, "twovol") {
		volumeSyncDelay := 30 * time.Second
		log.Printf("Sleeping for %v to allow volume to be in sync with /tmp/log/ for case %s", volumeSyncDelay, brCase.Name)
		// TODO this should be a function, not an arbitrary sleep
		time.Sleep(volumeSyncDelay)
	}

	// create backup
	log.Printf("Creating backup %s for case %s", backupName, brCase.Name)
	if len(brCase.ExcludedResources) > 0 {
		log.Printf("Creating backup with excluded resources: %v", brCase.ExcludedResources)
		err = lib.CreateCustomBackupForNamespaces(dpaCR.Client, namespace, backupName, []string{brCase.Namespace}, nil, brCase.ExcludedResources, brCase.BackupRestoreType == lib.KOPIA, brCase.BackupRestoreType == lib.CSIDataMover)
	} else {
		err = lib.CreateBackupForNamespaces(dpaCR.Client, namespace, backupName, []string{brCase.Namespace}, brCase.BackupRestoreType == lib.KOPIA, brCase.BackupRestoreType == lib.CSIDataMover)
	}
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// wait for backup to not be running
	gomega.Eventually(lib.IsBackupDone(dpaCR.Client, namespace, backupName), brCase.BackupTimeout, time.Second*10).Should(gomega.BeTrue())
	// TODO only log on fail?
	describeBackup := lib.DescribeBackup(dpaCR.Client, namespace, backupName)
	ginkgo.GinkgoWriter.Println(describeBackup)

	backupLogs := lib.BackupLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
	backupErrorLogs := lib.BackupErrorLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
	accumulatedTestLogs = append(accumulatedTestLogs, describeBackup, backupLogs)

	if !brCase.SkipVerifyLogs {
		gomega.Expect(backupErrorLogs).Should(gomega.Equal([]string{}))
	}

	// check if backup succeeded
	succeeded, err := lib.IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(succeeded).To(gomega.Equal(true))
	log.Printf("Backup for case %s succeeded", brCase.Name)

	return nsRequiresResticDCWorkaround
}

func runRestore(brCase BackupRestoreCase, backupName, restoreName string, nsRequiresResticDCWorkaround bool) {
	log.Printf("Creating restore %s for case %s", restoreName, brCase.Name)
	var err error

	// Use custom restore if hooks or resource filters are specified
	if brCase.RestoreHooks != nil || len(brCase.IncludedResources) > 0 || len(brCase.RestoreExcludedResources) > 0 {
		log.Printf("Creating custom restore with hooks and/or resource filters")
		if brCase.RestoreHooks != nil {
			log.Printf("Restore hooks configured: %d resource hook(s)", len(brCase.RestoreHooks.Resources))
		}
		if len(brCase.IncludedResources) > 0 {
			log.Printf("Included resources: %v", brCase.IncludedResources)
		}
		if len(brCase.RestoreExcludedResources) > 0 {
			log.Printf("Excluded resources: %v", brCase.RestoreExcludedResources)
		}
		err = lib.CreateCustomRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName, brCase.IncludedResources, brCase.RestoreExcludedResources, brCase.RestoreHooks)
	} else {
		err = lib.CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
	}
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Eventually(lib.IsRestoreDone(dpaCR.Client, namespace, restoreName), time.Minute*60, time.Second*10).Should(gomega.BeTrue())
	// TODO only log on fail?
	describeRestore := lib.DescribeRestore(dpaCR.Client, namespace, restoreName)
	ginkgo.GinkgoWriter.Println(describeRestore)

	restoreLogs := lib.RestoreLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	restoreErrorLogs := lib.RestoreErrorLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	accumulatedTestLogs = append(accumulatedTestLogs, describeRestore, restoreLogs)

	if !brCase.SkipVerifyLogs {
		gomega.Expect(restoreErrorLogs).Should(gomega.Equal([]string{}))
	}

	// Check if restore succeeded
	succeeded, err := lib.IsRestoreCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(succeeded).To(gomega.Equal(true))

	if nsRequiresResticDCWorkaround {
		// We run the dc-post-restore.sh script for both restic and
		// kopia backups and for any DCs with attached volumes,
		// regardless of whether it was restic or kopia backup.
		// The script is designed to work with labels set by the
		// openshift-velero-plugin and can be run without pre-conditions.
		log.Printf("Running dc-post-restore.sh script.")
		err = lib.RunDcPostRestoreScript(restoreName)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
}

func getFailedTestLogs(oadpNamespace string, appNamespace string, installTime time.Time, report ginkgo.SpecReport) {
	baseReportDir := artifact_dir + "/" + report.LeafNodeText
	log.Println("Storing failed test logs in: ", baseReportDir)
	err := os.MkdirAll(baseReportDir, 0755)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Println("Printing OADP namespace events")
	lib.PrintNamespaceEventsAfterTime(kubernetesClientForSuiteRun, oadpNamespace, installTime)
	err = lib.SavePodLogs(kubernetesClientForSuiteRun, oadpNamespace, baseReportDir)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	if appNamespace != "" {
		log.Println("Printing app namespace events")
		lib.PrintNamespaceEventsAfterTime(kubernetesClientForSuiteRun, appNamespace, installTime)
		err = lib.SavePodLogs(kubernetesClientForSuiteRun, appNamespace, baseReportDir)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
}

func tearDownBackupAndRestore(brCase BackupRestoreCase, installTime time.Time, report ginkgo.SpecReport) {
	log.Println("Post backup and restore state: ", report.State.String())
	gatherLogs(brCase, installTime, report)
	tearDownDPAResources(brCase)
	deleteNamespace(brCase.Namespace)
}

func tearDownDPAResources(brCase BackupRestoreCase) {
	if brCase.BackupRestoreType == lib.CSI || brCase.BackupRestoreType == lib.CSIDataMover {
		log.Printf("Deleting VolumeSnapshot for CSI backuprestore of %s", brCase.Name)
		snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", provider)
		err := lib.UninstallApplication(dpaCR.Client, snapshotClassPath)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}

	// Clean up DownloadRequests to prevent accumulation of stale resources
	log.Printf("Deleting DownloadRequests in namespace %s", namespace)
	err := lib.DeleteDownloadRequests(dpaCR.Client, namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	err = dpaCR.Delete()
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
}

func gatherLogs(brCase BackupRestoreCase, installTime time.Time, report ginkgo.SpecReport) {
	if report.Failed() {
		knownFlake = lib.CheckIfFlakeOccurred(accumulatedTestLogs)
		accumulatedTestLogs = nil
		getFailedTestLogs(namespace, brCase.Namespace, installTime, report)
	}
}

func deleteNamespace(namespace string) {
	err := lib.DeleteNamespace(kubernetesClientForSuiteRun, namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, namespace), time.Minute*5, time.Second*5).Should(gomega.BeTrue())
}

var _ = ginkgo.Describe("Backup and restore tests", ginkgo.Ordered, func() {
	var lastBRCase ApplicationBackupRestoreCase
	var lastInstallTime time.Time
	updateLastBRcase := func(brCase ApplicationBackupRestoreCase) {
		lastBRCase = brCase
	}
	updateLastInstallTime := func() {
		lastInstallTime = time.Now()
	}

	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		tearDownBackupAndRestore(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	var _ = ginkgo.AfterAll(func() {
		// DPA just needs to have BSL so gathering of backups/restores logs/describe work
		// using kopia to collect more info (DaemonSet)
		waitOADPReadiness(lib.KOPIA)

		//DPT Test and MustGather should be paired together
		log.Printf("skipMustGather: %v", skipMustGather)
		if !skipMustGather {
			log.Printf("Creating real DataProtectionTest before must-gather")
			bsls, err := dpaCR.ListBSLs()
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			bslName := bsls.Items[0].Name
			err = lib.CreateUploadTestOnlyDPT(dpaCR.Client, dpaCR.Namespace, bslName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			log.Printf("Running OADP must-gather")
			err = lib.RunMustGather(artifact_dir, dpaCR.Client)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		} else {
			log.Printf("Skipping MustGather and DataProtectionTest")
		}

		err := dpaCR.Delete()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	ginkgo.DescribeTable("Backup and restore applications",
		func(brCase ApplicationBackupRestoreCase, expectedErr error) {
			if ginkgo.CurrentSpecReport().NumAttempts > 1 && !knownFlake {
				ginkgo.Fail("No known FLAKE found in a previous run, marking test as failed.")
			}
			runApplicationBackupAndRestore(brCase, updateLastBRcase, updateLastInstallTime)
		},
		ginkgo.Entry("MySQL application CSI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-csi-e2e",
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application CSI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-csi-e2e",
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application two Vol CSI", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-twovol-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-twovol-csi-e2e",
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   todoListReady(true, true, "mysql"),
				PostRestoreVerify: todoListReady(false, true, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application KOPIA", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-kopia-e2e",
				BackupRestoreType: lib.KOPIA,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application DATAMOVER", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-datamover-e2e",
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application DATAMOVER", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent-csi.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-datamover-e2e",
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application BlockDevice DATAMOVER", ginkgo.FlakeAttempts(flakeAttempts), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent-block.yaml",
			PvcSuffixName:       "-block-mode",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-blockdevice-e2e",
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("MySQL application Native-Snapshots", ginkgo.FlakeAttempts(flakeAttempts), ginkgo.Label("aws", "azure", "gcp"), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "mysql-native-snapshots-e2e",
				BackupRestoreType: lib.NativeSnapshots,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Mongo application Native-Snapshots", ginkgo.FlakeAttempts(flakeAttempts), ginkgo.Label("aws", "azure", "gcp"), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mongo-persistent",
				Name:              "mongo-native-snapshots-e2e",
				BackupRestoreType: lib.NativeSnapshots,
				PreBackupVerify:   todoListReady(true, false, "mongo"),
				PostRestoreVerify: todoListReady(false, false, "mongo"),
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),
		ginkgo.Entry("Parks application Native-Snapshots", ginkgo.FlakeAttempts(flakeAttempts), ginkgo.Label("aws", "azure", "gcp"), ApplicationBackupRestoreCase{
			ApplicationTemplate: "./sample-applications/parks-app/manifest.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "parks-app",
				Name:              "mongo-parksapp-native-snapshots-e2e",
				BackupRestoreType: lib.NativeSnapshots,
				PreBackupVerify:   parksAppReady(true, false, true),
				PostRestoreVerify: parksAppReady(false, false, false),
				BackupTimeout:     20 * time.Minute,
				ExcludedResources: []string{"jobs.batch", "events", "events.events.k8s.io", "buildconfigs.build.openshift.io", "builds.build.openshift.io"},
				RestoreHooks: &velero.RestoreHooks{
					Resources: []velero.RestoreResourceHookSpec{
						{
							Name:               "wait for app to be ready",
							IncludedNamespaces: []string{"parks-app"},
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "restify",
								},
							},
							PostHooks: []velero.RestoreResourceHook{
								{
									Exec: &velero.ExecRestoreHook{
										Container: "restify",
										Command: []string{
											"/bin/sh",
											"-c",
											"sleep 10\ncurl -f http://restify:8080/status || echo \"App not ready yet\"",
										},
										ExecTimeout: metav1.Duration{Duration: 1 * time.Minute},
										WaitTimeout: metav1.Duration{Duration: 5 * time.Minute},
										OnError:     velero.HookErrorModeFail,
									},
								},
							},
						},
					},
				},
			},
		}, nil),
	)
})

// Helper function to create a dummy CA certificate with unique identifier
func createDummyCACert(identifier string) []byte {
	certTemplate := `-----BEGIN CERTIFICATE-----
MIIDazCCAlOgAwIBAgIUUf8+3K8zsP/w1P3VQ5jlMxALinkwDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExDjAMBgNVBAoM
BU9BQVBQMREWFAYDVQQDDA1EVU1NWS1DQS1DRVJUMB4XDTI0MDEwMTAwMDAwMFoX
DTM0MDEwMTAwMDAwMFowRTELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3Ju
aWExDjAMBgNVBAoMBU9BQVBQMREWFAYDVQQDDA1EVU1NWS1DQS1DRVJUMIIBIJAN
BgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA0VUxbPWcfcOJC2qKZVv5nKqY7OZw
%s-CERT-CONTENT-%s-CERT-CONTENT-%s-CERT-CONTENT-%s-CERT-CONTENT-%s
%s-CERT-CONTENT-%s-CERT-CONTENT-%s-CERT-CONTENT-%s-CERT-CONTENT-%s
%s-CERT-CONTENT-%s-CERT-CONTENT-%s-CERT-CONTENT-%s-CERT-CONTENT-%s
%s-CERT-CONTENT-%s-CERT-CONTENT-%s-CERT-CONTENT-%s-CERT-CONTENT-%s
%s-CERT-CONTENT-%s-CERT-CONTENT-%s-CERT-CONTENT-%s-CERT-CONTENT-%s
ngpurposesonly1234567890QIDAQABMA0GCSqGSIb3DQEBCwUAA4IBAQBYfMVqNb
iVL1x+dummyenddummyenddummyenddummyenddummyenddummyenddummyenddum
%s-CERT-END-%s-CERT-END-%s-CERT-END-%s-CERT-END-%s-CERT-END-%s-END
%s-CERT-END-%s-CERT-END-%s-CERT-END-%s-CERT-END-%s-CERT-END-%s-END
ddummyenddummyenddummyenddummyend
-----END CERTIFICATE-----`

	// Replace placeholders with the identifier
	cert := certTemplate
	for i := 0; i < 50; i++ {
		cert = strings.Replace(cert, "%s", identifier, 1)
	}
	return []byte(cert)
}

var _ = ginkgo.Describe("Multiple BSL with custom CA cert tests", ginkgo.Ordered, func() {
	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		log.Printf("Cleaning up after BSL CA cert test")
		if !skipMustGather && ctx.SpecReport().Failed() {
			log.Printf("Running must-gather for failed test")
			_ = lib.RunMustGather(artifact_dir, dpaCR.Client)
		}
		log.Printf("Deleting DPA")
		err := dpaCR.Delete()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		log.Printf("Waiting for velero to be deleted")
		gomega.Eventually(lib.VeleroIsDeleted(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
	})

	ginkgo.DescribeTable("BSL CA certificate handling with multiple BSLs",
		func(backupImages bool, expectCACertHandling bool) {
			testNamespace := "test-bsl-cacert"

			log.Printf("Creating test namespace %s", testNamespace)
			err := lib.CreateNamespace(kubernetesClientForSuiteRun, testNamespace)
			gomega.Expect(err).To(gomega.BeNil())
			gomega.Expect(lib.DoesNamespaceExist(kubernetesClientForSuiteRun, testNamespace)).Should(gomega.BeTrue())

			defer func() {
				log.Printf("Cleaning up test namespace %s", testNamespace)
				_ = lib.DeleteNamespace(kubernetesClientForSuiteRun, testNamespace)
			}()

			log.Printf("Test case: backupImages=%v, expectCACertHandling=%v", backupImages, expectCACertHandling)

			// Create unique CA certificates for each BSL
			secondCACert := createDummyCACert("SECOND")
			thirdCACert := createDummyCACert("THIRD")

			log.Printf("Creating DPA with three BSLs and backupImages=%v", backupImages)
			dpaSpec := dpaCR.Build(lib.CSI)

			// Set the backupImages flag
			dpaSpec.BackupImages = &backupImages

			// Add a second BSL with custom CA cert (it doesn't need to be available)
			secondBSL := oadpv1alpha1.BackupLocation{
				Velero: &velero.BackupStorageLocationSpec{
					Provider: dpaCR.BSLProvider,
					Default:  false,
					Config:   dpaCR.BSLConfig,
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: dpaCR.BSLSecretName,
						},
						Key: "cloud",
					},
					StorageType: velero.StorageType{
						ObjectStorage: &velero.ObjectStorageLocation{
							Bucket: dpaCR.BSLBucket,
							Prefix: dpaCR.BSLBucketPrefix + "-secondary",
							CACert: secondCACert,
						},
					},
				},
			}

			// Add a third BSL with another custom CA cert
			thirdBSL := oadpv1alpha1.BackupLocation{
				Velero: &velero.BackupStorageLocationSpec{
					Provider: dpaCR.BSLProvider,
					Default:  false,
					Config:   dpaCR.BSLConfig,
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: dpaCR.BSLSecretName,
						},
						Key: "cloud",
					},
					StorageType: velero.StorageType{
						ObjectStorage: &velero.ObjectStorageLocation{
							Bucket: dpaCR.BSLBucket,
							Prefix: dpaCR.BSLBucketPrefix + "-third",
							CACert: thirdCACert,
						},
					},
				},
			}

			dpaSpec.BackupLocations = append(dpaSpec.BackupLocations, secondBSL, thirdBSL)

			err = dpaCR.CreateOrUpdate(dpaSpec)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			log.Print("Checking if DPA is reconciled")
			gomega.Eventually(dpaCR.IsReconciledTrue(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			log.Printf("Waiting for Velero Pod to be running")
			gomega.Eventually(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			// Verify CA certificate handling based on backupImages flag
			log.Printf("Verifying CA certificate handling (backupImages: %v)", backupImages)

			veleroPods, err := kubernetesClientForSuiteRun.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
				LabelSelector: "component=velero",
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(veleroPods.Items)).To(gomega.BeNumerically(">", 0))

			veleroPod := veleroPods.Items[0]
			veleroContainer := veleroPod.Spec.Containers[0]

			if !backupImages {
				// When backupImages is false, NO CA cert processing should occur
				log.Printf("Verifying NO CA certificate processing when backupImages=false")

				// Check AWS_CA_BUNDLE env var does NOT exist
				awsCABundleFound := false
				for _, env := range veleroContainer.Env {
					if env.Name == "AWS_CA_BUNDLE" {
						awsCABundleFound = true
						log.Printf("ERROR: Found unexpected AWS_CA_BUNDLE environment variable: %s", env.Value)
						break
					}
				}
				gomega.Expect(awsCABundleFound).To(gomega.BeFalse(), "AWS_CA_BUNDLE environment variable should NOT be set when backupImages=false")

				// Verify CA cert ConfigMap is NOT mounted
				caCertVolumeMountFound := false
				for _, mount := range veleroContainer.VolumeMounts {
					if mount.Name == "custom-ca-certs" {
						caCertVolumeMountFound = true
						log.Printf("ERROR: Found unexpected CA cert volume mount: %s at %s", mount.Name, mount.MountPath)
						break
					}
				}
				gomega.Expect(caCertVolumeMountFound).To(gomega.BeFalse(), "CA cert volume should NOT be mounted when backupImages=false")

				// Verify the ConfigMap does NOT exist
				configMapName := "oadp-" + dpaCR.Name + "-ca-bundle"
				_, err := kubernetesClientForSuiteRun.CoreV1().ConfigMaps(namespace).Get(context.Background(), configMapName, metav1.GetOptions{})
				gomega.Expect(err).To(gomega.HaveOccurred(), "CA bundle ConfigMap should NOT exist when backupImages=false")
				gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue(), "ConfigMap should be not found")

			} else {
				// When backupImages is true, CA cert processing should include all three BSLs
				log.Printf("Verifying CA certificate processing when backupImages=true")

				// Check AWS_CA_BUNDLE env var exists
				awsCABundleFound := false
				awsCABundlePath := ""
				for _, env := range veleroContainer.Env {
					if env.Name == "AWS_CA_BUNDLE" {
						awsCABundleFound = true
						awsCABundlePath = env.Value
						log.Printf("Found AWS_CA_BUNDLE environment variable: %s", awsCABundlePath)
						break
					}
				}
				gomega.Expect(awsCABundleFound).To(gomega.BeTrue(), "AWS_CA_BUNDLE environment variable should be set when backupImages=true")
				gomega.Expect(awsCABundlePath).To(gomega.Equal("/etc/velero/ca-certs/ca-bundle.pem"))

				// Verify CA cert ConfigMap is mounted
				caCertVolumeMountFound := false
				for _, mount := range veleroContainer.VolumeMounts {
					if mount.Name == "ca-certificate-bundle" && mount.MountPath == "/etc/velero/ca-certs" {
						caCertVolumeMountFound = true
						log.Printf("Found CA cert volume mount: %s at %s", mount.Name, mount.MountPath)
						break
					}
				}
				gomega.Expect(caCertVolumeMountFound).To(gomega.BeTrue(), "CA cert volume should be mounted when backupImages=true")

				// Verify the ConfigMap exists and contains all three custom CAs plus system CAs
				log.Printf("Verifying CA certificate ConfigMap contents")
				configMapName := "velero-ca-bundle"
				configMap, err := kubernetesClientForSuiteRun.CoreV1().ConfigMaps(namespace).Get(context.Background(), configMapName, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				caBundleContent, exists := configMap.Data["ca-bundle.pem"]
				gomega.Expect(exists).To(gomega.BeTrue(), "ca-bundle.pem should exist in ConfigMap")

				// Verify bundle contains all three custom certificates
				gomega.Expect(caBundleContent).To(gomega.ContainSubstring("SECOND-CERT-CONTENT"), "CA bundle should contain second BSL's certificate")
				gomega.Expect(caBundleContent).To(gomega.ContainSubstring("THIRD-CERT-CONTENT"), "CA bundle should contain third BSL's certificate")

				// Verify bundle contains system certificates marker
				gomega.Expect(caBundleContent).To(gomega.ContainSubstring("# System default CA certificates"), "CA bundle should include system certificates marker")

				log.Printf("CA bundle size: %d bytes", len(caBundleContent))

				// Verify that the bundle is reasonably large (indicating system certs are included)
				// System certs are typically > 100KB
				gomega.Expect(len(caBundleContent)).To(gomega.BeNumerically(">", 50000), "CA bundle should be large enough to include system certificates")
			}

			// Check BSL status - only the default BSL needs to be available
			log.Print("Checking if default BSL is available")
			bsls, err := dpaCR.ListBSLs()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(bsls.Items)).To(gomega.Equal(3), "Should have 3 BSLs configured")

			// Find the default BSL
			var defaultBSL *velero.BackupStorageLocation
			for i, bsl := range bsls.Items {
				if bsl.Spec.Default {
					defaultBSL = &bsls.Items[i]
					break
				}
			}
			gomega.Expect(defaultBSL).NotTo(gomega.BeNil(), "Default BSL should exist")

			// Only the default BSL needs to be available for the test
			gomega.Eventually(func() bool {
				bsl := &velero.BackupStorageLocation{}
				err := dpaCR.Client.Get(context.Background(), client.ObjectKey{
					Namespace: namespace,
					Name:      defaultBSL.Name,
				}, bsl)
				if err != nil {
					return false
				}
				return bsl.Status.Phase == velero.BackupStorageLocationPhaseAvailable
			}, time.Minute*3, time.Second*5).Should(gomega.BeTrue(), "Default BSL should be available")

			log.Printf("Deploying test application")
			err = lib.InstallApplication(dpaCR.Client, "./sample-applications/nginx/nginx-deployment.yaml")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// nginx-deployment.yaml creates its own namespace, so we just wait for deployment to be ready
			gomega.Eventually(lib.IsDeploymentReady(dpaCR.Client, "nginx-example", "nginx-deployment"), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			log.Printf("Creating backup using default BSL")
			backupUid, _ := uuid.NewUUID()
			backupName := fmt.Sprintf("backup-bsl-cacert-%s", backupUid.String())
			err = lib.CreateBackupForNamespaces(dpaCR.Client, namespace, backupName, []string{"nginx-example"}, true, true)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Eventually(func() bool {
				result, _ := lib.IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
				return result
			}, time.Minute*10, time.Second*10).Should(gomega.BeTrue())

			log.Printf("Verifying backup was created with default BSL")
			completedBackup, err := lib.GetBackup(dpaCR.Client, namespace, backupName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			// Verify it used the default BSL
			gomega.Expect(completedBackup.Spec.StorageLocation).Should(gomega.Equal(defaultBSL.Name))

			log.Printf("Deleting application namespace")
			err = lib.DeleteNamespace(kubernetesClientForSuiteRun, "nginx-example")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, "nginx-example"), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			log.Printf("Creating restore from backup")
			restoreUid, _ := uuid.NewUUID()
			restoreName := fmt.Sprintf("restore-bsl-cacert-%s", restoreUid.String())
			err = lib.CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Eventually(func() bool {
				result, _ := lib.IsRestoreCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
				return result
			}, time.Minute*10, time.Second*10).Should(gomega.BeTrue())

			log.Printf("Verifying application was restored")
			gomega.Eventually(lib.IsDeploymentReady(dpaCR.Client, "nginx-example", "nginx-deployment"), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

			log.Printf("Test completed successfully - backupImages=%v test passed", backupImages)
		},
		ginkgo.Entry("three BSLs with backupImages=false (no CA cert handling)", false, false),
		ginkgo.Entry("three BSLs with backupImages=true (full CA cert handling with concatenation)", true, true),
	)
})

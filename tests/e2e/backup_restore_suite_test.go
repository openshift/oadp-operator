package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	volsync "github.com/backube/volsync/api/v1alpha1"
	"github.com/google/uuid"
	vsmv1alpha1 "github.com/konveyor/volume-snapshot-mover/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"github.com/openshift/oadp-operator/controllers"
	"github.com/openshift/oadp-operator/pkg/common"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachtypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VerificationFunction func(client.Client, string) error

type appVerificationFunction func(bool, bool, BackupRestoreType) VerificationFunction

func dataMoverReady(preBackupState, twoVol bool, appVerificationFunction appVerificationFunction) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, appNamespace string) error {
		// check volsync subscription exists
		Eventually(InstalledSubscriptionCSV(ocClient, "openshift-operators", "volsync-product"), timeoutMultiplier*time.Minute*10, time.Second*10).ShouldNot(Equal(""))
		// check volsync controller is ready
		fmt.Printf("waiting for volsync controller readiness")
		Eventually(IsDeploymentReady(ocClient, common.VolSyncDeploymentNamespace, common.VolSyncDeploymentName), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		Eventually(IsDeploymentReady(ocClient, namespace, common.DataMover), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		return appVerificationFunction(preBackupState, twoVol, CSIDataMover)(ocClient, appNamespace)
	})
}

func mongoready(preBackupState bool, twoVol bool, backupRestoreType BackupRestoreType) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		Eventually(IsDCReady(ocClient, namespace, "todolist"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		exists, err := DoesSCCExist(ocClient, "mongo-persistent-scc")
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("did not find Mongo scc")
		}
		err = VerifyBackupRestoreData(artifact_dir, namespace, "todolist-route", "todolist", preBackupState, false, backupRestoreType) // TODO: VERIFY PARKS APP DATA
		return err
	})
}
func mysqlReady(preBackupState bool, twoVol bool, backupRestoreType BackupRestoreType) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		fmt.Printf("checking for the NAMESPACE: %s\n ", namespace)
		// This test confirms that SCC restore logic in our plugin is working
		//Eventually(IsDCReady(ocClient, "mssql-persistent", "mysql"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		Eventually(IsDeploymentReady(ocClient, namespace, "mysql"), timeoutMultiplier*time.Minute*10, time.Second*10).Should(BeTrue())
		exists, err := DoesSCCExist(ocClient, "mysql-persistent-scc")
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("did not find MYSQL scc")
		}
		err = VerifyBackupRestoreData(artifact_dir, namespace, "todolist-route", "todolist", preBackupState, twoVol, backupRestoreType)
		return err
	})
}

var _ = Describe("AWS backup restore tests", func() {

	var _ = BeforeEach(func() {
		testSuiteInstanceName := "ts-" + instanceName
		dpaCR.Name = testSuiteInstanceName
	})

	type BackupRestoreCase struct {
		ApplicationTemplate  string
		ApplicationNamespace string
		Name                 string
		BackupRestoreType    BackupRestoreType
		PreBackupVerify      VerificationFunction
		PostRestoreVerify    VerificationFunction
		AppReadyDelay        time.Duration
		MaxK8SVersion        *K8sVersion
		MinK8SVersion        *K8sVersion
	}

	var lastBRCase BackupRestoreCase
	var lastInstallTime time.Time
	var _ = ReportAfterEach(func(report SpecReport) {
		if report.State == types.SpecStateSkipped || report.State == types.SpecStatePending {
			// do not run if the test is skipped
			return
		}
		GinkgoWriter.Println("Report after each: state: ", report.State.String())
		baseReportDir := artifact_dir + "/" + report.LeafNodeText
		err := os.MkdirAll(baseReportDir, 0755)
		Expect(err).NotTo(HaveOccurred())
		if report.Failed() {
			// print namespace error events for app namespace
			if lastBRCase.ApplicationNamespace != "" {
				GinkgoWriter.Println("Printing app namespace events")
				PrintNamespaceEventsAfterTime(lastBRCase.ApplicationNamespace, lastInstallTime)
			}
			GinkgoWriter.Println("Printing oadp namespace events")
			PrintNamespaceEventsAfterTime(namespace, lastInstallTime)
			if lastBRCase.BackupRestoreType == CSIDataMover {
				GinkgoWriter.Println("Printing volsync namespace events")
				PrintNamespaceEventsAfterTime(common.VolSyncDeploymentNamespace, lastInstallTime)

				pvcList := vsmv1alpha1.VolumeSnapshotBackupList{}
				err := dpaCR.Client.List(context.Background(), &pvcList, &client.ListOptions{Namespace: lastBRCase.ApplicationNamespace})
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("PVC app ns list %v\n", pvcList)
				err = dpaCR.Client.List(context.Background(), &pvcList, &client.ListOptions{Namespace: namespace})
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("PVC oadp ns list %v\n", pvcList)

				vsbList := vsmv1alpha1.VolumeSnapshotBackupList{}
				err = dpaCR.Client.List(context.Background(), &vsbList, &client.ListOptions{Namespace: lastBRCase.ApplicationNamespace})
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("VSB list %v\n", vsbList)

				vsrList := vsmv1alpha1.VolumeSnapshotRestoreList{}
				err = dpaCR.Client.List(context.Background(), &vsrList, &client.ListOptions{Namespace: namespace})
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("VSR list %v\n", vsrList)

				replicationSource := volsync.ReplicationSourceList{}
				err = dpaCR.Client.List(context.Background(), &replicationSource, &client.ListOptions{Namespace: namespace})
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("ReplicationSource list %v", replicationSource)

				replicationDestination := volsync.ReplicationDestinationList{}
				err = dpaCR.Client.List(context.Background(), &replicationDestination, &client.ListOptions{Namespace: namespace})
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("ReplicationDestination list %v", replicationDestination)

				volsyncIsReady, _ := IsDeploymentReady(dpaCR.Client, common.VolSyncDeploymentNamespace, common.VolSyncDeploymentName)()
				fmt.Printf("volsync controller is ready: %v", volsyncIsReady)

				vsmIsReady, _ := IsDeploymentReady(dpaCR.Client, namespace, common.DataMover)()
				fmt.Printf("volume-snapshot-mover is ready: %v", vsmIsReady)

				GinkgoWriter.Println("Printing volume-snapshot-mover deployment pod logs")
				GinkgoWriter.Print(GetDeploymentPodContainerLogs(namespace, common.DataMover, common.DataMoverControllerContainer))
			}
			err = SavePodLogs(namespace, baseReportDir)
			Expect(err).NotTo(HaveOccurred())
			err = SavePodLogs(lastBRCase.ApplicationNamespace, baseReportDir)
			Expect(err).NotTo(HaveOccurred())
		}

		err = dpaCR.Client.Delete(context.Background(), &corev1.Namespace{ObjectMeta: v1.ObjectMeta{
			Name:      lastBRCase.ApplicationNamespace,
			Namespace: lastBRCase.ApplicationNamespace,
		}}, &client.DeleteOptions{})
		if k8serror.IsNotFound(err) {
			err = nil
		}
		Expect(err).ToNot(HaveOccurred())
		// Additional cleanup for data mover case
		if lastBRCase.BackupRestoreType == CSIDataMover {
			// check for VSB and VSR objects and delete them
			vsbList := vsmv1alpha1.VolumeSnapshotBackupList{}
			err = dpaCR.Client.List(context.Background(), &vsbList, &client.ListOptions{Namespace: lastBRCase.ApplicationNamespace})
			Expect(err).NotTo(HaveOccurred())
			for _, vsb := range vsbList.Items {
				// patch to remove finalizer from vsb to allow deletion
				patch := client.RawPatch(apimachtypes.JSONPatchType, []byte(`[{"op": "remove", "path": "/metadata/finalizers"}]`))
				err = dpaCR.Client.Patch(context.Background(), &vsb, patch)
				Expect(err).NotTo(HaveOccurred())
				err = dpaCR.Client.Delete(context.Background(), &vsb, &client.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
			}
			vsrList := vsmv1alpha1.VolumeSnapshotRestoreList{}
			err = dpaCR.Client.List(context.Background(), &vsrList, &client.ListOptions{Namespace: lastBRCase.ApplicationNamespace})
			Expect(err).NotTo(HaveOccurred())
			for _, vsr := range vsrList.Items {
				// patch to remove finalizer from vsr to allow deletion
				patch := client.RawPatch(apimachtypes.JSONPatchType, []byte(`[{"op": "remove", "path": "/metadata/finalizers"}]`))
				err = dpaCR.Client.Patch(context.Background(), &vsr, patch)
				Expect(err).NotTo(HaveOccurred())
				err = dpaCR.Client.Delete(context.Background(), &vsr, &client.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
			}
		}
		err = dpaCR.Delete()
		Expect(err).ToNot(HaveOccurred())
		Eventually(IsNamespaceDeleted(lastBRCase.ApplicationNamespace), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())
	})

	updateLastInstallTime := func() {
		lastInstallTime = time.Now()
	}

	DescribeTable("backup and restore applications",
		func(brCase BackupRestoreCase, expectedErr error) {
			// Data Mover is only supported on aws, azure, and gcp.
			if brCase.BackupRestoreType == CSIDataMover && provider != "aws" && provider != "azure" && provider != "gcp" {
				Skip(provider + " unsupported data mover provider")
			}
			if provider == "azure" && (brCase.BackupRestoreType == CSI || brCase.BackupRestoreType == CSIDataMover) {
				if brCase.MinK8SVersion == nil {
					brCase.MinK8SVersion = &K8sVersion{Major: "1", Minor: "23"}
				}
			}
			if notVersionTarget, reason := NotServerVersionTarget(brCase.MinK8SVersion, brCase.MaxK8SVersion); notVersionTarget {
				Skip(reason)
			}

			lastBRCase = brCase

			err := dpaCR.Build(brCase.BackupRestoreType)
			Expect(err).NotTo(HaveOccurred())

			//updateLastInstallingNamespace(dpaCR.Namespace)
			updateLastInstallTime()

			err = dpaCR.CreateOrUpdate(&dpaCR.CustomResource.Spec)
			Expect(err).NotTo(HaveOccurred())

			fmt.Printf("Cluster type: %s \n", provider)

			log.Printf("Waiting for velero pod to be running")
			Eventually(AreVeleroPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())

			if brCase.BackupRestoreType == RESTIC {
				log.Printf("Waiting for restic pods to be running")
				Eventually(AreNodeAgentPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			if brCase.BackupRestoreType == CSI || brCase.BackupRestoreType == CSIDataMover {
				if provider == "aws" || provider == "ibmcloud" || provider == "gcp" || provider == "azure" {
					log.Printf("Creating VolumeSnapshotClass for CSI backuprestore of %s", brCase.Name)
					snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", provider)
					err = InstallApplication(dpaCR.Client, snapshotClassPath)
					Expect(err).ToNot(HaveOccurred())
				}
				if brCase.BackupRestoreType == CSIDataMover {
					dpaCR.Client.Create(context.Background(), &corev1.Secret{
						ObjectMeta: v1.ObjectMeta{
							Name:      controllers.ResticsecretName,
							Namespace: dpaCR.Namespace,
						},
						StringData: map[string]string{
							controllers.ResticPassword: "e2e-restic-password",
						},
					}, &client.CreateOptions{})
				}
			}

			// TODO: check registry deployments are deleted
			// TODO: check S3 for images

			backupUid, _ := uuid.NewUUID()
			restoreUid, _ := uuid.NewUUID()
			backupName := fmt.Sprintf("%s-%s", brCase.Name, backupUid.String())
			restoreName := fmt.Sprintf("%s-%s", brCase.Name, restoreUid.String())

			// install app
			updateLastInstallTime()
			log.Printf("Installing application for case %s", brCase.Name)
			err = InstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())
			if brCase.BackupRestoreType == CSI || brCase.BackupRestoreType == CSIDataMover {
				log.Printf("Creating pvc for case %s", brCase.Name)
				var pvcPath string
				if strings.Contains(brCase.Name, "twovol") {
					pvcPath = fmt.Sprintf("./sample-applications/%s/pvc-twoVol/%s.yaml", brCase.ApplicationNamespace, provider)
				} else {
					pvcPath = fmt.Sprintf("./sample-applications/%s/pvc/%s.yaml", brCase.ApplicationNamespace, provider)
				}
				err = InstallApplication(dpaCR.Client, pvcPath)
				Expect(err).ToNot(HaveOccurred())
			}

			// wait for pods to be running
			Eventually(AreAppBuildsReady(dpaCR.Client, brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*5, time.Second*5).Should(BeTrue())
			Eventually(AreApplicationPodsRunning(brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*9, time.Second*5).Should(BeTrue())

			// Run optional custom verification
			log.Printf("Running pre-backup function for case %s", brCase.Name)
			err = brCase.PreBackupVerify(dpaCR.Client, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())

			nsRequiresResticDCWorkaround, err := NamespaceRequiresResticDCWorkaround(dpaCR.Client, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())

			log.Printf("Sleeping for %v to allow application to be ready for case %s", brCase.AppReadyDelay, brCase.Name)
			time.Sleep(brCase.AppReadyDelay)
			// create backup
			log.Printf("Creating backup %s for case %s", backupName, brCase.Name)
			backup, err := CreateBackupForNamespaces(dpaCR.Client, namespace, backupName, []string{brCase.ApplicationNamespace}, brCase.BackupRestoreType == RESTIC)
			Expect(err).ToNot(HaveOccurred())

			// wait for backup to not be running
			Eventually(IsBackupDone(dpaCR.Client, namespace, backupName), timeoutMultiplier*time.Minute*20, time.Second*10).Should(BeTrue())
			GinkgoWriter.Println(DescribeBackup(dpaCR.Client, backup))
			Expect(BackupErrorLogs(dpaCR.Client, backup)).To(Equal([]string{}))

			// check if backup succeeded
			succeeded, err := IsBackupCompletedSuccessfully(dpaCR.Client, backup)
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))
			log.Printf("Backup for case %s succeeded", brCase.Name)

			if brCase.BackupRestoreType == CSI {
				// wait for volume snapshot to be Ready
				Eventually(AreVolumeSnapshotsReady(dpaCR.Client, backupName), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
			}
			// if Data Mover case, wait for VSB to be gone from app namespace
			if brCase.BackupRestoreType == CSIDataMover {
				Eventually(ThereAreNoVolumeSnapshotBackups(dpaCR.Client, brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*4, time.Second*10).Should(BeTrue())
			}
			// uninstall app
			log.Printf("Uninstalling app for case %s", brCase.Name)
			err = UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())

			// Wait for namespace to be deleted
			Eventually(IsNamespaceDeleted(brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*4, time.Second*5).Should(BeTrue())

			updateLastInstallTime()
			// run restore
			log.Printf("Creating restore %s for case %s", restoreName, brCase.Name)
			restore, err := CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
			Expect(err).ToNot(HaveOccurred())
			Eventually(IsRestoreDone(dpaCR.Client, namespace, restoreName), timeoutMultiplier*time.Minute*60, time.Second*10).Should(BeTrue())
			GinkgoWriter.Println(DescribeRestore(dpaCR.Client, restore))
			Expect(RestoreErrorLogs(dpaCR.Client, restore)).To(Equal([]string{}))

			// Check if restore succeeded
			succeeded, err = IsRestoreCompletedSuccessfully(dpaCR.Client, namespace, restoreName)
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))

			if brCase.BackupRestoreType == RESTIC && nsRequiresResticDCWorkaround {
				// run the restic post restore script if restore type is RESTIC
				log.Printf("Running restic post restore script for case %s", brCase.Name)
				err = RunResticPostRestoreScript(restoreName)
				Expect(err).ToNot(HaveOccurred())
			}

			// verify app is running
			Eventually(AreAppBuildsReady(dpaCR.Client, brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			Eventually(AreApplicationPodsRunning(brCase.ApplicationNamespace), timeoutMultiplier*time.Minute*9, time.Second*5).Should(BeTrue())

			// Run optional custom verification
			log.Printf("Running post-restore function for case %s", brCase.Name)
			err = brCase.PostRestoreVerify(dpaCR.Client, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())

			if brCase.BackupRestoreType == CSI || brCase.BackupRestoreType == CSIDataMover {
				log.Printf("Deleting VolumeSnapshot for CSI backuprestore of %s", brCase.Name)
				snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", provider)
				err = UninstallApplication(dpaCR.Client, snapshotClassPath)
				Expect(err).ToNot(HaveOccurred())
			}

		},
		Entry("MySQL application CSI", Label("ibmcloud", "aws", "gcp", "azure"), BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mysql-persistent/mysql-persistent-csi.yaml",
			ApplicationNamespace: "mysql-persistent",
			Name:                 "mysql-csi-e2e",
			BackupRestoreType:    CSI,
			PreBackupVerify:      mysqlReady(true, false, CSI),
			PostRestoreVerify:    mysqlReady(false, false, CSI),
		}, nil),
		Entry("Mongo application CSI", Label("ibmcloud", "aws", "gcp", "azure"), BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			ApplicationNamespace: "mongo-persistent",
			Name:                 "mongo-csi-e2e",
			BackupRestoreType:    CSI,
			PreBackupVerify:      mongoready(true, false, CSI),
			PostRestoreVerify:    mongoready(false, false, CSI),
		}, nil),
		Entry("MySQL application two Vol CSI", Label("ibmcloud", "aws", "gcp", "azure"), BackupRestoreCase{
			ApplicationTemplate:  fmt.Sprintf("./sample-applications/mysql-persistent/mysql-persistent-twovol-csi.yaml"),
			ApplicationNamespace: "mysql-persistent",
			Name:                 "mysql-twovol-csi-e2e",
			BackupRestoreType:    CSI,
			AppReadyDelay:        30 * time.Second,
			PreBackupVerify:      mysqlReady(true, true, CSI),
			PostRestoreVerify:    mysqlReady(false, true, CSI),
		}, nil),
		Entry("Mongo application RESTIC", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			ApplicationNamespace: "mongo-persistent",
			Name:                 "mongo-restic-e2e",
			BackupRestoreType:    RESTIC,
			PreBackupVerify:      mongoready(true, false, RESTIC),
			PostRestoreVerify:    mongoready(false, false, RESTIC),
		}, nil),
		Entry("MySQL application RESTIC", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mysql-persistent/mysql-persistent.yaml",
			ApplicationNamespace: "mysql-persistent",
			Name:                 "mysql-restic-e2e",
			BackupRestoreType:    RESTIC,
			PreBackupVerify:      mysqlReady(true, false, RESTIC),
			PostRestoreVerify:    mysqlReady(false, false, RESTIC),
		}, nil),
		PEntry("Mongo application DATAMOVER", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mongo-persistent/mongo-persistent-csi.yaml",
			ApplicationNamespace: "mongo-persistent",
			Name:                 "mongo-datamover-e2e",
			BackupRestoreType:    CSIDataMover,
			PreBackupVerify:      dataMoverReady(true, false, mongoready),
			PostRestoreVerify:    dataMoverReady(false, false, mongoready),
		}, nil),
		// TODO: Re-implement this test to upstream data mover
		PEntry("MySQL application DATAMOVER", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mysql-persistent/mysql-persistent-csi.yaml",
			ApplicationNamespace: "mysql-persistent",
			Name:                 "mysql-datamover-e2e",
			BackupRestoreType:    CSIDataMover,
			PreBackupVerify:      dataMoverReady(true, false, mysqlReady),
			PostRestoreVerify:    dataMoverReady(false, false, mysqlReady),
		}, nil),
	)
})

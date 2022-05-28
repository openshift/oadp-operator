package e2e_test

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/oadp-operator/api/v1alpha1"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	i "github.com/openshift/oadp-operator/tests/e2e/lib/init"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VerificationFunction func(*DpaCustomResource, string) error

var _ = Describe("AWS backup restore tests", func() {

	var _ = BeforeEach(func() {})

	var _ = AfterEach(func() {
		GinkgoWriter.Println("Grabbing velero failure logs before dpa deletion")
		GinkgoWriter.Println(GetVeleroContainerFailureLogsAsString(i.GetNamespace()))
		err := dpaCR.Delete()
		Expect(err).ToNot(HaveOccurred())

	})
	var lastInstallingApplicationNamespace string
	var lastInstallTime time.Time
	var _ = ReportAfterEach(func(report SpecReport) {
		if report.Failed() {
			// print namespace error events for app namespace
			if lastInstallingApplicationNamespace != "" {
				PrintNamespaceEventsAfterTime(lastInstallingApplicationNamespace, lastInstallTime)
			}
		}
	})

	type BackupRestoreCase struct {
		ApplicationTemplate  string
		ApplicationNamespace string
		Name                 string
		BackupRestoreType    BackupRestoreType
		PreBackupVerify      VerificationFunction
		PostRestoreVerify    VerificationFunction
		MaxK8SVersion        *K8sVersion
		MinK8SVersion        *K8sVersion
		dpaCrOpts            []DpaCROption
		backupOpts           []BackupOpts
	}

	mongoReady := VerificationFunction(func(dpaCR *DpaCustomResource, namespace string) error {
		Eventually(IsDCReady(dpaCR.Client, "mongo-persistent", "todolist"), i.GetTimeoutMultiplier()*time.Minute*10, time.Second*10).Should(BeTrue())
		// err := VerifyBackupRestoreData(i.GetArtifact_Dir(), namespace, "restify", "parks-app") // TODO: VERIFY PARKS APP DATA
		return nil
	})
	mysqlReady := VerificationFunction(func(dpaCR *DpaCustomResource, namespace string) error {
		// This test confirms that SCC restore logic in our plugin is working
		//Eventually(IsDCReady(ocClient, "mssql-persistent", "mysql"), i.GetTimeoutMultiplier()*time.Minute*10, time.Second*10).Should(BeTrue())
		Eventually(IsDeploymentReady(dpaCR.Client, "mysql-persistent", "mysql"), i.GetTimeoutMultiplier()*time.Minute*10, time.Second*10).Should(BeTrue())
		exists, err := DoesSCCExist(dpaCR.Client, "mysql-persistent-scc")
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("did not find MYSQL scc")
		}
		err = VerifyBackupRestoreData(i.GetArtifact_Dir(), namespace, "todolist-route", "todolist")
		return err
	})

	updateLastInstallingNamespace := func(namespace string) {
		lastInstallingApplicationNamespace = namespace
		lastInstallTime = time.Now()
	}

	DescribeTable("backup and restore applications",
		func(brCase BackupRestoreCase, expectedErr error) {
			if notVersionTarget, reason := NotServerVersionTarget(brCase.MinK8SVersion, brCase.MaxK8SVersion); notVersionTarget {
				Skip(reason)
			}

			err := dpaCR.Build(brCase.BackupRestoreType, brCase.dpaCrOpts...)
			Expect(err).NotTo(HaveOccurred())

			updateLastInstallingNamespace(i.GetNamespace())
			err = dpaCR.CreateOrUpdate(&dpaCR.CustomResource.Spec)
			Expect(err).NotTo(HaveOccurred())
			Eventually(dpaCR.DPAReconcileError(), i.GetTimeoutMultiplier()*time.Minute*2, time.Second*5).Should(BeFalse())
			log.Printf("Waiting for velero pod to be running")
			Eventually(AreVeleroDeploymentReplicasReady(i.GetNamespace()), i.GetTimeoutMultiplier() *time.Minute*3, time.Second*5).Should(BeTrue())
			for n, bl := range dpaCR.CustomResource.Spec.BackupLocations {
				if bl.Velero != nil {
					Eventually(BackupStorageLocationIsAvailable(dpaCR.Client, fmt.Sprintf("%s-%d", i.GetTestSuiteInstanceName(), n+1), i.GetNamespace()), i.GetTimeoutMultiplier()*time.Minute*2, time.Second*5).Should(BeTrue())
				}
			}
			if brCase.BackupRestoreType == RESTIC {
				log.Printf("Waiting for restic pods to be running")
				Eventually(AreResticDaemonsetUpdatedAndReady(i.GetNamespace()), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			if brCase.BackupRestoreType == CSI {
				if i.GetProvider() == "aws" || i.GetProvider() == "ibmcloud" {
					log.Printf("Creating VolumeSnapshotClass for CSI backuprestore of %s", brCase.Name)
					snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", i.GetProvider())
					err = InstallApplication(dpaCR.Client, snapshotClassPath)
					Expect(err).ToNot(HaveOccurred())
				}
			}

			if dpaCR.CustomResource.BackupImages() {
				log.Printf("Waiting for registry pods to be running")
				Eventually(AreRegistryDeploymentsAvailable(i.GetNamespace()), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			backupUid, _ := uuid.NewUUID()
			restoreUid, _ := uuid.NewUUID()
			backupName := fmt.Sprintf("%s-%s", brCase.Name, backupUid.String())
			restoreName := fmt.Sprintf("%s-%s", brCase.Name, restoreUid.String())

			// install app
			updateLastInstallingNamespace(brCase.ApplicationNamespace)
			log.Printf("Installing application for case %s", brCase.Name)
			err = InstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())
			// wait for pods to be running
			Eventually(AreAppBuildsReady(dpaCR.Client, brCase.ApplicationNamespace), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			Eventually(AreApplicationPodsRunning(brCase.ApplicationNamespace), i.GetTimeoutMultiplier()*time.Minute*9, time.Second*5).Should(BeTrue())

			// Run optional custom verification
			log.Printf("Running pre-backup function for case %s", brCase.Name)
			err = brCase.PreBackupVerify(dpaCR, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())

			nsRequiresResticDCWorkaround, err := NamespaceRequiresResticDCWorkaround(dpaCR.Client, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())
			// create backup
			log.Printf("Creating backup %s for case %s", backupName, brCase.Name)
			backup, err := CreateBackupForNamespaces(dpaCR.Client, i.GetNamespace(), backupName, []string{brCase.ApplicationNamespace}, brCase.backupOpts...)
			Expect(err).ToNot(HaveOccurred())

			// wait for backup to not be running
			Eventually(IsBackupDone(dpaCR.Client, i.GetNamespace(), backupName), i.GetTimeoutMultiplier()*time.Minute*4, time.Second*10).Should(BeTrue())
			GinkgoWriter.Println(DescribeBackup(dpaCR.Client, backup))
			Expect(BackupErrorLogs(dpaCR.Client, backup)).To(Equal([]string{}))

			// check if backup succeeded
			succeeded, err := IsBackupCompletedSuccessfully(dpaCR.Client, backup)
			Expect(err).ToNot(HaveOccurred())
			Expect(succeeded).To(Equal(true))
			log.Printf("Backup for case %s succeeded", brCase.Name)

			if brCase.BackupRestoreType == CSI {
				// wait for volume snapshot to be Ready
				Eventually(AreVolumeSnapshotsReady(dpaCR.Client, backupName), i.GetTimeoutMultiplier()*time.Minute*4, time.Second*10).Should(BeTrue())
			}

			// uninstall app
			log.Printf("Uninstalling app for case %s", brCase.Name)
			err = UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())

			// Wait for namespace to be deleted
			Eventually(IsNamespaceDeleted(brCase.ApplicationNamespace), i.GetTimeoutMultiplier()*time.Minute*2, time.Second*5).Should(BeTrue())

			updateLastInstallingNamespace(brCase.ApplicationNamespace)
			// Check if backup needs restic deploymentconfig workaround. https://github.com/openshift/oadp-operator/blob/master/docs/TROUBLESHOOTING.md#deployconfig
			if brCase.BackupRestoreType == RESTIC && nsRequiresResticDCWorkaround {
				log.Printf("DC found in backup i.GetNamespace(), using DC restic workaround")
				var dcWorkaroundResources = []string{"replicationcontroller", "deploymentconfig", "templateinstances.template.openshift.io"}
				// run restore
				log.Printf("Creating restore %s excluding DC workaround resources for case %s", restoreName, brCase.Name)
				noDcDrestoreName := fmt.Sprintf("%s-no-dc-workaround", restoreName)
				restore, err := CreateRestoreFromBackup(dpaCR.Client, i.GetNamespace(), backupName, noDcDrestoreName, WithExcludedResources(dcWorkaroundResources))
				Expect(err).ToNot(HaveOccurred())
				Eventually(IsRestoreDone(dpaCR.Client, i.GetNamespace(), noDcDrestoreName), i.GetTimeoutMultiplier()*time.Minute*4, time.Second*10).Should(BeTrue())
				GinkgoWriter.Println(DescribeRestore(dpaCR.Client, restore))
				Expect(RestoreErrorLogs(dpaCR.Client, restore)).To(Equal([]string{}))

				// Check if restore succeeded
				succeeded, err = IsRestoreCompletedSuccessfully(dpaCR.Client, i.GetNamespace(), noDcDrestoreName)
				Expect(err).ToNot(HaveOccurred())
				Expect(succeeded).To(Equal(true))
				Eventually(AreAppBuildsReady(dpaCR.Client, brCase.ApplicationNamespace), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())

				// run restore
				log.Printf("Creating restore %s including DC workaround resources for case %s", restoreName, brCase.Name)
				withDcRestoreName := fmt.Sprintf("%s-with-dc-workaround", restoreName)
				restore, err = CreateRestoreFromBackup(dpaCR.Client, i.GetNamespace(), backupName, withDcRestoreName, WithIncludedResources(dcWorkaroundResources))
				Expect(err).ToNot(HaveOccurred())
				Eventually(IsRestoreDone(dpaCR.Client, i.GetNamespace(), withDcRestoreName), i.GetTimeoutMultiplier()*time.Minute*4, time.Second*10).Should(BeTrue())
				GinkgoWriter.Println(DescribeRestore(dpaCR.Client, restore))
				Expect(RestoreErrorLogs(dpaCR.Client, restore)).To(Equal([]string{}))

				// Check if restore succeeded
				succeeded, err = IsRestoreCompletedSuccessfully(dpaCR.Client, i.GetNamespace(), withDcRestoreName)
				Expect(err).ToNot(HaveOccurred())
				Expect(succeeded).To(Equal(true))

			} else {
				// run restore
				log.Printf("Creating restore %s for case %s", restoreName, brCase.Name)
				restore, err := CreateRestoreFromBackup(dpaCR.Client, i.GetNamespace(), backupName, restoreName)
				Expect(err).ToNot(HaveOccurred())
				Eventually(IsRestoreDone(dpaCR.Client, i.GetNamespace(), restoreName), i.GetTimeoutMultiplier()*time.Minute*4, time.Second*10).Should(BeTrue())
				GinkgoWriter.Println(DescribeRestore(dpaCR.Client, restore))
				Expect(RestoreErrorLogs(dpaCR.Client, restore)).To(Equal([]string{}))

				// Check if restore succeeded
				succeeded, err = IsRestoreCompletedSuccessfully(dpaCR.Client, i.GetNamespace(), restoreName)
				Expect(err).ToNot(HaveOccurred())
				Expect(succeeded).To(Equal(true))
			}

			// verify app is running
			Eventually(AreAppBuildsReady(dpaCR.Client, brCase.ApplicationNamespace), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			Eventually(AreApplicationPodsRunning(brCase.ApplicationNamespace), i.GetTimeoutMultiplier()*time.Minute*9, time.Second*5).Should(BeTrue())

			// Run optional custom verification
			log.Printf("Running post-restore function for case %s", brCase.Name)
			err = brCase.PostRestoreVerify(dpaCR, brCase.ApplicationNamespace)
			Expect(err).ToNot(HaveOccurred())

			// Test is successful, clean up everything
			log.Printf("Uninstalling application for case %s", brCase.Name)
			err = UninstallApplication(dpaCR.Client, brCase.ApplicationTemplate)
			Expect(err).ToNot(HaveOccurred())

			// Wait for namespace to be deleted
			Eventually(IsNamespaceDeleted(brCase.ApplicationNamespace), i.GetTimeoutMultiplier()*time.Minute*2, time.Second*5).Should(BeTrue())

			if brCase.BackupRestoreType == CSI {
				log.Printf("Deleting VolumeSnapshot for CSI backuprestore of %s", brCase.Name)
				snapshotClassPath := fmt.Sprintf("./sample-applications/snapclass-csi/%s.yaml", i.GetProvider())
				err = UninstallApplication(dpaCR.Client, snapshotClassPath)
				Expect(err).ToNot(HaveOccurred())
			}

		},
		Entry("MySQL application CSI", Label("ibmcloud", "aws"), BackupRestoreCase{
			ApplicationTemplate:  fmt.Sprintf("./sample-applications/mysql-persistent/mysql-persistent-csi-%s-template.yaml", i.GetProvider()),
			ApplicationNamespace: "mysql-persistent",
			Name:                 "mysql-e2e",
			BackupRestoreType:    CSI,
			PreBackupVerify:      mysqlReady,
			PostRestoreVerify:    mysqlReady,
		}, nil),
		Entry("Mongo application", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mongo-persistent/mongo-persistent.yaml",
			ApplicationNamespace: "mongo-persistent",
			Name:                 "mongo-e2e",
			BackupRestoreType:    RESTIC,
			PreBackupVerify:      mongoReady,
			PostRestoreVerify:    mongoReady,
		}, nil),
		Entry("MySQL application", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mysql-persistent/mysql-persistent-template.yaml",
			ApplicationNamespace: "mysql-persistent",
			Name:                 "mysql-e2e",
			BackupRestoreType:    RESTIC,
			PreBackupVerify:      mysqlReady,
			PostRestoreVerify:    mysqlReady,
		}, nil),
		Entry("MySQL application NoDefaultBackupStorageLocation", BackupRestoreCase{
			ApplicationTemplate:  "./sample-applications/mysql-persistent/mysql-persistent-template.yaml",
			ApplicationNamespace: "mysql-persistent",
			Name:                 "mysql-e2e",
			BackupRestoreType:    RESTIC,
			PreBackupVerify: VerificationFunction(func(dpaCR *DpaCustomResource, namespace string) error {
				bsl := velerov1.BackupStorageLocation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      i.GetTestSuiteInstanceName() + "nobsl-1",
						Namespace: i.GetNamespace(),
					},
					Spec: *VeleroBSL(),
				}
				// create BSL
				err := CreateBackupStorageLocation(bsl)
				if err != nil {
					return err
				}
				// wait for BSL to be available
				Eventually(BackupStorageLocationIsAvailable(dpaCR.Client, dpaCR.Name+"nobsl-1", i.GetNamespace()), i.GetTimeoutMultiplier()*time.Minute*2, time.Second*5).Should(BeTrue())
				return mysqlReady(dpaCR, namespace)
			}),
			PostRestoreVerify: VerificationFunction(func(dpaCR *DpaCustomResource, namespace string) error {
				// delete BSL
				err := DeleteBackupStorageLocation(dpaCR.Client, velerov1.BackupStorageLocation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      i.GetTestSuiteInstanceName() + "nobsl-1",
						Namespace: i.GetNamespace(),
					}})
				if err != nil {
					return err
				}
				return mysqlReady(dpaCR, namespace)
			}),
			dpaCrOpts: []DpaCROption{
				WithVeleroConfig(&v1alpha1.VeleroConfig{
					FeatureFlags:                    GetDpa().Spec.Configuration.Velero.FeatureFlags,
					DefaultPlugins:                  GetDpa().Spec.Configuration.Velero.DefaultPlugins,
					CustomPlugins:                   GetDpa().Spec.Configuration.Velero.CustomPlugins,
					PodConfig:                       GetDpa().Spec.Configuration.Velero.PodConfig,
					RestoreResourcesVersionPriority: GetDpa().Spec.Configuration.Velero.RestoreResourcesVersionPriority,
					LogLevel:                        GetDpa().Spec.Configuration.Velero.LogLevel,
					NoDefaultBackupLocation:         true, // the only difference from the default velero config
				}),
				WithBackupImages(false),
				WithBackupLocations([]v1alpha1.BackupLocation{}),     //empty backuplocations
				WithSnapshotLocations([]v1alpha1.SnapshotLocation{}), //empty snapshotlocations
			},
			backupOpts: []BackupOpts{WithBackupStorageLocation(i.GetTestSuiteInstanceName() + "nobsl-1")}, // e2e_sute_test.go: dpaCR.name = "ts-" + instanceName
		}, nil),
	)
})

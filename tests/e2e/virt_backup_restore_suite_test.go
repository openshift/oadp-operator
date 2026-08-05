package e2e_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

// TODO duplication of todoListReady in tests/e2e/backup_restore_suite_test.go
func vmTodoListReady(preBackupState bool, twoVol bool, database string) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		log.Printf("checking for the NAMESPACE: %s", namespace)
		gomega.Eventually(lib.IsDeploymentReady(ocClient, namespace, database), time.Minute*10, time.Second*10).Should(gomega.BeTrue())
		// in VM tests, DeploymentConfig was refactored to Deployment (to avoid deprecation warnings)
		// gomega.Eventually(lib.IsDCReady(ocClient, namespace, "todolist"), time.Minute*10, time.Second*10).Should(gomega.BeTrue())
		gomega.Eventually(lib.IsDeploymentReady(ocClient, namespace, "todolist"), time.Minute*10, time.Second*10).Should(gomega.BeTrue())
		gomega.Eventually(lib.AreApplicationPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*9, time.Second*5).Should(gomega.BeTrue())
		// This test confirms that SCC restore logic in our plugin is working
		err := lib.DoesSCCExist(ocClient, database+"-persistent-scc")
		if err != nil {
			return err
		}
		err = lib.VerifyBackupRestoreData(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, kubeConfig, artifact_dir, namespace, "todolist-route", "todolist", "todolist", preBackupState, twoVol, true)
		return err
	})
}

func getLatestCirrosImageURL() (string, error) {
	cirrosVersionURL := "https://download.cirros-cloud.net/version/released"

	resp, err := http.Get(cirrosVersionURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	latestCirrosVersion := strings.TrimSpace(string(body))

	imageURL := fmt.Sprintf("https://download.cirros-cloud.net/%s/cirros-%s-x86_64-disk.img", latestCirrosVersion, latestCirrosVersion)

	return imageURL, nil
}

func vmPoweredOff(vmnamespace, vmname string) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		isOff := func() bool {
			status, err := lib.GetVmStatus(dynamicClientForSuiteRun, vmnamespace, vmname)
			if err != nil {
				log.Printf("Error getting VM status: %v", err)
			}
			log.Printf("VM status is: %s\n", status)
			return status == "Stopped"
		}
		gomega.Eventually(isOff, time.Minute*10, time.Second*10).Should(gomega.BeTrue())
		return nil
	})
}

// vmPvcsBound verifies that each named PVC exists and is Bound in the given
// namespace. Used to confirm all disks of a multi-PVC VM came back after restore.
func vmPvcsBound(pvcNamespace string, pvcNames ...string) VerificationFunction {
	return VerificationFunction(func(ocClient client.Client, namespace string) error {
		allBound := func() bool {
			for _, name := range pvcNames {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				pvc, err := kubernetesClientForSuiteRun.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, name, metav1.GetOptions{})
				cancel()
				if err != nil {
					log.Printf("Error getting PVC %s/%s: %v", pvcNamespace, name, err)
					return false
				}
				if pvc.Status.Phase != corev1.ClaimBound {
					log.Printf("PVC %s/%s is %s, not yet Bound", pvcNamespace, name, pvc.Status.Phase)
					return false
				}
			}
			return true
		}
		gomega.Eventually(allBound, time.Minute*10, time.Second*10).Should(gomega.BeTrue(),
			"expected PVCs %v in namespace %s to be Bound", pvcNames, pvcNamespace)
		return nil
	})
}

type VmBackupRestoreCase struct {
	BackupRestoreCase
	Template   string
	InitDelay  time.Duration
	PowerState string
}

func runVmBackupAndRestore(brCase VmBackupRestoreCase, updateLastBRcase func(brCase VmBackupRestoreCase), v *lib.VirtOperator) {
	updateLastBRcase(brCase)

	// Create DPA
	backupName, restoreName := prepareBackupAndRestore(brCase.BackupRestoreCase, func() {})

	// Ensure a clean namespace before each spec. The previous spec's restore
	// leaves the namespace populated with a VM/PVC that may use a stale storage
	// class. Deleting it first guarantees the template creates a fresh DV.
	_ = v.RemoveVm(brCase.Namespace, brCase.Name, 2*time.Minute)
	err := lib.DeleteNamespace(v.Clientset, brCase.Namespace)
	gomega.Expect(err).To(gomega.BeNil())
	gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), time.Minute*2, time.Second*5).Should(gomega.BeTrue())

	err = lib.CreateNamespace(v.Clientset, brCase.Namespace)
	gomega.Expect(err).To(gomega.BeNil())

	err = lib.InstallApplication(v.Client, brCase.Template)
	if err != nil {
		fmt.Printf("Failed to install VM template %s: %v", brCase.Template, err)
	}
	gomega.Expect(err).To(gomega.BeNil())

	// Wait for VM to start, then give some time for cloud-init to run.
	// Afterward, run through the standard application verification to make sure
	// the application itself is working correctly.
	err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		status, err := v.GetVmStatus(brCase.Namespace, brCase.Name)
		return status == "Running", err
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	err = v.WaitForVMReady(brCase.Namespace, brCase.Name, 5*time.Minute)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	if brCase.InitDelay > 0 {
		log.Printf("Waiting %v for VM %s/%s to finish booting (cloud-init, etc.)", brCase.InitDelay, brCase.Namespace, brCase.Name)
		time.Sleep(brCase.InitDelay)
	}

	// Check if this VM should be running or stopped for this test.
	// Depend on pre-backup verification function to poll state.
	if brCase.PowerState == "Stopped" {
		log.Print("Stopping VM before backup as specified in test case.")
		err = v.StopVm(brCase.Namespace, brCase.Name)
		gomega.Expect(err).To(gomega.BeNil())
	}

	// Run optional custom verification
	if brCase.PreBackupVerify != nil {
		log.Printf("Running pre-backup custom function for case %s", brCase.Name)
		err := brCase.PreBackupVerify(dpaCR.Client, brCase.Namespace)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}

	// Back up VM
	nsRequiresResticDCWorkaround := runBackup(brCase.BackupRestoreCase, backupName)

	// Delete everything in test namespace
	err = v.RemoveVm(brCase.Namespace, brCase.Name, 5*time.Minute)
	gomega.Expect(err).To(gomega.BeNil())
	err = lib.DeleteNamespace(v.Clientset, brCase.Namespace)
	gomega.Expect(err).To(gomega.BeNil())
	gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), time.Minute*5, time.Second*5).Should(gomega.BeTrue())

	// Do restore
	runRestore(brCase.BackupRestoreCase, backupName, restoreName, nsRequiresResticDCWorkaround)

	// Run optional custom verification
	if brCase.PostRestoreVerify != nil {
		log.Printf("Running post-restore custom function for VM case %s", brCase.Name)
		err = brCase.PostRestoreVerify(dpaCR.Client, brCase.Namespace)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}

	// avoid finalizers in namespace deletion
	err = v.RemoveVm(brCase.Namespace, brCase.Name, 5*time.Minute)
	gomega.Expect(err).To(gomega.BeNil())
}

func runCBTVmBackup(brCase VmBackupRestoreCase, updateLastBRcase func(brCase VmBackupRestoreCase), v *lib.VirtOperator) {
	updateLastBRcase(brCase)

	backupName, _ := prepareBackupAndRestore(brCase.BackupRestoreCase, func() {})

	gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), time.Minute*2, time.Second*5).Should(gomega.BeTrue())
	err := lib.CreateNamespace(v.Clientset, brCase.Namespace)
	gomega.Expect(err).To(gomega.BeNil())

	err = lib.InstallApplication(v.Client, brCase.Template)
	if err != nil {
		fmt.Printf("Failed to install VM template %s: %v", brCase.Template, err)
	}
	gomega.Expect(err).To(gomega.BeNil())

	log.Printf("Waiting for VM %s/%s to reach Running status", brCase.Namespace, brCase.Name)
	err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 15*time.Minute, true, func(ctx context.Context) (bool, error) {
		status, err := v.GetVmStatus(brCase.Namespace, brCase.Name)
		if err != nil {
			log.Printf("VM %s/%s not yet available: %v", brCase.Namespace, brCase.Name, err)
			return false, nil
		}
		return status == "Running", nil
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	err = v.WaitForVMReady(brCase.Namespace, brCase.Name, 5*time.Minute)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	if brCase.InitDelay > 0 {
		log.Printf("Waiting %v for VM %s/%s to finish booting (cloud-init, etc.)", brCase.InitDelay, brCase.Namespace, brCase.Name)
		time.Sleep(brCase.InitDelay)
	}

	log.Printf("VM %s/%s is fully booted, CBT enabled, proceeding with backup", brCase.Namespace, brCase.Name)

	log.Printf("Creating kubevirt volume policy ConfigMap for custom action routing")
	err = lib.EnsureKubevirtVolumePolicy(dpaCR.Client, namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	log.Printf("Creating backup %s with kubevirt volume policy for case %s", backupName, brCase.Name)
	err = lib.CreateBackupWithVolumePolicy(dpaCR.Client, namespace, backupName, []string{brCase.Namespace}, true)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// Verify that the kubevirt-datamover controller creates a VirtualMachineBackupTracker
	// in the VM namespace during the backup, confirming VEP-25 incremental backup is active.
	vmbtSeen := false
	vmbtCheckDone := make(chan struct{})
	go func() {
		defer close(vmbtCheckDone)
		for i := 0; i < 60; i++ {
			found, checkErr := v.CheckVMBackupTrackerExists(brCase.Namespace)
			if checkErr == nil && found {
				log.Printf("VirtualMachineBackupTracker observed in %s — VEP-25 incremental backup confirmed", brCase.Namespace)
				vmbtSeen = true
				return
			}
			time.Sleep(5 * time.Second)
		}
		log.Printf("VirtualMachineBackupTracker was not observed in %s during backup window", brCase.Namespace)
	}()

	gomega.Eventually(lib.IsKubevirtDMBackupDone(dpaCR.Client, dynamicClientForSuiteRun, namespace, backupName), brCase.BackupTimeout, time.Second*10).Should(gomega.BeTrue())
	<-vmbtCheckDone
	describeBackup := lib.DescribeBackup(dpaCR.Client, namespace, backupName)
	ginkgo.GinkgoWriter.Println(describeBackup)

	succeeded, err := lib.IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(succeeded).To(gomega.Equal(true))
	log.Printf("Backup for case %s succeeded", brCase.Name)

	gomega.Expect(vmbtSeen).To(gomega.BeTrue(), "expected VirtualMachineBackupTracker to be observed during backup (VEP-25 incremental backup)")

	err = v.RemoveVm(brCase.Namespace, brCase.Name, 5*time.Minute)
	gomega.Expect(err).To(gomega.BeNil())
	err = lib.DeleteNamespace(v.Clientset, brCase.Namespace)
	gomega.Expect(err).To(gomega.BeNil())
	gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), time.Minute*5, time.Second*5).Should(gomega.BeTrue())
}

var _ = ginkgo.Describe("VM backup and restore tests", ginkgo.Ordered, func() {
	var v *lib.VirtOperator
	var err error
	wasInstalledFromTest := false

	cirrosDownloadedFromTest := false
	bootImageNamespace := "openshift-virtualization-os-images"

	var lastBRCase VmBackupRestoreCase
	var lastInstallTime time.Time
	updateLastBRcase := func(brCase VmBackupRestoreCase) {
		lastBRCase = brCase
	}

	var _ = ginkgo.BeforeAll(func() {
		indexTag := ""
		if useCommunityHco {
			indexTag = hcoIndexTag
			log.Printf("Creating community HCO CatalogSource with index tag %s", hcoIndexTag)
			err = lib.EnsureCommunityHcoCatalog(dynamicClientForSuiteRun, hcoIndexTag, 2*time.Minute)
			gomega.Expect(err).To(gomega.BeNil())
		}

		v, err = lib.GetVirtOperator(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, dynamicClientForSuiteRun, useUpstreamHco, indexTag)
		gomega.Expect(err).To(gomega.BeNil())
		gomega.Expect(v).ToNot(gomega.BeNil())

		if !v.IsVirtInstalled() {
			err = v.EnsureVirtInstallation()
			gomega.Expect(err).To(gomega.BeNil())
			wasInstalledFromTest = true
		}

		// Pre-flight: require HCO >= 1.18 and backup.kubevirt.io CRDs for VEP-25.
		if useCommunityHco {
			err = v.RequireVEP25Support()
			gomega.Expect(err).To(gomega.BeNil(), "VEP-25 pre-flight check failed — HCO 1.18+ and backup.kubevirt.io CRDs are required")
		}

		if kvmEmulation {
			err = v.EnsureEmulation(20 * time.Second)
			gomega.Expect(err).To(gomega.BeNil())
		} else {
			log.Println("Avoiding setting KVM emulation, by command line request")
		}

		log.Printf("Creating test storage classes test-sc-immediate and test-sc-wffc")
		err = v.CreateImmediateModeStorageClass("test-sc-immediate")
		gomega.Expect(err).To(gomega.BeNil())
		err = v.CreateWaitForFirstConsumerStorageClass("test-sc-wffc")
		gomega.Expect(err).To(gomega.BeNil())

		url, err := getLatestCirrosImageURL()
		gomega.Expect(err).To(gomega.BeNil())
		err = v.EnsureNamespace(bootImageNamespace, 1*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())
		if !v.CheckDataVolumeExists(bootImageNamespace, "cirros") {
			err = v.EnsureDataVolumeFromUrl(bootImageNamespace, "cirros", url, "150Mi", 5*time.Minute)
			gomega.Expect(err).To(gomega.BeNil())
			cirrosDownloadedFromTest = true
		}
		// Always ensure the DataSource exists, even if the DataVolume was
		// left over from a previous test run where the DataSource was not created.
		if !v.CheckDataSourceExists(bootImageNamespace, "cirros") {
			err = v.CreateDataSourceFromPvc(bootImageNamespace, "cirros")
			gomega.Expect(err).To(gomega.BeNil())
		}
		dpaCR.VeleroDefaultPlugins = append(dpaCR.VeleroDefaultPlugins, v1alpha1.DefaultPluginKubeVirt)
		dpaCR.VeleroDefaultPlugins = append(dpaCR.VeleroDefaultPlugins, v1alpha1.DefaultPluginKubeVirtDataMover)

		// TODO: remove once migtools/kubevirt-datamover-plugin#41 merges and the default
		// plugin image includes its fix — this pins to that PR's build in the meantime.
		if dpaCR.UnsupportedOverrides == nil {
			dpaCR.UnsupportedOverrides = map[v1alpha1.UnsupportedImageKey]string{}
		}
		dpaCR.UnsupportedOverrides[v1alpha1.KubeVirtDatamoverPluginImageKey] = "quay.io/tkaovila/kubevirt-datamover-plugin:pr-41"

		// TODO: remove once migtools/kubevirt-datamover-controller#124 (DataDownload
		// controller for VM restore, issue #73 phase 3) merges — pins to that PR's build
		// in the meantime so restore-from-CBT scenarios can exercise it pre-merge.
		dpaCR.UnsupportedOverrides[v1alpha1.KubeVirtDatamoverControllerImageKey] = "quay.io/tkaovila/kdm-controller:issue73-phase3"

		err = lib.DeleteBackupRepositories(runTimeClientForSuiteRun, namespace)
		gomega.Expect(err).To(gomega.BeNil())
		err = lib.InstallApplication(v.Client, "./sample-applications/virtual-machines/cirros-test/cirros-rbac.yaml")
		gomega.Expect(err).To(gomega.BeNil())

		// Fedora DataSource must be available in openshift-virtualization-os-images
		// for the Fedora VM specs, regardless of upstream/downstream or community/GA HCO.
		if v.CheckDataSourceExists("openshift-virtualization-os-images", "fedora") {
			log.Printf("Fedora DataSource already exists in openshift-virtualization-os-images, skipping creation")
		} else {
			log.Printf("Creating fedora DataSource in openshift-virtualization-os-images namespace")
			pvcNamespace, pvcName, err := v.GetDataSourcePvc("kubevirt-os-images", "fedora")
			if err != nil {
				log.Printf("Fedora DataSource is not PVC-backed, trying snapshot: %v", err)
				snapshotNamespace, snapshotName, snapErr := v.GetDataSourceSnapshot("kubevirt-os-images", "fedora")
				gomega.Expect(snapErr).To(gomega.BeNil())
				err = v.CreateTargetDataSourceFromSnapshot(snapshotNamespace, "openshift-virtualization-os-images", snapshotName, "fedora")
				gomega.Expect(err).To(gomega.BeNil())
			} else {
				err = v.CreateTargetDataSourceFromPvc(pvcNamespace, "openshift-virtualization-os-images", pvcName, "fedora")
				gomega.Expect(err).To(gomega.BeNil())
			}
		}

		log.Printf("Enabling CBT feature gate and label selector for kubevirt-datamover tests")
		err = v.EnableCBTFeatureGate(5 * time.Minute)
		gomega.Expect(err).To(gomega.BeNil())
		err = v.EnableCBTLabelSelector(30 * time.Second)
		gomega.Expect(err).To(gomega.BeNil())

	})

	var _ = ginkgo.AfterAll(func() {
		// DPA just needs to have BSL so gathering of backups/restores logs/describe work
		// using kopia to collect more info (DaemonSet)
		NewOADPDeploymentOperationDefault().Deploy(lib.KOPIA)

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
		}

		err = dpaCR.Delete()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		if v != nil {
			log.Printf("Removing test storage classes")
			_ = v.RemoveStorageClass("test-sc-immediate")
			_ = v.RemoveStorageClass("test-sc-wffc")
		}

		if v != nil && cirrosDownloadedFromTest {
			v.RemoveDataSource(bootImageNamespace, "cirros")
			v.RemoveDataVolume(bootImageNamespace, "cirros", 2*time.Minute)
		}

		if v != nil && wasInstalledFromTest {
			log.Printf("Skipping HCO/virt removal — leaving installation intact for reuse")
		}
	})

	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		tearDownBackupAndRestore(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	ginkgo.DescribeTable("Backup and restore virtual machines",
		func(brCase VmBackupRestoreCase, expectedError error) {
			runVmBackupAndRestore(brCase, updateLastBRcase, v)
		},

		ginkgo.Entry("no-application CSI datamover backup and restore, CirrOS VM", ginkgo.Label("virt"), VmBackupRestoreCase{
			Template:  "./sample-applications/virtual-machines/cirros-test/cirros-test.yaml",
			InitDelay: 2 * time.Minute, // Just long enough to get to login prompt, VM is marked running while kernel messages are still scrolling by
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "cirros-test",
				Name:              "cirros-test",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSIDataMover,
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),

		ginkgo.Entry("no-application CSI backup and restore, CirrOS VM", ginkgo.Label("virt"), VmBackupRestoreCase{
			Template:  "./sample-applications/virtual-machines/cirros-test/cirros-test.yaml",
			InitDelay: 2 * time.Minute, // Just long enough to get to login prompt, VM is marked running while kernel messages are still scrolling by
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "cirros-test",
				Name:              "cirros-test",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSI,
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),

		ginkgo.Entry("no-application CSI backup and restore, powered-off CirrOS VM", ginkgo.Label("virt"), VmBackupRestoreCase{
			Template:   "./sample-applications/virtual-machines/cirros-test/cirros-test.yaml",
			InitDelay:  2 * time.Minute,
			PowerState: "Stopped",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "cirros-test",
				Name:              "cirros-test",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSI,
				BackupTimeout:     20 * time.Minute,
				PreBackupVerify:   vmPoweredOff("cirros-test", "cirros-test"),
			},
		}, nil),

		ginkgo.Entry("immediate binding no-application CSI datamover backup and restore, CirrOS VM", ginkgo.Label("virt"), VmBackupRestoreCase{
			Template:  "./sample-applications/virtual-machines/cirros-test/cirros-test-immediate.yaml",
			InitDelay: 2 * time.Minute, // Just long enough to get to login prompt, VM is marked running while kernel messages are still scrolling by
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "cirros-test",
				Name:              "cirros-test",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSIDataMover,
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),

		ginkgo.Entry("immediate binding no-application CSI backup and restore, CirrOS VM", ginkgo.Label("virt"), VmBackupRestoreCase{
			Template:  "./sample-applications/virtual-machines/cirros-test/cirros-test-immediate.yaml",
			InitDelay: 2 * time.Minute, // Just long enough to get to login prompt, VM is marked running while kernel messages are still scrolling by
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "cirros-test",
				Name:              "cirros-test",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSI,
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),

		ginkgo.Entry("immediate binding no-application CSI+datamover backup and restore, powered-off CirrOS VM", ginkgo.Label("virt"), VmBackupRestoreCase{
			Template:   "./sample-applications/virtual-machines/cirros-test/cirros-test-immediate.yaml",
			InitDelay:  2 * time.Minute,
			PowerState: "Stopped",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "cirros-test",
				Name:              "cirros-test",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSIDataMover,
				BackupTimeout:     20 * time.Minute,
				PreBackupVerify:   vmPoweredOff("cirros-test", "cirros-test"),
			},
		}, nil),

		ginkgo.Entry("immediate binding no-application CSI backup and restore, powered-off CirrOS VM", ginkgo.Label("virt"), VmBackupRestoreCase{
			Template:   "./sample-applications/virtual-machines/cirros-test/cirros-test-immediate.yaml",
			InitDelay:  2 * time.Minute,
			PowerState: "Stopped",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "cirros-test",
				Name:              "cirros-test",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSI,
				BackupTimeout:     20 * time.Minute,
				PreBackupVerify:   vmPoweredOff("cirros-test", "cirros-test"),
			},
		}, nil),

		ginkgo.Entry("no-application CSI datamover backup and restore, multi-PVC CirrOS VM", ginkgo.Label("virt"), VmBackupRestoreCase{
			Template:  "./sample-applications/virtual-machines/cirros-test/cirros-test-multipvc.yaml",
			InitDelay: 2 * time.Minute, // Just long enough to get to login prompt, VM is marked running while kernel messages are still scrolling by
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "cirros-multipvc-test",
				Name:              "cirros-multipvc-test",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSIDataMover,
				BackupTimeout:     20 * time.Minute,
				PostRestoreVerify: vmPvcsBound("cirros-multipvc-test", "cirros-multipvc-test-disk", "cirros-multipvc-test-datadisk"),
			},
		}, nil),

		ginkgo.PEntry("todolist CSI backup and restore, in a Fedora VM", ginkgo.Label("virt"), VmBackupRestoreCase{
			Template:  "./sample-applications/virtual-machines/fedora-todolist/fedora-todolist.yaml",
			InitDelay: 3 * time.Minute, // For cloud-init
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "fedora-todolist",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   vmTodoListReady(true, false, "mysql"),
				PostRestoreVerify: vmTodoListReady(false, false, "mysql"),
				BackupTimeout:     45 * time.Minute,
			},
		}, nil),
	)

	ginkgo.DescribeTable("Kubevirt datamover backup with CBT",
		func(brCase VmBackupRestoreCase, expectedError error) {
			runCBTVmBackup(brCase, updateLastBRcase, v)
		},

		ginkgo.Entry("no-application kubevirt-datamover backup, CirrOS VM with CBT", ginkgo.Label("virt"), VmBackupRestoreCase{
			Template: "./sample-applications/virtual-machines/cirros-test/cirros-test-cbt.yaml",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "cirros-test",
				Name:              "cirros-test",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSIDataMover,
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),

		// FEDORA is not yet ready for CI and CBT
		// ginkgo.Entry("todolist kubevirt-datamover backup, Fedora VM with CBT", ginkgo.Label("virt"), VmBackupRestoreCase{
		// 	Template:  "./sample-applications/virtual-machines/kubevirt-dm/fedora-todolist-cbt.yaml",
		// 	InitDelay: 3 * time.Minute,
		// 	BackupRestoreCase: BackupRestoreCase{
		// 		Namespace:         "mysql-persistent",
		// 		Name:              "fedora-todolist",
		// 		SkipVerifyLogs:    true,
		// 		BackupRestoreType: lib.CSIDataMover,
		// 		BackupTimeout:     45 * time.Minute,
		// 	},
		// }, nil),
	)

	// Automates scenarios 1-3 from https://github.com/openshift/oadp-operator/issues/2252
	// (a manual test writeup of kubevirt-datamover incremental-backup-sequence behavior).
	// Reuses the outer BeforeAll's HCO/CBT-feature-gate/storage-class setup and the shared
	// VirtOperator v.
	//
	// Scenario 4 (delete libvirt checkpoints with maxIncrementalBackups=0) hits an unfixed
	// upstream bug (CNV-85377: virt-controller never falls back to full, VMB hangs
	// Initializing forever) — scaffolded below as a real, compiling ginkgo.PIt rather than
	// deleted or left as a comment, ready to flip to ginkgo.It once that bug is fixed.
	ginkgo.Describe("Kubevirt datamover incremental backup sequence", ginkgo.Ordered, func() {
		const (
			incSeqNamespace = "cirros-test"
			incSeqVMName    = "cirros-test"
			incSeqTemplate  = "./sample-applications/virtual-machines/cirros-test/cirros-test-cbt.yaml"
		)

		// Registered with the outer scope's updateLastBRcase/prepareBackupAndRestore below so
		// the shared AfterEach (declared in the outer Describe, which also fires after specs in
		// this nested Describe) tears down THIS case's deployment/namespace instead of stale
		// state left over from the last DescribeTable entry that ran before it.
		incSeqCase := VmBackupRestoreCase{
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         incSeqNamespace,
				Name:              incSeqVMName,
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSIDataMover,
				BackupTimeout:     20 * time.Minute,
			},
		}

		var backupCount int

		runSequenceBackup := func(expectedType string) {
			backupCount++
			backupName := fmt.Sprintf("cirros-incr-seq-%d", backupCount)

			err := lib.EnsureKubevirtVolumePolicy(dpaCR.Client, namespace)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to ensure kubevirt volume policy")
			err = lib.CreateBackupWithVolumePolicy(dpaCR.Client, namespace, backupName, []string{incSeqNamespace}, true)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to create backup %s", backupName)

			gomega.Eventually(lib.IsKubevirtDMBackupDone(dpaCR.Client, dynamicClientForSuiteRun, namespace, backupName), 20*time.Minute, time.Second*10).
				Should(gomega.BeTrue(), "backup %s did not complete", backupName)
			succeeded, err := lib.IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to check completion status of backup %s", backupName)
			gomega.Expect(succeeded).To(gomega.BeTrue(), "backup %s did not complete successfully", backupName)

			dataUploadName, expectedBackupType, err := lib.GetDataUploadForBackup(dpaCR.Client, namespace, backupName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get DataUpload for backup %s", backupName)
			gomega.Expect(expectedBackupType).To(gomega.Equal(expectedType), "controller's expected-backup-type annotation on DataUpload")

			actualBackupType, _, err := v.GetVMBBackupType(incSeqNamespace, dataUploadName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get VirtualMachineBackup status for DataUpload %s", dataUploadName)
			gomega.Expect(actualBackupType).To(gomega.Equal(expectedType), "actual VirtualMachineBackup.Status.Type")
			gomega.Expect(actualBackupType).To(gomega.Equal(expectedBackupType), "expected vs. actual backup type must not mismatch")
		}

		var _ = ginkgo.BeforeAll(func() {
			updateLastBRcase(incSeqCase)
			prepareBackupAndRestore(incSeqCase.BackupRestoreCase, func() {})

			_ = v.RemoveVm(incSeqNamespace, incSeqVMName, 2*time.Minute)
			err := lib.DeleteNamespace(v.Clientset, incSeqNamespace)
			gomega.Expect(err).To(gomega.BeNil(), "failed to delete namespace %s before setup", incSeqNamespace)
			gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, incSeqNamespace), time.Minute*2, time.Second*5).
				Should(gomega.BeTrue(), "namespace %s was not deleted before setup", incSeqNamespace)

			err = lib.CreateNamespace(v.Clientset, incSeqNamespace)
			gomega.Expect(err).To(gomega.BeNil(), "failed to create namespace %s", incSeqNamespace)
			err = lib.InstallApplication(v.Client, incSeqTemplate)
			gomega.Expect(err).To(gomega.BeNil(), "failed to install VM template %s in namespace %s", incSeqTemplate, incSeqNamespace)

			log.Printf("Waiting for VM %s/%s to reach Running status", incSeqNamespace, incSeqVMName)
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 15*time.Minute, true, func(ctx context.Context) (bool, error) {
				status, statusErr := v.GetVmStatus(incSeqNamespace, incSeqVMName)
				if statusErr != nil {
					return false, nil
				}
				return status == "Running", nil
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "VM %s/%s did not reach Running status", incSeqNamespace, incSeqVMName)
			err = v.WaitForVMReady(incSeqNamespace, incSeqVMName, 5*time.Minute)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "VM %s/%s was not ready", incSeqNamespace, incSeqVMName)
		})

		// A single ordered spec, not four separate ginkgo.It()s: the shared AfterEach
		// (declared in the outer Describe) undeploys the CSI+datamover stack and deletes
		// incSeqNamespace after every spec it fires for, which would tear down this VM
		// between scenarios if they were split into multiple Its. Collapsing them into one
		// It's sequential steps means that teardown only fires once, after the whole
		// sequence — and this It does its own full cleanup at the end anyway (matching
		// runVmBackupAndRestore's "avoid finalizers in namespace deletion" convention), so
		// the shared AfterEach's redundant Undeploy/deleteNamespace afterward is a no-op.
		ginkgo.It("full backup, incremental chain, restart, and max-limit fallback", ginkgo.Label("virt"), func() {
			ginkgo.By("backup 1: first-ever backup is full")
			runSequenceBackup("full")

			ginkgo.By("backup 2: second backup is incremental")
			runSequenceBackup("incremental")

			ginkgo.By("VM restart does not invalidate the checkpoint chain")
			err := v.RestartVmAndWaitRunning(incSeqNamespace, incSeqVMName, 10*time.Minute)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "VM %s/%s failed to restart", incSeqNamespace, incSeqVMName)
			runSequenceBackup("incremental")

			ginkgo.By("hitting maxIncrementalBackups forces a full backup")
			// Per-VM annotation override (takes effect immediately, unlike patching the
			// DPA-level setting which requires waiting for a controller rollout).
			err = v.SetVMAnnotation(incSeqNamespace, incSeqVMName, "kubevirt-datamover.io/max-incremental-backups", "2")
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to set max-incremental-backups annotation on VM %s/%s", incSeqNamespace, incSeqVMName)
			// backupCount is 3 (1 full + 2 incremental) at this point, so
			// incrementalCount(2) >= maxIncrementalBackups(2) forces this backup full.
			runSequenceBackup("full")

			err = v.RemoveVm(incSeqNamespace, incSeqVMName, 5*time.Minute)
			gomega.Expect(err).To(gomega.BeNil(), "failed to remove VM %s/%s", incSeqNamespace, incSeqVMName)
			err = lib.DeleteNamespace(v.Clientset, incSeqNamespace)
			gomega.Expect(err).To(gomega.BeNil(), "failed to delete namespace %s", incSeqNamespace)
		})

		ginkgo.PIt("backup after deleting libvirt checkpoints with maxIncrementalBackups=0 hangs forever — blocked by CNV-85377", ginkgo.Label("virt"), func() {
			// See https://redhat.atlassian.net/browse/CNV-85377 and
			// https://github.com/openshift/oadp-operator/issues/2252 (Test 4): once a
			// libvirt checkpoint is deleted from the virt-launcher pod, virt-controller
			// repeatedly fails with "Domain checkpoint not found" and never falls back to
			// a full backup — the VMB stays Initializing forever, so this can't pass today.
			err := v.SetVMAnnotation(incSeqNamespace, incSeqVMName, "kubevirt-datamover.io/max-incremental-backups", "0")
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to set max-incremental-backups annotation on VM %s/%s", incSeqNamespace, incSeqVMName)

			out, err := v.RunVirshCommand(kubeConfig, incSeqNamespace, incSeqVMName, "checkpoint-list", "--domain", incSeqVMName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to list libvirt checkpoints for VM %s/%s", incSeqNamespace, incSeqVMName)
			log.Printf("libvirt checkpoints before deletion: %s", out)

			// TODO: parse real `virsh checkpoint-list` table output once run against a live
			// cluster — this naive split is a placeholder for pending, non-running code.
			for _, checkpoint := range strings.Fields(out) {
				_, err := v.RunVirshCommand(kubeConfig, incSeqNamespace, incSeqVMName, "checkpoint-delete", checkpoint)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to delete checkpoint %s for VM %s/%s", checkpoint, incSeqNamespace, incSeqVMName)
			}

			// Known to hang — documents the bug's current behavior, not the desired one.
			runSequenceBackup("full")
		})
	})
})

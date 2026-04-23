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

		url, err := getLatestCirrosImageURL()
		gomega.Expect(err).To(gomega.BeNil())
		err = v.EnsureNamespace(bootImageNamespace, 1*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())
		if !v.CheckDataVolumeExists(bootImageNamespace, "cirros") {
			err = v.EnsureDataVolumeFromUrl(bootImageNamespace, "cirros", url, "150Mi", 5*time.Minute)
			gomega.Expect(err).To(gomega.BeNil())
			err = v.CreateDataSourceFromPvc(bootImageNamespace, "cirros")
			gomega.Expect(err).To(gomega.BeNil())
			cirrosDownloadedFromTest = true
		}
		dpaCR.VeleroDefaultPlugins = append(dpaCR.VeleroDefaultPlugins, v1alpha1.DefaultPluginKubeVirt)
		dpaCR.VeleroDefaultPlugins = append(dpaCR.VeleroDefaultPlugins, v1alpha1.DefaultPluginKubeVirtDataMover)

		err = lib.DeleteBackupRepositories(runTimeClientForSuiteRun, namespace)
		gomega.Expect(err).To(gomega.BeNil())
		err = lib.InstallApplication(v.Client, "./sample-applications/virtual-machines/cirros-test/cirros-rbac.yaml")
		gomega.Expect(err).To(gomega.BeNil())

		// Fedora DataSource setup is only needed for GA OpenShift Virt (TEST_VIRT_GA).
		// Community HCO tests (TEST_VIRT) use CirrOS and do not require it.
		if v.Upstream && v.CommunityIndex == "" {
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

		ginkgo.Entry("todolist CSI backup and restore, in a Fedora VM", ginkgo.Label("virt"), VmBackupRestoreCase{
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

		ginkgo.Entry("todolist kubevirt-datamover backup, Fedora VM with CBT", ginkgo.Label("virt"), VmBackupRestoreCase{
			Template:  "./sample-applications/virtual-machines/cirros-test/kubevirt-dm/fedora-todolist-cbt.yaml",
			InitDelay: 3 * time.Minute,
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "fedora-todolist",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSIDataMover,
				BackupTimeout:     45 * time.Minute,
			},
		}, nil),
	)
})

package e2e_test

import (
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
		err = lib.VerifyBackupRestoreData(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, kubeConfig, artifact_dir, namespace, "todolist-route", "todolist", "todolist", preBackupState, twoVol)
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

func runVmBackupAndRestore(brCase VmBackupRestoreCase, expectedErr error, updateLastBRcase func(brCase VmBackupRestoreCase), updateLastInstallTime func(), v *lib.VirtOperator) {
	updateLastBRcase(brCase)

	// Create DPA
	backupName, restoreName := prepareBackupAndRestore(brCase.BackupRestoreCase, func() {})

	err := lib.CreateNamespace(v.Clientset, brCase.Namespace)
	gomega.Expect(err).To(gomega.BeNil())

	err = lib.InstallApplication(v.Client, brCase.Template)
	if err != nil {
		fmt.Printf("Failed to install VM template %s: %v", brCase.Template, err)
	}
	gomega.Expect(err).To(gomega.BeNil())

	// Wait for VM to start, then give some time for cloud-init to run.
	// Afterward, run through the standard application verification to make sure
	// the application itself is working correctly.
	err = wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) {
		status, err := v.GetVmStatus(brCase.Namespace, brCase.Name)
		return status == "Running", err
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// TODO: find a better way to check for clout-init completion
	if brCase.InitDelay > 0*time.Second {
		log.Printf("Sleeping to wait for cloud-init to be ready...")
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
	updateLastInstallTime := func() {
		lastInstallTime = time.Now()
	}

	var _ = ginkgo.BeforeAll(func() {
		v, err = lib.GetVirtOperator(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, dynamicClientForSuiteRun, useUpstreamHco)
		gomega.Expect(err).To(gomega.BeNil())
		gomega.Expect(v).ToNot(gomega.BeNil())

		if !v.IsVirtInstalled() {
			err = v.EnsureVirtInstallation()
			gomega.Expect(err).To(gomega.BeNil())
			wasInstalledFromTest = true
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

		err = v.CreateImmediateModeStorageClass("test-sc-immediate")
		gomega.Expect(err).To(gomega.BeNil())
		err = v.CreateWaitForFirstConsumerStorageClass("test-sc-wffc")
		gomega.Expect(err).To(gomega.BeNil())
		err = lib.DeleteBackupRepositories(runTimeClientForSuiteRun, namespace)
		gomega.Expect(err).To(gomega.BeNil())
		err = lib.InstallApplication(v.Client, "./sample-applications/virtual-machines/cirros-test/cirros-rbac.yaml")
		gomega.Expect(err).To(gomega.BeNil())

		if v.Upstream {
			log.Printf("Creating fedora DataSource in openshift-virtualization-os-images namespace")
			pvcNamespace, pvcName, err := v.GetDataSourcePvc("kubevirt-os-images", "fedora")
			gomega.Expect(err).To(gomega.BeNil())
			err = v.CreateTargetDataSourceFromPvc(pvcNamespace, "openshift-virtualization-os-images", pvcName, "fedora")
			gomega.Expect(err).To(gomega.BeNil())
		}

	})

	var _ = ginkgo.AfterAll(func() {
		if v != nil && cirrosDownloadedFromTest {
			v.RemoveDataSource(bootImageNamespace, "cirros")
			v.RemoveDataVolume(bootImageNamespace, "cirros", 2*time.Minute)
		}

		if v != nil && wasInstalledFromTest {
			v.EnsureVirtRemoval()
		}

		err := v.RemoveStorageClass("test-sc-immediate")
		gomega.Expect(err).To(gomega.BeNil())
		err = v.RemoveStorageClass("test-sc-wffc")
		gomega.Expect(err).To(gomega.BeNil())
	})

	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		tearDownBackupAndRestore(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	ginkgo.DescribeTable("Backup and restore virtual machines",
		func(brCase VmBackupRestoreCase, expectedError error) {
			runVmBackupAndRestore(brCase, expectedError, updateLastBRcase, updateLastInstallTime, v)
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
})

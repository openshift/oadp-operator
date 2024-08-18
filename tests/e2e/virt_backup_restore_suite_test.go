package e2e_test

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

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
		Eventually(isOff, time.Minute*10, time.Second*10).Should(BeTrue())
		return nil
	})
}

type VmBackupRestoreCase struct {
	BackupRestoreCase
	Template   string
	InitDelay  time.Duration
	PowerState string
	RestoreErr error
}

func runVmBackupAndRestore(brCase VmBackupRestoreCase, expectedErr error, updateLastBRcase func(brCase VmBackupRestoreCase), updateLastInstallTime func(), v *lib.VirtOperator) {
	updateLastBRcase(brCase)

	// Create DPA
	backupName, restoreName := prepareBackupAndRestore(brCase.BackupRestoreCase, func() {})

	err := lib.CreateNamespace(v.Clientset, brCase.Namespace)
	Expect(err).To(BeNil())

	err = lib.InstallApplication(v.Client, brCase.Template)
	if err != nil {
		fmt.Printf("Failed to install VM template %s: %v", brCase.Template, err)
	}
	Expect(err).To(BeNil())

	// Wait for VM to start, then give some time for cloud-init to run.
	// Afterward, run through the standard application verification to make sure
	// the application itself is working correctly.
	err = wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) {
		status, err := v.GetVmStatus(brCase.Namespace, brCase.Name)
		return status == "Running", err
	})
	Expect(err).ToNot(HaveOccurred())

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
		Expect(err).To(BeNil())
	}

	// Run optional custom verification
	if brCase.PreBackupVerify != nil {
		log.Printf("Running pre-backup custom function for case %s", brCase.Name)
		err := brCase.PreBackupVerify(dpaCR.Client, brCase.Namespace)
		Expect(err).ToNot(HaveOccurred())
	}

	// Back up VM
	nsRequiresResticDCWorkaround := runBackup(brCase.BackupRestoreCase, backupName)

	// Delete everything in test namespace
	err = v.RemoveVm(brCase.Namespace, brCase.Name, 5*time.Minute)
	Expect(err).To(BeNil())
	err = lib.DeleteNamespace(v.Clientset, brCase.Namespace)
	Expect(err).To(BeNil())
	Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), time.Minute*5, time.Second*5).Should(BeTrue())

	// Do restore
	runVmRestore(brCase, backupName, restoreName, nsRequiresResticDCWorkaround)

	// Run optional custom verification
	if brCase.PostRestoreVerify != nil {
		log.Printf("Running post-restore custom function for VM case %s", brCase.Name)
		err = brCase.PostRestoreVerify(dpaCR.Client, brCase.Namespace)
		Expect(err).ToNot(HaveOccurred())
	}

	// avoid finalizers in namespace deletion
	err = v.RemoveVm(brCase.Namespace, brCase.Name, 5*time.Minute)
	Expect(err).To(BeNil())
}

var _ = Describe("VM backup and restore tests", Ordered, func() {
	var v *lib.VirtOperator
	var err error
	wasInstalledFromTest := false
	var lastBRCase VmBackupRestoreCase
	var lastInstallTime time.Time
	updateLastBRcase := func(brCase VmBackupRestoreCase) {
		lastBRCase = brCase
	}
	updateLastInstallTime := func() {
		lastInstallTime = time.Now()
	}

	var _ = BeforeAll(func() {
		v, err = lib.GetVirtOperator(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, dynamicClientForSuiteRun)
		Expect(err).To(BeNil())
		Expect(v).ToNot(BeNil())

		if !v.IsVirtInstalled() {
			err = v.EnsureVirtInstallation()
			Expect(err).To(BeNil())
			wasInstalledFromTest = true
		}

		err = v.EnsureEmulation(20 * time.Second)
		Expect(err).To(BeNil())

		url, err := getLatestCirrosImageURL()
		Expect(err).To(BeNil())
		err = v.EnsureDataVolumeFromUrl("openshift-virtualization-os-images", "cirros", url, "128Mi", 5*time.Minute)
		Expect(err).To(BeNil())
		err = v.CreateDataSourceFromPvc("openshift-virtualization-os-images", "cirros")
		Expect(err).To(BeNil())

		dpaCR.VeleroDefaultPlugins = append(dpaCR.VeleroDefaultPlugins, v1alpha1.DefaultPluginKubeVirt)

		err = v.CreateImmediateModeStorageClass("test-sc-immediate")
		Expect(err).To(BeNil())
	})

	var _ = AfterAll(func() {
		v.RemoveDataSource("openshift-virtualization-os-images", "cirros")
		v.RemoveDataVolume("openshift-virtualization-os-images", "cirros", 2*time.Minute)

		if v != nil && wasInstalledFromTest {
			v.EnsureVirtRemoval()
		}

		err := v.RemoveStorageClass("test-sc-immediate")
		Expect(err).To(BeNil())
	})

	var _ = AfterEach(func(ctx SpecContext) {
		tearDownBackupAndRestore(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	DescribeTable("Backup and restore virtual machines",
		func(brCase VmBackupRestoreCase, expectedError error) {
			runVmBackupAndRestore(brCase, expectedError, updateLastBRcase, updateLastInstallTime, v)
		},

		Entry("no-application CSI datamover backup and restore, CirrOS VM", Label("virt"), VmBackupRestoreCase{
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

		Entry("no-application CSI backup and restore, CirrOS VM", Label("virt"), VmBackupRestoreCase{
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

		Entry("no-application CSI backup and restore, powered-off CirrOS VM", Label("virt"), VmBackupRestoreCase{
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

		Entry("no-application CSI+datamover backup and restore, powered-off CirrOS VM", Label("virt"), VmBackupRestoreCase{
			Template:   "./sample-applications/virtual-machines/cirros-test/cirros-test.yaml",
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
			RestoreErr: errors.New("error to expose snapshot: error to wait target PVC consumed"),
		}, nil),

		Entry("immediate binding no-application CSI datamover backup and restore, CirrOS VM", Label("virt"), VmBackupRestoreCase{
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

		Entry("immediate binding no-application CSI backup and restore, CirrOS VM", Label("virt"), VmBackupRestoreCase{
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

		Entry("immediate binding no-application CSI+datamover backup and restore, powered-off CirrOS VM", Label("virt"), VmBackupRestoreCase{
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

		Entry("immediate binding no-application CSI backup and restore, powered-off CirrOS VM", Label("virt"), VmBackupRestoreCase{
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

		Entry("todolist CSI backup and restore, in a Fedora VM", Label("virt"), VmBackupRestoreCase{
			Template:  "./sample-applications/virtual-machines/fedora-todolist/fedora-todolist.yaml",
			InitDelay: 3 * time.Minute, // For cloud-init
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "fedora-todolist",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   todoListReady(true, false, "mysql"),
				PostRestoreVerify: todoListReady(false, false, "mysql"),
				BackupTimeout:     45 * time.Minute,
			},
		}, nil),
	)
})

// Temporary VM-specific copy of runRestore. This is here to avoid making big
// changes to runRestore when they are likely to be taken out in the near future.
func runVmRestore(brCase VmBackupRestoreCase, backupName, restoreName string, nsRequiresResticDCWorkaround bool) {
	log.Printf("Creating restore %s for case %s", restoreName, brCase.Name)
	err := lib.CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
	Expect(err).ToNot(HaveOccurred())
	Eventually(lib.IsRestoreDone(dpaCR.Client, namespace, restoreName), time.Minute*60, time.Second*10).Should(BeTrue())
	// TODO only log on fail?
	describeRestore := lib.DescribeRestore(veleroClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	GinkgoWriter.Println(describeRestore)

	restoreLogs := lib.RestoreLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	restoreErrorLogs := lib.RestoreErrorLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	accumulatedTestLogs = append(accumulatedTestLogs, describeRestore, restoreLogs)

	if !brCase.SkipVerifyLogs {
		Expect(restoreErrorLogs).Should(Equal([]string{}))
	}

	// Check if restore succeeded
	succeeded, err := lib.IsRestoreCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	if brCase.RestoreErr != nil {
		Expect(succeeded).To(Equal(false))
		Expect(err.Error() + describeRestore).To(ContainSubstring(brCase.RestoreErr.Error()))
	} else {
		Expect(err).ToNot(HaveOccurred())
		Expect(succeeded).To(Equal(true))
	}

	if nsRequiresResticDCWorkaround {
		// We run the dc-post-restore.sh script for both restic and
		// kopia backups and for any DCs with attached volumes,
		// regardless of whether it was restic or kopia backup.
		// The script is designed to work with labels set by the
		// openshift-velero-plugin and can be run without pre-conditions.
		log.Printf("Running dc-post-restore.sh script.")
		err = lib.RunDcPostRestoreScript(restoreName)
		Expect(err).ToNot(HaveOccurred())
	}
}

package e2e_test

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	ginkgov2 "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"

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

type VmBackupRestoreCase struct {
	BackupRestoreCase
	Template        string
	InitDelay       time.Duration
	Source          string
	SourceNamespace string
}

func runVmBackupAndRestore(brCase VmBackupRestoreCase, expectedErr error, updateLastBRcase func(brCase VmBackupRestoreCase), updateLastInstallTime func(), v *lib.VirtOperator) {
	updateLastBRcase(brCase)

	diskName := brCase.Name + "-disk"
	vmName := brCase.Name + "-vm"

	// Create DPA
	backupName, restoreName := prepareBackupAndRestore(brCase.BackupRestoreCase, func() {})

	err := lib.CreateNamespace(v.Clientset, brCase.Namespace)
	gomega.Expect(err).To(gomega.BeNil())

	if brCase.Template == "" { // No template: CirrOS test
		// Create a standalone VM disk. This defaults to cloning the CirrOS image.
		// This uses a DataVolume so import and clone progress can be monitored.
		// Behavioral notes: CDI will garbage collect DataVolumes if they are not
		// attached to anything, so this disk needs to include the annotation
		// storage.deleteAfterCompletion in order to keep it around long enough to
		// attach to a VM. CDI also waits until the DataVolume is attached to
		// something before running whatever import process it has been asked to
		// run, so this disk also needs the storage.bind.immediate.requested
		// annotation to get it to start the cloning process.
		err = v.CreateDiskFromYaml(brCase.Namespace, diskName, 5*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())

		err = v.CreateVm(brCase.Namespace, vmName, 5*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())

		// Remove the Data Volume, but keep the PVC attached to the VM. The
		// DataVolume must be removed before backing up, because otherwise the
		// restore process might try to follow the instructions in the spec. For
		// example, a DataVolume that lists a cloned PVC will get restored in the
		// same state, and attempt to clone the PVC again after restore. The
		// DataVolume also needs to be detached from the VM, so that the VM restore
		// does not try to use the DataVolume that was deleted.
		err = v.DetachPvc(brCase.Namespace, diskName, 2*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())
		err = v.RemoveDataVolume(brCase.Namespace, diskName, 2*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())

	} else { // Fedora test
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
	}

	// TODO: find a better way to check for clout-init completion
	if brCase.InitDelay > 0*time.Second {
		log.Printf("Sleeping to wait for cloud-init to be ready...")
		time.Sleep(brCase.InitDelay)
	}

	// Back up VM
	nsRequiresResticDCWorkaround := runBackup(brCase.BackupRestoreCase, backupName)

	// Delete everything in test namespace
	if brCase.Template == "" { // No template: CirrOS test
		err = v.RemoveVm(brCase.Namespace, vmName, 2*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())
		err = v.RemovePvc(brCase.Namespace, diskName, 2*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())
	} else { // Fedora test
		err = v.RemoveVm(brCase.Namespace, brCase.Name, 2*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())
	}
	err = lib.DeleteNamespace(v.Clientset, brCase.Namespace)
	gomega.Expect(err).To(gomega.BeNil())
	gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), timeoutMultiplier*time.Minute*5, time.Second*5).Should(gomega.BeTrue())

	// Do restore
	runRestore(brCase.BackupRestoreCase, backupName, restoreName, nsRequiresResticDCWorkaround)

	// Run optional custom verification
	if brCase.PostRestoreVerify != nil {
		log.Printf("Running post-restore function for VM case %s", brCase.Name)
		err = brCase.PostRestoreVerify(dpaCR.Client, brCase.Namespace)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
}

var _ = ginkgov2.Describe("VM backup and restore tests", ginkgov2.Ordered, func() {
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

	var _ = ginkgov2.BeforeAll(func() {
		v, err = lib.GetVirtOperator(runTimeClientForSuiteRun, kubernetesClientForSuiteRun, dynamicClientForSuiteRun)
		gomega.Expect(err).To(gomega.BeNil())
		gomega.Expect(v).ToNot(gomega.BeNil())

		if !v.IsVirtInstalled() {
			err = v.EnsureVirtInstallation()
			gomega.Expect(err).To(gomega.BeNil())
			wasInstalledFromTest = true
		}

		err = v.EnsureEmulation(20 * time.Second)
		gomega.Expect(err).To(gomega.BeNil())

		url, err := getLatestCirrosImageURL()
		gomega.Expect(err).To(gomega.BeNil())
		err = v.EnsureDataVolumeFromUrl("openshift-cnv", "cirros-dv", url, "128Mi", 5*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())

		dpaCR.CustomResource.Spec.Configuration.Velero.DefaultPlugins = append(dpaCR.CustomResource.Spec.Configuration.Velero.DefaultPlugins, v1alpha1.DefaultPluginKubeVirt)
	})

	var _ = ginkgov2.AfterAll(func() {
		v.RemoveDataVolume("openshift-cnv", "cirros-dv", 2*time.Minute)

		if v != nil && wasInstalledFromTest {
			v.EnsureVirtRemoval()
		}
	})

	var _ = ginkgov2.AfterEach(func(ctx ginkgov2.SpecContext) {
		tearDownBackupAndRestore(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	ginkgov2.DescribeTable("Backup and restore virtual machines",
		func(brCase VmBackupRestoreCase, expectedError error) {
			runVmBackupAndRestore(brCase, expectedError, updateLastBRcase, updateLastInstallTime, v)
		},

		ginkgov2.Entry("no-application CSI datamover backup and restore, CirrOS VM", ginkgov2.Label("virt"), VmBackupRestoreCase{
			Source:          "cirros-dv",
			SourceNamespace: "openshift-cnv",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "cirros-test",
				Name:              "cirros-test",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSIDataMover,
				BackupTimeout:     20 * time.Minute,
			},
		}, nil),

		ginkgov2.Entry("todolist CSI backup and restore, in a Fedora VM", ginkgov2.Label("virt"), VmBackupRestoreCase{
			Template:  "./sample-applications/virtual-machines/fedora-todolist/fedora-todolist.yaml",
			InitDelay: 3 * time.Minute,
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "mysql-persistent",
				Name:              "fedora-todolist",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSI,
				PreBackupVerify:   mysqlReady(true, false),
				PostRestoreVerify: mysqlReady(false, false),
				BackupTimeout:     45 * time.Minute,
			},
		}, nil),
	)
})

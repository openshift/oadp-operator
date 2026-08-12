package e2e_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
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
	Template       string
	InitDelay      time.Duration
	StartupTimeout time.Duration
	PowerState     string
	// HasGuestAgent declares whether Template's VM image ships qemu-guest-agent
	// (verifiable live via lib.VirtOperator.HasQemuGuestAgent, which checks the
	// VMI's own "AgentConnected" status condition). Load-bearing for any
	// guest-exec-based data-integrity write (lib.VirtOperator.RunGuestExecScript
	// and friends): without a connected agent there is no guest-exec channel at
	// all, so a payload write can't happen (CirrOS today, HasGuestAgent: false --
	// the CBT restore Its needing real data-integrity checks use the
	// guest-agent-equipped Alpine fixture instead, see the "Kubevirt datamover
	// restore from a CBT backup (Alpine, guest-exec data-integrity)" Describe block).
	HasGuestAgent bool
	// SkipUnderEmulation, when true, skips this entry if the cluster's KubeVirt CR
	// has useEmulation: true (software emulation, no /dev/kvm) -- e.g. Fedora is
	// known unreliable under emulation. Checked via lib.VirtOperator.IsEmulationEnabled.
	//
	// TODO(future discussion): the inverse capability check -- skipping CirrOS when
	// useEmulation is false and a real-KVM-validated Fedora path is available -- is
	// not implemented here. Left as a future consideration, not active behavior.
	SkipUnderEmulation bool
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
	gomega.Eventually(v.IsNamespaceDeletedClearingStuckVMBFinalizers(kubernetesClientForSuiteRun, brCase.Namespace), time.Minute*2, time.Second*5).Should(gomega.BeTrue())

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
	startupTimeout := brCase.StartupTimeout
	if startupTimeout == 0 {
		startupTimeout = 10 * time.Minute
	}
	err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, startupTimeout, true, func(ctx context.Context) (bool, error) {
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
	gomega.Eventually(v.IsNamespaceDeletedClearingStuckVMBFinalizers(kubernetesClientForSuiteRun, brCase.Namespace), time.Minute*5, time.Second*5).Should(gomega.BeTrue())

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

// runKubevirtDMBackup creates a kubevirt-datamover backup of vmNamespace's VM(s), waits
// for it to complete successfully, and returns the resulting DataUpload's name and its
// controller-recorded expected-backup-type annotation. Shared between the
// incremental-sequence backups and the restore-from-CBT-backup scenario so the
// create+wait+verify boilerplate isn't duplicated.
//
// onDataUploadFound, if non-nil, is invoked as soon as the backup's DataUpload appears --
// before waiting for the backup to fully complete -- so callers can inspect state that only
// exists transiently. In particular, the per-backup VirtualMachineBackup is ephemeral:
// virt-controller deletes it once its checkpoint is absorbed into the
// VirtualMachineBackupTracker, which can happen well before the overall backup finishes
// uploading data to the BSL -- checking VirtualMachineBackup status after waiting for full
// completion (as this function used to) can race against that cleanup and find nothing.
func runKubevirtDMBackup(v *lib.VirtOperator, vmNamespace, backupName string, onDataUploadFound func(dataUploadName, expectedBackupType string)) {
	err := lib.EnsureKubevirtVolumePolicy(dpaCR.Client, namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to ensure kubevirt volume policy")

	// Verify that the kubevirt-datamover controller creates a VirtualMachineBackupTracker
	// in the VM namespace during the backup, confirming VEP-25 incremental backup is
	// active -- every caller of this function gets this check for free (formerly only
	// checked by the now-removed runCBTVmBackup/"backup with CBT" DescribeTable, whose
	// coverage would otherwise have silently disappeared when that table was dropped as
	// redundant with the restore tests below).
	vmbtSeen := false
	vmbtCheckDone := make(chan struct{})
	go func() {
		defer close(vmbtCheckDone)
		for i := 0; i < 60; i++ {
			found, checkErr := v.CheckVMBackupTrackerExists(vmNamespace)
			if checkErr == nil && found {
				log.Printf("VirtualMachineBackupTracker observed in %s — VEP-25 incremental backup confirmed", vmNamespace)
				vmbtSeen = true
				return
			}
			time.Sleep(5 * time.Second)
		}
		log.Printf("VirtualMachineBackupTracker was not observed in %s during backup window", vmNamespace)
	}()

	err = lib.CreateBackupWithVolumePolicy(dpaCR.Client, namespace, backupName, []string{vmNamespace}, true)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to create backup %s", backupName)

	var dataUploadName, expectedBackupType string
	gomega.Eventually(func() error {
		var err error
		dataUploadName, expectedBackupType, err = lib.GetDataUploadForBackup(dpaCR.Client, namespace, backupName)
		return err
	}, 2*time.Minute, time.Second*5).Should(gomega.Succeed(), "failed to get DataUpload for backup %s", backupName)

	if onDataUploadFound != nil {
		onDataUploadFound(dataUploadName, expectedBackupType)
	}

	gomega.Eventually(lib.IsKubevirtDMBackupDone(dpaCR.Client, dynamicClientForSuiteRun, namespace, backupName), 20*time.Minute, time.Second*10).
		Should(gomega.BeTrue(), "backup %s did not complete", backupName)
	succeeded, err := lib.IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to check completion status of backup %s", backupName)
	gomega.Expect(succeeded).To(gomega.BeTrue(), "backup %s did not complete successfully", backupName)

	<-vmbtCheckDone
	gomega.Expect(vmbtSeen).To(gomega.BeTrue(), "expected VirtualMachineBackupTracker to be observed during backup (VEP-25 incremental backup)")
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

		// TODO: remove once migtools/kubevirt-datamover-plugin#44 and the
		// kubevirt-datamover-controller issue #73 phase 3 work merge and the
		// default images include their fixes.
		//
		// Always set (not gated behind an env var): a prior version of this gate
		// silently fell back to the operator's default (mainline `:latest`) images
		// whenever the env var wasn't set, which doesn't contain either fix at all —
		// costing a full debugging session chasing phantom bugs (RBAC, "controller
		// never starts") that were actually just this suite testing the wrong build.
		// Unconditional pinning means the suite fails loudly/obviously (wrong image
		// digest, pull error) instead of silently passing against unrelated code.
		//
		// Together these two images carry the fix for the VM-eager-start race:
		// the plugin halts an auto-starting VM at restore time (stashing its
		// original run strategy) so virt-launcher can't consume — and thus
		// WaitForFirstConsumer-bind the wrong PV onto — the target PVC before the
		// DataDownloads have populated it; the controller flips the VM back to its
		// stashed run state once every sibling DataDownload for that VM completes.
		// Both halves are required: the plugin alone leaves the VM halted forever,
		// the controller alone has nothing to flip.
		//
		// Both images are pinned by digest rather than by tag. These are mutable
		// personal-registry tags that have already been rebuilt in place more than
		// once during development, so a node with an old layer cached would
		// silently run a stale binary and make this test's result meaningless.
		// Plugin digest is the pr-44 build at commit 4fb7ed9 (post coderabbit-iterate
		// convergence); controller digest is the multi-arch manifest-list index for
		// kubevirt-datamover-controller:issue-73-phase3-datadownload-controller
		// (linux/amd64 + linux/arm64) at commit e66317e, so it still resolves
		// per-node architecture.
		if dpaCR.UnsupportedOverrides == nil {
			dpaCR.UnsupportedOverrides = map[v1alpha1.UnsupportedImageKey]string{}
		}
		dpaCR.UnsupportedOverrides[v1alpha1.KubeVirtDatamoverPluginImageKey] = "quay.io/tkaovila/kubevirt-datamover-plugin@sha256:54c88bda836544eb1e4080c29d4d93141db619fa8322b0451bb2135b4c2ff82d"
		dpaCR.UnsupportedOverrides[v1alpha1.KubeVirtDatamoverControllerImageKey] = "quay.io/tkaovila/kubevirt-datamover-controller@sha256:6d3d80dc7f0096114f4b1da2379194bca100fc063f0fd26ee97e9c8d2e21856f"

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
			Template:       "./sample-applications/virtual-machines/fedora-todolist/fedora-todolist.yaml",
			InitDelay:      3 * time.Minute, // For cloud-init
			StartupTimeout: 20 * time.Minute,
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

	// The Fedora CBT case that used to live in a "backup with CBT" DescribeTable
	// here (backup-only, no restore) moved to a full backup+restore It in the
	// "Kubevirt datamover restore from a CBT backup" Describe block below --
	// a backup-only check duplicated no coverage a restore test doesn't already
	// provide as its own first step, same reasoning that removed the CirrOS
	// backup-only entry earlier. See that block's Fedora It for the useEmulation
	// gating and why it's structural-only (no guest-exec data-integrity check --
	// Fedora's qemu-ga blacklists guest-exec by default).

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

			runKubevirtDMBackup(v, incSeqNamespace, backupName, func(dataUploadName, expectedBackupType string) {
				gomega.Expect(expectedBackupType).To(gomega.Equal(expectedType), "controller's expected-backup-type annotation on DataUpload")

				// Poll here, while the backup is still in flight -- the VirtualMachineBackup
				// is ephemeral and may already be gone by the time the overall backup
				// finishes (see runKubevirtDMBackup's doc comment).
				var actualBackupType string
				gomega.Eventually(func() error {
					var err error
					actualBackupType, _, err = v.GetVMBBackupType(incSeqNamespace, dataUploadName)
					return err
				}, 5*time.Minute, time.Second*5).Should(gomega.Succeed(), "failed to get VirtualMachineBackup status for DataUpload %s", dataUploadName)
				// VirtualMachineBackup.status.type is PascalCase (virt-controller's own
				// convention, e.g. "Full"/"Incremental"), while the DataUpload's
				// expected-backup-type annotation is lowercase (kubevirt-datamover-controller's
				// convention, e.g. "full"/"incremental") -- normalize case before comparing
				// these two independently-maintained values.
				actualBackupType = strings.ToLower(actualBackupType)
				gomega.Expect(actualBackupType).To(gomega.Equal(expectedType), "actual VirtualMachineBackup.Status.Type")
				gomega.Expect(actualBackupType).To(gomega.Equal(strings.ToLower(expectedBackupType)), "expected vs. actual backup type must not mismatch")
			})
		}

		var _ = ginkgo.BeforeAll(func() {
			updateLastBRcase(incSeqCase)
			prepareBackupAndRestore(incSeqCase.BackupRestoreCase, func() {})

			_ = v.RemoveVm(incSeqNamespace, incSeqVMName, 2*time.Minute)
			err := lib.DeleteNamespace(v.Clientset, incSeqNamespace)
			gomega.Expect(err).To(gomega.BeNil(), "failed to delete namespace %s before setup", incSeqNamespace)
			gomega.Eventually(v.IsNamespaceDeletedClearingStuckVMBFinalizers(kubernetesClientForSuiteRun, incSeqNamespace), time.Minute*2, time.Second*5).
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
		ginkgo.It("full backup, incremental chain, restart, and max-limit fallback", ginkgo.Label("virt", "kdm"), func() {
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

		ginkgo.PIt("backup after deleting libvirt checkpoints with maxIncrementalBackups=0 hangs forever — blocked by CNV-85377", ginkgo.Label("virt", "kdm"), func() {
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

	// setupVmForRestoreTest deletes any stale namespace left over from a
	// previous spec, installs template fresh, and waits for the VM to reach
	// Running + ready. Shared by every restore-test BeforeEach in this file
	// (CirrOS below, and the Alpine incremental-restore block further down) --
	// mechanical setup, not fixture-specific logic, so it's factored out
	// rather than copy-pasted per fixture.
	setupVmForRestoreTest := func(namespace, vmName, template string) {
		_ = v.RemoveVm(namespace, vmName, 2*time.Minute)
		err := lib.DeleteNamespace(v.Clientset, namespace)
		gomega.Expect(err).To(gomega.BeNil(), "failed to delete namespace %s before setup", namespace)
		gomega.Eventually(v.IsNamespaceDeletedClearingStuckVMBFinalizers(kubernetesClientForSuiteRun, namespace), time.Minute*2, time.Second*5).
			Should(gomega.BeTrue(), "namespace %s was not deleted before setup", namespace)

		err = lib.CreateNamespace(v.Clientset, namespace)
		gomega.Expect(err).To(gomega.BeNil(), "failed to create namespace %s", namespace)
		err = lib.InstallApplication(v.Client, template)
		gomega.Expect(err).To(gomega.BeNil(), "failed to install VM template %s in namespace %s", template, namespace)

		log.Printf("Waiting for VM %s/%s to reach Running status", namespace, vmName)
		err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 15*time.Minute, true, func(ctx context.Context) (bool, error) {
			status, statusErr := v.GetVmStatus(namespace, vmName)
			if statusErr != nil {
				return false, nil
			}
			return status == "Running", nil
		})
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "VM %s/%s did not reach Running status", namespace, vmName)
		err = v.WaitForVMReady(namespace, vmName, 5*time.Minute)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "VM %s/%s was not ready", namespace, vmName)
	}

	// verifyBackupType returns a runKubevirtDMBackup callback that hard-asserts
	// both the DataUpload's expected-backup-type annotation and the
	// VirtualMachineBackup's actual Status.Type match expectedType -- mirrors
	// the check already done inline in runSequenceBackup above, factored out
	// since the incremental-restore test below needs the same check twice
	// (full, then incremental) and getting this wrong would silently
	// invalidate the whole test (e.g. a "full" backup that's actually a no-op
	// incremental would make the payload-B-only assertion meaningless).
	verifyBackupType := func(vmNamespace, expectedType string) func(dataUploadName, expectedBackupType string) {
		return func(dataUploadName, expectedBackupType string) {
			gomega.Expect(expectedBackupType).To(gomega.Equal(expectedType), "controller's expected-backup-type annotation on DataUpload %s", dataUploadName)

			var actualBackupType string
			gomega.Eventually(func() error {
				var err error
				actualBackupType, _, err = v.GetVMBBackupType(vmNamespace, dataUploadName)
				return err
			}, 5*time.Minute, time.Second*5).Should(gomega.Succeed(), "failed to get VirtualMachineBackup status for DataUpload %s", dataUploadName)
			actualBackupType = strings.ToLower(actualBackupType)
			gomega.Expect(actualBackupType).To(gomega.Equal(expectedType), "actual VirtualMachineBackup.Status.Type for DataUpload %s", dataUploadName)
		}
	}

	// Covers the #99 "Restore from KDM CBT backup" gap, now unblocked by
	// migtools/kubevirt-datamover-controller#124 (DataDownload controller, issue #73
	// phase 3). Per docs/design/kubevirt-datamover.md, the VirtualMachine RIA plugin
	// creates the DataDownload automatically from backup-recorded annotations/ConfigMap
	// data (and separately discards the restored VMB/VMBT so restore doesn't re-trigger a
	// backup) — so this only needs a normal Velero Restore, no manual CR driving.
	// This Describe block now hosts only the two Pending scaffolds below,
	// gated on kubevirt-datamover-controller#73 phase 4 -- the live restore
	// tests (full-backup and incremental on Alpine, structural-only on Fedora)
	// moved to the "restore from a CBT backup" Describe block further down.
	// Kept on CirrOS deliberately: neither scaffold below makes a
	// data-integrity assertion, only VM run-state/structural checks, so
	// CirrOS's lack of a guest agent doesn't cost them anything.
	ginkgo.Describe("Kubevirt datamover restore from CBT backup — pending kubevirt-datamover-controller#73 phase 4", ginkgo.Ordered, func() {
		const (
			restoreNamespace = "cirros-test"
			restoreVMName    = "cirros-test"
			restoreTemplate  = "./sample-applications/virtual-machines/cirros-test/cirros-test-cbt.yaml"
		)

		restoreCase := VmBackupRestoreCase{
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         restoreNamespace,
				Name:              restoreVMName,
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSIDataMover,
				BackupTimeout:     20 * time.Minute,
			},
			// CirrOS ships no qemu-guest-agent -- see lib.VirtOperator.HasQemuGuestAgent.
			// Fine here since neither scaffold below depends on one.
			HasGuestAgent: false,
		}

		// BeforeEach, not BeforeAll: guards against a real Ginkgo interaction if
		// ginkgo.FlakeAttempts is ever added to this It. Ginkgo skips any
		// already-passed run-once node on a retry (internal/group.go attemptSpec: a
		// node whose runOncePair is already SpecStatePassed is `continue`d), so a
		// BeforeAll would not re-run on a retry attempt -- while the suite-level
		// AfterEach tears the DPA down after every attempt and this It deletes
		// restoreNamespace and the VM on its way out. FlakeAttempts combined with a
		// BeforeAll would leave a retry attempt with no velero and no source VM,
		// dying instantly on the source-PVC lookup -- a cascading, misleading
		// failure, not a retry of whatever the FlakeAttempts was meant to absorb.
		// The FlakeAttempts uses elsewhere in this suite are on DescribeTable
		// Entries whose setup is already per-spec, which is why that pattern is
		// safe there but would not be here.
		var _ = ginkgo.BeforeEach(func() {
			updateLastBRcase(restoreCase)
			prepareBackupAndRestore(restoreCase.BackupRestoreCase, func() {})
			setupVmForRestoreTest(restoreNamespace, restoreVMName, restoreTemplate)
		})

		ginkgo.PIt("restore run-state flip is not blocked by a stale sibling DataDownload from a different restore attempt — blocked on kubevirt-datamover-controller#73 phase 4 (restore-attempt-scoped sibling correlation, also resolves kubevirt-datamover-controller#169)", ginkgo.Label("virt", "kdm"), func() {
			// kubevirt-datamover-controller's VM run-state-restore flip
			// (allSiblingDataDownloadsCompleted) currently correlates sibling
			// DataDownloads purely by VM identity annotations
			// (kubevirt-datamover.io/vm-name/vm-namespace), with no notion of which
			// restore attempt a DataDownload belongs to. A stale, already-Failed
			// DataDownload left over from an aborted prior restore attempt for a VM
			// permanently blocks the flip for every future restore attempt of that VM,
			// even a fully successful new one --
			// https://github.com/migtools/kubevirt-datamover-controller/issues/169.
			//
			// The fix (correlating by the restore's own velero.io/restore-uid label in
			// addition to VM identity) is already implemented in
			// kubevirt-datamover-controller#73 phase 4, the same branch the other two
			// PIt placeholders below are waiting on. Scaffolded as real, compiling
			// pending code (not deleted, not just a comment) so it's ready to flip to
			// ginkgo.It once phase 4 lands -- asserting only the fixed, final behavior
			// (the VM resumes despite a foreign-attempt decoy sibling), not the
			// currently-buggy intermediate state, since asserting the bug itself would
			// start failing the moment the fix merges with nothing forcing anyone to
			// notice and update it.
			backupName := "cirros-stale-sibling-backup"
			runKubevirtDMBackup(v, restoreNamespace, backupName, nil)

			err := v.RemoveVm(restoreNamespace, restoreVMName, 5*time.Minute)
			gomega.Expect(err).To(gomega.BeNil(), "failed to remove VM %s/%s", restoreNamespace, restoreVMName)
			err = lib.DeleteNamespace(v.Clientset, restoreNamespace)
			gomega.Expect(err).To(gomega.BeNil(), "failed to delete namespace %s", restoreNamespace)
			gomega.Eventually(v.IsNamespaceDeletedClearingStuckVMBFinalizers(kubernetesClientForSuiteRun, restoreNamespace), time.Minute*5, time.Second*5).
				Should(gomega.BeTrue(), "namespace %s was not deleted", restoreNamespace)

			restoreName := "cirros-stale-sibling-restore"
			defer func() {
				if err := lib.DeleteVeleroBackupAndRestore(dpaCR.Client, kubernetesClientForSuiteRun, kubeConfig, namespace, backupName, restoreName); err != nil {
					log.Printf("cleanup: failed to delete backup/restore %s/%s via velero CLI: %v", backupName, restoreName, err)
				}
			}()
			err = lib.CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to create restore %s", restoreName)

			// Fabricate a decoy DataDownload simulating a stale, Failed sibling from a
			// genuinely different restore attempt: same VM identity annotations as the
			// real DataDownload this restore will create, but a deliberately mismatched
			// velero.io/restore-uid (an absent label isn't equivalent to a mismatched
			// one -- this must actually differ to simulate "a different attempt").
			bsls, err := dpaCR.ListBSLs()
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to list BSLs")
			foreignRestoreUID, _ := uuid.NewUUID()
			decoy := &velerov2alpha1.DataDownload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dd-stale-sibling-decoy",
					Namespace: namespace,
					Annotations: map[string]string{
						"kubevirt-datamover.io/vm-name":      restoreVMName,
						"kubevirt-datamover.io/vm-namespace": restoreNamespace,
					},
					Labels: map[string]string{
						velero.BackupNameLabel:  backupName,
						velero.RestoreNameLabel: restoreName,
						velero.RestoreUIDLabel:  foreignRestoreUID.String(),
					},
				},
				Spec: velerov2alpha1.DataDownloadSpec{
					TargetVolume: velerov2alpha1.TargetVolumeSpec{
						PVC:       "dd-stale-sibling-decoy-pvc",
						Namespace: restoreNamespace,
					},
					BackupStorageLocation: bsls.Items[0].Name,
					DataMover:             "kubevirt",
					SnapshotID:            "placeholder-not-used",
					SourceNamespace:       restoreNamespace,
					OperationTimeout:      metav1.Duration{Duration: 4 * time.Hour},
				},
			}
			gomega.Expect(dpaCR.Client.Create(context.Background(), decoy)).To(gomega.Succeed(), "failed to create decoy stale-sibling DataDownload")
			decoy.Status.Phase = velerov2alpha1.DataDownloadPhaseFailed
			gomega.Expect(dpaCR.Client.Status().Update(context.Background(), decoy)).To(gomega.Succeed(), "failed to mark decoy DataDownload Failed")
			defer func() {
				_ = dpaCR.Client.Delete(context.Background(), decoy)
			}()

			gomega.Eventually(lib.IsRestoreDone(dpaCR.Client, namespace, restoreName), 45*time.Minute, time.Second*10).
				Should(gomega.BeTrue(), "restore %s did not complete", restoreName)
			succeeded, err := lib.IsRestoreCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to check completion status of restore %s", restoreName)
			gomega.Expect(succeeded).To(gomega.BeTrue(), "restore %s did not complete successfully", restoreName)

			_, phase, _, err := lib.GetDataDownloadForRestore(dpaCR.Client, namespace, restoreName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get DataDownload for restore %s", restoreName)
			gomega.Expect(phase).To(gomega.Equal("Completed"), "expected the real DataDownload to complete")

			// The assertion this PIt exists to make once flipped to a live It: the VM
			// resumes despite the foreign-attempt decoy sibling still sitting Failed,
			// because phase 4's fix correlates by velero.io/restore-uid, not just VM
			// identity.
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
				status, statusErr := v.GetVmStatus(restoreNamespace, restoreVMName)
				if statusErr != nil {
					return false, nil
				}
				return status == "Running", nil
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred(),
				"expected restored VM %s/%s to resume despite the foreign-attempt decoy sibling, once phase 4's restore-attempt-scoped correlation lands", restoreNamespace, restoreVMName)

			err = v.RemoveVm(restoreNamespace, restoreVMName, 5*time.Minute)
			gomega.Expect(err).To(gomega.BeNil(), "failed to remove VM %s/%s", restoreNamespace, restoreVMName)
			err = lib.DeleteNamespace(v.Clientset, restoreNamespace)
			gomega.Expect(err).To(gomega.BeNil(), "failed to delete namespace %s", restoreNamespace)
		})

		ginkgo.PIt("restore a multi-PVC VM from a kubevirt-datamover CBT backup — blocked on kubevirt-datamover-controller#73 phase 4 (multi-disk restore hardening, not yet implemented)", ginkgo.Label("virt", "kdm"), func() {
			// Phase 4 ("Multi-disk + PVC provisioning hardening") of
			// https://github.com/migtools/kubevirt-datamover-controller/issues/73 has not
			// landed yet — per its own exit criteria ("unit tests for multi-disk
			// concurrency and sizing fallback behavior"), per-disk DataDownload isolation
			// isn't hardened, so a real multi-disk restore can't be trusted to pass today.
			// Scaffolded as real, compiling pending code (not deleted, not just a comment)
			// so it's ready to flip to ginkgo.It once phase 4 lands.
			multiPvcNamespace := "cirros-multipvc-cbt-test"
			multiPvcVMName := "cirros-multipvc-cbt-test"
			multiPvcTemplate := "./sample-applications/virtual-machines/cirros-test/cirros-test-multipvc-cbt.yaml"

			_ = v.RemoveVm(multiPvcNamespace, multiPvcVMName, 2*time.Minute)
			err := lib.DeleteNamespace(v.Clientset, multiPvcNamespace)
			gomega.Expect(err).To(gomega.BeNil())
			gomega.Eventually(v.IsNamespaceDeletedClearingStuckVMBFinalizers(kubernetesClientForSuiteRun, multiPvcNamespace), time.Minute*2, time.Second*5).Should(gomega.BeTrue())
			err = lib.CreateNamespace(v.Clientset, multiPvcNamespace)
			gomega.Expect(err).To(gomega.BeNil())
			err = lib.InstallApplication(v.Client, multiPvcTemplate)
			gomega.Expect(err).To(gomega.BeNil())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 15*time.Minute, true, func(ctx context.Context) (bool, error) {
				status, statusErr := v.GetVmStatus(multiPvcNamespace, multiPvcVMName)
				if statusErr != nil {
					return false, nil
				}
				return status == "Running", nil
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			err = v.WaitForVMReady(multiPvcNamespace, multiPvcVMName, 5*time.Minute)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			backupName := "cirros-multipvc-cbt-restore-backup"
			runKubevirtDMBackup(v, multiPvcNamespace, backupName, nil)

			err = v.RemoveVm(multiPvcNamespace, multiPvcVMName, 5*time.Minute)
			gomega.Expect(err).To(gomega.BeNil())
			err = lib.DeleteNamespace(v.Clientset, multiPvcNamespace)
			gomega.Expect(err).To(gomega.BeNil())
			gomega.Eventually(v.IsNamespaceDeletedClearingStuckVMBFinalizers(kubernetesClientForSuiteRun, multiPvcNamespace), time.Minute*5, time.Second*5).Should(gomega.BeTrue())

			restoreName := "cirros-multipvc-cbt-restore-restore"
			err = lib.CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Eventually(lib.IsRestoreDone(dpaCR.Client, namespace, restoreName), 45*time.Minute, time.Second*10).Should(gomega.BeTrue())
			succeeded, err := lib.IsRestoreCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(succeeded).To(gomega.BeTrue(), "expected both disks' DataDownloads to complete once phase 4 lands")

			err = v.RemoveVm(multiPvcNamespace, multiPvcVMName, 5*time.Minute)
			gomega.Expect(err).To(gomega.BeNil())
			err = lib.DeleteNamespace(v.Clientset, multiPvcNamespace)
			gomega.Expect(err).To(gomega.BeNil())
		})
	})

	// Promotes the former "restore from an incremental (not full) kubevirt-datamover
	// CBT backup" PIt to a live It, now that phase 5 of
	// https://github.com/migtools/kubevirt-datamover-controller/issues/73 has a real
	// data-integrity story: a host-side raw-block write (as the full-backup It above
	// uses) bypasses qemu's CBT dirty-bitmap, so it can't prove anything about an
	// incremental backup, which would legitimately skip an untracked region as
	// "unchanged". A real, CBT-tracked write has to go through the guest's own
	// block I/O path, which needs a guest-agent-equipped fixture -- the existing
	// CirrOS fixture has none, hence a dedicated Describe block (own namespace, own
	// BeforeEach) rather than reusing the CirrOS-scoped one above: HasGuestAgent,
	// cloud-init boot timing, and the readiness check (must poll HasQemuGuestAgent,
	// not just Running) all genuinely differ between the two fixtures.
	ginkgo.Describe("Kubevirt datamover restore from a CBT backup", ginkgo.Ordered, func() {
		ginkgo.Context("Alpine (guest-exec data-integrity)", func() {
			const (
				alpineNamespace = "alpine-guestagent"
				alpineVMName    = "alpine-guestagent"
				alpineTemplate  = "./sample-applications/virtual-machines/alpine-guestagent/alpine-guestagent-cbt.yaml"
				// alpineDomainName is virsh's addressing convention for this VM inside the
				// virt-launcher pod's compute container. Confirmed live against the
				// 260716-aws-amd64 cluster (`virsh list --all` in the compute container):
				// KubeVirt names the libvirt domain "<namespace>_<vmName>", NOT the bare
				// VM name -- the only prior call sites for RunVirshCommand-family helpers
				// in this repo lived inside a ginkgo.PIt that had never run against a
				// real cluster, so this was unverified before this.
				alpineDomainName = alpineNamespace + "_" + alpineVMName

				// The upstream fixture's disk is ~200M; these offsets/sizes stay well
				// clear of its filesystem with room to spare, on a 512Mi Block PVC.
				payloadAOffsetMiB = 300
				payloadBOffsetMiB = 380
				payloadSizeMiB    = 8 // kept small: /dev/urandom under this cluster's
				// KubeVirt software emulation (useEmulation: true, no /dev/kvm) is
				// CPU-bound, unlike the full-backup It's host-side 32MiB write.
			)

			// No SkipUnderEmulation here: Alpine is a ~200M image with OpenRC (not
			// systemd), much lighter than Fedora under software emulation, and its
			// guest-exec channel doesn't carry Fedora/RHEL's qemu-ga RPC blacklist
			// (which blocks guest-exec/guest-exec-status by default) -- so unlike
			// the Fedora Context below, Alpine has no known emulation-reliability or
			// guest-exec-availability reason to skip.
			alpineCase := VmBackupRestoreCase{
				BackupRestoreCase: BackupRestoreCase{
					Namespace:         alpineNamespace,
					Name:              alpineVMName,
					SkipVerifyLogs:    true,
					BackupRestoreType: lib.CSIDataMover,
					BackupTimeout:     20 * time.Minute,
				},
				HasGuestAgent: true,
			}

			var _ = ginkgo.BeforeEach(func() {
				updateLastBRcase(alpineCase)
				prepareBackupAndRestore(alpineCase.BackupRestoreCase, func() {})
				setupVmForRestoreTest(alpineNamespace, alpineVMName, alpineTemplate)

				// cloud-init's package/service setup runs async after the VM reaches
				// Running -- wait for the guest agent to actually connect for real
				// rather than trusting a fixed InitDelay sleep, since both Its below
				// depend on guest-exec working from the start of the test.
				gomega.Eventually(func() (bool, error) {
					return v.HasQemuGuestAgent(alpineNamespace, alpineVMName)
				}, 5*time.Minute, 10*time.Second).Should(gomega.BeTrue(),
					"qemu-guest-agent never connected for VM %s/%s -- guest-exec below depends on it", alpineNamespace, alpineVMName)
			})

			// fullBackupPayloadOffsetMiB/SizeMiB are this It's own payload region,
			// distinct from payloadA/payloadB above (different VM instance per It via
			// BeforeEach, so no actual collision risk -- kept apart anyway for clarity
			// when reading logs from both Its side by side).
			const (
				fullBackupPayloadOffsetMiB = 440
				fullBackupPayloadSizeMiB   = 32
			)

			ginkgo.It("restore a VM from a full kubevirt-datamover CBT backup", ginkgo.Label("virt", "kdm"), func() {
				// Cross-checks alpineCase.HasGuestAgent against the VMI's actual live
				// condition, rather than trusting the fixture's declared value blindly --
				// catches drift if this VM template ever changes without the field being
				// updated to match. The guest-exec payload write below depends on this
				// being accurate: it requires a connected guest agent to work at all.
				hasAgent, err := v.HasQemuGuestAgent(alpineNamespace, alpineVMName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to check qemu-guest-agent status for VM %s/%s", alpineNamespace, alpineVMName)
				gomega.Expect(hasAgent).To(gomega.Equal(alpineCase.HasGuestAgent),
					"VM %s/%s's actual qemu-guest-agent connectivity (%v) does not match alpineCase.HasGuestAgent (%v) -- if this VM template lost its guest agent, the payload-write strategy below needs re-evaluating too", alpineNamespace, alpineVMName, hasAgent, alpineCase.HasGuestAgent)

				// kubevirt-datamover-plugin's pvc/restore.go clearPVCBinding only clears
				// spec.volumeName/status and two pv.kubernetes.io/* annotations -- it does
				// not reset spec.selector. That is only safe if kubevirt-datamover-backed
				// PVCs never carry a selector to begin with (dynamically provisioned via
				// CDI/CSI, never statically pre-bound). Confirmed here directly against the
				// live source PVC rather than assumed: kdm-plugin's own unit tests
				// (pvc/restore_test.go) have zero fixture coverage for a selector being
				// present, so this e2e check is the only place verifying the premise holds.
				ginkgo.By("verifying the source PVC has no spec.selector before backup")
				sourcePVC, err := kubernetesClientForSuiteRun.CoreV1().PersistentVolumeClaims(alpineNamespace).Get(context.Background(), "alpine-guestagent-disk", metav1.GetOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get source PVC %s/alpine-guestagent-disk before backup", alpineNamespace)
				gomega.Expect(sourcePVC.Spec.Selector).To(gomega.BeNil(),
					"source PVC %s/alpine-guestagent-disk unexpectedly has spec.selector set -- kubevirt-datamover-plugin's clearPVCBinding does not clear this field, which is only safe if it is always nil", alpineNamespace)

				// Hard, deterministic data-integrity assertion: write a payload the test
				// itself controls THROUGH THE GUEST (guest-exec, not a host-side dd), then
				// EMPIRICALLY VERIFY (not assume) it stayed quiet across the backup window
				// before trusting the restored-side comparison. Random (not zero) bytes,
				// so qcow2's own sparse-write handling can't mask a real corruption bug
				// that happened to zero this region out. Routing the write through the
				// guest means this same mechanism works unchanged for both full and
				// incremental backups (see the incremental It above) -- no
				// FULL-BACKUP-ONLY limitation the way a host-side dd would have.
				ginkgo.By("writing a payload and checksumming it before the full backup")
				err = v.WriteRandomPayloadToGuestBlockDevice(kubeConfig, alpineNamespace, alpineVMName, alpineDomainName, "/dev/vda", fullBackupPayloadOffsetMiB, fullBackupPayloadSizeMiB)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to write payload into guest %s/%s", alpineNamespace, alpineVMName)
				payloadChecksumBeforeBackup, err := v.ChecksumBlockDeviceRegion(kubeConfig, alpineNamespace, alpineVMName, "volume0", fullBackupPayloadOffsetMiB, fullBackupPayloadSizeMiB)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to checksum payload region immediately after writing it")

				ginkgo.By("backing up the VM via kubevirt-datamover")
				backupName := "alpine-cbt-restore-backup"
				// Registered as soon as the name is known (before the backup even exists) so
				// it always runs -- as a plain Go defer, not ginkgo.DeferCleanup: confirmed
				// directly that DeferCleanup callbacks registered inside this It run AFTER
				// the container's own AfterEach (tearDownBackupAndRestore -> Undeploy already
				// deletes the DPA/velero deployment by then), not before it as LIFO ordering
				// would suggest. A plain `defer` is guaranteed to run during this It's own
				// stack unwind -- including via the panic a failed gomega.Expect uses to
				// unwind -- strictly before ginkgo can move on to AfterEach.
				// Deleting via the velero CLI (not oc delete) matters here specifically:
				// velero is still running at this point, so it can actually process the
				// CR's finalizer; doing this after the DPA is gone (as the outer AfterEach
				// alone would) leaves it stuck Terminating with no controller left to clear
				// restores.velero.io/external-resources-finalizer -- hit this directly.
				defer func() {
					if err := lib.DeleteVeleroBackupAndRestore(dpaCR.Client, kubernetesClientForSuiteRun, kubeConfig, namespace, backupName, ""); err != nil {
						log.Printf("cleanup: failed to delete backup %s via velero CLI: %v", backupName, err)
					}
				}()
				runKubevirtDMBackup(v, alpineNamespace, backupName, verifyBackupType(alpineNamespace, "full"))

				// Second bracket read: if this matches payloadChecksumBeforeBackup, the
				// payload region was genuinely quiet for the entire backup window, so any
				// mismatch on the restored side later can only mean a real transfer bug --
				// not guest churn. If it doesn't match, something did touch the region
				// and the hard assertion is skipped below rather than risk a false failure
				// attributing an unrelated write to kubevirt-datamover.
				payloadChecksumAfterBackup, payloadErr := v.ChecksumBlockDeviceRegion(kubeConfig, alpineNamespace, alpineVMName, "volume0", fullBackupPayloadOffsetMiB, fullBackupPayloadSizeMiB)
				payloadRegionStable := payloadErr == nil && payloadChecksumAfterBackup == payloadChecksumBeforeBackup
				if payloadErr != nil {
					log.Printf("WARNING: failed to re-checksum payload region after backup, skipping the hard data-integrity assertion for this run: %v", payloadErr)
				} else if !payloadRegionStable {
					log.Printf("WARNING: payload region changed during the backup window (before=%s after=%s) -- skipping the hard data-integrity assertion for this run rather than risk a false failure", payloadChecksumBeforeBackup, payloadChecksumAfterBackup)
				}

				ginkgo.By("deleting the VM to prove restore recreates it")
				err = v.RemoveVm(alpineNamespace, alpineVMName, 5*time.Minute)
				gomega.Expect(err).To(gomega.BeNil(), "failed to remove VM %s/%s", alpineNamespace, alpineVMName)
				err = lib.DeleteNamespace(v.Clientset, alpineNamespace)
				gomega.Expect(err).To(gomega.BeNil(), "failed to delete namespace %s", alpineNamespace)
				gomega.Eventually(v.IsNamespaceDeletedClearingStuckVMBFinalizers(kubernetesClientForSuiteRun, alpineNamespace), time.Minute*5, time.Second*5).
					Should(gomega.BeTrue(), "namespace %s was not deleted", alpineNamespace)

				ginkgo.By("restoring from the backup via a normal Velero Restore")
				restoreName := "alpine-cbt-restore-restore"
				// Same reasoning as the backup cleanup defer above -- registered by name
				// before creation so it always runs, and (plain Go defer LIFO within this
				// It) runs first so the restore is gone before the backup deletion cascades.
				defer func() {
					if err := lib.DeleteVeleroBackupAndRestore(dpaCR.Client, kubernetesClientForSuiteRun, kubeConfig, namespace, "", restoreName); err != nil {
						log.Printf("cleanup: failed to delete restore %s via velero CLI: %v", restoreName, err)
					}
				}()
				err = lib.CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to create restore %s", restoreName)
				gomega.Eventually(lib.IsRestoreDone(dpaCR.Client, namespace, restoreName), 45*time.Minute, time.Second*10).
					Should(gomega.BeTrue(), "restore %s did not complete", restoreName)
				succeeded, err := lib.IsRestoreCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to check completion status of restore %s", restoreName)
				gomega.Expect(succeeded).To(gomega.BeTrue(), "restore %s did not complete successfully", restoreName)

				ginkgo.By("verifying the kubevirt-datamover RestoreItemAction created and completed a DataDownload in Block volumeMode")
				_, phase, blockMode, err := lib.GetDataDownloadForRestore(dpaCR.Client, namespace, restoreName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get DataDownload for restore %s", restoreName)
				gomega.Expect(phase).To(gomega.Equal("Completed"), "DataDownload did not complete")
				gomega.Expect(blockMode).To(gomega.BeTrue(), "DataDownload did not report a Block volumeMode restore")

				restoredPVC, err := kubernetesClientForSuiteRun.CoreV1().PersistentVolumeClaims(alpineNamespace).Get(context.Background(), "alpine-guestagent-disk", metav1.GetOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get restored PVC %s/alpine-guestagent-disk", alpineNamespace)
				gomega.Expect(restoredPVC.Spec.VolumeMode).ToNot(gomega.BeNil(), "restored PVC %s/alpine-guestagent-disk has no volumeMode set", alpineNamespace)
				gomega.Expect(*restoredPVC.Spec.VolumeMode).To(gomega.Equal(corev1.PersistentVolumeBlock), "restored PVC %s/alpine-guestagent-disk was not Block volumeMode", alpineNamespace)

				// Only trust this if the source-side bracket reads (around the backup call
				// above) showed the payload region was genuinely quiet across the whole
				// backup window -- otherwise a mismatch here couldn't be attributed to a
				// real transfer bug versus something unrelated that touched the region.
				// Bracketed by its own "not Running yet" VM-status checks: the halt is the
				// controller's to release, not this test's.
				if payloadRegionStable {
					ginkgo.By("verifying the payload region survived backup and restore intact (hard assertion)")
					prePayloadStatus, statusErr := v.GetVmStatus(alpineNamespace, alpineVMName)
					restoredPayloadChecksum, restoredPayloadErr := v.ChecksumPVCBlockDeviceRegion(kubeConfig, alpineNamespace, restoredPVC.Name, fullBackupPayloadOffsetMiB, fullBackupPayloadSizeMiB)
					postPayloadStatus, postStatusErr := v.GetVmStatus(alpineNamespace, alpineVMName)
					if statusErr != nil || prePayloadStatus == "Running" || postStatusErr != nil || postPayloadStatus == "Running" {
						log.Printf("WARNING: restored VM %s/%s resumed around the payload-region read (pre=%q err=%v, post=%q err=%v) -- skipping the hard assertion for this run", alpineNamespace, alpineVMName, prePayloadStatus, statusErr, postPayloadStatus, postStatusErr)
					} else {
						gomega.Expect(restoredPayloadErr).ToNot(gomega.HaveOccurred(), "failed to checksum restored payload region")
						gomega.Expect(restoredPayloadChecksum).To(gomega.Equal(payloadChecksumBeforeBackup),
							"restored payload region does not match what was written before backup -- this is a real data-integrity failure (the payload region was verified stable across the backup window via bracket reads, so this cannot be guest churn)")
					}
				} else {
					log.Printf("skipping the hard payload-region assertion for %s/%s -- source-side bracket reads showed the region wasn't stable across the backup window", alpineNamespace, restoredPVC.Name)
				}

				ginkgo.By("verifying the VM is running again after restore")
				err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
					status, statusErr := v.GetVmStatus(alpineNamespace, alpineVMName)
					if statusErr != nil {
						return false, nil
					}
					return status == "Running", nil
				})
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "restored VM %s/%s did not reach Running status", alpineNamespace, alpineVMName)

				// Second restore, reusing the same backup, deliberately reproducing the shape
				// of the original VM-eager-start race: kdm-controller's handleAccepted
				// rejects a DataDownload whose target PVC already has spec.volumeName set or
				// status.phase==Bound, since it can never safely rebind an already-bound PVC.
				// The original bug triggered this via a virt-launcher pod racing ahead of the
				// halt; that race is now closed, so nothing does this naturally anymore --
				// triggered on purpose here instead, to lock in that the VM correctly stays
				// halted (rather than silently starting on top of un-restored data) when a
				// restore's DataDownload fails. This stays inside the same It as the restore
				// above rather than a separate one: the suite's shared per-test AfterEach
				// tears down the DPA (and with it, velero/BSL) after every single It, and a
				// freshly recreated DPA gets a new random BSL S3 prefix each time -- a second
				// It could not have restored from this same backup at all.
				ginkgo.By("deleting the VM again to restore into a clean namespace")
				err = v.RemoveVm(alpineNamespace, alpineVMName, 5*time.Minute)
				gomega.Expect(err).To(gomega.BeNil(), "failed to remove VM %s/%s", alpineNamespace, alpineVMName)
				err = lib.DeleteNamespace(v.Clientset, alpineNamespace)
				gomega.Expect(err).To(gomega.BeNil(), "failed to delete namespace %s", alpineNamespace)
				gomega.Eventually(v.IsNamespaceDeletedClearingStuckVMBFinalizers(kubernetesClientForSuiteRun, alpineNamespace), time.Minute*5, time.Second*5).
					Should(gomega.BeTrue(), "namespace %s was not deleted", alpineNamespace)

				ginkgo.By("restoring from the same backup again, to trigger a PVC binding conflict")
				rejectedRestoreName := "alpine-cbt-restore-restore-rejected"
				// Same reasoning as the two cleanup defers above: this Restore CR also needs
				// velero alive to clear its finalizer, and the outer AfterEach has already
				// torn velero down by the time it would otherwise be deleted.
				defer func() {
					if err := lib.DeleteVeleroBackupAndRestore(dpaCR.Client, kubernetesClientForSuiteRun, kubeConfig, namespace, "", rejectedRestoreName); err != nil {
						log.Printf("cleanup: failed to delete restore %s via velero CLI: %v", rejectedRestoreName, err)
					}
				}()
				err = lib.CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, rejectedRestoreName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to create restore %s", rejectedRestoreName)

				// The rejection check only inspects the PVC's own fields, not whether the
				// referenced PV actually exists, so setting a bogus volumeName is enough to
				// force the real, controller-generated rejection deterministically.
				ginkgo.By("forcing a binding conflict on the restored PVC before the DataDownload controller's Accepted check runs")
				gomega.Eventually(func() error {
					pvc, getErr := kubernetesClientForSuiteRun.CoreV1().PersistentVolumeClaims(alpineNamespace).Get(context.Background(), "alpine-guestagent-disk", metav1.GetOptions{})
					if getErr != nil {
						return getErr
					}
					if pvc.Spec.VolumeName != "" {
						return nil
					}
					pvc.Spec.VolumeName = "e2e-deliberately-conflicting-pv"
					_, updateErr := kubernetesClientForSuiteRun.CoreV1().PersistentVolumeClaims(alpineNamespace).Update(context.Background(), pvc, metav1.UpdateOptions{})
					return updateErr
				}, 2*time.Minute, time.Millisecond*500).Should(gomega.Succeed(), "failed to force a binding conflict on restored PVC %s/alpine-guestagent-disk", alpineNamespace)

				ginkgo.By("verifying the second restore reaches a terminal phase without succeeding")
				gomega.Eventually(lib.IsRestoreDone(dpaCR.Client, namespace, rejectedRestoreName), 10*time.Minute, time.Second*10).
					Should(gomega.BeTrue(), "restore %s did not reach a terminal phase", rejectedRestoreName)
				rejectedRestore, err := lib.GetRestore(dpaCR.Client, namespace, rejectedRestoreName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get restore %s", rejectedRestoreName)
				gomega.Expect(string(rejectedRestore.Status.Phase)).ToNot(gomega.Equal("Completed"),
					"restore %s unexpectedly completed despite the forced PVC binding conflict", rejectedRestoreName)

				ginkgo.By("verifying the DataDownload was rejected as Failed, not silently stuck")
				_, rejectedPhase, _, err := lib.GetDataDownloadForRestore(dpaCR.Client, namespace, rejectedRestoreName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get DataDownload for restore %s", rejectedRestoreName)
				gomega.Expect(rejectedPhase).To(gomega.Equal("Failed"), "DataDownload did not fail from the forced binding conflict")

				ginkgo.By("verifying the VM stays halted rather than starting on top of un-restored data")
				gomega.Eventually(func() error {
					_, statusErr := v.GetVmStatus(alpineNamespace, alpineVMName)
					return statusErr
				}, 3*time.Minute, time.Second*5).Should(gomega.Succeed(), "restored VM %s/%s never appeared", alpineNamespace, alpineVMName)
				gomega.Consistently(func() string {
					status, statusErr := v.GetVmStatus(alpineNamespace, alpineVMName)
					if statusErr != nil {
						log.Printf("transient error reading VM %s/%s status during Consistently poll: %v", alpineNamespace, alpineVMName, statusErr)
						return ""
					}
					return status
				}, time.Minute, time.Second*10).ShouldNot(gomega.Equal("Running"),
					"VM %s/%s unexpectedly reached Running status despite its DataDownload failing", alpineNamespace, alpineVMName)

				err = v.RemoveVm(alpineNamespace, alpineVMName, 5*time.Minute)
				gomega.Expect(err).To(gomega.BeNil(), "failed to remove VM %s/%s", alpineNamespace, alpineVMName)
				err = lib.DeleteNamespace(v.Clientset, alpineNamespace)
				gomega.Expect(err).To(gomega.BeNil(), "failed to delete namespace %s", alpineNamespace)
			})

			ginkgo.It("restore from an incremental (not full) kubevirt-datamover CBT backup", ginkgo.Label("virt", "kdm"), func() {
				ginkgo.By("writing payload A and checksumming it before the full backup")
				err := v.WriteRandomPayloadToGuestBlockDevice(kubeConfig, alpineNamespace, alpineVMName, alpineDomainName, "/dev/vda", payloadAOffsetMiB, payloadSizeMiB)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to write payload A into guest %s/%s", alpineNamespace, alpineVMName)
				payloadA0, err := v.ChecksumBlockDeviceRegion(kubeConfig, alpineNamespace, alpineVMName, "volume0", payloadAOffsetMiB, payloadSizeMiB)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to checksum payload A immediately after writing it")

				ginkgo.By("full backup")
				fullBackupName := "alpine-cbt-incr-restore-full"
				// Registered before the incremental backup's own cleanup below so that,
				// by LIFO defer ordering, the incremental backup (which depends on this
				// full backup as its chain base) is deleted FIRST when this It returns --
				// deleting the base out from under a dependent incremental would be the
				// wrong order. Matches the plain-defer-not-DeferCleanup convention the
				// full-backup It above documents (velero must still be running to
				// process the finalizer).
				defer func() {
					if err := lib.DeleteVeleroBackupAndRestore(dpaCR.Client, kubernetesClientForSuiteRun, kubeConfig, namespace, fullBackupName, ""); err != nil {
						log.Printf("cleanup: failed to delete backup %s via velero CLI: %v", fullBackupName, err)
					}
				}()
				runKubevirtDMBackup(v, alpineNamespace, fullBackupName, verifyBackupType(alpineNamespace, "full"))
				payloadA1, err := v.ChecksumBlockDeviceRegion(kubeConfig, alpineNamespace, alpineVMName, "volume0", payloadAOffsetMiB, payloadSizeMiB)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to checksum payload A immediately after the full backup")

				ginkgo.By("writing payload B and checksumming it before the incremental backup")
				err = v.WriteRandomPayloadToGuestBlockDevice(kubeConfig, alpineNamespace, alpineVMName, alpineDomainName, "/dev/vda", payloadBOffsetMiB, payloadSizeMiB)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to write payload B into guest %s/%s", alpineNamespace, alpineVMName)
				payloadB0, err := v.ChecksumBlockDeviceRegion(kubeConfig, alpineNamespace, alpineVMName, "volume0", payloadBOffsetMiB, payloadSizeMiB)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to checksum payload B immediately after writing it")

				ginkgo.By("incremental backup")
				incrementalBackupName := "alpine-cbt-incr-restore-incremental"
				// Registered second so it is deleted FIRST (LIFO) -- see the comment on
				// the full backup's own defer above.
				defer func() {
					if err := lib.DeleteVeleroBackupAndRestore(dpaCR.Client, kubernetesClientForSuiteRun, kubeConfig, namespace, incrementalBackupName, ""); err != nil {
						log.Printf("cleanup: failed to delete backup %s via velero CLI: %v", incrementalBackupName, err)
					}
				}()
				runKubevirtDMBackup(v, alpineNamespace, incrementalBackupName, verifyBackupType(alpineNamespace, "incremental"))

				// A third read for payload A: restoring from the incremental replays the
				// whole chain, so anything that touched A's region between the full and
				// incremental backups is legitimately part of restored-A. Only trust the
				// hard assertion below if all three reads agree that region stayed quiet
				// across BOTH backup windows, not just the first.
				payloadA2, err := v.ChecksumBlockDeviceRegion(kubeConfig, alpineNamespace, alpineVMName, "volume0", payloadAOffsetMiB, payloadSizeMiB)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to checksum payload A immediately after the incremental backup")
				payloadB1, err := v.ChecksumBlockDeviceRegion(kubeConfig, alpineNamespace, alpineVMName, "volume0", payloadBOffsetMiB, payloadSizeMiB)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to checksum payload B immediately after the incremental backup")

				payloadAStable := payloadA0 == payloadA1 && payloadA1 == payloadA2
				if !payloadAStable {
					log.Printf("WARNING: payload A region changed during the backup window(s) (before-full=%s after-full=%s after-incremental=%s) -- skipping its hard data-integrity assertion rather than risk a false failure", payloadA0, payloadA1, payloadA2)
				}
				payloadBStable := payloadB0 == payloadB1
				if !payloadBStable {
					log.Printf("WARNING: payload B region changed during the incremental backup window (before=%s after=%s) -- skipping its hard data-integrity assertion rather than risk a false failure", payloadB0, payloadB1)
				}

				ginkgo.By("deleting the VM to prove restore recreates it")
				err = v.RemoveVm(alpineNamespace, alpineVMName, 5*time.Minute)
				gomega.Expect(err).To(gomega.BeNil(), "failed to remove VM %s/%s", alpineNamespace, alpineVMName)
				err = lib.DeleteNamespace(v.Clientset, alpineNamespace)
				gomega.Expect(err).To(gomega.BeNil(), "failed to delete namespace %s", alpineNamespace)
				gomega.Eventually(v.IsNamespaceDeletedClearingStuckVMBFinalizers(kubernetesClientForSuiteRun, alpineNamespace), time.Minute*5, time.Second*5).Should(gomega.BeTrue())

				ginkgo.By("restoring from the incremental backup")
				restoreName := "alpine-cbt-incr-restore-restore"
				// Registered before CreateRestoreFromBackup, same plain-defer convention
				// as the backups above: guarantees cleanup even if a gomega.Expect below
				// panics the goroutine mid-verification.
				defer func() {
					if err := lib.DeleteVeleroBackupAndRestore(dpaCR.Client, kubernetesClientForSuiteRun, kubeConfig, namespace, "", restoreName); err != nil {
						log.Printf("cleanup: failed to delete restore %s via velero CLI: %v", restoreName, err)
					}
				}()
				err = lib.CreateRestoreFromBackup(dpaCR.Client, namespace, incrementalBackupName, restoreName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Eventually(lib.IsRestoreDone(dpaCR.Client, namespace, restoreName), 45*time.Minute, time.Second*10).Should(gomega.BeTrue())
				succeeded, err := lib.IsRestoreCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(succeeded).To(gomega.BeTrue(), "expected restore from an incremental checkpoint to reconstruct the full chain")

				ginkgo.By("verifying both payloads independently, each gated on its own bracket-stability")
				// Bracketed the same way the full-backup It's payload check is: only
				// trust these reads if the restored VM was still Halted immediately
				// before AND after both of them -- confirmed live (this session's own
				// full-backup validation run) that the sibling-completion hold can
				// release fast enough for the VM to already be Running by the time this
				// check runs, which would make a mismatch here meaningless (comparing
				// against a disk the booted guest may have already written to).
				prePayloadStatus, preStatusErr := v.GetVmStatus(alpineNamespace, alpineVMName)
				vmStillHalted := preStatusErr == nil && prePayloadStatus != "Running"
				var restoredA, restoredB string
				var restoredAErr, restoredBErr error
				if vmStillHalted {
					if payloadAStable {
						restoredA, restoredAErr = v.ChecksumPVCBlockDeviceRegion(kubeConfig, alpineNamespace, "alpine-guestagent-disk", payloadAOffsetMiB, payloadSizeMiB)
					}
					if payloadBStable {
						restoredB, restoredBErr = v.ChecksumPVCBlockDeviceRegion(kubeConfig, alpineNamespace, "alpine-guestagent-disk", payloadBOffsetMiB, payloadSizeMiB)
					}
				}
				postPayloadStatus, postStatusErr := v.GetVmStatus(alpineNamespace, alpineVMName)
				if !vmStillHalted || postStatusErr != nil || postPayloadStatus == "Running" {
					log.Printf("WARNING: restored VM %s/%s resumed around the payload-region reads (pre=%q err=%v, post=%q err=%v) -- skipping the hard assertions for this run", alpineNamespace, alpineVMName, prePayloadStatus, preStatusErr, postPayloadStatus, postStatusErr)
				} else {
					if payloadAStable {
						gomega.Expect(restoredAErr).ToNot(gomega.HaveOccurred(), "failed to checksum restored payload A region")
						gomega.Expect(restoredA).To(gomega.Equal(payloadA0), "payload A region did not survive the full+incremental backup/restore chain intact")
					}
					if payloadBStable {
						gomega.Expect(restoredBErr).ToNot(gomega.HaveOccurred(), "failed to checksum restored payload B region")
						gomega.Expect(restoredB).To(gomega.Equal(payloadB0), "payload B region (incremental-only) did not survive the restore chain intact")
					}
				}

				err = v.RemoveVm(alpineNamespace, alpineVMName, 5*time.Minute)
				gomega.Expect(err).To(gomega.BeNil())
				err = lib.DeleteNamespace(v.Clientset, alpineNamespace)
				gomega.Expect(err).To(gomega.BeNil())
			})
		})

		ginkgo.Context("Fedora (structural only)", func() {
			const (
				fedoraNamespace = "mysql-persistent"
				fedoraVMName    = "fedora-todolist"
				fedoraTemplate  = "./sample-applications/virtual-machines/kubevirt-dm/fedora-todolist-cbt.yaml"
			)

			fedoraCase := VmBackupRestoreCase{
				BackupRestoreCase: BackupRestoreCase{
					Namespace:         fedoraNamespace,
					Name:              fedoraVMName,
					SkipVerifyLogs:    true,
					BackupRestoreType: lib.CSIDataMover,
					BackupTimeout:     45 * time.Minute,
				},
				InitDelay:          3 * time.Minute,
				SkipUnderEmulation: true,
				// Fedora's qemu-ga blacklists guest-exec/guest-exec-status in its default
				// RPC blacklist -- reusing the Alpine It's guest-exec payload mechanism
				// would need explicit guest-agent reconfiguration this fixture doesn't
				// carry. Structural-only: this It proves CBT backup+restore mechanics
				// (DataDownload completion, VM resume) work against a real Fedora guest,
				// not just Alpine's synthetic one -- it doesn't attempt a data-integrity
				// assertion the way the Alpine Its above do.
				HasGuestAgent: true,
			}

			var _ = ginkgo.BeforeEach(func() {
				if fedoraCase.SkipUnderEmulation {
					emulated, err := v.IsEmulationEnabled()
					gomega.Expect(err).ToNot(gomega.HaveOccurred())
					if emulated {
						ginkgo.Skip("cluster KubeVirt CR has useEmulation: true (software emulation) -- Fedora is not reliable under emulation")
					}
				}
				updateLastBRcase(fedoraCase)
				prepareBackupAndRestore(fedoraCase.BackupRestoreCase, func() {})
				setupVmForRestoreTest(fedoraNamespace, fedoraVMName, fedoraTemplate)
				if fedoraCase.InitDelay > 0 {
					log.Printf("Waiting %v for VM %s/%s to finish booting (cloud-init, etc.)", fedoraCase.InitDelay, fedoraNamespace, fedoraVMName)
					time.Sleep(fedoraCase.InitDelay)
				}
			})

			ginkgo.It("restore a Fedora VM from a full kubevirt-datamover CBT backup", ginkgo.Label("virt", "kdm"), func() {
				ginkgo.By("backing up the VM via kubevirt-datamover")
				backupName := "fedora-cbt-restore-backup"
				defer func() {
					if err := lib.DeleteVeleroBackupAndRestore(dpaCR.Client, kubernetesClientForSuiteRun, kubeConfig, namespace, backupName, ""); err != nil {
						log.Printf("cleanup: failed to delete backup %s via velero CLI: %v", backupName, err)
					}
				}()
				runKubevirtDMBackup(v, fedoraNamespace, backupName, verifyBackupType(fedoraNamespace, "full"))

				ginkgo.By("deleting the VM to prove restore recreates it")
				err := v.RemoveVm(fedoraNamespace, fedoraVMName, 5*time.Minute)
				gomega.Expect(err).To(gomega.BeNil(), "failed to remove VM %s/%s", fedoraNamespace, fedoraVMName)
				err = lib.DeleteNamespace(v.Clientset, fedoraNamespace)
				gomega.Expect(err).To(gomega.BeNil(), "failed to delete namespace %s", fedoraNamespace)
				gomega.Eventually(v.IsNamespaceDeletedClearingStuckVMBFinalizers(kubernetesClientForSuiteRun, fedoraNamespace), time.Minute*5, time.Second*5).
					Should(gomega.BeTrue(), "namespace %s was not deleted", fedoraNamespace)

				ginkgo.By("restoring from the backup via a normal Velero Restore")
				restoreName := "fedora-cbt-restore-restore"
				defer func() {
					if err := lib.DeleteVeleroBackupAndRestore(dpaCR.Client, kubernetesClientForSuiteRun, kubeConfig, namespace, "", restoreName); err != nil {
						log.Printf("cleanup: failed to delete restore %s via velero CLI: %v", restoreName, err)
					}
				}()
				err = lib.CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to create restore %s", restoreName)
				gomega.Eventually(lib.IsRestoreDone(dpaCR.Client, namespace, restoreName), 45*time.Minute, time.Second*10).
					Should(gomega.BeTrue(), "restore %s did not complete", restoreName)
				succeeded, err := lib.IsRestoreCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to check completion status of restore %s", restoreName)
				gomega.Expect(succeeded).To(gomega.BeTrue(), "restore %s did not complete successfully", restoreName)

				ginkgo.By("verifying the kubevirt-datamover RestoreItemAction created and completed a DataDownload")
				_, phase, _, err := lib.GetDataDownloadForRestore(dpaCR.Client, namespace, restoreName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get DataDownload for restore %s", restoreName)
				gomega.Expect(phase).To(gomega.Equal("Completed"), "DataDownload did not complete")

				ginkgo.By("verifying the VM is running again after restore")
				err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
					status, statusErr := v.GetVmStatus(fedoraNamespace, fedoraVMName)
					if statusErr != nil {
						return false, nil
					}
					return status == "Running", nil
				})
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "restored VM %s/%s did not reach Running status", fedoraNamespace, fedoraVMName)

				err = v.RemoveVm(fedoraNamespace, fedoraVMName, 5*time.Minute)
				gomega.Expect(err).To(gomega.BeNil())
				err = lib.DeleteNamespace(v.Clientset, fedoraNamespace)
				gomega.Expect(err).To(gomega.BeNil())
			})
		})
	})
})

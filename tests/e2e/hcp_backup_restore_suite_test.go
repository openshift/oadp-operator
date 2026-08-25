package e2e_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
	libhcp "github.com/openshift/oadp-operator/tests/e2e/lib/hcp"
)

type HCBackupRestoreMode string

const (
	HCModeCreate       HCBackupRestoreMode = "create"        // Create new HostedCluster for test
	HCModeExternal     HCBackupRestoreMode = "external"      // Get external HostedCluster
	HCModeExternalROSA HCBackupRestoreMode = "external-rosa" // Get external HostedCluster for ROSA where DPA and some other resources are already installed
)

// runHCPBackupAndRestore is the unified function that handles both create and external HC modes
func runHCPBackupAndRestore(
	brCase HCPBackupRestoreCase,
	updateLastBRcase func(HCPBackupRestoreCase),
	updateLastInstallTime func(),
	h *libhcp.HCHandler,
) {
	var err error
	updateLastBRcase(brCase)
	updateLastInstallTime()

	log.Printf("Preparing backup and restore")

	backupUid, _ := uuid.NewUUID()
	restoreUid, _ := uuid.NewUUID()
	backupName := fmt.Sprintf("%s-%s", brCase.Name, backupUid.String())
	restoreName := fmt.Sprintf("%s-%s", brCase.Name, restoreUid.String())

	oadpDeploymentOperation := NewOADPDeploymentOperationDefault()
	if brCase.Mode == HCModeExternalROSA {
		oadpDeploymentOperation = NewOADPDeploymentOperationROSA()
	}
	oadpDeploymentOperation.Deploy(brCase.BackupRestoreType)

	// Ensure that an existing backup repository is deleted
	err = lib.DeleteBackupRepositories(runTimeClientForSuiteRun, namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// For ROSA the DPA is managed by ManifestWork in service cluster and would be reverted back.
	if brCase.Mode != HCModeExternalROSA {
		err := h.AddHCPPluginToDPA(dpaCR.Namespace, dpaCR.Name, false)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to add HCP plugin to DPA: %v", err)
		// TODO: move the wait for HC just after the DPA modification to allow reconciliation to go ahead without waiting for the HC to be created
		// Wait for HCP plugin to be added
		gomega.Eventually(libhcp.IsHCPPluginAdded(h.Client, dpaCR.Namespace, dpaCR.Name), 3*time.Minute, 1*time.Second).Should(gomega.BeTrue())
	}

	h.HCPNamespace = libhcp.GetHCPNamespace(brCase.BackupRestoreCase.Name, hcNamespace)

	// Unified HostedCluster setup
	switch brCase.Mode {
	case HCModeCreate:
		// Create new HostedCluster for test
		h.HostedCluster, err = h.DeployHCManifest(brCase.Template, brCase.Provider, brCase.BackupRestoreCase.Name, hcNamespace)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	case HCModeExternal, HCModeExternalROSA:
		// Get existing HostedCluster
		h.HostedCluster, err = h.GetHostedCluster(brCase.BackupRestoreCase.Name, hcNamespace)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	default:
		ginkgo.Fail(fmt.Sprintf("unknown HCP mode: %s", brCase.Mode))
	}

	// Pre-backup verification
	if brCase.PreBackupVerify != nil {
		log.Printf("Validating HC pre-backup")
		err := brCase.PreBackupVerify(runTimeClientForSuiteRun, "" /*unused*/)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run HCP pre-backup verification: %v", err)
	}

	if brCase.Mode == HCModeExternal || brCase.Mode == HCModeExternalROSA {
		// Pre-backup verification for guest cluster
		if brCase.PreBackupVerifyGuest != nil {
			log.Printf("Validating guest cluster pre-backup")
			hcKubeconfig, err := h.GetHostedClusterKubeconfig(h.HostedCluster)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			crClientForHC, err := client.New(hcKubeconfig, client.Options{Scheme: lib.Scheme})
			gomega.Eventually(h.ValidateClient(crClientForHC), 5*time.Minute, 2*time.Second).Should(gomega.BeTrue())
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			err = brCase.PreBackupVerifyGuest(crClientForHC, "" /*unused*/)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run pre-backup verification for guest cluster: %v", err)
		}
	}

	// Backup HCP & HC
	log.Printf("Backing up HC")
	includedResources := libhcp.HCPIncludedResources
	excludedResources := libhcp.HCPExcludedResources
	includedNamespaces := []string{hcNamespace, libhcp.GetHCPNamespace(h.HostedCluster.Name, hcNamespace)}

	nsRequiresResticDCWorkaround := runHCPBackup(brCase.BackupRestoreCase, backupName, h, includedNamespaces, includedResources, excludedResources)

	// Delete everything in HCP namespace
	log.Printf("Deleting HCP & HC")
	switch brCase.Mode {
	case HCModeExternalROSA:
		err = h.BackupManifestWork()
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to backup ManifestWork: %v", err)
		// For ROSA the DPA is managed by ManifestWork in service cluster.
		// Need to delete the ManifestWork.
		err = h.DeleteManifestWork(libhcp.Wait30Min)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to delete ManifestWork: %v", err)
	default:
		err = h.RemoveHCP(libhcp.Wait10Min)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to remove HCP: %v", err)
	}

	// Restore HC
	log.Printf("Restoring HC")
	runHCPRestore(brCase.BackupRestoreCase, backupName, restoreName, nsRequiresResticDCWorkaround)

	// Unified post-restore verification
	if brCase.PostRestoreVerify != nil {
		log.Printf("Validating HC post-restore")
		err = brCase.PostRestoreVerify(runTimeClientForSuiteRun, "" /*unused*/)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run HCP post-restore verification: %v", err)
	}

	if brCase.Mode == HCModeExternal || brCase.Mode == HCModeExternalROSA {
		// Post-restore verification for guest cluster
		if brCase.PostRestoreVerifyGuest != nil {
			log.Printf("Validating guest cluster post-restore")
			hcKubeconfig, err := h.GetHostedClusterKubeconfig(h.HostedCluster)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			crClientForHC, err := client.New(hcKubeconfig, client.Options{Scheme: lib.Scheme})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Eventually(h.ValidateClient(crClientForHC), libhcp.ValidateHCPTimeout, libhcp.WaitForNextCheckTimeout).Should(gomega.BeTrue())
			err = brCase.PostRestoreVerifyGuest(crClientForHC, "" /*unused*/)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run post-restore verification for guest cluster: %v", err)
		}
	}
}

type VerificationFunctionGuest func(client.Client, string) error

type HCPBackupRestoreCase struct {
	BackupRestoreCase
	Mode                   HCBackupRestoreMode
	PreBackupVerifyGuest   VerificationFunctionGuest
	PostRestoreVerifyGuest VerificationFunctionGuest
	Template               string // Optional: only used when Mode == HCPModeCreate
	Provider               string // Optional: only used when Mode == HCPModeCreate
}

var _ = ginkgo.Describe("HCP Backup and Restore tests", ginkgo.Ordered, func() {
	var (
		lastInstallTime time.Time
		lastBRCase      HCPBackupRestoreCase
		h               *libhcp.HCHandler
		ctx             = context.Background()
	)

	updateLastBRcase := func(brCase HCPBackupRestoreCase) {
		lastBRCase = brCase
	}

	updateLastInstallTime := func() {
		lastInstallTime = time.Now()
	}

	// Before All
	var _ = ginkgo.BeforeAll(func() {
		// Wait for CatalogSource to be ready
		err := libhcp.WaitForCatalogSourceReady(
			ctx,
			runTimeClientForSuiteRun,
			libhcp.RHOperatorsNamespace,
			libhcp.OCPMarketplaceNamespace,
			time.Minute*5,
		)
		if err != nil {
			ginkgo.Fail(fmt.Sprintf("HCP tests failed: CatalogSource not ready timeout: %v", err))
			return
		}

		// Wait for multicluster-engine PackageManifest to be available
		err = libhcp.WaitForPackageManifest(
			ctx,
			runTimeClientForSuiteRun,
			libhcp.MCEName,
			libhcp.OCPMarketplaceNamespace,
			time.Minute*5,
		)
		if err != nil {
			ginkgo.Fail(fmt.Sprintf("HCP tests failed: multicluster-engine PackageManifest not available timeout: %v", err))
			return
		}

		// Below OCP 4.19, MCE's current default channel (stable-2.11) can't be used --
		// MCE 2.11's backplane-operator refuses to reconcile there (a floor enforced in
		// code, not visible via OLM/catalog metadata; see OADP-8716). SelectMCEChannel
		// pins to stable-2.8 in that case, matching the ACM 2.13 / MCE 2.8 pairing the
		// real GovCloud HCP target environment runs on OCP 4.18 (OADP-7564) -- and
		// leaves the channel unset (floating on MCE's default) on OCP 4.19+, where HCP
		// e2e also runs (4.22/4.23/5.0 periodics) and MCE 2.11 installs fine. Check with
		// the OADP/HCP team from time to time (Brae Troutman, as of this writing) for
		// updated ACM/MCE version requirements.
		//
		// Production also pins the HyperShift operator image itself to a specific SHA
		// (passed to the MCE hypershift addon), rather than relying on whatever image
		// ships bundled with this MCE version. That's done via an admin-controlled
		// ConfigMap named "hypershift-override-images" in the multicluster-engine
		// namespace (keyed by image-stream name -- see stolostron/hypershift-addon-operator's
		// pkg/util/constant.go, HypershiftOverrideImagesCM/ImageStreamHypershiftOperator),
		// separate from the tenant-writable hypershift-operator-install-flags ConfigMap
		// the CVE-2026-66808 fix locked down. Not replicated here yet -- if a SHA pin
		// becomes necessary for this test too, source the value from an env var at
		// runtime rather than hardcoding it (the real pinned image used in production
		// isn't something to commit to a public repo).
		mceChannel, err := libhcp.SelectMCEChannel(ctx, runTimeClientForSuiteRun)
		if err != nil {
			ginkgo.Fail(fmt.Sprintf("HCP tests failed: could not determine MCE channel for this cluster's OCP version: %v", err))
			return
		}

		reqOperators := []libhcp.RequiredOperator{
			{
				Name:          libhcp.MCEName,
				Namespace:     libhcp.MCENamespace,
				Channel:       mceChannel,
				OperatorGroup: libhcp.MCEOperatorGroup,
			},
		}

		// Install MCE and Hypershift operators
		h, err = libhcp.InstallRequiredOperators(ctx, runTimeClientForSuiteRun, reqOperators)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(h).ToNot(gomega.BeNil())
		gomega.Eventually(lib.IsDeploymentReady(h.Client, libhcp.MCENamespace, libhcp.MCEOperatorName), libhcp.Wait10Min, time.Second*5).Should(gomega.BeTrue(), func() string {
			libhcp.DumpHypershiftDiagnostics(ctx, h.Client, kubernetesClientForSuiteRun)
			return "MCE operator deployment never became ready"
		})

		// Deploy the MCE manifest
		err = h.DeployMCEManifest()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		// Deploy the MCE and wait for it to be ready
		gomega.Eventually(lib.IsDeploymentReady(h.Client, libhcp.MCENamespace, libhcp.MCEOperatorName), libhcp.Wait10Min, time.Second*5).Should(gomega.BeTrue(), func() string {
			libhcp.DumpHypershiftDiagnostics(ctx, h.Client, kubernetesClientForSuiteRun)
			return "MCE operator deployment never became ready after applying MCE manifest"
		})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		// Validate the Hypershift operator.
		gomega.Eventually(lib.IsDeploymentReady(h.Client, libhcp.HONamespace, libhcp.HypershiftOperatorName), libhcp.Wait10Min, time.Second*5).Should(gomega.BeTrue(), func() string {
			libhcp.DumpHypershiftDiagnostics(ctx, h.Client, kubernetesClientForSuiteRun)
			return "Hypershift operator deployment never became ready"
		})
	})

	// After All
	var _ = ginkgo.AfterAll(func() {
		if h != nil {
			err := h.RemoveHCP(libhcp.Wait10Min)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to remove HCP: %v", err)
		}
	})

	// After Each
	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		if h != nil {
			h.RemoveHCP(libhcp.Wait10Min)
		}
		tearDownBackupAndRestore(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
	})

	ginkgo.DescribeTable("Basic HCP backup and restore test",
		func(brCase HCPBackupRestoreCase, expectedErr error) {
			if ginkgo.CurrentSpecReport().NumAttempts > 1 && !knownFlake {
				ginkgo.Fail("No known FLAKE found in a previous run, marking test as failed.")
			}
			runHCPBackupAndRestore(brCase, updateLastBRcase, updateLastInstallTime, h)
		},

		// Test Cases
		ginkgo.Entry("None HostedCluster backup and restore", ginkgo.Label("hcp"), HCPBackupRestoreCase{
			Mode:     HCModeCreate,
			Template: libhcp.HCPNoneManifest,
			Provider: "None",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         libhcp.GetHCPNamespace(fmt.Sprintf("%s-none", libhcp.HostedClusterPrefix), libhcp.ClustersNamespace),
				Name:              fmt.Sprintf("%s-none", libhcp.HostedClusterPrefix),
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(fmt.Sprintf("%s-none", libhcp.HostedClusterPrefix), libhcp.ClustersNamespace)),
				PostRestoreVerify: libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(fmt.Sprintf("%s-none", libhcp.HostedClusterPrefix), libhcp.ClustersNamespace)),
				BackupTimeout:     libhcp.HCPBackupTimeout,
			},
		}, nil),

		ginkgo.Entry("Agent HostedCluster backup and restore", ginkgo.Label("hcp"), HCPBackupRestoreCase{
			Mode:     HCModeCreate,
			Template: libhcp.HCPAgentManifest,
			Provider: "Agent",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         libhcp.GetHCPNamespace(fmt.Sprintf("%s-agent", libhcp.HostedClusterPrefix), libhcp.ClustersNamespace),
				Name:              fmt.Sprintf("%s-agent", libhcp.HostedClusterPrefix),
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(fmt.Sprintf("%s-agent", libhcp.HostedClusterPrefix), libhcp.ClustersNamespace)),
				PostRestoreVerify: libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(fmt.Sprintf("%s-agent", libhcp.HostedClusterPrefix), libhcp.ClustersNamespace)),
				BackupTimeout:     libhcp.HCPBackupTimeout,
			},
		}, nil),
	)
})

// TODO: Modify the runBackup function to inject the filtered error logs to avoid repeating code with this
func runHCPBackup(brCase BackupRestoreCase, backupName string, h *libhcp.HCHandler, namespaces []string, includedResources, excludedResources []string) bool {
	nsRequiresResticDCWorkaround, err := lib.NamespaceRequiresResticDCWorkaround(h.Client, brCase.Namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// create backup
	log.Printf("Creating backup %s for case %s", backupName, brCase.Name)
	err = lib.CreateCustomBackupForNamespaces(h.Client, namespace, backupName, namespaces, includedResources, excludedResources, brCase.BackupRestoreType == lib.KOPIA, brCase.BackupRestoreType == lib.CSIDataMover)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// wait for backup to not be running
	gomega.Eventually(lib.IsBackupDone(h.Client, namespace, backupName), brCase.BackupTimeout, time.Second*10).Should(gomega.BeTrue())
	// TODO only log on fail?
	describeBackup := lib.DescribeBackup(h.Client, namespace, backupName)
	ginkgo.GinkgoWriter.Println(describeBackup)

	backupLogs := lib.BackupLogs(kubernetesClientForSuiteRun, h.Client, namespace, backupName)
	backupErrorLogs := lib.BackupErrorLogs(kubernetesClientForSuiteRun, h.Client, namespace, backupName)
	accumulatedTestLogs = append(accumulatedTestLogs, describeBackup, backupLogs)

	// Check error logs for non-relevant errors
	filteredBackupErrorLogs := libhcp.FilterErrorLogs(backupErrorLogs)

	if !brCase.SkipVerifyLogs {
		gomega.Expect(filteredBackupErrorLogs).Should(gomega.Equal([]string{}))
	}

	// check if backup succeeded
	succeeded, err := lib.IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, h.Client, namespace, backupName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(succeeded).To(gomega.Equal(true))
	log.Printf("Backup for case %s succeeded", brCase.Name)

	if brCase.BackupRestoreType == lib.CSI {
		// wait for volume snapshot to be Ready
		gomega.Eventually(lib.AreVolumeSnapshotsReady(h.Client, backupName), time.Minute*4, time.Second*10).Should(gomega.BeTrue())
	}

	return nsRequiresResticDCWorkaround
}

// TODO: Modify the runRestore function to inject the filtered error logs to avoid repeating code with this
func runHCPRestore(brCase BackupRestoreCase, backupName string, restoreName string, nsRequiresResticDCWorkaround bool) {
	log.Printf("Creating restore %s for case %s", restoreName, brCase.Name)
	err := lib.CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Eventually(lib.IsRestoreDone(dpaCR.Client, namespace, restoreName), time.Minute*60, time.Second*10).Should(gomega.BeTrue())
	// TODO only log on fail?
	describeRestore := lib.DescribeRestore(dpaCR.Client, namespace, restoreName)
	ginkgo.GinkgoWriter.Println(describeRestore)

	restoreLogs := lib.RestoreLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	restoreErrorLogs := lib.RestoreErrorLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
	accumulatedTestLogs = append(accumulatedTestLogs, describeRestore, restoreLogs)

	// Check error logs for non-relevant errors
	filteredRestoreErrorLogs := libhcp.FilterErrorLogs(restoreErrorLogs)

	if !brCase.SkipVerifyLogs {
		gomega.Expect(filteredRestoreErrorLogs).Should(gomega.Equal([]string{}))
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

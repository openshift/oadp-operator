package e2e_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	ginkgov2 "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

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
	Source          string
	SourceNamespace string
}

func runVmBackupAndRestore(brCase VmBackupRestoreCase, expectedErr error, updateLastBRcase func(brCase VmBackupRestoreCase), updateLastInstallTime func(), v *lib.VirtOperator) {
	updateLastBRcase(brCase)

	// Create DPA
	backupName, restoreName := prepareBackupAndRestore(brCase.BackupRestoreCase, func() {})

	err := lib.CreateNamespace(v.Clientset, brCase.Namespace)
	gomega.Expect(err).To(gomega.BeNil())

	// Create VM from clone of CirrOS image
	err = v.CloneDisk(brCase.SourceNamespace, brCase.Source, brCase.Namespace, brCase.Name, 5*time.Minute)
	gomega.Expect(err).To(gomega.BeNil())

	err = v.CreateVm(brCase.Namespace, brCase.Name, 5*time.Minute)
	gomega.Expect(err).To(gomega.BeNil())

	// Remove the Data Volume, but keep the PVC attached to the VM
	err = v.DetachPvc(brCase.Namespace, brCase.Name, 2*time.Minute)
	gomega.Expect(err).To(gomega.BeNil())
	err = v.RemoveDataVolume(brCase.Namespace, brCase.Name, 2*time.Minute)
	gomega.Expect(err).To(gomega.BeNil())

	// Back up VM
	nsRequiresResticDCWorkaround := runBackup(brCase.BackupRestoreCase, backupName)

	// Delete everything in test namespace
	err = v.RemoveVm(brCase.Namespace, brCase.Name, 2*time.Minute)
	gomega.Expect(err).To(gomega.BeNil())
	err = v.RemovePvc(brCase.Namespace, brCase.Name, 2*time.Minute)
	gomega.Expect(err).To(gomega.BeNil())
	err = lib.DeleteNamespace(v.Clientset, brCase.Namespace)
	gomega.Expect(err).To(gomega.BeNil())
	gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, brCase.Namespace), timeoutMultiplier*time.Minute*4, time.Second*5).Should(gomega.BeTrue())

	// Do restore
	runRestore(brCase.BackupRestoreCase, backupName, restoreName, nsRequiresResticDCWorkaround)
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

		ginkgov2.Entry("default virtual machine backup and restore", ginkgov2.Label("virt"), VmBackupRestoreCase{
			Source:          "cirros-dv",
			SourceNamespace: "openshift-cnv",
			BackupRestoreCase: BackupRestoreCase{
				Namespace:         "cirros-test-vm",
				Name:              "cirros-vm",
				SkipVerifyLogs:    true,
				BackupRestoreType: lib.CSIDataMover,
			},
		}, nil),
	)
})

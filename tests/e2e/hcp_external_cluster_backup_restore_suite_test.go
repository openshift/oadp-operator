package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
	libhcp "github.com/openshift/oadp-operator/tests/e2e/lib/hcp"
)

const (
	testNamespace = "test"
	testPodName   = "test-pod-oadp-8924"
)

// External cluster backup and restore tests will skip creating HostedCluster resource. They expect the cluster
// to already have HostedCluster with a data plane.
// The tests are skipped unless hc_backup_restore_mode flag is properly configured.
var _ = ginkgo.Describe("HCP external cluster Backup and Restore tests", ginkgo.Ordered, func() {
	var (
		lastInstallTime time.Time
		lastBRCase      HCPBackupRestoreCase
		h               *libhcp.HCHandler
	)

	updateLastBRcase := func(brCase HCPBackupRestoreCase) {
		lastBRCase = brCase
	}

	updateLastInstallTime := func() {
		lastInstallTime = time.Now()
	}

	var _ = ginkgo.BeforeAll(func() {
		if HCBackupRestoreMode(hcBackupRestoreMode) != HCModeExternal &&
			HCBackupRestoreMode(hcBackupRestoreMode) != HCModeExternalROSA {
			ginkgo.Skip("Skipping HCP full backup and restore test for non-existent HCP")
		}

		h = &libhcp.HCHandler{
			Ctx:                  context.Background(),
			Client:               runTimeClientForSuiteRun,
			ClientServiceCluster: crClientForServiceCluster,
			HCOCPTestImage:       libhcp.HCOCPTestImage,
		}
	})

	// After Each
	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		if slices.Contains(ginkgo.CurrentSpecReport().Labels(), "hcp_guest") {
			return
		}
		gatherLogs(lastBRCase.BackupRestoreCase, lastInstallTime, ctx.SpecReport())
		oadpDeploymentOperation := NewOADPDeploymentOperationDefault()
		if HCBackupRestoreMode(hcBackupRestoreMode) == HCModeExternalROSA {
			oadpDeploymentOperation = NewOADPDeploymentOperationROSA()
		}
		oadpDeploymentOperation.Undeploy(lastBRCase.BackupRestoreCase.BackupRestoreType)
	})

	ginkgo.It("HCP external cluster backup and restore test", ginkgo.Label("hcp_external"), func() {
		if ginkgo.CurrentSpecReport().NumAttempts > 1 && !knownFlake {
			ginkgo.Fail("No known FLAKE found in a previous run, marking test as failed.")
		}

		runHCPBackupAndRestore(HCPBackupRestoreCase{
			Mode:                   HCBackupRestoreMode(hcBackupRestoreMode),
			PreBackupVerifyGuest:   preBackupVerifyGuest(),
			PostRestoreVerifyGuest: postBackupVerifyGuest(),
			BackupRestoreCase: BackupRestoreCase{
				Name:              hcName,
				BackupRestoreType: lib.CSIDataMover,
				PreBackupVerify:   libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(hcName, hcNamespace)),
				PostRestoreVerify: libhcp.ValidateHCP(libhcp.ValidateHCPTimeout, libhcp.Wait10Min, []string{}, libhcp.GetHCPNamespace(hcName, hcNamespace)),
				BackupTimeout:     libhcp.HCPBackupTimeout,
			},
		}, updateLastBRcase, updateLastInstallTime, h)
	})

	ginkgo.It("HCP external cluster guest pod backup test (OADP-8924)", ginkgo.Label("hcp_external", "hcp_guest"), func() {
		if ginkgo.CurrentSpecReport().NumAttempts > 1 && !knownFlake {
			ginkgo.Fail("No known FLAKE found in a previous run, marking test as failed.")
		}

		gomega.Expect(hcName).NotTo(gomega.BeEmpty(), "the hosted cluster name is required")
		var err error
		h.HostedCluster, err = h.GetHostedCluster(hcName, hcNamespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		guestConfig, err := h.GetHostedClusterKubeconfig(h.HostedCluster)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		guestClient, err := client.New(guestConfig, client.Options{Scheme: lib.Scheme})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Eventually(h.ValidateClient(guestClient), 5*time.Minute, 2*time.Second).Should(gomega.BeTrue())
		guestKubeClient, err := kubernetes.NewForConfig(guestConfig)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		apiserverConfig := &corev1.ConfigMap{}
		err = guestClient.Get(context.Background(), client.ObjectKey{Namespace: "openshift-apiserver", Name: "config"}, apiserverConfig)
		gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue(), "guest cluster unexpectedly has openshift-apiserver/config: %v", err)

		gomega.Expect(lib.CreateNamespace(guestKubeClient, namespace)).To(gomega.Succeed())
		credentials, err := lib.ReadFile(bslCredFile)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		guestSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: bslSecretName, Namespace: namespace},
			Data:       map[string][]byte{"cloud": credentials},
			Type:       corev1.SecretTypeOpaque,
		}
		_, err = guestKubeClient.CoreV1().Secrets(namespace).Create(context.Background(), guestSecret, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			err = nil
		} else if err == nil {
			ginkgo.DeferCleanup(func() {
				gomega.Expect(lib.DeleteSecret(guestKubeClient, namespace, bslSecretName)).To(gomega.Succeed())
			})
		}
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		guestDPA := *dpaCR
		guestDPA.Client = guestClient
		guestDPA.SnapshotLocations = nil
		guestDPA.VeleroDefaultPlugins = nil
		for _, plugin := range dpaCR.VeleroDefaultPlugins {
			if plugin == "openshift" || plugin == "aws" {
				guestDPA.VeleroDefaultPlugins = append(guestDPA.VeleroDefaultPlugins, plugin)
			}
		}
		gomega.Expect(guestDPA.VeleroDefaultPlugins).To(gomega.ContainElement("openshift"))
		gomega.Expect(guestDPA.CreateOrUpdate(guestDPA.Build(lib.KOPIA))).To(gomega.Succeed())
		ginkgo.DeferCleanup(func() {
			gomega.Expect(guestDPA.Delete()).To(gomega.Succeed())
		})
		gomega.Eventually(guestDPA.IsReconciledTrue(), 10*time.Minute, 5*time.Second).Should(gomega.BeTrue())
		gomega.Eventually(lib.VeleroPodIsRunning(guestKubeClient, namespace), 10*time.Minute, 5*time.Second).Should(gomega.BeTrue())
		gomega.Eventually(guestDPA.BSLsAreAvailable(), 5*time.Minute, 5*time.Second).Should(gomega.BeTrue())

		gomega.Expect(createTestNamespace(guestClient)).To(gomega.Succeed())
		gomega.Expect(createTestPod(guestClient)).To(gomega.Succeed())
		backupName := "hcp-guest-" + uuid.NewString()
		gomega.Expect(lib.CreateBackupForNamespaces(guestClient, namespace, backupName, []string{testNamespace}, false, false)).To(gomega.Succeed())
		gomega.Eventually(lib.IsBackupDone(guestClient, namespace, backupName), 30*time.Minute, 10*time.Second).Should(gomega.BeTrue())
		backupLogs := lib.BackupLogs(guestKubeClient, guestClient, namespace, backupName)
		gomega.Expect(backupLogs).NotTo(gomega.BeEmpty())
		gomega.Expect(backupLogs).To(gomega.ContainSubstring(testPodName))
		gomega.Expect(backupLogs).NotTo(gomega.ContainSubstring("configmaps \"config\" not found"))
		succeeded, err := lib.IsBackupCompletedSuccessfully(guestKubeClient, guestClient, namespace, backupName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(succeeded).To(gomega.BeTrue())
		gomega.Expect(validateTestPod(guestClient)).To(gomega.Succeed())
	})
})

func preBackupVerifyGuest() VerificationFunctionGuest {
	return func(crClientGuest client.Client, _ string) error {
		var errs []error
		errs = append(errs, createTestNamespace(crClientGuest))
		// Add more verifications here if needed
		return errors.Join(errs...)
	}
}

func postBackupVerifyGuest() VerificationFunctionGuest {
	return func(crClientGuest client.Client, _ string) error {
		var errs []error
		errs = append(errs, validateTestNamespace(crClientGuest))
		// Add more verifications here if needed
		return errors.Join(errs...)
	}
}

func createTestNamespace(crClientGuest client.Client) error {
	ns := &corev1.Namespace{}
	ns.Name = testNamespace
	err := crClientGuest.Create(context.Background(), ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func validateTestNamespace(crClientGuest client.Client) error {
	ns := &corev1.Namespace{}
	err := crClientGuest.Get(context.Background(), client.ObjectKey{Name: testNamespace}, ns)
	if err != nil {
		return err
	}
	return nil
}

// createTestPod creates a simple workload pod in the test namespace on the
// guest cluster. When this pod is included in a Velero backup, the
// openshift-velero-plugin backup action calls GetRegistryInfo(), which reads the
// ConfigMap "config" from the openshift-apiserver namespace. On HCP guest
// clusters that ConfigMap is absent, triggering the bug fixed in OADP-8924.
func createTestPod(crClientGuest client.Client) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNamespace,
			Labels: map[string]string{
				"app":       "oadp-e2e-test",
				"test-case": "oadp-8924",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "sleep",
					Image:   "registry.access.redhat.com/ubi9/ubi-minimal:latest",
					Command: []string{"/bin/sh", "-c", "sleep infinity"},
				},
			},
		},
	}
	createCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := crClientGuest.Create(createCtx, pod)
	if apierrors.IsAlreadyExists(err) {
		existing := &corev1.Pod{}
		getCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if getErr := crClientGuest.Get(getCtx, client.ObjectKeyFromObject(pod), existing); getErr != nil {
			return fmt.Errorf("failed to inspect existing test pod %s/%s: %w", testNamespace, testPodName, getErr)
		}
		if existing.Labels["app"] != pod.Labels["app"] ||
			existing.Labels["test-case"] != pod.Labels["test-case"] ||
			len(existing.Spec.Containers) != len(pod.Spec.Containers) ||
			existing.Spec.Containers[0].Image != pod.Spec.Containers[0].Image {
			return fmt.Errorf("existing pod %s/%s is not the expected test workload", testNamespace, testPodName)
		}
	} else if err != nil {
		return fmt.Errorf("failed to create test pod %s/%s: %w", testNamespace, testPodName, err)
	}

	// Wait for the pod to be running so the backup encounters a live pod.
	gomega.Eventually(func() bool {
		p := &corev1.Pod{}
		getCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if getErr := crClientGuest.Get(getCtx, client.ObjectKey{
			Name:      testPodName,
			Namespace: testNamespace,
		}, p); getErr != nil {
			return false
		}
		return p.Status.Phase == corev1.PodRunning
	}, 5*time.Minute, 10*time.Second).Should(gomega.BeTrue(),
		"test pod %s/%s did not reach Running phase", testNamespace, testPodName)

	return nil
}

// validateTestPod verifies the test pod exists on the guest cluster after
// restore.
func validateTestPod(crClientGuest client.Client) error {
	pod := &corev1.Pod{}
	getCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := crClientGuest.Get(getCtx, client.ObjectKey{
		Name:      testPodName,
		Namespace: testNamespace,
	}, pod)
	if err != nil {
		return fmt.Errorf("failed to validate test pod %s/%s after restore: %w", testNamespace, testPodName, err)
	}
	return nil
}

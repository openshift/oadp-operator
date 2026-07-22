package e2e_test

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	imagev1 "github.com/openshift/api/image/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

// ocCommandTimeout bounds runOC calls so a stalled registry import or API
// call fails the test instead of hanging indefinitely.
const ocCommandTimeout = 2 * time.Minute

// runOC shells out to the oc binary, mirroring the pattern already used by
// lib.RunMustGather. This test needs `oc new-build`/`oc tag`/`oc delete istag`,
// which have no controller-runtime client equivalent that reproduces the same
// server-side ImageStreamTag resolution behavior as the CLI.
func runOC(args ...string) (string, error) {
	log.Printf("Running: oc %s", strings.Join(args, " "))
	ctx, cancel := context.WithTimeout(context.Background(), ocCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "oc", args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("command timed out after %v: oc %s", ocCommandTimeout, strings.Join(args, " "))
	}
	return string(out), err
}

// Regression test for https://github.com/openshift/openshift-velero-plugin/issues/443
//
// ImageStream backup fails with "manifest unknown" when a status.tags[].items[]
// entry references another namespace's repository in the internal registry
// (e.g. because the image was promoted via `oc tag`), and the source
// repository no longer serves that digest (e.g. the source tag was deleted or
// pruned). The openshift-velero-plugin's imagecopy logic builds the copy
// source path directly from that stale DockerImageReference, so the copy
// 404s even though the digest is still fully servable from the ImageStream
// actually being backed up.
var _ = ginkgo.Describe("ImageStream backup with stale cross-namespace tag reference", ginkgo.Ordered, func() {
	const (
		sourceNamespace = "imagestream-stale-ref-src"
		targetNamespace = "imagestream-stale-ref-dst"
		imageStreamName = "imagefoo"
		tagName         = "v1"
		// Base image for the in-cluster build. The build's push (not this
		// pull) is what lands a managed image in the source namespace's
		// internal-registry repository — an external `oc tag` import would
		// record the external pull spec in status and the plugin would skip
		// it entirely. Same Red Hat registry precedent as
		// sample-applications/parks-app.
		buildBaseImage = "registry.access.redhat.com/ubi9/ubi-micro:latest"
	)

	var backupName, restoreName string

	ginkgo.BeforeAll(func() {
		log.Print("Creating DPA for imagestream stale-reference test")
		dpaSpec := dpaCR.Build(lib.CSI)
		err := dpaCR.CreateOrUpdate(dpaSpec)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		log.Print("Checking if DPA is reconciled")
		gomega.Eventually(dpaCR.IsReconciledTrue(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

		log.Print("Waiting for Velero Pod to be running")
		gomega.Eventually(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())

		log.Print("Checking if BSL is available")
		gomega.Eventually(dpaCR.BSLsAreAvailable(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
	})

	ginkgo.AfterAll(func() {
		log.Print("Cleaning up imagestream stale-reference test")
		// The test body deletes both namespaces on the happy path, so
		// tolerate NotFound but surface any other deletion failure.
		for _, ns := range []string{sourceNamespace, targetNamespace} {
			if err := lib.DeleteNamespace(kubernetesClientForSuiteRun, ns); err != nil && !apierrors.IsNotFound(err) {
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to delete namespace %s", ns)
			}
		}

		log.Print("Deleting DPA")
		err := dpaCR.Delete()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		log.Print("Waiting for velero to be deleted")
		gomega.Eventually(lib.VeleroIsDeleted(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
	})

	ginkgo.It("backs up and restores successfully after the source repository's tag is deleted", func() {
		log.Printf("Creating source namespace %s", sourceNamespace)
		err := lib.CreateNamespace(kubernetesClientForSuiteRun, sourceNamespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(lib.DoesNamespaceExist(kubernetesClientForSuiteRun, sourceNamespace)).Should(gomega.BeTrue())

		log.Printf("Creating target namespace %s", targetNamespace)
		err = lib.CreateNamespace(kubernetesClientForSuiteRun, targetNamespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(lib.DoesNamespaceExist(kubernetesClientForSuiteRun, targetNamespace)).Should(gomega.BeTrue())

		sourceRef := fmt.Sprintf("%s/%s:%s", sourceNamespace, imageStreamName, tagName)

		// An in-cluster build pushes its output to the internal registry,
		// creating an ImageStreamMapping whose status item records the
		// internal source-namespace repository path — the state this bug
		// depends on. An external `oc tag` import would instead record the
		// external pull spec verbatim and the plugin would never copy it.
		log.Printf("Building %s in-cluster so a managed image is pushed to the internal registry", sourceRef)
		out, err := runOC("new-build", "-n", sourceNamespace,
			"--name", imageStreamName,
			"--to", fmt.Sprintf("%s:%s", imageStreamName, tagName),
			"-D", "FROM "+buildBaseImage)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), out)

		log.Print("Waiting for the build to push the image to the source ImageStream")
		var sourceItems []imagev1.TagEvent
		gomega.Eventually(func() ([]imagev1.TagEvent, error) {
			var err error
			sourceItems, err = getTagItems(sourceNamespace, imageStreamName, tagName)
			return sourceItems, err
		}, time.Minute*10, time.Second*10).ShouldNot(gomega.BeEmpty())

		gomega.Expect(sourceItems[0].DockerImageReference).To(
			gomega.ContainSubstring(sourceNamespace+"/"+imageStreamName),
			"expected the build push to record an internal-registry reference in the source stream",
		)

		// Construct the target ImageStream's status directly via the API
		// rather than promoting through `oc tag`: every `oc tag` construction
		// tried here either left the digest unresolved (--reference), got its
		// DockerImageReference normalized to the destination's own repository
		// by the apiserver (plain ImageStreamTag-kind cross-namespace tag), or
		// (--source=docker without --reference) depends on the image import
		// controller actually completing an import of an internal-registry
		// reference — observed to never resolve within 5 minutes, identically
		// across all three OCP lanes tested, so not a flake. Writing
		// status.tags directly reproduces exactly the condition the plugin
		// depends on (a tag item whose DockerImageReference still points at
		// another namespace's repository) without depending on any of that
		// CLI/controller behavior — this mirrors how the openshift-velero-
		// plugin's own restore path recreates ImageStream status, since that
		// is the only way a restored stream's tags are ever populated.
		//
		// Deliberately no spec.tags entry: a cross-namespace
		// Kind=ImageStreamImage spec.tags[].from is validated by the
		// apiserver on every create (including the one Velero's restore
		// performs), and fails once the source is gone — that's a separate,
		// restore-time admission concern this test isn't targeting. The
		// plugin's copy gate only reads status.tags, so spec.tags isn't
		// needed to reproduce or fix the backup-time bug.
		log.Printf("Creating target ImageStream %s/%s with a stale cross-namespace tag reference", targetNamespace, imageStreamName)
		targetStream := &imagev1.ImageStream{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: targetNamespace,
				Name:      imageStreamName,
			},
		}
		err = dpaCR.Client.Create(context.Background(), targetStream)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		targetStream.Status = imagev1.ImageStreamStatus{
			DockerImageRepository: fmt.Sprintf("image-registry.openshift-image-registry.svc:5000/%s/%s", targetNamespace, imageStreamName),
			Tags: []imagev1.NamedTagEventList{
				{
					Tag: tagName,
					Items: []imagev1.TagEvent{
						{
							Created:              metav1.Now(),
							DockerImageReference: sourceItems[0].DockerImageReference,
							Image:                sourceItems[0].Image,
						},
					},
				},
			},
		}
		err = dpaCR.Client.Status().Update(context.Background(), targetStream)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		log.Println("Verifying target tag's dockerImageReference is scoped to the source namespace's repository (stale-reference precondition)")
		targetItems, err := getTagItems(targetNamespace, imageStreamName, tagName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(targetItems).NotTo(gomega.BeEmpty())
		gomega.Expect(targetItems[0].DockerImageReference).To(
			gomega.ContainSubstring(sourceNamespace+"/"+imageStreamName),
			"test setup invalid: target ImageStream tag should still reference the source namespace's repository at this point",
		)

		log.Printf("Deleting source tag %s (simulates the source being pruned/removed)", sourceRef)
		out, err = runOC("delete", "istag", fmt.Sprintf("%s:%s", imageStreamName, tagName), "-n", sourceNamespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), out)

		log.Printf("Deleting source namespace %s so its repository is definitively unavailable", sourceNamespace)
		err = lib.DeleteNamespace(kubernetesClientForSuiteRun, sourceNamespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Eventually(
			lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, sourceNamespace),
			time.Minute*4,
			time.Second*5,
		).Should(gomega.BeTrue())

		log.Printf("Backing up namespace %s", targetNamespace)
		backupUid, _ := uuid.NewUUID()
		backupName = fmt.Sprintf("backup-imagestream-stale-ref-%s", backupUid.String())
		err = lib.CreateBackupForNamespaces(dpaCR.Client, namespace, backupName, []string{targetNamespace}, false, false)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Eventually(lib.IsBackupDone(dpaCR.Client, namespace, backupName), time.Minute*10, time.Second*10).Should(gomega.BeTrue())

		log.Print("Verifying backup completed successfully (not PartiallyFailed due to stale cross-namespace image copy)")
		result, err := lib.IsBackupCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(result).To(gomega.BeTrue())

		// A Completed backup alone is a false green if the imagestream
		// image copy never ran (e.g. image backup disabled): assert the
		// plugin actually copied, and from the target stream's OWN
		// repository — the fixed behavior — rather than the stale
		// cross-namespace reference.
		log.Print("Verifying the imagestream image copy executed from the stream's own repository")
		backupLogs := lib.BackupLogs(kubernetesClientForSuiteRun, dpaCR.Client, namespace, backupName)
		gomega.Expect(backupLogs).To(gomega.ContainSubstring("copying from"))
		gomega.Expect(backupLogs).To(gomega.ContainSubstring(fmt.Sprintf("%s/%s@", targetNamespace, imageStreamName)))

		log.Printf("Deleting namespace %s to force restore-from-backup", targetNamespace)
		err = lib.DeleteNamespace(kubernetesClientForSuiteRun, targetNamespace)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Eventually(lib.IsNamespaceDeleted(kubernetesClientForSuiteRun, targetNamespace), time.Minute*4, time.Second*5).Should(gomega.BeTrue())

		log.Print("Restoring from backup")
		restoreUid, _ := uuid.NewUUID()
		restoreName = fmt.Sprintf("restore-imagestream-stale-ref-%s", restoreUid.String())
		err = lib.CreateRestoreFromBackup(dpaCR.Client, namespace, backupName, restoreName)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Eventually(lib.IsRestoreDone(dpaCR.Client, namespace, restoreName), time.Minute*10, time.Second*10).Should(gomega.BeTrue())

		log.Print("Verifying restore completed successfully")
		restoreResult, err := lib.IsRestoreCompletedSuccessfully(kubernetesClientForSuiteRun, dpaCR.Client, namespace, restoreName)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(restoreResult).To(gomega.BeTrue())

		log.Print("Verifying ImageStream was restored with its tag intact")
		gomega.Eventually(func() ([]imagev1.TagEvent, error) {
			return getTagItems(targetNamespace, imageStreamName, tagName)
		}, time.Minute*2, time.Second*5).ShouldNot(gomega.BeEmpty())
	})
})

// getTagItems returns the TagEvent history for a given ImageStream tag. A
// missing ImageStream or tag returns (nil, nil) so pollers can keep waiting;
// any other client error is returned so callers surface auth/server failures
// instead of masking them as "not found".
func getTagItems(ns, imageStreamName, tag string) ([]imagev1.TagEvent, error) {
	imageStream := &imagev1.ImageStream{}
	err := dpaCR.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: imageStreamName}, imageStream)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, namedTagEventList := range imageStream.Status.Tags {
		if namedTagEventList.Tag == tag {
			return namedTagEventList.Items, nil
		}
	}
	return nil, nil
}

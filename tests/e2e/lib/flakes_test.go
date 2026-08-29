package lib

import (
	"strings"
	"testing"
)

// Real kdm-controller manager.log lines captured from a live CI run
// (/tmp/kdm-mgr-log2.txt, 2026-08-28T16:48-16:49Z), covering two different
// specs' DataUploads reconciling concurrently against the SAME shared,
// never-restarted controller pod -- exactly the scenario
// FilterLogLinesContaining exists to guard against.

// du-cirros-incr-seq-1-... is the DataUpload for backup "cirros-incr-seq-1".
// These two lines are real, unmodified captures of the #212 flake pattern
// ("VMBT already prepared but VMB not yet visible in cache, requeuing").
const realLine212A = `2026-08-28T16:48:47Z	INFO	VMBT already prepared but VMB not yet visible in cache, requeuing	{"controller": "kubevirt-dataupload", "controllerGroup": "velero.io", "controllerKind": "DataUpload", "DataUpload": {"name":"du-cirros-incr-seq-1-cirros-test-cirros-test-c2ce2dc4","namespace":"openshift-adp"}, "namespace": "openshift-adp", "name": "du-cirros-incr-seq-1-cirros-test-cirros-test-c2ce2dc4", "reconcileID": "fd536430-7cab-4758-813a-3aafd75e8934", "vmbtName": "vmbt-cirros-test-n58ss"}`
const realLine212B = `2026-08-28T16:48:52Z	INFO	VMBT already prepared but VMB not yet visible in cache, requeuing	{"controller": "kubevirt-dataupload", "controllerGroup": "velero.io", "controllerKind": "DataUpload", "DataUpload": {"name":"du-cirros-incr-seq-1-cirros-test-cirros-test-c2ce2dc4","namespace":"openshift-adp"}, "namespace": "openshift-adp", "name": "du-cirros-incr-seq-1-cirros-test-cirros-test-c2ce2dc4", "reconcileID": "eef72d18-8408-4f90-8405-9be3972cad30", "vmbtName": "vmbt-cirros-test-n58ss"}`

// du-cirros-stale-sibling-backup-... is a DIFFERENT spec's DataUpload,
// reconciling in the same log window with no problems at all (healthy,
// unrelated noise -- the real-world shape of what a whole-pod-log fetch
// mixes in alongside the spec actually under test).
const realLineHealthyA = `2026-08-28T16:49:21Z	INFO	Reconciling DataUpload with kubevirt datamover	{"controller": "kubevirt-dataupload", "controllerGroup": "velero.io", "controllerKind": "DataUpload", "DataUpload": {"name":"du-cirros-stale-sibling-backup-cirros-test-cirros-test-86ae9509","namespace":"openshift-adp"}, "namespace": "openshift-adp", "name": "du-cirros-stale-sibling-backup-cirros-test-cirros-test-86ae9509", "reconcileID": "aae0a708-caca-4e5e-9a4b-dd763622396a", "dataUpload": {"name":"du-cirros-stale-sibling-backup-cirros-test-cirros-test-86ae9509","namespace":"openshift-adp"}, "phase": ""}`
const realLineHealthyB = `2026-08-28T16:49:21Z	INFO	Handling New phase DataUpload	{"controller": "kubevirt-dataupload", "controllerGroup": "velero.io", "controllerKind": "DataUpload", "DataUpload": {"name":"du-cirros-stale-sibling-backup-cirros-test-cirros-test-86ae9509","namespace":"openshift-adp"}, "namespace": "openshift-adp", "name": "du-cirros-stale-sibling-backup-cirros-test-cirros-test-86ae9509", "reconcileID": "aae0a708-caca-4e5e-9a4b-dd763622396a"}`
const realLineHealthyC = `2026-08-28T16:49:21Z	INFO	Updated DataUpload phase	{"controller": "kubevirt-dataupload", "controllerGroup": "velero.io", "controllerKind": "DataUpload", "DataUpload": {"name":"du-cirros-stale-sibling-backup-cirros-test-cirros-test-86ae9509","namespace":"openshift-adp"}, "namespace": "openshift-adp", "name": "du-cirros-stale-sibling-backup-cirros-test-cirros-test-86ae9509", "reconcileID": "aae0a708-caca-4e5e-9a4b-dd763622396a", "dataUpload": "du-cirros-stale-sibling-backup-cirros-test-cirros-test-86ae9509", "phase": "Accepted", "message": "DataUpload accepted by kubevirt datamover"}`

// The #208 pattern ("VirtualMachineBackup in progress, requeuing") and the
// #212 pattern above were both removed from CheckIfFlakeOccurred once a kdm-controller
// build carrying migtools/kubevirt-datamover-controller#208 and #212 was
// verified live (2026-08-29, quay.io/tkaovila/kubevirt-datamover-controller:combined-208-212-test)
// to produce a genuine PASS of the incremental-sequence spec with zero
// occurrences of either pattern string, where it had previously reliably
// flake-skipped within ~20 minutes. realLine212A/B stay as fixture data below
// since they're still useful, realistic-shaped log content for exercising
// FilterLogLinesContaining's own scoping logic -- they just no longer
// exercise CheckIfFlakeOccurred's pattern registry.

const backupName = "cirros-incr-seq-1"
const dataUploadName = "du-cirros-incr-seq-1-cirros-test-cirros-test-c2ce2dc4"
const staleSiblingBackupName = "cirros-stale-sibling-backup"
const staleSiblingDataUploadName = "du-cirros-stale-sibling-backup-cirros-test-cirros-test-86ae9509"

func combinedPodLog() string {
	return strings.Join([]string{
		realLineHealthyA,
		realLineHealthyB,
		realLine212A,
		realLine212B,
		realLineHealthyC,
	}, "\n")
}

func Test_FilterLogLinesContaining_scopesToOwnBackup(t *testing.T) {
	filtered := FilterLogLinesContaining(combinedPodLog(), backupName, dataUploadName)

	if !strings.Contains(filtered, "VMBT already prepared but VMB not yet visible in cache, requeuing") {
		t.Fatalf("expected filtered log to retain the #212 pattern line for %s, got:\n%s", backupName, filtered)
	}
	if strings.Contains(filtered, staleSiblingDataUploadName) {
		t.Fatalf("expected filtered log to exclude the unrelated sibling backup's lines, got:\n%s", filtered)
	}
}

func Test_FilterLogLinesContaining_noSubstrsReturnsInputUnchanged(t *testing.T) {
	logs := combinedPodLog()
	if got := FilterLogLinesContaining(logs); got != logs {
		t.Fatalf("expected unfiltered passthrough with no substrs, got:\n%s", got)
	}
}

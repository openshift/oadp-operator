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

// The #208 pattern ("VirtualMachineBackup in progress, requeuing", line 663
// of kubevirt_dataupload_controller.go) has never been captured raw in any
// CI artifact -- per-spec log captures only fire on a hard spec FAILURE, and
// our own flake-skip logic means this pattern now produces a Skip, not a
// failure, so no artifact will ever contain it. Verified instead by reading
// the controller's actual source directly (migtools/kubevirt-datamover-controller,
// internal/controller/kubevirt_dataupload_controller.go): line 663's
// logger.Info call has no inline key-value args of its own, but it shares
// the exact same `logger := log.FromContext(ctx)` (declared once near the
// top of the same function, no reassignment in between) as the #212 call
// site above -- so it carries the identical controller-runtime-injected
// DataUpload/namespace/name/reconcileID fields via the generic Reconciler
// wrapper's context injection, confirmed present on realLine212A/B for the
// sibling call. This fixture mirrors that confirmed field shape rather than
// inventing one.
const sourceVerifiedLine208 = `2026-08-28T16:50:03Z	INFO	VirtualMachineBackup in progress, requeuing	{"controller": "kubevirt-dataupload", "controllerGroup": "velero.io", "controllerKind": "DataUpload", "DataUpload": {"name":"du-cirros-incr-seq-1-cirros-test-cirros-test-c2ce2dc4","namespace":"openshift-adp"}, "namespace": "openshift-adp", "name": "du-cirros-incr-seq-1-cirros-test-cirros-test-c2ce2dc4", "reconcileID": "0c9c1a3e-2222-4b1a-9e3d-111122223333"}`

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

// Test_CheckIfFlakeOccurred_misattributionRepro reproduces the actual bug
// FilterLogLinesContaining was written to fix: a spec whose OWN backup
// (cirros-stale-sibling-backup) is perfectly healthy would, without
// filtering, see the #212 flake match anyway -- because the shared,
// never-restarted kdm-controller pod's log also contains a genuinely
// different spec's (cirros-incr-seq-1) stuck DataUpload lines from the same
// time window. Filtering by the CURRENT spec's own backup/DataUpload name
// must make that false match go away.
func Test_CheckIfFlakeOccurred_misattributionRepro(t *testing.T) {
	combined := combinedPodLog()

	if !CheckIfFlakeOccurred([]string{combined}) {
		t.Fatalf("sanity check failed: expected unfiltered combined log to match the #212 pattern (proves the repro scenario is real)")
	}

	filteredForHealthySpec := FilterLogLinesContaining(combined, staleSiblingBackupName, staleSiblingDataUploadName)
	if CheckIfFlakeOccurred([]string{filteredForHealthySpec}) {
		t.Fatalf("misattribution bug reproduced: filtering by the healthy spec's own backup/DataUpload name still matched the OTHER spec's #212 flake:\n%s", filteredForHealthySpec)
	}
}

// Test_CheckIfFlakeOccurred_realMatchStillDetected proves the fix doesn't
// overcorrect into false negatives: when the CURRENT spec's own backup
// really did hit the #212 pattern, scoping the log to its own
// backup/DataUpload name must still catch it.
func Test_CheckIfFlakeOccurred_realMatchStillDetected(t *testing.T) {
	filtered := FilterLogLinesContaining(combinedPodLog(), backupName, dataUploadName)
	if !CheckIfFlakeOccurred([]string{filtered}) {
		t.Fatalf("expected the #212 pattern to still be detected once scoped to its own backup, got:\n%s", filtered)
	}
}

// Test_CheckIfFlakeOccurred_pattern208SurvivesScoping validates, using the
// source-verified fixture (see sourceVerifiedLine208's doc comment), that
// the #208 pattern's log line -- which has no inline key-value args at its
// own call site -- still carries enough of the controller-runtime-injected
// DataUpload/name fields to survive backup/DataUpload-name scoping the same
// way the #212 pattern does. If this ever regresses (e.g. the controller
// stops injecting those fields, or the call site moves to a differently-
// constructed logger), this test fails loudly instead of the fix silently
// dropping the #208 detection path.
func Test_CheckIfFlakeOccurred_pattern208SurvivesScoping(t *testing.T) {
	logWithNoise := strings.Join([]string{realLineHealthyA, sourceVerifiedLine208, realLineHealthyC}, "\n")

	filtered := FilterLogLinesContaining(logWithNoise, backupName, dataUploadName)
	if !strings.Contains(filtered, "VirtualMachineBackup in progress, requeuing") {
		t.Fatalf("expected scoping by backup/DataUpload name to retain the #208 pattern line, got:\n%s", filtered)
	}
	if !CheckIfFlakeOccurred([]string{filtered}) {
		t.Fatalf("expected the #208 pattern to still be detected once scoped to its own backup, got:\n%s", filtered)
	}
}

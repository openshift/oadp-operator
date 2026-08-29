package lib

import (
	"log"
	"regexp"
	"strings"
)

var errorIgnorePatterns = []string{
	"received EOF, stopping recv loop",
	"Checking for AWS specific error information",
	"awserr.Error contents",
	"Error creating parent directories for blob-info-cache-v1.boltdb",
	"blob unknown",
	"num errors=0",
	"level=debug", // debug logs may contain the text error about recoverable errors so ignore them
	"Unable to retrieve in-cluster version",
	"restore warning",

	// Ignore managed fields errors per https://github.com/vmware-tanzu/velero/pull/6110 and avoid e2e failure.
	// https://prow.ci.openshift.org/view/gs/origin-ci-test/pr-logs/pull/openshift_oadp-operator/1126/pull-ci-openshift-oadp-operator-master-4.10-operator-e2e-aws/1690109468546699264#1:build-log.txt%3A686
	"level=error msg=\"error patch for managed fields ",
	"VolumeSnapshot has a temporary error Failed to create snapshot: error updating status for volume snapshot content snapcontent-",
	"Skipping hypershift plugin execution - not a hypershift backup: error checking for HostedControlPlane CRD",

	// Data mover volume restore limitation per https://github.com/vmware-tanzu/velero/issues/7946#issuecomment-2196590014
	"failed to restore volume with StorageClass, claim Selector is not supported",
}

type FlakePattern struct {
	Issue               string
	Description         string
	StringSearchPattern string
}

// FilterLogLinesContaining returns only the lines of logs that contain at
// least one of the given substrings. kdm-controller runs as a single shared
// pod across every spec in a suite run (it's never restarted between specs),
// so a raw, unfiltered pod-log fetch mixes lines from whichever OTHER
// DataUpload/backup happened to be reconciling at the same moment. Passing
// that unfiltered text straight to CheckIfFlakeOccurred lets a stale line
// from a completely different, earlier spec's backup match a known-bug
// pattern and misattribute it to the CURRENT spec -- confirmed as a real risk
// live: the skip path this filtering protects deletes the current spec's own
// backup via DeleteVeleroBackupAndRestore before skipping, so a
// misattributed match could make an otherwise-healthy spec delete its own
// good backup over noise from someone else's stuck one. Callers should pass
// the specific backup name and/or DataUpload name they actually care about.
func FilterLogLinesContaining(logs string, substrs ...string) string {
	if len(substrs) == 0 {
		return logs
	}
	var matched []string
	for _, line := range strings.Split(logs, "\n") {
		for _, s := range substrs {
			if s != "" && strings.Contains(line, s) {
				matched = append(matched, line)
				break
			}
		}
	}
	return strings.Join(matched, "\n")
}

// CheckIfFlakeOccurred checks for known flake patterns in the provided logs (typically logs from the test ran).
//
// Parameters:
//
//	logs ([]string):    Logs to be examined for known flake patterns.
//
// Name-bearing status of each pattern below, for callers pre-filtering with
// FilterLogLinesContaining (verified against kdm-controller source
// migtools/kubevirt-datamover-controller@internal/controller/kubevirt_dataupload_controller.go,
// 2026-08-29 -- re-check this if that file's logger plumbing changes):
//   - CNV-85377 ("is being attached to VMI"): not found verbatim in
//     kdm-controller source; if it ever surfaces there it would be via a VMB
//     condition Reason/Message logged inline (e.g. "reason", cond.Reason) on
//     kdm-controller's shared, context-injected logger -- also safe to scope
//     by backup/DataUpload name. Otherwise it's virt-controller's own
//     condition text, which reaches us only via Kubernetes Events, same as
//     the #18957 pattern below.
//   - kubevirt#18957 ("VMI backup status was lost"): virt-controller's own
//     event text, never appears in any pod log -- callers must check this
//     against Namespace Events (already namespace-scoped), never filtered
//     pod logs, or it will never match.
//   - external-snapshotter#876, velero#5856, OADP-5086: velero/snapshot-controller
//     strings that never appear in kdm-controller's own log; irrelevant to
//     scoping decisions made against kdm-controller pod logs specifically.
func CheckIfFlakeOccurred(logs []string) bool {
	flakePatterns := []FlakePattern{
		{
			Issue:               "https://github.com/kubernetes-csi/external-snapshotter/pull/876",
			Description:         "Race condition in the VolumeSnapshotBeingCreated",
			StringSearchPattern: "Failed to check and update snapshot content: failed to remove VolumeSnapshotBeingCreated annotation on the content snapcontent-",
		},
		{
			Issue:               "https://github.com/vmware-tanzu/velero/issues/5856",
			Description:         "Transient S3 bucket errors and limits",
			StringSearchPattern: "Error copying image: writing blob: uploading layer chunked: received unexpected HTTP status: 500 Internal Server Error",
		},
		{
			Issue:               "https://issues.redhat.com/browse/OADP-5086",
			Description:         "Startup probe timeout causing deployment readiness failure after restore",
			StringSearchPattern: "deployment is not in a ready state",
		},
		{
			Issue:               "https://redhat.atlassian.net/browse/CNV-85377",
			Description:         "virt-controller's VirtualMachineBackup status can silently stop advancing after the underlying attach already succeeded: startBackup()'s attach branch (kubevirt/kubevirt pkg/storage/cbt/backup.go) returns without writing a status condition or requeuing, so recovery depends entirely on a VMI watch event that may never fire again -- also reported as https://redhat.atlassian.net/browse/CNV-89684",
			StringSearchPattern: "is being attached to VMI",
		},
		{
			Issue:               "https://github.com/kubevirt/kubevirt/pull/18957",
			Description:         "virt-controller's reconcileStart() (pkg/storage/cbt/backup.go) can mark an already-successfully-completed VirtualMachineBackup Failed with reason SourceLost: cleanupVMIState() clears the VMI's BackupStatus and re-triggers reconcile before the backup's own just-written terminal status is visible via the informer's async cache, so the stale reconcile wrongly concludes the status was lost mid-flight -- observed live immediately after a real 'Completed VirtualMachineBackup, warning: ...' event, first seen testing kubevirt/kubevirt nightly (v1.20.0). Fixed at kubevirt/kubevirt#18957 (checks the API server directly before concluding lost); distinct from CNV-85377/kubevirt/kubevirt#18949.",
			StringSearchPattern: "VMI backup status was lost",
		},
	}
	logString := strings.Join(logs, "\n")

	for _, pattern := range flakePatterns {
		re := regexp.MustCompile(pattern.StringSearchPattern)
		if re.MatchString(logString) {
			log.Printf("FLAKE DETECTION: Match found for issue %s: %s\n", pattern.Issue, pattern.Description)
			return true
		}
	}
	log.Println("FLAKE DETECTION: No known flakes found.")
	return false
}

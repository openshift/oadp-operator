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
		{
			Issue:               "https://github.com/migtools/kubevirt-datamover-controller/pull/208",
			Description:         "kubevirt-datamover-controller's evaluateVMBackupStatus/isVMBTerminal looked only for a VirtualMachineBackup condition of type \"Done\" (kubevirtbackupv1alpha1.ConditionDone, from its vendored kubevirt.io/api v1.8.0-alpha.0), but kubevirt nightly (v1.20.0+) renamed that condition to \"Complete\" -- the VMB itself completes fine (conditions show Complete=True, reason=CompletedWithWarning), but the controller never recognized it and just logged \"VirtualMachineBackup in progress, requeuing\" forever, confirmed live: 243 occurrences over a 20-minute test timeout. Fixed at migtools/kubevirt-datamover-controller#208 (declares a local conditionComplete=\"Complete\" literal and accepts both the old and new condition names, rather than bumping the vendored kubevirt.io/api -- TDD via TestHandleAccepted_VMBStatusDetection/TestIsVMBTerminal). A dependency-version-skew bug in kdm-controller, not this repo or kubevirt itself, surfaced only because HCO_INDEX_TAG now defaults to nightly.",
			StringSearchPattern: "VirtualMachineBackup in progress, requeuing",
		},
		{
			Issue:               "https://github.com/migtools/kubevirt-datamover-controller/pull/212",
			Description:         "kubevirt-datamover-controller's DataUpload reconcile logs \"VMBT already prepared but VMB not yet visible in cache, requeuing\" every ~5s forever. Root cause (migtools/kubevirt-datamover-controller#211): a guard checking findVMBForDataUpload's result via the informer cache was written when that lookup had no APIReader direct-read fallback and prepareVMBackupTracker used to delete-and-recreate the VMBT -- neither is true anymore (the lookup already falls back to a direct API read, and the VMBT is now reused via a VM-name-hash label instead of being deleted), so vmb==nil at the guard is never just \"not yet cached\", it's a real absence. The actual delay is Step 4's VMB creation being rejected by KubeVirt's admission webhook (one active VMB per VM) *after* Step 2 already persisted the VMBTName annotation -- every subsequent reconcile then short-circuits on the now-stale guard and Step 4 is never retried, so the VMB is never created and the DataUpload spins until OperationTimeout. Fixed at migtools/kubevirt-datamover-controller#212 (fixes #211): removes the guard entirely, falling through to the existing idempotent Steps 2-4, so a cleared admission conflict lets VMB creation actually retry and succeed. Our backup-delete-before-skip workaround (see the two ginkgo.Skip sites in this file) remains a useful defensive backstop even after this lands.",
			StringSearchPattern: "VMBT already prepared but VMB not yet visible in cache, requeuing",
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

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
}

type FlakePattern struct {
	Issue               string
	Description         string
	StringSearchPattern string
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

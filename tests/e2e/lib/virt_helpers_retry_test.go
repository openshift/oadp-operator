package lib

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func Test_isTransientEmptyChecksumOutput(t *testing.T) {
	if !isTransientEmptyChecksumOutput("dd if=/dev/volume0 | sha256sum", "   ", nil) {
		t.Fatalf("expected empty successful sha256sum output to be treated as transient")
	}
	if isTransientEmptyChecksumOutput("echo hello", "", nil) {
		t.Fatalf("expected non-checksum commands to skip empty-output retry")
	}
	if isTransientEmptyChecksumOutput("dd if=/dev/volume0 | sha256sum", "abc  -", errors.New("boom")) {
		t.Fatalf("expected command errors to be handled by normal error path")
	}
}

func Test_execShellCommandInPodWithRetry_retriesEmptyChecksumOutput(t *testing.T) {
	oldExec := executeShellCommandInPodWithRetryImpl
	oldSleep := sleepForPodExecRetry
	defer func() {
		executeShellCommandInPodWithRetryImpl = oldExec
		sleepForPodExecRetry = oldSleep
	}()

	calls := 0
	sleepCalls := 0
	executeShellCommandInPodWithRetryImpl = func(_ ProxyPodParameters, _ string) (string, string, error) {
		calls++
		if calls < 3 {
			return "", "", nil
		}
		return "deadbeef  -\n", "", nil
	}
	sleepForPodExecRetry = func(_ time.Duration) {
		sleepCalls++
	}

	stdout, stderr, err := execShellCommandInPodWithRetry(ProxyPodParameters{}, "dd if=/dev/volume0 | sha256sum")
	if err != nil {
		t.Fatalf("expected eventual success, got err=%v", err)
	}
	if stdout == "" || stderr != "" {
		t.Fatalf("expected non-empty checksum stdout and empty stderr, got stdout=%q stderr=%q", stdout, stderr)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts (2 retries), got %d", calls)
	}
	if sleepCalls != 2 {
		t.Fatalf("expected 2 backoff sleeps, got %d", sleepCalls)
	}
}

func Test_execShellCommandInPodWithRetry_stopsAfterMaxEmptyChecksumRetries(t *testing.T) {
	oldExec := executeShellCommandInPodWithRetryImpl
	oldSleep := sleepForPodExecRetry
	defer func() {
		executeShellCommandInPodWithRetryImpl = oldExec
		sleepForPodExecRetry = oldSleep
	}()

	calls := 0
	executeShellCommandInPodWithRetryImpl = func(_ ProxyPodParameters, _ string) (string, string, error) {
		calls++
		return "", "", nil
	}
	sleepForPodExecRetry = func(_ time.Duration) {}

	stdout, stderr, err := execShellCommandInPodWithRetry(ProxyPodParameters{}, "dd if=/dev/volume0 | sha256sum")
	if err != nil {
		t.Fatalf("expected nil err after exhausted empty-output retries, got %v", err)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("expected empty stdout/stderr after exhausted retries, got stdout=%q stderr=%q", stdout, stderr)
	}
	if calls != 3 {
		t.Fatalf("expected max 3 attempts, got %d", calls)
	}
}

// Test_ChecksumBlockDeviceRegion_persistentEmptyOutputReturnsCallerError is
// the caller-level counterpart to
// Test_execShellCommandInPodWithRetry_stopsAfterMaxEmptyChecksumRetries: it
// exercises checksumBlockDeviceRegionForPod (ChecksumBlockDeviceRegion's core
// logic, split out so it doesn't need a real/fake clientset to resolve a
// pod) to confirm that persistent empty sha256sum output -- even after the
// retry wrapper gives up and returns a nil error -- is still surfaced as a
// real error, not silently treated as a valid (empty) checksum. Without this
// test, a regression that made the checksum path stop rejecting empty
// output would still pass the retry-wrapper-only test above.
func Test_ChecksumBlockDeviceRegion_persistentEmptyOutputReturnsCallerError(t *testing.T) {
	oldExec := executeShellCommandInPodWithRetryImpl
	oldSleep := sleepForPodExecRetry
	defer func() {
		executeShellCommandInPodWithRetryImpl = oldExec
		sleepForPodExecRetry = oldSleep
	}()

	executeShellCommandInPodWithRetryImpl = func(_ ProxyPodParameters, _ string) (string, string, error) {
		return "", "", nil
	}
	sleepForPodExecRetry = func(_ time.Duration) {}

	v := &VirtOperator{}

	_, err := v.checksumBlockDeviceRegionForPod(nil, "alpine-guestagent", "virt-launcher-alpine-guestagent-abcde", "volume0", 0, 1)
	if err == nil {
		t.Fatalf("expected ChecksumBlockDeviceRegion to return a caller-facing error on persistent empty sha256sum output, got nil")
	}
	if !strings.Contains(err.Error(), "sha256sum produced no output") {
		t.Fatalf("expected the empty-output error message, got: %v", err)
	}
}

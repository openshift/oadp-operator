package release

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine test file path")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(filename)))
}

// branchVersion returns the oadp-X.Y version from the current branch.
// Checks PULL_BASE_REF first (set by Prow to the PR target branch),
// then falls back to the local git branch name.
func branchVersion(t *testing.T) string {
	t.Helper()
	if ref := os.Getenv("PULL_BASE_REF"); strings.HasPrefix(ref, oadpBranchPrefix) {
		return ref
	}
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("failed to get current branch: %v", err)
	}
	branch := strings.TrimSpace(string(out))
	if !strings.HasPrefix(branch, oadpBranchPrefix) {
		return ""
	}
	return branch
}

func readBundleFile(t *testing.T, root, relPath string) []byte {
	t.Helper()
	path := filepath.Join(root, relPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Skipf("%s does not exist (only present on release branches)", relPath)
		}
		t.Fatalf("failed to read %s: %v", relPath, err)
	}
	return data
}

func reportErrors(t *testing.T, errs []error) {
	t.Helper()
	for _, err := range errs {
		t.Error(err)
	}
}

func assertErrors(t *testing.T, errs []error, wantErrs int, wantMsg string) {
	t.Helper()
	if len(errs) != wantErrs {
		t.Fatalf("got %d errors, want %d: %v", len(errs), wantErrs, errs)
	}
	if wantMsg != "" {
		for _, err := range errs {
			if strings.Contains(err.Error(), wantMsg) {
				return
			}
		}
		t.Errorf("expected error containing %q, got %v", wantMsg, errs)
	}
}

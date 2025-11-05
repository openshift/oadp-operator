package e2e_test

import (
	"fmt"
	"os"
	"strings"

	"github.com/onsi/ginkgo/v2"
)

// TestSkipInfo contains information about why a test is skipped
type TestSkipInfo struct {
	BugNumber string
	Reason    string
	// Set to true if the skip can be overridden by environment variable
	Overridable bool
}

// testSkipRegistry maps test names to skip information
// This is the central registry for all skipped tests
var testSkipRegistry = map[string]TestSkipInfo{
	// Mongo CSI and DATAMOVER tests - skipped due to known issues
	"Mongo application CSI": {
		BugNumber:   "OADP-XXXX",
		Reason:      "CSI snapshots failing for Mongo workloads",
		Overridable: true,
	},
	"Mongo application DATAMOVER": {
		BugNumber:   "OADP-XXXX",
		Reason:      "DATAMOVER backup failing for Mongo workloads",
		Overridable: true,
	},
	"Mongo application BlockDevice DATAMOVER": {
		BugNumber:   "OADP-XXXX",
		Reason:      "Block device DATAMOVER failing for Mongo workloads",
		Overridable: true,
	},
	"Mongo application CSI via CLI": {
		BugNumber:   "OADP-XXXX",
		Reason:      "CSI snapshots failing for Mongo workloads via CLI",
		Overridable: true,
	},
	"Mongo application DATAMOVER via CLI": {
		BugNumber:   "OADP-XXXX",
		Reason:      "DATAMOVER backup failing for Mongo workloads via CLI",
		Overridable: true,
	},
	"Mongo application BlockDevice DATAMOVER via CLI": {
		BugNumber:   "OADP-XXXX",
		Reason:      "Block device DATAMOVER failing for Mongo workloads via CLI",
		Overridable: true,
	},
}

// shouldSkipTest checks if a test should be skipped based on the registry
// and environment variable settings
func shouldSkipTest(testName string) (bool, string) {
	skipInfo, exists := testSkipRegistry[testName]
	if !exists {
		return false, ""
	}

	// Check if skips are disabled via environment variable
	// Set OADP_SKIP_KNOWN_FAILURES=false to run all tests regardless of skip registry
	if skipInfo.Overridable {
		skipEnv := os.Getenv("OADP_SKIP_KNOWN_FAILURES")
		if strings.ToLower(skipEnv) == "false" {
			return false, ""
		}
	}

	skipMessage := fmt.Sprintf("SKIPPED: %s - Bug: %s - Reason: %s",
		testName, skipInfo.BugNumber, skipInfo.Reason)
	return true, skipMessage
}

// SkipIfNeeded checks if the current test should be skipped and skips it if necessary
// This should be called at the beginning of each test that might be skipped
func SkipIfNeeded(testName string) {
	if shouldSkip, message := shouldSkipTest(testName); shouldSkip {
		ginkgo.Skip(message)
	}
}

// GetSkippedTestsList returns a formatted list of all tests in the skip registry
// Useful for documentation and CI reporting
func GetSkippedTestsList() string {
	if len(testSkipRegistry) == 0 {
		return "No tests are currently skipped"
	}

	var sb strings.Builder
	sb.WriteString("Currently skipped tests:\n")
	for testName, skipInfo := range testSkipRegistry {
		sb.WriteString(fmt.Sprintf("  - %s\n", testName))
		sb.WriteString(fmt.Sprintf("    Bug: %s\n", skipInfo.BugNumber))
		sb.WriteString(fmt.Sprintf("    Reason: %s\n", skipInfo.Reason))
		if skipInfo.Overridable {
			sb.WriteString("    Override: Set OADP_SKIP_KNOWN_FAILURES=false to run this test\n")
		}
	}
	return sb.String()
}

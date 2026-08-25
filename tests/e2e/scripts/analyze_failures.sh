#!/bin/bash
# Analyze test failures with Claude via Vertex AI after Ginkgo suite completes
# Only runs if tests failed and Claude analysis is not skipped
#
# NOTE: This script is no longer invoked by CI. Presubmit E2E failure analysis
# now runs as an openshift/release step-registry post-step (oadp-analyze-e2e-failure)
# using the shared claude-ai-helpers image and sa-claude-openshift-ci credential,
# outside this repo's test container. This script is kept for local/manual use only
# -- run it by hand after `make test-e2e` with GOOGLE_APPLICATION_CREDENTIALS and
# ANTHROPIC_VERTEX_PROJECT_ID set, or a plain ANTHROPIC_API_KEY. See
# docs/design/claude-prow-failure-analysis_design.md for the superseded design.
#
# Features:
# - Claude CLI availability check before invoking
# - Proper exit code capture (avoids pipefail issues)
# - Large artifact preprocessing with subagent pattern
# - Secret redaction on all output
#
# Note: Prow's build-log.txt is written by CI infrastructure AFTER tests complete,
# so it is NOT available during this analysis. We rely on:
# - JUnit reports (junit_report.xml)
# - must-gather diagnostics
# - Per-test pod log directories

set +e  # Don't exit on Claude failure

ARTIFACT_DIR=${ARTIFACT_DIR:-/tmp}
SKIP_CLAUDE=${SKIP_CLAUDE_ANALYSIS:-false}
EXIT_CODE=$1

# Size thresholds for preprocessing (in bytes)
LARGE_FILE_THRESHOLD=${LARGE_FILE_THRESHOLD:-1048576}  # 1MB
MAX_LOG_LINES=${MAX_LOG_LINES:-500}  # Max lines to include per log file

# Redact sensitive information from logs and output
# Redacts: API keys, tokens, passwords, service account keys, AWS credentials
redact_secrets() {
    sed -E \
        -e 's/AKIA[0-9A-Z]{16}/[REDACTED-AWS-ACCESS-KEY]/g' \
        -e 's/(aws_secret_access_key[" :=]+)[A-Za-z0-9/+=]{40}/\1[REDACTED-AWS-SECRET]/g' \
        -e 's/"private_key": ?"-----BEGIN[^"]*END[^"]*"/"private_key": "[REDACTED-GCP-PRIVATE-KEY]"/g' \
        -e 's/Bearer +[A-Za-z0-9._~+-]+=*/Bearer [REDACTED-TOKEN]/g' \
        -e 's/(password[" :=]+)[^ "'\'']+/\1[REDACTED-PASSWORD]/gi' \
        -e 's/(passwd[" :=]+)[^ "'\'']+/\1[REDACTED-PASSWORD]/gi' \
        -e 's/(api[_-]?key[" :=]+)[^ "'\'']+/\1[REDACTED-APIKEY]/gi' \
        -e 's/(token[" :=]+)[A-Za-z0-9._~+-]+=*/\1[REDACTED-TOKEN]/gi' \
        -e 's/(secret[" :=]+)[^ "'\'']{16,}/\1[REDACTED-SECRET]/gi' \
        -e 's/eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*/[REDACTED-JWT-TOKEN]/g' \
        -e 's/-----BEGIN (RSA |EC )?PRIVATE KEY-----[^-]*-----END (RSA |EC )?PRIVATE KEY-----/[REDACTED-PRIVATE-KEY]/g' \
        -e 's/(client[_-]?secret[" :=]+)[^ "'\'']+/\1[REDACTED-CLIENT-SECRET]/gi' \
        -e 's/(authorization[" :]+)[^ "'\'']+/\1[REDACTED-AUTH]/gi'
}

# Get file size in bytes (cross-platform)
get_file_size() {
    local file="$1"
    if [[ "$OSTYPE" == "darwin"* ]]; then
        stat -f%z "$file" 2>/dev/null || echo 0
    else
        stat -c%s "$file" 2>/dev/null || echo 0
    fi
}

# Extract relevant errors from a large log file using Claude subagent
# This delegates focused log analysis to a quick Claude invocation
# Arguments: $1 = log file path, $2 = output summary file
extract_log_errors() {
    local log_file="$1"
    local output_file="$2"
    local file_size=$(get_file_size "$log_file")

    if [ "$file_size" -lt "$LARGE_FILE_THRESHOLD" ]; then
        # Small file - include directly (just tail/head for context)
        echo "=== Log: $(basename "$log_file") (${file_size} bytes) ===" >> "$output_file"
        head -n 50 "$log_file" >> "$output_file"
        echo "..." >> "$output_file"
        tail -n 100 "$log_file" >> "$output_file"
        return 0
    fi

    echo "  Preprocessing large log: $(basename "$log_file") (${file_size} bytes)"

    # Use Claude subagent to extract relevant errors from large log
    # Timeout of 60s for each subagent invocation
    # Using --add-dir to grant access to artifact directories (bypasses sandbox CWD restrictions)
    local subagent_output
    subagent_output=$(timeout 60 claude \
      --add-dir "${ARTIFACT_DIR}" --add-dir "/go/src" \
      --allowedTools "Read Grep Bash(grep:*) Bash(head:*) Bash(tail:*)" \
      --print "You are a log analysis assistant. Extract error messages, stack traces, and related context from this log file.

AVAILABLE TOOLS: You have access to Read, Grep, and Bash commands (grep, head, tail only). Use these tools to read and analyze the log file. Do NOT attempt to use any other tools.

Log file: $log_file

Read the log file and output a summary containing:

1. **Error lines**: All lines containing 'error', 'Error', 'ERROR', 'fatal', 'Fatal', 'FATAL', 'panic', 'failed', 'Failed'

2. **Stack traces**: Lines starting with goroutine, at, or containing .go: source references

3. **Package context**: When you find an error from a specific Go package (identified by path like 'pkg/controller/', 'velero/pkg/', 'internal/'), include 3-5 additional log lines from the SAME package that occurred shortly before the error. This provides context for what the component was doing when it failed.

4. **Timeout and failure messages**: Any lines indicating timeouts or test failures

5. **Correlation**: Group related errors together - if multiple errors reference the same resource (backup name, PVC, pod), keep them together with their context.

6. **Source references**: When you find errors from Velero packages (pkg/backup/, pkg/restore/, pkg/controller/, pkg/nodeagent/), note the file:line references for later source code investigation.

Format each error group as:
--- [package/component name] ---
[context lines from same package]
[ERROR line]
[stack trace if present]

Maximum output: 250 lines. If more errors exist, prioritize the last 150 lines (most recent).
Do NOT include debug/info level messages unless they are from the same package as an error and occurred within 10 lines before it." 2>/dev/null)

    if [ $? -eq 0 ] && [ -n "$subagent_output" ]; then
        echo "=== Log: $(basename "$log_file") (subagent extracted) ===" >> "$output_file"
        echo "$subagent_output" | head -n 200 >> "$output_file"
    else
        # Fallback: grep for errors if Claude fails
        echo "=== Log: $(basename "$log_file") (fallback grep) ===" >> "$output_file"
        grep -i -E '(error|fatal|panic|failed|timeout|exception)' "$log_file" 2>/dev/null | tail -n 100 >> "$output_file"
    fi
}

# Preprocess large must-gather and per-test logs into summaries
# Creates ${ARTIFACT_DIR}/preprocessed-logs.txt with extracted errors
preprocess_large_artifacts() {
    local summary_file="${ARTIFACT_DIR}/preprocessed-logs.txt"
    echo "# Preprocessed Log Summaries" > "$summary_file"
    echo "# Generated by analyze_failures.sh subagent preprocessing" >> "$summary_file"
    echo "# Timestamp: $(date -u '+%Y-%m-%d %H:%M:%S UTC')" >> "$summary_file"
    echo "" >> "$summary_file"

    local large_files_found=0

    # Find large log files in per-test directories
    if [ -d "${ARTIFACT_DIR}" ]; then
        while IFS= read -r log_file; do
            [ -z "$log_file" ] && continue
            large_files_found=$((large_files_found + 1))
            extract_log_errors "$log_file" "$summary_file"
            echo "" >> "$summary_file"
        done < <(find "${ARTIFACT_DIR}" -maxdepth 4 -name "*.log" -type f 2>/dev/null | while read f; do
            size=$(get_file_size "$f")
            if [ "$size" -ge "$LARGE_FILE_THRESHOLD" ]; then
                echo "$f"
            fi
        done | head -20)
    fi

    # Process must-gather pod logs if they're large
    if [ -d "${ARTIFACT_DIR}/must-gather" ]; then
        while IFS= read -r log_file; do
            [ -z "$log_file" ] && continue
            large_files_found=$((large_files_found + 1))
            extract_log_errors "$log_file" "$summary_file"
            echo "" >> "$summary_file"
        done < <(find "${ARTIFACT_DIR}/must-gather" -name "*.log" -type f 2>/dev/null | while read f; do
            size=$(get_file_size "$f")
            if [ "$size" -ge "$LARGE_FILE_THRESHOLD" ]; then
                echo "$f"
            fi
        done | head -20)
    fi

    if [ "$large_files_found" -eq 0 ]; then
        echo "No large log files found requiring preprocessing" >> "$summary_file"
    else
        echo "Preprocessed $large_files_found large log files"
    fi

    echo "$summary_file"
}

# Check for Claude CLI availability
if ! command -v claude &> /dev/null; then
    echo "⚠ Claude CLI not found in PATH"
    echo "Skipping Claude analysis (install with: npm install -g @anthropic-ai/claude-code)"
    exit $EXIT_CODE
fi

# Verify Vertex AI configuration
if [ -z "$GOOGLE_APPLICATION_CREDENTIALS" ] || [ -z "$ANTHROPIC_VERTEX_PROJECT_ID" ]; then
    echo "⚠ Vertex AI not configured (missing GOOGLE_APPLICATION_CREDENTIALS or ANTHROPIC_VERTEX_PROJECT_ID)"
    echo "Skipping Claude analysis"
    exit $EXIT_CODE
fi

if [ "$SKIP_CLAUDE" = "true" ]; then
    echo "Claude analysis skipped (SKIP_CLAUDE_ANALYSIS=true)"
    exit $EXIT_CODE
fi

if [ $EXIT_CODE -ne 0 ]; then
    echo "=== Test failures detected, invoking Claude analysis via Vertex AI ==="
    echo "GCP Project: $ANTHROPIC_VERTEX_PROJECT_ID"
    echo "Vertex AI Region: ${CLOUD_ML_REGION:-global}"
    echo "ARTIFACT_DIR: $ARTIFACT_DIR"

    # Preprocess large artifacts with subagent pattern
    echo "Preprocessing large log files..."
    PREPROCESSED_FILE=$(preprocess_large_artifacts)
    echo "Preprocessed summaries saved to: $PREPROCESSED_FILE"

    # Create analysis prompt with reference to preprocessed logs
    cat > "${ARTIFACT_DIR}/claude-prompt.txt" << 'PROMPT_EOF'
# OADP E2E Test Failure Analysis Request

You are analyzing a failed OADP (OpenShift API for Data Protection) E2E test run from Prow CI.

## Available Artifacts

1. **junit_report.xml**: Structured test results with pass/fail status and failure messages
2. **must-gather/**: OADP diagnostics collection with structure:
   - `clusters/<cluster-id>/oadp-must-gather-summary.md` - High-level summary
   - `clusters/<cluster-id>/namespaces/openshift-adp/` - OADP namespace resources (pod logs, DPA, BSL, VSL, backups, restores)
   - `clusters/<cluster-id>/cluster-scoped-resources/` - Cluster-wide resources (CSI drivers, storage classes)
3. **<TestName>/**: Per-test directories containing:
   - `openshift-adp/<pod-name>/*.log` - Velero, node-agent, plugin logs
   - `<app-namespace>/<pod-name>/*.log` - Application pod logs
4. **preprocessed-logs.txt**: Pre-extracted errors from large log files (>1MB)
   - Contains error summaries from large logs that were too big to analyze directly
   - Use this for quick access to relevant errors without reading full logs
5. **Velero Source Code**: `/go/src/github.com/openshift/velero/`
   - OpenShift's fork of Velero with OADP-specific patches
   - Use to investigate error messages originating from Velero packages
   - Key directories: `pkg/backup/`, `pkg/restore/`, `pkg/controller/`, `pkg/nodeagent/`
6. **OADP Operator Source Code**: `/go/src/github.com/openshift/oadp-operator/`
   - The OADP operator codebase being tested
   - Key directories: `internal/controller/`, `pkg/`, `api/v1alpha1/`
   - Use to investigate OADP-specific errors and reconciliation logic

**Note**: Prow's build-log.txt is written by CI infrastructure after tests complete and is NOT available during this analysis. Use the artifacts listed above.

## Known Flake Patterns

Read the known flake patterns from the source file:
- File: /go/src/github.com/openshift/oadp-operator/tests/e2e/lib/flakes.go

This file contains:
- `flakePatterns` slice with Issue, Description, and StringSearchPattern fields
- `errorIgnorePatterns` slice with strings that should be ignored in error analysis

Cross-reference failures against these patterns before diagnosing as real failures.

## Source Code Investigation

When analyzing failures, use the source code to understand error origins:

1. Locate the error message in the source code
2. Trace the code path that led to the error
3. Identify what conditions trigger the error
4. Check if the error is recoverable, transient, or indicates a real bug
5. Look for related error handling or retry logic

### Velero Source (`/go/src/github.com/openshift/velero/`)

Key Velero packages:
- `pkg/backup/` - Backup workflow and item processing
- `pkg/restore/` - Restore workflow and item processing
- `pkg/controller/` - Kubernetes controllers for backup/restore CRs
- `pkg/nodeagent/` - Node agent (restic/kopia) operations
- `pkg/persistence/` - Object storage operations
- `pkg/plugin/` - Plugin framework and built-in plugins

### OADP Operator Source (`/go/src/github.com/openshift/oadp-operator/`)

Key OADP packages:
- `internal/controller/` - DPA reconciler and other controllers
- `pkg/velero/` - Velero deployment and configuration
- `pkg/credentials/` - Cloud credential management
- `api/v1alpha1/` - CRD type definitions
- `tests/e2e/lib/` - E2E test utilities and flake patterns

## Analysis Tasks

1. Parse junit_report.xml to identify all failed tests and extract failure messages
2. Read preprocessed-logs.txt FIRST for quick access to errors from large log files
3. For each failed test:
   a. Check the per-test directory (<TestName>/) for pod logs with error details
   b. Review must-gather diagnostics for OADP component status
   c. Search must-gather pod logs for error patterns
   d. Identify root cause (real bug vs known flake vs environmental issue)
   e. Provide evidence-based diagnosis with file paths and log excerpts
   f. For backup/restore tests involving deployment readiness or startup probe failures,
      extract container startup timing data from per-test pod logs including:
      - Restore completion timestamp (look for "restore phase: Completed")
      - Pod condition timestamps: Initialized, PodReadyToStartContainers, ContainersReady, Ready
      - Deployment MinimumReplicasAvailable timestamp
      - Startup probe failure count (count "Startup probe failed" or "failed startup probe" lines)
      - Container restart count (count "will be restarted" lines)
      - IsDeploymentReady polling cycle count (count "deployment not available" or "Deployment todolist status" lines)
      - Total test duration from JUnit report or Ginkgo enter/exit timestamps
      Compare these against typical healthy baseline values (see Output Format below).
4. Summarize overall cluster health from must-gather
5. Provide actionable recommendations prioritized by severity

## Output Format

Generate a markdown document with this exact structure:

```markdown
# OADP E2E Test Failure Analysis
*Generated by Claude via Vertex AI on <timestamp>*

## Executive Summary
- **Total Tests**: X
- **Failed Tests**: Y
- **Known Flakes**: Z
- **Critical Issues**: N (real bugs requiring immediate attention)
- **Environmental Issues**: M (transient cloud/cluster issues)

## Failed Tests Analysis

### 1. <TestName> [CRITICAL|WARNING|FLAKE|ENVIRONMENTAL]

**Root Cause**: <One-sentence summary>

**Evidence**:
```
junit_report.xml: "<failure message from JUnit>"
must-gather: <specific resource status or log finding>
Pod logs (<TestName>/<namespace>/<pod>/*.log): "<error message>"
```

**Diagnosis**: <Detailed analysis of what went wrong and why>

**Likely Cause**: <Environmental/bug/config/flake with reasoning>

**Recommended Actions**:
1. <Specific action with details>
2. <Specific action with details>

**Related Issues**: <GitHub issue links if pattern matches known issues>

---

### 2. <Next Failed Test> [...]

[Repeat for each failed test]

## Known Flakes Detected

- ✓ VolumeSnapshotBeingCreated race condition (matched pattern in <file>)
- ✗ AWS rate limiting (not detected)

## Container Startup Timing Comparison

For each failed backup/restore test that involves deployment readiness or startup probe failures,
extract timing data from per-test pod logs and present a comparison table against typical healthy
baseline values. Skip this section for tests that did not involve deployment readiness checks.

**Test**: <TestName>

| Metric | Typical (healthy baseline) | This Run (failed) |
|--------|---------------------------|--------------------|
| Post-restore container startup | ~25-30 seconds | <actual seconds or "Never succeeded (Xs timeout)"> |
| Startup probe failures | 0 | <count from pod logs> |
| Container restarts | 0 | <count from pod logs> |
| IsDeploymentReady polls needed | 2-3 | <count or "timed out at X min"> |
| Total test duration | ~3-4 minutes | <actual duration from JUnit> |

**How to extract these values from logs**:
- Restore completion: look for "restore phase: Completed" timestamp in per-test logs
- Container ready: look for ContainersReady or MinimumReplicasAvailable pod condition timestamps
- Startup probe failures: count "Startup probe failed" or "failed startup probe" log lines
- Container restarts: count "will be restarted" log lines
- IsDeploymentReady polls: count "deployment not available" or "Deployment todolist status" log lines
- Test duration: from JUnit report or Ginkgo "[It]" enter/exit timestamps

Note: The "Typical" column values are baseline reference values from healthy OADP E2E runs on AWS.
Adjust if running on a different cloud provider. If multiple backup/restore tests failed, include
a separate table for each.

## Cluster Health Summary

From must-gather analysis:

**OADP Components**:
- Velero deployment: <status, restart count, resource usage>
- Node Agent daemonset: <X/Y running, any issues>
- Backup Storage Location: <Available/Unavailable, last sync time>
- Volume Snapshot Location: <Available/Unavailable, provider status>

**Cluster Resources**:
- CSI drivers: <driver names and status>
- Storage classes: <available SCs>
- Resource pressure: <CPU/memory/storage issues if any>

**Recent Events**:
<Significant namespace events from must-gather>

## Recommendations (Prioritized)

### Immediate Actions (Critical)
1. <Action for critical bug>
2. <Action for critical bug>

### Investigation Needed
1. <Item requiring further investigation>
2. <Item requiring further investigation>

### Flake Handling
1. <Suggestion for known flakes>

### Configuration Review
1. <Config changes that might help>

## Analysis Confidence

- **High Confidence**: <List tests where root cause is clear>
- **Medium Confidence**: <List tests needing more data>
- **Low Confidence**: <List tests with ambiguous failures>

## Suggested Next Steps for Developer

1. Review critical issues first (prioritized above)
2. Check if failures match existing GitHub issues
3. Re-run flakes to confirm transient nature
4. Investigate environmental issues in cluster/cloud provider

## Must-Gather Improvement Suggestions

If information was missing or incomplete during analysis, list what additional data would have helped:

### Missing Data That Would Have Helped
- <What was needed and why it would have helped diagnosis>
- <Specific resource/log/metric that was missing>

### Recommended Must-Gather Enhancements
1. **<Category>**: <Specific improvement suggestion>
   - Current gap: <What's missing>
   - Suggested addition: <What to collect>
   - Example: <Concrete example of the data needed>

Examples of potential improvements:
- Additional pod logs (e.g., init containers, sidecar containers)
- Specific CRD status fields not currently captured
- Cluster-level resources affecting OADP (NetworkPolicies, ResourceQuotas)
- Timing/metrics data (pod startup times, API latencies)
- Cloud provider specific diagnostics (S3 bucket policies, IAM roles)
```

## Important Guidelines

- Be specific: Cite file paths and excerpts from artifacts (JUnit, must-gather, per-test logs)
- Be evidence-based: Don't speculate without supporting log evidence
- Distinguish failure types: Real bugs vs flakes vs environmental vs configuration
- Be actionable: Recommendations should be concrete and implementable
- Be concise: Developers need quick insights, not verbose analysis
- Cross-reference: Link similar failures across multiple tests
- Prioritize: Put critical issues before warnings before flakes
- Use preprocessed-logs.txt: Check this file first for errors from large log files
- Timing comparison: For any backup/restore test failure involving deployment readiness
  timeouts or startup probe failures, ALWAYS include the Container Startup Timing Comparison
  table. Extract actual timing values from per-test pod logs and compare against the healthy
  baseline. This is critical for distinguishing environmental slowness from real bugs.
- Must-gather feedback: When you cannot determine root cause due to missing information,
  explicitly note what additional must-gather data would have helped. This feedback loop
  improves future debugging capabilities.
PROMPT_EOF

    # Count failed tests from JUnit (count individual test failures, not just suites)
    FAILED_COUNT=0
    if [ -f "${ARTIFACT_DIR}/junit_report.xml" ]; then
        # Count <failure> tags for individual test failures
        FAILED_COUNT=$(grep -c '<failure' "${ARTIFACT_DIR}/junit_report.xml" 2>/dev/null || echo "0")
    fi

    echo "Found $FAILED_COUNT test failures"
    echo "Invoking Claude for analysis..."

    # Create temp file for Claude output to properly capture exit code
    TEMP_OUTPUT=$(mktemp)
    trap "rm -f $TEMP_OUTPUT" EXIT

    # Invoke Claude via Vertex AI
    # Using --print flag for headless/non-interactive mode suitable for CI automation
    # Using --add-dir to grant access to artifact directories (bypasses sandbox CWD restrictions)
    # Write to temp file first, then apply redaction - this avoids pipefail masking Claude exit code
    timeout 600 claude \
      --add-dir "${ARTIFACT_DIR}" --add-dir "/go/src" \
      --allowedTools "Read Grep Glob Bash(ls:*) Bash(cat:*) Bash(head:*) Bash(tail:*) Bash(grep:*) Bash(find:*) Bash(wc:*)" \
      --print "You are analyzing OADP E2E test failures from Prow CI.

AVAILABLE TOOLS: You have access to the following tools ONLY:
- Read: Read files from ${ARTIFACT_DIR} and /go/src directories
- Grep: Search file contents
- Glob: Find files by pattern
- Bash: ls, cat, head, tail, grep, find, wc commands only

Use these tools to read and analyze artifacts. Do NOT attempt to use Write, Edit, WebFetch, or any other tools.

Read the analysis instructions in: ${ARTIFACT_DIR}/claude-prompt.txt

Analyze these artifacts:
1. JUnit report: ${ARTIFACT_DIR}/junit_report.xml
2. Preprocessed log errors: ${ARTIFACT_DIR}/preprocessed-logs.txt (check this FIRST for large log summaries)
3. Must-gather: ${ARTIFACT_DIR}/must-gather/
4. Per-test failure directories: ${ARTIFACT_DIR}/*/
5. Velero source code: /go/src/github.com/openshift/velero/
6. OADP operator source code: /go/src/github.com/openshift/oadp-operator/

When errors reference Velero or OADP packages, read the relevant source code to understand:
- What conditions trigger the error
- If there's retry logic that should have handled it
- If this is a known limitation or edge case

Note: Prow's build-log.txt is NOT available during this analysis (it's written after tests complete).
Focus on JUnit results, preprocessed log summaries, must-gather diagnostics, per-test pod logs, and source code investigation.

Generate comprehensive failure analysis following the output format specified in the prompt.
Focus on actionable insights and clear root cause identification.

IMPORTANT SECURITY NOTE:
Do NOT include any API keys, tokens, passwords, or service account keys in your analysis.
If you encounter credentials in logs, reference them generically (e.g., \"AWS credentials found in log\")." > "$TEMP_OUTPUT" 2>&1

    CLAUDE_EXIT=$?

    # Apply secret redaction to output
    redact_secrets < "$TEMP_OUTPUT" > "${ARTIFACT_DIR}/claude-failure-analysis.md"

    if [ $CLAUDE_EXIT -eq 0 ]; then
        echo "✓ Claude analysis completed successfully (with secret redaction)"
        echo "✓ Analysis saved to: ${ARTIFACT_DIR}/claude-failure-analysis.md"

        # Show summary (first 80 lines)
        echo ""
        echo "=== Claude Analysis Preview ==="
        head -80 "${ARTIFACT_DIR}/claude-failure-analysis.md"
        echo "=== (Full analysis available in Prow artifacts) ==="
    elif [ $CLAUDE_EXIT -eq 124 ]; then
        echo "✗ Claude analysis timed out after 10 minutes"
        echo "Large artifacts may have exceeded token limits"
        echo "Partial analysis may be in ${ARTIFACT_DIR}/claude-failure-analysis.md"
    else
        echo "✗ Claude analysis failed (exit code: $CLAUDE_EXIT)"
        echo "Check ${ARTIFACT_DIR}/claude-failure-analysis.md for error details"
    fi

    # Cleanup temp file (trap handles this, but explicit is clearer)
    rm -f "$TEMP_OUTPUT"
else
    echo "Tests passed, skipping Claude analysis"
fi

exit $EXIT_CODE

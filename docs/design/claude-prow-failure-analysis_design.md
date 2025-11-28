# Design proposal: Claude-Powered Failure Analysis in Prow CI

## Abstract

Automatically analyze OADP E2E test failures in Prow CI using Claude Code via Google Vertex AI.
After Ginkgo test suite completes with failures, invoke Claude to analyze JUnit reports, must-gather diagnostics, and per-test pod logs, then output a comprehensive root cause analysis to Prow's artifact storage for developer consumption.

## Background

OADP operators E2E test suite runs in OpenShift Prow CI using Ginkgo framework.
When tests fail, developers must manually sift through must-gather archives, JUnit reports, and per-test pod logs to diagnose root causes.
This manual analysis is time-consuming and requires deep domain knowledge of Velero, CSI snapshots, cloud provider APIs, and Kubernetes internals.
The repository already has comprehensive artifact collection infrastructure including must-gather integration, JUnit reports, and per-test failure logs.
We have access to Google Vertex AI for Claude inference, which can be leveraged to automate failure analysis.

**Note**: Prow's `build-log.txt` is written by CI infrastructure **after** tests complete and is NOT available during analysis. This design relies on artifacts generated during test execution: JUnit reports, must-gather diagnostics, and per-test pod log directories.

## Goals

- Automatically analyze test failures after Ginkgo suite completes using Claude via Vertex AI
- Output structured analysis to `${ARTIFACT_DIR}/claude-failure-analysis.md` for Prow GCS storage
- Minimal impact to test execution time (analysis runs post-suite, not during tests)
- Cost-effective implementation (only analyze on failures, not successful runs)
- Graceful degradation (Claude failure doesn't block test result reporting)

## Non Goals

- Live cluster diagnostics during test execution (agentic real-time monitoring)
- Auto-remediation of failures (no automated fixes)
- Analysis of successful test runs (cost control)
- Real-time streaming analysis (only post-suite batch analysis)

## High-Level Design

Add Claude CLI to the Prow CI container image (`build/ci-Dockerfile`).
Create a wrapper script (`tests/e2e/scripts/analyze_failures.sh`) that runs after Ginkgo exits.
If tests failed, invoke Claude with paths to JUnit reports, must-gather artifacts, and per-test log directories.
Claude analyzes artifacts using Vertex AI and generates a markdown summary.
Output is written to `${ARTIFACT_DIR}/claude-failure-analysis.md` where Prow uploads it to GCS.
Modify Makefile `test-e2e` target to invoke the analysis script regardless of test exit code.

## Detailed Design

### Container Modifications

**File**: `build/ci-Dockerfile`

Add Claude CLI installation after kubectl installation:

```dockerfile
FROM quay.io/konveyor/builder AS builder

WORKDIR /go/src/github.com/openshift/oadp-operator

COPY ./ .

# Make analysis script executable for CI execution
RUN chmod +x tests/e2e/scripts/analyze_failures.sh

# Install kubectl (multi-arch)
ARG TARGETARCH
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/${TARGETARCH}/kubectl" && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin/

# Install Node.js and Claude CLI
# Using NodeSource setup script for RHEL-based images
RUN curl -fsSL https://rpm.nodesource.com/setup_20.x | bash - && \
    dnf install -y nodejs && \
    npm install -g @anthropic-ai/claude-code && \
    dnf clean all

RUN go mod download && \
    mkdir -p $(go env GOCACHE) && \
    chmod -R 777 ./ $(go env GOCACHE) $(go env GOPATH)
```

**Note**: The `COPY ./ .` command includes the `.claude/` directory with permissions configuration (see below).

### Claude Code Permissions Configuration

Claude Code permissions are configured through two mechanisms:

1. **Runtime `--allowedTools` flag** (primary): Explicitly grants file access at invocation time
2. **`.claude/config.json`** (secondary): General tool permissions and deny rules

#### Runtime Permissions via --allowedTools

The analysis script uses the `--allowedTools` CLI flag to explicitly grant Read permissions for artifact paths:

```bash
claude --print \
  --allowedTools "Read(${ARTIFACT_DIR}/**) Read(/go/src/**) Grep Glob Bash(ls:*) Bash(cat:*) ..." \
  "prompt..."
```

**Why runtime permissions instead of config file?**

Claude Code's sandbox mode restricts filesystem access to the current working directory (CWD) and its subdirectories. In Prow CI:
- CWD is `/go/src/github.com/openshift/oadp-operator`
- Artifacts are at `/logs/artifacts/` (outside CWD)

Path-specific permissions in `.claude/config.json` (e.g., `Read(/logs/**)`) are overridden by sandbox CWD restrictions. The `--allowedTools` flag bypasses these restrictions by explicitly granting permissions at invocation time.

#### Static Configuration File

**File**: `.claude/config.json`

General tool permissions and deny rules (path permissions handled at runtime):

```json
{
  "permissions": {
    "allow": [
      "Read",
      "Glob",
      "Grep",
      "Bash(ls:*)",
      "Bash(cat:*)",
      "Bash(head:*)",
      "Bash(tail:*)",
      "Bash(grep:*)",
      "Bash(sed:*)",
      "Bash(awk:*)",
      "Bash(find:*)",
      "Bash(tree:*)",
      "Bash(wc:*)",
      "Bash(sort:*)",
      "Bash(uniq:*)",
      "Bash(cut:*)",
      "Bash(tr:*)",
      "Bash(jq:*)",
      "Bash(less:*)",
      "Bash(more:*)",
      "Bash(file:*)",
      "Bash(du:*)",
      "Bash(stat:*)",
      "Bash(zcat:*)",
      "Bash(gunzip:*)",
      "Bash(tar:*)"
    ],
    "deny": [
      "Write",
      "Edit",
      "Bash(rm:*)",
      "Bash(curl:*)",
      "Bash(wget:*)",
      "Bash(git:push*)",
      "Bash(docker:*)",
      "Bash(kubectl:delete*)",
      "Bash(kubectl:apply*)",
      "Bash(make:*)",
      "WebFetch",
      "WebSearch"
    ]
  }
}
```

**Permission Design**:
- **Read-only analysis**: Claude can read logs, search files, and run analysis commands
- **No modifications**: Denies Write, Edit, and destructive Bash commands
- **Tool allowlist**: Bash commands for log analysis including compression tools (tar, zcat, gunzip)
- **Network isolation**: Denies WebFetch and WebSearch to prevent external calls

This configuration is automatically included in the container via `COPY ./ .` in the Dockerfile.

### Analysis Script Implementation

**File**: `tests/e2e/scripts/analyze_failures.sh` (new)

**Key Features**:

1. **Claude CLI availability check**: Validates `claude` command exists before attempting analysis
2. **Proper exit code capture**: Writes to temp file first to avoid pipefail issues
3. **Large artifact preprocessing**: Summarizes high-noise log files via Claude subagents
4. **Subagent pattern**: Delegates log extraction to focused Claude invocations that include package context (lines from the same Go package that emitted errors)
5. **Secret redaction**: Automatically redacts credentials from all output

```bash
#!/bin/bash
# Analyze test failures with Claude via Vertex AI after Ginkgo suite completes
# Only runs if tests failed and Claude analysis is not skipped
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

# Extract relevant errors from a large log file using Claude subagent
# This delegates focused log analysis to a quick Claude invocation
# Arguments: $1 = log file path, $2 = output summary file
extract_log_errors() {
    local log_file="$1"
    local output_file="$2"
    local file_size=$(stat -f%z "$log_file" 2>/dev/null || stat -c%s "$log_file" 2>/dev/null || echo 0)

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
    # Using --allowedTools to explicitly grant file access (bypasses sandbox CWD restrictions)
    local subagent_output
    subagent_output=$(timeout 60 claude \
      --allowedTools "Read(${ARTIFACT_DIR}/**) Read(/go/src/**) Grep Bash(grep:*) Bash(head:*) Bash(tail:*)" \
      --print "You are a log analysis assistant. Extract error messages, stack traces, and related context from this log file.

AVAILABLE TOOLS: You have access to Read, Grep, and Bash commands (grep, head, tail only). Use these tools to read and analyze the log file. Do NOT attempt to use any other tools.

Log file: $log_file

Read the log file and output a summary containing:

1. **Error lines**: All lines containing 'error', 'Error', 'ERROR', 'fatal', 'Fatal', 'FATAL', 'panic', 'failed', 'Failed'

2. **Stack traces**: Lines starting with goroutine, at, or containing .go: source references

3. **Package context**: When you find an error from a specific Go package (identified by path like 'pkg/controller/', 'velero/pkg/', 'internal/'), include 3-5 additional log lines from the SAME package that occurred shortly before the error. This provides context for what the component was doing when it failed.

4. **Timeout and failure messages**: Any lines indicating timeouts or test failures

5. **Correlation**: Group related errors together - if multiple errors reference the same resource (backup name, PVC, pod), keep them together with their context.

Format each error group as:
--- [package/component name] ---
[context lines from same package]
[ERROR line]
[stack trace if present]

Maximum output: 250 lines. If more errors exist, prioritize the last 150 lines (most recent).
Do NOT include debug/info level messages unless they are from the same package as an error and occurred within 10 lines before it." 2>/dev/null)

    if [ $? -eq 0 ] && [ -n "$subagent_output" ]; then
        echo "=== Log: $(basename "$log_file") (subagent extracted) ===" >> "$output_file"
        echo "$subagent_output" | head -n 250 >> "$output_file"
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
    echo "" >> "$summary_file"

    local large_files_found=0

    # Find large log files in per-test directories
    if [ -d "${ARTIFACT_DIR}" ]; then
        while IFS= read -r log_file; do
            [ -z "$log_file" ] && continue
            large_files_found=$((large_files_found + 1))
            extract_log_errors "$log_file" "$summary_file"
            echo "" >> "$summary_file"
        done < <(find "${ARTIFACT_DIR}" -name "*.log" -size +${LARGE_FILE_THRESHOLD}c 2>/dev/null | head -20)
    fi

    # Process must-gather pod logs if they're large
    if [ -d "${ARTIFACT_DIR}/must-gather" ]; then
        while IFS= read -r log_file; do
            [ -z "$log_file" ] && continue
            large_files_found=$((large_files_found + 1))
            extract_log_errors "$log_file" "$summary_file"
            echo "" >> "$summary_file"
        done < <(find "${ARTIFACT_DIR}/must-gather" -name "*.log" -size +${LARGE_FILE_THRESHOLD}c 2>/dev/null | head -20)
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

**Note**: Prow's build-log.txt is written by CI infrastructure after tests complete and is NOT available during this analysis. Use the artifacts listed above.

## Known Flake Patterns

Read the known flake patterns from the source file:
- File: /go/src/github.com/openshift/oadp-operator/tests/e2e/lib/flakes.go

This file contains:
- `flakePatterns` slice with Issue, Description, and StringSearchPattern fields
- `errorIgnorePatterns` slice with strings that should be ignored in error analysis

Cross-reference failures against these patterns before diagnosing as real failures.

## Analysis Tasks

1. Parse junit_report.xml to identify all failed tests and extract failure messages
2. Read preprocessed-logs.txt FIRST for quick access to errors from large log files
3. For each failed test:
   a. Check the per-test directory (<TestName>/) for pod logs with error details
   b. Review must-gather diagnostics for OADP component status
   c. Search must-gather pod logs for error patterns
   d. Identify root cause (real bug vs known flake vs environmental issue)
   e. Provide evidence-based diagnosis with file paths and log excerpts
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
    # Using --allowedTools to explicitly grant file access (bypasses sandbox CWD restrictions)
    # Write to temp file first, then apply redaction - this avoids pipefail masking Claude exit code
    timeout 600 claude \
      --allowedTools "Read(${ARTIFACT_DIR}/**) Read(/go/src/**) Grep Glob Bash(ls:*) Bash(cat:*) Bash(head:*) Bash(tail:*) Bash(grep:*) Bash(find:*) Bash(wc:*)" \
      --print "You are analyzing OADP E2E test failures from Prow CI.

AVAILABLE TOOLS: You have access to the following tools ONLY:
- Read: Read files from ${ARTIFACT_DIR}/** and /go/src/**
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

Note: Prow's build-log.txt is NOT available during this analysis (it's written after tests complete).
Focus on JUnit results, preprocessed log summaries, must-gather diagnostics, and per-test pod logs.

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

        # Show summary (first 80 lines) - also redacted
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
```

**Key Implementation Details**:

1. **Claude CLI Check**: The script validates `claude` command exists before attempting any analysis, providing a clear error message if missing.

2. **Runtime Permissions via --allowedTools**: Claude Code's sandbox mode restricts filesystem access to the current working directory. Since artifacts are at `/logs/artifacts/` (outside the CWD `/go/src/github.com/openshift/oadp-operator`), the script uses the `--allowedTools` CLI flag to explicitly grant Read permissions:
   ```bash
   claude --print --allowedTools "Read(${ARTIFACT_DIR}/**) Read(/go/src/**) Grep Glob ..."
   ```
   This bypasses sandbox CWD restrictions and ensures Claude can access artifact files.

3. **Proper Exit Code Capture**: Instead of piping Claude output directly through `redact_secrets` (which could mask the real exit code due to `pipefail`), the script:
   - Writes Claude output to a temp file
   - Captures Claude's exit code separately
   - Then applies redaction to the temp file
   - Uses trap for cleanup

4. **Subagent Pattern for Large Logs**: The `extract_log_errors()` function invokes Claude as a focused subagent to extract only error-relevant lines from large log files (>1MB). This:
   - Reduces token usage for the main analysis
   - Increases accuracy by pre-filtering noise
   - Has fallback to grep if subagent fails

5. **Preprocessing Pipeline**: Before main analysis, `preprocess_large_artifacts()` scans for large log files and creates `preprocessed-logs.txt` with extracted errors. The main Claude analysis references this file for quick access to relevant errors.

**File Permissions**: The script is made executable in `build/ci-Dockerfile` during container build (see Dockerfile section above).

### Makefile Integration

**File**: `Makefile`

Modify the `test-e2e` target (around line 855) to invoke analysis script:

```makefile
.PHONY: test-e2e
test-e2e: test-e2e-setup install-ginkgo
	ginkgo run -mod=mod $(GINKGO_FLAGS) $(GINKGO_ARGS) tests/e2e/ -- \
	  -settings=$(SETTINGS_TMP)/oadpcreds \
	  -credentials=$(CLOUD_CREDENTIALS_LOCATION) \
	  -provider=$(PROVIDER) \
	  -ci-credentials=$(CI_CRED_LOCATION) \
	  -velero-namespace=$(VELERO_NAMESPACE) \
	  -velero-instance=$(VELERO_INSTANCE_NAME) \
	  -artifact-dir=$(ARTIFACT_DIR) \
	  -kvm-emulation=$(KVM_EMULATION) \
	  -skip-must-gather=$(SKIP_MUST_GATHER) \
	  -skip-flakes-skip=$(SKIP_FLAKES_SKIP) \
	  || EXIT_CODE=$$?; \
	if [ "$(OPENSHIFT_CI)" = "true" ]; then \
		./tests/e2e/scripts/analyze_failures.sh $${EXIT_CODE:-0}; \
	fi; \
	exit $${EXIT_CODE:-0}
```

Key changes:
- Capture Ginkgo exit code in `EXIT_CODE` variable
- Only run analysis when `OPENSHIFT_CI=true` (prevents running on local dev)
- Invoke script with exit code as parameter (script made executable in ci-Dockerfile)
- Preserve original exit code for Prow result reporting

### Vertex AI Configuration

**Environment Variables Required**:

| Variable | Description | Example Value | Set By |
|----------|-------------|---------------|--------|
| `GOOGLE_APPLICATION_CREDENTIALS` | Path to GCP service account JSON key | `/var/run/oadp-credentials/gcp-claude-code-credentials` | Vault mount |
| `CLAUDE_CODE_USE_VERTEX` | Enable Claude Code Vertex AI | `1` | Makefile |
| `CLOUD_ML_REGION` | Vertex AI region (global recommended) | `global` | Makefile |
| `ANTHROPIC_VERTEX_PROJECT_ID` | GCP project ID for Vertex AI | `openshift-ci-vertex` | Vault file |
| `SKIP_CLAUDE_ANALYSIS` | Opt-out flag | `true` (to skip) | Optional |

**Prow CI Configuration**:

The existing oadp-credentials collection already provides the `/var/run/oadp-credentials/` mount.
Only environment variables need to be added to the CI configuration.

File: `ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-oadp-dev__4.20.yaml` (in openshift/release repo)

```yaml
tests:
- as: e2e-aws
  steps:
    test:
    - as: test
      credentials:
      # Existing credentials (already provides /var/run/oadp-credentials/)
      - namespace: test-credentials
        name: oadp-credentials
        mount_path: /var/run/oadp-credentials
      env:
      # Existing environment variables
      - name: CLOUD_CREDENTIALS
        value: /var/run/oadp-credentials/credentials
      - name: PROVIDER
        value: aws
      # ... other existing vars ...

      # NEW: Vertex AI configuration (add these environment variables)
      - name: GOOGLE_APPLICATION_CREDENTIALS
        value: /var/run/oadp-credentials/gcp-claude-code-credentials
      - name: CLAUDE_CODE_USE_VERTEX
        value: "1"
      - name: CLOUD_ML_REGION
        value: global
      - name: ANTHROPIC_VERTEX_PROJECT_ID
        value: openshift-ci-vertex

      commands: |
        export ARTIFACT_DIR=${ARTIFACT_DIR}
        export VELERO_NAMESPACE=openshift-adp
        make test-e2e
      from: test-oadp-operator
```

**Adding Vertex AI Key to Existing Vault Collection** (OpenShift CI admin task):

```bash
# Create GCP service account in appropriate GCP project
gcloud iam service-accounts create oadp-ci-vertex-claude \
  --display-name="OADP CI Vertex AI Claude" \
  --project=openshift-ci-vertex

# Grant Vertex AI User role
gcloud projects add-iam-policy-binding openshift-ci-vertex \
  --member="serviceAccount:oadp-ci-vertex-claude@openshift-ci-vertex.iam.gserviceaccount.com" \
  --role="roles/aiplatform.user"

# Create and download key
gcloud iam service-accounts keys create gcp-claude-code-credentials.json \
  --iam-account=oadp-ci-vertex-claude@openshift-ci-vertex.iam.gserviceaccount.com

# Add to existing oadp-credentials vault collection
# Contact OpenShift CI team to add two files to the existing oadp-credentials collection:
# 1. gcp-claude-code-credentials.json -> Service account key file
# 2. gcp-claude-code-project-id -> Plain text file containing GCP project ID (e.g., "openshift-ci-vertex")
#
# Collection: oadp-credentials (already exists)
# Files in collection:
#   - gcp-claude-code-credentials (JSON key)
#   - gcp-claude-code-project-id (project ID as plain text)
# Namespace: test-credentials
# Will appear at:
#   - /var/run/oadp-credentials/gcp-claude-code-credentials
#   - /var/run/oadp-credentials/gcp-claude-code-project-id

# Secure cleanup
rm gcp-claude-code-credentials.json
```

The OpenShift CI team manages the vault backend and the existing `oadp-credentials` collection.
Adding the Vertex AI files to this collection does not require any openshift/release configuration changes - the mount path already exists.

**Project ID File**:
The `gcp-claude-code-project-id` file contains only the GCP project ID as plain text (e.g., `openshift-ci-vertex`).
This allows the Makefile to read the project ID dynamically without hardcoding it.

### Artifact Structure

Prow GCS artifact layout:

```
gs://origin-ci-test/pr-logs/pull/openshift_oadp-operator/<PR>/<job-name>/<build-id>/
├── build-log.txt                          # Ginkgo stdout/stderr (NOT available during analysis)
├── artifacts/
│   ├── junit_report.xml                   # Test results (PRIMARY - available)
│   ├── must-gather/                       # OADP diagnostics (available)
│   │   └── clusters/<cluster-id>/
│   │       ├── oadp-must-gather-summary.md
│   │       ├── namespaces/
│   │       │   └── openshift-adp/
│   │       │       ├── pods/
│   │       │       ├── backups/
│   │       │       └── restores/
│   │       └── cluster-scoped-resources/
│   ├── MySQL application CSI/             # Per-test logs (available)
│   │   ├── openshift-adp/
│   │   │   └── velero-<hash>/
│   │   │       ├── velero.log
│   │   │       ├── node-agent.log
│   │   │       └── aws-plugin.log
│   │   └── mysql-persistent/
│   │       └── mysql-<hash>/
│   │           └── mysql.log
│   ├── claude-prompt.txt                  # NEW: Analysis prompt (for debugging)
│   └── claude-failure-analysis.md         # NEW: Claude output
└── finished.json
```

**Note**: `build-log.txt` is written by Prow CI infrastructure after tests complete. The analysis script runs before this file exists, so Claude analyzes JUnit reports, must-gather, and per-test log directories instead.

Access URL pattern:
```
https://prow.ci.openshift.org/view/gs/origin-ci-test/pr-logs/pull/openshift_oadp-operator/<PR>/<job-name>/<build-id>/artifacts/claude-failure-analysis.md
```

### Claude Output Format Example

**File**: `${ARTIFACT_DIR}/claude-failure-analysis.md`

```markdown
# OADP E2E Test Failure Analysis
*Generated by Claude via Vertex AI on 2025-01-20 15:34:22 UTC*

## Executive Summary
- **Total Tests**: 42
- **Failed Tests**: 3
- **Known Flakes**: 1
- **Critical Issues**: 1 (MySQL VolumeSnapshot timeout)
- **Environmental Issues**: 1 (AWS API rate limiting)

## Failed Tests Analysis

### 1. MySQL application CSI [CRITICAL]

**Root Cause**: VolumeSnapshot reconciliation timeout after 10 minutes

**Evidence**:
```
junit_report.xml: "VolumeSnapshot mysql-pvc not ready after 10m0s"
must-gather: clusters/12345678/namespaces/openshift-adp/volumesnapshots.yaml
  status.readyToUse: false
  status.error: "snapshot-12345: rpc error: code = DeadlineExceeded"
Pod logs (MySQL application CSI/openshift-adp/node-agent-abc123/node-agent.log):
  "CSI driver timeout creating snapshot for pvc mysql-pvc"
```

**Diagnosis**: The CSI driver failed to create VolumeSnapshot within the allocated 10-minute timeout.
The error indicates a DeadlineExceeded RPC error from the CSI driver, suggesting the AWS EBS snapshot creation itself timed out or was throttled.
Must-gather shows the VolumeSnapshot exists but remains in pending state with readyToUse=false.

**Likely Cause**: AWS API rate limiting or CSI driver resource exhaustion.
The cluster may have hit AWS API rate limits for EBS snapshot operations, or the CSI driver pod may be under-resourced.

**Recommended Actions**:
1. Check AWS CloudWatch for EBS API throttling events in the test cluster's region
2. Review CSI driver pod resource requests/limits - increase if CPU/memory constrained
3. Consider increasing VolumeSnapshot timeout from 10m to 15m in test configuration
4. Add retry logic with exponential backoff for snapshot creation

**Related Issues**: Pattern matches https://github.com/kubernetes-csi/external-snapshotter/pull/876 (VolumeSnapshotBeingCreated race condition)

---

### 2. MongoDB FSB application [FLAKE]

**Root Cause**: Known flake - transient S3 bucket write error during FS backup

**Evidence**:
```
junit_report.xml: "Backup failed: error uploading backup"
Pod logs (MongoDB FSB application/openshift-adp/velero-xyz/velero.log):
  "Error copying image: writing blob: unexpected EOF"
  "Backup failed: error uploading backup: RequestTimeout: upload timeout"
```

**Diagnosis**: This matches the known flake pattern for transient S3 errors documented in tests/e2e/lib/flakes.go.

**Likely Cause**: Transient network issue or S3 service hiccup (see Velero issue #5856)

**Recommended Actions**:
1. Re-run test to confirm flake vs persistent issue
2. If persistent, check S3 bucket region configuration matches BSL_REGION

**Related Issues**: https://github.com/vmware-tanzu/velero/issues/5856

---

### 3. DPA deployment validation [ENVIRONMENTAL]

**Root Cause**: Image pull backoff for velero-plugin-for-aws

**Evidence**:
```
junit_report.xml: "DPA not ready: velero deployment not available"
must-gather events: "Failed to pull image quay.io/konveyor/velero-plugin-for-aws:v1.10.1"
must-gather pod status: "ErrImagePull: rate limit exceeded"
```

**Diagnosis**: Quay.io rate limiting prevented pulling the AWS plugin image.
This is an environmental issue with the container registry, not a code defect.

**Likely Cause**: CI cluster hit Quay.io anonymous rate limits

**Recommended Actions**:
1. Configure authenticated Quay.io pull secret in openshift-adp namespace
2. Use internal mirror/cache for frequently pulled images
3. This will resolve on retry when rate limit window resets

**Related Issues**: None (environmental)

## Known Flakes Detected

- ✓ S3 transient write errors (matched "Error copying image: writing blob" in per-test logs)
- ✗ VolumeSnapshotBeingCreated race condition (not detected - MySQL failure is different)

## Cluster Health Summary

From must-gather analysis:

**OADP Components**:
- Velero deployment: 1/1 running, 0 restarts, CPU 45m/200m, Memory 128Mi/512Mi
- Node Agent daemonset: 3/3 running on all worker nodes, no errors
- Backup Storage Location: Available, last sync 2m ago, 127 backups
- Volume Snapshot Location: Available, AWS provider configured for us-east-1

**Cluster Resources**:
- CSI drivers: ebs.csi.aws.com (v1.28.0) - Ready
- Storage classes: gp3-csi (default), gp2-csi
- Resource pressure: None detected on worker nodes

**Recent Events**:
- Warning: ImagePullBackOff for AWS plugin (rate limit)
- Error: VolumeSnapshot mysql-pvc timeout after 10m

## Recommendations (Prioritized)

### Immediate Actions (Critical)
1. Investigate MySQL VolumeSnapshot timeout - check AWS API throttling and CSI driver resources
2. Consider increasing snapshot timeout from 10m to 15m to accommodate slower snapshot operations

### Investigation Needed
1. Review AWS CloudWatch metrics for EBS API throttling in us-east-1
2. Analyze CSI driver pod CPU/memory usage patterns during snapshot creation
3. Check if other tests in the suite are creating many snapshots concurrently (resource contention)

### Flake Handling
1. Re-run MongoDB FSB test - likely to pass on retry (known S3 flake)
2. Update flake detection if this pattern recurs frequently

### Configuration Review
1. Add authenticated Quay.io pull secrets to prevent image pull rate limiting
2. Consider using image mirrors or caching proxy for CI

## Analysis Confidence

- **High Confidence**: MongoDB FSB (known flake pattern), DPA deployment (clear image pull error)
- **Medium Confidence**: MySQL CSI (likely AWS throttling, but needs CloudWatch verification)
- **Low Confidence**: None

## Suggested Next Steps for Developer

1. **Priority 1**: Check AWS CloudWatch for EBS throttling in the test cluster (MySQL failure)
2. **Priority 2**: Re-run the full suite to confirm MongoDB FSB as flake
3. **Priority 3**: Work with CI team to add Quay.io auth (DPA failure)
4. If MySQL failure persists after resolving AWS throttling, increase snapshot timeout and add retries
```

## Alternatives Considered

### Ginkgo AfterSuite Hook vs Post-Test Wrapper Script

**Option A**: Implement Claude analysis in Ginkgo `AfterSuite` hook
- Pros: Integrated with test framework, access to Go test context
- Cons: Claude failure could interfere with test reporting, harder to isolate errors, requires modifying test code

**Option B**: External wrapper script invoked by Makefile (chosen)
- Pros: Clean separation of concerns, Claude failure doesn't impact test results, easier to debug independently
- Cons: Requires Makefile modification, slightly more complex plumbing

**Decision**: Chose Option B for better error isolation and simpler rollback.

### Inline Analysis During Tests vs Post-Suite

**Option A**: Analyze each test failure as it happens (AfterEach hook)
- Pros: Immediate feedback, smaller context per analysis
- Cons: Significant test execution time overhead, per-test API costs, incomplete context (can't correlate multiple failures)

**Option B**: Single analysis after all tests complete (chosen)
- Pros: No test execution overhead, full suite context for correlation, single API call cost-efficient
- Cons: Delayed feedback until suite completion

**Decision**: Chose Option B to avoid impacting test execution time (critical for CI velocity).

### Model Selection

Evaluated Claude models for cost vs capability:

- **claude-sonnet-4.5**: Best reasoning for complex log analysis, ~$3/M tokens input
- **claude-haiku-4**: Faster and cheaper, but may miss subtle patterns
- **claude-opus-4**: Most capable but expensive for CI automation

**Decision**: Use `claude-sonnet-4.5` (default in Claude Code CLI) as it provides optimal balance of accuracy and cost for technical log analysis.

## Security Considerations

### GCP Service Account Permissions

The Vertex AI service account requires minimal permissions:
- `roles/aiplatform.user` - Allows calling Vertex AI endpoints for inference
- No access to cluster resources, Kubernetes API, or OADP secrets required
- No broad GCP project permissions (storage, compute, etc.)

Service account is scoped to only:
- `aiplatform.endpoints.predict` - Call Vertex AI Claude models
- `aiplatform.endpoints.get` - Retrieve endpoint metadata
- No write permissions to GCP resources

### Credential Storage

Vertex AI credentials stored in existing OpenShift CI vault collection:

- Collection name: `oadp-credentials` in `test-credentials` namespace (reuses existing collection)
- Files in collection:
  - `gcp-claude-code-credentials` - Service account JSON key
  - `gcp-claude-code-project-id` - GCP project ID as plain text
- Mounted read-only at:
  - `/var/run/oadp-credentials/gcp-claude-code-credentials`
  - `/var/run/oadp-credentials/gcp-claude-code-project-id`
- Never logged or exposed in artifacts
- Stored alongside OADP cloud credentials (AWS/Azure/GCP backup credentials) in same collection
- Managed by OpenShift CI infrastructure team via vault backend
- No openshift/release configuration changes needed (mount path already exists)

### Credentials in Logs

Analysis script automatically redacts sensitive data:

- `GOOGLE_APPLICATION_CREDENTIALS` path logged, not contents
- Service account key never read or echoed
- Claude inputs (must-gather, JUnit, per-test logs) are already non-sensitive CI logs
- No OADP backup credentials passed to Claude
- **Automatic redaction** applied to all Claude output before saving to artifacts

**Redaction Patterns**:

The `redact_secrets()` function removes:
- AWS credentials (AKIA* access keys, secret access keys)
- GCP service account private keys (PEM format in JSON)
- Bearer tokens and JWT tokens (eyJ* format)
- Passwords and passphrases in configs (password=, passwd=)
- API keys (api_key=, apiKey=, X-API-Key)
- Generic secrets (secret= with 16+ chars)
- Client secrets and authorization headers
- RSA/EC private keys (PEM format)

All matched patterns are replaced with `[REDACTED-*]` markers in the analysis output.
This prevents credential leakage even if Claude inadvertently includes secrets in its analysis.

### Audit Trail

All Claude API calls logged in Vertex AI audit logs:
- Request timestamps, model used, token counts
- No payload logging (artifacts not stored by Vertex AI)
- GCP Cloud Audit Logs track service account usage

## Compatibility

### No Breaking Changes

- Existing test execution flow unchanged
- Analysis runs post-suite, doesn't modify test behavior
- All existing artifacts (junit_report.xml, must-gather, pod logs) generated as before
- Prow result reporting unaffected (original test exit code preserved)

### Opt-Out Mechanism

Disable Claude analysis via environment variable:
```yaml
env:
- name: SKIP_CLAUDE_ANALYSIS
  value: "true"
```

Analysis automatically skipped if:
- `SKIP_CLAUDE_ANALYSIS=true`
- Vertex AI credentials missing (`GOOGLE_APPLICATION_CREDENTIALS` or `ANTHROPIC_VERTEX_PROJECT_ID` unset)
- Tests passed (exit code 0)

### Graceful Degradation

If Claude analysis fails:
- Error logged to console
- Partial/error output written to `claude-failure-analysis.md`
- Original test exit code returned (Prow sees test result correctly)
- Must-gather and other artifacts still collected normally

Failure modes:
- Claude CLI not installed: Script logs warning, exits with original test code
- Vertex AI timeout (>10min): Script logs timeout, preserves test result
- API authentication error: Script logs error, preserves test result

### Version Compatibility

- Claude CLI installed from latest stable release
- Works with current Ginkgo v2 framework
- Compatible with existing must-gather collection (v1.0+ format)
- No changes to JUnit XML format required

## Implementation

### Phase 1: MVP (Single PR in oadp-operator)

**Files Modified in oadp-operator**:

1. `build/ci-Dockerfile` - Add Claude CLI installation (~10 lines)
2. `tests/e2e/scripts/analyze_failures.sh` - New analysis script (~150 lines)
3. `Makefile` - Modify test-e2e target to set Vertex AI env vars from vault files (~15 lines)
   - Only runs Claude analysis when OPENSHIFT_CI=true
   - Reads GOOGLE_APPLICATION_CREDENTIALS from `/var/run/oadp-credentials/gcp-claude-code-credentials`
   - Reads ANTHROPIC_VERTEX_PROJECT_ID from `/var/run/oadp-credentials/gcp-claude-code-project-id`
   - Sets CLAUDE_CODE_USE_VERTEX=1 and CLOUD_ML_REGION=global
4. `docs/design/claude-prow-failure-analysis_design.md` - This design doc
5. `CLAUDE.md` - Add documentation section (~20 lines)

**External Configuration** (required for Claude analysis to activate):

1. Vault Collection Setup (OpenShift CI admin one-time task) - **REQUIRED**:
   - Create GCP service account with `roles/aiplatform.user`
   - Add two files to existing `oadp-credentials` vault collection:
     - `gcp-claude-code-credentials` - Service account JSON key
     - `gcp-claude-code-project-id` - Plain text file with project ID (e.g., "openshift-ci-vertex")
   - Files will appear at:
     - `/var/run/oadp-credentials/gcp-claude-code-credentials`
     - `/var/run/oadp-credentials/gcp-claude-code-project-id`
   - Makefile automatically reads these files and sets environment variables

2. `openshift/release` repo environment variables - **OPTIONAL** (for documentation/consistency):
   - File: `ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-oadp-dev__4.20.yaml`
   - Can add env vars explicitly in CI config, but Makefile already sets them from vault files
   - NO credential mount changes needed (reuses existing /var/run/oadp-credentials/)

**Graceful Degradation**:

Phase 1 can be merged and deployed immediately.
The analysis script detects missing credentials and gracefully skips Claude analysis without affecting test execution or results.
Claude analysis will activate automatically once env vars and vault credentials are configured.

### Testing Plan

**Local Testing**:
```bash
# Prerequisites
1. GCP project with Vertex AI API enabled
2. Service account with aiplatform.user role
3. Service account key JSON downloaded

# Setup
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa-key.json
export ANTHROPIC_VERTEX_PROJECT_ID=my-vertex-project
export CLAUDE_CODE_USE_VERTEX=1
export CLOUD_ML_REGION=global
export ARTIFACT_DIR=/tmp/oadp-artifacts

# Install Claude CLI
curl -fsSL https://cli.claude.ai/install.sh | sh

# Run tests (with known failure for testing)
make test-e2e GINKGO_ARGS="--ginkgo.focus='MySQL application CSI'"

# Verify output
cat /tmp/oadp-artifacts/claude-failure-analysis.md
```

**PR Testing in Prow**:
1. Create draft PR with all file changes
2. Coordinate with OpenShift CI team to:
   - Create `gcp-vertex-ai-sa` secret
   - Update CI config with Vertex AI env vars
3. Comment `/test oadp-operator-e2e-aws` to trigger presubmit
4. Check Prow artifacts: `https://prow.ci.openshift.org/view/gs/.../artifacts/claude-failure-analysis.md`
5. Verify analysis quality by comparing to manual diagnosis
6. Test skip flag: Re-run with `SKIP_CLAUDE_ANALYSIS=true`, verify no analysis generated
7. Test graceful degradation: Temporarily remove Vertex AI creds, verify test results still reported correctly

### Success Criteria

- ✅ Claude CLI successfully installed in `test-oadp-operator` image
- ✅ Vertex AI credentials properly mounted and accessible
- ✅ Analysis script executes only on test failures (not on success)
- ✅ `claude-failure-analysis.md` generated in ARTIFACT_DIR
- ✅ Analysis appears in Prow GCS artifacts viewer
- ✅ Analysis quality: Identifies root causes for >80% of real failures
- ✅ Known flakes correctly detected using patterns from `tests/e2e/lib/flakes.go`
- ✅ Claude failure doesn't block test result reporting
- ✅ Execution time <10 minutes for typical failed runs
- ✅ Cost <$1 per failed test run

### Rollback Plan

**Quick Disable** (no code changes):
Set environment variable in Prow config:
```yaml
env:
- name: SKIP_CLAUDE_ANALYSIS
  value: "true"
```

**Complete Removal** (revert PR):
```bash
git revert <claude-integration-commit-sha>
```
Reverts:
- ci-Dockerfile (removes Claude CLI installation)
- Makefile (removes analysis script invocation)
- analyze_failures.sh deletion

**Impact of Rollback**:
- Zero impact to test execution or results
- Original artifacts (must-gather, junit, pod logs) unaffected
- Prow reporting continues normally

### Cost Analysis

**Vertex AI Pricing** (estimated for us-east5):
- Input: $3.00 per million tokens (~$0.003 per 1K tokens)
- Output: $15.00 per million tokens (~$0.015 per 1K tokens)

**Typical Failed Test Run**:
- must-gather summary: ~5,000 tokens
- JUnit XML: ~1,000 tokens
- Per-test logs (3 failures): ~10,000 tokens
- Total input: ~16,000 tokens → ~$0.05
- Output: ~5,000 tokens → ~$0.08
- **Total per failed run: ~$0.13**

**Monthly Estimate** (100 failed runs/month):
- 100 runs × $0.13 = **$13/month**
- With retries and variations: **~$15-25/month**

**Cost Controls**:
- Only analyze on failures (not ~1000 successful runs/month)
- 10-minute timeout prevents runaway token usage
- Single analysis per suite (not per-test)
- No analysis on successful runs

### Timeline

**Week 1**: MVP implementation and local testing
- Day 1-2: Implement ci-Dockerfile, analyze_failures.sh, Makefile changes
- Day 3: Local testing with Vertex AI credentials
- Day 4: Documentation (CLAUDE.md, design doc)
- Day 5: Code review and iteration

**Week 2**: Prow integration and validation
- Day 1: Coordinate with OpenShift CI team for secret creation
- Day 2: Update openshift/release CI config
- Day 3-4: PR testing in Prow, verify artifact upload
- Day 5: Analyze 10+ real failed CI runs, validate analysis quality

**Week 3**: Production rollout
- Day 1-2: Address feedback from test runs
- Day 3: Merge PR
- Day 4-5: Monitor production usage, gather developer feedback

## Open Issues

### Optimal Claude Model Selection

**Question**: Should we use `claude-sonnet-4.5` or allow model override via environment variable?

**Considerations**:
- Sonnet: Best balance of cost ($3/M input) and accuracy for log analysis
- Opus: Superior reasoning but 3x cost - overkill for most failures
- Haiku: 10x cheaper but may miss subtle failure patterns

**Proposed**: Default to `claude-sonnet-4.5`, add optional `CLAUDE_MODEL` env var for experiments.

### Token Limits for Large Artifact Sets

**Question**: How to handle very large must-gather archives or many per-test log directories?

**Considerations**:
- Claude Code CLI may truncate or fail on very large inputs
- Some test runs generate extensive must-gather with many namespaces
- Vertex AI has 200K token context window for Sonnet

**Design Decision**: Use a **subagent preprocessing pattern**:

1. **Large file detection**: Files >1MB are identified before main analysis
2. **Subagent extraction**: Each large log file is processed by a focused 60-second Claude invocation that extracts only error-relevant lines
3. **Preprocessed summary**: All extracted errors are collected into `preprocessed-logs.txt`
4. **Main analysis optimization**: The primary Claude analysis references the preprocessed summary first, avoiding full log reads

**Benefits**:
- Reduces token usage by ~80% for large log files
- Higher analysis accuracy by filtering noise upfront
- Parallelizable (future enhancement: run subagents concurrently)
- Graceful fallback to grep if subagent fails

**Configuration**:
```bash
LARGE_FILE_THRESHOLD=${LARGE_FILE_THRESHOLD:-1048576}  # 1MB default
MAX_LOG_LINES=${MAX_LOG_LINES:-500}                    # Max lines per log
```

### Multi-Cloud Artifact Variation Handling

**Question**: Do AWS, Azure, GCP test runs produce different artifact structures that need special handling?

**Considerations**:
- Cloud-specific errors (AWS throttling vs Azure quota vs GCP permissions)
- Provider-specific must-gather content (AWS EBS vs Azure Disk vs GCP PD)
- Different CSI driver logs

**Current Approach**: Generic prompt works across providers (already handles AWS/Azure/GCP in prompt examples).

**Future Enhancement**: Add cloud provider detection and specialized prompts based on PROVIDER env var.

### Handling Test Suite Expansion

**Question**: As E2E suite grows (currently 42 tests → future 100+ tests), will analysis degrade or exceed time limits?

**Considerations**:
- More tests = more per-test directories to analyze
- More failures = more content for Claude to process
- 10-minute timeout may be insufficient

**Proposed**:
- Monitor analysis duration over time
- Consider parallel analysis (split failures into batches)
- Increase timeout to 15-20 minutes if needed

### Integration with Existing Flake Detection

**Question**: Should Claude replace or augment the current regex-based flake detection in `tests/e2e/lib/flakes.go`?

**Current State**: `CheckIfFlakeOccurred()` uses simple regex patterns.

**Proposed**: Keep both:
- Regex flake detection runs during test (fast, catches known patterns immediately)
- Claude analysis runs post-suite (comprehensive, identifies new flakes)
- Claude cross-references its findings with known patterns from `flakes.go`

**Action**: Document both mechanisms in CLAUDE.md, clarify when each is used.

### Feedback Loop

**Question**: How do we improve Claude prompts and analysis quality based on developer feedback?

**Proposed**:
1. Add "Was this analysis helpful? (Y/N)" prompt to output
2. Collect feedback in GitHub issues with `claude-analysis` label
3. Quarterly review of analysis quality with E2E team
4. Iterate on prompt based on common misses or false positives

**Tracking**: Create GitHub issue template for Claude analysis feedback.

# Design proposal: Claude-Powered Failure Analysis in Prow CI

## Abstract

Automatically analyze OADP E2E test failures in Prow CI using Claude Code via Google Vertex AI.
After Ginkgo test suite completes with failures, invoke Claude to analyze build logs, must-gather diagnostics, and test artifacts, then output a comprehensive root cause analysis to Prow's artifact storage for developer consumption.

## Background

OADP operators E2E test suite runs in OpenShift Prow CI using Ginkgo framework.
When tests fail, developers must manually sift through large build-log.txt files (often 50MB+), must-gather archives, and per-test pod logs to diagnose root causes.
This manual analysis is time-consuming and requires deep domain knowledge of Velero, CSI snapshots, cloud provider APIs, and Kubernetes internals.
The repository already has comprehensive artifact collection infrastructure including must-gather integration, JUnit reports, and per-test failure logs.
We have access to Google Vertex AI for Claude inference, which can be leveraged to automate failure analysis.

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
If tests failed, invoke Claude with paths to build-log.txt, must-gather artifacts, and JUnit reports.
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

**File**: `.claude/config.json` (new)

Configure Claude Code permissions for CI analysis with read-only access:

```json
{
  "permissions": {
    "allow": [
      "Read",
      "Glob",
      "Grep",
      "Read(/go/src/**)",
      "Read(/logs/**)",
      "Read(/tmp/**)",
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
      "Bash(file:*)"
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
- **Path-specific access**: Grants access to `/go/src/**`, `/logs/**`, `/tmp/**` for artifacts
- **Tool allowlist**: Specific Bash commands for log analysis (grep, awk, sed, etc.)
- **Network isolation**: Denies WebFetch and WebSearch to prevent external calls

This configuration is automatically included in the container via `COPY ./ .` in the Dockerfile.

### Analysis Script Implementation

**File**: `tests/e2e/scripts/analyze_failures.sh` (new)

```bash
#!/bin/bash
# Analyze test failures with Claude via Vertex AI after Ginkgo suite completes
# Only runs if tests failed and Claude analysis is not skipped

set +e  # Don't exit on Claude failure

ARTIFACT_DIR=${ARTIFACT_DIR:-/tmp}
SKIP_CLAUDE=${SKIP_CLAUDE_ANALYSIS:-false}
EXIT_CODE=$1

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

    # Find build-log.txt (typically in parent directory of ARTIFACT_DIR)
    BUILD_LOG="${ARTIFACT_DIR}/../build-log.txt"
    if [ ! -f "$BUILD_LOG" ]; then
        BUILD_LOG="/logs/build-log.txt"
    fi

    if [ ! -f "$BUILD_LOG" ]; then
        echo "Warning: build-log.txt not found at expected locations"
        BUILD_LOG="<not available>"
    fi

    # Create analysis prompt
    cat > "${ARTIFACT_DIR}/claude-prompt.txt" << 'PROMPT_EOF'
# OADP E2E Test Failure Analysis Request

You are analyzing a failed OADP (OpenShift API for Data Protection) E2E test run from Prow CI.

## Available Artifacts

1. **build-log.txt**: Complete Ginkgo test output (stdout/stderr) - contains all test execution logs
2. **must-gather/**: OADP diagnostics collection with structure:
   - `clusters/<cluster-id>/oadp-must-gather-summary.md` - High-level summary
   - `clusters/<cluster-id>/namespaces/openshift-adp/` - OADP namespace resources (pod logs, DPA, BSL, VSL, backups, restores)
   - `clusters/<cluster-id>/cluster-scoped-resources/` - Cluster-wide resources (CSI drivers, storage classes)
3. **junit_report.xml**: Structured test results with pass/fail status
4. **<TestName>/**: Per-test directories containing:
   - `openshift-adp/<pod-name>/*.log` - Velero, node-agent, plugin logs
   - `<app-namespace>/<pod-name>/*.log` - Application pod logs

## Known Flake Patterns (see tests/e2e/lib/flakes.go)

Check for these known flakes before diagnosing as real failures:
- "Failed to check and update snapshot content" - CSI VolumeSnapshotBeingCreated race condition (issue #876)
- "Error copying image: writing blob" - Transient S3 bucket errors (issue #5856)
- AWS rate limiting errors
- DNS resolution timeouts
- Image pull backoff errors

## Analysis Tasks

1. Parse junit_report.xml to identify all failed tests
2. For each failed test:
   a. Extract relevant log snippets from build-log.txt (search by test name)
   b. Review must-gather diagnostics for OADP component status
   c. Check per-test pod logs for error messages
   d. Identify root cause (real bug vs known flake vs environmental issue)
   e. Provide evidence-based diagnosis with line numbers/log snippets
3. Summarize overall cluster health from must-gather
4. Provide actionable recommendations prioritized by severity

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
build-log.txt:LINE: "<relevant log excerpt>"
must-gather: <specific resource status or log finding>
Pod logs (<namespace>/<pod>/<container>): "<error message>"
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

- ✓ VolumeSnapshotBeingCreated race condition (matched pattern in build-log.txt:LINES)
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

- Be specific: Always cite line numbers from build-log.txt, not "somewhere in the log"
- Be evidence-based: Don't speculate without supporting log evidence
- Distinguish failure types: Real bugs vs flakes vs environmental vs configuration
- Be actionable: Recommendations should be concrete and implementable
- Be concise: Developers need quick insights, not verbose analysis
- Cross-reference: Link similar failures across multiple tests
- Prioritize: Put critical issues before warnings before flakes
PROMPT_EOF

    # Count failed tests from JUnit
    FAILED_COUNT=0
    if [ -f "${ARTIFACT_DIR}/junit_report.xml" ]; then
        FAILED_COUNT=$(grep -c 'failures="[1-9]' "${ARTIFACT_DIR}/junit_report.xml" 2>/dev/null || echo "0")
    fi

    echo "Found $FAILED_COUNT test suites with failures"
    echo "Invoking Claude for analysis..."

    # Invoke Claude via Vertex AI in headless mode
    # Claude Code CLI uses Vertex AI when CLAUDE_CODE_USE_VERTEX=1 and credentials are set
    # Using --print flag for non-interactive/headless operation suitable for CI automation
    timeout 600 claude --print "You are analyzing OADP E2E test failures from Prow CI.

Read the analysis instructions in: ${ARTIFACT_DIR}/claude-prompt.txt

Analyze these artifacts:
1. Build log: ${BUILD_LOG}
2. Must-gather: ${ARTIFACT_DIR}/must-gather/
3. JUnit report: ${ARTIFACT_DIR}/junit_report.xml
4. Test failure directories: ${ARTIFACT_DIR}/*/

Generate comprehensive failure analysis following the output format specified in the prompt.
Focus on actionable insights and clear root cause identification.

IMPORTANT SECURITY NOTE:
Do NOT include any API keys, tokens, passwords, or service account keys in your analysis.
If you encounter credentials in logs, reference them generically (e.g., \"AWS credentials found in log\")." 2>&1 | redact_secrets > "${ARTIFACT_DIR}/claude-failure-analysis.md"

    CLAUDE_EXIT=$?

    if [ $CLAUDE_EXIT -eq 0 ]; then
        echo "✓ Claude analysis completed successfully"
        echo "✓ Analysis saved to: ${ARTIFACT_DIR}/claude-failure-analysis.md"

        # Show summary (first 80 lines)
        echo ""
        echo "=== Claude Analysis Preview ==="
        head -80 "${ARTIFACT_DIR}/claude-failure-analysis.md"
        echo "=== (Full analysis available in Prow artifacts) ==="
    elif [ $CLAUDE_EXIT -eq 124 ]; then
        echo "✗ Claude analysis timed out after 10 minutes"
        echo "Large build-log.txt may have exceeded token limits"
        echo "Partial analysis may be in ${ARTIFACT_DIR}/claude-failure-analysis.md"
    else
        echo "✗ Claude analysis failed (exit code: $CLAUDE_EXIT)"
        echo "Check ${ARTIFACT_DIR}/claude-failure-analysis.md for error details"
    fi

    # Cleanup prompt (keep in artifacts for debugging)
    # rm -f "${ARTIFACT_DIR}/claude-prompt.txt"
else
    echo "Tests passed, skipping Claude analysis"
fi

exit $EXIT_CODE
```

**File Permissions**: The script is made executable in `build/ci-Dockerfile` during container build (see Dockerfile section below).

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
├── build-log.txt                          # Ginkgo stdout/stderr
├── artifacts/
│   ├── junit_report.xml                   # Test results
│   ├── must-gather/                       # OADP diagnostics
│   │   └── clusters/<cluster-id>/
│   │       ├── oadp-must-gather-summary.md
│   │       ├── namespaces/
│   │       │   └── openshift-adp/
│   │       │       ├── pods/
│   │       │       ├── backups/
│   │       │       └── restores/
│   │       └── cluster-scoped-resources/
│   ├── MySQL application CSI/             # Per-test logs
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
build-log.txt:1247: "VolumeSnapshot mysql-pvc not ready after 10m0s"
must-gather: clusters/12345678/namespaces/openshift-adp/volumesnapshots.yaml
  status.readyToUse: false
  status.error: "snapshot-12345: rpc error: code = DeadlineExceeded"
Pod logs (openshift-adp/node-agent-abc123/node-agent):
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
build-log.txt:2103: "Error copying image: writing blob: unexpected EOF"
velero logs: "Backup failed: error uploading backup: RequestTimeout: upload timeout"
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
build-log.txt:856: "Pod openshift-adp/velero-plugin-for-aws-xyz ImagePullBackOff"
must-gather events: "Failed to pull image quay.io/konveyor/velero-plugin-for-aws:v1.10.1"
Pod status: "ErrImagePull: rate limit exceeded"
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

- ✓ S3 transient write errors (matched "Error copying image: writing blob" in build-log.txt:2103)
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
- Never logged or exposed in build-log.txt or artifacts
- Stored alongside OADP cloud credentials (AWS/Azure/GCP backup credentials) in same collection
- Managed by OpenShift CI infrastructure team via vault backend
- No openshift/release configuration changes needed (mount path already exists)

### Credentials in Logs

Analysis script automatically redacts sensitive data:

- `GOOGLE_APPLICATION_CREDENTIALS` path logged, not contents
- Service account key never read or echoed
- Claude inputs (build-log.txt, must-gather) are already non-sensitive CI logs
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
- No payload logging (build-log.txt not stored by Vertex AI)
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
- Vertex AI credentials missing (`GOOGLE_APPLICATION_CREDENTIALS` or `GOOGLE_CLOUD_PROJECT` unset)
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
- Build-log.txt not found: Script logs warning, analyzes available artifacts only

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
- build-log.txt: ~50,000 tokens (200KB text)
- must-gather summary: ~5,000 tokens
- JUnit XML: ~1,000 tokens
- Per-test logs (3 failures): ~10,000 tokens
- Total input: ~66,000 tokens → ~$0.20
- Output: ~5,000 tokens → ~$0.08
- **Total per failed run: ~$0.28**

**Monthly Estimate** (100 failed runs/month):
- 100 runs × $0.28 = **$28/month**
- With retries and variations: **~$30-50/month**

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

### Token Limits for Large build-log.txt Files

**Question**: How to handle build-log.txt files >200KB (>50K tokens, approaching context limits)?

**Considerations**:
- Claude Code CLI may truncate or fail on very large inputs
- Some test runs generate 500KB+ logs with verbose output
- Vertex AI has 200K token context window for Sonnet

**Proposed Solutions**:
1. Preprocess build-log.txt: Extract only failed test sections (grep for test names from JUnit)
2. Implement smart truncation: Keep first 10KB, last 50KB, + failed test sections
3. Add token counting and warn if approaching limits

**Recommendation**: Start with full file, monitor token usage, implement truncation if needed.

### Multi-Cloud Artifact Variation Handling

**Question**: Do AWS, Azure, GCP test runs produce different artifact structures that need special handling?

**Considerations**:
- Cloud-specific errors (AWS throttling vs Azure quota vs GCP permissions)
- Provider-specific must-gather content (AWS EBS vs Azure Disk vs GCP PD)
- Different CSI driver logs

**Current Approach**: Generic prompt works across providers (already handles AWS/Azure/GCP in prompt examples).

**Future Enhancement**: Add cloud provider detection and specialized prompts:
```bash
PROVIDER=$(grep 'PROVIDER=' build-log.txt | head -1 | cut -d= -f2)
# Load provider-specific flake patterns and analysis hints
```

### Handling Test Suite Expansion

**Question**: As E2E suite grows (currently 42 tests → future 100+ tests), will analysis degrade or exceed time limits?

**Considerations**:
- More tests = longer build-log.txt
- More failures = more per-test directories to analyze
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

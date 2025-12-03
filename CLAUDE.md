# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

OADP (OpenShift API for Data Protection) is a Kubernetes operator that installs and manages Velero for backup and restore operations in OpenShift clusters. It extends Velero with OpenShift-specific features like Security Context Constraints (SCC), cloud credential management, and monitoring integration.

## Prerequisites

**Go Version**: Go 1.24.0 (with toolchain go1.24.5)

**macOS Users**: Install GNU sed (required for bundle generation and other targets)

```bash
brew install gnu-sed
```

**Container Tool**: Docker or Podman (auto-detected, defaults to Docker if available)

- Override with: `CONTAINER_TOOL=podman make <target>`

**Tool Version Checking**: Run `make versions` to check all tool versions and detect mismatches

## Development Commands

### Essential Commands

```bash
# Discovery and validation
make help                   # Display all available targets with descriptions
make versions               # Check tool versions and detect mismatches

# Development workflow
make test                    # Run unit tests, linting, and validation (recommended before commits)
make build                   # Build manager binary
make deploy-olm             # Deploy for testing via OLM (recommended for PR testing)
make undeploy-olm           # Remove OLM deployment

# Code generation (run after API changes)
make generate               # Generate DeepCopy methods
make manifests              # Generate CRDs and RBAC manifests
make bundle                 # Generate OLM bundle
make api-isupdated          # Check if API is up to date
make bundle-isupdated       # Check if bundle is up to date

# Linting and formatting
make lint                   # Run golangci-lint
make lint-fix               # Fix linting issues automatically
make fmt                    # Format code with go fmt

# Special targets
make update-non-admin-manifests  # Update NAC manifests from external repo
```

### Testing Commands

```bash
make test-e2e               # Run end-to-end tests (requires setup)
make test-e2e-setup         # Setup E2E test environment
make test-e2e-cleanup       # Cleanup after E2E tests

# Test variations
TEST_VIRT=true make test-e2e      # Run virtualization tests
TEST_UPGRADE=true make test-e2e   # Run upgrade tests
TEST_CLI=true make test-e2e       # Run CLI-based tests

# Run focused tests
GINKGO_ARGS="--ginkgo.focus='test name'" make test-e2e
```

### Cloud Authentication Deployment

Deploy OADP with cloud-native authentication (STS, Workload Identity, WIF):

```bash
make deploy-olm-stsflow         # Deploy with standardized flow UI (interactive)
make deploy-olm-stsflow-aws     # Deploy with AWS STS
make deploy-olm-stsflow-gcp     # Deploy with GCP Workload Identity Federation
make deploy-olm-stsflow-azure   # Deploy with Azure Workload Identity
```

These targets automate cloud credential setup using cloud-native identity providers instead of manual credential files. The standardized flow provides an interactive UI for configuration.

### E2E Test Setup Requirements

E2E tests require these environment variables:

- `OADP_CRED_FILE`: Path to backup location credentials
- `OADP_BUCKET`: S3 bucket name for backups
- `CI_CRED_FILE`: Path to snapshot location credentials
- `VSL_REGION`: Volume snapshot location region
- `BSL_REGION`: Backup storage location region (optional, defaults to us-east-1)

**Test Labels**: Tests are filtered by cloud provider labels: `aws`, `gcp`, `azure`, `ibmcloud`, `virt`, `hcp`, `cli`, `upgrade`

**Common Test Issues**:

- ttl.sh images expire after TTL_DURATION (default 1h), which may cause test failures if running tests long after initial deployment

## Important Environment Variables

**Operator Configuration**:

- `IMG`: Custom operator image (default: `quay.io/konveyor/oadp-operator:latest`)
- `VERSION`: Override version (default: `99.0.0`)
- `OADP_TEST_NAMESPACE`: Namespace for operator (default: `openshift-adp`)

**Image Build and Registry**:

- `CONTAINER_TOOL`: Container tool to use (`docker` or `podman`, auto-detected)
- `TTL_DURATION`: ttl.sh image expiry time (default: `1h`, max: `24h`)
- `BUNDLE_IMG`: Custom bundle image

**Cloud Provider Credentials** (for E2E tests):

- `OADP_CRED_FILE`, `OADP_BUCKET`, `CI_CRED_FILE`: Backup/snapshot credentials
- `VSL_REGION`, `BSL_REGION`: Cloud regions for volume/backup storage locations

## Git Repository Information

**Upstream Repository**: `openshift/oadp-operator`

**IMPORTANT - Pull Request Target**: Always target `oadp-dev` branch for PRs, NOT `main`

**Branch Structure**:

- Development branch: `oadp-dev` (target for all PRs)
- Release branches: `oadp-major.minor` (e.g., `oadp-1.4`, `oadp-1.5`)
- Many remote branches from various contributors exist

You can verify the current default branch with `git ls-remote --symref upstream HEAD`.

## Architecture Overview

### Core APIs (Custom Resources)

- **DataProtectionApplication (DPA)**: Primary resource that configures the entire OADP/Velero stack
- **CloudStorage**: Manages cloud storage configurations for backup locations
- **DataProtectionTest**: Framework for testing backup/restore operations
- **Non-Admin resources**: Enable multi-tenant backup scenarios (NonAdminBackup, NonAdminRestore)

### Key Controllers

- **DataProtectionApplicationReconciler**: Main controller that orchestrates Velero deployment and configuration
- **CloudStorageReconciler**: Manages cloud storage backend setup
- **DataProtectionTestReconciler**: Handles data protection testing workflows

### Package Structure

- `api/v1alpha1/`: CRD type definitions and API schemas
- `internal/controller/`: Controller implementations and reconciliation logic
- `pkg/credentials/`: Cloud credential management and authentication flows
- `pkg/velero/`: Velero-specific utilities and integration code
- `pkg/cloudprovider/`: Multi-cloud provider abstractions (AWS, Azure, GCP, IBM)
- `tests/e2e/`: Comprehensive end-to-end test suites using Ginkgo

### Integration Points

The operator manages these key integrations:

- **Velero**: Core backup/restore engine with OpenShift-specific patches
- **Cloud Providers**: AWS (including STS), Azure (Workload Identity), GCP (WIF), IBM Cloud, OpenStack
- **OpenShift**: SCC management, monitoring integration, image registry
- **Storage**: CSI snapshots, data mover functionality for cross-cluster scenarios

### Development Workflow

1. Use `make deploy-olm` for testing code changes (builds and deploys current branch)
2. Always run `make test` before committing to validate code quality
3. For API changes: run `make generate && make manifests && make bundle`
4. E2E tests require cloud credentials and should be run in appropriate test environments
5. The operator follows standard controller-runtime patterns with comprehensive validation and status reporting

### Special Features

- **Multi-cloud standardized authentication**: Supports cloud-native identity (STS, WIF, Workload Identity)
- **Non-admin backup**: Multi-tenant backup capabilities for namespace-scoped users
- **Data mover**: Cross-cluster backup/restore using VolSync integration
- **OpenShift Virtualization**: Backup/restore support for KubeVirt VMs
- **Must-gather integration**: Diagnostic collection for troubleshooting

### Bundle and Release Management

- Uses OLM (Operator Lifecycle Manager) for deployment and upgrades
- Bundle generation includes multiple service accounts (velero, non-admin-controller)
- Supports multiple channels (dev, stable) for different release streams
- Version compatibility matrix maintained in `PARTNERS.md`

When making changes, always consider the multi-cloud nature of the operator and test against the comprehensive E2E suite that covers various cloud providers and backup scenarios.

## CI/Prow Testing

E2E tests in presubmit CI are automatically triggered via OpenShift's Prow infrastructure:

**CI Configuration**: Tests are defined in the [openshift/release](https://github.com/openshift/release) repository at:
- `ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-oadp-dev__4.20.yaml`

**Test Container Image**: The `test-oadp-operator` image is built from [build/ci-Dockerfile](build/ci-Dockerfile), which:
- Uses `quay.io/konveyor/builder` as the base image
- Installs kubectl for cluster operations
- Downloads Go dependencies and prepares the build environment
- Provides the runtime environment for executing E2E tests in CI

**How it works**:
1. When a PR is opened against `oadp-dev`, Prow automatically triggers configured test jobs
2. The ci-Dockerfile builds a test container with all necessary dependencies
3. E2E tests run inside this container against a provisioned OpenShift cluster
4. Test results are reported back to the PR

**Viewing test results**: Check the PR's "Checks" tab or visit [prow.ci.openshift.org](https://prow.ci.openshift.org) for detailed test logs.

### Automated Failure Analysis with Claude

When E2E tests fail in Prow CI, Claude Code automatically analyzes the failures and generates a comprehensive report.

**How it works**:

1. After test execution completes with failures, the analysis script (`tests/e2e/scripts/analyze_failures.sh`) is invoked
2. Claude runs in headless mode (`--print` flag) for non-interactive CI automation via Vertex AI
3. Claude analyzes artifacts written by the E2E test code: JUnit reports, must-gather diagnostics, and per-test pod logs
4. A detailed markdown report is generated at `${ARTIFACT_DIR}/claude-failure-analysis.md`
5. The report includes root cause analysis, known flake detection, and actionable recommendations

**Important**: Claude analyzes only artifacts generated during test execution (JUnit, must-gather, per-test logs). Prow's build-log.txt is written by CI infrastructure after tests complete and is not available during analysis.

**Accessing the analysis**:

- Find `claude-failure-analysis.md` in the Prow artifacts directory alongside other test outputs
- URL pattern: `https://prow.ci.openshift.org/view/gs/origin-ci-test/pr-logs/pull/openshift_oadp-operator/<PR>/<job-name>/<build-id>/artifacts/claude-failure-analysis.md`

**Configuration**:

- Analysis requires Vertex AI credentials configured in the CI environment
- Gracefully skips if credentials are not available (no impact on test execution)
- Can be disabled by setting `SKIP_CLAUDE_ANALYSIS=true`
- **Automatic secret redaction**: API keys, tokens, passwords, and credentials are automatically redacted from output

For more details, see the [design document](docs/design/claude-prow-failure-analysis_design.md).

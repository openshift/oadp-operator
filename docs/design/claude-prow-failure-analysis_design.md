# Design: E2E Failure Analysis via the claude-ai-helpers Step-Registry

## Abstract

OADP's Prow E2E jobs automatically analyze test failures using Claude Code, producing a root-cause report from JUnit results, must-gather diagnostics, and per-test pod logs. The analysis runs as an `openshift/release` step-registry post-step, using infrastructure shared across CI teams rather than anything baked into this repository's own image or build.

## Background

When E2E tests fail, developers must sift through must-gather archives, JUnit reports, and per-test pod logs to diagnose root causes — time-consuming work that requires domain knowledge of Velero, CSI snapshots, cloud provider APIs, and Kubernetes internals. The repository already collects comprehensive artifacts (must-gather, JUnit, per-test logs) that make this analysis possible to automate.

Several CI teams in the OpenShift org (`medik8s`, `hypershift`, `security/adversary-scan`, and others) already run this kind of analysis via a shared `claude-ai-helpers` base image — built from `openshift-eng/ai-helpers` — invoked as an `openshift/release` step-registry post-step. OADP's E2E jobs follow the same pattern rather than maintaining a bespoke in-repo implementation.

**Note**: Prow's `build-log.txt` is written by CI infrastructure **after** tests complete and is not available during analysis. The step relies on artifacts generated during test execution: JUnit reports, must-gather diagnostics, and per-test pod log directories.

## Goals

- Automatically analyze test failures after the E2E test step completes
- Output a structured markdown report to the job's Prow artifact directory
- No maintenance burden on this repository's test image or `Makefile`
- Reuse a credential and base image already shared across CI teams
- Graceful degradation — analysis failures never affect test result reporting

## Non-Goals

- Live cluster diagnostics during test execution
- Auto-remediation of failures
- Analysis of successful test runs
- Real-time streaming analysis (only post-suite batch analysis)

## Architecture

```
E2E test step (make test-e2e)
        │  writes junit_report.xml, must-gather/, per-test logs to $ARTIFACT_DIR
        ▼
oadp-analyze-e2e-failure  (openshift/release step-registry post-step)
        │  runs claude-ai-helpers image, authenticates via sa-claude-openshift-ci
        ▼
${ARTIFACT_DIR}/claude-failure-analysis.md  (Prow GCS artifacts)
```

The step is defined and wired entirely in `openshift/release`:

- **Image**: the shared `claude-ai-helpers` base image (`ci` namespace), built from `openshift-eng/ai-helpers`, with Claude Code and CI-analysis skills preinstalled. This repository's own `build/ci-Dockerfile` and `test-oadp-operator` image are unmodified.
- **Credential**: the shared, already-provisioned `test-credentials/sa-claude-openshift-ci` secret — no per-repo vault collection or credential file is needed.
- **Wiring**: `oadp-analyze-e2e-failure` is registered as a post-step and attached to the `oadp-1.6` and `oadp-dev` ci-operator test configs, running immediately after the E2E test step regardless of its outcome.
- **Inputs**: the same artifacts the old in-repo script used — `junit_report.xml`, `must-gather/`, and per-test pod log directories under `$ARTIFACT_DIR`.
- **Output**: a markdown report written into the job's artifact directory, viewable alongside other Prow artifacts. All output passes through secret redaction before being written.

Because the step runs outside this repository's test container, this repo carries no Claude CLI installation, no Vertex AI credential wiring, and no `Makefile` hook — the only remaining piece here is a local/manual convenience script.

### Local/manual use

`tests/e2e/scripts/analyze_failures.sh` is kept in this repository for developers who want to run the same analysis by hand against a local `make test-e2e` run (with `GOOGLE_APPLICATION_CREDENTIALS`/`ANTHROPIC_VERTEX_PROJECT_ID`, or a plain `ANTHROPIC_API_KEY`, set in their own shell). It is not invoked by CI or by any `Makefile` target.

## Security Considerations

- **Credential scope**: `sa-claude-openshift-ci` is owned and rotated by the shared CI infrastructure team, not per-repo — this repo has no credential material to manage or leak.
- **Redaction**: analysis output is passed through secret redaction (API keys, tokens, passwords, service-account keys, AWS credentials) before being written to the artifact directory.
- **Read-only**: the step only reads test artifacts; it has no access to cluster credentials, kubeconfig, or OADP backup/restore credentials.

## Compatibility

- No changes to test execution flow — the post-step runs after, not during, the E2E test step.
- Original test exit code and Prow result reporting are unaffected by analysis success or failure.
- Applies to `oadp-1.6` and `oadp-dev`. `oadp-1.3`, `oadp-1.4`, and `oadp-1.5` do not run this step.

## References

- [openshift-eng/ai-helpers](https://github.com/openshift-eng/ai-helpers) — shared `claude-ai-helpers` image and CI-analysis skills
- `openshift/release` step-registry references: `medik8s-analyze-e2e-failure`, `hypershift-analyze-e2e-failure`

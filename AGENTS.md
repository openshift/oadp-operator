# AGENTS.md — AI Agent Instructions for openshift/oadp-operator

## Project Overview
The OADP (OpenShift API for Data Protection) Operator is the central component of the OADP ecosystem. It installs and manages Velero and its plugins on OpenShift clusters, providing backup, restore, and disaster recovery capabilities for cluster resources and persistent volumes. Built with the Operator SDK using controller-runtime.

- **Primary Language**: Go
- **Module**: `github.com/openshift/oadp-operator`
- **Default Branch**: `oadp-dev`

## Build Instructions
```bash
# Build the operator binary (includes manifests, codegen, fmt, vet)
make build

# Build Docker image
make docker-build

# Push Docker image
make docker-push

# Run the operator locally against a cluster
make run
```

## Test Instructions
```bash
# Run unit tests (includes manifests generation, codegen, fmt, vet, envtest)
make test

# Run Go vet
make vet

# Run Go fmt
make fmt

# Run specific tests
go test ./internal/controller/... -run TestName
go test ./pkg/... -run TestName

# E2E tests are in tests/ directory
```

## Linting
```bash
# Run golangci-lint
make lint

# Run golangci-lint with auto-fix
make lint-fix
```

Configuration: `.golangci.yaml`

## Code Generation
```bash
# Generate CRD manifests (WebhookConfiguration, ClusterRole, CRDs)
make manifests

# Generate DeepCopy methods
make generate
```

## Code Conventions
- Operator SDK / controller-runtime patterns
- API types in `api/` with version directories (v1alpha1)
- Controllers in `internal/controller/`
- Shared packages in `pkg/`
- Test helpers and e2e tests in `tests/`
- CRD and RBAC manifests in `config/`
- OLM bundle in `bundle/`
- Follow existing error handling patterns (wrapped errors)
- Use kubebuilder markers for RBAC and CRD generation

## Project Structure
```
api/           - OADP API types (DataProtectionApplication, etc.)
  v1alpha1/    - v1alpha1 API version
cmd/           - Operator entry point
config/        - Kubernetes manifests
  crd/         - CRD definitions
  rbac/        - RBAC rules
  manager/     - Deployment manifests
  samples/     - Example CR instances
bundle/        - OLM operator bundle
build/         - Build scripts and configs
docs/          - Documentation
hack/          - Development scripts
internal/      - Private packages
  controller/  - Reconciler implementations
  common/      - Shared internal utilities
pkg/           - Shared packages
  credentials/ - Cloud credential handling
  velero/      - Velero installation helpers
scripts/       - Utility scripts
tests/         - E2E and integration tests
  e2e/         - End-to-end test suite
```

## CI/CD
- Prow CI for OpenShift (primary)
- Linter config: `.golangci.yaml`
- Reproduce CI locally:
  ```bash
  make build
  make lint
  make test
  ```

## Common Tasks

### Adding a new field to the DataProtectionApplication CRD
1. Add the field to `api/v1alpha1/` types
2. Run `make generate` to update DeepCopy methods
3. Run `make manifests` to regenerate CRD YAML
4. Update the reconciler in `internal/controller/` to handle the new field
5. Update `bundle/` for OLM
6. Add unit tests

### Adding a new Velero plugin integration
1. Update the Velero deployment spec in `pkg/velero/`
2. Add plugin container/init-container configuration
3. Update RBAC if needed (`config/rbac/`)
4. Add e2e tests in `tests/e2e/`

### Working with the OLM Bundle
1. Bundle manifests in `bundle/`
2. Regenerate with `make bundle`
3. Test with `operator-sdk bundle validate`

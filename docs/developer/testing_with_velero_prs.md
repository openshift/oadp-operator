# Testing OADP with Custom Velero PRs

This guide explains how to test OADP operator with a custom Velero build from an upstream PR. This is useful when you need to verify OADP compatibility with upcoming Velero changes or test bug fixes before they're merged.

## Overview

The `make deploy-olm-velero-pr` target automates the process of:
1. Fetching a PR from vmware-tanzu/velero
2. Cherry-picking it onto openshift/velero's oadp-dev branch
3. Building a custom Velero image
4. Deploying OADP operator with that custom Velero image

## Prerequisites

### 1. Velero Repository

Clone the OpenShift Velero fork and configure remotes:

```bash
# Clone the repository
git clone https://github.com/openshift/velero ~/git/velero
cd ~/git/velero

# Add upstream remote
git remote add upstream https://github.com/vmware-tanzu/velero
git fetch --all
```

If you want to use a different location, set `VELERO_REPO_PATH`:
```bash
export VELERO_REPO_PATH=/path/to/your/velero
```

### 2. Container Registry Authentication

Authenticate to GitHub Container Registry:

```bash
# Create a GitHub Personal Access Token with 'write:packages' scope
# Then login:
docker login ghcr.io -u YOUR_GITHUB_USERNAME
# Or with podman:
podman login ghcr.io -u YOUR_GITHUB_USERNAME
```

### 3. OpenShift Cluster Access

Ensure you're logged into an OpenShift cluster:

```bash
oc login <your-cluster-url>
```

## Usage

### Quick Start - Full Workflow

Deploy OADP with a custom Velero build from PR #9407:

```bash
make deploy-olm-velero-pr VELERO_PR_NUMBER=9407
```

This single command will:
1. Build the Velero image from the PR
2. Push it to `ghcr.io/<your-username>/velero:pr9407`
3. Build OADP operator with the custom image reference
4. Deploy via OLM to your cluster

### Custom Parameters

Override default settings:

```bash
make deploy-olm-velero-pr \
  VELERO_PR_NUMBER=9407 \
  GHCR_USER=myusername \
  VELERO_REPO_PATH=/custom/path/to/velero \
  VELERO_IMAGE_TAG=custom-tag
```

**Available parameters:**
- `VELERO_PR_NUMBER` (required): PR number from vmware-tanzu/velero
- `VELERO_REPO_PATH` (optional): Path to velero repository (default: `~/git/velero`)
- `GHCR_USER` (optional): GitHub username for GHCR (default: from `git config user.name`)
- `VELERO_IMAGE_TAG` (optional): Custom image tag (default: `pr<number>`)
- `VELERO_IMAGE` (optional): Full image override

### Step-by-Step Workflow

If you need more control over the process:

#### 1. Build Velero Image

```bash
make build-velero-pr VELERO_PR_NUMBER=9407
```

This will:
- Checkout `oadp-dev` branch
- Fetch PR from upstream
- Cherry-pick commits
- Build using `Dockerfile.ubi`

#### 2. Push to Registry

```bash
make push-velero-pr VELERO_PR_NUMBER=9407
```

#### 3. Deploy OADP (skip Velero build/push)

If you've already built and pushed the image separately:

```bash
make deploy-olm RELATED_IMAGE_VELERO=ghcr.io/myuser/velero:pr9407
```

### Cleanup

Remove the deployment and reset the Velero repository:

```bash
make undeploy-olm-velero-pr VELERO_PR_NUMBER=9407
```

This will:
- Undeploy OADP operator via OLM
- Reset Velero repository to clean `oadp-dev` branch
- Delete the PR branch

## Troubleshooting

### Cherry-pick Conflicts

If the PR has conflicts with `oadp-dev`:

```bash
cd ~/git/velero
# Resolve conflicts manually
git status
# Edit conflicting files
git add <resolved-files>
git cherry-pick --continue
```

Then continue with the build:

```bash
make build-velero-pr VELERO_PR_NUMBER=9407
```

### Build Failures

Check the Velero Dockerfile.ubi requirements:
```bash
cd ~/git/velero
cat Dockerfile.ubi
```

Ensure all build dependencies are available.

### Image Push Issues

Verify GHCR authentication:
```bash
docker login ghcr.io
# Test with a simple push
docker pull alpine:latest
docker tag alpine:latest ghcr.io/$USER/test:latest
docker push ghcr.io/$USER/test:latest
```

### Verify Deployment

Check that the custom Velero image is being used:

```bash
oc get deployment velero -n openshift-adp -o jsonpath='{.spec.template.spec.containers[0].image}'
```

Expected output: `ghcr.io/<your-user>/velero:pr9407`

## Example Workflow

Complete example testing Velero PR #9407:

```bash
# 1. Ensure prerequisites
oc login https://api.my-cluster.com:6443
docker login ghcr.io

# 2. Deploy OADP with custom Velero
make deploy-olm-velero-pr VELERO_PR_NUMBER=9407

# 3. Verify deployment
oc get pods -n openshift-adp
oc get deployment velero -n openshift-adp -o yaml | grep image:

# 4. Test your scenario
# ... run your tests ...

# 5. Cleanup
make undeploy-olm-velero-pr VELERO_PR_NUMBER=9407
```

## How It Works

### Image Reference Replacement

The target modifies `config/manager/manager.yaml` to replace:
```yaml
- name: RELATED_IMAGE_VELERO
  value: quay.io/konveyor/velero:latest
```

With:
```yaml
- name: RELATED_IMAGE_VELERO
  value: ghcr.io/<user>/velero:pr<number>
```

This environment variable is read by the OADP operator to determine which Velero image to deploy.

### Temporary Build Directory

Similar to `deploy-olm`, the target uses a temporary directory for the build to avoid modifying your working tree. All changes to `config/manager/manager.yaml` are isolated to the build.

### Bundle Image Naming

The bundle image includes the PR number for easy identification:
```
ttl.sh/oadp-operator-velero-pr9407-<git-rev>:1h
```

## Advanced Usage

### Testing Multiple PRs

To test multiple Velero PRs in sequence:

```bash
for pr in 9407 9408 9409; do
  echo "Testing PR #$pr"
  make deploy-olm-velero-pr VELERO_PR_NUMBER=$pr
  # Run your tests
  ./run-tests.sh
  make undeploy-olm-velero-pr VELERO_PR_NUMBER=$pr
done
```

### Using Local Velero Changes

If you have local changes in the Velero repository:

```bash
cd ~/git/velero
# Make your changes
git add .
git commit -m "Local changes"

# Build and deploy without cherry-picking
cd ~/oadp-operator
make build-velero-pr VELERO_PR_NUMBER=local
make push-velero-pr VELERO_PR_NUMBER=local
```

### Persistent Image Tag

Use a custom tag that doesn't include the PR number:

```bash
make deploy-olm-velero-pr \
  VELERO_PR_NUMBER=9407 \
  VELERO_IMAGE_TAG=my-test-build
```

Image will be: `ghcr.io/<user>/velero:my-test-build`

## Related Documentation

- [Install from Source](install_from_source.md) - General development deployment guide
- [Testing Guide](testing/TESTING.md) - E2E testing documentation
- [OLM Hacking](olm_hacking.md) - Working with OLM bundles

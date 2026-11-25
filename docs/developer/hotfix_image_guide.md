# Hotfix Image Creation Guide

## Overview

This guide describes how to build hotfix images for OADP components that closely match the shipped product images. Use this when an urgent fix is needed before the next z-stream release and a customer environment needs a patched image immediately.

There are three build paths depending on your access level and FIPS requirements:

- **Red Hat internal (Tier 1)**: Uses the RHEL builder with the FIPS-certified Go toolchain — the same builder used for shipped product images. Requires `registry.redhat.io` login with internal entitlement.
- **FIPS-aware (Tier 2)**: Uses the `konveyor/builder` with the `ubi9-openssl-fips` tag — builds `golang-fips/go` from source with `GOEXPERIMENT=strictfipsruntime` and OpenSSL linking. Publicly available from `quay.io`, no entitlements needed.
- **Standard development (Tier 3)**: Uses the `konveyor/builder` with the `ubi9-latest` tag — downloads upstream Go from `go.dev`. Publicly available from `quay.io`, no entitlements needed. No FIPS support.

## Prerequisites

- Access to a container runtime: `podman` (recommended on RHEL/Fedora) or `docker`
- A container registry to push images to (e.g. `quay.io`, a customer's private registry)
- The source repository for the component you are patching, checked out to the correct release branch
- For the RHEL builder path (Tier 1): Red Hat `registry.redhat.io` login with internal entitlement
- For the external contributor path (Tier 2/3): no special entitlements needed

## Builder Image Options

There are three tiers of builder image, offering different levels of fidelity to the shipped product images.

### Tier 1: Production-Equivalent (Red Hat Internal Only)

```
registry.redhat.io/openshift/golang-builder:golang-builder-v1.26-rhel9
```

- This is the **same builder** used for shipped product images (via `ocp-build-data` config `stream: rhel-9-golang`).
- Includes the **FIPS-certified** Red Hat Go Toolset.
- Requires `registry.redhat.io` login with internal entitlement:
  ```bash
  podman login registry.redhat.io
  ```
- **Use this for any hotfix images deployed in customer production environments.**

### Tier 2: FIPS-Aware (Publicly Available)

```
quay.io/konveyor/builder:ubi9-openssl-fips
```

- Builds [`golang-fips/go`](https://github.com/golang-fips/go) from source with `GOEXPERIMENT=strictfipsruntime` and OpenSSL linking.
- Publicly available from `quay.io` — anonymous pull, **no entitlements or Red Hat employee access needed**.
- Good option for external contributors who need FIPS-like behavior in hotfix images.
- Source: <https://github.com/konveyor/builder/blob/main/Dockerfile.ubi9-openssl-fips>

### Tier 3: Standard Development (Publicly Available)

```
quay.io/konveyor/builder:ubi9-latest
```

- This is what the repository's default `Dockerfile` uses.
- UBI9 base, downloads upstream Go from `go.dev`.
- Publicly available from `quay.io` — anonymous pull, **no entitlements or Red Hat employee access needed**.
- No FIPS support.
- Good for general development and testing.
- Source: <https://github.com/konveyor/builder/blob/main/Dockerfile.ubi9>

## Building a Hotfix Image — Step by Step

### 4a. Single Component Hotfix (Velero or a Plugin)

Use this path when the fix is in one of the OADP sub-components (Velero, a plugin, etc.) and the operator itself does not need changes.

#### 1. Identify the Component

Determine which component needs the fix. See the [Image Key to Repository Mapping](#image-key-to-repository-mapping-table) table below for the full list of components and their source repositories.

#### 2. Clone and Check Out the Release Branch

```bash
git clone https://github.com/<org>/<repo>.git
cd <repo>
git checkout oadp-1.6   # Use the appropriate release branch
```

> **Note**
> Use the org and repo from the [Image Key to Repository Mapping](#image-key-to-repository-mapping-table) table. Some components are under `openshift/` (e.g. `openshift/velero`) while others are under `migtools/` (e.g. `migtools/kubevirt-velero-plugin`).

#### 3. Apply the Fix

Cherry-pick the fix commit or apply the patch manually:

```bash
git cherry-pick <commit-sha>
# or apply a patch file
git apply fix.patch
```

#### 4. Build the Image

Most component repos have a `konflux.Dockerfile` that uses the RHEL builder. Build with:

```bash
podman build -f konflux.Dockerfile \
  -t quay.io/<your-org>/<component>:hotfix-<jira-id> .
```

For external contributors **without** `registry.redhat.io` access, temporarily replace the FROM line. Choose the builder that matches your FIPS requirements:

```bash
# Option A: FIPS-aware builder (Tier 2) — recommended for hotfixes needing FIPS-like behavior
sed -i 's|FROM registry.redhat.io/openshift/golang-builder:golang-builder-v1.26-rhel9|FROM quay.io/konveyor/builder:ubi9-openssl-fips|' konflux.Dockerfile

# Option B: Standard development builder (Tier 3) — for non-FIPS development/testing
sed -i 's|FROM registry.redhat.io/openshift/golang-builder:golang-builder-v1.26-rhel9|FROM quay.io/konveyor/builder:ubi9-latest|' konflux.Dockerfile

podman build -f konflux.Dockerfile \
  -t quay.io/<your-org>/<component>:hotfix-<jira-id> .
# Don't commit this change!
git checkout konflux.Dockerfile
```

> **Note**
> Both `quay.io/konveyor/builder` tags are publicly available with anonymous pulls — no Red Hat entitlements are needed. If the target environment requires FIPS compliance, prefer the `ubi9-openssl-fips` tag (Tier 2) as a middle ground.

#### 5. Push the Image

```bash
podman push quay.io/<your-org>/<component>:hotfix-<jira-id>
```

#### 6. Apply the Override in the DPA Custom Resource

Edit the DataProtectionApplication CR to point to the hotfix image using `unsupportedOverrides`. For example, to override the Velero image:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
  namespace: openshift-adp
spec:
  unsupportedOverrides:
    veleroImageFqin: quay.io/<your-org>/velero:hotfix-<jira-id>
  configuration:
    velero:
      defaultPlugins:
        - openshift
        - aws
    # ... rest of your DPA spec
```

> **Note**
> The `unsupportedOverrides` field accepts any of the image keys listed in the [mapping table](#image-key-to-repository-mapping-table). Replace the key name and image reference as appropriate for the component you patched.

### 4b. Operator Hotfix (OADP Operator Itself)

When the fix is in the OADP operator code, you need to rebuild the operator image **and** the OLM bundle/catalog, because the operator image is referenced from the OLM bundle.

> **Note**
> The operator installation process differs between OLMv0 (CatalogSource + Subscription) and OLMv1 (ClusterCatalog + ClusterExtension). The catalog build steps (steps 1–3 below) are the same for both — only the installation method differs. Both approaches are covered below.

#### 1. Build the Operator Image

```bash
cd oadp-operator
make docker-build IMG=quay.io/<your-org>/oadp-operator:hotfix-<jira-id>
make docker-push IMG=quay.io/<your-org>/oadp-operator:hotfix-<jira-id>
```

> **Note**
> The default `Dockerfile` uses `quay.io/konveyor/builder:ubi9-latest` (Tier 3). To build with the production-equivalent RHEL builder (Tier 1) for maximum fidelity, build directly with podman:
> ```bash
> podman build --build-arg BUILDPLATFORM=linux/amd64 \
>   --from registry.redhat.io/openshift/golang-builder:golang-builder-v1.26-rhel9 \
>   -t quay.io/<your-org>/oadp-operator:hotfix-<jira-id> .
> podman push quay.io/<your-org>/oadp-operator:hotfix-<jira-id>
> ```
>
> For external contributors without `registry.redhat.io` access who need FIPS-like behavior, use the Tier 2 FIPS-aware builder instead:
> ```bash
> podman build --build-arg BUILDPLATFORM=linux/amd64 \
>   --from quay.io/konveyor/builder:ubi9-openssl-fips \
>   -t quay.io/<your-org>/oadp-operator:hotfix-<jira-id> .
> podman push quay.io/<your-org>/oadp-operator:hotfix-<jira-id>
> ```

#### 2. Generate and Build the Bundle Image

First, regenerate the bundle manifests so the CSV references your hotfix operator image:

```bash
make bundle IMG=quay.io/<your-org>/oadp-operator:hotfix-<jira-id>
```

Then build and push the bundle image:

```bash
make bundle-build \
  BUNDLE_IMG=quay.io/<your-org>/oadp-operator-bundle:hotfix-<jira-id> \
  IMG=quay.io/<your-org>/oadp-operator:hotfix-<jira-id>
make bundle-push \
  BUNDLE_IMG=quay.io/<your-org>/oadp-operator-bundle:hotfix-<jira-id>
```

#### 3. Build the Catalog Image

```bash
make catalog-build \
  CATALOG_IMG=quay.io/<your-org>/oadp-operator-catalog:hotfix-<jira-id> \
  BUNDLE_IMG=quay.io/<your-org>/oadp-operator-bundle:hotfix-<jira-id>
make catalog-push \
  CATALOG_IMG=quay.io/<your-org>/oadp-operator-catalog:hotfix-<jira-id>
```

#### 4. Create a CatalogSource

Apply a `CatalogSource` on the target cluster that points to your custom catalog:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: oadp-hotfix-catalog
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: quay.io/<your-org>/oadp-operator-catalog:hotfix-<jira-id>
  displayName: OADP Hotfix
  updateStrategy:
    registryPoll:
      interval: 10m
```

Then install (or update) the OADP operator from this catalog source. OLM will pull the bundle and deploy the hotfixed operator image.

#### 5. (OLMv1) Install via ClusterCatalog and ClusterExtension

OLMv1 is the successor to OLMv0 and is available on OpenShift 4.17+ (Tech Preview) and 4.18+ (GA). Instead of a CatalogSource and Subscription, OLMv1 uses a **ClusterCatalog** and **ClusterExtension** from the `olm.operatorframework.io/v1` API group.

The catalog image built in step 3 is the same — only the installation resources differ.

First, create a `ClusterCatalog` pointing to your hotfix catalog image (this replaces the CatalogSource from step 4):

```yaml
apiVersion: olm.operatorframework.io/v1
kind: ClusterCatalog
metadata:
  name: oadp-hotfix-catalog
spec:
  source:
    type: Image
    image:
      ref: quay.io/<your-org>/oadp-operator-catalog:hotfix-<jira-id>
```

Then, create a ServiceAccount with appropriate RBAC for OLMv1 to use when installing the operator. The ServiceAccount needs permissions to create the resources defined in the operator's bundle (Deployments, Services, CRDs, etc.):

```bash
oc create serviceaccount oadp-hotfix-sa -n openshift-adp
# Grant the ServiceAccount the necessary permissions for operator installation.
# For a hotfix, cluster-admin is the simplest option:
oc adm policy add-cluster-role-to-user cluster-admin -z oadp-hotfix-sa -n openshift-adp
```

Next, create a `ClusterExtension` to install the operator from that catalog (this replaces the Subscription):

```yaml
apiVersion: olm.operatorframework.io/v1
kind: ClusterExtension
metadata:
  name: oadp-operator
spec:
  namespace: openshift-adp
  serviceAccount:
    name: oadp-hotfix-sa
  source:
    sourceType: Catalog
    catalog:
      packageName: oadp-operator
      selector:
        matchLabels:
          olm.operatorframework.io/metadata.name: oadp-hotfix-catalog
```

> **Warning**
> **OLMv1 install mode limitation:** OLMv1 defaults to `AllNamespaces` install mode, but the OADP operator CSV only supports `OwnNamespace` (see the `installModes` section in `bundle/manifests/oadp-operator.clusterserviceversion.yaml`). This means:
>
> - **OCP 4.18**: OLMv1 requires `AllNamespaces` and does not support `OwnNamespace`. Because the OADP CSV sets `AllNamespaces: false`, the ClusterExtension install **will fail** on OCP 4.18. Use OLMv0 (CatalogSource + Subscription from step 4) instead.
> - **OCP 4.19+**: OLMv1 supports `OwnNamespace` as Tech Preview. You can install OADP via OLMv1 by opting into the Tech Preview feature gate, but be aware this is not yet GA.
>
> Until the OADP CSV adds `AllNamespaces` support or OLMv1 graduates `OwnNamespace` to GA, the safest approach for hotfixes is OLMv0 on OCP 4.18 and OLMv1 with the `OwnNamespace` Tech Preview on OCP 4.19+.

OLMv1 will resolve the `oadp-operator` package from the `oadp-hotfix-catalog` ClusterCatalog and deploy the hotfixed operator image.

For more details on OLMv1, see the [operator-framework documentation](https://operator-framework.github.io/operator-controller/).

### 4c. Multi-Component Hotfix

When the fix spans multiple components (for example, Velero **and** a plugin), build each component image separately following [section 4a](#4a-single-component-hotfix-velero-or-a-plugin), then apply all overrides in a single DPA CR update:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
  namespace: openshift-adp
spec:
  unsupportedOverrides:
    veleroImageFqin: quay.io/<your-org>/velero:hotfix-<jira-id>
    awsPluginImageFqin: quay.io/<your-org>/velero-plugin-for-aws:hotfix-<jira-id>
  configuration:
    velero:
      defaultPlugins:
        - openshift
        - aws
    # ... rest of your DPA spec
```

> **Warning**
> Apply all image overrides in a **single** DPA update. Updating the DPA multiple times in quick succession may trigger multiple reconciliation cycles and cause transient pod restarts.

### 4d. Hotfixes Involving CRD Changes

When a hotfix in a sub-component repo changes a CRD (Custom Resource Definition), those CRD changes must be synced back to the `oadp-operator` bundle before building the operator hotfix image. The OLM bundle at `bundle/manifests/` ships the CRDs that get installed on the cluster — if you skip this step, the bundle will contain stale CRDs that don't match the hotfixed component's API, which can cause validation errors or missing fields at runtime.

> **Note**
> If the hotfix only changes code (not API types or CRD schemas), this section can be skipped entirely. It is only needed when the fix modifies types that affect the generated CRD YAML (e.g. adding/removing fields, changing validation, updating descriptions in `+kubebuilder` markers).

#### Which Repos Own Which CRDs

The CRDs in `config/crd/bases/` and `bundle/manifests/` come from multiple source repositories:

| CRD File | Source Repository |
|----------|-------------------|
| `oadp.openshift.io_dataprotectionapplications.yaml` | `openshift/oadp-operator` (this repo) |
| `oadp.openshift.io_cloudstorages.yaml` | `openshift/oadp-operator` (this repo) |
| `oadp.openshift.io_dataprotectiontests.yaml` | `openshift/oadp-operator` (this repo) |
| `oadp.openshift.io_nonadminbackups.yaml` | `migtools/oadp-non-admin` |
| `oadp.openshift.io_nonadminrestores.yaml` | `migtools/oadp-non-admin` |
| `oadp.openshift.io_nonadmindownloadrequests.yaml` | `migtools/oadp-non-admin` |
| `oadp.openshift.io_nonadminbackupstoragelocationrequests.yaml` | `migtools/oadp-non-admin` |
| `oadp.openshift.io_nonadminbackupstoragelocations.yaml` | `migtools/oadp-non-admin` |
| `oadp.openshift.io_virtualmachinefilerestores.yaml` | `migtools/oadp-vm-file-restore` |
| `oadp.openshift.io_virtualmachinebackupsdiscoveries.yaml` | `migtools/oadp-vm-file-restore` |
| `velero.io_backups.yaml` | `openshift/velero` |
| `velero.io_restores.yaml` | `openshift/velero` |
| `velero.io_schedules.yaml` | `openshift/velero` |
| `velero.io_backupstoragelocations.yaml` | `openshift/velero` |
| `velero.io_volumesnapshotlocations.yaml` | `openshift/velero` |
| `velero.io_backuprepositories.yaml` | `openshift/velero` |
| `velero.io_deletebackuprequests.yaml` | `openshift/velero` |
| `velero.io_downloadrequests.yaml` | `openshift/velero` |
| `velero.io_podvolumebackups.yaml` | `openshift/velero` |
| `velero.io_podvolumerestores.yaml` | `openshift/velero` |
| `velero.io_datadownloads.yaml` | `openshift/velero` |
| `velero.io_datauploads.yaml` | `openshift/velero` |
| `velero.io_serverstatusrequests.yaml` | `openshift/velero` |

#### Sync Workflow

The `oadp-operator` Makefile provides dedicated targets for syncing CRDs from sub-component repos. Which target to use depends on which repo's CRDs changed:

**For `migtools/oadp-non-admin` CRD changes:**

```bash
# Clone and check out the hotfixed non-admin repo
git clone https://github.com/migtools/oadp-non-admin.git
cd oadp-non-admin
git checkout <hotfix-branch-or-commit>
cd ../oadp-operator

# Sync CRDs and RBAC using the Makefile target
make update-non-admin-manifests NON_ADMIN_CONTROLLER_PATH=../oadp-non-admin
```

This copies CRDs from `config/crd/bases/`, RBAC manifests, and sample CRs from the non-admin repo into the operator, then runs `make bundle` to regenerate the OLM bundle.

**For `migtools/oadp-vm-file-restore` CRD changes:**

```bash
# Clone and check out the hotfixed vm-file-restore repo
git clone https://github.com/migtools/oadp-vm-file-restore.git
cd oadp-vm-file-restore
git checkout <hotfix-branch-or-commit>
cd ../oadp-operator

# Sync CRDs and RBAC using the Makefile target
make update-vmfr-manifests VMFR_CONTROLLER_PATH=../oadp-vm-file-restore
```

**For `openshift/velero` CRD changes:**

There is no dedicated Makefile target for Velero CRDs. Copy the updated CRD files manually:

```bash
# Clone and check out the hotfixed velero repo
git clone https://github.com/openshift/velero.git
cd velero
git checkout <hotfix-branch-or-commit>
cd ../oadp-operator

# Copy updated Velero CRDs into the operator's CRD bases
cp ../velero/config/crd/v1/bases/velero.io_*.yaml config/crd/bases/

# Regenerate the OLM bundle to pick up the new CRDs
make bundle
```

**For `openshift/oadp-operator` (this repo) CRD changes:**

If the hotfix modifies API types in `api/v1alpha1/`, regenerate CRDs locally:

```bash
make generate    # Regenerate DeepCopy methods
make manifests   # Regenerate CRDs and RBAC from kubebuilder markers
make bundle      # Regenerate the OLM bundle
```

#### After Syncing

Once the CRDs are synced and the bundle is regenerated, continue with the operator build and bundle/catalog build steps from [section 4b](#4b-operator-hotfix-oadp-operator-itself) to build and push the updated operator, bundle, and catalog images.

## FIPS Certification Note

> **Warning**
> This section is critical for customer-facing hotfixes in environments that require FIPS compliance.

Shipped OADP images are built with the **FIPS-certified Red Hat Go Toolset** — the same toolchain provided by the `registry.redhat.io/openshift/golang-builder` image (Tier 1). This is configured in `openshift-eng/ocp-build-data` under `images/oadp-operator.yml` via `from.builder: stream: rhel-9-golang`.

- **Only** images built with the Tier 1 RHEL builder (`registry.redhat.io/openshift/golang-builder:golang-builder-v1.26-rhel9`) use the same FIPS-certified Go toolchain as the shipped product.
- Images built with `quay.io/konveyor/builder:ubi9-openssl-fips` (Tier 2) use `golang-fips/go` with `GOEXPERIMENT=strictfipsruntime` and OpenSSL linking — this provides FIPS-like behavior but is **not** the same Red Hat-certified toolchain.
- Images built with `quay.io/konveyor/builder:ubi9-latest` (Tier 3) use upstream Go and have **no** FIPS support.
- For customer environments that require formal FIPS certification, you **must** use the Tier 1 RHEL builder. Tier 2 is a reasonable middle ground when Tier 1 is not accessible.

## Image Key to Repository Mapping Table

The table below maps each `unsupportedOverrides` key to the source repository where that component is built:

| Image Key | Source Repository | Dockerfile |
|-----------|------------------|------------|
| `veleroImageFqin` | openshift/velero | konflux.Dockerfile |
| `awsPluginImageFqin` | openshift/velero-plugin-for-aws | konflux.Dockerfile |
| `gcpPluginImageFqin` | openshift/velero-plugin-for-gcp | konflux.Dockerfile |
| `azurePluginImageFqin` | openshift/velero-plugin-for-microsoft-azure | konflux.Dockerfile |
| `openshiftPluginImageFqin` | openshift/openshift-velero-plugin | konflux.Dockerfile |
| `legacyAWSPluginImageFqin` | openshift/velero-plugin-for-legacy-aws | konflux.Dockerfile |
| `kubevirtPluginImageFqin` | migtools/kubevirt-velero-plugin | Dockerfile |
| `kubevirtDatamoverPluginImageFqin` | migtools/oadp-vmdp | konflux.Dockerfile |
| `kubevirtDatamoverControllerImageFqin` | migtools/kubevirt-datamover-controller | konflux.Dockerfile |
| `hypershiftPluginImageFqin` | openshift/hypershift-oadp-plugin | konflux.Dockerfile |
| `nonAdminControllerImageFqin` | migtools/oadp-non-admin | konflux.Dockerfile |
| `vmFileRestoreControllerImageFqin` | migtools/oadp-vm-file-restore | konflux.Dockerfile |
| `vmFileRestoreAccessImageFqin` | migtools/kubevirt-velero-plugin | Dockerfile |
| `vmFileRestoreSSHImageFqin` | migtools/kubevirt-velero-plugin | Dockerfile |
| `vmFileRestoreBrowserImageFqin` | migtools/kubevirt-velero-plugin | Dockerfile |

> **Note**
> This table reflects image keys from the DPA API (`api/v1alpha1/dataprotectionapplication_types.go`). The `operator-type` and `tech-preview-ack` keys are not image overrides and are intentionally omitted.

## Verification

After deploying the hotfix image, verify it is running correctly:

### 1. Confirm Pods Are Running with the New Image

```bash
oc get pods -n openshift-adp -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.containers[*]}{.image}{"\n"}{end}{end}'
```

### 2. Verify the Image Digest

```bash
oc describe pod <pod-name> -n openshift-adp | grep Image:
```

Compare the image digest against what you pushed:

```bash
podman inspect quay.io/<your-org>/<component>:hotfix-<jira-id> --format '{{.Digest}}'
```

### 3. Run a Test Backup and Restore

Run a basic backup/restore cycle to confirm the fix works as expected:

```bash
# Create a test backup
oc create -f - <<EOF
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: hotfix-test-backup
  namespace: openshift-adp
spec:
  includedNamespaces:
    - <test-namespace>
  storageLocation: default
EOF

# Monitor backup progress
oc get backup hotfix-test-backup -n openshift-adp -o jsonpath='{.status.phase}'

# Once completed, create a test restore
oc create -f - <<EOF
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: hotfix-test-restore
  namespace: openshift-adp
spec:
  backupName: hotfix-test-backup
  includedNamespaces:
    - <test-namespace>
EOF

# Monitor restore progress
oc get restore hotfix-test-restore -n openshift-adp -o jsonpath='{.status.phase}'
```

### 4. Check Operator Logs for Errors

```bash
oc logs -n openshift-adp -l control-plane=controller-manager --tail=50
```

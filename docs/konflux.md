# Onboarding a New OADP Component to Konflux

This guide walks you through the end-to-end process of adding a new container image to the OADP release stream, built and shipped via Red Hat's Konflux build system.

If you've never done this before, don't worry -- the process is mostly mechanical once you understand which repos need what. The diagram below shows the full pipeline at a glance, and each step after that tells you exactly what to do and where.

## How It All Fits Together

```
Source repo (e.g. migtools/oadp-cli)
        |
        | auto-mirrored (Step 1)
        v
Private mirror (openshift-priv/migtools-oadp-cli)
        |
        | Konflux pulls source + builds image (Step 5)
        v
Build config (ocp-build-data, e.g. oadp-1.6 branch)
        |
        | image pushed to (Step 3)
        v
Delivery repo (registry.redhat.io/oadp/oadp-cli-rhel9)
        |
        | released via RPA (Step 4)
        v
Stage / Prod registries
```

## Repositories You'll Touch

| Repository | What you do there | Access |
|---|---|---|
| **Source repo** (e.g. `migtools/oadp-cli`) | Add a `konflux.Dockerfile` | GitHub (public) |
| **openshift/release** | Whitelist your repo for `openshift-priv` mirroring | [GitHub](https://github.com/openshift/release) (public) |
| **openshift-priv/migtools-\<repo\>** | Nothing -- this is auto-created from the whitelist | GitHub (restricted) |
| **openshift-eng/ocp-build-data** | Add image build config + public_upstreams mapping | [GitHub](https://github.com/openshift-eng/ocp-build-data) |
| **releng/pyxis-repo-configs** | Create the delivery repo definition | [GitLab (internal)](https://gitlab.cee.redhat.com/releng/pyxis-repo-configs) |
| **oadp-operator** | Add image-references entry + CSV relatedImage | GitHub |
| **Stage/Prod RPAs** | Add your image to the release pipeline | Internal web UI |

## Step-by-Step Process

### Step 1: Create the openshift-priv mirror

**Repo:** `openshift/release`
**File:** `core-services/openshift-priv/_whitelist.yaml`

Add your repo under the appropriate org section. For `migtools` repos:

```yaml
  migtools:
    - oadp-cli          # <-- add your repo here
    - oadp-non-admin
    # ...
```

After the PR merges, the mirror at `openshift-priv/migtools-<repo-name>` is auto-created within ~3-4 hours. ART can then perform a test build.

**Naming convention:** repos under `migtools/` are mirrored as `openshift-priv/migtools-<repo-name>`.

**Reference PR:** [openshift/release#68955](https://github.com/openshift/release/pull/68955)

### Step 2: Prepare the source repo

Your source repo needs a `konflux.Dockerfile` that follows the standard OADP build pattern:

```dockerfile
FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_golang_1.25 AS builder

COPY . /workspace
WORKDIR /workspace

ENV GOEXPERIMENT=strictfipsruntime

RUN CGO_ENABLED=1 GOOS=linux go build -mod=readonly -a -tags strictfipsruntime \
    -o /workspace/bin/<your-binary> ./<your-cmd-path>/

FROM registry.redhat.io/ubi9/ubi:latest

RUN dnf -y install openssl && dnf -y reinstall tzdata && dnf clean all

COPY --from=builder /workspace/bin/<your-binary> /usr/local/bin/<your-binary>
COPY LICENSE /licenses/

LABEL description="<description>"
LABEL io.k8s.description="<description>"
LABEL io.k8s.display-name="<display name>"
LABEL io.openshift.tags="oadp,migration,backup"
LABEL summary="<summary>"
```

Key requirements:

- **Builder image:** `brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_golang_1.25`
- **FIPS compliance:** `CGO_ENABLED=1`, `GOEXPERIMENT=strictfipsruntime`, `-tags strictfipsruntime`
- **Build flags:** `-mod=readonly` -- hermetic builds block network access, so dependencies must be prefetched
- **Runtime base:** `registry.redhat.io/ubi9/ubi:latest`
- **Use `dnf`, not `microdnf`** -- ART auto-replaces `dnf` with `microdnf` at build time via ocp-build-data modifications
- **License:** copy your `LICENSE` file to `/licenses/`

### Step 3: Create the delivery repo

**Repo:** `gitlab.cee.redhat.com/releng/pyxis-repo-configs`
**Path:** `products/oadp/oadp.yaml`

Add an entry for your new image (e.g. `oadp/oadp-cli-rhel9`). The image name here must use the `-rhel9` suffix -- this is the Red Hat registry naming convention for RHEL 9-based images.

Look at `products/ocp/` or `products/mta/` for examples of the file format.

> **Note:** The image name you choose here (`oadp/<something>-rhel9`) must match exactly across pyxis-repo-configs, ocp-build-data (`name` and `delivery_repo_names` fields), and the operator's `image-references`. If these don't match, the build will succeed but the image won't land in the right delivery repo.

### Step 4: Add to stage and prod RPAs

Add the new image as a component in the stage and prod RPA (Release Pipeline Access) configurations. This allows the image to flow through the release pipeline to stage and production registries.

> **Needs research:** The exact process for RPA configuration (URLs, who has access, what the UI looks like) is not fully documented here. Ask the ART team or your release engineering contact for guidance.

### Step 5: Add build config to ocp-build-data

**Repo:** `openshift-eng/ocp-build-data`
**Branch:** `oadp-1.6` (or whichever release stream you're targeting)

#### 5a. Create image config

Create `images/<your-component>.yml`:

```yaml
cachito:
  packages:
    gomod:
      - path: .
content:
  source:
    dockerfile: konflux.Dockerfile
    git:
      branch:
        target: oadp-1.6
      url: git@github.com:openshift-priv/migtools-<your-repo>.git
      web: https://github.com/migtools/<your-repo>
    modifications:
      - action: replace
        match: "dnf -y"
        replacement: "microdnf -y"
      - action: replace
        match: "dnf clean all"
        replacement: "microdnf clean all"
distgit:
  component: <your-component>-container
  branch: rhaos-{MAJOR}.{MINOR}-rhel-9
delivery:
  delivery_repo_names:
    - oadp/<your-delivery-repo>
for_payload: false
enabled_repos:
  - rhel-9-appstream-rpms
  - rhel-9-baseos-rpms
from:
  builder:
    - stream: rhel-9-golang
  member: base-rhel9
name: oadp/<your-delivery-repo>
owners:
  - oadp-maintainers@redhat.com
dependents:
  - oadp-operator
konflux:
  cachi2:
    lockfile:
      rpms:
        - tzdata
jira:
  project: OADP
  component: <your-component>-container
```

**Reference:** see any existing file under `images/oadp-*.yml` on the same branch for a working example (e.g. `oadp-velero-plugin-for-aws.yml`).

#### 5b. Add public_upstreams mapping

In `group.yml`, add the mapping between the private mirror and public repo:

```yaml
public_upstreams:
  # ... existing entries ...
  - private: "https://github.com/openshift-priv/migtools-<your-repo>"
    public:  "https://github.com/migtools/<your-repo>"
```

This mapping is only needed for repos under `migtools/` (or other non-`openshift` orgs). Repos already under `openshift/` are covered by the default mapping.

### Step 6: Wire into the operator bundle

The operator needs two things to pick up the new image at build time.

#### 6a. Add to `bundle/image-references`

This is the file where ART maps community image pull specs to productized Konflux-built images. Add an entry like:

```yaml
- name: oadp/<your-delivery-repo>
  from:
    kind: DockerImage
    name: <community-image-pullspec>:<branch>
```

- The `name` field must match the `name` in your ocp-build-data `images/<file>.yml` (e.g. `oadp/oadp-cli-rhel9`)
- The `from.name` is the community image reference (e.g. `quay.io/konveyor/oadp-cli-binaries:oadp-1.6`)

At build time, ART replaces `from.name` with the Konflux-built image digest. This is how the stitching works -- the community image name does **not** need to match the productized image name.

#### 6b. Add to the CSV as a `relatedImage`

Add the image to the operator's ClusterServiceVersion (`bundle/manifests/oadp-operator.clusterserviceversion.yaml`) in two places:

1. As a `RELATED_IMAGE_*` environment variable in the operator deployment
2. As an entry in the `relatedImages` list

The community pull spec used here should match the `from.name` you used in `image-references`.

## Build System Details

### How Konflux builds work for OADP

- **No `.tekton/` or `.konflux/` directories needed** in source repos -- pipelines are managed externally by ART tooling
- **No cachi2 lockfiles needed in-repo** -- dependency resolution is handled externally based on ocp-build-data config
- **Hermetic builds:** network is blocked during the build. Dependencies are prefetched by Konflux/cachi2 beforehand
- **Multi-arch:** images are built for x86_64, aarch64, ppc64le, s390x

### ocp-build-data branch structure

OADP uses product-level branches, not per-component branches:

```
ocp-build-data/
  oadp-1.5/          <-- all OADP 1.5 components
  oadp-1.6/          <-- all OADP 1.6 components
```

Each branch contains:

- `group.yml` -- shared config (Go version, arches, RHEL repos, Konflux settings, OCP targets)
- `streams.yml` -- base image references (golang builder, UBI, ose-cli)
- `images/` -- per-component build configs
- `releases.yml` -- release assembly config (may be empty)

### Name matching across systems

This is the most common source of confusion. Here's where image names must align:

| System | Field | Example |
|---|---|---|
| pyxis-repo-configs | delivery repo name | `oadp/oadp-cli-rhel9` |
| ocp-build-data | `name` | `oadp/oadp-cli-rhel9` |
| ocp-build-data | `delivery.delivery_repo_names` | `oadp/oadp-cli-rhel9` |
| operator `image-references` | `name` | `oadp/oadp-cli-rhel9` |

The community image name (e.g. `quay.io/konveyor/oadp-cli-binaries`) is independent -- it only appears in `image-references` `.from.name` and the operator CSV, and does **not** need to follow the same naming.

## Tips and Gotchas

- Any package installed in a builder/intermediate Dockerfile stage will need to be pinned in `ocp-build-data` for that image's `images/<file>.yml`
- If you don't have a good example in the `oadp-1.6` branch in ocp-build-data, the `mta-8.1` branch probably does
- For multiple images built from a single repo, if you don't set `WORKDIR` then full-depth paths from the repo root should be used for `COPY` lines
- For any `go.mod` in a subdirectory, you'll need to add it in the `images/<file>.yml` under `cachito.packages.gomod`

## References

- [ART Konflux onboarding docs](https://art-docs.engineering.redhat.com/konflux/onboard-external-operators/) (internal, requires cert)
- [openshift/release#68955](https://github.com/openshift/release/pull/68955) -- reference PR for whitelist additions
- [openshift-eng/ocp-build-data](https://github.com/openshift-eng/ocp-build-data) -- build configuration repo

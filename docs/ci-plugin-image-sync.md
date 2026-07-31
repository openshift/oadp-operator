# CI Plugin Image Substitution Guide

## Problem

OADP plugin images are built in OpenShift CI and promoted to an
ImageStream, but the quay.io mirrors that `pkg/common/common.go`
references can lag behind by hours. Without explicit substitution
entries, CI e2e tests run against stale images and may miss
regressions introduced by the code under test.

## Solution

The ci-operator configs in
[openshift/release](https://github.com/openshift/release) support two
mechanisms that, together, inject freshly promoted CI images into the
operator at test time:

| Mechanism | Purpose |
|-----------|---------|
| `base_images` | Import a plugin image from its CI ImageStream into the test namespace |
| `operator.substitutions` | Replace the quay.io pullspec in `config/manager/manager.yaml` with the imported CI image |

The operator binary reads each plugin image from the `RELATED_IMAGE_*`
environment variables defined in `config/manager/manager.yaml`. The
`operator.substitutions` entry overwrites the value of the
corresponding env var so that the running operator deploys the CI build
instead of the quay.io mirror.

## Step-by-step checklist for adding a new plugin image

### 1. Add the image constant

In `pkg/common/common.go`, add a new constant to the `// Images` block:

```go
NewPluginImage = "quay.io/konveyor/new-plugin:oadp-1.6"
```

### 2. Add the RELATED\_IMAGE env var

In `config/manager/manager.yaml`, add a new environment variable to the
manager container:

```yaml
- name: RELATED_IMAGE_NEW_PLUGIN
  value: quay.io/konveyor/new-plugin:oadp-1.6
```

### 3. Find the plugin repo's CI tag

Look up the plugin repo's ci-operator config in openshift/release. For
example, for `openshift-velero-plugin`:

```
ci-operator/config/openshift/openshift-velero-plugin/
```

In that config, find the `promotion.to` stanza to identify the
**namespace** and **name** of the target ImageStream, and find the
`images[].to` field to get the **exact tag** the image is promoted as.

Example (from the openshift-velero-plugin config):

```yaml
promotion:
  to:
  - namespace: konveyor
    name: oadp-1.6
images:
- to: openshift-velero-plugin
```

The important values are:
- **namespace**: `konveyor`
- **ImageStream name**: `oadp-1.6`
- **tag**: `openshift-velero-plugin`

### 4. Add a `base_images` entry

In each OADP variant config file under
`ci-operator/config/openshift/oadp-operator/`, add a `base_images`
entry that imports the plugin image:

```yaml
base_images:
  # ... existing entries ...
  openshift-velero-plugin:          # key must match the tag from step 3
    namespace: konveyor
    name: oadp-1.6
    tag: openshift-velero-plugin    # must exactly match the "to:" tag
```

> **Important**: the `tag` value here MUST exactly match the `to:` field
> from the plugin repo's ci-operator config. A mismatch causes the
> import to silently fail, and CI tests fall back to the stale quay.io
> image.

### 5. Add an `operator.substitutions` entry

In the same variant config files, add a substitution that replaces the
quay.io pullspec with the CI image:

```yaml
operator:
  substitutions:
  # ... existing entries ...
  - pullspec: quay.io/konveyor/openshift-velero-plugin:oadp-1.6
    with: pipeline:openshift-velero-plugin
```

The `pullspec` must match the value used in
`config/manager/manager.yaml` (and `pkg/common/common.go`) exactly.
The `with` field references the base image imported in step 4 via the
`pipeline:` prefix.

### 6. Update ALL variant configs

Repeat steps 4 and 5 in **every** variant config file for the branch.
These files follow the naming pattern:

```
ci-operator/config/openshift/oadp-operator/openshift-oadp-operator-oadp-1.6__4.XX.yaml
```

For example:
- `openshift-oadp-operator-oadp-1.6__4.22.yaml`
- `openshift-oadp-operator-oadp-1.6__4.23.yaml`
- `openshift-oadp-operator-oadp-1.6__4.24.yaml`
- etc.

Missing a variant means that OCP version's CI jobs will still use the
stale quay.io image.

## Complete example: openshift-velero-plugin

Below is a condensed example showing all the pieces for the
`openshift-velero-plugin` image.

**`pkg/common/common.go`** (already present):

```go
OpenshiftPluginImage = "quay.io/konveyor/openshift-velero-plugin:oadp-1.6"
```

**`config/manager/manager.yaml`** (already present):

```yaml
- name: RELATED_IMAGE_OPENSHIFT_VELERO_PLUGIN
  value: quay.io/konveyor/openshift-velero-plugin:oadp-1.6
```

**ci-operator variant config** (in openshift/release):

```yaml
base_images:
  openshift-velero-plugin:
    namespace: konveyor
    name: oadp-1.6
    tag: openshift-velero-plugin

operator:
  substitutions:
  - pullspec: quay.io/konveyor/openshift-velero-plugin:oadp-1.6
    with: pipeline:openshift-velero-plugin
```

## Troubleshooting

| Symptom | Likely cause |
|---------|-------------|
| CI tests use an old image despite a recent plugin PR merge | Missing or incorrect `base_images` / `operator.substitutions` entry |
| `base_images` import fails silently | The `tag` does not match the plugin repo's `to:` field |
| Only some OCP versions are affected | A variant config file was not updated |

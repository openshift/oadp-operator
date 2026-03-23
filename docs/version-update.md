# OADP Version Update Guide

This document outlines the files and fields that need to be updated for OADP operator version bumps on the oadp-1.6 branch.

## Files to Update

For a patch version update (e.g., 1.6.0 → 1.6.1), update these 4 files:

### 1. Makefile

Update `DEFAULT_VERSION` variable:

```makefile
DEFAULT_VERSION := 1.6.0
```

### 2. config/manifests/bases/oadp-operator.clusterserviceversion.yaml

Update 4 fields:

```yaml
metadata:
  annotations:
    olm.skipRange: '>=1.5.0 <1.6.0'
  name: oadp-operator.v1.6.0
spec:
  version: 1.6.0
```

### 3. bundle/manifests/oadp-operator.clusterserviceversion.yaml

Update 4 fields (same as above):

```yaml
metadata:
  annotations:
    olm.skipRange: '>=1.5.0 <1.6.0'
  name: oadp-operator.v1.6.0
spec:
  version: 1.6.0
```

### 4. bundle/oadp-operator.package.yaml

Update 1 field:

```yaml
channels:
- name: stable
  currentCSV: oadp-operator.v1.6.0
```

## Summary

| File | Fields to Update |
|------|-----------------|
| `Makefile` | `DEFAULT_VERSION` |
| `config/manifests/bases/oadp-operator.clusterserviceversion.yaml` | `olm.skipRange`, `name`, `replaces` (after initial release), `version` |
| `bundle/manifests/oadp-operator.clusterserviceversion.yaml` | `olm.skipRange`, `name`, `replaces` (after initial release), `version` |
| `bundle/oadp-operator.package.yaml` | `currentCSV` |

**Total**: 4 files, up to 9 field changes per version bump.

After updating the version fields, run `make bundle` to regenerate the bundle with the new version, then verify with `make bundle-isupdated`.

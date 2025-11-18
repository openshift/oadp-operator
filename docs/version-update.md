# OADP Operator Version Update Guide

This document outlines the files and fields that need to be updated for OADP operator version bumps.

## Files to Update

For a patch version update (e.g., 1.3.8 → 1.3.9), update these 4 files:

### 1. Makefile

Update `DEFAULT_VERSION` variable:

```makefile
DEFAULT_VERSION := 1.3.9  # was 1.3.8
```

### 2. config/manifests/bases/oadp-operator.clusterserviceversion.yaml

Update 3 fields:

```yaml
metadata:
  annotations:
    olm.skipRange: '>=0.0.0 <1.3.9'  # was <1.3.8
  name: oadp-operator.v1.3.0  # UNCHANGED - base version for 1.3.x branch
spec:
  replaces: oadp-operator.v1.3.8  # was v1.3.7
  version: 1.3.9  # was 1.3.8
```

### 3. bundle/manifests/oadp-operator.clusterserviceversion.yaml

Update 4 fields:

```yaml
metadata:
  annotations:
    olm.skipRange: '>=0.0.0 <1.3.9'  # was <1.3.8
  name: oadp-operator.v1.3.9  # was v1.3.8
spec:
  replaces: oadp-operator.v1.3.8  # was v1.3.7
  version: 1.3.9  # was 1.3.8
```

### 4. bundle/oadp-operator.package.yaml

Update 1 field:

```yaml
channels:
- name: stable-1.3
  currentCSV: oadp-operator.v1.3.9  # was v1.3.8
```

## Summary

**Total changes**: 4 files, 9 lines modified

- `Makefile`: 1 change
- `config/manifests/bases/oadp-operator.clusterserviceversion.yaml`: 3 changes
- `bundle/manifests/oadp-operator.clusterserviceversion.yaml`: 4 changes
- `bundle/oadp-operator.package.yaml`: 1 change

## Field Descriptions

- **DEFAULT_VERSION**: Main version variable used throughout the build process
- **olm.skipRange**: Tells OLM which versions can be skipped during upgrades
- **metadata.name**: The full name of this CSV version
  - In `config/manifests/bases`: Always remains `oadp-operator.v1.3.0` (base version)
  - In `bundle/manifests`: Updated to current version (e.g., `oadp-operator.v1.3.9`)
- **spec.replaces**: Points to the immediate previous version (creates upgrade path)
- **spec.version**: The semantic version number
- **currentCSV**: Indicates the current version in this channel

## Notes

- The `config/manifests/bases` file keeps `metadata.name` as `v1.3.0` (base version for 1.3.x)
- The `bundle/manifests` file updates `metadata.name` to the current version
- Channel name is `stable-1.3` (not just `stable`)
- Always verify changes with `git diff` before committing
- Ensure the `replaces` field points to the immediately previous version to maintain the upgrade path

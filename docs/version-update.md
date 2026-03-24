# OADP Operator Version Update Guide

This document outlines the files and fields that need to be updated for OADP operator version bumps.

## Files to Update

For a patch version update (e.g., 1.5.5 → 1.5.6), update these 4 files:

### 1. Makefile

Update `DEFAULT_VERSION` variable:

```makefile
DEFAULT_VERSION := 1.5.6  # was 1.5.5
```

### 2. config/manifests/bases/oadp-operator.clusterserviceversion.yaml

Update 4 fields:

```yaml
metadata:
  annotations:
    olm.skipRange: '>=1.4.0 <1.5.6'  # was <1.5.5
  name: oadp-operator.v1.5.6  # was v1.5.5
spec:
  replaces: oadp-operator.v1.5.5  # was v1.5.4
  version: 1.5.6  # was 1.5.5
```

### 3. bundle/manifests/oadp-operator.clusterserviceversion.yaml

Update 4 fields (same as above):

```yaml
metadata:
  annotations:
    olm.skipRange: '>=1.4.0 <1.5.6'  # was <1.5.5
  name: oadp-operator.v1.5.6  # was v1.5.5
spec:
  replaces: oadp-operator.v1.5.5  # was v1.5.4
  version: 1.5.6  # was 1.5.5
```

### 4. bundle/oadp-operator.package.yaml

Update 1 field:

```yaml
channels:
- name: stable
  currentCSV: oadp-operator.v1.5.6  # was v1.5.5
```

## Summary

**Total changes**: 4 files, 10 lines modified

- `Makefile`: 1 change
- `config/manifests/bases/oadp-operator.clusterserviceversion.yaml`: 4 changes
- `bundle/manifests/oadp-operator.clusterserviceversion.yaml`: 4 changes
- `bundle/oadp-operator.package.yaml`: 1 change

## Field Descriptions

- **DEFAULT_VERSION**: Main version variable used throughout the build process
- **olm.skipRange**: Tells OLM which versions can be skipped during upgrades
- **metadata.name**: The full name of this CSV version
- **spec.replaces**: Points to the immediate previous version (creates upgrade path)
- **spec.version**: The semantic version number
- **currentCSV**: Indicates the current version in this channel

## Notes

- The Makefile structure was refactored to define `DEFAULT_VERSION` only once (previously it appeared twice)
- Always verify changes with `git diff` before committing
- Ensure the `replaces` field points to the immediately previous version to maintain the upgrade path

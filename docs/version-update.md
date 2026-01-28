# OADP Operator Version Update Guide

This document outlines the files and fields that need to be updated for OADP operator version bumps.

## Files to Update

For a patch version update (e.g., 1.4.7 → 1.4.8), update these 4 files:

### 1. Makefile

Update `DEFAULT_VERSION` variable:

```makefile
DEFAULT_VERSION := 1.4.8 # was 1.4.7
```

### 2. config/manifests/bases/oadp-operator.clusterserviceversion.yaml

Update 4 fields:

```yaml
metadata:
  annotations:
    olm.skipRange: '>=0.0.0 <1.4.8' # was <1.4.7
  name: oadp-operator.v1.4.8 # was v1.4.7
spec:
  replaces: oadp-operator.v1.4.7 # was v1.4.6
  version: 1.4.8 # was 1.4.7
```

### 3. bundle/manifests/oadp-operator.clusterserviceversion.yaml

Update 4 fields (same as above):

```yaml
metadata:
  annotations:
    olm.skipRange: '>=0.0.0 <1.4.8' # was <1.4.7
  name: oadp-operator.v1.4.8 # was v1.4.7
spec:
  replaces: oadp-operator.v1.4.7 # was v1.4.6
  version: 1.4.8 # was 1.4.7
```

### 4. bundle/oadp-operator.package.yaml

Update 1 field:

```yaml
channels:
- name: stable-1.4
  currentCSV: oadp-operator.v1.4.8 # was v1.4.7
```

## Summary

**Total changes**: 4 files, 10 lines modified

- `Makefile`: 2 changes
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

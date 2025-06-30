# Design proposal: Wildcard Namespace Support for Velero Backup CLI

## Abstract

This proposal enables full wildcard namespace support for Velero backup CLI commands to resolve patterns like `prod-*` or `*-staging` into actual namespace names during command execution.
Currently, wildcard patterns are stored literally in backup specs causing backup failures when the system tries to list resources in non-existent namespaces.

## Background

Velero's backup CLI currently accepts wildcard patterns for namespace inclusion and exclusion but has incomplete implementation.
When users run commands like `velero backup create test --include-namespaces "prod-*"`, the wildcard pattern is stored literally in the backup specification without being resolved to actual namespace names.

This causes backup failures because the system attempts to list resources in namespaces literally named `"prod-*"` which don't exist.
The disconnect occurs between namespace collection (which uses proper glob matching) and resource collection (which uses literal patterns), resulting in inconsistent behavior and silent failures.

## Goals

- Enable wildcard patterns in backup CLI commands to work correctly by resolving them to actual namespace names
- Provide clear error messages when wildcard patterns are invalid or match no namespaces
- Maintain backward compatibility with existing exact namespace specifications

## Non Goals

- Wildcard support for restore operations (future enhancement)
- Complex regex patterns beyond standard glob patterns
- Dynamic namespace matching during backup execution (patterns resolved at CLI time only)

## High-Level Design

The solution resolves wildcard patterns during CLI command processing before creating backup objects.
When a user specifies patterns like `prod-*`, the CLI will query the Kubernetes API to fetch all active namespaces, apply glob pattern matching, and store only the resolved actual namespace names in the backup specification.

The implementation adds a wildcard expansion step in the backup CLI that occurs after client initialization but before backup object creation, ensuring that only valid namespace names reach the backup execution phase.

## Detailed Design

### Core Components

1. **Namespace Resolver Utility**: A new utility function that takes wildcard patterns, queries the Kubernetes API for all active namespaces, and returns resolved namespace names using glob pattern matching.

2. **CLI Integration**: Modify the backup create command's completion phase to call the namespace resolver before creating the backup object, replacing wildcard patterns with actual namespace names.

### Supported Pattern Examples

- `prod-*` matches `prod-app`, `prod-db`, `prod-cache`
- `*-staging` matches `app-staging`, `db-staging`  
- `*` matches all active namespaces
- Mixed patterns: `"prod-*,kube-system"` resolves to actual names plus exact matches

### Behavior Changes

**Current behavior**:
```bash
velero backup create test --include-namespaces "prod-*"
# Backup fails - tries to find resources in namespace literally named "prod-*"
```

**New behavior**:
```bash
velero backup create test --include-namespaces "prod-*"
# CLI resolves to: ["prod-app", "prod-db", "prod-cache"]
# Backup succeeds with all resources from matching namespaces
```

## Alternatives Considered

### Server-side Resolution
Resolving wildcards during backup execution was considered but rejected because it would require significant changes to the backup controller and could lead to inconsistent backup behavior if namespaces are created/deleted during backup execution.

### Dynamic Pattern Matching
Implementing dynamic namespace matching that updates during backup execution was considered but deemed too complex and could result in unpredictable backup contents.

## Security Considerations

The implementation requires LIST permission on namespaces resource, which is typically available to users who can create backups.
No additional privileges are required beyond what's already needed for backup operations.
Wildcard expansion reveals namespace names to CLI users, which is consistent with current Velero behavior.

## Compatibility

Exact namespace names continue to work unchanged, ensuring full backward compatibility.
Existing backup objects with wildcard patterns will continue to exhibit current behavior until recreated with the updated CLI.
No changes to backup CRD schema or server-side processing are required.

## Implementation

Implementation involves two main components:
1. A namespace resolver utility that queries Kubernetes API and performs glob pattern matching
2. Integration into the backup CLI's completion phase to resolve patterns before backup object creation

The work can be completed incrementally with unit tests, integration tests, and documentation updates.
Estimated timeline is 2-3 development cycles with proper testing and validation.

## Open Issues

Error handling strategy for cases where wildcard patterns match no namespaces needs clarification - should this be treated as an error or proceed with empty namespace list.
Performance impact on very large clusters (1000+ namespaces) requires testing and potential optimization. 
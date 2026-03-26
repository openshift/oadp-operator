# Velero Server Args: Duration Field Type Fix

Date: 2026-03-26
Jira: [OADP-3379](https://redhat.atlassian.net/browse/OADP-3379)

## Abstract

The `spec.configuration.velero.args` duration fields (e.g., `fs-backup-timeout`) use Go's `time.Duration` which serializes as an integer (nanoseconds) in the CRD schema.
This forces users to specify values like `7200000000000` instead of `"2h"` or `"120m"`, which is error-prone and inconsistent with Velero's own CLI interface.
This proposal introduces a new API version (`v1alpha2`) to change these fields to `metav1.Duration` (string-based), with a conversion webhook for backward compatibility.

## Background

The `ServerFlags` struct in v1alpha1 defines 10 duration fields as `*time.Duration`:

- `backup-sync-period`, `fs-backup-timeout`, `terminating-resource-timeout`, `default-backup-ttl`, `store-validation-frequency`, `item-operation-sync-frequency`, `default-repo-maintain-frequency`, `garbage-collection-frequency`, `default-item-operation-timeout`, `resource-timeout`

`time.Duration` is `int64` (nanoseconds).
The CRD OpenAPI schema therefore enforces `type: integer`, rejecting strings like `"120m"`.
If a user sets `fs-backup-timeout: 120`, Velero receives `--fs-backup-timeout=120ns` — almost certainly not what they intended.

Other fields in the DPA already use `*metav1.Duration` (e.g., `dataMoverPrepareTimeout`, `resourceTimeout` in NodeAgent config), making the v1alpha1 `ServerFlags` inconsistent with the rest of the API.

An existing workaround exists via the `oadp.openshift.io/unsupported-velero-server-args` annotation + ConfigMap, but this replaces *all* server args and is not a user-friendly solution for a single field override.

## Goals

- Allow users to specify duration values as human-readable strings (`"2h"`, `"120m"`) in `spec.configuration.velero.args`.
- Maintain full backward compatibility with existing v1alpha1 DPA resources.

## Non Goals

- Migrating storage version to v1alpha2 (that is a future release concern).
- Adding new fields or changing non-duration fields in the args struct.
- Deprecating v1alpha1.

## High-Level Design

Introduce `api/v1alpha2/` with the `ServerFlags` duration fields changed from `*time.Duration` to `*metav1.Duration`.
v1alpha1 remains the hub (storage version).
A conversion webhook handles v1alpha2 ↔ v1alpha1 translation.

The conversion is lossless: `time.Duration` and `metav1.Duration` represent the same underlying value (`int64` nanoseconds), just with different serialization formats.

## Detailed Design

### API Changes (v1alpha2)

The `ServerFlags` struct in v1alpha2 changes all 10 duration fields:

```go
// v1alpha1 (unchanged)
PodVolumeOperationTimeout *time.Duration `json:"fs-backup-timeout,omitempty"`

// v1alpha2 (new)
PodVolumeOperationTimeout *metav1.Duration `json:"fs-backup-timeout,omitempty"`
```

All other types (`DataProtectionApplication`, `VeleroConfig`, `GlobalFlags`, `LoggingFlags`, etc.) are copied to v1alpha2 unchanged.

Comments on duration fields are updated to remove "(in nanoseconds)" and instead document the expected string format.

### Hub and Spoke Conversion

**Hub**: v1alpha1 (storage version, no changes needed — implements empty `Hub()` marker method).

**Spoke**: v1alpha2 implements `ConvertTo()` and `ConvertFrom()`:

```go
// v1alpha2 → v1alpha1 (ConvertTo)
// *metav1.Duration → *time.Duration
if src.PodVolumeOperationTimeout != nil {
    dst.PodVolumeOperationTimeout = &src.PodVolumeOperationTimeout.Duration
}

// v1alpha1 → v1alpha2 (ConvertFrom)
// *time.Duration → *metav1.Duration
if src.PodVolumeOperationTimeout != nil {
    dst.PodVolumeOperationTimeout = &metav1.Duration{Duration: *src.PodVolumeOperationTimeout}
}
```

**Why v1alpha1 as hub**: It is the existing storage version. Keeping it as hub means no etcd migration is needed, existing resources are untouched, and rollback is safe (the operator can always read the stored format).

### CRD Configuration

```yaml
versions:
  - name: v1alpha1
    served: true
    storage: true
  - name: v1alpha2
    served: true
    storage: false
conversion:
  strategy: Webhook
  webhook:
    clientConfig:
      service:
        name: oadp-operator-webhook
        namespace: openshift-adp
        path: /convert
```

### Controller Changes

None.
The controller reconciles the storage version (v1alpha1), so `pkg/velero/server/args.go` and all controller code remain unchanged.
The conversion webhook handles translation before objects reach the controller.

### User Experience

After this change, both of the following are valid:

```yaml
# v1alpha1 (existing, still works)
apiVersion: oadp.openshift.io/v1alpha1
spec:
  configuration:
    velero:
      args:
        fs-backup-timeout: 7200000000000

# v1alpha2 (new, human-readable)
apiVersion: oadp.openshift.io/v1alpha2
spec:
  configuration:
    velero:
      args:
        fs-backup-timeout: "2h"
```

## Alternatives Considered

### 1. Change field types in-place in v1alpha1

Directly changing `*time.Duration` → `*metav1.Duration` in v1alpha1.

**Rejected because**: This is a breaking change. Existing DPA resources with integer duration values would fail schema validation after the CRD is updated. There is no safe upgrade path.

### 2. Custom type that accepts both integers and strings

A custom `DurationOrInt` type with a JSON unmarshaler that parses both formats.

**Rejected because**: The CRD OpenAPI schema can only declare one type (`integer` or `string`, not both). While the Go unmarshaler could handle it, the schema validation at the API server level would still reject one format or the other. Using `// +kubebuilder:validation:Type=""` to disable schema validation weakens the API contract.

### 3. Use the unsupported-args ConfigMap workaround

Document that users should use the `oadp.openshift.io/unsupported-velero-server-args` annotation to pass string durations via ConfigMap.

**Rejected as a long-term solution because**: It replaces *all* server args (not just the one field), is not discoverable, bypasses all CRD validation, and is explicitly labeled "unsupported." Acceptable as a short-term workaround while v1alpha2 is implemented.

## Security Considerations

No security impact.
The conversion webhook runs within the operator pod and handles only type conversion of duration values.
No new attack surface is introduced.

## Compatibility

- **Existing v1alpha1 DPAs**: Fully compatible, no changes required.
- **New v1alpha2 DPAs**: Available after upgrade, accepted immediately.
- **Downgrade**: Safe. If the operator is rolled back to a version without v1alpha2 support, v1alpha2 is no longer served, but all data is stored as v1alpha1 and remains accessible.
- **OLM**: Webhook lifecycle (certs, registration) is managed by OLM on OpenShift.

## Implementation

1. Scaffold `api/v1alpha2/` types (copy from v1alpha1, change duration fields).
2. Add `Hub()` marker to v1alpha1 `DataProtectionApplication`.
3. Implement `ConvertTo()`/`ConvertFrom()` on v1alpha2 `DataProtectionApplication`.
4. Update CRD kustomization to include both versions with conversion webhook config.
5. Run `make generate && make manifests && make bundle`.
6. Add unit tests for round-trip conversion.
7. Add E2E test: create DPA via v1alpha2 with string durations, verify Velero deployment args.

## Open Issues

- Should v1alpha2 be promoted to storage version in a future release? If so, a `StorageVersionMigrator` run would be needed to rewrite existing etcd objects. This is out of scope for the initial implementation.
- The `AutoCorrect()` method currently assigns `*time.Duration` to `PodVolumeOperationTimeout`. This logic lives in v1alpha1 and applies to the hub, so it does not need changes. However, if v1alpha2 becomes the hub in the future, it would need updating.

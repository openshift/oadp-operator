# Image Pull Policy Error Handling - Known Technical Debt

## Overview

Multiple controllers call `common.GetImagePullPolicy()` but only log errors instead of returning them. This means regex validation failures are silently swallowed, and deployments continue with potentially unexpected image pull policies.

## Affected Locations

All 7 usages follow the same pattern of logging errors without returning them:

### 1. KubeVirt DataMover Controller
**File**: `internal/controller/kubevirt_datamover_controller.go:96-99`
```go
imagePullPolicy, err := common.GetImagePullPolicy(r.dpa.Spec.ImagePullPolicy, kubevirtDatamoverControllerImage)
if err != nil {
    r.Log.Error(err, "imagePullPolicy regex failed")
}
```

### 2. Non-Admin Controller
**File**: `internal/controller/nonadmin_controller.go:128-131`
```go
imagePullPolicy, err := common.GetImagePullPolicy(r.dpa.Spec.ImagePullPolicy, nonAdminImage)
if err != nil {
    r.Log.Error(err, "imagePullPolicy regex failed")
}
```

### 3. Node Agent
**File**: `internal/controller/nodeagent.go:620-623`
```go
imagePullPolicy, err := common.GetImagePullPolicy(dpa.Spec.ImagePullPolicy, getVeleroImage(dpa))
if err != nil {
    r.Log.Error(err, "imagePullPolicy regex failed")
}
```

### 4. VM File Restore Controller
**File**: `internal/controller/vmfilerestore_controller.go:129-132`
```go
imagePullPolicy, err := common.GetImagePullPolicy(r.dpa.Spec.ImagePullPolicy, vmFileRestoreControllerImage)
if err != nil {
    r.Log.Error(err, "imagePullPolicy regex failed")
}
```

### 5. Velero - Plugin Containers
**File**: `internal/controller/velero.go:495-498`
```go
imagePullPolicy, err := common.GetImagePullPolicy(dpa.Spec.ImagePullPolicy, credentials.GetPluginImage(plugin, dpa))
if err != nil {
    r.Log.Error(err, "imagePullPolicy regex failed")
}
```

### 6. Velero - Custom Plugins
**File**: `internal/controller/velero.go:566-569`
```go
imagePullPolicy, err := common.GetImagePullPolicy(dpa.Spec.ImagePullPolicy, plugin.Image)
if err != nil {
    r.Log.Error(err, "imagePullPolicy regex failed")
}
```

### 7. Velero - Main Container
**File**: `internal/controller/velero.go:608-611`
```go
imagePullPolicy, err := common.GetImagePullPolicy(dpa.Spec.ImagePullPolicy, getVeleroImage(dpa))
if err != nil {
    r.Log.Error(err, "imagePullPolicy regex failed")
}
```

## Function Behavior

**File**: `pkg/common/common.go:216-237`

The `GetImagePullPolicy` function:
1. Returns the user override if provided
2. Compiles regex patterns for SHA256 and SHA512 digests
3. Returns `corev1.PullIfNotPresent` if image has a digest
4. Returns `corev1.PullAlways` as default

**On error**, it returns `corev1.PullAlways` as a safe fallback:
```go
sha256regex, err := regexp.Compile("@sha256:[a-f0-9]{64}")
if err != nil {
    return corev1.PullAlways, err  // Safe default
}
```

## Current Impact

**Risk Level**: Low

The error can only occur if regex compilation fails. Since the regex patterns are hardcoded constants:
- `"@sha256:[a-f0-9]{64}"`
- `"@sha512:[a-f0-9]{128}"`

Regex compilation failure is **extremely unlikely** (would require a Go runtime bug).

**Current behavior when error occurs**:
- Error is logged to operator logs
- Deployment continues with `corev1.PullAlways` policy
- User may not notice unless they check logs

**Potential issues**:
- Configuration problems are not surfaced to the user via DPA status
- Unexpected `PullAlways` behavior when user expects `PullIfNotPresent` (for digest-based images)
- Silent failures make troubleshooting harder

## Potential Fixes (Future Work)

Any fix must be applied **consistently across all 7 locations**.

### Option 1: Return Error (Recommended)
```go
imagePullPolicy, err := common.GetImagePullPolicy(r.dpa.Spec.ImagePullPolicy, image)
if err != nil {
    return err  // Propagate to caller, surfaces in DPA reconciliation
}
```

**Pros**: Configuration problems become visible in DPA status conditions
**Cons**: Breaks current behavior; requires updating all 7 call sites

### Option 2: Emit Kubernetes Event
```go
imagePullPolicy, err := common.GetImagePullPolicy(r.dpa.Spec.ImagePullPolicy, image)
if err != nil {
    r.Log.Error(err, "imagePullPolicy regex failed")
    r.EventRecorder.Event(r.dpa, corev1.EventTypeWarning, "ImagePullPolicyValidationFailed",
        fmt.Sprintf("Failed to determine image pull policy: %v", err))
}
```

**Pros**: More visible to users than logs alone
**Cons**: Still allows deployment with unexpected policy

### Option 3: Remove Error Return from GetImagePullPolicy
If regex compilation can never realistically fail, simplify the function:
```go
func GetImagePullPolicy(override *corev1.PullPolicy, image string) corev1.PullPolicy {
    // ... no error return
    // Use regexp.MustCompile since patterns are constants
}
```

**Pros**: Cleaner API, acknowledges error is unrealistic
**Cons**: Panics on malformed regex (acceptable for hardcoded patterns)

## Decision

**Status**: Documented, no immediate action required

**Rationale**: This is an established pattern across 7 call sites. The error scenario is extremely unlikely (hardcoded regex patterns), and the fallback behavior (`PullAlways`) is safe. The impact on users is minimal.

**Future Action**: If this becomes problematic, implement Option 1 (return error) consistently across all 7 locations to surface configuration issues in DPA status conditions.

## References

- GetImagePullPolicy implementation: `pkg/common/common.go:216-237`
- Velero controller note on using GetImagePullPolicy: `internal/controller/velero.go:332`

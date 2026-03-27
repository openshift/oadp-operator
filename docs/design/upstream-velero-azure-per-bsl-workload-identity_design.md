# Support per-BSL Azure Workload Identity credentials

## Abstract

Fix `NewCredential()` in [`pkg/util/azure/credential.go`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go) to read Azure Workload Identity parameters (`AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_FEDERATED_TOKEN_FILE`) from the per-BSL credential file (the `creds` map) instead of exclusively from environment variables.
This enables multiple BackupStorageLocations to use different Azure Workload Identity managed identities within a single Velero deployment.

## Background

Velero supports per-BSL credentials since v1.6 ([design doc](design/Implemented/secrets.md)).
When a BSL specifies `spec.credential`, Velero's `FileStore.Path()` materializes the Secret to a temp file and passes the file path as `config["credentialsFile"]` to the plugin's `Init()`.

For Azure, [`LoadCredentials()`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/util.go#L57-L76) in `pkg/util/azure/util.go` correctly reads this per-BSL credential file via `godotenv.Read()` into a `creds` map.
This map is then passed to [`NewCredential()`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go#L31) in `pkg/util/azure/credential.go`.

However, the Workload Identity branch of [`NewCredential()` (lines 48-54)](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go#L48-L54) ignores the `creds` map entirely:

```go
// workload identity credential
if len(os.Getenv("AZURE_FEDERATED_TOKEN_FILE")) > 0 {
    return azidentity.NewWorkloadIdentityCredential(&azidentity.WorkloadIdentityCredentialOptions{
        AdditionallyAllowedTenants: additionalTenants,
        ClientOptions:              options,
    })
}
```

- Detection uses `os.Getenv("AZURE_FEDERATED_TOKEN_FILE")` (pod-level env var, not per-BSL)
- `NewWorkloadIdentityCredential` with no explicit `ClientID`/`TenantID`/`TokenFilePath` falls back to env vars
- All BSLs share the same pod-level Azure identity regardless of per-BSL credential file content

This is inconsistent with the [service principal/certificate branch (lines 38-46)](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go#L38-L46) which correctly reads from the `creds` map, and with how the AWS and GCP plugins handle per-BSL credentials.

## Goals

- Enable per-BSL Azure Workload Identity credentials by reading `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, and `AZURE_FEDERATED_TOKEN_FILE` from the per-BSL `creds` map.
- Maintain full backward compatibility when no per-BSL credential is provided (env var fallback).

## Non Goals

- Changes to the Azure plugin repository (`velero-plugin-for-microsoft-azure`). The fix is entirely in the shared utility code at `velero/pkg/util/azure/`.
- Changes to the BSL API or credential file format.

## High-Level Design

Modify [`NewCredential()`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go#L31) in `pkg/util/azure/credential.go` to check the `creds` map first for Workload Identity parameters, falling back to environment variables when the map values are empty.
Pass the resolved values explicitly to `azidentity.NewWorkloadIdentityCredentialOptions`.

### Why the `creds` map is the correct mechanism

Velero's per-BSL credential pipeline works as follows:

1. Each BSL can specify `spec.credential`, a reference to a Kubernetes Secret containing provider-specific credentials.
2. Velero's [`FileStore.Path()`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/internal/credentials/file_store.go#L66-L88) reads the Secret and writes its content to a unique temp file on disk (e.g., `/tmp/credentials/openshift-adp/nonadmin-creds-tenant-a-cloud`).
3. The temp file path is injected as `config["credentialsFile"]` into the plugin's `Init()`.
4. For Azure, [`LoadCredentials()`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/util.go#L57-L76) reads this per-BSL credential file via `godotenv.Read()` and returns a `creds` map — a `map[string]string` containing the KEY=VALUE pairs from that specific BSL's credential file.
5. This `creds` map is then passed to `NewCredential(creds, clientOptions)`.

**The `creds` map is per-BSL** — each BSL gets its own map populated from its own credential file.
**Environment variables are per-process** — all BSLs in the same Velero pod share the same env vars.

When `NewCredential()` reads from env vars (current behavior), every BSL authenticates as the same Azure identity — the one configured at the pod level.
When `NewCredential()` reads from the `creds` map (proposed behavior), each BSL authenticates as the identity specified in its own credential file.

This is how the AWS and GCP plugins already work: they read `config["credentialsFile"]` and pass it to their respective SDKs, which parse per-BSL credentials from the file.
The Azure plugin's `LoadCredentials()` correctly parses the file into the `creds` map, but `NewCredential()` ignores it for Workload Identity — this is the bug.

## Detailed Design

### Current code ([`pkg/util/azure/credential.go`, lines 48-54](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go#L48-L54))

```go
// workload identity credential
if len(os.Getenv("AZURE_FEDERATED_TOKEN_FILE")) > 0 {
    return azidentity.NewWorkloadIdentityCredential(&azidentity.WorkloadIdentityCredentialOptions{
        AdditionallyAllowedTenants: additionalTenants,
        ClientOptions:              options,
    })
}
```

### Proposed code

```go
// workload identity credential
// Check per-BSL credential file first, fall back to environment variables
federatedTokenFile := creds["AZURE_FEDERATED_TOKEN_FILE"]
if federatedTokenFile == "" {
    federatedTokenFile = os.Getenv("AZURE_FEDERATED_TOKEN_FILE")
}
if len(federatedTokenFile) > 0 {
    tenantID := creds[CredentialKeyTenantID]
    if tenantID == "" {
        tenantID = os.Getenv("AZURE_TENANT_ID")
    }
    clientID := creds[CredentialKeyClientID]
    if clientID == "" {
        clientID = os.Getenv("AZURE_CLIENT_ID")
    }
    return azidentity.NewWorkloadIdentityCredential(&azidentity.WorkloadIdentityCredentialOptions{
        TenantID:                   tenantID,
        ClientID:                   clientID,
        TokenFilePath:              federatedTokenFile,
        AdditionallyAllowedTenants: additionalTenants,
        ClientOptions:              options,
    })
}
```

### Example BSL with per-BSL Azure Workload Identity credential

The per-BSL credential file is referenced via `spec.credential` on the BSL, and `useAAD: "true"` must be set in the config to use Azure AD authentication instead of storage account keys.

```yaml
apiVersion: velero.io/v1
kind: BackupStorageLocation
metadata:
  name: tenant-a-bsl
  namespace: openshift-adp
spec:
  provider: velero.io/azure
  credential:
    name: nonadmin-creds-tenant-a
    key: cloud
  objectStorage:
    bucket: tenant-a-backups    # Azure Blob container name
    prefix: velero
  config:
    useAAD: "true"              # Required: use Azure AD auth, not storage account keys
    storageAccount: mybackupsa
    storageAccountKeyEnvName: ""  # Ensure no key-based fallback
```

The referenced Secret contains the per-BSL credential file:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: nonadmin-creds-tenant-a
  namespace: openshift-adp
type: Opaque
stringData:
  cloud: |
    AZURE_SUBSCRIPTION_ID=<subscription-id>
    AZURE_TENANT_ID=<tenant-id>
    AZURE_CLIENT_ID=<tenant-a-managed-identity-client-id>
    AZURE_CLOUD_NAME=AzurePublicCloud
    AZURE_FEDERATED_TOKEN_FILE=/var/run/secrets/openshift/serviceaccount/token
```

Each tenant BSL references a different Secret with a different `AZURE_CLIENT_ID`, pointing to a different User-Assigned Managed Identity scoped to that tenant's blob container.

### Why this is safe

1. When no per-BSL credential file is provided, `LoadCredentials()` returns an empty map, so `creds["AZURE_FEDERATED_TOKEN_FILE"]` is empty, and the function falls back to `os.Getenv()` — existing behavior preserved.
2. When a per-BSL credential file IS provided, `LoadCredentials()` parses the KEY=VALUE file via `godotenv.Read()`, populating the map with per-BSL values.
3. The `azidentity.WorkloadIdentityCredentialOptions` struct already has `TenantID`, `ClientID`, and `TokenFilePath` fields — they are simply unused in the current code. The Azure SDK falls back to env vars when these fields are empty strings.

### Test changes ([`pkg/util/azure/credential_test.go`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential_test.go))

Add a test case verifying that per-BSL WI credentials from the `creds` map take precedence over environment variables:

```go
// per-BSL workload identity credential (creds map takes precedence)
os.Setenv("AZURE_TENANT_ID", "env-tenant")
os.Setenv("AZURE_CLIENT_ID", "env-client")
os.Setenv("AZURE_FEDERATED_TOKEN_FILE", "/tmp/env-token")
creds = map[string]string{
    "AZURE_TENANT_ID":             "per-bsl-tenant",
    "AZURE_CLIENT_ID":             "per-bsl-client",
    "AZURE_FEDERATED_TOKEN_FILE":  "/tmp/per-bsl-token",
}
tokenCredential, err = NewCredential(creds, options)
require.NoError(t, err)
assert.IsType(t, &azidentity.WorkloadIdentityCredential{}, tokenCredential)
// Verify the credential uses per-BSL values, not env vars
// (azidentity doesn't expose these fields, so verify via behavior or reflection)
os.Clearenv()
```

## Alternatives Considered

### Process-wide env var override

Instead of modifying `NewCredential()`, the caller could temporarily set environment variables to per-BSL values before calling `NewCredential()`, then restore them.
This approach is used by the AWS plugin (`os.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")`).
Rejected because it is not thread-safe and creates process-wide side effects that can affect concurrent BSL operations.

### Separate function for per-BSL WI

Create a new function `NewPerBSLWorkloadIdentityCredential()` that reads from the `creds` map.
Rejected because the fix is simpler as a fallback chain within the existing function, and avoids API surface expansion.

## Security Considerations

This change enables per-BSL Azure Workload Identity, which is a security improvement:
- Enables tenant-isolated backup storage with per-namespace managed identities
- Eliminates the need for long-term static credentials in multi-tenant scenarios
- Each managed identity can be scoped to a specific Azure Blob container
- The projected SA token has a 1-hour expiry and is automatically rotated

## Compatibility

Fully backward compatible:
- When no per-BSL credential file is provided, behavior is identical to current code
- When a per-BSL credential file IS provided but contains no WI fields, behavior falls through to managed identity (same as current)
- The Azure SDK's `WorkloadIdentityCredentialOptions` already supports explicit `TenantID`, `ClientID`, `TokenFilePath` fields
- No changes to the BSL API, credential file format, or plugin interface

## Implementation

Single PR to `vmware-tanzu/velero`:
1. Modify [`pkg/util/azure/credential.go`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go#L48-L54) (~15 lines changed)
2. Add test case to [`pkg/util/azure/credential_test.go`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential_test.go)
3. Update [`pkg/util/azure/storage_test.go`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/storage_test.go) if needed for integration coverage

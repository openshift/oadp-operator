# Non-Admin Namespace-Scoped Short-Lived Cloud Credentials

_Jira: [OADP-7660](https://redhat.atlassian.net/browse/OADP-7660)_

## Abstract

Enable NonAdminBackupStorageLocation (NonAdminBSL) to use short-lived, cloud-native credentials (AWS STS, GCP WIF, Azure Workload Identity) instead of requiring long-term static credentials.
Each non-admin namespace receives its own scoped cloud identity so that NonAdminA cannot access NonAdminB's backup data.

## Background

OADP 1.5 and 1.6 support cloud-native token-based authentication for admin-level DataProtectionApplication (DPA) deployments via the standardized STS flow (PRs [#1712](https://github.com/openshift/oadp-operator/pull/1712), [#1828](https://github.com/openshift/oadp-operator/pull/1828), [#1925](https://github.com/openshift/oadp-operator/pull/1925)).
These token-auth flows create a single set of cloud credentials (one per cloud provider) mounted into the Velero pod, tied to the `velero` Kubernetes service account in the OADP namespace.

However, NonAdminBSL currently requires non-admin users to provide long-term credentials (access keys, service account JSON keys, client secrets) in a Secret within their namespace.
The non-admin controller copies these into the OADP namespace and references them in the Velero BSL.
On clusters using cloud-native identity (ROSA with STS, GCP clusters with WIF, Azure clusters with Workload Identity), non-admin users are forced to create and manage long-term credentials even though the platform provides short-lived token infrastructure.
This is a security concern and a usability gap.

### Current Architecture

The OADP operator declares support for token-based authentication via CSV annotations:

```yaml
features.operators.openshift.io/token-auth-aws: "true"
features.operators.openshift.io/token-auth-azure: "true"
features.operators.openshift.io/token-auth-gcp: "true"
```

The standardized STS flow ([`pkg/credentials/stsflow/stsflow.go`](https://github.com/openshift/oadp-operator/blob/f2f8c5ed3c610dbede4a087347d6bc776038ca8f/pkg/credentials/stsflow/stsflow.go)) detects environment variables set by OLM during installation and creates provider-specific credential secrets:

| Provider | Secret Name | Credential Type | Key Mechanism |
|----------|------------|-----------------|---------------|
| AWS | `cloud-credentials` | INI profile with `role_arn` + `web_identity_token_file` | `AssumeRoleWithWebIdentity` |
| GCP | `cloud-credentials-gcp` | JSON `external_account` with `service_account_impersonation_url` | WIF token exchange + SA impersonation |
| Azure | `cloud-credentials-azure` + `azure-workload-identity-env` | Env-var format with `AZURE_CLIENT_ID` + `AZURE_FEDERATED_TOKEN_FILE` | Azure AD federated token exchange |

All three use the same projected service account token at `/var/run/secrets/openshift/serviceaccount/token` (1-hour expiry, audience `"openshift"`).

### Key Constraint

Velero runs as a **single deployment** with a **single Kubernetes service account** (`velero`).
The projected SA token proves "I am the Velero service account" to all cloud providers.
Velero already supports per-BSL credentials (since v1.6) via the `spec.credential` field on `BackupStorageLocation`, which writes each BSL's credential secret to a temporary file and passes it to the cloud plugin.
However, all per-BSL credential files that reference `web_identity_token_file` use the same projected token.

**The isolation does not come from different tokens — it comes from different cloud IAM roles/identities, each scoped to a specific bucket/prefix.**

## Goals

- Enable NonAdminBSL to use short-lived, cloud-native credentials for AWS STS, GCP WIF, and Azure Workload Identity.
- Ensure per-namespace isolation: NonAdminA's credentials cannot access NonAdminB's backup storage.
- Define a clear admin workflow for provisioning per-namespace cloud identities.

## Non Goals

- Automating cloud-side IAM resource creation (IAM roles, GCP service accounts, Azure managed identities) from within the operator. This is the cluster administrator's responsibility, consistent with OpenShift's Cloud Credential Operator model.
- Major architectural changes to upstream Velero's plugin framework. Targeted fixes to cloud plugin credential handling are in scope.
- Supporting non-admin users who do not have an administrator willing to pre-provision cloud identities.
- Replacing the existing long-term credential flow for NonAdminBSL. Both paths should coexist.

## High-Level Design

The strategy is **"per-BSL credential files with a shared token, cloud-scoped roles."**

### Admin Tasks (one-time setup per tenant namespace)

1. **Create a cloud identity** scoped to a specific bucket/prefix for the tenant namespace (e.g., AWS IAM role, GCP service account, or Azure managed identity).
The cloud identity trusts the cluster's OIDC issuer with subject `system:serviceaccount:openshift-adp:velero`.
The cloud identity's permissions are restricted to `s3://backup-bucket/nonadmin-team-a/*` (or equivalent GCS/Azure Blob path).
2. **Create a credential secret** in the OADP namespace containing a cloud-specific credential file that references the tenant's cloud identity and the shared Velero SA token path (`/var/run/secrets/openshift/serviceaccount/token`).
3. **Configure DPA `credentialMappings`** to map the tenant namespace to its credential secret, specifying `enforceBucketPrefix` to lock the BSL to the correct storage path.

### Non-Admin User Tasks

1. **Create a NonAdminBSL** in their namespace specifying the cloud provider, bucket, and prefix.
The non-admin user does NOT create or reference any credential — the credential is injected automatically by the non-admin controller based on the namespace mapping.
2. **Create NonAdminBackup/NonAdminRestore** resources referencing their NonAdminBSL.

### How NonAdminA is Isolated from NonAdminB

Each tenant namespace gets a **different cloud identity** with permissions scoped to a **different bucket/prefix**.
All tenants share the same Velero service account token — the token proves "I am the Velero pod" to the cloud provider.
The isolation does not come from different tokens; it comes from different cloud IAM roles/identities, each scoped to a specific storage path.

```
Admin provisions:
  team-a → IAM role A (can only access s3://bucket/nonadmin-team-a/*)
  team-b → IAM role B (can only access s3://bucket/nonadmin-team-b/*)

At runtime:
  NonAdminBSL in team-a namespace
    → non-admin controller selects credential A (by namespace mapping)
    → Velero BSL created with spec.credential → credential A
    → Velero assumes IAM role A using shared SA token
    → can ONLY access s3://bucket/nonadmin-team-a/*

  NonAdminBSL in team-b namespace
    → non-admin controller selects credential B (by namespace mapping)
    → Velero BSL created with spec.credential → credential B
    → Velero assumes IAM role B using shared SA token
    → can ONLY access s3://bucket/nonadmin-team-b/*
```

**Why team-a cannot access team-b's backups:**

1. **Credential selection is by namespace, not user input.** The non-admin controller determines which credential to use based on the DPA's `credentialMappings`. Even if a team-a user knows team-b's bucket path or cloud identity details, they cannot influence which credential file is used — it is always credential A for namespace team-a.
2. **Prefix enforcement.** The `enforceBucketPrefix` setting in the credential mapping rejects any NonAdminBSL whose `objectStorage.prefix` does not match the enforced prefix. Team-a's NonAdminBSLs can only target `nonadmin-team-a/`.
3. **Cloud IAM scoping.** Even if credential selection were somehow bypassed, IAM role A's permission policy only allows access to `s3://bucket/nonadmin-team-a/*`. The cloud provider enforces this regardless of what the BSL configuration says.
4. **Credential secrets are invisible to non-admin users.** The credential secrets reside in the OADP namespace (`openshift-adp`), which non-admin users have no RBAC access to. They cannot read, modify, or discover the credential files or the cloud identity details they contain.

## Detailed Design

### 1. Per-Namespace Credential Formats

Each cloud provider uses a different credential file format, but the pattern is the same: **substitute the cloud identity (role/SA/client-id) while keeping the same token file path.**

#### AWS STS

Per-namespace credential file (INI format):

```ini
[default]
sts_regional_endpoints = regional
role_arn = arn:aws:iam::123456789012:role/oadp-nonadmin-<namespace>
web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token
```

- Each namespace gets a different `role_arn`
- All share the same `web_identity_token_file` (Velero's projected SA token)
- The IAM role's trust policy trusts the cluster's OIDC provider with subject `system:serviceaccount:openshift-adp:velero`
- The IAM role's permission policy restricts S3 access to a specific bucket/prefix

**IAM role trust policy:**
```json
{
  "Effect": "Allow",
  "Principal": {"Federated": "arn:aws:iam::123456789012:oidc-provider/<OIDC_URL>"},
  "Action": "sts:AssumeRoleWithWebIdentity",
  "Condition": {
    "StringEquals": {
      "<OIDC_URL>:sub": "system:serviceaccount:openshift-adp:velero"
    }
  }
}
```

**IAM role permission policy (scoped to namespace prefix):**
```json
{
  "Effect": "Allow",
  "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:ListBucket"],
  "Resource": [
    "arn:aws:s3:::backup-bucket",
    "arn:aws:s3:::backup-bucket/nonadmin-<namespace>/*"
  ],
  "Condition": {
    "StringLike": {"s3:prefix": ["nonadmin-<namespace>/*"]}
  }
}
```

**BSL config requirement:** `enableSharedConfig: "true"` must be set for STS credential files to work (already implemented in [`internal/controller/bsl.go`](https://github.com/openshift/oadp-operator/blob/f2f8c5ed3c610dbede4a087347d6bc776038ca8f/internal/controller/bsl.go)).

#### GCP WIF

Per-namespace credential file (JSON `external_account` format):

```json
{
  "type": "external_account",
  "audience": "//iam.googleapis.com/projects/<PROJECT_NUMBER>/locations/global/workloadIdentityPools/<POOL_ID>/providers/<PROVIDER_ID>",
  "subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
  "token_url": "https://sts.googleapis.com/v1/token",
  "service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/oadp-nonadmin-<namespace>@<project>.iam.gserviceaccount.com:generateAccessToken",
  "credential_source": {
    "file": "/var/run/secrets/openshift/serviceaccount/token",
    "format": {"type": "text"}
  }
}
```

- Each namespace gets a different `service_account_impersonation_url` pointing to a namespace-specific GCP service account
- The same WIF pool/provider is reused (no per-namespace pool needed)
- The GCP SA has a workload identity binding allowing impersonation by the Velero K8s SA:

```bash
gcloud iam service-accounts add-iam-policy-binding \
  oadp-nonadmin-<namespace>@<project>.iam.gserviceaccount.com \
  --role roles/iam.workloadIdentityUser \
  --member "principal://iam.googleapis.com/projects/<PROJECT_NUMBER>/locations/global/workloadIdentityPools/<POOL_ID>/subject/system:serviceaccount:openshift-adp:velero"
```

- The GCP SA's IAM permissions are scoped to the namespace's GCS prefix

**No changes needed to velero-plugin-for-gcp.** The Google Cloud SDK's `external_account` credential type is already fully supported (detected automatically by `option.WithCredentialsFile()`).

#### Azure Workload Identity

Per-namespace credential file (env-var format):

```
AZURE_SUBSCRIPTION_ID=<subscription-id>
AZURE_TENANT_ID=<tenant-id>
AZURE_CLIENT_ID=<per-namespace-managed-identity-client-id>
AZURE_CLOUD_NAME=AzurePublicCloud
AZURE_FEDERATED_TOKEN_FILE=/var/run/secrets/openshift/serviceaccount/token
```

- Each namespace gets a different `AZURE_CLIENT_ID` pointing to a namespace-specific Azure User-Assigned Managed Identity
- Each managed identity has a federated identity credential trusting the cluster's OIDC issuer with subject `system:serviceaccount:openshift-adp:velero`
- Each managed identity has a role assignment scoped to a specific Azure Blob container

```bash
# Create federated identity credential
az identity federated-credential create \
  --name "velero-<namespace>" \
  --identity-name "oadp-nonadmin-<namespace>" \
  --resource-group "<rg>" \
  --issuer "<CLUSTER_OIDC_ISSUER>" \
  --subject "system:serviceaccount:openshift-adp:velero"

# Scope storage access
az role assignment create \
  --role "Storage Blob Data Contributor" \
  --assignee-object-id <managed-identity-principal-id> \
  --scope "/subscriptions/<SUB_ID>/resourceGroups/<RG>/providers/Microsoft.Storage/storageAccounts/<ACCOUNT>/blobServices/default/containers/<namespace>-backups"
```

**Azure requires an upstream Velero fix.**
Code-level analysis of [`velero/pkg/util/azure/credential.go:48-54`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go#L48-L54) confirms that the Workload Identity path reads **only from environment variables**, ignoring the per-BSL `creds` map entirely:

```go
// Current code (BROKEN for per-BSL WI):
if len(os.Getenv("AZURE_FEDERATED_TOKEN_FILE")) > 0 {
    return azidentity.NewWorkloadIdentityCredential(&azidentity.WorkloadIdentityCredentialOptions{
        AdditionallyAllowedTenants: additionalTenants,
        ClientOptions:              options,
    })
}
```

The `azidentity.NewWorkloadIdentityCredential` with no explicit `TenantID`/`ClientID`/`TokenFilePath` options falls back to reading `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_FEDERATED_TOKEN_FILE` from environment variables.
Even though [`LoadCredentials()`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/util.go#L57-L76) correctly parses the per-BSL credential file via `godotenv.Read()` into the `creds` map, the WI branch never reads from `creds`.
All BSLs share the same pod-level Azure identity regardless of what's in their per-BSL credential file.
See [Upstream Prerequisites](#upstream-prerequisites-azure-workload-identity-fix) for the required fix.

### 2. Admin Workflow

The cluster administrator is responsible for provisioning cloud-side resources before non-admin users can use short-lived credentials.

#### Step 1: Create Cloud Identities (per namespace)

For each non-admin namespace that needs short-lived credentials:

**AWS:**
1. Create an IAM role with a trust policy for the cluster's OIDC provider and the Velero SA subject
2. Attach a permission policy scoped to the namespace's S3 prefix

**GCP:**
1. Create a GCP service account
2. Add a workload identity binding allowing impersonation from the Velero K8s SA
3. Grant GCS permissions scoped to the namespace's prefix

**Azure:**
1. Create a User-Assigned Managed Identity
2. Create a federated identity credential trusting the cluster's OIDC issuer and Velero SA subject
3. Assign `Storage Blob Data Contributor` role scoped to the namespace's blob container

#### Step 2: Create Credential Secrets

Create the per-namespace credential secret in the OADP namespace:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: nonadmin-creds-<namespace>
  namespace: openshift-adp
  labels:
    oadp.openshift.io/secret-type: sts-credentials
    oadp.openshift.io/nonadmin-namespace: <namespace>
type: Opaque
stringData:
  cloud: |
    <provider-specific credential content from Section 1>
```

#### Step 3: Configure DPA NonAdmin Section

A new field in the DPA `NonAdmin` struct maps namespaces to their credential secrets:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: dpa
  namespace: openshift-adp
spec:
  nonAdmin:
    enable: true
    credentialMappings:
      - namespaces: ["team-a"]
        credential:
          name: nonadmin-creds-team-a
          key: cloud
        provider: aws
        enforceBucketPrefix: "nonadmin-team-a"
      - namespaces: ["team-b", "team-c"]
        credential:
          name: nonadmin-creds-team-b
          key: cloud
        provider: gcp
        enforceBucketPrefix: "nonadmin-team-b"
```

**Alternative: Auto-discovery.** Instead of explicit mapping, the operator could discover credential secrets by label (`oadp.openshift.io/nonadmin-namespace: <namespace>`) in the OADP namespace.
This reduces DPA configuration burden but requires a labeling convention.

### 3. Non-Admin Controller Changes

The non-admin controller (in [`migtools/oadp-non-admin`](https://github.com/migtools/oadp-non-admin)) needs these changes:

#### 3a. Credential Secret Resolution

When creating a Velero BSL from a NonAdminBSL, the non-admin controller must:

1. Check if a per-namespace credential mapping exists (via DPA config or label-based discovery)
2. If yes, set the Velero BSL's `spec.credential` to reference the mapped credential secret in the OADP namespace
3. If no mapping exists, fall back to the existing behavior (copy long-term credential secret from user namespace)

#### 3b. BSL Config Injection

For AWS STS credentials, the controller must ensure `enableSharedConfig: "true"` is set in the BSL config.
This is already implemented for admin-level BSLs in [`internal/controller/bsl.go`](https://github.com/openshift/oadp-operator/blob/f2f8c5ed3c610dbede4a087347d6bc776038ca8f/internal/controller/bsl.go) but needs to be replicated in the non-admin flow.

#### 3c. Prefix Enforcement

When a credential mapping specifies `enforceBucketPrefix`, the controller must validate that the NonAdminBSL's `objectStorage.prefix` matches the enforced prefix.
This ensures the BSL can only access the bucket path that the cloud IAM identity is scoped to.

### 4. API Changes

#### DPA NonAdmin Struct Extension

```go
type NonAdmin struct {
    // ... existing fields ...

    // CredentialMappings maps non-admin namespaces to pre-provisioned cloud credential secrets.
    // When a NonAdminBSL is created in a mapped namespace, the corresponding credential is used
    // instead of requiring the non-admin user to provide long-term credentials.
    // +optional
    CredentialMappings []NonAdminCredentialMapping `json:"credentialMappings,omitempty"`
}

type NonAdminCredentialMapping struct {
    // Namespaces is the list of non-admin namespaces this credential applies to.
    // +kubebuilder:validation:MinItems=1
    Namespaces []string `json:"namespaces"`

    // Credential references the pre-provisioned credential secret in the OADP namespace.
    Credential corev1.SecretKeySelector `json:"credential"`

    // Provider is the cloud provider for this credential (aws, gcp, azure).
    // +kubebuilder:validation:Enum=aws;gcp;azure
    Provider string `json:"provider"`

    // EnforceBucketPrefix restricts NonAdminBSLs using this credential to a specific
    // objectStorage.prefix value, ensuring alignment with the cloud IAM policy scope.
    // +optional
    EnforceBucketPrefix string `json:"enforceBucketPrefix,omitempty"`
}
```

### 5. Projected Token Volume

The existing projected SA token volume in the Velero deployment ([`internal/controller/velero.go` lines 321-336](https://github.com/openshift/oadp-operator/blob/f2f8c5ed3c610dbede4a087347d6bc776038ca8f/internal/controller/velero.go#L321-L336)) is sufficient.
All per-namespace credential files reference the same token path `/var/run/secrets/openshift/serviceaccount/token`.
No changes needed to the token volume configuration.

### 6. Validation

The DPA reconciler should validate:

1. Each credential mapping's secret exists in the OADP namespace
2. No namespace appears in multiple credential mappings
3. The credential secret contains the expected key
4. If `enforceBucketPrefix` is set, it is non-empty and valid

The non-admin controller should validate:

1. When a NonAdminBSL specifies a credential *and* a credential mapping exists for the namespace, warn and prefer the mapping
2. When `enforceBucketPrefix` is set, the NonAdminBSL's `objectStorage.prefix` must match
3. The BSL provider matches the credential mapping's provider

## Upstream Prerequisites: Azure Workload Identity Fix

Per-BSL Azure Workload Identity requires a targeted fix in upstream Velero at [`pkg/util/azure/credential.go`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go).
The `NewCredential` function must read WI parameters from the `creds` map (populated from the per-BSL credential file) instead of exclusively from environment variables.

**Required change in [`velero/pkg/util/azure/credential.go`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go#L48-L54):**

```go
// Fixed code: read from creds map (per-BSL) with env var fallback
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

**Why this is safe:**
- When no per-BSL credential file is provided, `creds` is empty, so the function falls back to env vars (existing behavior preserved).
- When a per-BSL credential file IS provided, `LoadCredentials()` parses the KEY=VALUE file via `godotenv.Read()`, populating `creds["AZURE_CLIENT_ID"]`, `creds["AZURE_TENANT_ID"]`, and `creds["AZURE_FEDERATED_TOKEN_FILE"]`.
- The `azidentity.WorkloadIdentityCredentialOptions` struct already has `TenantID`, `ClientID`, and `TokenFilePath` fields — they are simply unused in the current code.

**Delivery:** This fix should be submitted as an upstream PR to `vmware-tanzu/velero` before or in parallel with the OADP non-admin credential work.
The fix is backward-compatible and benefits all Velero users, not just OADP.

## Verified Per-BSL Credential Flow by Provider

The following analysis is based on code-level tracing through Velero's credential handling pipeline.

### Common Flow

When a BSL has `spec.credential` set, Velero's [`persistence/object_store.go:170-178`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/persistence/object_store.go#L170-L178) calls `credentialStore.Path()` which:
1. Reads the referenced Secret from Kubernetes
2. Writes the secret data to a temp file at `<fsRoot>/<namespace>/<secretName>-<key>`
3. Injects the temp file path as `config["credentialsFile"]` into the plugin's `Init()` config map

Each cloud plugin then reads `config["credentialsFile"]` during initialization.

### AWS STS: Verified Working

**Code path:** `velero-plugin-for-aws/config.go` → `WithCredentialsFile()`

1. The AWS plugin receives `config["credentialsFile"]` (per-BSL temp file path)
2. Passes it to both `config.WithSharedCredentialsFiles` and `config.WithSharedConfigFiles`
3. The AWS SDK v2 shared config loader natively parses `web_identity_token_file` + `role_arn` from INI profiles and sets up `AssumeRoleWithWebIdentity`
4. **IRSA env var clearing:** When a per-BSL credential file is provided, the plugin **clears pod-level IRSA env vars** (`AWS_WEB_IDENTITY_TOKEN_FILE`, `AWS_ROLE_ARN`, `AWS_ROLE_SESSION_NAME`) to prevent the SDK from using pod-level identity instead of the per-BSL credential

**Caveat:** The IRSA env var clearing is a process-wide side effect.
If Velero processes BSLs sequentially in the same plugin process, a BSL WITH per-BSL credentials will clear IRSA env vars, potentially causing a subsequent BSL WITHOUT per-BSL credentials (relying on pod IRSA) to fail.
This is a known design pattern in the AWS plugin.
For the NonAdminBSL use case, all BSLs should have explicit per-BSL credentials, so this is not a concern.

**`enableSharedConfig` note:** With AWS SDK v2 (plugin v1.10+), `enableSharedConfig` is accepted but ignored — `WithSharedConfigFiles()` always enables shared config parsing.
It was required in SDK v1 (plugin v1.7.x).
Setting it remains harmless and aids backward compatibility.

**Verdict: No changes needed. Per-BSL STS credentials work today.**

### GCP WIF: Verified Working

**Code path:** GCP plugin `object_store.go Init()` → `option.WithCredentialsFile()` → `google.CredentialsFromJSON()`

1. The GCP plugin receives `config["credentialsFile"]` and passes it to `option.WithCredentialsFile()`
2. The Google Auth library reads the JSON file, detects `type: "external_account"`, and creates an `externalaccount.Config`
3. The `credential_source.file` path (`/var/run/secrets/openshift/serviceaccount/token`) is an **absolute path** — the library uses `os.Open(cs.File)` directly (`golang.org/x/oauth2/google/externalaccount/filecredsource.go:25`), so it resolves correctly regardless of where the temp credential JSON is stored
4. Each per-BSL credential file can specify a different `service_account_impersonation_url` pointing to a namespace-specific GCP service account

**Known limitation:** `CreateSignedURL()` fails for `external_account` credentials because `googleAccessID` is not set (no private key available for signing).
This affects `velero backup download` / download URL functionality but does NOT affect backup/restore operations.

**Verdict: No changes needed. Per-BSL WIF credentials work today for core backup/restore.**

### Azure WI: Verified Broken, Fix Required

**Code path:** [`storage.go:63`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/storage.go#L63) → [`util.go:57`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/util.go#L57) → [`credential.go:31`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go#L31)

1. `NewStorageClient()` calls `LoadCredentials(config)` which reads the per-BSL credential file via `godotenv.Read()` into a `creds` map — this step works correctly ✅
2. `NewCredential(creds, clientOptions)` is called at `credential.go:31`
3. The WI branch at line 49 checks `os.Getenv("AZURE_FEDERATED_TOKEN_FILE")` — **environment variable, not the `creds` map** ❌
4. `azidentity.NewWorkloadIdentityCredential` uses env vars for `TenantID`, `ClientID`, `TokenFilePath` — **per-BSL values ignored** ❌

**Verdict: Upstream fix required. See [Upstream Prerequisites](#upstream-prerequisites-azure-workload-identity-fix).**

## Prior Art from Similar Projects

The per-namespace credential scoping pattern proposed here is well-established across the Kubernetes ecosystem.
Several projects solve similar problems and inform our design:

### Crossplane `ProviderConfig`

[Crossplane](https://github.com/crossplane/crossplane) uses a `ProviderConfig` resource to map cloud credentials to managed resources.
Each `ProviderConfig` can reference a different cloud credential (Secret, IRSA, WIF) and resources select which `ProviderConfig` to use via `providerConfigRef`.
This is analogous to our `CredentialMappings` concept, where each NonAdminBSL implicitly selects a credential based on its namespace.

**Pattern adopted:** Admin-controlled credential provisioning with resource-level selection.

### External Secrets Operator `SecretStore`/`ClusterSecretStore`

[External Secrets](https://github.com/external-secrets/external-secrets) uses namespace-scoped `SecretStore` and cluster-scoped `ClusterSecretStore` resources to define per-namespace cloud authentication.
Each `SecretStore` can have its own IRSA role annotation or GCP WIF configuration, and `ExternalSecret` resources in that namespace use the local `SecretStore`.

**Pattern adopted:** Namespace-level credential scoping where the admin provisions the credential binding and non-admin users consume it without needing to know the cloud-side details.

### cert-manager `ClusterIssuer`/`Issuer`

[cert-manager](https://github.com/cert-manager/cert-manager) supports namespace-scoped `Issuer` and cluster-scoped `ClusterIssuer` resources with per-issuer cloud credentials for DNS01 challenges.
Each issuer can reference a different cloud credential secret, and cert-manager's controller resolves the appropriate credential when processing each certificate request.

**Pattern adopted:** Single controller deployment (like Velero) handling multiple credential contexts by passing per-resource credentials to cloud API calls.

### Cluster API `allowedNamespaces` Pattern

[Cluster API](https://github.com/kubernetes-sigs/cluster-api) uses cluster-scoped identity resources (`AWSClusterRoleIdentity`, `AzureClusterIdentity`) with an `allowedNamespaces` field that explicitly controls which namespaces may reference them.
If `allowedNamespaces` is nil: no namespace can use it (deny-all default).
If `allowedNamespaces` is empty object: any namespace can use it.
This provides a declarative, auditable access control layer on credential resources.

**Pattern adopted:** If OADP introduces a cluster-scoped credential binding resource, it should use `allowedNamespaces` with a nil default (deny-all) to prevent accidental cross-tenant access.

### Gateway API `ReferenceGrant` Pattern

The [Gateway API](https://gateway-api.sigs.k8s.io/api-types/referencegrant/) uses `ReferenceGrant` (namespace-scoped) to explicitly authorize cross-namespace references.
This pattern requires the target namespace owner to consent to references from other namespaces.
It was designed to prevent confused deputy attacks (CVE-2021-25740-style).

**Pattern adopted:** If NonAdminBSL needs to reference resources across namespaces, a ReferenceGrant-style handshake ensures the resource owner explicitly permits the reference.

### OpenShift CCO `CredentialsRequest`

The [Cloud Credential Operator](https://github.com/openshift/cloud-credential-operator) is the closest prior art.
It creates per-component scoped credentials with STS via `CredentialsRequest`.
Each request specifies `serviceAccountNames` (which K8s SAs will use the credential) and `cloudTokenPath` (projected SA token mount).
Extending this pattern to per-namespace non-admin credentials is a natural evolution.

### AWS Session Tags (Alternative to Per-Namespace IAM Roles)

AWS STS `AssumeRole` supports up to 50 session tags.
A single "non-admin backup" IAM role could use ABAC policies with `aws:PrincipalTag/Namespace` to restrict S3 access to tenant-specific prefixes, avoiding the need for one IAM role per namespace.
This reduces IAM sprawl but requires the operator to inject session tags during role assumption.

**Consideration:** This is a potential future optimization but adds complexity to the initial implementation.

### Key Lessons from Prior Art

1. **Admin provisions, user consumes:** All successful multi-tenant credential patterns separate the admin workflow (creating cloud identities and binding them) from the user workflow (creating workloads that consume pre-provisioned credentials). Our design follows this pattern.

2. **Credential resolution at the controller level:** Crossplane, External Secrets, and cert-manager all resolve credentials in the controller, not in the workload. Similarly, the OADP non-admin controller (not the Velero pod itself) should resolve which credential to use for each NonAdminBSL.

3. **Cloud-side scoping is essential:** All projects emphasize that Kubernetes RBAC alone is insufficient — cloud IAM policies must enforce the isolation boundary. Our design's reliance on cloud-scoped roles/SAs/identities aligns with this.

4. **Avoid per-namespace service accounts in a shared deployment:** Projects like External Secrets explored and rejected per-namespace Kubernetes service accounts for shared controllers, preferring the "single SA + multiple cloud roles" pattern. This validates our approach.

5. **Validate, don't just proxy:** The ["Breaking the Bulkhead" paper (NDSS 2026)](https://arxiv.org/abs/2507.03387) found 14% of Kubernetes operators are vulnerable to cross-namespace reference attacks. The operator MUST validate that users creating NonAdminBSLs cannot influence which credential is selected — the credential must be determined entirely by namespace, not by user-specified fields.

6. **Token projection over Secret copying:** Where possible, use projected SA tokens (mounted volumes with automatic rotation) rather than copying static credential Secrets between namespaces. This eliminates an entire class of credential leakage.

## Alternatives Considered

### Per-Namespace Kubernetes Service Accounts with IRSA/WIF

Instead of sharing the Velero SA token with different cloud roles, each non-admin namespace could get its own K8s service account with its own IRSA/WIF binding.
This would require either:
- Multiple Velero deployments (one per namespace) — impractical
- A sidecar/proxy that can present different SA identities per request — complex

Rejected because the shared-token + cloud-scoped-roles approach achieves the same isolation with much less complexity.

### Operator-Managed Cloud IAM Resources

The operator could create IAM roles, GCP SAs, and Azure managed identities automatically when a NonAdminBSL is created.
This would require the operator to have cloud admin permissions, which violates the principle of least privilege and is inconsistent with OpenShift's CCO model.
This could be a future enhancement but is out of scope for the initial implementation.

### NonAdminBSL-Level Credential Override

Instead of namespace-level credential mapping in the DPA, each NonAdminBSL could directly reference a credential secret.
This is already supported (the `credential` field exists on NonAdminBSL), but for short-lived credentials the secret must exist in the OADP namespace (not the user's namespace) because it references a token file path only available in the Velero pod.
The namespace-level mapping in the DPA provides a cleaner admin-controlled workflow.

## Security Considerations

### Isolation Model

Isolation relies on **cloud-side IAM scoping**, not on Kubernetes-level token differentiation.
All per-namespace credential files use the same projected Velero SA token.
The token proves "I am the Velero service account" to the cloud provider.
The credential file's role/SA/client-ID determines which cloud permissions are granted.
Each cloud role/SA/identity is scoped to a specific bucket/prefix.

### Shared-Token Trust Model

All three cloud providers share a fundamental characteristic: with a single Velero service account, the projected SA token is identical for all per-namespace credential files.
The token proves "I am `system:serviceaccount:openshift-adp:velero`" to the cloud provider.
**No cloud provider can distinguish which namespace or BSL triggered the request based on the token alone.**
Isolation relies entirely on (1) the operator-level credential selection logic and (2) cloud-side IAM scoping on each target identity.

**IMPORTANT: The non-admin controller MUST NOT allow users to specify arbitrary credential references in their NonAdminBSL.**
If a NonAdminBSL specifies a `credential` field AND a `CredentialMapping` exists for the namespace, the controller MUST ignore the user-specified credential and use the admin-provisioned mapping.
If no mapping exists, the controller falls back to the existing behavior (long-term credential in user's namespace).

### Common Mitigations (all providers)

These mitigations apply to all three cloud providers and MUST all be enforced:

1. **Credential secrets are admin-provisioned, not user-controlled:**
Non-admin users do NOT create or specify credential files.
The `CredentialMappings` in the DPA are configured by the cluster admin, and the non-admin controller selects the credential based on the user's namespace — not based on any user-specified field.
Non-admin users only create a NonAdminBSL with `provider`, `bucket`, `prefix`, and `objectStorage` fields; the credential is injected by the controller.

2. **Prefix enforcement prevents bucket/path redirection:**
The `enforceBucketPrefix` field in `CredentialMappings` ensures a NonAdminBSL in a mapped namespace can only target the specific bucket prefix that the cloud IAM identity is scoped to.
The non-admin controller MUST reject any NonAdminBSL whose `objectStorage.prefix` does not match the enforced prefix.
Combined with `EnforceBSLSpec` (which can lock down the bucket name), this prevents users from redirecting their BSL to another tenant's path.

3. **Non-admin users never see credential secrets:**
The credential secrets reside in the OADP namespace (`openshift-adp`) which is not accessible to non-admin users.
Non-admin users cannot inspect, modify, or copy credential files.
They cannot discover another tenant's cloud identity details (GCP SA email, IAM role ARN, Azure managed identity client ID) from within Kubernetes.

4. **Cloud-side IAM scoping limits blast radius:**
Each cloud identity is scoped to a specific bucket/prefix/container.
Even if a credential were somehow misrouted, the cloud IAM policy restricts access to only that identity's designated storage scope.

5. **NonAdminBSL approval workflow (optional additional defense):**
When `RequireApprovalForBSL: true` is set in the DPA, an admin must approve each NonAdminBSL before a Velero BSL is created.
This provides a human-in-the-loop check for the BSL configuration.

### AWS STS Impersonation Risk Analysis

**Core risk:** The Velero SA token can be used to call `sts:AssumeRoleWithWebIdentity` for any IAM role whose trust policy includes the Velero SA's `sub` claim (`system:serviceaccount:openshift-adp:velero`).
AWS IAM evaluates only: (1) is the OIDC token valid? (2) does the role's trust policy allow the token's `sub` claim?
Since all per-namespace IAM roles trust the same `sub`, any process with access to the token can assume any of these roles.

**AWS-specific constraints:**

| Mechanism | Applicability | Verdict |
|-----------|--------------|---------|
| `sts:ExternalId` | Does NOT work with `AssumeRoleWithWebIdentity` — only available on `AssumeRole` | Not applicable |
| `<OIDC_URL>:sub` condition | Same `sub` for all namespaces (single Velero SA) | Cannot differentiate |
| `<OIDC_URL>:aud` condition | Same audience for all namespaces (e.g., `"openshift"`) | Cannot differentiate |
| Custom JWT claims | AWS IAM only supports `sub`, `aud`, `amr` from OIDC tokens — no custom claims like `kubernetes.io/namespace` | Not supported |
| `sts:SourceIdentity` | Could scope per-namespace, but AWS SDK's shared config provider does not support setting it via INI profile | Future enhancement (requires plugin changes) |
| Session tags (`aws:PrincipalTag`) | Same limitation — SDK doesn't support via INI profile | Future enhancement |
| Session policies | Can restrict permissions at assumption time, but SDK doesn't support via INI profile | Future enhancement |

**AWS-specific mitigations (beyond common mitigations):**

1. **Per-role IAM permission policy scoping:** Each IAM role's permission policy MUST restrict S3 access to `arn:aws:s3:::backup-bucket/nonadmin-<namespace>/*`. Even if a role is incorrectly assumed, the blast radius is limited to that namespace's S3 prefix.

2. **Resource Control Policies (RCPs):** Deploy an RCP using the `EnforceTrustedOIDCProviders` pattern from [AWS data-perimeter-policy-examples](https://github.com/aws-samples/data-perimeter-policy-examples) to ensure only the cluster's OIDC provider can federate into per-namespace roles. This prevents external OIDC providers from assuming the roles.

3. **Trust policy hardening:** Use `StringEquals` (not `StringLike`) for the `sub` condition. Document exact trust policy examples for administrators. Avoid wildcard patterns that could match unintended subjects.

4. **CloudTrail monitoring:** Every `AssumeRoleWithWebIdentity` call generates a CloudTrail event with the `roleArn`, `subjectFromWebIdentityToken`, and source IP. Create CloudWatch alarms for unexpected role assumption patterns on `oadp-nonadmin-*` roles.

5. **Separate AWS accounts per tenant (strongest isolation):** Each tenant's IAM role and S3 bucket in a separate AWS account eliminates cross-tenant S3 access even if role assumption succeeds. The per-account IAM boundary is the strongest isolation AWS provides.

**Future enhancement:** Modify the Velero AWS plugin to set `SourceIdentity` to the namespace name during `AssumeRoleWithWebIdentity`, and enforce the value in each role's trust policy. This would add a cryptographic per-namespace binding at the AWS level but requires upstream plugin changes.

### GCP WIF Impersonation Risk Analysis

**Core risk:** The Velero SA's federated identity (via WIF pool subject `system:serviceaccount:openshift-adp:velero`) can call `generateAccessToken` on any GCP service account that has a `roles/iam.workloadIdentityUser` binding for that subject.
The `external_account` credential JSON is a client-side hint — the real authorization boundary is the IAM binding on each target GCP SA.
If SA-A and SA-B both have `workloadIdentityUser` bindings for the same WIF subject, that subject can impersonate both.

**GCP-specific constraints:**

| Mechanism | Applicability | Verdict |
|-----------|--------------|---------|
| WIF `attributeCondition` (CEL) | Controls token exchange gate, NOT impersonation scope | Cannot restrict which SAs are impersonated |
| `principal://` vs `principalSet://` | Both bind to the same subject — doesn't differentiate | Not helpful |
| Separate WIF pools | Creates different principal IDs, but heavyweight | Not practical per-namespace |
| IAM Conditions on bindings | Can restrict at project level with `resource.name` filter | Useful for defense-in-depth |
| IAM Deny Policies | Can block impersonation of specific SAs | Useful but requires maintenance |
| Principal Access Boundary (PAB) | Can restrict which resources a principal is eligible to access | Additional defense layer |

**GCP-specific mitigations (beyond common mitigations):**

1. **Minimize IAM scope on each GCP SA:** Each per-namespace GCP SA should have the minimum possible GCS permissions, scoped to its specific bucket/prefix. Use IAM conditions on `objectAdmin` bindings to restrict to specific GCS prefixes.

2. **Credential file URL validation:** When the operator/controller creates or processes credential files, validate that:
   - `token_url` is `https://sts.googleapis.com/v1/token`
   - `service_account_impersonation_url` matches `https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/*:generateAccessToken`
   - `credential_source.file` points to the expected projected SA token path
   Google's auth libraries do NOT validate these URLs against known domains — a tampered credential file could redirect token exchange to an attacker-controlled server.

3. **Enable GCP audit logging:** Enable Data Access audit logs for both the **Security Token Service API** and the **IAM API**. Monitor `GenerateAccessToken` events for unexpected SA impersonation patterns. Each event includes `protoPayload.authenticationInfo.principalSubject` (the federated principal) and `request.name` (the target GCP SA).

4. **Use `attributeCondition` on WIF provider:** While this cannot restrict per-SA impersonation, it ensures only the Velero SA can authenticate:
   ```cel
   assertion.sub == "system:serviceaccount:openshift-adp:velero"
   ```

5. **Organization Policy constraints:** Use `constraints/iam.workloadIdentityPoolProviders` to restrict which OIDC issuer URIs can be configured in WIF pools. This prevents unauthorized WIF pool creation.

6. **IAM deny policies (optional defense-in-depth):** Create deny policies blocking the Velero federated identity from impersonating GCP SAs outside the known set of per-namespace SAs. Requires maintenance as namespaces are added/removed.

### Azure WI Impersonation Risk Analysis

**Core risk:** The Velero SA token can be exchanged for an Azure AD access token for any User-Assigned Managed Identity (MI) that has a federated identity credential (FIC) matching the token's `iss`, `sub`, and `aud` claims.
The `AZURE_CLIENT_ID` in the credential file determines which MI is targeted.
Unlike GCP, Azure requires an **explicit** FIC on each MI — but the FIC only validates `iss` (issuer), `sub` (subject), and `aud` (audience), with no support for additional conditions.

**Azure-specific constraints:**

| Mechanism | Applicability | Verdict |
|-----------|--------------|---------|
| Federated identity credential conditions | Only validates `iss`, `sub`, `aud` — no IP, custom claims, or conditions | Cannot differentiate per-namespace |
| Flexible Federated Identity Credentials (Preview) | Only available for app registrations, NOT managed identities | Not applicable |
| Conditional Access policies | Do NOT apply to managed identities (only service principals) | Not applicable |
| Azure ABAC for Blob Storage | GA — supports conditions based on `@Principal` custom security attributes | Powerful defense-in-depth |
| Azure Policy on FIC creation | Can block/require naming conventions for FIC creation | Restricts who can establish trust |
| Per-cluster OIDC issuer (ARO) | Each ARO cluster has a unique, unforgeable OIDC issuer URL | Prevents cross-cluster token confusion |

**Azure-specific limits:**

| Resource | Limit |
|----------|-------|
| Federated identity credentials per managed identity | 20 (not a concern for 1-FIC-per-MI design) |
| Managed identities per tenant (verified domain) | 300,000 |
| Managed identities per subscription per region (creation rate) | 80 ops / 20 seconds |

**Azure-specific mitigations (beyond common mitigations):**

1. **Per-container RBAC scoping:** Each managed identity's `Storage Blob Data Contributor` role assignment MUST be scoped to its specific Azure Blob container:
   ```
   --scope "/subscriptions/<SUB>/resourceGroups/<RG>/providers/Microsoft.Storage/storageAccounts/<ACCOUNT>/blobServices/default/containers/<namespace>-backups"
   ```
   Azure RBAC supports container-level role assignments, providing strong per-namespace isolation.

2. **Azure ABAC conditions on storage (recommended defense-in-depth):** Assign custom security attributes (e.g., `BackupNamespace=team-a`) to each namespace's managed identity, then create role assignments with ABAC conditions:
   ```
   @Resource[Microsoft.Storage/storageAccounts/blobServices/containers:name] StringEquals @Principal[Microsoft.Directory/CustomSecurityAttributes/Id:OADP_BackupNamespace]
   ```
   This ensures each MI can only access the container matching its attribute, even if an unconditional role assignment exists at a higher scope.

3. **Azure RBAC on FIC management:** Restrict who can create/modify federated identity credentials on managed identities. Adding a FIC establishes the trust relationship — this is the critical operation to protect. Use Azure Policy to require specific naming conventions or tags on MIs.

4. **Monitoring and alerting:** Stream `ManagedIdentitySignInLogs` to Azure Log Analytics. Monitor for unexpected managed identity sign-ins. Alert on `AADSTS70021` errors which indicate federated credential mismatches. Correlate token exchange events with storage access logs.

5. **`useAAD: "true"` on BSL config:** For Azure WI with per-BSL credentials, the BSL config MUST include `useAAD: "true"` to prevent the Azure plugin from attempting storage account key exchange (which requires `resourceGroup` and management plane permissions). The non-admin controller should automatically set this.

6. **FIC audience must match projected token:** The federated identity credential's `audience` field MUST match the projected SA token's `aud` claim. For OADP, the projected token uses audience `"openshift"` (configured in [`internal/controller/velero.go` lines 321-336](https://github.com/openshift/oadp-operator/blob/f2f8c5ed3c610dbede4a087347d6bc776038ca8f/internal/controller/velero.go#L321-L336)), so the FIC must specify `"openshift"` — NOT Azure's default `"api://AzureADTokenExchange"`. Mismatched audiences cause silent authentication failures (`AADSTS70021`).

7. **Disable shared key access on storage accounts:** Set `--allow-shared-key-access false` on the storage account to force Azure AD-only authentication. This prevents bypass of RBAC scoping via storage account access keys or SAS tokens.

### Impersonation Risk Summary

| Risk | AWS STS | GCP WIF | Azure WI |
|------|---------|---------|----------|
| **Can same token target multiple identities?** | Yes — any role with matching trust policy | Yes — any SA with `workloadIdentityUser` binding | Yes — any MI with matching FIC |
| **Cloud-level per-namespace scoping?** | No — `sub` claim is identical for all | No — same WIF subject for all | No — `sub` claim is identical for all |
| **Isolation mechanism** | Per-role S3 permission policy | Per-SA GCS IAM bindings | Per-MI Azure RBAC container scope |
| **Strongest cloud defense** | Separate AWS accounts per tenant | IAM deny policies + audit logging | Azure ABAC on storage with custom attributes |
| **Future enhancement** | `SourceIdentity` injection (requires plugin changes) | Credential Access Boundaries | Flexible FIC for MIs (when available) |
| **Admin misconfiguration risk** | Trust policy with `StringLike` wildcard | Missing IAM scope on GCP SA | Role assignment at storage account (not container) scope |

**Key conclusion:** For all three providers, the isolation model is sound **as long as the common mitigations are enforced** (admin-provisioned credentials, prefix enforcement, credential secrets in OADP namespace).
The operator-level credential selection logic is the primary security boundary.
Cloud-side IAM scoping provides defense-in-depth.
No cloud provider offers a mechanism to restrict which identities a given OIDC subject can target based solely on the token — this is a fundamental architectural property of the shared-token model.

### Blast Radius

If a per-namespace credential secret is compromised:
- **AWS:** Attacker can only access the S3 prefix scoped to that namespace's IAM role
- **GCP:** Attacker can only access the GCS prefix scoped to that namespace's GCP SA
- **Azure:** Attacker can only access the Azure Blob container scoped to that namespace's managed identity
- Other namespaces' backup data is unaffected

The credential secrets reside in the OADP namespace, which is not accessible to non-admin users.
Non-admin users never see or interact with the cloud credential files.

### Token Security

The projected SA token is bound to the `velero` service account, has a 1-hour expiry, and is audience-scoped to `"openshift"`.
Cloud providers validate the token against the cluster's OIDC issuer.
Even if the token were exfiltrated, it can only be used to assume roles that explicitly trust the `system:serviceaccount:openshift-adp:velero` subject in their trust policy.

## Compatibility

### Backward Compatibility

- Existing NonAdminBSLs with long-term credentials continue to work unchanged
- The `credentialMappings` field is optional; if absent, the existing credential flow is used
- No changes to the admin-level DPA STS flow

### Velero Compatibility

- Velero's per-BSL `spec.credential` field ([`persistence/object_store.go:170-178`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/persistence/object_store.go#L170-L178)) materializes secrets into temp files and passes the path as `config["credentialsFile"]` to plugins — this mechanism works correctly for all providers
- **AWS:** Per-BSL STS credential files verified working via [`repository/config/aws.go:80-94`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/repository/config/aws.go#L80-L94)
- **GCP:** Per-BSL WIF credential files verified working via [`repository/config/gcp.go:41-45`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/repository/config/gcp.go#L41-L45)
- **Azure:** Per-BSL WI credential files verified **broken** — `NewCredential()` at [`util/azure/credential.go:49`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go#L49) reads `AZURE_FEDERATED_TOKEN_FILE` from env var, ignoring the per-BSL `creds` map. Requires [upstream fix](#upstream-prerequisites-azure-workload-identity-fix)

### Cloud Provider Compatibility

| Provider | Feasibility | Verified | Plugin Change Required | Notes |
|----------|------------|----------|----------------------|-------|
| AWS STS | **High** | ✅ Code-verified | None | Per-BSL INI profile with different `role_arn` + shared `web_identity_token_file`. AWS SDK v2 shared config loader handles this natively. Requires `enableSharedConfig: "true"` on BSL. |
| GCP WIF | **High** | ✅ Code-verified | None | Per-BSL `external_account` JSON with different `service_account_impersonation_url` + shared `credential_source.file` (absolute path). Google Auth library handles this natively. |
| Azure WI | **High** (after fix) | ❌ Broken today | [Upstream fix needed](#upstream-prerequisites-azure-workload-identity-fix) | `NewCredential()` ignores per-BSL `creds` map for WI auth, uses only env vars. Fix is ~15 lines, backward-compatible, benefits all Velero users. |

## Implementation

### Phase 0: Upstream Velero Azure Fix (prerequisite, can be parallelized)

1. **Upstream PR to `vmware-tanzu/velero`** — Fix [`pkg/util/azure/credential.go`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go#L48-L54) `NewCredential()` to read WI parameters from the `creds` map with env var fallback (see [Upstream Prerequisites](#upstream-prerequisites-azure-workload-identity-fix))
2. **Upstream unit test** — Add test case to [`credential_test.go`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential_test.go) verifying per-BSL WI credential resolution from `creds` map
3. **Backport/vendor** — Once merged upstream, update Velero dependency in OADP

### Phase 1: Core OADP Infrastructure

1. **DPA API extension** — Add `CredentialMappings` to `NonAdmin` struct in [`api/v1alpha1/dataprotectionapplication_types.go`](https://github.com/openshift/oadp-operator/blob/f2f8c5ed3c610dbede4a087347d6bc776038ca8f/api/v1alpha1/dataprotectionapplication_types.go)
2. **CRD regeneration** — `make generate && make manifests && make bundle`
3. **DPA validation** — Validate credential mappings in [`internal/controller/validator.go`](https://github.com/openshift/oadp-operator/blob/f2f8c5ed3c610dbede4a087347d6bc776038ca8f/internal/controller/validator.go)
4. **Non-admin controller update** ([`migtools/oadp-non-admin`](https://github.com/migtools/oadp-non-admin)) — Resolve per-namespace credentials when creating Velero BSLs, set `enableSharedConfig` for AWS, enforce prefix

### Phase 2: Cloud Provider E2E Testing

1. **AWS STS e2e test** — NonAdminBSL with per-namespace IAM role, verify backup/restore, verify cross-namespace isolation
2. **GCP WIF e2e test** — NonAdminBSL with per-namespace GCP SA impersonation, verify backup/restore, verify cross-namespace isolation
3. **Azure WI e2e test** — NonAdminBSL with per-namespace managed identity (requires Phase 0 fix), verify backup/restore, verify cross-namespace isolation

### Phase 3: Documentation and Tooling

1. **Admin guide** — Step-by-step for each cloud provider (IAM role/SA/identity creation, credential secret creation, DPA configuration)
2. **Automation scripts** — Helper scripts for creating per-namespace cloud identities (similar to `ccoctl`)
3. **Sample DPA configurations** — Examples for each cloud provider

## Open Issues

1. **Credential mapping granularity:** Should credential mappings be per-namespace or per-NonAdminBSL? Per-namespace is simpler for admins but limits flexibility (one cloud identity per namespace). Per-NonAdminBSL would allow a namespace to have BSLs in different cloud providers but adds complexity.

2. **Auto-discovery vs explicit mapping:** Should the operator discover per-namespace credentials by label on secrets in the OADP namespace, or should they be explicitly listed in the DPA? Label-based discovery is more dynamic but harder to audit. Explicit mapping is more visible but requires DPA updates when adding namespaces.

3. **Non-admin controller repository:** The non-admin controller lives in `migtools/oadp-non-admin`, a separate repository. Changes to credential resolution must be coordinated between the two repositories. The DPA reconciler passes configuration to the non-admin controller deployment, but the credential mapping logic itself runs in the non-admin controller.

4. **Scaling limits:** Azure has a limit of 20 federated identity credentials per managed identity (one FIC per managed identity in this design, so not a concern). AWS has no hard limit on IAM roles but has a default limit of 1000 roles per account. GCP has a limit of 100 service accounts per project. For large clusters with many non-admin namespaces, these limits should be documented.

5. **Azure BSL `useAAD` requirement:** For Azure WI with per-BSL credentials, the BSL config must include `useAAD: "true"` to prevent the Azure plugin from attempting to exchange storage account access keys (which requires `resourceGroup` and management plane permissions). The non-admin controller should automatically set this when a WI credential mapping is configured for Azure.

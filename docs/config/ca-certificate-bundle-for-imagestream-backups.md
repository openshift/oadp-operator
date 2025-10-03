# CA Certificate Bundle for ImageStream Backups

## TLDR

**What**: OADP automatically mounts custom CA certificates from BackupStorageLocations into Velero to enable ImageStream backups with self-signed or internal certificates.

**When to use**: Only needed for OpenShift ImageStream backups in environments with custom CAs. Regular Velero backups don't require this.

**How to enable**: Set `spec.backupImages: true` (default) and configure `caCert` in your BSL. See [Configuration Examples](#configuration-examples).

**How to disable**: Set `spec.backupImages: false` to skip CA mounting. See [Disabling](#disabling-imagestream-backup-ca-handling).

**Key components**:

- **ConfigMap**: `velero-ca-bundle` contains concatenated CA certificates
- **Environment variable**: `AWS_CA_BUNDLE=/etc/velero/ca-certs/ca-bundle.pem`
- **Control field**: `spec.backupImages` in DataProtectionApplication CR

**Quick facts**:

- Certificate updates sync within 1-2 minutes (kubelet sync period)
- Changing `backupImages` setting restarts Velero pod
- Only collects from AWS provider BSLs currently
- Works with S3-compatible storage (MinIO, NooBaa, Ceph RGW)

**Jump to**:

- [Key Concepts](#key-concepts) - Understand how it works
- [Configuration Examples](#configuration-examples) - Quick setup
- [Troubleshooting](#troubleshooting) - Fix common issues

## Overview

OADP automatically collects CA certificates from BackupStorageLocations (BSLs) and mounts them into the Velero deployment to enable ImageStream backups in environments with custom Certificate Authorities.

See [ImageStream Backup Scope](#imagestream-backup-scope) in Key Concepts to understand why this is needed only for ImageStream backups and not for regular Velero operations.

Configuration is controlled via the `spec.backupImages` field - see [backupImages Control Field](#backupimages-control-field) for behavior details and [Disabling ImageStream Backup CA Handling](#disabling-imagestream-backup-ca-handling) for how to turn it off.

## Key Concepts

This section defines core concepts referenced throughout the document.

### ImageStream Backup Scope

This CA certificate mounting feature is **exclusively for OpenShift ImageStream backups**.

**ImageStream backups require special handling** because:

- They delegate to openshift-velero-plugin
- The plugin uses docker-distribution S3 driver for image layer copying
- The S3 driver can only read CA certificates from the `AWS_CA_BUNDLE` environment variable
- It cannot access Velero's BSL `caCert` configuration directly

**Regular Velero backups (pods, PVCs, namespaces, etc.)** do NOT need this feature:

- Velero directly uses the `caCert` field from BackupStorageLocation spec
- CA certificate validation happens within Velero's own code
- No environment variable-based CA handling needed

### Two CA Certificate Mechanisms

OADP/Velero supports CA certificates through **two independent mechanisms**:

#### BSL `caCert` Field (Native Velero mechanism)

- Configured in BackupStorageLocation spec: `spec.objectStorage.caCert`
- Base64-encoded CA certificate bundle
- Velero passes this directly to plugins for S3 API operations
- Works for velero-plugin-for-aws and regular Velero backups
- **Always available**, regardless of `backupImages` setting
- Does NOT require `AWS_CA_BUNDLE` environment variable

#### `AWS_CA_BUNDLE` Environment Variable (AWS SDK mechanism)

- Set by OADP when `backupImages: true` (or nil, defaults to true)
- Points to mounted file: `/etc/velero/ca-certs/ca-bundle.pem`
- Read by AWS SDK at session creation time
- **Required for imagestream backups** (docker-distribution S3 driver)
- **Overrides BSL `caCert`** for velero-plugin-for-aws when both are present
- **Not set** when `backupImages: false`

#### Component Behavior Summary

| Component | `backupImages: true` | `backupImages: false` |
|-----------|---------------------|----------------------|
| **velero-plugin-for-aws** | Uses `AWS_CA_BUNDLE` (overrides BSL `caCert`) | Uses ONLY BSL `caCert` field |
| **ImageStream backups** | ✅ Works (requires `AWS_CA_BUNDLE`) | ❌ Fails with custom CAs |
| **Velero BSL validation** | Uses `AWS_CA_BUNDLE` (overrides BSL `caCert`) via velero-plugin-for-aws | Uses BSL `caCert` via velero-plugin-for-aws |

**Why both mechanisms exist**:

The BSL `caCert` field is a **Velero BackupStorageLocation spec field**, but it's not an **S3 storage driver parameter**. Here's the critical distinction:

- **Velero BSL spec**: Contains fields like `caCert`, `bucket`, `region`, etc.
- **S3 storage driver parameters**: The subset of configuration passed to the **docker-distribution S3 driver** (in openshift/docker-distribution fork), includes: bucket, credentials, region, endpoint
  - **Not to be confused with**: velero-plugin-for-aws, which uses AWS SDK directly (not docker-distribution)
  - **Only for ImageStream backups**: docker-distribution S3 driver is used by openshift-velero-plugin for copying image layers
- **docker-distribution S3 driver does NOT have a `caCert` parameter** - it has no way to receive CA certificates via configuration

When openshift-velero-plugin calls the docker-distribution S3 driver:
1. It passes S3 driver parameters (bucket, region, credentials) extracted from BSL
2. The S3 driver creates an AWS SDK session using these parameters
3. The AWS SDK reads `AWS_CA_BUNDLE` from the **process environment** (not from driver parameters)
4. There's no path to pass BSL `caCert` to the S3 driver - it must come from environment

When `AWS_CA_BUNDLE` is set in the Velero pod environment, the AWS SDK reads it at session creation and uses it for **all** AWS SDK operations, including:
- ImageStream backups (via docker-distribution S3 driver)
- BSL validation (via velero-plugin-for-aws)
- Regular Velero backups (via velero-plugin-for-aws)

This is why `AWS_CA_BUNDLE` **overrides** BSL `caCert` for velero-plugin-for-aws when both are present.

### backupImages Control Field

The `spec.backupImages` field in DataProtectionApplication CR controls CA certificate mounting:

**When `true` (default)**:

- CA certificates collected from AWS BSLs
- ConfigMap `velero-ca-bundle` created
- Volume mounted at `/etc/velero/ca-certs`
- `AWS_CA_BUNDLE` environment variable set
- ImageStream backups work with custom CAs

**When `false`**:

- No CA certificate processing
- No ConfigMap created
- No volume mount added
- No `AWS_CA_BUNDLE` set
- ImageStream backups fail with custom CAs (only work with public CAs)

**Default behavior**: When not specified, defaults to `true` via the `BackupImages()` method.

See [Disabling ImageStream Backup CA Handling](#disabling-imagestream-backup-ca-handling) for detailed configuration.

### ConfigMap Sync Timing

Based on [Kubernetes documentation](https://kubernetes.io/docs/concepts/configuration/configmap/) and [issue #20200](https://github.com/kubernetes/kubernetes/issues/20200):

**Update timing**:

- **ConfigMap update**: Instant (via `controllerutil.CreateOrPatch`)
- **File sync to pod**: 1-2 minutes (kubelet sync period + cache TTL)
  - Kubelet sync period: 1 minute (default)
  - Kubelet ConfigMap cache TTL: 1 minute (default)
  - **Total maximum delay**: Up to 2 minutes
  - **Typical delay**: 60-90 seconds

**Important behavior**:

- ConfigMap updates do NOT restart pods automatically
- Environment variables (like `AWS_CA_BUNDLE`) are NOT updated automatically
- The `AWS_CA_BUNDLE` points to a file path - the file content is updated by kubelet
- Applications must detect and reload configuration changes

**Implications for certificate updates**:

- New AWS SDK sessions (for new backup operations) use the updated certificate file
- Existing AWS SDK sessions continue using old certificates until session recreated
- **Practical effect**: Certificate updates available for new backups after kubelet sync period

### Pod Restart Triggers

**Velero pod WILL restart when**:

- `backupImages` changed from `false` to `true` (volume mount added)
- `backupImages` changed from `true` to `false` (volume mount removed)
- First CA certificate is added (volume mount added to deployment)
- Last CA certificate is removed (volume mount removed from deployment)
- `AWS_CA_BUNDLE` environment variable is added/removed

**Velero pod will NOT restart when**:

- CA certificate content is updated in existing BSL
- ConfigMap data is modified (only file content changes)
- `backupImages` remains unchanged

**Impact on running backups**:

- During ConfigMap update (no restart): Running backups may complete, new backups use updated certs
- During pod restart: Running backups **will fail**, Velero marks as `PartiallyFailed`
- **Recommendation**: Avoid changing `backupImages` or adding/removing CA certificates during active backups. For Non-DPA BSL discovery, use safe trigger mechanisms instead - see [Triggering Discovery of Non-DPA BSL Changes](#triggering-discovery-of-non-dpa-bsl-changes)

### AWS SDK Session Behavior

The AWS SDK and Docker Distribution S3 driver read CA certificates at **session creation time only**:

- Once an AWS SDK session is created, it does NOT automatically reload certificates from disk
- New sessions (for new backup operations) read from the current certificate file
- Each imagestream backup operation typically creates new SDK sessions
- This means certificate updates become effective for new backup operations after the kubelet sync period

### Certificate Collection Scope

**Currently collected from**:

- Only AWS provider BackupStorageLocations
- BSLs defined in DPA `spec.backupLocations` (OADP-managed)
- Additional BSLs in the same namespace (external/non-OADP BSLs)
- System default CA certificates (appended for fallback)

**How external BSLs are discovered**:

**For CA certificate collection** (`internal/controller/bsl.go:processCACertForBSLs`):
- Lists **all** BSLs in namespace: `r.List(r.Context, allBSLs, client.InNamespace(dpa.Namespace))`
- **No label filtering** - discovers both OADP-managed and external BSLs
- Filters out BSLs already processed from DPA spec by name
- Only collects from AWS provider BSLs

**For ImageStream backup support** (`internal/controller/registry.go:545-553`):
- Lists BSLs **with label filter**: `app.kubernetes.io/component: bsl`
- Creates registry secrets only for labeled BSLs (required by [openshift-velero-plugin](https://github.com/openshift/openshift-velero-plugin/blob/64292f953c3e2ecd623e9388b2a65c08bb9cfbe2/velero-plugins/imagestream/shared.go#L70-L73))

**Using external BSLs for ImageStream backups**:

External BSLs (created outside DPA spec) CAN be used for ImageStream backups if you:
1. Manually add the required label: `app.kubernetes.io/component: bsl`
2. Ensure the BSL has AWS provider and `caCert` configured
3. The OADP registry controller will then create the necessary registry secret

**OADP-managed BSL labels** (automatically applied):
- `app.kubernetes.io/name: oadp-operator-velero`
- `app.kubernetes.io/managed-by: oadp-operator`
- `app.kubernetes.io/component: bsl` ← **Required for registry secret creation**

**Not collected from**:

- Non-AWS provider BSLs (Azure, GCP, etc.)
- BSLs in different namespaces
- Manually created certificate files

**Why only AWS**: While the underlying [udistribution](https://github.com/migtools/udistribution) library supports multiple cloud storage drivers (Azure, GCS, Swift, OSS), OADP currently only implements CA certificate collection from AWS BSLs. Other providers may require provider-specific CA configuration.

## Why ImageStream Backups Need Special CA Handling

### Component Relationship and Flow

ImageStream backups involve a chain of components that work together to copy container image layers to backup storage:

```doc
┌─────────────────────────────────────────────────────────────────┐
│                  Component Relationship                          │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  1. Velero (vmware-tanzu/velero)                                 │
│     - Orchestrates all backup operations                         │
│     - Calls registered plugins for resource-specific handling    │
│     - Provides BSL configuration to plugins via API              │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│  2. openshift-velero-plugin                                      │
│     (github.com/openshift/openshift-velero-plugin)               │
│                                                                   │
│     - OpenShift-specific Velero plugin                           │
│     - Registers backup/restore actions for ImageStream resources│
│     - Source: velero-plugins/imagestream/shared.go:57            │
│     - Uses udistribution library to access storage drivers       │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│  3. udistribution (github.com/migtools/udistribution)            │
│                                                                   │
│     - Go library for programmatic registry storage access        │
│     - Modifies and wraps distribution/distribution library       │
│     - Uses openshift/docker-distribution as dependency           │
│       (via go.mod replace directive)                             │
│     - Provides client interface to storage drivers WITHOUT       │
│       requiring a running HTTP server                            │
│     - Supports multiple storage backends:                        │
│       • S3 (AWS, MinIO, Ceph RGW)                                │
│       • Azure Blob Storage                                       │
│       • Google Cloud Storage                                     │
│       • Swift (OpenStack)                                        │
│       • Alibaba Cloud OSS                                        │
│     - Allows direct programmatic calls to storage operations     │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│  4. openshift/docker-distribution S3 Driver                      │
│     (github.com/openshift/docker-distribution)                   │
│                                                                   │
│     - OpenShift fork of distribution/distribution                │
│     - Container image distribution library                       │
│     - Uses AWS SDK Go v1 (github.com/aws/aws-sdk-go v1.43.16)   │
│     - S3 storage driver: registry/storage/driver/s3-aws/s3.go    │
│     - Creates AWS SDK sessions via session.NewSessionWithOptions │
│     - AWS SDK automatically reads AWS_CA_BUNDLE env variable     │
│       during session initialization (built-in SDK behavior)      │
│     - Configures custom CA certificates for TLS verification     │
│     - Performs actual image layer upload/download operations     │
└─────────────────────────────────────────────────────────────────┘
```

### The ImageStream Backup Flow

```doc
┌─────────────────────────────────────────────────────────────────┐
│                  ImageStream Backup Flow                         │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│  1. Velero calls openshift-velero-plugin for ImageStream backup │
│     Source: openshift-velero-plugin/velero-plugins/imagestream  │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│  2. Plugin uses udistribution to access storage driver           │
│     - udistribution provides programmatic interface              │
│     - No HTTP server needed for storage operations               │
│     - Initializes appropriate storage driver based on config     │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│  3. Docker Distribution S3 Driver handles storage operations     │
│     Source: openshift/docker-distribution/registry/storage/     │
│             driver/s3-aws/s3.go                                  │
│                                                                   │
│     Key behavior:                                                │
│     - Reads AWS_CA_BUNDLE environment variable                   │
│     - Creates AWS SDK session with custom CA bundle              │
│     - Uses for all S3 copy operations                            │
│     - CANNOT access Velero's BSL caCert configuration            │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│  4. AWS SDK performs image layer copies to S3                    │
│     - Copies container image layers to S3 backup location        │
│     - Uses custom CA for TLS verification with S3 endpoints      │
│     - Requires valid CA chain for HTTPS connections              │
└─────────────────────────────────────────────────────────────────┘
```

### Code References

#### 1. OpenShift Velero Plugin - ImageStream Backup

- **Backup**: [`openshift-velero-plugin/velero-plugins/imagestream/backup.go`](https://github.com/openshift/openshift-velero-plugin/blob/master/velero-plugins/imagestream/backup.go)
  - Calls `GetUdistributionTransportForLocation()` to create udistribution transport
  - Passes transport to `imagecopy.CopyLocalImageStreamImages()` for image copying
- **Shared Code**: [`openshift-velero-plugin/velero-plugins/imagestream/shared.go`](https://github.com/openshift/openshift-velero-plugin/blob/master/velero-plugins/imagestream/shared.go)
  - `GetRegistryEnvsForLocation()` retrieves **S3 storage driver parameters** from BSL and converts to env var strings
  - Storage driver parameters include: credentials, bucket, region, endpoint, etc.
  - `GetUdistributionTransportForLocation()` calls `udistribution.NewTransportFromNewConfig(config, envs)`
  - **Key distinction**: BSL has `caCert` field (Velero spec), but this is NOT an S3 driver parameter
  - **`AWS_CA_BUNDLE`** comes from Velero pod's environment (set by OADP controller), not from BSL storage config

#### 2. udistribution Client Library

- **Transport Creation**: [`migtools/udistribution/pkg/image/udistribution/docker_transport.go`](https://github.com/migtools/udistribution/blob/main/pkg/image/udistribution/docker_transport.go)
  - `NewTransportFromNewConfig(config, envs)` creates transport with client
  - Calls `client.NewClient(config, envs)` to initialize
- **Client Initialization**: [`migtools/udistribution/pkg/client/client.go`](https://github.com/migtools/udistribution/blob/main/pkg/client/client.go)
  - `NewClient(config, envs)` parses configuration using `uconfiguration.ParseEnvironment(config, envs)`
  - Creates `handlers.App` which initializes storage drivers
  - **Key point**: Environment variables in `envs` parameter are **S3 storage driver parameters only**
  - S3 driver parameters do NOT include CA certificates - the S3 driver has no `caCert` parameter
  - `AWS_CA_BUNDLE` must already exist in the **process environment** from Velero pod
- **Purpose**: Wraps distribution/distribution to provide programmatic storage driver access without HTTP server

#### 3. Docker Distribution S3 Driver

- **S3 Driver**: [`openshift/docker-distribution/registry/storage/driver/s3-aws/s3.go:559`](https://github.com/openshift/docker-distribution/blob/release-4.19/registry/storage/driver/s3-aws/s3.go#L559)
  - Creates AWS SDK session via `session.NewSessionWithOptions(sessionOptions)`
  - AWS SDK v1 (`github.com/aws/aws-sdk-go v1.43.16`) automatically reads environment variables during session initialization
  - The S3 driver itself does NOT directly read `AWS_CA_BUNDLE` - this is handled by the AWS SDK
- **Session Creation**: AWS SDK's built-in environment variable loading includes `AWS_CA_BUNDLE`

#### 4. AWS SDK v1 Environment Configuration

- **Session Package**: [`aws-sdk-go/aws/session/env_config.go`](https://github.com/aws/aws-sdk-go/blob/main/aws/session/env_config.go)
  - `NewSessionWithOptions()` automatically loads configuration from **process environment variables** (via `os.Getenv`)
  - Reads `AWS_CA_BUNDLE` environment variable during session initialization
  - Loads custom CA certificates for TLS validation
  - Sets the CA bundle as the HTTP client's custom root CA
  - **Quote**: "Sets the path to a custom Credentials Authority (CA) Bundle PEM file that the SDK will use instead of the system's root CA bundle"
  - **Critical**: AWS SDK reads from process environment, NOT from configuration passed to storage driver

#### 5. OADP Controller Implementation

- Location: `internal/controller/velero.go:443`
- Controls when CA certificate processing occurs based on `dpa.BackupImages()`
- Calls `processCACertificatesForVelero()` only when imagestream backups are enabled
- Mounts CA bundle as file and sets `AWS_CA_BUNDLE` environment variable pointing to it

### Why Different from Regular Velero Backups

ImageStream backups require this special CA handling while regular Velero backups do not. See [ImageStream Backup Scope](#imagestream-backup-scope) and [Two CA Certificate Mechanisms](#two-ca-certificate-mechanisms) in Key Concepts for the detailed explanation of why docker-distribution cannot access Velero's BSL `caCert` configuration.

## Implementation Details

### Certificate Collection and Mounting

The implementation is in `internal/controller/`:

#### 1. Certificate Collection (`bsl.go:908-1124`)

```go
func (r *DataProtectionApplicationReconciler) processCACertForBSLs() (string, error)
```

**Collection Strategy**:

- Only collects from **AWS provider BSLs** (imagestream backup uses S3)
- Scans DPA `spec.backupLocations` for CA certificates
- Scans additional BSLs in namespace (not in DPA spec)
- Includes system default CA certificates for fallback
- Validates PEM format and deduplicates certificates

**Output**: ConfigMap `velero-ca-bundle` with concatenated certificates

#### 2. Velero Deployment Configuration (`velero.go:854-916`)

```go
func (r *DataProtectionApplicationReconciler) processCACertificatesForVelero(
    veleroDeployment *appsv1.Deployment,
    veleroContainer *corev1.Container,
) error
```

**Deployment Configuration**:

```go
// Volume mount
Volume{
    Name: "ca-certificate-bundle",
    VolumeSource: corev1.VolumeSource{
        ConfigMap: &corev1.ConfigMapVolumeSource{
            LocalObjectReference: corev1.LocalObjectReference{
                Name: "velero-ca-bundle",
            },
        },
    },
}

VolumeMount{
    Name:      "ca-certificate-bundle",
    MountPath: "/etc/velero/ca-certs",
    ReadOnly:  true,
}

// Environment variable for AWS SDK
EnvVar{
    Name:  "AWS_CA_BUNDLE",
    Value: "/etc/velero/ca-certs/ca-bundle.pem",
}
```

### When CA Bundle is Created

The CA bundle ConfigMap and volume mount are created based on the `spec.backupImages` field and presence of CA certificates in AWS BSLs:

**Creation conditions**:

1. `spec.backupImages` is `true` or `nil` (defaults to true)
2. At least one AWS provider BSL has `caCert` configured

**What gets created**: See [Certificate Collection Scope](#certificate-collection-scope) for details on what certificates are collected.

**Disabling**: When `backupImages: false`, no CA processing occurs. See [backupImages Control Field](#backupimages-control-field) and [Disabling ImageStream Backup CA Handling](#disabling-imagestream-backup-ca-handling) for complete behavior details.

### E2E Test Validation

From `tests/e2e/backup_restore_suite_test.go`:

**When `backupImages=true`** (line 638-649):

```go
// Verify AWS_CA_BUNDLE is set when backing up images
awsCABundleFound := false
for _, env := range veleroContainer.Env {
    if env.Name == "AWS_CA_BUNDLE" {
        awsCABundleFound = true
        awsCABundlePath := env.Value
        log.Printf("Found AWS_CA_BUNDLE environment variable: %s", awsCABundlePath)
    }
}
gomega.Expect(awsCABundleFound).To(gomega.BeTrue(),
    "AWS_CA_BUNDLE environment variable should be set when backupImages=true")
```

**When `backupImages=false`** (line 606-615):

```go
// Verify AWS_CA_BUNDLE is NOT set when NOT backing up images
awsCABundleFound := false
for _, env := range veleroContainer.Env {
    if env.Name == "AWS_CA_BUNDLE" {
        awsCABundleFound = true
        log.Printf("ERROR: Found unexpected AWS_CA_BUNDLE environment variable: %s", env.Value)
    }
}
gomega.Expect(awsCABundleFound).To(gomega.BeFalse(),
    "AWS_CA_BUNDLE environment variable should NOT be set when backupImages=false")
```

## Disabling ImageStream Backup CA Handling

### When to Disable

Consider disabling CA certificate handling (`backupImages: false`) when:

1. **No ImageStream Backups Required**: Your cluster doesn't use imagestreams or you don't need to back them up
2. **Public CA Certificates Only**: Your S3 endpoints use certificates from trusted public CAs
3. **Resource Optimization**: Reduce unnecessary ConfigMap creation and volume mounts
4. **Simplified Configuration**: Avoid CA certificate management overhead

### How to Disable

Set `spec.backupImages` to `false` in the DataProtectionApplication CR:

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: oadp-dpa
spec:
  backupImages: false  # Disable CA certificate mounting for imagestream backups
  configuration:
    velero:
      defaultPlugins:
        - aws
    nodeAgent:
      enable: true
  backupLocations:
    - name: default
      velero:
        provider: aws
        default: true
        objectStorage:
          bucket: my-backup-bucket
          # caCert field can still be specified for Velero's native CA handling
          # but will NOT be mounted or used for AWS_CA_BUNDLE
          caCert: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUQuLi4KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
        config:
          region: us-east-1
```

### API Definition

**Type Definition** (`api/v1alpha1/dataprotectionapplication_types.go:803-805`):

```go
// backupImages is used to specify whether you want to deploy a registry for enabling backup and restore of images
// +optional
BackupImages *bool `json:"backupImages,omitempty"`
```

**CRD Schema** (`config/crd/bases/oadp.openshift.io_dataprotectionapplications.yaml:56-58`):

```yaml
backupImages:
  description: backupImages is used to specify whether you want to deploy a registry for enabling backup and restore of images
  type: boolean
```

### Behavior When Disabled

With `backupImages: false`:

1. **No CA Certificate Processing**:
   - `processCACertForBSLs()` is not called
   - ConfigMap `velero-ca-bundle` is not created
   - No certificate collection or validation occurs

2. **Velero Deployment**:
   - No `ca-certificate-bundle` volume added
   - No volume mount at `/etc/velero/ca-certs`
   - No `AWS_CA_BUNDLE` environment variable set

3. **Regular Velero Backups**:
   - Continue to work normally
   - Use BSL `caCert` field for TLS validation
   - No impact on pod/PVC/namespace backups

4. **ImageStream Backups**:
   - Will fail if using custom CA certificates
   - Only work if S3 endpoints use public CA certificates
   - Error: `x509: certificate signed by unknown authority`

### Default Behavior

**When `backupImages` is not specified** (nil):

- Defaults to `true` via the `BackupImages()` method
- CA certificate processing is enabled
- ConfigMap and volume mount are created if CA certificates exist in BSLs

## Certificate Rotation and Updates

### ConfigMap Update Behavior

See [ConfigMap Sync Timing](#configmap-sync-timing) and [AWS SDK Session Behavior](#aws-sdk-session-behavior) in Key Concepts for how certificate updates propagate.

**Quick summary**:

- ConfigMap updates don't restart pods
- Files sync to pods within 1-2 minutes (kubelet sync period)
- New backup operations pick up updated certificates after sync completes
- Existing SDK sessions continue using old certificates until recreated

### Update Flow in OADP

#### When ConfigMap Updates Occur

The OADP controller updates the `velero-ca-bundle` ConfigMap in response to several triggers:

**1. DPA Spec Changes**:

- User modifies `spec.backupLocations[*].velero.objectStorage.caCert`
- User modifies `spec.backupLocations[*].cloudStorage.caCert`
- User adds/removes backup locations with CA certificates
- Controller watches DPA resource via `For(&oadpv1alpha1.DataProtectionApplication{})`

**2. BSL Resource Changes**:

- OADP-managed BSLs are created/updated via `controllerutil.CreateOrPatch`
- Controller owns BSL resources via `Owns(&velerov1.BackupStorageLocation{})`
- Any changes to owned BSLs trigger DPA reconciliation (via controller ownership)
- Only owned BSLs (with `oadp.openshift.io/oadp: "True"` label) trigger reconciliation automatically
- BSLs created outside of OADP (in same namespace) are scanned during reconciliation but don't trigger it
- Non-OADP BSLs are discovered via `r.List()` call in `processCACertForBSLs()` during each reconciliation

**3. Secret Label Changes**:

- Controller watches Secrets via `Watches(&corev1.Secret{}, &labelHandler{})`
- Secrets with labels `openshift.io/oadp: "True"` and `dataprotectionapplication.name: <dpa-name>` trigger reconciliation
- BSL credential secrets are automatically labeled by `UpdateCredentialsSecretLabels()` (bsl.go:371-407)
- This enables detection of credential updates that might affect BSL configuration

**4. ConfigMap Lifecycle**:

- ConfigMap has controller reference to DPA: `controllerutil.SetControllerReference(dpa, configMap, r.Scheme)`
- Controller owns ConfigMaps via `Owns(&corev1.ConfigMap{})`
- ConfigMap updates use `controllerutil.CreateOrPatch` for idempotent updates
- Only updates when certificate content actually changes (prevents unnecessary pod disruptions)

#### Complete Update Flow

```doc
Trigger Event (DPA change, BSL update, or Secret label change)
         │
         ↓
DPA Controller Reconciliation Loop Starts
         │
         ↓
ReconcileBackupStorageLocations() executes (line 98 in controller)
  │
  ├─ Creates/updates BSL resources from DPA spec
  ├─ Labels BSL secrets to enable watching
  └─ Sets controller references for ownership
         │
         ↓
ReconcileVeleroDeployment() executes (line 107 in controller)
         │
         ↓
Check dpa.BackupImages() == true (velero.go:443)
         │
         ↓
processCACertForBSLs() Collects Certificates (bsl.go:908-1124)
  │
  ├─ Scans DPA spec.backupLocations for AWS BSL CA certs
  ├─ Lists all BSLs in namespace (includes non-DPA BSLs)
  ├─ Collects only from AWS provider BSLs
  ├─ Validates PEM format for each certificate
  ├─ Deduplicates certificates (unique cert tracking)
  ├─ Appends system default CA certificates
  └─ Returns ConfigMap name or empty string
         │
         ↓
ConfigMap "velero-ca-bundle" Created/Updated
  │
  ├─ Uses controllerutil.CreateOrPatch (idempotent)
  ├─ Data.ca-bundle.pem = concatenated certificates
  ├─ Sets controller reference to DPA
  ├─ Event recorded: "CACertificateConfigMapReconciled"
  └─ Only updates if content changed
         │
         ↓
processCACertificatesForVelero() Configures Deployment (velero.go:854-916)
  │
  ├─ Adds volume mount if ConfigMap exists
  ├─ Mounts at /etc/velero/ca-certs
  ├─ Sets AWS_CA_BUNDLE environment variable
  └─ Only modifies deployment if mount state changed
         │
         ↓
Velero Deployment Updated (if spec changed)
  │
  ├─ Pod restart ONLY if volume mount added/removed
  └─ No restart if only ConfigMap data changed
         │
         ↓
Kubelet Syncs Volume Contents (1-2 minutes)
         │
         ↓
File /etc/velero/ca-certs/ca-bundle.pem Updated in Pod
         │
         ↓
Next ImageStream Backup Creates New AWS SDK Session
         │
         ↓
New Session Reads Updated Certificate File
```

#### Reconciliation Timing and Behavior

**Immediate Triggers** (instant reconciliation):

1. **DPA Spec Modification**: Any change to DataProtectionApplication resource
   - Watched via `For(&oadpv1alpha1.DataProtectionApplication{})`
   - Direct reconciliation of the modified DPA

2. **Owned Resource Changes**: Resources with controller reference to DPA
   - BSLs created by OADP (via `Owns(&velerov1.BackupStorageLocation{})`)
   - ConfigMaps (via `Owns(&corev1.ConfigMap{})`)
   - Deployments, Services, etc.
   - Trigger reconciliation of owner DPA
   - Predicate filter: Only if generation changed (spec modification) or has `openshift.io/oadp` label

3. **Labeled Secret Changes**: Secrets with OADP labels
   - Watched via `Watches(&corev1.Secret{}, &labelHandler{})`
   - Must have labels: `openshift.io/oadp: "True"` AND `dataprotectionapplication.name: <dpa-name>`
   - Create, Update, Delete, or Generic events all trigger reconciliation
   - Used for BSL credential secret updates

**Eventual Consistency**:

1. **ConfigMap Content Updates**: Within seconds
   - `controllerutil.CreateOrPatch` is immediate
   - But file sync to pod takes 1-2 minutes (kubelet)

2. **File Sync to Pod**: 1-2 minutes
   - Kubelet sync period: 1 minute (default)
   - Kubelet ConfigMap cache TTL: 1 minute (default)
   - Total: up to 2 minutes for file content to appear in pod

3. **New Backup Operations**: Immediately after file sync
   - Next AWS SDK session creation reads updated certificate file
   - Each backup operation typically creates new SDK sessions

**No Automatic Trigger** (only detected during next scheduled reconciliation):

1. **Manual BSL Creation Outside DPA**: Not watched directly
   - BSLs without controller reference to DPA
   - BSLs without `openshift.io/oadp` label
   - Only discovered when reconciliation runs for other reasons
   - Scanned via `r.List()` in `processCACertForBSLs()`

2. **Direct ConfigMap Edits**: Overwritten on next reconciliation
   - DPA reconciliation regenerates ConfigMap content
   - DPA is the source of truth for CA certificate bundle

3. **Certificate File Changes**: Not supported
   - Changes directly to files on disk (bypassing ConfigMap)
   - Not detected or monitored

**Predicate Filtering** (from `predicate.go`):

The controller uses `veleroPredicate()` to filter events:

- **Update events**: Only trigger if `generation` changed (spec modification)
- **Create events**: Trigger if resource has `openshift.io/oadp` label or is DPA
- **Delete events**: Trigger if resource has `openshift.io/oadp` label or is DPA
- This prevents status-only updates from triggering unnecessary reconciliations

**Typical Reconciliation Scenarios**:

1. **User edits DPA CA cert**: Instant → ConfigMap update → 1-2 min file sync → new backups use cert
2. **User adds new BSL with CA**: Instant (owned resource) → ConfigMap update → 1-2 min → effective
3. **User updates BSL credential secret**: Instant (if labeled) → full reconciliation → ConfigMap update
4. **User manually creates BSL with CA outside DPA**: No trigger → discovered at next DPA reconciliation
5. **Velero updates BSL status**: No trigger (generation unchanged, status-only update filtered by predicate)

### When Velero Pod Restarts

See [Pod Restart Triggers](#pod-restart-triggers) in Key Concepts for the complete list of conditions that cause pod restarts vs those that don't.

**Impact on running backups**: Pod restarts cause running imagestream backups to fail. ConfigMap-only updates allow running backups to complete while new backups use updated certificates after the kubelet sync period.

### Triggering Discovery of Non-DPA BSL Changes

Non-OADP BSLs (BackupStorageLocations created outside of DPA spec) are discovered via `r.List()` call in `processCACertForBSLs()` during each reconciliation. They do NOT automatically trigger reconciliation when modified.

**Safe trigger mechanisms** (ConfigMap-only update, no pod restart):

1. **DPA Annotation (Recommended)**:

   ```bash
   oc annotate dpa <dpa-name> -n openshift-adp reconcile=$(date +%s)
   ```

   - Triggers immediate reconciliation
   - Updates ConfigMap if certificates changed
   - Does NOT modify deployment spec
   - Does NOT restart Velero pod

2. **DPA Metadata Update (Alternative)**:

   ```bash
   oc patch dpa <dpa-name> -n openshift-adp --type=merge -p '{"metadata":{"labels":{"last-sync":"'$(date +%s)'"}}}'
   ```

   - Triggers reconciliation via metadata change
   - Safe - metadata changes don't affect deployment

**Unsafe mechanisms** (causes pod restart):

- ❌ Toggling `backupImages` setting
- ❌ Adding/removing DPA `spec.backupLocations` unnecessarily

**Future improvement**: OADP may implement a watch on all BSLs in the namespace (not just owned ones) to automatically detect Non-DPA BSL changes, eliminating the need for manual triggering. Currently, `Owns(&velerov1.BackupStorageLocation{})` only watches OADP-created BSLs.

## Configuration Examples

### Example 1: ImageStream Backups Enabled with Custom CA

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: oadp-dpa
spec:
  configuration:
    velero:
      defaultPlugins:
        - openshift  # Required for imagestream backups
        - aws
    nodeAgent:
      enable: true
  backupImages: true  # Enable imagestream backups (this is the default)
  backupLocations:
    - name: default
      velero:
        provider: aws
        default: true
        objectStorage:
          bucket: my-backup-bucket
          prefix: velero
          caCert: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURYVENDQWtXZ0F3SUJBZ0lKQUtKLi4uCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K
        config:
          region: us-east-1
          s3Url: https://s3-compatible.example.com
          s3ForcePathStyle: "true"
```

**Result**:

- ConfigMap `velero-ca-bundle` created with custom CA + system CAs
- Velero pod has volume mount at `/etc/velero/ca-certs`
- `AWS_CA_BUNDLE=/etc/velero/ca-certs/ca-bundle.pem` set
- ImageStream backup operations use custom CA for S3 TLS validation
- Regular Velero backups work normally using BSL `caCert` directly

### Example 2: ImageStream Backups Disabled

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: oadp-dpa
spec:
  configuration:
    velero:
      defaultPlugins:
        - openshift
        - aws
    nodeAgent:
      enable: true
  backupImages: false  # Explicitly disable imagestream backup CA handling
  backupLocations:
    - name: default
      velero:
        provider: aws
        default: true
        objectStorage:
          bucket: my-backup-bucket
          prefix: velero
          # caCert still used by Velero for its own S3 operations
          caCert: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURYVENDQWtXZ0F3SUJBZ0lKQUtKLi4uCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K
        config:
          region: us-east-1
          s3Url: https://s3-compatible.example.com
          s3ForcePathStyle: "true"
```

**Result**:

- ConfigMap `velero-ca-bundle` **NOT** created
- Velero pod **NO** volume mount at `/etc/velero/ca-certs`
- `AWS_CA_BUNDLE` environment variable **NOT** set
- Regular Velero backups work using BSL `caCert`
- ImageStream backups will fail if custom CA is required

### Example 3: Multiple AWS BSLs for Different Environments

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: oadp-dpa
spec:
  configuration:
    velero:
      defaultPlugins:
        - openshift
        - aws
  backupImages: true
  backupLocations:
    - name: production
      velero:
        provider: aws
        default: true
        objectStorage:
          bucket: prod-backups
          caCert: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUQuLi4gKFByb2R1Y3Rpb24gQ0EpCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K
        config:
          region: us-east-1
          s3Url: https://s3.prod.example.com

    - name: disaster-recovery
      velero:
        provider: aws
        objectStorage:
          bucket: dr-backups
          caCert: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUUuLi4gKERSIFNpdGUgQ0EgLSBkaWZmZXJlbnQgQ0EpCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K
        config:
          region: us-west-2
          s3Url: https://s3.dr.example.com
```

**Result**:

- ConfigMap contains: Production CA + DR CA + System CAs
- All certificates concatenated and deduplicated
- ImageStream backups to both locations work with their respective custom CAs

## Scope and Limitations

### What This Feature Enables

✅ **ImageStream backups** in environments with:

- Custom Certificate Authorities (internal CAs)
- Self-signed certificates on S3 endpoints
- MITM proxy infrastructure
- Air-gapped environments with internal CAs

✅ **Automatic certificate management**:

- Collection from all AWS BSLs
- Deduplication of certificates
- System CA fallback
- ConfigMap lifecycle management

✅ **Opt-out capability**:

- Disable via `spec.backupImages: false`
- Reduce overhead when imagestream backups not needed

### What This Feature Does NOT Cover

❌ **Primary design target is imagestream backups**: While `AWS_CA_BUNDLE` affects all AWS SDK usage, this feature was specifically designed for imagestream backup operations

❌ **Non-AWS provider CA collection**: See [Certificate Collection Scope](#certificate-collection-scope) - OADP currently only collects CA certificates from AWS BSLs

### How Components Use CA Certificates

See [Two CA Certificate Mechanisms](#two-ca-certificate-mechanisms) in Key Concepts for a complete explanation of how different components (velero-plugin-for-aws, imagestream backups, BSL validation) use CA certificates differently with `backupImages: true` vs `false`.

**Key points**:

- velero-plugin-for-aws: `AWS_CA_BUNDLE` overrides BSL `caCert` when both present (affects all AWS SDK operations)
- ImageStream backups: REQUIRE `AWS_CA_BUNDLE` environment variable
- Velero BSL validation: Uses velero-plugin-for-aws, so also affected by `AWS_CA_BUNDLE` override behavior

**When to disable** `backupImages: false`:

- No imagestream backups needed
- BSL `caCert` sufficient for regular Velero backups
- Reduce ConfigMap/volume mount overhead

**When to keep enabled** `backupImages: true` (default):

- Need imagestream backups with custom CAs
- Want redundant CA mechanisms
- Unsure and want maximum compatibility

### Provider Support

**Primary Implementation**:

- AWS (and S3-compatible providers like MinIO, NooBaa, Ceph RGW)
- Uses `AWS_CA_BUNDLE` environment variable for the S3 driver
- This is the most common and well-tested configuration

**Additional Cloud Provider Support**:

The underlying [udistribution](https://github.com/migtools/udistribution) library used for imagestream backups supports multiple cloud storage drivers:

- **Azure Blob Storage**: Uses Azure storage driver
- **Google Cloud Storage (GCS)**: Uses GCS storage driver
- **OpenStack Swift**: Uses Swift storage driver
- **Alibaba OSS**: Uses OSS storage driver

**Implementation Notes**:

- ImageStream backups can work with multiple cloud providers through docker-distribution drivers
- Each driver may have its own CA certificate configuration mechanism
- `AWS_CA_BUNDLE` specifically targets the S3-AWS driver
- Other providers may require provider-specific CA configuration
- OADP currently collects and mounts CA certificates primarily for AWS BSLs

## Troubleshooting

### Verify CA Bundle for ImageStream Backups

```bash
# Check if backupImages is enabled
oc get dpa -n openshift-adp -o jsonpath='{.items[0].spec.backupImages}'
# Output: true (or empty, which defaults to true)

# Verify ConfigMap exists (only if CA certs configured AND backupImages=true)
oc get configmap velero-ca-bundle -n openshift-adp

# Check Velero deployment has AWS_CA_BUNDLE
oc get deployment velero -n openshift-adp -o yaml | grep AWS_CA_BUNDLE

# Verify certificate file in pod
oc exec -n openshift-adp deployment/velero -- cat /etc/velero/ca-certs/ca-bundle.pem

# Test imagestream backup
velero backup create test-imagestream-backup --include-resources imagestreams
```

### Common Issues

#### Issue: ImageStream backup fails with "certificate signed by unknown authority"

**Symptoms**:

- Regular Velero backups work fine
- ImageStream backups fail with TLS errors
- Error message: `x509: certificate signed by unknown authority`

**Diagnosis**:

```bash
# Verify backupImages is enabled
oc get dpa -n openshift-adp -o jsonpath='{.items[0].spec.backupImages}'

# Check if BSL has caCert configured
oc get backupstoragelocation -n openshift-adp default -o yaml | grep -A 10 caCert

# Verify AWS_CA_BUNDLE in Velero pod
oc exec -n openshift-adp deployment/velero -- printenv AWS_CA_BUNDLE

# Check certificate is mounted
oc exec -n openshift-adp deployment/velero -- test -f /etc/velero/ca-certs/ca-bundle.pem && echo "CA bundle exists" || echo "Missing"
```

**Resolution**:

1. Ensure `spec.backupImages` is not set to `false` - see [backupImages Control Field](#backupimages-control-field)
2. Add `caCert` to your AWS BSL configuration (see [self_signed_certs.md](./self_signed_certs.md))
3. Ensure certificate is PEM-encoded: `openssl x509 -in cert.pem -text -noout`
4. Trigger DPA reconciliation: `oc annotate dpa <name> reconcile=$(date +%s)`
5. Wait for ConfigMap creation and pod volume sync (see [ConfigMap Sync Timing](#configmap-sync-timing))

#### Issue: AWS_CA_BUNDLE not set even with caCert configured

**Symptoms**:

- BSL has `caCert` field populated
- ConfigMap `velero-ca-bundle` does not exist
- `AWS_CA_BUNDLE` environment variable is not set

**Diagnosis**:

```bash
# Check if backupImages is disabled
oc get dpa -n openshift-adp -o jsonpath='{.items[0].spec.backupImages}'

# Check if provider is AWS
oc get backupstoragelocation -n openshift-adp default -o jsonpath='{.spec.provider}'
```

**Root Causes**:

1. `spec.backupImages` is explicitly set to `false` - see [backupImages Control Field](#backupimages-control-field)
2. Provider is not `aws` - see [Certificate Collection Scope](#certificate-collection-scope)

**Resolution**:

- Enable imagestream backups: Set `spec.backupImages: true` or remove the field (defaults to true)
- Ensure provider is `aws` and `caCert` is configured
- For non-AWS providers: See [Provider Support](#provider-support) - OADP currently only processes CA certificates from AWS BSLs

#### Issue: Velero pod restarted after changing backupImages setting

**Symptoms**:

- Changed `spec.backupImages` from `false` to `true` (or vice versa)
- Velero pod restarted
- Running backups marked as `PartiallyFailed`

**Root Cause**: See [Pod Restart Triggers](#pod-restart-triggers) - changing `backupImages` adds/removes volume mount from deployment spec

**Prevention**:

1. Plan `backupImages` changes during maintenance windows
2. Verify no backups running: `velero backup get --output json | jq '.items[] | select(.status.phase=="InProgress")'`
3. Set `backupImages` correctly in initial DPA configuration

**Note**: If you need to trigger discovery of Non-DPA BSL changes, use safe trigger mechanisms instead of toggling `backupImages`. See [Triggering Discovery of Non-DPA BSL Changes](#triggering-discovery-of-non-dpa-bsl-changes) for DPA annotation method that updates ConfigMap without restarting pod.

## Reference Links

- [OpenShift Velero Plugin - ImageStream Shared Code](https://github.com/openshift/openshift-velero-plugin/blob/64292f953c3e2ecd623e9388b2a65c08bb9cfbe2/velero-plugins/imagestream/shared.go#L57)
- [Docker Distribution S3 Driver](https://github.com/openshift/docker-distribution/blob/release-4.19/registry/storage/driver/s3-aws/s3.go)
- [AWS SDK v2 CustomCABundle](https://github.com/aws/aws-sdk-go-v2/blob/1c707a7bc6b5b0bba75e5643d9e3be2f3f779bc1/config/env_config.go#L176-L192)
- [Kubernetes ConfigMap Update Behavior](https://github.com/kubernetes/kubernetes/issues/20200)
- [Self-Signed Certificates Configuration](./self_signed_certs.md)

## Summary

OADP automatically manages CA certificates for **OpenShift ImageStream backups** in environments with custom CAs.

**Quick Reference**:

- **Purpose**: Enable imagestream backups with custom CA certificates
- **Scope**: See [ImageStream Backup Scope](#imagestream-backup-scope) - imagestream backups only
- **Mechanism**: See [Two CA Certificate Mechanisms](#two-ca-certificate-mechanisms) - mounts certificates via `AWS_CA_BUNDLE`
- **Control**: See [backupImages Control Field](#backupimages-control-field) - enabled by default, can be disabled
- **Updates**: See [ConfigMap Sync Timing](#configmap-sync-timing) - certificate changes effective within 1-2 minutes
- **Restart behavior**: See [Pod Restart Triggers](#pod-restart-triggers) - pod restarts only when volume mount changes

For detailed setup, see [Configuration Examples](#configuration-examples). For issues, see [Troubleshooting](#troubleshooting).

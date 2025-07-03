# Standardized Flow Implementation for OADP Operator

This document outlines the implementation of the standardized authentication workflow for OADP Operator, supporting AWS STS, Azure Workload Identity, and GCP Workload Identity Federation (WIF).

## Overview

The OADP Operator implements a standardized authentication flow that provides secure authentication mechanisms across multiple cloud providers by leveraging short-lived tokens instead of long-lived credentials. This implementation follows the standardized authentication workflow defined in OpenShift Enhancement Proposal #1800 and supports:

- **AWS**: Security Token Service (STS) with IAM roles
- **Azure**: Workload Identity with managed identities
- **GCP**: Workload Identity Federation (WIF) with service account impersonation

## Implementation Details

The OADP Operator implements a standardized workflow for short-lived token authentication across AWS, Azure, and GCP cloud providers. This standardized approach conforms to [OpenShift Enhancement Proposal #1800](https://github.com/openshift/enhancements/pull/1800) and is triggered during operator startup to create the necessary secrets for authentication.

The key benefits of this approach include:

- **Enhanced Security**: Uses short-lived tokens instead of long-lived credentials
- **Simplified Management**: Direct secret creation without external dependencies
- **Consistent Experience**: Same workflow pattern across all supported cloud providers
- **OpenShift Integration**: Leverages OpenShift service account tokens for authentication

### 1. Environment Variables and Configuration

The operator detects cloud provider configuration through environment variables provided during OLM installation. Each cloud provider has specific environment variables that must be provided for the standardized flow to be triggered:

- **Cloud Provider Detection**: The operator automatically detects which cloud provider configuration is present based on the provided environment variables
- **OLM Integration**: Environment variables are set during operator installation through the OpenShift Console
- **Single Provider**: Only one cloud provider's credentials should be configured at a time

Specific environment variables for each cloud provider are detailed in the cloud-specific sections below.

### 2. Standardized Credential Flow

The `STSStandardizedFlow()` function in the `stsflow` package handles credential creation for all supported cloud providers:


```go
func STSStandardizedFlow() (string, error) {
    // AWS STS environment variables
    roleARN := os.Getenv(RoleARNEnvKey)

    // GCP WIF environment variables
    serviceAccountEmail := os.Getenv(ServiceAccountEmailEnvKey)
    projectNumber := os.Getenv(ProjectNumberEnvKey)
    poolId := os.Getenv(PoolIDEnvKey)
    providerId := os.Getenv(ProviderId)

    // Azure environment variables
    clientID := os.Getenv(ClientIDEnvKey)
    tenantID := os.Getenv(TenantIDEnvKey)
    subscriptionID := os.Getenv(SubscriptionIDEnvKey)

    // Check if any cloud provider credentials are provided
    hasAWSCreds := len(roleARN) > 0
    hasGCPCreds := len(serviceAccountEmail) > 0 &&
        len(projectNumber) > 0 && len(poolId) > 0 && len(providerId) > 0
    hasAzureCreds := len(clientID) > 0 && len(tenantID) > 0 && len(subscriptionID) > 0

    // If no credentials are provided, return nil
    if !hasAWSCreds && !hasGCPCreds && !hasAzureCreds {
        return "", nil
    }

    // Create appropriate secret based on cloud provider
    if hasAWSCreds {
        if err := CreateOrUpdateSTSAWSSecret(logger, roleARN, installNS, kcfg); err != nil {
            return "", err
        }
        return VeleroAWSSecretName, nil
    } else if hasGCPCreds {
        if err := CreateOrUpdateSTSGCPSecret(logger, serviceAccountEmail, projectNumber,
                                             poolId, providerId, installNS, kcfg); err != nil {
            return "", err
        }
        return VeleroGCPSecretName, nil
    } else if hasAzureCreds {
        if err := CreateOrUpdateSTSAzureSecret(logger, clientID, tenantID,
                                               subscriptionID, installNS, kcfg); err != nil {
            return "", err
        }
        return VeleroAzureSecretName, nil
    }

    return "", nil
}
```

### 3. Cloud Provider Secret Creation

The operator creates cloud-specific secrets using a common `CreateOrUpdateSTSSecret` function, but with different credential formats for each provider:

- **Unified Interface**: All cloud providers use the same secret creation function with provider-specific credential data
- **Secret Naming**: Each provider uses a distinct secret name pattern for easy identification
- **Token Integration**: All providers leverage the OpenShift service account token at `/var/run/secrets/openshift/serviceaccount/token`
- **Atomic Operations**: Secrets are created or updated atomically to ensure consistency

The specific secret formats and implementations for each cloud provider are detailed in the cloud-specific sections below.

### 4. Secret Creation and Management

The `CreateOrUpdateSTSSecret` function handles Secret creation and updates for all cloud providers:


```go
func CreateOrUpdateSTSSecret(setupLog logr.Logger, credStringData map[string]string,
                            secretNS string, kubeconf *rest.Config) error {
    // Determine secret name based on the cloud provider
    secretName := getSecretNameFromCredentials(credStringData)

    secret := corev1.Secret{
        ObjectMeta: metav1.ObjectMeta{
            Name:      secretName,  // e.g., "cloud-credentials", "cloud-credentials-gcp", "cloud-credentials-azure"
            Namespace: secretNS,
        },
        StringData: credStringData,
    }

    // Create the secret, or update if it already exists
    if err := clientInstance.Create(context.Background(), &secret); err != nil {
        if errors.IsAlreadyExists(err) {
            // Update existing secret
            // ... patch logic ...
        }
    }

    // Secret is now available for use by Velero
}
```

The function automatically determines the appropriate secret name based on the cloud provider:

- AWS: `cloud-credentials`
- Azure: `cloud-credentials-azure`
- GCP: `cloud-credentials-gcp`

### 5. Integration with the Operator

The standardized flow is integrated at multiple points:

#### a. Operator Startup (main.go)


```go
// Create Secret and wait for STS cred to exists
if _, err := stsflow.STSStandardizedFlow(); err != nil {
    setupLog.Error(err, "error setting up STS Standardized Flow")
    os.Exit(1)
}
```

#### b. CloudStorage Controller

The CloudStorage controller also calls `STSStandardizedFlow()` to ensure secrets exist before creating cloud storage:


```go
// check if STSStandardizedFlow was successful
if secretName, err = stsflow.STSStandardizedFlow(); err != nil {
    r.Log.Error(err, "unable to check for STS creds secret")
    return ctrl.Result{}, err
}
```

### 6. Secret Names and Content

The implementation uses consistent secret naming and content patterns:

| Cloud Provider | Secret Name | Secret Keys | Content Type |
|----------------|-------------|-------------|---------------|
| AWS | `cloud-credentials` | `credentials` | AWS credentials file format |
| Azure | `cloud-credentials-azure` | `cloud`, `subscriptionId`, `tenantId`, `clientId` | Individual key-value pairs |
| GCP | `cloud-credentials-gcp` | `service_account.json` | GCP external account JSON |

These secrets are created directly by the operator without dependency on external operators like the Cloud Credentials Operator (CCO).

## Usage Instructions

### Prerequisites

#### AWS STS Prerequisites

1. **OpenShift cluster with AWS STS enabled**
   - Cluster must be installed with manual credentials mode
   - Reference: [Installing a cluster on AWS with short-term credentials](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html-single/installing_on_aws/index#installing-aws-with-short-term-creds_installing-aws-customizations)

2. **AWS IAM Role with required permissions**

   - Create an IAM role with trust policy for the OpenShift service account
   - Attach policies with S3 and EC2 permissions for backup operations
   - Note the Role ARN for installation

#### Azure Workload Identity Prerequisites

1. **OpenShift cluster with Azure Workload Identity enabled**
   - Cluster must be installed with manual credentials mode
   - Reference: [Installing a cluster on Azure with short-term credentials](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html-single/installing_on_azure/#installing-azure-with-short-term-creds_installing-azure-customizations)

2. **Azure Managed Identity with required permissions**

   - Create a managed identity with Storage Blob Data Contributor role
   - Note the Client ID, Tenant ID, and Subscription ID

#### GCP WIF Prerequisites

1. **OpenShift cluster with GCP Workload Identity Federation enabled**
   - Cluster must be installed with manual credentials mode
   - Workload Identity Pool and Provider must be configured
   - Reference: [Installing a cluster on GCP with short-term credentials](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html-single/installing_on_gcp/#installing-gcp-with-short-term-creds_installing-gcp-customizations)
   - Prepare the following information from your cluster:
     - GCP Project Number
     - Workload Identity Pool
     - Workload Identity Provider

2. **GCP Service Account with required permissions**

   - Create a service account in your GCP project with the following roles:

     ```text
     compute.disks.create
     compute.disks.createSnapshot
     compute.disks.get
     compute.snapshots.create
     compute.snapshots.delete
     compute.snapshots.get
     compute.snapshots.useReadOnly
     compute.zones.get
     iam.serviceAccounts.signBlob
     storage.objects.create
     storage.objects.delete
     storage.objects.get
     storage.objects.list
     ```

   - Note the Service Account email for the next steps

3. **Grant necessary permissions for GCP**

   Grant the `system:serviceaccount:openshift-adp:velero` service account the `roles/iam.serviceAccountTokenCreator` role:
   ```bash
   gcloud iam service-accounts add-iam-policy-binding <SERVICE_ACCOUNT_EMAIL> \
     --project=<YOUR_PROJECT> \
     --role=roles/iam.serviceAccountTokenCreator \
     --member="principal://iam.googleapis.com/projects/<PROJECT_NUMBER>/locations/global/workloadIdentityPools/<POOL_ID>/subject/system:serviceaccount:openshift-adp:velero"
   ```

   Also bind the service account to the workload identity for the controller manager:
   ```bash
   gcloud iam service-accounts add-iam-policy-binding \
     <SERVICE_ACCOUNT_EMAIL> \
     --role roles/iam.workloadIdentityUser \
     --member "principal://iam.googleapis.com/projects/<PROJECT_NUMBER>/locations/global/workloadIdentityPools/<POOL_ID>/subject/system:serviceaccount:openshift-adp:openshift-adp-controller-manager"
   ```

### Installation Steps

The installation process is similar for all cloud providers, with cloud-specific configuration provided through the OpenShift Console.

1. **Install OADP Operator via OLM**

   For testing the operator from source:
   ```bash
   make deploy-olm && oc delete csv -n openshift-adp oadp-operator.v99.0.0
   ```

   Then install via the console with the appropriate cloud provider parameter:

   **For AWS:**
   ```bash
   $(BROWSER) $(oc whoami --show-console)/operatorhub/subscribe?pkg=oadp-operator&catalog=oadp-operator-catalog&catalogNamespace=openshift-adp&targetNamespace=openshift-adp&channel=operator-sdk-run-bundle&version=99.0.0&tokenizedAuth=AWS
   ```

   **For Azure:**
   ```bash
   $(BROWSER) $(oc whoami --show-console)/operatorhub/subscribe?pkg=oadp-operator&catalog=oadp-operator-catalog&catalogNamespace=openshift-adp&targetNamespace=openshift-adp&channel=operator-sdk-run-bundle&version=99.0.0&tokenizedAuth=Azure
   ```

   **For GCP:**
   ```bash
   $(BROWSER) $(oc whoami --show-console)/operatorhub/subscribe?pkg=oadp-operator&catalog=oadp-operator-catalog&catalogNamespace=openshift-adp&targetNamespace=openshift-adp&channel=operator-sdk-run-bundle&version=99.0.0&tokenizedAuth=GCP
   ```

   > **Tip:** The URL contains `&tokenizedAuth=<CLOUD>` which allows you to test the secret creation functionality even on a cluster without the specific cloud provider configuration. You can input dummy data and see the secret created for testing purposes.

   When installing through the OpenShift Console, you'll be prompted to provide cloud-specific configuration:

   **AWS Configuration:**
   - **Role ARN**: The AWS IAM role ARN to assume

   **Azure Configuration:**
   - **Client ID**: Azure managed identity client ID
   - **Tenant ID**: Azure tenant ID
   - **Subscription ID**: Azure subscription ID

   **GCP Configuration:**
   - **GCP Project Number**: Your GCP project number
   - **Pool ID**: The workload identity pool ID
   - **Provider ID**: The workload identity provider ID
   - **Service Account Email**: The email of the GCP service account to impersonate

   These values will be set as environment variables on the operator deployment.

2. **Verify Secret Creation**

   After installation, verify that the secret was created by the operator:

   **For AWS:**
   ```bash
   oc get secret cloud-credentials -n openshift-adp
   oc extract secret/cloud-credentials -n openshift-adp --to=-
   ```

   **For Azure:**
   ```bash
   oc get secret cloud-credentials-azure -n openshift-adp
   oc describe secret cloud-credentials-azure -n openshift-adp
   ```

   **For GCP:**
   ```bash
   oc get secret cloud-credentials-gcp -n openshift-adp
   oc extract secret/cloud-credentials-gcp -n openshift-adp --to=-
   ```

   The secrets should contain the appropriate configuration for each cloud provider's authentication mechanism.

3. **Create Data Protection Application (DPA)**

   **AWS DPA Example:**
   ```yaml
   apiVersion: oadp.openshift.io/v1alpha1
   kind: DataProtectionApplication
   metadata:
     name: dpa-aws
     namespace: openshift-adp
   spec:
     configuration:
       velero:
         defaultPlugins:
         - openshift
         - aws
     backupLocations:
       - velero:
           provider: aws
           default: true
           credential:
             key: credentials
             name: cloud-credentials
           config:
             region: us-east-1  # REQUIRED: Must specify region here
           objectStorage:
             bucket: <bucket_name>
             prefix: <prefix>
   ```

   **Azure DPA Example:**
   ```yaml
   apiVersion: oadp.openshift.io/v1alpha1
   kind: DataProtectionApplication
   metadata:
     name: dpa-azure
     namespace: openshift-adp
   spec:
     configuration:
       velero:
         defaultPlugins:
         - openshift
         - azure
     backupLocations:
       - velero:
           provider: azure
           default: true
           credential:
             key: cloud
             name: cloud-credentials-azure
           config:
             resourceGroup: <resource_group>
             storageAccount: <storage_account>
           objectStorage:
             bucket: <container_name>
             prefix: <prefix>
   ```

   **GCP DPA Example:**
   ```yaml
   apiVersion: oadp.openshift.io/v1alpha1
   kind: DataProtectionApplication
   metadata:
     name: dpa-gcp
     namespace: openshift-adp
   spec:
     configuration:
       velero:
         defaultPlugins:
         - openshift
         - gcp
     backupLocations:
       - velero:
           provider: gcp
           default: true
           credential:
             key: service_account.json
             name: cloud-credentials-gcp
           objectStorage:
             bucket: <bucket_name>
             prefix: <prefix>
   ```

### Testing the Installation

1. Create a test backup:
   ```bash
   velero backup create test-backup --include-namespaces=<namespace>
   ```

2. Verify the backup completed:
   ```bash
   velero backup describe test-backup
   ```

## Troubleshooting

### Common Issues

1. **Secret not created**
   - Check operator logs: `oc logs -n openshift-adp deployment/openshift-adp-controller-manager`
   - Verify all environment variables are set correctly on the operator deployment
   - Ensure the cloud provider credentials/identities have the correct permissions
   - The operator creates the secret directly without relying on external operators

2. **Authentication failures**

   **AWS:**
   - Verify the IAM role trust policy includes the OpenShift service account
   - Check that the role ARN is correctly formatted
   - Ensure the token file exists at `/var/run/secrets/openshift/serviceaccount/token`

   **Azure:**
   - Verify the managed identity is correctly assigned
   - Check that all Azure IDs (client, tenant, subscription) are valid
   - Ensure the federated identity credential is configured

   **GCP:**
   - Verify the workload identity binding is correct
   - Check that the audience URL is properly formatted
   - Ensure the token file exists at `/var/run/secrets/openshift/serviceaccount/token`

3. **Backup failures**
   - Verify the storage bucket/container exists and is accessible
   - Check that the cloud identity has appropriate storage permissions
   - Review Velero pod logs for detailed error messages
   - Ensure the DPA configuration matches your cloud provider setup

## Key Differences from CCO-based Approach

This implementation follows the standardized authentication workflow (OEP #1800) and differs from CCO-based approaches:

1. **Direct Secret Creation**: The operator creates credentials secrets directly without requiring CredentialsRequest resources
2. **Environment Variable Configuration**: Authentication parameters are provided via environment variables during operator installation
3. **No External Dependencies**: The operator handles all credential management internally without relying on the Cloud Credentials Operator
4. **Standardized Workflow**: Uses the same pattern across AWS STS, GCP WIF, and Azure for consistency

## Cloud-Specific Implementation Details

### AWS STS Implementation

#### AWS Environment Variables
```go
RoleARNEnvKey = "ROLE_ARN"  // AWS IAM role ARN to assume
```

#### AWS Prerequisites
1. **OpenShift cluster with AWS STS enabled**
   - Cluster must be installed with manual credentials mode
   - Reference: [Installing a cluster on AWS with short-term credentials](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html-single/installing_on_aws/index#installing-aws-with-short-term-creds_installing-aws-customizations)

2. **AWS IAM Role with required permissions**
   - Create an IAM role with trust policy for the OpenShift service account
   - Attach policies with S3 and EC2 permissions for backup operations
   - Note the Role ARN for installation

#### AWS Secret Creation
The `CreateOrUpdateSTSAWSSecret` function creates a Secret with AWS STS configuration:

```go
func CreateOrUpdateSTSAWSSecret(setupLog logr.Logger, roleARN, secretNS string,
                               kubeconf *rest.Config) error {
    return CreateOrUpdateSTSSecret(setupLog, map[string]string{
        AWSSecretCredentialsFileKey: fmt.Sprintf(`[default]
role_arn = %s
web_identity_token_file = %s`, roleARN, WebIdentityTokenPath),
    }, secretNS, kubeconf)
}
```

#### AWS Secret Format
- **Secret Name**: `cloud-credentials`
- **Secret Key**: `credentials` (AWS credentials file format)
- **Content**: Standard AWS credentials file with role_arn and web_identity_token_file

#### AWS DPA Configuration
```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: dpa-aws
  namespace: openshift-adp
spec:
  configuration:
    velero:
      defaultPlugins:
      - openshift
      - aws
  backupLocations:
    - velero:
        provider: aws
        default: true
        credential:
          key: credentials
          name: cloud-credentials
        config:
          region: us-east-1  # REQUIRED: Must specify region here
        objectStorage:
          bucket: <bucket_name>
          prefix: <prefix>
```

#### Dynamic Region Configuration

The OADP operator automatically patches the AWS credentials secret with the region information from the first BackupStorageLocation. The region is obtained from:
1. The BSL `config.region` if specified
2. Automatic discovery using the `aws.GetBucketRegion()` function if the bucket is discoverable

This enhancement eliminates the need to manually configure the region in the secret.

**Important**: The standardized flow only supports the first BSL configuration. Additional BSLs in different regions require separate credentials and should not use the standardized flow secret.

### Azure Workload Identity Implementation

#### Azure Environment Variables

```go
ClientIDEnvKey      = "AZURE_CLIENT_ID"       // Azure managed identity client ID
TenantIDEnvKey      = "AZURE_TENANT_ID"       // Azure tenant ID
SubscriptionIDEnvKey = "AZURE_SUBSCRIPTION_ID" // Azure subscription ID
```

#### Azure Prerequisites

1. **OpenShift cluster with Azure Workload Identity enabled**
   - Cluster must be installed with manual credentials mode
   - Reference: [Installing a cluster on Azure with short-term credentials](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html-single/installing_on_azure/#installing-azure-with-short-term-creds_installing-azure-customizations)

2. **Azure Managed Identity with required permissions**
   - Create a managed identity with Storage Blob Data Contributor role
   - Note the Client ID, Tenant ID, and Subscription ID

#### Azure Secret Creation

The `CreateOrUpdateSTSAzureSecret` function creates a Secret with Azure configuration:

```go
func CreateOrUpdateSTSAzureSecret(setupLog logr.Logger, azureClientId, azureTenantId, 
                                 azureSubscriptionId, secretNS string, kubeconf *rest.Config) error {
    // Azure federated identity credentials format
    return CreateOrUpdateSTSSecret(setupLog, VeleroAzureSecretName, map[string]string{
        "azurekey": fmt.Sprintf(`
AZURE_SUBSCRIPTION_ID=%s
AZURE_TENANT_ID=%s
AZURE_CLIENT_ID=%s
AZURE_CLOUD_NAME=AzurePublicCloud
`, azureSubscriptionId, azureTenantId, azureClientId)}, secretNS, kubeconf)
}
```

#### Azure Secret Format

- **Secret Name**: `cloud-credentials-azure`
- **Secret Key**: `azurekey`
- **Content**: Azure environment variables format following [Velero Azure Plugin Option 3: Use Azure AD Workload Identity](https://github.com/vmware-tanzu/velero-plugin-for-microsoft-azure/tree/main#option-3-use-azure-ad-workload-identity)

#### Azure Dynamic Resource Group Configuration

The OADP operator automatically patches the Azure credentials secret with the `AZURE_RESOURCE_GROUP` environment variable from the first BackupStorageLocation that includes the `resourceGroup` in its configuration. This enhancement eliminates the need to manually configure the resource group in the secret.

**Important**: The standardized flow only supports the first BSL configuration. Additional BSLs with different resource groups require separate credentials and should not use the standardized flow secret.

#### Azure DPA Configuration

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: dpa-azure
  namespace: openshift-adp
spec:
  configuration:
    velero:
      defaultPlugins:
      - openshift
      - azure
  backupLocations:
    - velero:
        provider: azure
        default: true
        credential:
          key: azurekey
          name: cloud-credentials-azure
        config:
          resourceGroup: <resource_group>
          storageAccount: <storage_account>
        objectStorage:
          bucket: <container_name>
          prefix: <prefix>
```

### GCP Workload Identity Federation Implementation

#### GCP Environment Variables

```go
ProjectNumberEnvKey       = "PROJECT_NUMBER"        // GCP project number
PoolIDEnvKey              = "POOL_ID"               // Workload identity pool ID
ProviderId                = "PROVIDER_ID"           // Workload identity provider ID
ServiceAccountEmailEnvKey = "SERVICE_ACCOUNT_EMAIL" // Service account email to impersonate
```

#### GCP Prerequisites

1. **OpenShift cluster with GCP Workload Identity Federation enabled**
   - Cluster must be installed with manual credentials mode
   - Workload Identity Pool and Provider must be configured
   - Reference: [Installing a cluster on GCP with short-term credentials](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html-single/installing_on_gcp/#installing-gcp-with-short-term-creds_installing-gcp-customizations)
   - Prepare the following information from your cluster:
     - GCP Project Number
     - Workload Identity Pool
     - Workload Identity Provider

2. **GCP Service Account with required permissions**
   - Create a service account in your GCP project with the following roles:

     ```text
     compute.disks.create
     compute.disks.createSnapshot
     compute.disks.get
     compute.snapshots.create
     compute.snapshots.delete
     compute.snapshots.get
     compute.snapshots.useReadOnly
     compute.zones.get
     iam.serviceAccounts.signBlob
     storage.objects.create
     storage.objects.delete
     storage.objects.get
     storage.objects.list
     ```

   - Note the Service Account email for the next steps

3. **Grant necessary permissions for GCP**

   Grant the `system:serviceaccount:openshift-adp:velero` service account the `roles/iam.serviceAccountTokenCreator` role:

   ```bash
   gcloud iam service-accounts add-iam-policy-binding <SERVICE_ACCOUNT_EMAIL> \
     --project=<YOUR_PROJECT> \
     --role=roles/iam.serviceAccountTokenCreator \
     --member="principal://iam.googleapis.com/projects/<PROJECT_NUMBER>/locations/global/workloadIdentityPools/<POOL_ID>/subject/system:serviceaccount:openshift-adp:velero"
   ```

   Also bind the service account to the workload identity for the controller manager:

   ```bash
   gcloud iam service-accounts add-iam-policy-binding \
     <SERVICE_ACCOUNT_EMAIL> \
     --role roles/iam.workloadIdentityUser \
     --member "principal://iam.googleapis.com/projects/<PROJECT_NUMBER>/locations/global/workloadIdentityPools/<POOL_ID>/subject/system:serviceaccount:openshift-adp:openshift-adp-controller-manager"
   ```

#### GCP Secret Creation

The `CreateOrUpdateSTSGCPSecret` function creates a Secret with the required GCP WIF configuration:

```go
func CreateOrUpdateSTSGCPSecret(setupLog logr.Logger, serviceAccountEmail, projectNumber,
                               poolId, providerId, secretNS string, kubeconf *rest.Config) error {
    audience := fmt.Sprintf("//iam.googleapis.com/projects/%s/locations/global/workloadIdentityPools/%s/providers/%s",
                           projectNumber, poolId, providerId)

    return CreateOrUpdateSTSSecret(setupLog, map[string]string{
        GcpSecretJSONKey: fmt.Sprintf(`{
    "type": "external_account",
    "audience": "%s",
    "subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
    "token_url": "https://sts.googleapis.com/v1/token",
    "service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken",
    "credential_source": {
        "file": "%s",
        "format": {
            "type": "text"
        }
    }
}`, audience, serviceAccountEmail, WebIdentityTokenPath),
    }, secretNS, kubeconf)
}
```

#### GCP Secret Format

- **Secret Name**: `cloud-credentials-gcp`
- **Secret Key**: `service_account.json`
- **Content**: GCP external account JSON following Google's external account format

#### GCP DPA Configuration

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: dpa-gcp
  namespace: openshift-adp
spec:
  configuration:
    velero:
      defaultPlugins:
      - openshift
      - gcp
  backupLocations:
    - velero:
        provider: gcp
        default: true
        credential:
          key: service_account.json
          name: cloud-credentials-gcp
        objectStorage:
          bucket: <bucket_name>
          prefix: <prefix>
```

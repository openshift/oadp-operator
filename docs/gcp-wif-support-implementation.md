# GCP WIF Support Implementation for OADP Operator

This document outlines the implementation of GCP Workload Identity Federation (WIF) support in the OADP Operator, following a standardized authentication workflow similar to AWS STS and Azure.

## Overview

GCP WIF provides a more secure authentication mechanism compared to long-lived service account keys by leveraging short-lived tokens. This implementation allows the OADP Operator to authenticate with GCP using Workload Identity Federation following the standardized authentication workflow defined in OpenShift Enhancement Proposal #1800.

## Implementation Details

The OADP Operator implements a standardized workflow for short-lived token authentication across multiple cloud providers including AWS STS, GCP WIF, and Azure. This standardized approach conforms to [OpenShift Enhancement Proposal #1800](https://github.com/openshift/enhancements/pull/1800) and is triggered during operator startup to create the necessary secrets for authentication.

### 1. Environment Variables and Configuration

The operator detects GCP WIF configuration through the following environment variables (provided during OLM installation):

```go
ProjectNumberEnvKey       = "PROJECT_NUMBER"        // GCP project number
PoolIDEnvKey              = "POOL_ID"               // Workload identity pool ID
ProviderId                = "PROVIDER_ID"           // Workload identity provider ID
ServiceAccountEmailEnvKey = "SERVICE_ACCOUNT_EMAIL" // Service account email to impersonate
```

All four environment variables must be provided for the GCP WIF workflow to be triggered.

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

### 3. Creating GCP WIF Secret

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

Key points:
- The secret key is `service_account.json` (defined by `GcpSecretJSONKey`)
- The audience is constructed from the project number, pool ID, and provider ID
- The token file path is `/var/run/secrets/openshift/serviceaccount/token`
- This follows the standard external account configuration format for GCP

### 4. Secret Creation and Management

The `CreateOrUpdateSTSSecret` function handles Secret creation and updates:

```go
func CreateOrUpdateSTSSecret(setupLog logr.Logger, credStringData map[string]string, 
                            secretNS string, kubeconf *rest.Config) error {
    secret := corev1.Secret{
        ObjectMeta: metav1.ObjectMeta{
            Name:      VeleroGCPSecretName,  // "cloud-credentials-gcp"
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

### 6. Secret Names

The implementation uses consistent secret names across cloud providers following the standardized workflow:
- AWS: `cloud-credentials`
- GCP: `cloud-credentials-gcp`  
- Azure: `cloud-credentials-azure`

These secrets are created directly by the operator without dependency on external operators.

## Usage Instructions

### Prerequisites

1. **OpenShift cluster with GCP Workload Identity Federation enabled**
   - Cluster must be installed with manual credentials mode
   - Workload Identity Pool and Provider must be configured
   - Reference: [Installing a cluster on GCP with manual credentials](https://docs.openshift.com/container-platform/latest/installing/installing_gcp/installing-gcp-manual.html)
   - Prepare the following information from your cluster:
     - GCP Project Number
     - Workload Identity Pool
     - Workload Identity Provider

2. **GCP Service Account with required permissions**
   - Create a service account in your GCP project with the following roles:
     ```
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

3. **Grant necessary permissions to Velero service account**
   
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

1. **Install OADP Operator via OLM**
   
   For testing the operator from source:
   ```bash
   make deploy-olm && oc delete csv -n openshift-adp oadp-operator.v99.0.0
   ```
   
   Then install via the console:
   ```bash
   $(BROWSER) $(oc whoami --show-console)/operatorhub/subscribe?pkg=oadp-operator&catalog=oadp-operator-catalog&catalogNamespace=openshift-adp&targetNamespace=openshift-adp&channel=operator-sdk-run-bundle&version=99.0.0&tokenizedAuth=GCP
   ```
   
   > **Tip:** The URL contains `&tokenizedAuth=GCP` which allows you to test the secret creation functionality even on a non-GCP WIF cluster. You can input dummy data and see the secret created for testing purposes.
   
   When installing through the OpenShift Console, you'll be prompted to provide:
   - **GCP Project Number**: Your GCP project number
   - **Pool ID**: The workload identity pool ID
   - **Provider ID**: The workload identity provider ID  
   - **Service Account Email**: The email of the GCP service account to impersonate

   These values will be set as environment variables on the operator deployment.

2. **Verify Secret Creation**
   
   After installation, verify that the secret was created by the operator:
   ```bash
   oc get secret cloud-credentials-gcp -n openshift-adp
   ```
   
   The secret should contain the GCP external account configuration needed for Workload Identity Federation.

3. **Create Data Protection Application (DPA)**
   
   ```yaml
   apiVersion: oadp.openshift.io/v1alpha1
   kind: DataProtectionApplication
   metadata:
     name: dpa-sample
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
   - Ensure the service account has the correct permissions
   - The operator creates the secret directly without relying on external operators

2. **Authentication failures**
   - Verify the workload identity binding is correct
   - Check that the audience URL is properly formatted
   - Ensure the token file exists at `/var/run/secrets/openshift/serviceaccount/token`

3. **Backup failures**
   - Verify the GCS bucket exists and is accessible
   - Check that the service account has storage permissions
   - Review Velero pod logs for detailed error messages

## Key Differences from CCO-based Approach

This implementation follows the standardized authentication workflow (OEP #1800) and differs from CCO-based approaches:

1. **Direct Secret Creation**: The operator creates credentials secrets directly without requiring CredentialsRequest resources
2. **Environment Variable Configuration**: Authentication parameters are provided via environment variables during operator installation
3. **No External Dependencies**: The operator handles all credential management internally without relying on the Cloud Credentials Operator
4. **Standardized Workflow**: Uses the same pattern across AWS STS, GCP WIF, and Azure for consistency

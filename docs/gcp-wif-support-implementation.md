# GCP WIF Support Implementation for OADP Operator

This document outlines the implementation of GCP Workload Identity Federation (WIF) support in the OADP Operator, similar to how AWS STS is currently supported.

## Overview

GCP WIF provides a more secure authentication mechanism compared to long-lived service account keys by leveraging short-lived tokens. This implementation allows the OADP Operator to authenticate with GCP using Workload Identity Federation through the Cloud Credentials Operator (CCO).

## Implementation Details

### 1. Detection of GCP WIF

The `CCOWorkflow()` function has been enhanced to check for GCP WIF environment variables:

```go
func CCOWorkflow() bool {
    roleARN := os.Getenv("ROLEARN")
    audience := os.Getenv("AUDIENCE")
    serviceAccountEmail := os.Getenv("SERVICE_ACCOUNT_EMAIL")
    
    if len(roleARN) > 0 || (len(audience) > 0 && len(serviceAccountEmail) > 0) {
        return true
    }
    return false
}
```

Two new helper functions were added to distinguish between AWS STS and GCP WIF:

```go
func IsAWSSTS() bool {
    roleARN := os.Getenv("ROLEARN")
    return len(roleARN) > 0
}

func IsGCPWIF() bool {
    audience := os.Getenv("AUDIENCE")
    serviceAccountEmail := os.Getenv("SERVICE_ACCOUNT_EMAIL")
    return len(audience) > 0 && len(serviceAccountEmail) > 0
}
```

### 2. Creating CredentialsRequest for GCP WIF

A new function `CreateOrUpdateGCPCredRequest` was implemented to create a CredentialsRequest for GCP WIF:

```go
func CreateOrUpdateGCPCredRequest(audience string, serviceAccountEmail string, cloudTokenPath string, secretNS string, kubeconf *rest.Config) error {
    // Creates a CredentialsRequest to obtain a secret for GCP WIF authentication
}
```

The `main.go` file was updated to handle GCP WIF workflow:

```go
if common.IsGCPWIF() {
    // Handle GCP WIF
    audience := os.Getenv("AUDIENCE")
    serviceAccountEmail := os.Getenv("SERVICE_ACCOUNT_EMAIL")
    gcpIdentityTokenFile := WebIdentityTokenPath
    
    setupLog.Info("GCP WIF specified by the user, following standardized WIF workflow")
    
    // create cred request
    if err := CreateOrUpdateGCPCredRequest(audience, serviceAccountEmail, gcpIdentityTokenFile, watchNamespace, kubeconf); err != nil {
        // handle error
    }
}
```

### 3. Getting GCP Credentials from Secret

A new function `GetGCPCredentialsFromSecret` was implemented to retrieve the GCP credentials from the Secret created by the CCO:

```go
func GetGCPCredentialsFromSecret(clientset kubernetes.Interface, namespace string) (string, error) {
    // Wait for the Secret to be created by CCO
    secret, err := WaitForSecret(clientset, namespace, "cloud-credentials")
    if err != nil {
        return "", fmt.Errorf("error waiting for GCP credentials Secret: %v", err)
    }
    
    // Read the service_account.json field from the Secret
    serviceAccountJSON, ok := secret.Data["service_account.json"]
    if !ok {
        return "", fmt.Errorf("cloud-credentials Secret does not contain service_account.json field")
    }
    
    return string(serviceAccountJSON), nil
}
```

### 4. Waiting for the Secret

A function `WaitForSecret` was implemented to wait for the Secret to be created by the CCO:

```go
func WaitForSecret(client kubernetes.Interface, namespace, name string) (*corev1.Secret, error) {
    // Wait for up to 10 minutes for the Secret to be created
}
```

## Usage

1. The user sets the following environment variables on the OADP Operator deployment:
   - `AUDIENCE`: The Workload Identity Pool audience
   - `SERVICE_ACCOUNT_EMAIL`: The GCP service account email to impersonate

2. The operator detects these variables and creates a CredentialsRequest for GCP WIF.

3. The CCO processes the CredentialsRequest and creates a Secret with the credentials.

4. The operator reads the Secret and uses the credentials to authenticate with GCP.

## Test Coverage

Unit tests have been implemented to ensure the GCP WIF support functions correctly:

- `TestGetGCPCredentialsFromSecret`: Tests retrieving GCP credentials from the Secret.
- `TestCreateOrUpdateGCPCredRequest`: Tests creating a CredentialsRequest for GCP WIF.

## Future Work

Additional features for future consideration:

1. Support for standardized update flow for OLM-managed operators leveraging short-lived token authentication.
2. Enhanced error handling and retry mechanisms for more robust integration with CCO.
3. Monitoring and alerting for credential expiration and renewal.

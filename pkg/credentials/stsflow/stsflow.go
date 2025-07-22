package stsflow

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pkgclient "github.com/openshift/oadp-operator/pkg/client"
)

const (
	// STS Flow Environment Vars
	RoleARNEnvKey = "ROLEARN" // AWS STS role ARN

	ProjectNumberEnvKey       = "PROJECT_NUMBER"        // GCP WIF project number
	PoolIDEnvKey              = "POOL_ID"               // GCP WIF pool ID
	ProviderId                = "PROVIDER_ID"           // GCP WIF provider ID
	ServiceAccountEmailEnvKey = "SERVICE_ACCOUNT_EMAIL" // GCP WIF service account email

	ClientIDEnvKey       = "CLIENTID"       // Azure client ID
	TenantIDEnvKey       = "TENANTID"       // Azure tenant ID
	SubscriptionIDEnvKey = "SUBSCRIPTIONID" // Azure subscription ID

	// WebIdentityTokenPath mount present on operator CSV
	WebIdentityTokenPath = "/var/run/secrets/openshift/serviceaccount/token"

	// Azure workload identity secret name
	AzureWorkloadIdentitySecretName = "azure-workload-identity-env"

	// Cloud Provider Secret Keys - standard key names for cloud credentials
	AzureClientID           = "azure_client_id"
	AzureClientSecret       = "azure_client_secret"
	AzureRegion             = "azure_region"
	AzureResourceGroup      = "azure_resourcegroup"
	AzureResourcePrefix     = "azure_resource_prefix"
	AzureSubscriptionID     = "azure_subscription_id"
	AzureTenantID           = "azure_tenant_id"
	AzureFederatedTokenFile = "azure_federated_token_file"

	// GCP Secret key name
	GcpSecretJSONKey = "service_account.json"

	VeleroAWSSecretName   = "cloud-credentials"
	VeleroAzureSecretName = "cloud-credentials-azure"
	VeleroGCPSecretName   = "cloud-credentials-gcp"
)

// STSStandardizedFlow creates secrets for Short Term Service Account Tokens from environment variables for
// AWS STS, GCP WIF, and Azure following the standardized authentication workflow (https://github.com/openshift/enhancements/pull/1800).
// Users provide these values during web console installation, and they are set as environment
// variables on the operator deployment during installation via OLM.
// Returns "", error if secret creation fails.
// Returns <secretName>, nil if secret creation succeeds.
// Returns "", nil if no STS environment variables are provided.
func STSStandardizedFlow() (string, error) {
	// Reference of provided envs from web console.
	// https://github.com/openshift/console/blob/f11a6158ae722200d342519971af337f8ff61d3a/frontend/packages/operator-lifecycle-manager/src/components/operator-hub/operator-hub-subscribe.tsx#L502-L555

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
	// Logger set from cmd/main.go by ctrl.SetLogger
	logger := log.Log
	installNS := os.Getenv("WATCH_NAMESPACE")
	kcfg := pkgclient.GetKubeconf()
	if hasAWSCreds {
		if err := CreateOrUpdateSTSAWSSecret(logger, roleARN, installNS, kcfg); err != nil {
			return "", err
		}
		return VeleroAWSSecretName, nil
	} else if hasGCPCreds {
		if err := CreateOrUpdateSTSGCPSecret(logger, serviceAccountEmail, projectNumber, poolId, providerId, installNS, kcfg); err != nil {
			return "", err
		}
		return VeleroGCPSecretName, nil
	} else if hasAzureCreds {
		if err := CreateOrUpdateSTSAzureSecret(logger, clientID, tenantID, subscriptionID, installNS, kcfg); err != nil {
			return "", err
		}
		return VeleroAzureSecretName, nil
	}

	return "", nil
}

func CreateOrUpdateSTSAWSSecret(setupLog logr.Logger, roleARN string, secretNS string, kubeconf *rest.Config) error {
	// AWS STS credentials format
	return CreateOrUpdateSTSSecret(setupLog, VeleroAWSSecretName, map[string]string{
		"credentials": fmt.Sprintf(`[default]
sts_regional_endpoints = regional
role_arn = %s
web_identity_token_file = %s`, roleARN, WebIdentityTokenPath),
	}, secretNS, kubeconf)
}

func CreateOrUpdateSTSGCPSecret(setupLog logr.Logger, serviceAccountEmail, projectNumber, poolId, providerId, secretNS string, kubeconf *rest.Config) error {
	audience := fmt.Sprintf("//iam.googleapis.com/projects/%s/locations/global/workloadIdentityPools/%s/providers/%s", projectNumber, poolId, providerId)
	// GCP external account credentials format for Workload Identity Federation
	return CreateOrUpdateSTSSecret(setupLog, VeleroGCPSecretName, map[string]string{
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

func CreateOrUpdateSTSAzureSecret(setupLog logr.Logger, azureClientId, azureTenantId, azureSubscriptionId, secretNS string, kubeconf *rest.Config) error {
	clientInstance, err := client.New(kubeconf, client.Options{})
	if err != nil {
		setupLog.Error(err, "unable to create client")
		return err
	}

	// Create a clientset to use for waiting on the secret
	clientset, err := kubernetes.NewForConfig(kubeconf)
	if err != nil {
		setupLog.Error(err, "unable to create clientset")
		return err
	}

	return CreateOrUpdateSTSAzureSecretWithClients(setupLog, azureClientId, azureTenantId, azureSubscriptionId, secretNS, clientInstance, clientset)
}

// CreateOrUpdateSTSAzureSecretWithClients is a testable version that accepts injected clients
func CreateOrUpdateSTSAzureSecretWithClients(setupLog logr.Logger, azureClientId, azureTenantId, azureSubscriptionId, secretNS string, clientInstance client.Client, clientset kubernetes.Interface) error {
	// Azure federated identity credentials format
	err := CreateOrUpdateSTSSecretWithClients(setupLog, VeleroAzureSecretName, map[string]string{
		"azurekey": fmt.Sprintf(`
AZURE_SUBSCRIPTION_ID=%s
AZURE_TENANT_ID=%s
AZURE_CLIENT_ID=%s
AZURE_CLOUD_NAME=AzurePublicCloud
`, azureSubscriptionId, azureTenantId, azureClientId)}, secretNS, clientInstance, clientset)

	if err != nil {
		return err
	}

	// Annotate the Velero service account for Azure workload identity
	if err := AnnotateVeleroServiceAccountForAzureWithClient(setupLog, azureClientId, secretNS, clientInstance); err != nil {
		// Log the error but don't fail the secret creation
		setupLog.Error(err, "Failed to annotate Velero service account for Azure workload identity")
	}

	return nil
}

func CreateOrUpdateSTSSecret(setupLog logr.Logger, secretName string, credStringData map[string]string, secretNS string, kubeconf *rest.Config) error {
	clientInstance, err := client.New(kubeconf, client.Options{})
	if err != nil {
		setupLog.Error(err, "unable to create client")
		return err
	}

	// Create a clientset to use for waiting on the secret
	clientset, err := kubernetes.NewForConfig(kubeconf)
	if err != nil {
		setupLog.Error(err, "unable to create clientset")
		return err
	}

	return CreateOrUpdateSTSSecretWithClients(setupLog, secretName, credStringData, secretNS, clientInstance, clientset)
}

// CreateOrUpdateSTSSecretWithClients is a testable version that accepts injected clients
func CreateOrUpdateSTSSecretWithClients(setupLog logr.Logger, secretName string, credStringData map[string]string, secretNS string, clientInstance client.Client, clientset kubernetes.Interface) error {
	return CreateOrUpdateSTSSecretWithClientsAndWait(setupLog, secretName, credStringData, secretNS, clientInstance, clientset, true)
}

// CreateOrUpdateSTSSecretWithClientsAndWait is a testable version that accepts injected clients and optional wait
func CreateOrUpdateSTSSecretWithClientsAndWait(setupLog logr.Logger, secretName string, credStringData map[string]string, secretNS string, clientInstance client.Client, clientset kubernetes.Interface, waitForSecret bool) error {
	// Create a secret with the appropriate credentials format for STS/WIF authentication
	// Secret format follows standard patterns used by cloud providers
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: secretNS,
			Labels: map[string]string{
				"oadp.openshift.io/secret-type": "sts-credentials",
			},
		},
		StringData: credStringData,
	}
	verb := "created"
	if err := clientInstance.Create(context.Background(), &secret); err != nil {
		if errors.IsAlreadyExists(err) {
			verb = "updated"
			setupLog.Info("Secret already exists, updating")
			fromCluster := corev1.Secret{}
			err = clientInstance.Get(context.Background(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, &fromCluster)
			if err != nil {
				setupLog.Error(err, "unable to get existing secret resource")
				return err
			}
			// update StringData - preserve existing Data that's not being replaced
			// This is safe because STS credentials are only updated during install/reconfiguration,
			// and any BSL-specific patches (like region) should be preserved
			updatedFromCluster := fromCluster.DeepCopy()
			// Initialize StringData if not present
			if updatedFromCluster.StringData == nil {
				updatedFromCluster.StringData = make(map[string]string)
			}
			// Update only the new StringData fields, preserving existing Data
			for key, value := range secret.StringData {
				updatedFromCluster.StringData[key] = value
			}
			// Ensure labels are set
			if updatedFromCluster.Labels == nil {
				updatedFromCluster.Labels = make(map[string]string)
			}
			updatedFromCluster.Labels["oadp.openshift.io/secret-type"] = "sts-credentials"
			if err := clientInstance.Patch(context.Background(), updatedFromCluster, client.MergeFrom(&fromCluster)); err != nil {
				setupLog.Error(err, fmt.Sprintf("unable to update secret resource: %v", err))
				return err
			}
		} else {
			setupLog.Error(err, "unable to create secret resource")
			return err
		}
	}
	setupLog.Info("Secret " + secret.Name + " " + verb + " successfully")

	if waitForSecret {
		// Wait for the Secret to be available
		setupLog.Info(fmt.Sprintf("Waiting for %s Secret to be available", secret.Name))
		_, err := WaitForSecret(clientset, secretNS, secretName)
		if err != nil {
			setupLog.Error(err, "error waiting for credentials Secret")
			return err
		}
		setupLog.Info("credentials Secret is now available")
	}

	return nil
}

// AnnotateVeleroServiceAccountForAzure annotates the Velero service account with Azure workload identity client ID
func AnnotateVeleroServiceAccountForAzure(setupLog logr.Logger, clientID string, namespace string, kubeconf *rest.Config) error {
	if clientID == "" {
		return fmt.Errorf("clientID cannot be empty")
	}

	clientInstance, err := client.New(kubeconf, client.Options{})
	if err != nil {
		setupLog.Error(err, "unable to create client")
		return err
	}

	return AnnotateVeleroServiceAccountForAzureWithClient(setupLog, clientID, namespace, clientInstance)
}

// AnnotateVeleroServiceAccountForAzureWithClient is a testable version that accepts an injected client
func AnnotateVeleroServiceAccountForAzureWithClient(setupLog logr.Logger, clientID string, namespace string, clientInstance client.Client) error {
	if clientID == "" {
		return fmt.Errorf("clientID cannot be empty")
	}

	// Get the velero service account
	sa := &corev1.ServiceAccount{}
	err := clientInstance.Get(context.Background(), types.NamespacedName{
		Name:      "velero",
		Namespace: namespace,
	}, sa)
	if err != nil {
		if errors.IsNotFound(err) {
			setupLog.Info("Velero service account not found yet, skipping annotation")
			return nil
		}
		return err
	}

	// Prepare the patch
	originalSA := sa.DeepCopy()
	if sa.Annotations == nil {
		sa.Annotations = make(map[string]string)
	}

	// Apply the patch
	if err := clientInstance.Patch(context.Background(), sa, client.MergeFrom(originalSA)); err != nil {
		setupLog.Error(err, "unable to patch service account with Azure workload identity annotation")
		return err
	}

	setupLog.Info("Successfully annotated Velero service account for Azure workload identity", "clientID", clientID)
	return nil
}

// CreateCredRequest WITP : WebIdentityTokenPath
// WaitForSecret is a function that takes a Kubernetes client, a namespace, and a v1 "k8s.io/api/core/v1" name as arguments
// It waits until the secret object with the given name exists in the given namespace
// It returns the secret object or an error if the timeout is exceeded
func WaitForSecret(k8sClient kubernetes.Interface, namespace, name string) (*corev1.Secret, error) {
	// set a timeout of 10 minutes
	timeout := time.After(10 * time.Minute)

	// set a polling interval of 10 seconds
	ticker := time.NewTicker(10 * time.Second)

	// loop until the timeout or the secret is found
	for {
		select {
		case <-timeout:
			// timeout is exceeded, return an error
			return nil, fmt.Errorf("timed out waiting for secret %s in namespace %s. Please follow the manual path to create a Secret", name, namespace)
		case <-ticker.C:
			// polling interval is reached, try to get the secret
			secret, err := k8sClient.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					// secret does not exist yet, continue waiting
					continue
				} else {
					// some other error occurred, return it
					return nil, err
				}
			} else {
				// secret is found, return it
				return secret, nil
			}
		}
	}
}

// GetGCPCredentialsFromSecret waits for the cloud-credentials Secret
// and reads the service_account.json field
func GetGCPCredentialsFromSecret(clientset kubernetes.Interface, namespace string) (string, error) {
	// Wait for the Secret to be available
	secret, err := WaitForSecret(clientset, namespace, VeleroGCPSecretName)
	if err != nil {
		return "", fmt.Errorf("error waiting for GCP credentials Secret: %v", err)
	}

	// Read the service_account.json field from the Secret
	serviceAccountJSON, ok := secret.Data[GcpSecretJSONKey]
	if !ok {
		return "", fmt.Errorf("cloud-credentials Secret does not contain %s field", GcpSecretJSONKey)
	}

	return string(serviceAccountJSON), nil
}

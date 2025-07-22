package controller

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openshift/oadp-operator/pkg/credentials/stsflow"
)

// ReconcileAzureWorkloadIdentitySecret ensures the Azure workload identity secret exists.
// These environment variables are required to trigger Velero's workload identity credential flow:
// https://github.com/vmware-tanzu/velero/blob/5c0cb58f6a4f95eb93c18e7e8f8d3d14b94d6805/pkg/util/azure/credential.go#L48-L54
// The azidentity.NewWorkloadIdentityCredential relies on these three environment variables:
// - AZURE_CLIENT_ID
// - AZURE_TENANT_ID
// - AZURE_FEDERATED_TOKEN_FILE
func (r *DataProtectionApplicationReconciler) ReconcileAzureWorkloadIdentitySecret(log logr.Logger) (bool, error) {
	dpa := r.dpa
	azureClientID := os.Getenv(stsflow.ClientIDEnvKey)

	// Only create secret if Azure workload identity environment variables are present
	azureTenantID := os.Getenv(stsflow.TenantIDEnvKey)
	if azureClientID == "" || azureTenantID == "" || os.Getenv(stsflow.SubscriptionIDEnvKey) == "" {
		// No Azure workload identity configured, nothing to do
		return true, nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stsflow.AzureWorkloadIdentitySecretName,
			Namespace: dpa.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, secret, func() error {
		// Add labels
		secret.Labels = getDpaAppLabels(dpa)

		// Set the data
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		// This secret is used via envFrom in both velero.go and nodeagent.go
		// to inject these environment variables into the Velero deployment and NodeAgent daemonset containers
		secret.Data["AZURE_CLIENT_ID"] = []byte(azureClientID)
		secret.Data["AZURE_TENANT_ID"] = []byte(azureTenantID)
		secret.Data["AZURE_FEDERATED_TOKEN_FILE"] = []byte(stsflow.WebIdentityTokenPath)

		// Set controller reference
		return controllerutil.SetControllerReference(dpa, secret, r.Scheme)
	})

	if err != nil {
		log.Error(err, "Error reconciling Azure workload identity secret")
		return false, err
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		r.EventRecorder.Event(secret,
			corev1.EventTypeNormal,
			"AzureWorkloadIdentitySecretReconciled",
			fmt.Sprintf("performed %s on azure workload identity secret %s/%s", op, secret.Namespace, secret.Name),
		)
	}

	return true, nil
}

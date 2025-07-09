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

// ReconcileAzureWorkloadIdentitySecret ensures the Azure workload identity secret exists
func (r *DataProtectionApplicationReconciler) ReconcileAzureWorkloadIdentitySecret(log logr.Logger) (bool, error) {
	dpa := r.dpa
	azureClientID := os.Getenv(stsflow.ClientIDEnvKey)

	// Only create secret if Azure workload identity environment variables are present
	if azureClientID == "" || os.Getenv(stsflow.TenantIDEnvKey) == "" || os.Getenv(stsflow.SubscriptionIDEnvKey) == "" {
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
		secret.Data["AZURE_CLIENT_ID"] = []byte(azureClientID)
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
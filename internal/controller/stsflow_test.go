package controller

import (
	"context"
	"os"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/credentials/stsflow"
)

func newEventRecorder() record.EventRecorder {
	return record.NewFakeRecorder(10)
}

func TestDPAReconciler_ReconcileAzureWorkloadIdentitySecret(t *testing.T) {
	tests := []struct {
		name           string
		dpa            *oadpv1alpha1.DataProtectionApplication
		envVars        map[string]string
		wantSecret     bool
		wantSecretData map[string]string
		wantError      bool
	}{
		{
			name: "Azure STS credentials present - should create secret",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
			},
			envVars: map[string]string{
				stsflow.ClientIDEnvKey:       "test-client-id",
				stsflow.TenantIDEnvKey:       "test-tenant-id",
				stsflow.SubscriptionIDEnvKey: "test-subscription-id",
			},
			wantSecret: true,
			wantSecretData: map[string]string{
				"AZURE_CLIENT_ID":            "test-client-id",
				"AZURE_TENANT_ID":            "test-tenant-id",
				"AZURE_FEDERATED_TOKEN_FILE": stsflow.WebIdentityTokenPath,
			},
			wantError: false,
		},
		{
			name: "No Azure credentials - should not create secret",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
			},
			envVars:    map[string]string{},
			wantSecret: false,
			wantError:  false,
		},
		{
			name: "Partial Azure credentials (missing tenant) - should not create secret",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
			},
			envVars: map[string]string{
				stsflow.ClientIDEnvKey:       "test-client-id",
				stsflow.SubscriptionIDEnvKey: "test-subscription-id",
			},
			wantSecret: false,
			wantError:  false,
		},
		{
			name: "Partial Azure credentials (missing subscription) - should not create secret",
			dpa: &oadpv1alpha1.DataProtectionApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dpa",
					Namespace: "test-ns",
				},
			},
			envVars: map[string]string{
				stsflow.ClientIDEnvKey: "test-client-id",
				stsflow.TenantIDEnvKey: "test-tenant-id",
			},
			wantSecret: false,
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for test
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			// Create scheme with required types
			scheme := runtime.NewScheme()
			oadpv1alpha1.AddToScheme(scheme)
			corev1.AddToScheme(scheme)

			// Create fake client
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.dpa).
				Build()

			// Create reconciler
			r := &DataProtectionApplicationReconciler{
				Client:        fakeClient,
				Scheme:        scheme,
				dpa:           tt.dpa,
				Log:           logr.Discard(),
				Context:       context.Background(),
				EventRecorder: newEventRecorder(),
			}

			// Call the function
			result, err := r.ReconcileAzureWorkloadIdentitySecret(logr.Discard())

			// Check error
			if (err != nil) != tt.wantError {
				t.Errorf("ReconcileAzureWorkloadIdentitySecret() error = %v, wantError %v", err, tt.wantError)
				return
			}

			// Should always return true unless there's an error
			if !tt.wantError && !result {
				t.Errorf("ReconcileAzureWorkloadIdentitySecret() result = %v, want true", result)
			}

			// Check if secret was created
			secret := &corev1.Secret{}
			err = fakeClient.Get(context.Background(), types.NamespacedName{
				Name:      stsflow.AzureWorkloadIdentitySecretName,
				Namespace: tt.dpa.Namespace,
			}, secret)

			if tt.wantSecret {
				if err != nil {
					t.Errorf("Expected secret to be created, but got error: %v", err)
					return
				}

				// Check secret data
				for key, expectedValue := range tt.wantSecretData {
					actualValue, exists := secret.Data[key]
					if !exists {
						t.Errorf("Expected secret data key %s to exist", key)
						continue
					}
					if string(actualValue) != expectedValue {
						t.Errorf("Secret data[%s] = %s, want %s", key, string(actualValue), expectedValue)
					}
				}

				// Check labels
				labels := secret.GetLabels()
				if labels[oadpv1alpha1.OadpOperatorLabel] != "True" {
					t.Errorf("Expected OADP operator label to be set")
				}

				// Check owner reference
				if len(secret.GetOwnerReferences()) != 1 {
					t.Errorf("Expected exactly one owner reference, got %d", len(secret.GetOwnerReferences()))
				} else {
					ownerRef := secret.GetOwnerReferences()[0]
					if ownerRef.Name != tt.dpa.Name {
						t.Errorf("Expected owner reference name to be %s, got %s", tt.dpa.Name, ownerRef.Name)
					}
				}
			} else {
				if err == nil {
					t.Errorf("Expected secret to not be created, but it was")
				}
			}
		})
	}
}

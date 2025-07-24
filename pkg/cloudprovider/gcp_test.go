package cloudprovider

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

func TestNewGCPProvider(t *testing.T) {
	tests := []struct {
		name            string
		credentialsJSON string
		bucket          string
		expectError     bool
	}{
		{
			name: "valid service account credentials",
			credentialsJSON: `{
				"type": "service_account",
				"project_id": "test-project",
				"private_key_id": "test-key-id",
				"private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC...\n-----END PRIVATE KEY-----\n",
				"client_email": "test@test-project.iam.gserviceaccount.com",
				"client_id": "123456789",
				"auth_uri": "https://accounts.google.com/o/oauth2/auth",
				"token_uri": "https://oauth2.googleapis.com/token"
			}`,
			bucket:      "test-bucket",
			expectError: false,
		},
		{
			name:            "invalid JSON",
			credentialsJSON: `invalid json`,
			bucket:          "test-bucket",
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			provider, err := NewGCPProvider(ctx, tt.bucket, []byte(tt.credentialsJSON))

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if provider != nil {
					t.Errorf("Expected provider to be nil on error")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if provider == nil {
					t.Errorf("Expected provider to be created")
				}
				if provider.bucket != tt.bucket {
					t.Errorf("Expected bucket %s, got %s", tt.bucket, provider.bucket)
				}
				// Clean up client
				if provider.client != nil {
					provider.client.Close()
				}
			}
		})
	}
}

func TestGCPProvider_UploadTest(t *testing.T) {
	// Skip test if no real credentials are provided
	t.Skip("Skipping upload test - requires real GCP credentials")

	// This test would require real GCP credentials and bucket
	// In a real test environment, you would:
	// 1. Set up a test GCS bucket
	// 2. Create service account credentials
	// 3. Run the actual upload test

	validCredentials := `{
		"type": "service_account",
		"project_id": "test-project",
		"private_key_id": "test-key-id",
		"private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC...\n-----END PRIVATE KEY-----\n",
		"client_email": "test@test-project.iam.gserviceaccount.com",
		"client_id": "123456789",
		"auth_uri": "https://accounts.google.com/o/oauth2/auth",
		"token_uri": "https://oauth2.googleapis.com/token"
	}`

	provider, err := NewGCPProvider(context.Background(), "test-bucket", []byte(validCredentials))
	if err != nil {
		t.Fatalf("Failed to create GCP provider: %v", err)
	}
	defer provider.client.Close()

	config := oadpv1alpha1.UploadSpeedTestConfig{
		FileSize: "1KB",
		Timeout:  metav1.Duration{Duration: 30 * time.Second},
	}

	ctx := context.Background()
	log := logr.Discard()

	speed, duration, err := provider.UploadTest(ctx, config, "test-bucket", log)
	if err != nil {
		t.Logf("Upload test failed as expected without real credentials: %v", err)
	} else {
		t.Logf("Upload test succeeded: speed=%d Mbps, duration=%v", speed, duration)
	}
}

func TestGCPProvider_GetBucketMetadata(t *testing.T) {
	// Skip test if no real credentials are provided
	t.Skip("Skipping metadata test - requires real GCP credentials")

	// This test would require real GCP credentials and bucket
	validCredentials := `{
		"type": "service_account",
		"project_id": "test-project",
		"private_key_id": "test-key-id",
		"private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC...\n-----END PRIVATE KEY-----\n",
		"client_email": "test@test-project.iam.gserviceaccount.com",
		"client_id": "123456789",
		"auth_uri": "https://accounts.google.com/o/oauth2/auth",
		"token_uri": "https://oauth2.googleapis.com/token"
	}`

	provider, err := NewGCPProvider(context.Background(), "test-bucket", []byte(validCredentials))
	if err != nil {
		t.Fatalf("Failed to create GCP provider: %v", err)
	}
	defer provider.client.Close()

	ctx := context.Background()
	log := logr.Discard()

	metadata, err := provider.GetBucketMetadata(ctx, "test-bucket", log)

	if err != nil {
		t.Logf("GetBucketMetadata failed as expected without real credentials: %v", err)
	}

	if metadata == nil {
		t.Error("Expected metadata to be returned even on error")
	}
}

func TestGCPProvider_Close(t *testing.T) {
	validCredentials := `{
		"type": "service_account",
		"project_id": "test-project",
		"private_key_id": "test-key-id",
		"private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC...\n-----END PRIVATE KEY-----\n",
		"client_email": "test@test-project.iam.gserviceaccount.com",
		"client_id": "123456789",
		"auth_uri": "https://accounts.google.com/o/oauth2/auth",
		"token_uri": "https://oauth2.googleapis.com/token"
	}`

	provider, err := NewGCPProvider(context.Background(), "test-bucket", []byte(validCredentials))
	if err != nil {
		t.Fatalf("Failed to create GCP provider: %v", err)
	}

	// Test Close method
	err = provider.Close()
	if err != nil {
		t.Errorf("Unexpected error closing provider: %v", err)
	}

	// Test Close on nil client
	provider.client = nil
	err = provider.Close()
	if err != nil {
		t.Errorf("Unexpected error closing provider with nil client: %v", err)
	}
}

package bucket_test

import (
	"os"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/bucket"
	"github.com/openshift/oadp-operator/pkg/credentials/stsflow"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		bucket  oadpv1alpha1.CloudStorage
		wantErr bool
		want    bool
	}{
		{
			name: "Test AWS",
			bucket: oadpv1alpha1.CloudStorage{
				Spec: oadpv1alpha1.CloudStorageSpec{
					Provider: oadpv1alpha1.AWSBucketProvider,
				},
			},
			wantErr: false,
			want:    true,
		},
		{
			name: "Error when invalid provider",
			bucket: oadpv1alpha1.CloudStorage{
				Spec: oadpv1alpha1.CloudStorageSpec{
					Provider: "invalid",
				},
			},
			wantErr: true,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bclnt, err := bucket.NewClient(tt.bucket, nil)

			if (err != nil) != tt.wantErr {
				t.Errorf("wanted err: %v but did not get one want err: %v", err, tt.wantErr)
				return
			}
			if (bclnt != nil) != tt.want {
				t.Errorf("want: %v but did got: %#v", tt.want, bclnt)
				return
			}
		})
	}
}

func TestSharedCredentialsFileFromSecret(t *testing.T) {
	tests := []struct {
		name            string
		secret          *corev1.Secret
		wantErr         bool
		errContains     string
		filePrefix      string
		validateContent func(t *testing.T, content []byte)
	}{
		{
			name: "AWS credentials",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-aws-secret",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					stsflow.AWSSecretCredentialsKey: []byte(`[default]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`),
				},
			},
			wantErr:    false,
			filePrefix: "cloud-credentials-aws-",
			validateContent: func(t *testing.T, content []byte) {
				if !strings.Contains(string(content), "aws_access_key_id") {
					t.Errorf("Expected AWS credentials content, got: %s", string(content))
				}
			},
		},
		{
			name: "GCP service account JSON",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gcp-secret",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					stsflow.GcpSecretJSONKey: []byte(`{
  "type": "service_account",
  "project_id": "test-project",
  "private_key_id": "key-id",
  "private_key": "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----\n",
  "client_email": "test@test-project.iam.gserviceaccount.com",
  "client_id": "123456789",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token"
}`),
				},
			},
			wantErr:    false,
			filePrefix: "cloud-credentials-gcp-",
			validateContent: func(t *testing.T, content []byte) {
				if !strings.Contains(string(content), "service_account") {
					t.Errorf("Expected GCP service account content, got: %s", string(content))
				}
			},
		},
		{
			name: "Empty credentials data for AWS",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-empty-aws",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					stsflow.AWSSecretCredentialsKey: {},
				},
			},
			wantErr:     true,
			errContains: "invalid secret",
		},
		{
			name: "Empty service account data for GCP",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-empty-gcp",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					stsflow.GcpSecretJSONKey: {},
				},
			},
			wantErr:     true,
			errContains: "invalid secret",
		},
		{
			name: "No recognized keys",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-invalid",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"unknown-key": []byte("some data"),
				},
			},
			wantErr:     true,
			errContains: "invalid secret: missing credentials key (for AWS) or service_account.json key (for GCP)",
		},
		{
			name: "Both AWS and GCP keys present (AWS takes precedence)",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-both",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					stsflow.AWSSecretCredentialsKey: []byte("aws-creds"),
					stsflow.GcpSecretJSONKey:        []byte("gcp-creds"),
				},
			},
			wantErr:    false,
			filePrefix: "cloud-credentials-aws-",
			validateContent: func(t *testing.T, content []byte) {
				if string(content) != "aws-creds" {
					t.Errorf("Expected AWS credentials to take precedence, got: %s", string(content))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename, err := bucket.SharedCredentialsFileFromSecret(tt.secret)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Verify file was created
			if _, err := os.Stat(filename); os.IsNotExist(err) {
				t.Errorf("Expected file to be created at %s, but it doesn't exist", filename)
				return
			}

			// Verify file prefix
			if tt.filePrefix != "" && !strings.Contains(filename, tt.filePrefix) {
				t.Errorf("Expected filename to contain prefix '%s', got: %s", tt.filePrefix, filename)
			}

			// Read and validate file content
			content, err := os.ReadFile(filename)
			if err != nil {
				t.Errorf("Failed to read created file: %v", err)
			} else if tt.validateContent != nil {
				tt.validateContent(t, content)
			}

			// Clean up
			os.Remove(filename)
		})
	}
}

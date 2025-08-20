package bucket

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"

	"github.com/openshift/oadp-operator/api/v1alpha1"
)

func TestValidateBucketName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"valid name", "my-bucket-123", false},
		{"valid with dots", "my.bucket.123", false},
		{"valid with underscores", "my_bucket_123", false},
		{"too short", "ab", true},
		{"too long", strings.Repeat("a", 64), true},
		{"starts with number", "123bucket", false},
		{"ends with hyphen", "bucket-", true},
		{"consecutive dots", "bucket..name", true},
		{"ip address format", "192.168.1.1", true},
		{"uppercase letters", "MyBucket", true},
		{"special characters", "bucket@name", true},
		{"starts with hyphen", "-bucket", true},
		{"ends with dot", "bucket.", true},
		{"valid minimum length", "abc", false},
		{"valid maximum length", strings.Repeat("a", 63), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBucketName(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExtractProjectID(t *testing.T) {
	// Create temporary service account file
	sa := serviceAccountKey{
		Type:        "service_account",
		ProjectID:   "test-project-123",
		ClientEmail: "test@test-project-123.iam.gserviceaccount.com",
		ClientID:    "123456789",
		PrivateKey:  "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----\n",
		AuthURI:     "https://accounts.google.com/o/oauth2/auth",
		TokenURI:    "https://oauth2.googleapis.com/token",
	}

	data, err := json.Marshal(sa)
	require.NoError(t, err)

	tmpFile, err := os.CreateTemp("", "test-sa-*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(data)
	require.NoError(t, err)
	tmpFile.Close()

	client := gcpBucketClient{}
	projectID, err := client.extractProjectID(tmpFile.Name())

	assert.NoError(t, err)
	assert.Equal(t, "test-project-123", projectID)

	// Test missing project ID
	saNoProject := serviceAccountKey{
		Type:        "service_account",
		ClientEmail: "test@test-project-123.iam.gserviceaccount.com",
	}

	data, err = json.Marshal(saNoProject)
	require.NoError(t, err)

	tmpFile2, err := os.CreateTemp("", "test-sa-no-project-*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile2.Name())

	_, err = tmpFile2.Write(data)
	require.NoError(t, err)
	tmpFile2.Close()

	_, err = client.extractProjectID(tmpFile2.Name())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project_id not found")

	// Test invalid JSON
	tmpFile3, err := os.CreateTemp("", "test-sa-invalid-*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile3.Name())

	_, err = tmpFile3.Write([]byte("invalid json"))
	require.NoError(t, err)
	tmpFile3.Close()

	_, err = client.extractProjectID(tmpFile3.Name())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error parsing credential JSON")

	// Test non-existent file
	_, err = client.extractProjectID("/non/existent/file.json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error reading credential file")
}

func TestConvertTagsToLabels(t *testing.T) {
	tests := []struct {
		name     string
		tags     map[string]string
		expected map[string]string
	}{
		{
			name: "normal tags",
			tags: map[string]string{
				"environment": "production",
				"team":        "backend",
			},
			expected: map[string]string{
				"environment": "production",
				"team":        "backend",
			},
		},
		{
			name: "tags with uppercase and special characters",
			tags: map[string]string{
				"Environment":   "Production",
				"Team-Name":     "Backend_API",
				"Cost.Center":   "IT-Ops",
				"Project@2024":  "MyProject",
				"Version#1.2.3": "Release",
			},
			expected: map[string]string{
				"environment":   "production",
				"team-name":     "backend_api",
				"cost_center":   "it-ops",
				"project_2024":  "myproject",
				"version_1_2_3": "release",
			},
		},
		{
			name: "tags with long keys and values",
			tags: map[string]string{
				strings.Repeat("a", 70): strings.Repeat("b", 70),
			},
			expected: map[string]string{
				strings.Repeat("a", 63): strings.Repeat("b", 63),
			},
		},
		{
			name:     "empty tags",
			tags:     map[string]string{},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := gcpBucketClient{
				bucket: v1alpha1.CloudStorage{
					Spec: v1alpha1.CloudStorageSpec{
						Tags: tt.tags,
					},
				},
			}

			labels := client.convertTagsToLabels()
			assert.Equal(t, tt.expected, labels)
		})
	}
}

func TestGetGCPLocation(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"us-central1", "us-central1"},         // GCP region
		{"europe-west1", "europe-west1"},       // GCP region
		{"asia-southeast1", "asia-southeast1"}, // GCP region
		{"", "us-central1"},                    // Default
		{"any-region", "any-region"},           // Pass through
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			client := gcpBucketClient{
				bucket: v1alpha1.CloudStorage{
					Spec: v1alpha1.CloudStorageSpec{
						Region: tt.input,
					},
				},
			}
			result := client.getGCPLocation()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsGCSRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name: "rate limit error",
			err: &googleapi.Error{
				Code:    429,
				Message: "Too Many Requests",
			},
			retryable: true,
		},
		{
			name: "server error 500",
			err: &googleapi.Error{
				Code:    500,
				Message: "Internal Server Error",
			},
			retryable: true,
		},
		{
			name: "server error 503",
			err: &googleapi.Error{
				Code:    503,
				Message: "Service Unavailable",
			},
			retryable: true,
		},
		{
			name: "gateway timeout",
			err: &googleapi.Error{
				Code:    504,
				Message: "Gateway Timeout",
			},
			retryable: true,
		},
		{
			name: "authentication failed",
			err: &googleapi.Error{
				Code:    401,
				Message: "Unauthenticated",
			},
			retryable: false,
		},
		{
			name: "permission denied",
			err: &googleapi.Error{
				Code:    403,
				Message: "Permission Denied",
			},
			retryable: false,
		},
		{
			name: "bad request",
			err: &googleapi.Error{
				Code:    400,
				Message: "Bad Request",
			},
			retryable: false,
		},
		{
			name: "not found",
			err: &googleapi.Error{
				Code:    404,
				Message: "Not Found",
			},
			retryable: false,
		},
		{
			name: "conflict",
			err: &googleapi.Error{
				Code:    409,
				Message: "Conflict",
			},
			retryable: false,
		},
		{
			name:      "timeout error string",
			err:       assert.AnError,
			retryable: false, // Our mock error doesn't contain "timeout"
		},
		{
			name:      "nil error",
			err:       nil,
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGCSRetryableError(tt.err)
			assert.Equal(t, tt.retryable, result)
		})
	}
}

func TestHandleGCSError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		operation     string
		bucketName    string
		expectedError string
		shouldBeNil   bool
	}{
		{
			name:        "bucket already exists on create",
			err:         &googleapi.Error{Code: 409, Message: "Bucket already exists"},
			operation:   "create",
			bucketName:  "test-bucket",
			shouldBeNil: true,
		},
		{
			name:        "bucket not found on delete",
			err:         &googleapi.Error{Code: 404, Message: "Not Found"},
			operation:   "delete",
			bucketName:  "test-bucket",
			shouldBeNil: true,
		},
		{
			name:        "bucket not found on exists",
			err:         &googleapi.Error{Code: 404, Message: "Not Found"},
			operation:   "exists",
			bucketName:  "test-bucket",
			shouldBeNil: true,
		},
		{
			name:          "authentication failed",
			err:           &googleapi.Error{Code: 401, Message: "Unauthenticated"},
			operation:     "create",
			bucketName:    "test-bucket",
			expectedError: "authentication failed: check service account key",
		},
		{
			name:          "permission denied",
			err:           &googleapi.Error{Code: 403, Message: "Permission Denied"},
			operation:     "create",
			bucketName:    "test-bucket",
			expectedError: "permission denied: check service account permissions for project",
		},
		{
			name:          "rate limit exceeded",
			err:           &googleapi.Error{Code: 429, Message: "Too Many Requests"},
			operation:     "create",
			bucketName:    "test-bucket",
			expectedError: "rate limit exceeded: too many requests",
		},
		{
			name:          "bad request",
			err:           &googleapi.Error{Code: 400, Message: "Bad Request"},
			operation:     "create",
			bucketName:    "test-bucket",
			expectedError: "invalid request: check bucket name and configuration",
		},
		{
			name:          "other error",
			err:           &googleapi.Error{Code: 500, Message: "Internal Server Error"},
			operation:     "create",
			bucketName:    "test-bucket",
			expectedError: "gcs error during create: Internal Server Error (HTTP 500)",
		},
		{
			name:          "conflict on non-create operation",
			err:           &googleapi.Error{Code: 409, Message: "Conflict"},
			operation:     "delete",
			bucketName:    "test-bucket",
			expectedError: "bucket 'test-bucket' already exists globally",
		},
		{
			name:          "generic error",
			err:           assert.AnError,
			operation:     "create",
			bucketName:    "test-bucket",
			expectedError: "failed to create bucket 'test-bucket':",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleGCSError(tt.err, tt.operation, tt.bucketName)
			if tt.shouldBeNil {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestGCSRetryConfig(t *testing.T) {
	// Test that the default retry config is properly set
	assert.Equal(t, 5, defaultGCSRetryConfig.maxRetries)
	assert.Equal(t, float64(1), defaultGCSRetryConfig.initialDelay.Seconds())
	assert.Equal(t, float64(32), defaultGCSRetryConfig.maxDelay.Seconds())
}

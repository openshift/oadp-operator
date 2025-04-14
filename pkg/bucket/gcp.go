package bucket

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/oadp-operator/api/v1alpha1"
)

type gcpBucketClient struct {
	bucket v1alpha1.CloudStorage
	client client.Client
}

func (g gcpBucketClient) Exists() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gcsClient, _, err := g.getGCSClient()
	if err != nil {
		return false, err
	}
	defer gcsClient.Close()

	bucket := gcsClient.Bucket(g.bucket.Spec.Name)
	_, err = bucket.Attrs(ctx)
	if err != nil {
		if err == storage.ErrBucketNotExist {
			return false, nil
		}
		// Return true for permission errors - unable to determine if bucket exists
		return true, fmt.Errorf("unable to determine bucket %v status: %v", g.bucket.Spec.Name, err)
	}

	// Tag bucket if it exists
	err = g.tagBucket(gcsClient)
	if err != nil {
		return true, err
	}

	return true, nil
}

func (g gcpBucketClient) Create() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Validate bucket name
	if err := validateBucketName(g.bucket.Spec.Name); err != nil {
		return false, err
	}

	gcsClient, projectID, err := g.getGCSClient()
	if err != nil {
		return false, err
	}
	defer gcsClient.Close()

	bucket := gcsClient.Bucket(g.bucket.Spec.Name)

	// Prepare bucket attributes
	attrs := &storage.BucketAttrs{
		Location: g.getGCPLocation(),
		Labels:   g.convertTagsToLabels(),
	}

	// Set storage class if configured
	if g.bucket.Spec.Config != nil {
		if storageClass, ok := g.bucket.Spec.Config["storageClass"]; ok && storageClass != "" {
			attrs.StorageClass = storageClass
		}
	}

	// Create bucket with retry logic
	err = withGCSRetry(func() error {
		return bucket.Create(ctx, projectID, attrs)
	}, defaultGCSRetryConfig)

	if err != nil {
		err = handleGCSError(err, "create", g.bucket.Spec.Name)
		if err == nil {
			// Bucket already exists - idempotent behavior
			return true, nil
		}
		return false, err
	}

	return true, nil
}

func (g gcpBucketClient) Delete() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second) // Longer timeout for object deletion
	defer cancel()

	gcsClient, _, err := g.getGCSClient()
	if err != nil {
		return false, err
	}
	defer gcsClient.Close()

	bucket := gcsClient.Bucket(g.bucket.Spec.Name)

	// Check if bucket exists first
	_, err = bucket.Attrs(ctx)
	if err != nil {
		if err == storage.ErrBucketNotExist {
			// Bucket doesn't exist - idempotent behavior
			return true, nil
		}
		return false, err
	}

	// Delete all objects in bucket (GCS requires empty bucket)
	err = g.deleteObjectsConcurrently(ctx, bucket)
	if err != nil {
		return false, err
	}

	// Delete the bucket with retry logic
	err = withGCSRetry(func() error {
		return bucket.Delete(ctx)
	}, defaultGCSRetryConfig)

	if err != nil {
		err = handleGCSError(err, "delete", g.bucket.Spec.Name)
		if err == nil {
			// Bucket already deleted - idempotent behavior
			return true, nil
		}
		return false, err
	}

	return true, nil
}

// getGCSClient creates a GCS client with authentication
// Supports both traditional service account keys and GCP Workload Identity Federation (WIF)
func (g gcpBucketClient) getGCSClient() (*storage.Client, string, error) {
	ctx := context.Background()

	// Get credential file from secret
	// This could be either a service account key or WIF external account credentials
	credFile, err := getCredentialFromCloudStorageSecret(g.client, g.bucket)
	if err != nil {
		return nil, "", err
	}

	// Parse credentials to extract project ID
	// Handles both service account and WIF credential formats
	projectID, err := g.extractProjectID(credFile)
	if err != nil {
		return nil, "", err
	}

	// Create GCS client with credentials
	// The GCS client automatically handles both service account and WIF credentials
	gcsClient, err := storage.NewClient(ctx, option.WithCredentialsFile(credFile))
	if err != nil {
		return nil, "", err
	}

	return gcsClient, projectID, nil
}

// serviceAccountKey represents the structure of a service account JSON file
type serviceAccountKey struct {
	Type         string `json:"type"`
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	ClientID     string `json:"client_id"`
	AuthURI      string `json:"auth_uri"`
	TokenURI     string `json:"token_uri"`
}

// externalAccountKey represents the structure of a GCP WIF external account JSON file
type externalAccountKey struct {
	Type                           string `json:"type"`
	Audience                       string `json:"audience"`
	SubjectTokenType               string `json:"subject_token_type"`
	TokenURL                       string `json:"token_url"`
	ServiceAccountImpersonationURL string `json:"service_account_impersonation_url"`
	CredentialSource               struct {
		File   string `json:"file"`
		Format struct {
			Type string `json:"type"`
		} `json:"format"`
	} `json:"credential_source"`
}

// extractProjectID parses the credential JSON file to extract project ID
func (g gcpBucketClient) extractProjectID(credentialFile string) (string, error) {
	data, err := os.ReadFile(credentialFile)
	if err != nil {
		return "", fmt.Errorf("error reading credential file: %v", err)
	}

	// First, check the type of credentials
	var typeCheck struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeCheck); err != nil {
		return "", fmt.Errorf("error parsing credential JSON: %v", err)
	}

	switch typeCheck.Type {
	case "service_account":
		// Traditional service account key
		var sa serviceAccountKey
		if err := json.Unmarshal(data, &sa); err != nil {
			return "", fmt.Errorf("error parsing service account JSON: %v", err)
		}
		if sa.ProjectID == "" {
			return "", fmt.Errorf("project_id not found in service account key")
		}
		return sa.ProjectID, nil

	case "external_account":
		// GCP WIF credentials - need to extract project ID from service account email
		var ea externalAccountKey
		if err := json.Unmarshal(data, &ea); err != nil {
			return "", fmt.Errorf("error parsing external account JSON: %v", err)
		}

		// Extract project ID from the service account impersonation URL
		// URL format: https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/SERVICE_ACCOUNT_EMAIL:generateAccessToken
		// Where SERVICE_ACCOUNT_EMAIL is like: my-sa@PROJECT_ID.iam.gserviceaccount.com
		projectID, err := g.extractProjectIDFromWIF(ea.ServiceAccountImpersonationURL)
		if err != nil {
			// If we can't extract from URL, check if project ID is in config
			if g.bucket.Spec.Config != nil {
				if pid, ok := g.bucket.Spec.Config["projectID"]; ok && pid != "" {
					return pid, nil
				}
			}
			// Try environment variable as last resort
			if pid := os.Getenv("GOOGLE_CLOUD_PROJECT"); pid != "" {
				return pid, nil
			}
			if pid := os.Getenv("GCP_PROJECT"); pid != "" {
				return pid, nil
			}
			return "", fmt.Errorf("unable to determine project ID from WIF credentials: %v", err)
		}
		return projectID, nil

	default:
		return "", fmt.Errorf("unsupported credential type: %s", typeCheck.Type)
	}
}

// extractProjectIDFromWIF extracts project ID from WIF service account impersonation URL
func (g gcpBucketClient) extractProjectIDFromWIF(impersonationURL string) (string, error) {
	// Extract service account email from URL
	// URL format: https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/SERVICE_ACCOUNT_EMAIL:generateAccessToken

	prefix := "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/"
	suffix := ":generateAccessToken"

	if !strings.HasPrefix(impersonationURL, prefix) || !strings.HasSuffix(impersonationURL, suffix) {
		return "", fmt.Errorf("invalid service account impersonation URL format")
	}

	// Extract the service account email
	start := len(prefix)
	end := len(impersonationURL) - len(suffix)
	if start >= end {
		return "", fmt.Errorf("invalid service account impersonation URL")
	}

	serviceAccountEmail := impersonationURL[start:end]

	// Extract project ID from service account email
	// Format: service-account-name@PROJECT_ID.iam.gserviceaccount.com
	parts := strings.Split(serviceAccountEmail, "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid service account email format")
	}

	domainParts := strings.Split(parts[1], ".")
	if len(domainParts) < 3 || domainParts[1] != "iam" || domainParts[2] != "gserviceaccount" {
		return "", fmt.Errorf("invalid service account email domain")
	}

	return domainParts[0], nil
}

// convertTagsToLabels converts CloudStorage tags to GCS labels
func (g gcpBucketClient) convertTagsToLabels() map[string]string {
	labels := make(map[string]string)

	for key, value := range g.bucket.Spec.Tags {
		// Convert to lowercase and replace invalid characters
		labelKey := strings.ToLower(key)
		labelKey = regexp.MustCompile(`[^a-z0-9_-]`).ReplaceAllString(labelKey, "_")

		labelValue := strings.ToLower(value)
		labelValue = regexp.MustCompile(`[^a-z0-9_-]`).ReplaceAllString(labelValue, "_")

		// Truncate to GCP limits
		if len(labelKey) > 63 {
			labelKey = labelKey[:63]
		}
		if len(labelValue) > 63 {
			labelValue = labelValue[:63]
		}

		labels[labelKey] = labelValue
	}

	return labels
}

// tagBucket applies tags to an existing bucket
func (g gcpBucketClient) tagBucket(gcsClient *storage.Client) error {
	ctx := context.Background()
	bucket := gcsClient.Bucket(g.bucket.Spec.Name)

	// Update labels
	newLabels := g.convertTagsToLabels()

	// Update bucket with new labels
	attrsToUpdate := storage.BucketAttrsToUpdate{}
	for key, value := range newLabels {
		attrsToUpdate.SetLabel(key, value)
	}

	_, err := bucket.Update(ctx, attrsToUpdate)
	if err != nil {
		return fmt.Errorf("error updating bucket labels: %v", err)
	}

	return nil
}

// validateBucketName validates GCS bucket naming rules
func validateBucketName(name string) error {
	if len(name) < 3 || len(name) > 63 {
		return fmt.Errorf("bucket name must be between 3 and 63 characters")
	}

	// Must start and end with letter or number
	if !regexp.MustCompile(`^[a-z0-9].*[a-z0-9]$`).MatchString(name) {
		return fmt.Errorf("bucket name must start and end with a letter or number")
	}

	// Can contain lowercase letters, numbers, hyphens, underscores, and dots
	if !regexp.MustCompile(`^[a-z0-9._-]+$`).MatchString(name) {
		return fmt.Errorf("bucket name can only contain lowercase letters, numbers, hyphens, underscores, and dots")
	}

	// Cannot have consecutive dots
	if strings.Contains(name, "..") {
		return fmt.Errorf("bucket name cannot contain consecutive dots")
	}

	// Cannot be IP address format
	if regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`).MatchString(name) {
		return fmt.Errorf("bucket name cannot be in IP address format")
	}

	return nil
}

// getGCPLocation returns the GCP location for bucket creation
func (g gcpBucketClient) getGCPLocation() string {
	region := g.bucket.Spec.Region
	if region == "" {
		return "us-central1" // Default GCP region
	}
	return region
}

// deleteObjectsConcurrently deletes objects in parallel for faster bucket cleanup
func (g gcpBucketClient) deleteObjectsConcurrently(ctx context.Context, bucket *storage.BucketHandle) error {
	const maxWorkers = 10
	const batchSize = 100

	objects := make(chan string, batchSize)
	errors := make(chan error, maxWorkers)

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		go func() {
			for objName := range objects {
				if err := bucket.Object(objName).Delete(ctx); err != nil {
					errors <- fmt.Errorf("error deleting object %s: %v", objName, err)
					return
				}
			}
			errors <- nil
		}()
	}

	// List and queue objects for deletion
	it := bucket.Objects(ctx, nil)
	go func() {
		defer close(objects)
		for {
			objAttrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				errors <- fmt.Errorf("error listing objects: %v", err)
				return
			}
			objects <- objAttrs.Name
		}
	}()

	// Wait for all workers to complete
	for i := 0; i < maxWorkers; i++ {
		if err := <-errors; err != nil {
			return err
		}
	}

	return nil
}

// gcsRetryConfig defines retry behavior for GCS operations
type gcsRetryConfig struct {
	maxRetries   int
	initialDelay time.Duration
	maxDelay     time.Duration
}

var defaultGCSRetryConfig = gcsRetryConfig{
	maxRetries:   5,
	initialDelay: 1 * time.Second,
	maxDelay:     32 * time.Second,
}

// withGCSRetry executes a function with exponential backoff retry logic
func withGCSRetry(operation func() error, config gcsRetryConfig) error {
	var lastErr error
	delay := config.initialDelay

	for attempt := 0; attempt <= config.maxRetries; attempt++ {
		if attempt > 0 {
			// Add jitter to prevent thundering herd
			jitter := time.Duration(rand.Int63n(int64(delay / 4)))
			time.Sleep(delay + jitter)

			// Exponential backoff
			delay = time.Duration(float64(delay) * 2.0)
			if delay > config.maxDelay {
				delay = config.maxDelay
			}
		}

		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isGCSRetryableError(err) {
			return err
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", config.maxRetries, lastErr)
}

// isGCSRetryableError determines if a GCS error should be retried
func isGCSRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if gerr, ok := err.(*googleapi.Error); ok {
		switch gerr.Code {
		case http.StatusTooManyRequests: // Too Many Requests
			return true
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout: // Server errors
			return true
		case http.StatusUnauthorized, http.StatusForbidden: // Authentication/Authorization failures
			return false
		case http.StatusBadRequest: // Bad Request (invalid names, etc.)
			return false
		case http.StatusNotFound: // Not Found
			return false
		case http.StatusConflict: // Conflict (bucket exists)
			return false
		}
	}

	// Network-related errors are typically retryable
	if strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "connection") ||
		strings.Contains(err.Error(), "network") {
		return true
	}

	return false
}

// handleGCSError provides consistent error handling for GCS operations
func handleGCSError(err error, operation string, bucketName string) error {
	if err == storage.ErrBucketNotExist {
		if operation == "exists" || operation == "delete" {
			return nil // Expected for these operations
		}
		return fmt.Errorf("bucket '%s' not found", bucketName)
	}

	if gerr, ok := err.(*googleapi.Error); ok {
		switch gerr.Code {
		case http.StatusConflict: // Conflict - Bucket already exists
			if operation == "create" {
				return nil // Idempotent behavior
			}
			return fmt.Errorf("bucket '%s' already exists globally", bucketName)
		case http.StatusNotFound: // Not Found
			if operation == "exists" || operation == "delete" {
				return nil // Expected for these operations
			}
			return fmt.Errorf("bucket '%s' not found", bucketName)
		case http.StatusUnauthorized: // Unauthenticated
			return fmt.Errorf("authentication failed: check service account key")
		case http.StatusForbidden: // Permission Denied
			return fmt.Errorf("permission denied: check service account permissions for project")
		case http.StatusTooManyRequests: // Too Many Requests
			return fmt.Errorf("rate limit exceeded: too many requests")
		case http.StatusBadRequest: // Bad Request
			return fmt.Errorf("invalid request: check bucket name and configuration")
		default:
			return fmt.Errorf("gcs error during %s: %s (HTTP %d): %v",
				operation, gerr.Message, gerr.Code, gerr)
		}
	}

	return fmt.Errorf("failed to %s bucket '%s': %w", operation, bucketName, err)
}

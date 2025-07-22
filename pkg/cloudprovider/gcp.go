package cloudprovider

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/storage"
	"github.com/go-logr/logr"
	"google.golang.org/api/option"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/utils"
)

type GCPProvider struct {
	client *storage.Client
	bucket string
}

// NewGCPProvider creates a GCPProvider using service account credentials
func NewGCPProvider(ctx context.Context, bucket string, credentialsJSON []byte) (*GCPProvider, error) {
	client, err := storage.NewClient(ctx, option.WithCredentialsJSON(credentialsJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create GCP storage client: %w", err)
	}

	return &GCPProvider{
		client: client,
		bucket: bucket,
	}, nil
}

// UploadTest performs a test upload and returns calculated speed and test duration
func (g *GCPProvider) UploadTest(ctx context.Context, config oadpv1alpha1.UploadSpeedTestConfig, bucket string, log logr.Logger) (int64, time.Duration, error) {
	log.Info("Starting GCP upload speed test", "fileSize", config.FileSize, "timeout", config.Timeout.Duration.String())

	testDataBytes, err := utils.ParseFileSize(config.FileSize)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse file size: %w", err)
	}

	// Validate file size doesn't exceed max limit
	if testDataBytes > 200*1024*1024 {
		return 0, 0, fmt.Errorf("test file size %d exceeds maximum allowed size %d", testDataBytes, 200*1024*1024)
	}

	// Create test data
	testData := make([]byte, testDataBytes)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Create a unique object name for the test
	objectName := fmt.Sprintf("dpt-upload-test-%d", time.Now().UnixNano())

	// Create upload context with timeout
	timeoutDuration := 30 * time.Second
	if config.Timeout.Duration != 0 {
		timeoutDuration = config.Timeout.Duration
	}
	uploadCtx, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()

	// Perform the upload and measure duration
	start := time.Now()

	bh := g.client.Bucket(bucket)
	obj := bh.Object(objectName)

	// Create writer
	w := obj.NewWriter(uploadCtx)
	w.ContentType = "application/octet-stream"

	// Write test data
	bytesWritten, err := w.Write(testData)
	if err != nil {
		w.Close()
		return 0, 0, fmt.Errorf("failed to write test data: %w", err)
	}

	// Close writer to complete upload
	if err := w.Close(); err != nil {
		return 0, 0, fmt.Errorf("failed to close writer: %w", err)
	}

	duration := time.Since(start)

	// Calculate speed in Mbps
	speedMbps := int64(float64(bytesWritten*8) / duration.Seconds() / 1000000)

	log.Info("GCP upload test completed", "bytesWritten", bytesWritten, "duration", duration.String())

	return speedMbps, duration, nil
}

// GetBucketMetadata retrieves the encryption and versioning config for a bucket
func (g *GCPProvider) GetBucketMetadata(ctx context.Context, bucket string, log logr.Logger) (*oadpv1alpha1.BucketMetadata, error) {
	log.Info("Retrieving GCP bucket metadata", "bucket", bucket)

	bh := g.client.Bucket(bucket)

	// Get bucket attributes
	attrs, err := bh.Attrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket attributes: %w", err)
	}

	metadata := &oadpv1alpha1.BucketMetadata{}

	// Determine encryption algorithm
	if attrs.Encryption != nil && attrs.Encryption.DefaultKMSKeyName != "" {
		metadata.EncryptionAlgorithm = "google-kms"
	} else {
		// GCS always encrypts data at rest using Google-managed keys by default
		metadata.EncryptionAlgorithm = "google-managed"
	}

	// Determine versioning status
	if attrs.VersioningEnabled {
		metadata.VersioningStatus = "Enabled"
	} else {
		metadata.VersioningStatus = "Suspended"
	}

	log.Info("Successfully retrieved GCP bucket metadata",
		"encryption", metadata.EncryptionAlgorithm,
		"versioning", metadata.VersioningStatus)

	return metadata, nil
}

// Close closes the GCP client
func (g *GCPProvider) Close() error {
	if g.client != nil {
		return g.client.Close()
	}
	return nil
}

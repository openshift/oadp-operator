package cloudprovider

import (
	"bytes"
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/utils"
)

const maxTestSizeBytes = 200 * 1024 * 1024

type AWSProvider struct {
	s3Client *s3.S3
}

// NewAWSProvider creates an AWSProvider using region, endpoint, and credentials.
func NewAWSProvider(region, endpoint, accessKey, secretKey string) *AWSProvider {
	awsConfig := &aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
	}

	// Optional custom S3-compatible endpoint (e.g., MinIO, Ceph)
	if endpoint != "" {
		awsConfig.Endpoint = aws.String(endpoint)
		awsConfig.S3ForcePathStyle = aws.Bool(true)
	}

	sess := session.Must(session.NewSession(awsConfig))
	s3Client := s3.New(sess)
	return &AWSProvider{
		s3Client: s3Client,
	}
}

func (a *AWSProvider) UploadTest(ctx context.Context, config oadpv1alpha1.UploadSpeedTestConfig, bucket string, log logr.Logger) (int64, time.Duration, error) {

	log.Info("Starting upload speed test", "fileSize", config.FileSize, "timeout", config.Timeout.Duration.String())

	testDataBytes, err := utils.ParseFileSize(config.FileSize)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid file size: %w", err)
	}

	if testDataBytes > maxTestSizeBytes {
		return 0, 0, fmt.Errorf("test file size %d exceeds max allowed %dMB (due to pod mem limit)", testDataBytes, maxTestSizeBytes/1024/1024)
	}

	timeoutDuration := 30 * time.Second
	if config.Timeout.Duration != 0 {
		timeoutDuration = config.Timeout.Duration
	}

	log.Info("Generating test payload for upload", "bytes", testDataBytes)
	payload := bytes.Repeat([]byte("0"), int(testDataBytes))

	key := fmt.Sprintf("dpt-upload-test-%d", time.Now().UnixNano())

	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()

	log.Info("Uploading to bucket...")
	start := time.Now()

	_, err = a.s3Client.PutObjectWithContext(ctxWithTimeout, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(payload),
	})

	duration := time.Since(start)

	if err != nil {
		return 0, duration, fmt.Errorf("upload failed: %w", err)
	}

	speedMbps := (float64(testDataBytes*8) / duration.Seconds()) / 1_000_000
	log.Info("Upload completed", "duration", duration.String(), "speedMbps", speedMbps)

	return int64(speedMbps), duration, nil
}

// GetBucketMetadata queries AWS S3 for bucket versioning and encryption settings.
// It returns a BucketMetadata struct containing this information.
func (a *AWSProvider) GetBucketMetadata(ctx context.Context, bucket string, log logr.Logger) (*oadpv1alpha1.BucketMetadata, error) {
	result := &oadpv1alpha1.BucketMetadata{}

	verOut, err := a.s3Client.GetBucketVersioningWithContext(ctx, &s3.GetBucketVersioningInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		log.Error(err, "GetBucketVersioningWithContext failed")
		result.ErrorMessage = fmt.Sprintf("failed to fetch versioning status for bucket %s: %v", bucket, err)
		return result, err
	}
	if verOut.Status != nil {
		result.VersioningStatus = *verOut.Status
	} else {
		result.VersioningStatus = "None"
	}

	log.Info("Fetched versioning", "status", result.VersioningStatus)

	encOut, err := a.s3Client.GetBucketEncryptionWithContext(ctx, &s3.GetBucketEncryptionInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		// Handle cases where encryption is not enabled
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "ServerSideEncryptionConfigurationNotFoundError" {
			result.EncryptionAlgorithm = "None"
			log.Info("Bucket encryption not configured")
		} else {
			log.Error(err, "GetBucketEncryptionWithContext failed")
			result.ErrorMessage = fmt.Sprintf("failed to fetch encryption config for bucket %s: %v", bucket, err)
			return result, err
		}
	} else if encOut != nil && encOut.ServerSideEncryptionConfiguration != nil && len(encOut.ServerSideEncryptionConfiguration.Rules) > 0 {
		rule := encOut.ServerSideEncryptionConfiguration.Rules[0]
		if rule.ApplyServerSideEncryptionByDefault != nil {
			result.EncryptionAlgorithm = *rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm
		} else {
			result.EncryptionAlgorithm = "Unknown"
		}
		log.Info("Fetched encryption config", "algorithm", result.EncryptionAlgorithm)
	} else {
		result.EncryptionAlgorithm = "None"
		log.Info("No encryption rules configured")
	}

	return result, nil
}

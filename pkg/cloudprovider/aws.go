package cloudprovider

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/openshift/oadp-operator/pkg/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
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

func (a *AWSProvider) UploadTest(ctx context.Context, config oadpv1alpha1.UploadSpeedTestConfig, bucket string) (int64, time.Duration, error) {
	testDataBytes, err := utils.ParseFileSize(config.FileSize)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid file size: %w", err)
	}


	if testDataBytes > maxTestSizeBytes {
		return 0, 0, fmt.Errorf("test file size %d exceeds max allowed size %dMB (pod mem: 512Mi)", testDataBytes, maxTestSizeBytes/1024/1024)
	}


	timeoutDuration := 30 * time.Second
	if config.TestTimeout != "" {
		if d, err := time.ParseDuration(config.TestTimeout); err == nil {
			timeoutDuration = d
		}
	}

	payload := bytes.Repeat([]byte("0"), int(testDataBytes))


	key := fmt.Sprintf("dpt-upload-test-%d", time.Now().UnixNano())

	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()

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

	speedmbps := (float64(testDataBytes*8) / duration.Seconds()) / 1_000_000

	return int64(speedmbps), duration, nil
}

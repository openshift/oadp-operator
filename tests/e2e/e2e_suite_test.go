package e2e

import (
	"flag"
	"log"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// Common vars obtained from flags passed in ginkgo.
var credentials, namespace, bucket, bucketFilePath, credSecretRef,
	instanceName, bsl_region, vsl_region, provider, bsl_profile, openshift_ci, ci_cred_file string
var timeoutMultiplier time.Duration

func init() {
	flag.StringVar(&credentials, "credentials", "", "Cloud Credentials file path location")
	flag.StringVar(&bucketFilePath, "velero_bucket", "myBucket", "AWS S3 data file path location")
	flag.StringVar(&namespace, "velero_namespace", "oadp-operator", "Velero Namespace")
	flag.StringVar(&bsl_region, "bsl_region", "us-east-1", "BSL region")
	flag.StringVar(&bsl_profile, "bsl_profile", "default", "AWS Profile for BSL")
	flag.StringVar(&vsl_region, "vsl_region", bsl_region, "VSL region")
	flag.StringVar(&provider, "provider", "aws", "BSL provider")
	flag.StringVar(&ci_cred_file, "ci_cred_file", credentials, "CI Cloud Cred File")
	flag.StringVar(&openshift_ci, "openshift_ci", "false", "ENV for tests")
	flag.StringVar(&credSecretRef, "creds_secret_ref", "cloud-credentials", "Credential secret ref for backup storage location")
	flag.StringVar(&instanceName, "velero_instance_name", "example-velero", "Velero Instance Name")
	timeoutMultiplierInput := flag.Int64("timeout_multiplier", 1, "Customize timeout multiplier from default (1)")
	timeoutMultiplier = 1
	if timeoutMultiplierInput != nil && *timeoutMultiplierInput >= 1 {
		timeoutMultiplier = time.Duration(*timeoutMultiplierInput)
	}
}

func TestOADPE2E(t *testing.T) {
	flag.Parse()
	s3Buffer, err := getJsonData(bucketFilePath)
	if err != nil {
		t.Fatalf("Error getting bucket json file: %v", err)
	}
	s3Data, err := decodeJson(s3Buffer) // Might need to change this later on to create s3 for each tests
	if err != nil {
		t.Fatalf("Error decoding json file: %v", err)
	}
	bucket = s3Data["velero-bucket-name"].(string)
	log.Println("Using velero prefix: " + veleroPrefix)
	RegisterFailHandler(Fail)
	RunSpecs(t, "OADP E2E Suite")
}

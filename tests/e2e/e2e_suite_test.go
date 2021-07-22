package e2e

import (
	"flag"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// Common vars obtained from flags passed in ginkgo.
var cloud, namespace, s3Bucket, s3BuckerFilePath, credSecretRef, instanceName string

func init() {
	flag.StringVar(&cloud, "cloud", "", "Cloud Credentials file path location")
	flag.StringVar(&s3BuckerFilePath, "s3_bucket", "myBucket", "AWS S3 data file path location")
	s3Data := decodeJson(getJsonData(s3BuckerFilePath)) // Might need to change this later on to create s3 for each tests
	s3Bucket = s3Data["velero-bucket-name"].(string)
	flag.StringVar(&namespace, "velero_namespace", "oadp-operator", "Velero Namespace")
	flag.StringVar(&credSecretRef, "creds_secret_ref", "cloud-credentials", "Credential secret ref for backup storage location")
	flag.StringVar(&instanceName, "velero_instance_name", "example-velero", "Velero Instance Name")
}

func TestOADPE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OADP E2E Suite")
}

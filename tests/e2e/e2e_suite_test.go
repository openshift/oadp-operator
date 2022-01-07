package e2e

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// Common vars obtained from flags passed in ginkgo.
var credFile, namespace, bucket, bucketFilePath, credSecretRef, instanceName, bsl_region, vsl_region, provider, bsl_profile, openshift_ci, ci_cred_file string
var timeoutMultiplier time.Duration

func init() {
	flag.StringVar(&credFile, "credentials", "", "Cloud Credentials file path location")
	flag.StringVar(&bucketFilePath, "velero_bucket", "myBucket", "AWS S3 data file path location")
	flag.StringVar(&namespace, "velero_namespace", "oadp-operator", "Velero Namespace")
	flag.StringVar(&bsl_region, "bsl_region", "us-east-1", "BSL region")
	flag.StringVar(&bsl_profile, "bsl_profile", "default", "AWS Profile for BSL")
	flag.StringVar(&vsl_region, "vsl_region", bsl_region, "VSL region")
	flag.StringVar(&provider, "provider", "aws", "BSL provider")
	flag.StringVar(&ci_cred_file, "ci_cred_file", credFile, "CI Cloud Cred File")
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
	s3Data, err := getJsonData(bucketFilePath)
	if err != nil {
		t.Fatalf("Error getting bucket json file: %v", err)
	}
	bucket = s3Data["velero-bucket-name"].(string)
	log.Println("Using velero prefix: " + veleroPrefix)
	RegisterFailHandler(Fail)
	RunSpecs(t, "OADP E2E Suite")
}

var vel *dpaCustomResource

var _ = BeforeSuite(func() {
	flag.Parse()
	s3Data, err := getJsonData(bucketFilePath)
	Expect(err).NotTo(HaveOccurred())
	bucket = s3Data["velero-bucket-name"].(string)

	vel = &dpaCustomResource{
		Namespace:     namespace,
		Bucket:        bucket,
		Provider:      provider,
		credentials:   credFile,
		credSecretRef: credSecretRef,
	}
	testSuiteInstanceName := "ts-" + instanceName
	vel.Name = testSuiteInstanceName
	// err := vel.createBsl()
	openshift_ci_bool, _ := strconv.ParseBool(openshift_ci)
	if openshift_ci_bool == true {
		switch vel.Provider {
		case "aws":
			cloudCredData, err := getCredsData(vel.credentials)
			Expect(err).NotTo(HaveOccurred())
			ciCredData, err := getCredsData(ci_cred_file)
			Expect(err).NotTo(HaveOccurred())
			cloudCredData = append(cloudCredData, []byte("\n")...)
			credData := append(cloudCredData, ciCredData...)
			vel.credentials = "/tmp/aws-credentials"
			err = putCredsData(vel.credentials, credData)
			Expect(err).NotTo(HaveOccurred())
			vel.awsConfig = dpaAwsConfig{
				BslProfile: bsl_profile,
				BslRegion:  bsl_region,
				VslRegion:  vsl_region,
			}
		case "gcp":
			cloudCredData, err := getCredsData(vel.credentials)
			Expect(err).NotTo(HaveOccurred())
			err = createCredentialsSecret(cloudCredData, namespace, "bsl-cloud-credentials-gcp")
			Expect(err).NotTo(HaveOccurred())
			vel.credentials = ci_cred_file
			vel.gcpConfig = dpaGcpConfig{
				VslRegion: vsl_region,
			}
		case "azure":
			// bsl cloud
			cloudCredData, err := getJsonData(vel.credentials)
			Expect(err).NotTo(HaveOccurred())
			vel.azureConfig = dpaAzureConfig{
				BslSubscriptionId: fmt.Sprintf("%v", cloudCredData["subscriptionId"]),
				BslResourceGroup:  fmt.Sprintf("%v", cloudCredData["resourceGroup"]),
				BslstorageAccount: fmt.Sprintf("%v", cloudCredData["storageAccount"]),
			}
			cloudCreds := getAzureCreds(cloudCredData)
			err = createCredentialsSecret(cloudCreds, namespace, "bsl-cloud-credentials-azure")
			Expect(err).NotTo(HaveOccurred())
			// ci cloud
			ciJsonData, err := getJsonData(ci_cred_file)
			Expect(err).NotTo(HaveOccurred())
			ciCreds := getAzureCreds(ciJsonData)
			vel.credentials = "/tmp/azure-credentials"
			err = putCredsData(vel.credentials, ciCreds)
			Expect(err).NotTo(HaveOccurred())
		}
	}
	credData, err := getCredsData(vel.credentials)
	Expect(err).NotTo(HaveOccurred())
	err = createCredentialsSecret(credData, namespace, credSecretRef)
	Expect(err).NotTo(HaveOccurred())
	vel.SetClient()
	Expect(doesNamespaceExist(namespace)).Should(BeTrue())
})

var _ = AfterSuite(func() {
	log.Printf("Deleting Velero CR")
	err := vel.Delete()
	Expect(err).ToNot(HaveOccurred())
	errs := deleteSecret(namespace, credSecretRef)
	Expect(errs).ToNot(HaveOccurred())
	Eventually(vel.IsDeleted(), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())
})

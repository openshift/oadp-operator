package e2e

import (
	"flag"
	"log"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

var veleroCR *veleroCustomResource

var _ = BeforeSuite(func() {
	flag.Parse()
	s3Buffer, err := getJsonData(s3BucketFilePath)
	Expect(err).NotTo(HaveOccurred())
	s3Data, err := decodeJson(s3Buffer) // Might need to change this later on to create s3 for each tests
	Expect(err).NotTo(HaveOccurred())
	s3Bucket = s3Data["velero-bucket-name"].(string)

	veleroCR = &veleroCustomResource{
		Namespace: namespace,
		Region:    region,
		Bucket:    s3Bucket,
		Provider:  provider,
	}
	testSuiteInstanceName := "ts-" + instanceName
	veleroCR.Name = testSuiteInstanceName

	veleroCR.SetClient()
	Expect(doesNamespaceExist(namespace)).Should(BeTrue())
})

var _ = AfterSuite(func() {
	log.Printf("Deleting Velero CR")
	err := veleroCR.Delete()
	Expect(err).ToNot(HaveOccurred())

	errs := deleteSecret(namespace, credSecretRef)
	Expect(errs).ToNot(HaveOccurred())
	Eventually(veleroCR.IsDeleted(), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())
})

var _ = Describe("Configuration testing for Velero Custom Resource", func() {

	var _ = BeforeEach(func() {
		credData, err := getCredsData(cloud)
		Expect(err).NotTo(HaveOccurred())

		err = createCredentialsSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())
	})

	type InstallCase struct {
		Name       string
		VeleroSpec *oadpv1alpha1.VeleroSpec
		Velero     *oadpv1alpha1.Velero
		WantError  bool
	}

	DescribeTable("Updating custom resource with new configuration",
		func(installCase InstallCase, expectedErr error) {
			err := veleroCR.CreateOrUpdate(installCase.VeleroSpec)
			Expect(err).ToNot(HaveOccurred())
			log.Printf("Waiting for velero pod to be running")
			Eventually(isVeleroPodRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			velero, err := veleroCR.Get()
			if err != nil {
				if len(velero.Spec.BackupStorageLocations) > 0 {
					// move these to velero_helper code
					log.Printf("Checking for bsl spec")
					for _, bsl := range velero.Spec.BackupStorageLocations {
						// Check if bsl matches the spec
					}
				}
				if len(velero.Spec.VolumeSnapshotLocations) > 0 {
					// move these to velero_helper code
					log.Printf("Checking for vsl spec")
				}
				if len(velero.Spec.DefaultVeleroPlugins) > 0 {
					// move these to velero_helper code
					log.Printf("Checking for default plugins")
				}
				//check for tolerations
				// check for custom velero plugins
				//restic installation
			}
		},
	)
})

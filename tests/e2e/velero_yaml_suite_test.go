package e2e

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type dpaTemplateParams struct {
	DpaName         string
	Namespace       string
	BslRegion       string
	ClusterProfile  string
	Provider        string
	CredentialsName string
	BucketName      string
	Prefix          string
}

var _ = Describe("Test DPA creation with", func() {
	var _ = BeforeEach(func() {
		testSuiteInstanceName := "ts-" + instanceName
		vel.Name = testSuiteInstanceName

		credData, err := getCredsData(cloud)
		Expect(err).NotTo(HaveOccurred())

		err = createCredentialsSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())
	})

	var _ = AfterEach(func() {
		err := vel.Delete()
		Expect(err).ToNot(HaveOccurred())

	})

	It("One Backup Storage Location templated from yaml", func() {
		// Create DPA and verify it is successful
		dpaTemplate, _ := filepath.Abs("templates/dpa_template.tmpl")
		params := dpaTemplateParams{
			DpaName:         vel.Name,
			Namespace:       namespace,
			BslRegion:       region,
			ClusterProfile:  clusterProfile,
			Provider:        provider,
			CredentialsName: credSecretRef,
			BucketName:      s3Bucket,
			Prefix:          veleroPrefix,
		}
		err := vel.CreateDpaFromYaml(dpaTemplate, params)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}

		Eventually(isVeleroPodRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())

		dpa, err := vel.Get()
		Expect(err).NotTo(HaveOccurred())
		if len(dpa.Spec.BackupLocations) > 0 {
			for _, bsl := range dpa.Spec.BackupLocations {
				// Check if bsl matches the spec
				Eventually(doesBSLExist(namespace, *bsl.Velero, &dpa.Spec), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
		}
	})
})

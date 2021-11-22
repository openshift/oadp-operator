package e2e

import (
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type veleroTemplateParams struct {
	VeleroName      string
	BslRegion       string
	Provider        string
	CredentialsName string
	BucketName      string
	Prefix          string
}

var _ = AfterEach(func() {
	// Delete Velero CR once the test has finished
	err := vel.Delete()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Test Velero CR creation with", func() {
	It("One Backup Storage Location templated from yaml", func() {
		// Create Velero CR and verify it is successful
		veleroTemplate, _ := filepath.Abs("templates/velero_bsl_template.tmpl")
		params := veleroTemplateParams{
			instanceName,
			region,
			provider,
			credSecretRef,
			s3Bucket,
			veleroPrefix,
		}
		err := vel.CreateVeleroFromYaml(veleroTemplate, params)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	})
})

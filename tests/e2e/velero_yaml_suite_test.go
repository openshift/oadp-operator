package e2e

import (
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type veleroTemplateParams struct {
	VeleroName      string
	BslRegion       string
	S3Url           string
	CredentialsName string
	BucketName      string
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
			"example-vel",
			"us-west-1",
			"http://192.156.2.4:9000",
			"cloud-credentials",
			"mybucket",
		}
		err := vel.CreateVeleroFromYaml(veleroTemplate, params)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	})
})

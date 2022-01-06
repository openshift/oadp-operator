package e2e

import (
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type dpaTemplateParams struct {
	DpaName         string
	BslRegion       string
	Provider        string
	CredentialsName string
	BucketName      string
	Prefix          string
}

var _ = AfterEach(func() {
	// Delete DPA once the test has finished
	err := vel.Delete()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Test DPA creation with", func() {
	It("One Backup Storage Location templated from yaml", func() {
		// Create DPA and verify it is successful
		dpaTemplate, _ := filepath.Abs("templates/dpa_template.tmpl")
		params := dpaTemplateParams{
			DpaName:         instanceName,
			BslRegion:       region,
			Provider:        provider,
			CredentialsName: credSecretRef,
			BucketName:      s3Bucket,
			Prefix:          veleroPrefix,
		}
		err := vel.CreateDpaFromYaml(dpaTemplate, params)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	})
})

package e2e

import (
	"path/filepath"

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

		credData, err := readFile(cloud)
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
			BslRegion:       dpa.Spec.BackupLocations[0].Velero.Config["region"],
			ClusterProfile:  clusterProfile,
			Provider:        dpa.Spec.BackupLocations[0].Velero.Provider,
			CredentialsName: credSecretRef,
			BucketName:      dpa.Spec.BackupLocations[0].Velero.ObjectStorage.Bucket,
			Prefix:          veleroPrefix,
		}
		err := vel.CreateDpaFromYaml(dpaTemplate, params)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	})
})

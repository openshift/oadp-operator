package e2e

import (
	"flag"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var testInstanceName string
var deploymentName string

var _ = Describe("The Velero Restic spec", func() {
	var _ = BeforeEach(func() {
		flag.Parse()
		s3Buffer, err := getJsonData(s3BucketFilePath)
		Expect(err).NotTo(HaveOccurred())
		s3Data, err := decodeJson(s3Buffer) // Might need to change this later on to create s3 for each tests
		Expect(err).NotTo(HaveOccurred())
		s3Bucket = s3Data["velero-bucket-name"].(string)

		testInstanceName = "ps-" + instanceName

		deploymentName = "velero"

		credData, err := getCredsData(cloud)
		Expect(err).NotTo(HaveOccurred())

		err = createCredentialsSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())

		err = installDefaultVelero(namespace, s3Bucket, credSecretRef, testInstanceName)
		Expect(err).ToNot(HaveOccurred())
	})

	var _ = AfterEach(func() {
		testInstanceName := "ps-" + instanceName
		err := uninstallVelero(namespace, testInstanceName)
		Expect(err).ToNot(HaveOccurred())

		errs := deleteSecret(namespace, credSecretRef)
		Expect(errs).ToNot(HaveOccurred())
	})

	Context("When the 'aws' default_velero_plugin is removed", func() {
		It("Should remove the aws plugin image", func() {

			// wait for Velero deployment to initialize
			Eventually(doesVeleroDeploymentExist(namespace, deploymentName), time.Minute*2, time.Second*5).Should(BeTrue())

			err := updateVeleroPlugins(namespace, testInstanceName, []string{
				"csi",
				"openshift",
			})
			Expect(err).ToNot(HaveOccurred())

			// wait for deployment to update
			Eventually(doesPluginExist(namespace, deploymentName, "velero-plugin-for-aws"), time.Minute*2, time.Second*5).Should(BeFalse())
		})
	})

	Context("When the 'openshift' default_velero_plugin is removed", func() {
		It("Should remove the openshift plugin image", func() {

			// wait for Velero deployment to initialize
			Eventually(doesVeleroDeploymentExist(namespace, deploymentName), time.Minute*2, time.Second*5).Should(BeTrue())

			err := updateVeleroPlugins(namespace, testInstanceName, []string{
				"aws",
				"csi",
			})
			Expect(err).ToNot(HaveOccurred())

			// wait for deployment to update
			Eventually(doesPluginExist(namespace, deploymentName, "openshift-velero-plugin"), time.Minute*2, time.Second*5).Should(BeFalse())
		})
	})

	Context("When the 'csi' default_velero_plugin is removed", func() {
		It("Should remove the csi plugin image", func() {

			// wait for Velero deployment to initialize
			Eventually(doesVeleroDeploymentExist(namespace, deploymentName), time.Minute*2, time.Second*5).Should(BeTrue())

			err := updateVeleroPlugins(namespace, testInstanceName, []string{
				"aws",
				"openshift",
			})
			Expect(err).ToNot(HaveOccurred())

			// wait for deployment to update
			Eventually(doesPluginExist(namespace, deploymentName, "velero-plugin-for-csi"), time.Minute*2, time.Second*5).Should(BeFalse())
		})
	})
	Context("When the 'csi' default_velero_plugin is added", func() {
		It("Should add the csi plugin image", func() {

			// wait for Velero deployment to initialize
			Eventually(doesVeleroDeploymentExist(namespace, deploymentName), time.Minute*2, time.Second*5).Should(BeTrue())

			err := updateVeleroPlugins(namespace, testInstanceName, []string{
				"aws",
				"openshift",
				"csi",
			})
			Expect(err).ToNot(HaveOccurred())

			// wait for deployment to update
			Eventually(doesPluginExist(namespace, deploymentName, "velero-plugin-for-csi"), time.Minute*2, time.Second*5).Should(BeTrue())
		})
	})
})

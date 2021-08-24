package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift/oadp-operator/api/v1alpha1"
)

var testInstanceName string
var deploymentName string

var _ = Describe("The Velero Restic spec", func() {
	var _ = BeforeEach(func() {
		testInstanceName = "ps-" + instanceName
		deploymentName = "velero"
		vel.Name = testInstanceName

		credData, err := getCredsData(cloud)
		Expect(err).NotTo(HaveOccurred())

		err = createCredentialsSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())

		err = vel.Build()
		Expect(err).NotTo(HaveOccurred())

		err = vel.Create()
		Expect(err).NotTo(HaveOccurred())
	})

	var _ = AfterEach(func() {
		err := vel.Delete()
		Expect(err).ToNot(HaveOccurred())

		errs := deleteSecret(namespace, credSecretRef)
		Expect(errs).ToNot(HaveOccurred())
	})

	Context("When the 'aws' default_velero_plugin is removed", func() {
		It("Should remove the aws plugin image", func() {

			// wait for Velero deployment to initialize
			Eventually(doesVeleroDeploymentExist(namespace, deploymentName), time.Minute*2, time.Second*5).Should(BeTrue())

			err := vel.removeVeleroPlugin(namespace, testInstanceName, []v1alpha1.DefaultPlugin{
				"csi",
				"openshift",
			}, "aws")
			Expect(err).ToNot(HaveOccurred())

			// wait for deployment to update
			Eventually(doesPluginExist(namespace, deploymentName, "velero-plugin-for-aws"), time.Minute*2, time.Second*5).Should(BeFalse())
		})
	})

	Context("When the 'openshift' default_velero_plugin is removed", func() {
		It("Should remove the openshift plugin image", func() {

			// wait for Velero deployment to initialize
			Eventually(doesVeleroDeploymentExist(namespace, deploymentName), time.Minute*2, time.Second*5).Should(BeTrue())

			err := vel.removeVeleroPlugin(namespace, testInstanceName, []v1alpha1.DefaultPlugin{
				"aws",
				"csi",
			}, "openshift")
			Expect(err).ToNot(HaveOccurred())

			// wait for deployment to update
			Eventually(doesPluginExist(namespace, deploymentName, "openshift-velero-plugin"), time.Minute*2, time.Second*5).Should(BeFalse())
		})
	})

	Context("When the 'csi' default_velero_plugin is removed", func() {
		It("Should remove the csi plugin image", func() {

			// wait for Velero deployment to initialize
			Eventually(doesVeleroDeploymentExist(namespace, deploymentName), time.Minute*2, time.Second*5).Should(BeTrue())

			err := vel.removeVeleroPlugin(namespace, testInstanceName, []v1alpha1.DefaultPlugin{
				"aws",
				"openshift",
			}, "csi")
			Expect(err).ToNot(HaveOccurred())

			// wait for deployment to update
			Eventually(doesPluginExist(namespace, deploymentName, "velero-plugin-for-csi"), time.Minute*2, time.Second*5).Should(BeFalse())
		})
	})

	// ***TODO: this needs specific gcp credentials - wait until enabled with CI
	// Context("When the 'gcp' default_velero_plugin is added", func() {
	// 	It("Should remove the csi plugin image", func() {

	// 		// wait for Velero deployment to initialize
	// 		Eventually(doesVeleroDeploymentExist(namespace, deploymentName), time.Minute*2, time.Second*5).Should(BeTrue())

	// 		err := vel.removeVeleroPlugin(namespace, testInstanceName, []v1alpha1.DefaultPlugin{
	// 			"aws",
	// 			"openshift",
	// 		}, "gcp")
	// 		Expect(err).ToNot(HaveOccurred())

	// 		// wait for deployment to update
	// 		Eventually(doesPluginExist(namespace, deploymentName, "velero-plugin-for-csi"), time.Minute*2, time.Second*5).Should(BeFalse())
	// 	})
	// })
})

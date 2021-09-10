package e2e

import (
	"flag"
	"github.com/openshift/oadp-operator/pkg/common"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var vel *veleroCustomResource

var _ = BeforeSuite(func() {
	flag.Parse()
	s3Buffer, err := getJsonData(s3BucketFilePath)
	Expect(err).NotTo(HaveOccurred())
	s3Data, err := decodeJson(s3Buffer) // Might need to change this later on to create s3 for each tests
	Expect(err).NotTo(HaveOccurred())
	s3Bucket = s3Data["velero-bucket-name"].(string)

	vel = &veleroCustomResource{
		Namespace: namespace,
		Region:    "us-east-1",
		Bucket:    s3Bucket,
		Provider:  "aws",
	}
	vel.SetClient()
	Expect(doesNamespaceExist(namespace)).Should(BeTrue())
})

var _ = AfterSuite(func() {
	Eventually(vel.IsDeleted(), time.Minute*2, time.Second*5).Should(BeTrue())
})

var _ = Describe("The default Velero custom resource", func() {
	var _ = BeforeEach(func() {
		testSuiteInstanceName := "ts-" + instanceName
		vel.Name = testSuiteInstanceName

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

	Context("When the default valid Velero CR is created", func() {
		It("Should create a Velero pod in the cluster", func() {
			Eventually(isVeleroPodRunning(namespace), time.Minute*3, time.Second*5).Should(BeTrue())
		})
		It("Should create a Restic daemonset in the cluster", func() {
			Eventually(areResticPodsRunning(namespace), time.Minute*3, time.Second*5).Should(BeTrue())
		})
		It("Should install the aws plugin", func() {
			Eventually(doesPluginExist(namespace, "velero", common.VeleroPluginForAWS), time.Minute*3, time.Second*5).Should(BeTrue())
		})
		It("Should install the openshift plugin", func() {
			Eventually(doesPluginExist(namespace, "velero", common.VeleroPluginForOpenshift), time.Minute*3, time.Second*5).Should(BeTrue())
		})
		/*
			It("Should install the csi plugin", func() {
				Eventually(doesPluginExist(namespace, "velero", "velero-plugin-for-csi"), time.Minute*2, time.Second*5).Should(BeTrue())
			})*/
	})
})

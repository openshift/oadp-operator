package e2e

import (
	"flag"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/utils/pointer"
	"log"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
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
		Region:    region,
		Bucket:    s3Bucket,
		Provider:  provider,
	}
	testSuiteInstanceName := "ts-" + instanceName
	vel.Name = testSuiteInstanceName

	vel.SetClient()
	Expect(doesNamespaceExist(namespace)).Should(BeTrue())
})

var _ = AfterSuite(func() {
	log.Printf("Deleting Velero CR")
	err := vel.Delete()
	Expect(err).ToNot(HaveOccurred())

	errs := deleteSecret(namespace, credSecretRef)
	Expect(errs).ToNot(HaveOccurred())
	Eventually(vel.IsDeleted(), time.Minute*2, time.Second*5).Should(BeTrue())
})

var _ = Describe("Configuration testing for Velero Custom Resource", func() {
	var _ = BeforeEach(func() {
		credData, err := getCredsData(cloud)
		Expect(err).NotTo(HaveOccurred())

		err = createCredentialsSecret(credData, namespace, credSecretRef)
		Expect(err).NotTo(HaveOccurred())
	})

	type InstallCase struct {
		Name                       string
		VeleroSpec                 *oadpv1alpha1.VeleroSpec
		ExpectRestic               bool
		ExpectedPlugins            []string
		ExpectedResticNodeSelector map[string]string
	}

	DescribeTable("Updating custom resource with new configuration",
		func(installCase InstallCase, expectedErr error) {
			err := vel.CreateOrUpdate(installCase.VeleroSpec)
			Expect(err).ToNot(HaveOccurred())
			log.Printf("Waiting for velero pod to be running")
			Eventually(isVeleroPodRunning(namespace), time.Minute*3, time.Second*5).Should(BeTrue())
			if installCase.ExpectRestic {
				log.Printf("Waiting for restic pods to be running")
				Eventually(areResticPodsRunning(namespace), time.Minute*3, time.Second*5).Should(BeTrue())
			} else {
				log.Printf("Waiting for restic daemonset to be deleted")
				Eventually(isResticDaemonsetDeleted(namespace), time.Minute*3, time.Second*5).Should(BeTrue())
			}
			log.Printf("Waiting for velero deployment to have expected plugins")
			for _, plugin := range installCase.ExpectedPlugins {
				Eventually(doesPluginExist(namespace, plugin), time.Minute*3, time.Second*5).Should(BeTrue())
			}
			for key, value := range installCase.ExpectedResticNodeSelector {
				log.Printf("Waiting for restic daemonset to get node selector")
				Eventually(resticDaemonSetHasNodeSelector(namespace, key, value), time.Minute*3, time.Second*5).Should(BeTrue())
			}
		},
		Entry("Default velero CR", InstallCase{
			Name: "default-cr",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
				OlmManaged:   pointer.Bool(false),
				EnableRestic: pointer.Bool(true),
				BackupStorageLocations: []velero.BackupStorageLocationSpec{
					{
						Provider: provider,
						Config: map[string]string{
							"region": region,
						},
						Default: true,
						StorageType: velero.StorageType{
							ObjectStorage: &velero.ObjectStorageLocation{
								Bucket: s3Bucket,
								Prefix: "velero",
							},
						},
					},
				},
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
					oadpv1alpha1.DefaultPluginAWS,
				},
			},
			ExpectRestic: true,
			ExpectedPlugins: []string{
				common.VeleroPluginForAWS,
				common.VeleroPluginForOpenshift,
			},
		}, nil),
		Entry("Default velero CR with restic disabled", InstallCase{
			Name: "default-cr-no-restic",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
				OlmManaged:   pointer.Bool(false),
				EnableRestic: pointer.Bool(false),
				BackupStorageLocations: []velero.BackupStorageLocationSpec{
					{
						Provider: provider,
						Config: map[string]string{
							"region": region,
						},
						Default: true,
						StorageType: velero.StorageType{
							ObjectStorage: &velero.ObjectStorageLocation{
								Bucket: s3Bucket,
								Prefix: "velero",
							},
						},
					},
				},
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
					oadpv1alpha1.DefaultPluginAWS,
				},
			},
			ExpectRestic: false,
			ExpectedPlugins: []string{
				common.VeleroPluginForAWS,
				common.VeleroPluginForOpenshift,
			},
		}, nil),
		Entry("Adding CSI plugin", InstallCase{
			Name: "default-cr-csi",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
				OlmManaged:   pointer.Bool(false),
				EnableRestic: pointer.Bool(true),
				BackupStorageLocations: []velero.BackupStorageLocationSpec{
					{
						Provider: provider,
						Config: map[string]string{
							"region": region,
						},
						Default: true,
						StorageType: velero.StorageType{
							ObjectStorage: &velero.ObjectStorageLocation{
								Bucket: s3Bucket,
								Prefix: "velero",
							},
						},
					},
				},
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
					oadpv1alpha1.DefaultPluginAWS,
					oadpv1alpha1.DefaultPluginCSI,
				},
			},
			ExpectRestic: true,
			ExpectedPlugins: []string{
				common.VeleroPluginForAWS,
				common.VeleroPluginForOpenshift,
				common.VeleroPluginForCSI,
			},
		}, nil),
		Entry("Set restic node selector", InstallCase{
			Name: "default-cr-node-selector",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
				OlmManaged:   pointer.Bool(false),
				EnableRestic: pointer.Bool(true),
				ResticNodeSelector: map[string]string{
					"foo": "bar",
				},
				BackupStorageLocations: []velero.BackupStorageLocationSpec{
					{
						Provider: provider,
						Config: map[string]string{
							"region": region,
						},
						Default: true,
						StorageType: velero.StorageType{
							ObjectStorage: &velero.ObjectStorageLocation{
								Bucket: s3Bucket,
								Prefix: "velero",
							},
						},
					},
				},
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
					oadpv1alpha1.DefaultPluginAWS,
					oadpv1alpha1.DefaultPluginCSI,
				},
			},
			ExpectRestic: true,
			ExpectedPlugins: []string{
				common.VeleroPluginForAWS,
				common.VeleroPluginForOpenshift,
				common.VeleroPluginForCSI,
			},
			ExpectedResticNodeSelector: map[string]string{
				"foo": "bar",
			},
		}, nil),
	)
})

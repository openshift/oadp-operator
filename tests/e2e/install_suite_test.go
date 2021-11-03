package e2e

import (
	"flag"
	"log"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"

	"github.com/openshift/oadp-operator/pkg/common"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/utils/pointer"

	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var vel *veleroCustomResource

var _ = BeforeSuite(func() {
	flag.Parse()
	s3Buffer, err := getJsonData(bucketFilePath)
	Expect(err).NotTo(HaveOccurred())
	s3Data, err := decodeJson(s3Buffer) // Might need to change this later on to create s3 for each tests
	Expect(err).NotTo(HaveOccurred())
	bucket = s3Data["velero-bucket-name"].(string)

	vel = &veleroCustomResource{
		Namespace:     namespace,
		BslRegion:     bsl_region,
		VslRegion:     vsl_region,
		Bucket:        bucket,
		Provider:      provider,
		BslProfile:    bsl_profile,
		credentials:   credentials,
		credSecretRef: credSecretRef,
	}
	testSuiteInstanceName := "ts-" + instanceName
	vel.Name = testSuiteInstanceName
	// err := vel.createBsl()
	openshift_ci_bool, _ := strconv.ParseBool(openshift_ci)
	if openshift_ci_bool == true {
		switch vel.Provider {
		case "aws":
			cloudCredData, err := getCredsData(vel.credentials)
			Expect(err).NotTo(HaveOccurred())
			ciCredData, err := getCredsData(ci_cred_file)
			Expect(err).NotTo(HaveOccurred())
			cloudCredData = append(cloudCredData, []byte("\n")...)
			credData := append(cloudCredData, ciCredData...)
			vel.credentials = "/tmp/aws-credentials"
			err = putCredsData(vel.credentials, credData)
			Expect(err).NotTo(HaveOccurred())
		}
	}
	credData, err := getCredsData(vel.credentials)
	Expect(err).NotTo(HaveOccurred())
	err = createCredentialsSecret(credData, namespace, credSecretRef)
	Expect(err).NotTo(HaveOccurred())
	vel.SetClient()
	Expect(doesNamespaceExist(namespace)).Should(BeTrue())
})

var _ = AfterSuite(func() {
	log.Printf("Deleting Velero CR")
	err := vel.Delete()
	Expect(err).ToNot(HaveOccurred())
	// err = vel.deleteBsl()
	Expect(err).NotTo(HaveOccurred())
	errs := deleteSecret(namespace, credSecretRef)
	Expect(errs).ToNot(HaveOccurred())
	Eventually(vel.IsDeleted(), timeoutMultiplier*time.Minute*2, time.Second*5).Should(BeTrue())
})

var _ = Describe("Configuration testing for Velero Custom Resource", func() {
	// var _ = BeforeEach(func() {
	// 	credData, err := getCredsData(vel.credentials)
	// 	Expect(err).NotTo(HaveOccurred())

	// 	err = createCredentialsSecret(credData, namespace, credSecretRef)
	// 	Expect(err).NotTo(HaveOccurred())
	// })

	type InstallCase struct {
		Name                       string
		VeleroSpec                 *oadpv1alpha1.VeleroSpec
		ExpectRestic               bool
		ExpectedPlugins            []string
		ExpectedResticNodeSelector map[string]string
	}

	DescribeTable("Updating custom resource with new configuration",
		func(installCase InstallCase, expectedErr error) {
			if len(installCase.VeleroSpec.BackupStorageLocations) > 0 {
				switch vel.Provider {
				case "aws":
					installCase.VeleroSpec.BackupStorageLocations[0].Config = map[string]string{
						"region": vel.BslRegion,
					}
					installCase.VeleroSpec.DefaultVeleroPlugins = append(installCase.VeleroSpec.DefaultVeleroPlugins, oadpv1alpha1.DefaultPluginAWS) // case "gcp":
					installCase.ExpectedPlugins = append(installCase.ExpectedPlugins, common.VeleroPluginForAWS)
					// 	config["serviceAccount"] = v.Region
				}
			}
			err := vel.CreateOrUpdate(installCase.VeleroSpec)
			Expect(err).ToNot(HaveOccurred())
			log.Printf("Waiting for velero pod to be running")
			Eventually(isVeleroPodRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			if installCase.ExpectRestic {
				log.Printf("Waiting for restic pods to be running")
				Eventually(areResticPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			} else {
				log.Printf("Waiting for restic daemonset to be deleted")
				Eventually(isResticDaemonsetDeleted(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			log.Printf("Waiting for velero deployment to have expected plugins")
			for _, plugin := range installCase.ExpectedPlugins {
				Eventually(doesPluginExist(namespace, plugin), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			for key, value := range installCase.ExpectedResticNodeSelector {
				log.Printf("Waiting for restic daemonset to get node selector")
				Eventually(resticDaemonSetHasNodeSelector(namespace, key, value), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			if installCase.VeleroSpec.BackupImages == nil || *installCase.VeleroSpec.BackupImages {
				log.Printf("Waiting for registry pods to be running")
				Eventually(areRegistryDeploymentsAvailable(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
		},
		Entry("Default velero CR", InstallCase{
			Name: "default-cr",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
				EnableRestic: pointer.Bool(true),
				BackupStorageLocations: []velero.BackupStorageLocationSpec{
					{
						Provider: provider,
						Default:  true,
						StorageType: velero.StorageType{
							ObjectStorage: &velero.ObjectStorageLocation{
								Bucket: bucket,
								Prefix: veleroPrefix,
							},
						},
					},
				},
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
				},
			},
			ExpectRestic: true,
			ExpectedPlugins: []string{
				common.VeleroPluginForOpenshift,
			},
		}, nil),
		Entry("Default velero CR with restic disabled", InstallCase{
			Name: "default-cr-no-restic",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
				EnableRestic: pointer.Bool(false),
				BackupStorageLocations: []velero.BackupStorageLocationSpec{
					{
						Provider: provider,
						Default:  true,
						StorageType: velero.StorageType{
							ObjectStorage: &velero.ObjectStorageLocation{
								Bucket: bucket,
								Prefix: veleroPrefix,
							},
						},
					},
				},
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
				},
			},
			ExpectRestic: false,
			ExpectedPlugins: []string{
				common.VeleroPluginForOpenshift,
			},
		}, nil),
		Entry("Adding CSI plugin", InstallCase{
			Name: "default-cr-csi",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
				EnableRestic: pointer.Bool(true),
				BackupStorageLocations: []velero.BackupStorageLocationSpec{
					{
						Provider: provider,
						Default:  true,
						StorageType: velero.StorageType{
							ObjectStorage: &velero.ObjectStorageLocation{
								Bucket: bucket,
								Prefix: veleroPrefix,
							},
						},
					},
				},
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
					oadpv1alpha1.DefaultPluginCSI,
				},
			},
			ExpectRestic: true,
			ExpectedPlugins: []string{
				common.VeleroPluginForOpenshift,
				common.VeleroPluginForCSI,
			},
		}, nil),
		Entry("Set restic node selector", InstallCase{
			Name: "default-cr-node-selector",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
				EnableRestic: pointer.Bool(true),
				ResticNodeSelector: map[string]string{
					"foo": "bar",
				},
				BackupStorageLocations: []velero.BackupStorageLocationSpec{
					{
						Provider: provider,
						Default:  true,
						StorageType: velero.StorageType{
							ObjectStorage: &velero.ObjectStorageLocation{
								Bucket: bucket,
								Prefix: veleroPrefix,
							},
						},
					},
				},
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
					oadpv1alpha1.DefaultPluginCSI,
				},
			},
			ExpectRestic: true,
			ExpectedPlugins: []string{
				common.VeleroPluginForOpenshift,
				common.VeleroPluginForCSI,
			},
			ExpectedResticNodeSelector: map[string]string{
				"foo": "bar",
			},
		}, nil),
		Entry("NoDefaultBackupLocation", InstallCase{
			Name: "default-cr-node-selector",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
				EnableRestic:            pointer.Bool(true),
				BackupStorageLocations:  []velero.BackupStorageLocationSpec{},
				NoDefaultBackupLocation: true,
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
				},
			},
			ExpectRestic: true,
			ExpectedPlugins: []string{
				common.VeleroPluginForOpenshift,
			},
		}, nil),
	)
})

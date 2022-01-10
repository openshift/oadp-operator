package e2e

import (
	"fmt"
	"log"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("Configuration testing for DPA Custom Resource", func() {

	type InstallCase struct {
		Name         string
		BRestoreType BackupRestoreType
		DpaSpec      *oadpv1alpha1.DataProtectionApplicationSpec
		WantError    bool
	}

	DescribeTable("Updating custom resource with new configuration",
		func(installCase InstallCase, expectedErr error) {
			//TODO: Calling vel.build() is the old pattern.
			//Change it later to make sure all the spec values are passed for every test case,
			// instead of assigning the values in advance to the DPA CR
			err := vel.Build(installCase.BRestoreType)
			Expect(err).NotTo(HaveOccurred())
			if len(installCase.DpaSpec.BackupLocations) > 0 {
				switch vel.Provider {
				case "aws":
					if installCase.DpaSpec.BackupLocations[0].Velero.Config == nil {
						installCase.DpaSpec.BackupLocations[0].Velero.Config = map[string]string{
							"region":  vel.awsConfig.BslRegion,
							"profile": vel.awsConfig.BslProfile,
						}
					} else {
						installCase.DpaSpec.BackupLocations[0].Velero.Config["region"] = vel.awsConfig.BslRegion
						installCase.DpaSpec.BackupLocations[0].Velero.Config["profile"] = vel.awsConfig.BslProfile
					}
					installCase.DpaSpec.Configuration.Velero.DefaultPlugins = append(installCase.DpaSpec.Configuration.Velero.DefaultPlugins, oadpv1alpha1.DefaultPluginAWS)
					installCase.DpaSpec.SnapshotLocations = []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &velero.VolumeSnapshotLocationSpec{
								Provider: vel.Provider,
								Config: map[string]string{
									"region":  vel.awsConfig.VslRegion,
									"profile": "default",
								},
							},
						},
					}
				case "gcp":
					if installCase.DpaSpec.BackupLocations[0].Velero.Config == nil {
						installCase.DpaSpec.BackupLocations[0].Velero.Config = map[string]string{
							"credentialsFile": "bsl-cloud-credentials-gcp/cloud",
						}
					} else {
						installCase.DpaSpec.BackupLocations[0].Velero.Config["credentialsFile"] = "bsl-cloud-credentials-gcp/cloud"
					}
					installCase.DpaSpec.Configuration.Velero.DefaultPlugins = append(installCase.DpaSpec.Configuration.Velero.DefaultPlugins, oadpv1alpha1.DefaultPluginGCP)
					installCase.DpaSpec.SnapshotLocations = []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &velero.VolumeSnapshotLocationSpec{
								Provider: vel.Provider,
								Config: map[string]string{
									"snapshotLocation": vel.gcpConfig.VslRegion,
								},
							},
						},
					}
				case "azure":
					if installCase.DpaSpec.BackupLocations[0].Velero.Config == nil {
						installCase.DpaSpec.BackupLocations[0].Velero.Config = map[string]string{
							"credentialsFile":         "bsl-cloud-credentials-azure/cloud",
							"subscriptionId":          vel.azureConfig.BslSubscriptionId,
							"storageAccount":          vel.azureConfig.BslStorageAccount,
							"resourceGroup":           vel.azureConfig.BslResourceGroup,
							"storageAccountKeyEnvVar": vel.azureConfig.BslStorageAccountKeyEnvVar,
						}
					} else {
						installCase.DpaSpec.BackupLocations[0].Velero.Config["credentialsFile"] = "bsl-cloud-credentials-azure/cloud"
						installCase.DpaSpec.BackupLocations[0].Velero.Config["subscriptionId"] = vel.azureConfig.BslSubscriptionId
						installCase.DpaSpec.BackupLocations[0].Velero.Config["storageAccount"] = vel.azureConfig.BslStorageAccount
						installCase.DpaSpec.BackupLocations[0].Velero.Config["resourceGroup"] = vel.azureConfig.BslResourceGroup
						installCase.DpaSpec.BackupLocations[0].Velero.Config["storageAccountKeyEnvVar"] = vel.azureConfig.BslStorageAccountKeyEnvVar
					}
					installCase.DpaSpec.Configuration.Velero.DefaultPlugins = append(installCase.DpaSpec.Configuration.Velero.DefaultPlugins, oadpv1alpha1.DefaultPluginMicrosoftAzure)
					installCase.DpaSpec.SnapshotLocations = []oadpv1alpha1.SnapshotLocation{
						{
							Velero: &velero.VolumeSnapshotLocationSpec{
								Provider: vel.Provider,
								Config: map[string]string{
									"subscriptionId": vel.azureConfig.VslSubscriptionId,
								},
							},
						},
					}
				}
			}
			err = vel.CreateOrUpdate(installCase.DpaSpec)
			Expect(err).ToNot(HaveOccurred())
			if installCase.WantError {
				// Eventually()
				log.Printf("Test case expected to error. Waiting for the error to show in DPA Status")
				Eventually(vel.GetNoErr().Status.Conditions[0].Type, timeoutMultiplier*time.Minute*3, time.Second*5).Should(Equal("Reconciled"))
				Eventually(vel.GetNoErr().Status.Conditions[0].Status, timeoutMultiplier*time.Minute*3, time.Second*5).Should(Equal(metav1.ConditionFalse))
				Eventually(vel.GetNoErr().Status.Conditions[0].Reason, timeoutMultiplier*time.Minute*3, time.Second*5).Should(Equal("Error"))
				Eventually(vel.GetNoErr().Status.Conditions[0].Message, timeoutMultiplier*time.Minute*3, time.Second*5).Should(Equal(expectedErr.Error()))
				return
			}
			log.Printf("Waiting for velero pod to be running")
			Eventually(isVeleroPodRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			dpa, err := vel.Get()
			Expect(err).NotTo(HaveOccurred())
			if len(dpa.Spec.BackupLocations) > 0 {
				log.Printf("Checking for bsl spec")
				for _, bsl := range dpa.Spec.BackupLocations {
					// Check if bsl matches the spec
					Eventually(doesBSLExist(namespace, *bsl.Velero, installCase.DpaSpec), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}
			if len(dpa.Spec.SnapshotLocations) > 0 {
				log.Printf("Checking for vsl spec")
				for _, vsl := range dpa.Spec.SnapshotLocations {
					Eventually(doesVSLExist(namespace, *vsl.Velero, installCase.DpaSpec), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}

			// Check for velero tolerations
			if len(dpa.Spec.Configuration.Velero.PodConfig.Tolerations) > 0 {
				log.Printf("Checking for velero tolerations")
				Eventually(verifyVeleroTolerations(namespace, dpa.Spec.Configuration.Velero.PodConfig.Tolerations), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			// check for velero resource allocations
			if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests != nil {
				log.Printf("Checking for velero resource allocation requests")
				Eventually(verifyVeleroResourceRequests(namespace, dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits != nil {
				log.Printf("Checking for velero resource allocation limits")
				Eventually(verifyVeleroResourceLimits(namespace, dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			//restic installation
			if dpa.Spec.Configuration.Restic != nil && *dpa.Spec.Configuration.Restic.Enable {
				log.Printf("Waiting for restic pods to be running")
				Eventually(areResticPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			} else {
				log.Printf("Waiting for restic daemonset to be deleted")
				Eventually(isResticDaemonsetDeleted(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			// check defaultplugins
			log.Printf("Waiting for velero deployment to have expected plugins")
			if len(dpa.Spec.Configuration.Velero.DefaultPlugins) > 0 {
				log.Printf("Checking for default plugins")
				for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
					Eventually(doesPluginExist(namespace, plugin), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}

			// check customplugins
			log.Printf("Waiting for velero deployment to have expected custom plugins")
			if len(dpa.Spec.Configuration.Velero.CustomPlugins) > 0 {
				log.Printf("Checking for custom plugins")
				for _, plugin := range dpa.Spec.Configuration.Velero.CustomPlugins {
					Eventually(doesCustomPluginExist(namespace, plugin), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}

			if dpa.Spec.Configuration.Restic != nil && dpa.Spec.Configuration.Restic.PodConfig != nil {
				for key, value := range dpa.Spec.Configuration.Restic.PodConfig.NodeSelector {
					log.Printf("Waiting for restic daemonset to get node selector")
					Eventually(resticDaemonSetHasNodeSelector(namespace, key, value), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}
			if dpa.Spec.BackupImages == nil || *installCase.DpaSpec.BackupImages {
				log.Printf("Waiting for registry pods to be running")
				Eventually(areRegistryDeploymentsAvailable(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

		},
		Entry("AWS Without Region No S3ForcePathStyle with BackupImages false should succeed", InstallCase{
			Name:         "default-no-region-no-s3forcepathstyle",
			BRestoreType: restic,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupImages: pointer.Bool(false),
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
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
				},
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginAWS,
						},
					},
				},
			},
			WantError: false,
		}, nil),
		Entry("AWS With Region And S3ForcePathStyle should succeed", InstallCase{
			Name:         "default-with-region-and-s3forcepathstyle",
			BRestoreType: restic,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: provider,
							Config: map[string]string{
								"s3ForcePathStyle": "true",
							},
							Default: true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: veleroPrefix,
								},
							},
						},
					},
				},
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginAWS,
						},
					},
				},
			},
			WantError: false,
		}, nil),
		Entry("AWS Without Region And S3ForcePathStyle true should fail", InstallCase{
			Name:         "default-with-region-and-s3forcepathstyle",
			BRestoreType: restic,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: provider,
							Config: map[string]string{
								"s3ForcePathStyle": "true",
							},
							Default: true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: veleroPrefix,
								},
							},
						},
					},
				},
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginAWS,
						},
					},
				},
			},
			WantError: true,
		}, fmt.Errorf("region for AWS backupstoragelocation cannot be empty when s3ForcePathStyle is true or when backing up images")),
	)
})

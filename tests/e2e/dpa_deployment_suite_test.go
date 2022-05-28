package e2e_test

import (
	"fmt"
	"log"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	i "github.com/openshift/oadp-operator/tests/e2e/lib/init"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("Configuration testing for DPA Custom Resource", func() {
	bucket := GetDpa().Spec.BackupLocations[0].Velero.ObjectStorage.Bucket
	bslConfig := GetDpa().Spec.BackupLocations[0].Velero.Config
	credential := GetDpa().Spec.BackupLocations[0].Velero.Credential

	type InstallCase struct {
		Name               string
		BRestoreType       BackupRestoreType
		DpaSpec            *oadpv1alpha1.DataProtectionApplicationSpec
		TestCarriageReturn bool
		WantError          bool
	}

	genericTests := []TableEntry{
		Entry("Default velero CR", InstallCase{
			Name:         "default-cr",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						DefaultPlugins: GetDpa().Spec.Configuration.Velero.DefaultPlugins,
						PodConfig:      &oadpv1alpha1.PodConfig{},
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(true),
					},
				},
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider:   i.GetProvider(),
							Credential: credential,

							Config:  bslConfig,
							Default: true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: GetVeleroPrefix(),
								},
							},
						},
					},
				},
			},
			WantError: false,
		}, nil),
		Entry("Default velero CR, test carriage return", InstallCase{
			Name:               "default-cr",
			BRestoreType:       RESTIC,
			TestCarriageReturn: true,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						DefaultPlugins: GetDpa().Spec.Configuration.Velero.DefaultPlugins,
						PodConfig:      &oadpv1alpha1.PodConfig{},
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(true),
					},
				},
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: i.GetProvider(),

							Config:  bslConfig,
							Default: true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: GetVeleroPrefix(),
								},
							},
						},
					},
				},
			},
			WantError: false,
		}, nil),
		Entry("Adding Velero custom plugin", InstallCase{
			Name:         "default-cr-velero-custom-plugin",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: append([]oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginCSI,
						}, GetDpa().Spec.Configuration.Velero.DefaultPlugins...),
						CustomPlugins: []oadpv1alpha1.CustomPlugin{
							{
								Name:  "encryption-plugin",
								Image: "quay.io/konveyor/openshift-velero-plugin:latest",
							},
						},
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(false),
					},
				},
				BackupImages: pointer.Bool(false),
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider:   i.GetProvider(),
							Credential: credential,

							Config:  bslConfig,
							Default: true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: GetVeleroPrefix(),
								},
							},
						},
					},
				},
			},
			WantError: false,
		}, nil),
		Entry("Adding Velero resource allocations", InstallCase{
			Name:         "default-cr-velero-resource-alloc",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{
							ResourceAllocations: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
						DefaultPlugins: append([]oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginCSI,
						}, GetDpa().Spec.Configuration.Velero.DefaultPlugins...),
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(false),
					},
				},
				BackupImages: pointer.Bool(false),
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider:   i.GetProvider(),
							Credential: credential,
							Config:     bslConfig,
							Default:    true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: GetVeleroPrefix(),
								},
							},
						},
					},
				},
			},
			WantError: false,
		}, nil),
		Entry("Adding AWS plugin", InstallCase{
			Name:         "default-cr-aws-plugin",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginCSI,
						},
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(false),
					},
				},
				BackupImages: pointer.Bool(false),
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider:   i.GetProvider(),
							Credential: credential,
							Config:     bslConfig,
							Default:    true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: GetVeleroPrefix(),
								},
							},
						},
					},
				},
			},
			WantError: false,
		}, nil),
		Entry("DPA CR with bsl and vsl", InstallCase{
			Name:         "default-cr-bsl-vsl",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:      &oadpv1alpha1.PodConfig{},
						DefaultPlugins: GetDpa().Spec.Configuration.Velero.DefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(true),
					},
				},
				SnapshotLocations: GetDpa().Spec.SnapshotLocations,
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider:   i.GetProvider(),
							Credential: credential,
							Config:     bslConfig,
							Default:    true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: GetVeleroPrefix(),
								},
							},
						},
					},
				},
			},
			WantError: false,
		}, nil),
		Entry("Default velero CR with restic disabled", InstallCase{
			Name:         "default-cr-no-restic",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:      &oadpv1alpha1.PodConfig{},
						DefaultPlugins: GetDpa().Spec.Configuration.Velero.DefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(false),
					},
				},
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider:   i.GetProvider(),
							Credential: credential,

							Config:  bslConfig,
							Default: true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: GetVeleroPrefix(),
								},
							},
						},
					},
				},
			},
			WantError: false,
		}, nil),
		Entry("Adding CSI plugin", InstallCase{
			Name:         "default-cr-csi",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: append([]oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginCSI,
						}, GetDpa().Spec.Configuration.Velero.DefaultPlugins...),
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(false),
					},
				},
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider:   i.GetProvider(),
							Credential: credential,
							Config:     bslConfig,
							Default:    true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: GetVeleroPrefix(),
								},
							},
						},
					},
				},
			},
			WantError: false,
		}, nil),
		Entry("Set restic node selector", InstallCase{
			Name:         "default-cr-node-selector",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider:   i.GetProvider(),
							Credential: credential,
							Config:     bslConfig,
							Default:    true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: GetVeleroPrefix(),
								},
							},
						},
					},
				},
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:      &oadpv1alpha1.PodConfig{},
						DefaultPlugins: GetDpa().Spec.Configuration.Velero.DefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{
							NodeSelector: map[string]string{
								"foo": "bar",
							},
						},
						Enable: pointer.Bool(true),
					},
				},
			},
			WantError: false,
		}, nil),
		Entry("Enable tolerations", InstallCase{
			Name:         "default-cr-tolerations",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider:   i.GetProvider(),
							Credential: credential,
							Config:     bslConfig,
							Default:    true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: GetVeleroPrefix(),
								},
							},
						},
					},
				},
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:      &oadpv1alpha1.PodConfig{},
						DefaultPlugins: GetDpa().Spec.Configuration.Velero.DefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{
							Tolerations: []corev1.Toleration{
								{
									Key:               "node.kubernetes.io/unreachable",
									Operator:          "Exists",
									Effect:            "NoExecute",
									TolerationSeconds: func(i int64) *int64 { return &i }(6000),
								},
							},
						},
						Enable: pointer.Bool(true),
					},
				},
			},
			WantError: false,
		}, nil),
		Entry("NoDefaultBackupLocation", InstallCase{
			Name:         "default-cr-node-selector",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:               &oadpv1alpha1.PodConfig{},
						NoDefaultBackupLocation: true,
						DefaultPlugins:          GetDpa().Spec.Configuration.Velero.DefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(true),
					},
				},
				BackupImages: pointer.BoolPtr(false),
			},
			WantError: false,
		}, nil),
	}

	awsTests := []TableEntry{
		Entry("AWS Without Region No S3ForcePathStyle with BackupImages false should succeed", Label("aws"), InstallCase{
			Name:         "default-no-region-no-s3forcepathstyle",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupImages: pointer.Bool(false),
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider:   i.GetProvider(),
							Credential: credential,
							Default:    true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: GetVeleroPrefix(),
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
		Entry("AWS With Region And S3ForcePathStyle should succeed", Label("aws"), InstallCase{
			Name:         "default-with-region-and-s3forcepathstyle",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider:   i.GetProvider(),
							Credential: credential,
							Config: map[string]string{
								"region":           bslConfig["region"],
								"s3ForcePathStyle": "true",
								"profile":          bslConfig["profile"],
							},
							Default: true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: GetVeleroPrefix(),
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
		Entry("AWS Without Region And S3ForcePathStyle true should fail", Label("aws"), InstallCase{
			Name:         "default-with-region-and-s3forcepathstyle",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider:   i.GetProvider(),
							Credential: credential,
							Config: map[string]string{
								"s3ForcePathStyle": "true",
							},
							Default: true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: GetVeleroPrefix(),
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
	}
	genericTests = append(genericTests, awsTests...)

	var lastInstallingApplicationNamespace string
	var lastInstallTime time.Time
	var _ = ReportAfterEach(func(report SpecReport) {
		if report.Failed() {
			// print namespace error events for app namespace
			if lastInstallingApplicationNamespace != "" {
				PrintNamespaceEventsAfterTime(lastInstallingApplicationNamespace, lastInstallTime)
			}
		}
	})
	DescribeTable("Updating custom resource with new configuration",

		func(installCase InstallCase, expectedErr error) {
			//TODO: Calling dpaCR.build() is the old pattern.
			//Change it later to make sure all the spec values are passed for every test case,
			// instead of assigning the values in advance to the DPA CR
			err := dpaCR.Build(installCase.BRestoreType)
			Expect(err).NotTo(HaveOccurred())
			if len(installCase.DpaSpec.BackupLocations) > 0 && installCase.TestCarriageReturn {
				// use carriage return credential.
				installCase.DpaSpec.BackupLocations[0].Velero.Credential = &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "credential-with-carriage-return"}, Key: "cloud"}
			}
			lastInstallingApplicationNamespace = i.GetNamespace()
			lastInstallTime = time.Now()
			err = dpaCR.CreateOrUpdate(installCase.DpaSpec)
			Expect(err).ToNot(HaveOccurred())
			if installCase.WantError {
				// Eventually()
				log.Printf("Test case expected to error. Waiting for the error to show in DPA Status")
				Eventually(dpaCR.DPAReconcileError(), i.GetTimeoutMultiplier()*time.Minute*2, time.Second*5).Should(BeTrue())
				return
			}
			Eventually(dpaCR.DPAReconcileError(), i.GetTimeoutMultiplier()*time.Minute*2, time.Second*5).Should(BeFalse())
			log.Printf("Waiting for velero pod to be running")
			Eventually(AreVeleroDeploymentReplicasReady(i.GetNamespace()), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			Eventually(dpaCR.DoesDPAMatchSpec(i.GetNamespace(), installCase.DpaSpec), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			for n := range installCase.DpaSpec.BackupLocations {
				// wait for BSL to be ready using name pattern from controller https://github.com/openshift/oadp-operator/blob/a29c162c64c42c25029b176ff8b6a92914906639/controllers/bsl.go#L95
				Eventually(BackupStorageLocationIsAvailable(dpaCR.Client, fmt.Sprintf("%s-%d", i.GetTestSuiteInstanceName(), n+1), i.GetNamespace()), i.GetTimeoutMultiplier()*time.Minute*2, time.Second*5).Should(BeTrue())
			}

			// Check for velero tolerations
			if len(installCase.DpaSpec.Configuration.Velero.PodConfig.Tolerations) > 0 {
				log.Printf("Checking for velero tolerations")
				Eventually(VerifyVeleroTolerations(i.GetNamespace(), installCase.DpaSpec.Configuration.Velero.PodConfig.Tolerations), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			// check for velero resource allocations
			if installCase.DpaSpec.Configuration.Velero.PodConfig.ResourceAllocations.Requests != nil {
				log.Printf("Checking for velero resource allocation requests")
				Eventually(VerifyVeleroResourceRequests(i.GetNamespace(), installCase.DpaSpec.Configuration.Velero.PodConfig.ResourceAllocations.Requests), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			if installCase.DpaSpec.Configuration.Velero.PodConfig.ResourceAllocations.Limits != nil {
				log.Printf("Checking for velero resource allocation limits")
				Eventually(VerifyVeleroResourceLimits(i.GetNamespace(), installCase.DpaSpec.Configuration.Velero.PodConfig.ResourceAllocations.Limits), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			//restic installation
			if installCase.DpaSpec.Configuration.Restic != nil && *installCase.DpaSpec.Configuration.Restic.Enable {
				log.Printf("Waiting for restic pods to be running")
				Eventually(AreResticDaemonsetUpdatedAndReady(i.GetNamespace()), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			} else {
				log.Printf("Waiting for restic daemonset to be deleted")
				Eventually(IsResticDaemonsetDeleted(i.GetNamespace()), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			// check defaultplugins
			log.Printf("Waiting for velero deployment to have expected plugins")
			if len(installCase.DpaSpec.Configuration.Velero.DefaultPlugins) > 0 {
				log.Printf("Checking for default plugins")
				for _, plugin := range installCase.DpaSpec.Configuration.Velero.DefaultPlugins {
					Eventually(DoesPluginExist(i.GetNamespace(), plugin), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}

			// check customplugins
			log.Printf("Waiting for velero deployment to have expected custom plugins")
			if len(installCase.DpaSpec.Configuration.Velero.CustomPlugins) > 0 {
				log.Printf("Checking for custom plugins")
				for _, plugin := range installCase.DpaSpec.Configuration.Velero.CustomPlugins {
					Eventually(DoesCustomPluginExist(i.GetNamespace(), plugin), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}

			if installCase.DpaSpec.Configuration.Restic != nil && installCase.DpaSpec.Configuration.Restic.PodConfig != nil {
				for key, value := range installCase.DpaSpec.Configuration.Restic.PodConfig.NodeSelector {
					log.Printf("Waiting for restic daemonset to get node selector")
					Eventually(ResticDaemonSetHasNodeSelector(i.GetNamespace(), key, value), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}
			// create dpa struct to access BackupImages function
			dpa := oadpv1alpha1.DataProtectionApplication{
				Spec: *installCase.DpaSpec,
			}
			if dpa.BackupImages() {
				log.Printf("Waiting for registry pods to be running")
				Eventually(AreRegistryDeploymentsAvailable(i.GetNamespace()), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			}

		}, genericTests,
	)

	type deletionCase struct {
		WantError bool
	}
	DescribeTable("DPA Deletion test",
		func(installCase deletionCase) {
			log.Printf("Building dpa")
			err := dpaCR.Build(RESTIC)
			Expect(err).NotTo(HaveOccurred())
			log.Printf("Creating dpa")
			err = dpaCR.CreateOrUpdate(&dpaCR.CustomResource.Spec)
			Expect(err).NotTo(HaveOccurred())
			Eventually(dpaCR.DPAReconcileError(), i.GetTimeoutMultiplier()*time.Minute*2, time.Second*5).Should(BeFalse())
			log.Printf("Waiting for velero pod to be running")
			Eventually(AreVeleroDeploymentReplicasReady(i.GetNamespace()), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			if dpaCR.CustomResource.BackupImages() {
				log.Printf("Waiting for registry pods to be running")
				Eventually(AreRegistryDeploymentsAvailable(i.GetNamespace()), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			log.Printf("Deleting dpa")
			err = dpaCR.Delete()
			if installCase.WantError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				log.Printf("Checking no velero pods are running")
				Eventually(AreVeleroDeploymentReplicasReady(i.GetNamespace()), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).ShouldNot(BeTrue())
				log.Printf("Checking no registry deployment available")
				Eventually(AreRegistryDeploymentsNotAvailable(i.GetNamespace()), i.GetTimeoutMultiplier()*time.Minute*3, time.Second*5).Should(BeTrue())
			}
		},
		Entry("Should succeed", deletionCase{WantError: false}),
	)
})

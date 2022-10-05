package e2e_test

import (
	"fmt"
	"log"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("Configuration testing for DPA Custom Resource", func() {
	provider := Dpa.Spec.BackupLocations[0].Velero.Provider
	bucket := Dpa.Spec.BackupLocations[0].Velero.ObjectStorage.Bucket
	bslConfig := Dpa.Spec.BackupLocations[0].Velero.Config

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
						DefaultPlugins: Dpa.Spec.Configuration.Velero.DefaultPlugins,
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
							Provider: provider,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: VeleroPrefix,
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
						DefaultPlugins: Dpa.Spec.Configuration.Velero.DefaultPlugins,
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
							Provider: provider,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: VeleroPrefix,
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
						}, Dpa.Spec.Configuration.Velero.DefaultPlugins...),
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
							Provider: provider,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: VeleroPrefix,
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
						}, Dpa.Spec.Configuration.Velero.DefaultPlugins...),
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
							Provider: provider,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: VeleroPrefix,
								},
							},
						},
					},
				},
			},
			WantError: false,
		}, nil),
		Entry("Provider plugin", InstallCase{
			Name:         "default-cr-aws-plugin",
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginCSI,
							func () oadpv1alpha1.DefaultPlugin {
								if provider == "aws" {
									return oadpv1alpha1.DefaultPluginAWS
								} else if provider == "azure" {
									return oadpv1alpha1.DefaultPluginMicrosoftAzure
								} else if provider == "gcp" {
									return oadpv1alpha1.DefaultPluginGCP
								}
								return ""
							}(),
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
							Provider: provider,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: VeleroPrefix,
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
						DefaultPlugins: Dpa.Spec.Configuration.Velero.DefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(true),
					},
				},
				SnapshotLocations: Dpa.Spec.SnapshotLocations,
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: provider,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: VeleroPrefix,
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
						DefaultPlugins: Dpa.Spec.Configuration.Velero.DefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(false),
					},
				},
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: provider,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: VeleroPrefix,
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
						}, Dpa.Spec.Configuration.Velero.DefaultPlugins...),
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(false),
					},
				},
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: provider,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: VeleroPrefix,
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
							Provider: provider,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: VeleroPrefix,
								},
							},
						},
					},
				},
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:      &oadpv1alpha1.PodConfig{},
						DefaultPlugins: Dpa.Spec.Configuration.Velero.DefaultPlugins,
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
							Provider: provider,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: VeleroPrefix,
								},
							},
						},
					},
				},
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:      &oadpv1alpha1.PodConfig{},
						DefaultPlugins: Dpa.Spec.Configuration.Velero.DefaultPlugins,
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
						DefaultPlugins:          Dpa.Spec.Configuration.Velero.DefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(true),
					},
				},
				BackupImages: pointer.Bool(false),
			},
			WantError: false,
		}, nil),
	}

	awsTests := []TableEntry{
		Entry("AWS Without Region No S3ForcePathStyle with BackupImages false should succeed", InstallCase{
			Name:         "default-no-region-no-s3forcepathstyle",
			BRestoreType: RESTIC,
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
									Prefix: VeleroPrefix,
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
			BRestoreType: RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: provider,
							Config: map[string]string{
								"region":           bslConfig["region"],
								"s3ForcePathStyle": "true",
								"profile":          bslConfig["profile"],
							},
							Default: true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: VeleroPrefix,
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
			BRestoreType: RESTIC,
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
									Prefix: VeleroPrefix,
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

	if provider == "aws" {
		genericTests = append(genericTests, awsTests...)
	}
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
			if len(installCase.DpaSpec.BackupLocations) > 0 {
				if installCase.DpaSpec.BackupLocations[0].Velero.Config != nil {
					installCase.DpaSpec.BackupLocations[0].Velero.Config["credentialsFile"] = "bsl-cloud-credentials-" + dpaCR.Provider + "/cloud"
					if installCase.TestCarriageReturn {
						installCase.DpaSpec.BackupLocations[0].Velero.Config["credentialsFile"] = "bsl-cloud-credentials-" + dpaCR.Provider + "-with-carriage-return/cloud"
					}
				}
			}
			lastInstallingApplicationNamespace = dpaCR.Namespace
			lastInstallTime = time.Now()
			err = dpaCR.CreateOrUpdate(installCase.DpaSpec)
			Expect(err).ToNot(HaveOccurred())
			if installCase.WantError {
				// Eventually()
				log.Printf("Test case expected to error. Waiting for the error to show in DPA Status")
				Eventually(dpaCR.GetNoErr().Status.Conditions[0].Type, timeoutMultiplier*time.Minute*3, time.Second*5).Should(Equal("Reconciled"))
				Eventually(dpaCR.GetNoErr().Status.Conditions[0].Status, timeoutMultiplier*time.Minute*3, time.Second*5).Should(Equal(metav1.ConditionFalse))
				Eventually(dpaCR.GetNoErr().Status.Conditions[0].Reason, timeoutMultiplier*time.Minute*3, time.Second*5).Should(Equal("Error"))
				Eventually(dpaCR.GetNoErr().Status.Conditions[0].Message, timeoutMultiplier*time.Minute*3, time.Second*5).Should(Equal(expectedErr.Error()))
				return
			}
			// sleep to accomodates throttled CI environment
			time.Sleep(20 * time.Second)
			// Capture logs right after DPA is reconciled for diffing after one minute.
			Eventually(dpaCR.GetNoErr().Status.Conditions[0].Type, timeoutMultiplier*time.Minute*3, time.Second*5).Should(Equal("Reconciled"))
			timeReconciled := time.Now()
			adpLogsAtReconciled, err := GetOpenShiftADPLogs(dpaCR.Namespace)
			Expect(err).NotTo(HaveOccurred())
			log.Printf("Waiting for velero pod to be running")
			Eventually(AreVeleroPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			dpa, err := dpaCR.Get()
			Expect(err).NotTo(HaveOccurred())
			if len(dpa.Spec.BackupLocations) > 0 {
				log.Printf("Checking for bsl spec")
				for _, bsl := range dpa.Spec.BackupLocations {
					// Check if bsl matches the spec
					Eventually(DoesBSLExist(namespace, *bsl.Velero, installCase.DpaSpec), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}
			if len(dpa.Spec.SnapshotLocations) > 0 {
				log.Printf("Checking for vsl spec")
				for _, vsl := range dpa.Spec.SnapshotLocations {
					Eventually(DoesVSLExist(namespace, *vsl.Velero, installCase.DpaSpec), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}

			// Check for velero tolerations
			if len(dpa.Spec.Configuration.Velero.PodConfig.Tolerations) > 0 {
				log.Printf("Checking for velero tolerations")
				Eventually(VerifyVeleroTolerations(namespace, dpa.Spec.Configuration.Velero.PodConfig.Tolerations), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			// check for velero resource allocations
			if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests != nil {
				log.Printf("Checking for velero resource allocation requests")
				Eventually(VerifyVeleroResourceRequests(namespace, dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits != nil {
				log.Printf("Checking for velero resource allocation limits")
				Eventually(VerifyVeleroResourceLimits(namespace, dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			//restic installation
			if dpa.Spec.Configuration.Restic != nil && *dpa.Spec.Configuration.Restic.Enable {
				log.Printf("Waiting for restic pods to be running")
				Eventually(AreResticPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			} else {
				log.Printf("Waiting for restic daemonset to be deleted")
				Eventually(IsResticDaemonsetDeleted(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			// check defaultplugins
			log.Printf("Waiting for velero deployment to have expected plugins")
			if len(dpa.Spec.Configuration.Velero.DefaultPlugins) > 0 {
				log.Printf("Checking for default plugins")
				for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
					Eventually(DoesPluginExist(namespace, plugin), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}

			// check customplugins
			log.Printf("Waiting for velero deployment to have expected custom plugins")
			if len(dpa.Spec.Configuration.Velero.CustomPlugins) > 0 {
				log.Printf("Checking for custom plugins")
				for _, plugin := range dpa.Spec.Configuration.Velero.CustomPlugins {
					Eventually(DoesCustomPluginExist(namespace, plugin), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}

			if dpa.Spec.Configuration.Restic != nil && dpa.Spec.Configuration.Restic.PodConfig != nil {
				for key, value := range dpa.Spec.Configuration.Restic.PodConfig.NodeSelector {
					log.Printf("Waiting for restic daemonset to get node selector")
					Eventually(ResticDaemonSetHasNodeSelector(namespace, key, value), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}
			if dpa.BackupImages() {
				log.Printf("Waiting for registry pods to be running")
				Eventually(AreRegistryDeploymentsAvailable(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			// wait at least 1 minute after reconciled
			Eventually(func() bool {
				//has it been at least 1 minute since reconciled?
				return time.Now().After(timeReconciled.Add(time.Minute))
			}, timeoutMultiplier*time.Minute*2, time.Second).Should(BeTrue())
			adpLogsAfterOneMinute, err := GetOpenShiftADPLogs(dpaCR.Namespace)
			Expect(err).NotTo(HaveOccurred())
			// We expect adp logs to be the same after 1 minute
			adpLogsDiff := cmp.Diff(adpLogsAtReconciled, adpLogsAfterOneMinute)
			// If registry deployment were deleted after CR update, we expect to see a new log entry, ignore that.
			if !strings.Contains(adpLogsDiff, "Registry Deployment deleted") {
				Expect(adpLogsDiff).To(Equal(""))
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
			log.Printf("Waiting for velero pod to be running")
			Eventually(AreVeleroPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			if dpaCR.CustomResource.BackupImages() {
				log.Printf("Waiting for registry pods to be running")
				Eventually(AreRegistryDeploymentsAvailable(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			log.Printf("Deleting dpa")
			err = dpaCR.Delete()
			if installCase.WantError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				log.Printf("Checking no velero pods are running")
				Eventually(AreVeleroPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).ShouldNot(BeTrue())
				log.Printf("Checking no registry deployment available")
				Eventually(AreRegistryDeploymentsNotAvailable(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
		},
		Entry("Should succeed", deletionCase{WantError: false}),
	)
})

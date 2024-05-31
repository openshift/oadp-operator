package e2e_test

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	ginkgov2 "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

var _ = ginkgov2.Describe("Configuration testing for DPA Custom Resource", func() {
	providerFromDPA := lib.Dpa.Spec.BackupLocations[0].Velero.Provider
	bucket := lib.Dpa.Spec.BackupLocations[0].Velero.ObjectStorage.Bucket
	bslConfig := lib.Dpa.Spec.BackupLocations[0].Velero.Config
	bslCredential := corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: "bsl-cloud-credentials-" + provider,
		},
		Key: "cloud",
	}

	type InstallCase struct {
		Name               string
		BRestoreType       lib.BackupRestoreType
		DpaSpec            *oadpv1alpha1.DataProtectionApplicationSpec
		TestCarriageReturn bool
		WantError          bool
	}
	type deletionCase struct {
		WantError bool
	}

	var lastInstallingApplicationNamespace string
	var lastInstallTime time.Time
	var _ = ginkgov2.AfterEach(func(ctx ginkgov2.SpecContext) {
		report := ctx.SpecReport()
		if report.Failed() {
			baseReportDir := artifact_dir + "/" + report.LeafNodeText
			err := os.MkdirAll(baseReportDir, 0755)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			// print namespace error events for app namespace
			if lastInstallingApplicationNamespace != "" {
				lib.PrintNamespaceEventsAfterTime(kubernetesClientForSuiteRun, lastInstallingApplicationNamespace, lastInstallTime)
			}
			err = lib.SavePodLogs(kubernetesClientForSuiteRun, lastInstallingApplicationNamespace, baseReportDir)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			log.Printf("Running must gather for failed deployment test - " + report.LeafNodeText)
			err = lib.RunMustGather(oc_cli, baseReportDir+"/must-gather")
			if err != nil {
				log.Printf("Failed to run must gather: " + err.Error())
			}
		}
	})
	ginkgov2.DescribeTable("Updating custom resource with new configuration",
		func(installCase InstallCase, expectedErr error) {
			//TODO: Calling dpaCR.build() is the old pattern.
			//Change it later to make sure all the spec values are passed for every test case,
			// instead of assigning the values in advance to the DPA CR
			err := dpaCR.Build(installCase.BRestoreType)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			if len(installCase.DpaSpec.BackupLocations) > 0 {
				if installCase.DpaSpec.BackupLocations[0].Velero.Credential == nil {
					installCase.DpaSpec.BackupLocations[0].Velero.Credential = &bslCredential
				}
				if installCase.TestCarriageReturn {
					installCase.DpaSpec.BackupLocations[0].Velero.Credential = &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "bsl-cloud-credentials-" + dpaCR.Provider + "-with-carriage-return",
						},
						Key: bslCredential.Key,
					}
				}
			}
			lastInstallingApplicationNamespace = dpaCR.Namespace
			lastInstallTime = time.Now()
			err = dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, installCase.DpaSpec)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			// sleep to accommodate throttled CI environment
			// TODO this should be a function, not an arbitrary sleep
			time.Sleep(20 * time.Second)
			// Capture logs right after DPA is reconciled for diffing after one minute.
			gomega.Eventually(dpaCR.GetNoErr(runTimeClientForSuiteRun).Status.Conditions[0].Type, timeoutMultiplier*time.Minute*3, time.Second*5).Should(gomega.Equal("Reconciled"))
			if installCase.WantError {
				log.Printf("Test case expected to error. Waiting for the error to show in DPA Status")
				gomega.Eventually(dpaCR.GetNoErr(runTimeClientForSuiteRun).Status.Conditions[0].Status, timeoutMultiplier*time.Minute*3, time.Second*5).Should(gomega.Equal(metav1.ConditionFalse))
				gomega.Eventually(dpaCR.GetNoErr(runTimeClientForSuiteRun).Status.Conditions[0].Reason, timeoutMultiplier*time.Minute*3, time.Second*5).Should(gomega.Equal("Error"))
				gomega.Eventually(dpaCR.GetNoErr(runTimeClientForSuiteRun).Status.Conditions[0].Message, timeoutMultiplier*time.Minute*3, time.Second*5).Should(gomega.Equal(expectedErr.Error()))
				return
			}
			timeReconciled := time.Now()
			adpLogsAtReconciled, err := lib.GetOpenShiftADPLogs(kubernetesClientForSuiteRun, dpaCR.Namespace)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			log.Printf("Waiting for velero pod to be running")
			gomega.Eventually(lib.AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			dpa, err := dpaCR.Get(runTimeClientForSuiteRun)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			if len(dpa.Spec.BackupLocations) > 0 {
				log.Printf("Checking for bsl spec")
				for _, bsl := range dpa.Spec.BackupLocations {
					// Check if bsl matches the spec
					gomega.Expect(lib.DoesBSLSpecMatchesDpa(namespace, *bsl.Velero, installCase.DpaSpec)).To(gomega.BeTrue())
				}
			}
			if len(dpa.Spec.SnapshotLocations) > 0 {
				log.Printf("Checking for vsl spec")
				for _, vsl := range dpa.Spec.SnapshotLocations {
					gomega.Expect(lib.DoesVSLSpecMatchesDpa(namespace, *vsl.Velero, installCase.DpaSpec)).To(gomega.BeTrue())
				}
			}

			// Check for velero tolerances
			if len(dpa.Spec.Configuration.Velero.PodConfig.Tolerations) > 0 {
				log.Printf("Checking for velero tolerances")
				gomega.Eventually(lib.VerifyVeleroTolerations(kubernetesClientForSuiteRun, namespace, dpa.Spec.Configuration.Velero.PodConfig.Tolerations), timeoutMultiplier*time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			}

			// check for velero resource allocations
			if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests != nil {
				log.Printf("Checking for velero resource allocation requests")
				gomega.Eventually(lib.VerifyVeleroResourceRequests(kubernetesClientForSuiteRun, namespace, dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests), timeoutMultiplier*time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			}

			if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits != nil {
				log.Printf("Checking for velero resource allocation limits")
				gomega.Eventually(lib.VerifyVeleroResourceLimits(kubernetesClientForSuiteRun, namespace, dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits), timeoutMultiplier*time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			}

			//restic installation with new and deprecated options
			if dpa.Spec.Configuration.Restic != nil && *dpa.Spec.Configuration.Restic.Enable {
				log.Printf("Waiting for restic pods to be running")
				gomega.Eventually(lib.AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*4, time.Second*5).Should(gomega.BeTrue())
			} else if dpa.Spec.Configuration.NodeAgent != nil && *dpa.Spec.Configuration.NodeAgent.Enable {
				log.Printf("Waiting for NodeAgent pods to be running")
				gomega.Eventually(lib.AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*4, time.Second*5).Should(gomega.BeTrue())
			} else {
				log.Printf("Waiting for NodeAgent daemonset to be deleted")
				gomega.Eventually(lib.IsNodeAgentDaemonsetDeleted(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*4, time.Second*5).Should(gomega.BeTrue())
			}

			// check defaultPlugins
			log.Printf("Waiting for velero deployment to have expected plugins")
			if len(dpa.Spec.Configuration.Velero.DefaultPlugins) > 0 {
				log.Printf("Checking for default plugins")
				for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
					// CSI under DefaultPlugins no longer installs an actual initcontainer as of OADP 1.4/Velero 1.14
					if plugin != oadpv1alpha1.DefaultPluginCSI {
						gomega.Eventually(lib.DoesPluginExist(kubernetesClientForSuiteRun, namespace, plugin), timeoutMultiplier*time.Minute*6, time.Second*5).Should(gomega.BeTrue())
					}
				}
			}

			// check customPlugins
			log.Printf("Waiting for velero deployment to have expected custom plugins")
			if len(dpa.Spec.Configuration.Velero.CustomPlugins) > 0 {
				log.Printf("Checking for custom plugins")
				for _, plugin := range dpa.Spec.Configuration.Velero.CustomPlugins {
					gomega.Eventually(lib.DoesCustomPluginExist(kubernetesClientForSuiteRun, namespace, plugin), timeoutMultiplier*time.Minute*6, time.Second*5).Should(gomega.BeTrue())
				}
			}

			log.Printf("Waiting for restic daemonSet to have nodeSelector")
			if dpa.Spec.Configuration.Restic != nil && dpa.Spec.Configuration.Restic.PodConfig != nil {
				for key, value := range dpa.Spec.Configuration.Restic.PodConfig.NodeSelector {
					log.Printf("Waiting for restic daemonSet to get node selector")
					gomega.Eventually(lib.NodeAgentDaemonSetHasNodeSelector(kubernetesClientForSuiteRun, namespace, key, value), timeoutMultiplier*time.Minute*6, time.Second*5).Should(gomega.BeTrue())
				}
			}
			log.Printf("Waiting for nodeAgent daemonSet to have nodeSelector")
			if dpa.Spec.Configuration.NodeAgent != nil && dpa.Spec.Configuration.NodeAgent.PodConfig != nil {
				for key, value := range dpa.Spec.Configuration.NodeAgent.PodConfig.NodeSelector {
					log.Printf("Waiting for NodeAgent daemonSet to get node selector")
					gomega.Eventually(lib.NodeAgentDaemonSetHasNodeSelector(kubernetesClientForSuiteRun, namespace, key, value), timeoutMultiplier*time.Minute*6, time.Second*5).Should(gomega.BeTrue())
				}
			}
			// wait at least 1 minute after reconciled
			gomega.Eventually(func() bool {
				//has it been at least 1 minute since reconciled?
				log.Printf("Waiting for 1 minute after reconciled: %v elapsed", time.Since(timeReconciled).String())
				return time.Now().After(timeReconciled.Add(time.Minute))
			}, timeoutMultiplier*time.Minute*5, time.Second*5).Should(gomega.BeTrue())
			adpLogsAfterOneMinute, err := lib.GetOpenShiftADPLogs(kubernetesClientForSuiteRun, dpaCR.Namespace)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			// We expect adp logs to be the same after 1 minute
			adpLogsDiff := cmp.Diff(adpLogsAtReconciled, adpLogsAfterOneMinute)
			// If registry deployment were deleted after CR update, we expect to see a new log entry, ignore that.
			// We also ignore case where deprecated restic entry was used
			if !strings.Contains(adpLogsDiff, "Registry Deployment deleted") && !strings.Contains(adpLogsDiff, "(Deprecation Warning) Use nodeAgent instead of restic, which is deprecated and will be removed with the OADP 1.4") {
				gomega.Expect(adpLogsDiff).To(gomega.Equal(""))
			}
		},
		ginkgov2.Entry("Default velero CR", InstallCase{
			Name:         "default-cr",
			BRestoreType: lib.RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						DefaultPlugins: lib.Dpa.Spec.Configuration.Velero.DefaultPlugins,
						PodConfig:      &oadpv1alpha1.PodConfig{},
					},
					Restic: &oadpv1alpha1.ResticConfig{
						NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
							PodConfig: &oadpv1alpha1.PodConfig{},
							Enable:    pointer.Bool(true),
						},
					},
				},
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
						},
					},
				},
			},
			WantError: false,
		}, nil),
		ginkgov2.Entry("Default velero CR, test carriage return", InstallCase{
			Name:               "default-cr",
			BRestoreType:       lib.RESTIC,
			TestCarriageReturn: true,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						DefaultPlugins: lib.Dpa.Spec.Configuration.Velero.DefaultPlugins,
						PodConfig:      &oadpv1alpha1.PodConfig{},
					},
					Restic: &oadpv1alpha1.ResticConfig{
						NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
							PodConfig: &oadpv1alpha1.PodConfig{},
							Enable:    pointer.Bool(true),
						},
					},
				},
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
						},
					},
				},
			},
			WantError: false,
		}, nil),
		ginkgov2.Entry("Adding Velero custom plugin", InstallCase{
			Name:         "default-cr-velero-custom-plugin",
			BRestoreType: lib.RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: append([]oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginCSI,
						}, lib.Dpa.Spec.Configuration.Velero.DefaultPlugins...),
						CustomPlugins: []oadpv1alpha1.CustomPlugin{
							{
								Name:  "encryption-plugin",
								Image: "quay.io/konveyor/openshift-velero-plugin:latest",
							},
						},
					},
					Restic: &oadpv1alpha1.ResticConfig{
						NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
							PodConfig: &oadpv1alpha1.PodConfig{},
							Enable:    pointer.Bool(false),
						},
					},
				},
				BackupImages: pointer.Bool(false),
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
						},
					},
				},
			},
			WantError: false,
		}, nil),
		ginkgov2.Entry("Adding Velero resource allocations", InstallCase{
			Name:         "default-cr-velero-resource-alloc",
			BRestoreType: lib.RESTIC,
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
						}, lib.Dpa.Spec.Configuration.Velero.DefaultPlugins...),
					},
					Restic: &oadpv1alpha1.ResticConfig{
						NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
							PodConfig: &oadpv1alpha1.PodConfig{},
							Enable:    pointer.Bool(false),
						},
					},
				},
				BackupImages: pointer.Bool(false),
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
						},
					},
				},
			},
			WantError: false,
		}, nil),
		ginkgov2.Entry("Provider plugin", InstallCase{
			Name:         "default-cr-aws-plugin",
			BRestoreType: lib.RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginCSI,
							func() oadpv1alpha1.DefaultPlugin {
								if providerFromDPA == "aws" {
									return oadpv1alpha1.DefaultPluginAWS
								} else if providerFromDPA == "azure" {
									return oadpv1alpha1.DefaultPluginMicrosoftAzure
								} else if providerFromDPA == "gcp" {
									return oadpv1alpha1.DefaultPluginGCP
								}
								return ""
							}(),
						},
					},
					Restic: &oadpv1alpha1.ResticConfig{
						NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
							PodConfig: &oadpv1alpha1.PodConfig{},
							Enable:    pointer.Bool(false),
						},
					},
				},
				BackupImages: pointer.Bool(false),
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
						},
					},
				},
			},
			WantError: false,
		}, nil),
		ginkgov2.Entry("DPA CR with bsl and vsl", InstallCase{
			Name:         "default-cr-bsl-vsl",
			BRestoreType: lib.RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:      &oadpv1alpha1.PodConfig{},
						DefaultPlugins: lib.Dpa.Spec.Configuration.Velero.DefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
							PodConfig: &oadpv1alpha1.PodConfig{},
							Enable:    pointer.Bool(true),
						},
					},
				},
				SnapshotLocations: lib.Dpa.Spec.SnapshotLocations,
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
						},
					},
				},
			},
			WantError: false,
		}, nil),
		ginkgov2.Entry("Default velero CR with restic disabled", InstallCase{
			Name:         "default-cr-no-restic",
			BRestoreType: lib.RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:      &oadpv1alpha1.PodConfig{},
						DefaultPlugins: lib.Dpa.Spec.Configuration.Velero.DefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
							PodConfig: &oadpv1alpha1.PodConfig{},
							Enable:    pointer.Bool(false),
						},
					},
				},
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
						},
					},
				},
			},
			WantError: false,
		}, nil),
		ginkgov2.Entry("Adding CSI plugin", InstallCase{
			Name:         "default-cr-csi",
			BRestoreType: lib.RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: append([]oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginCSI,
						}, lib.Dpa.Spec.Configuration.Velero.DefaultPlugins...),
					},
					Restic: &oadpv1alpha1.ResticConfig{
						NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
							PodConfig: &oadpv1alpha1.PodConfig{},
							Enable:    pointer.Bool(false),
						},
					},
				},
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
						},
					},
				},
			},
			WantError: false,
		}, nil),
		ginkgov2.Entry("Set restic node selector", InstallCase{
			Name:         "default-cr-node-selector",
			BRestoreType: lib.RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
						},
					},
				},
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:      &oadpv1alpha1.PodConfig{},
						DefaultPlugins: lib.Dpa.Spec.Configuration.Velero.DefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
							PodConfig: &oadpv1alpha1.PodConfig{
								NodeSelector: map[string]string{
									"foo": "bar",
								},
							},
							Enable: pointer.Bool(true),
						},
					},
				},
			},
			WantError: false,
		}, nil),
		ginkgov2.Entry("Enable tolerations", InstallCase{
			Name:         "default-cr-tolerations",
			BRestoreType: lib.RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
						},
					},
				},
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:      &oadpv1alpha1.PodConfig{},
						DefaultPlugins: lib.Dpa.Spec.Configuration.Velero.DefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
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
			},
			WantError: false,
		}, nil),
		ginkgov2.Entry("NoDefaultBackupLocation", InstallCase{
			Name:         "default-cr-node-selector",
			BRestoreType: lib.RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:               &oadpv1alpha1.PodConfig{},
						NoDefaultBackupLocation: true,
						DefaultPlugins:          lib.Dpa.Spec.Configuration.Velero.DefaultPlugins,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
							PodConfig: &oadpv1alpha1.PodConfig{},
							Enable:    pointer.Bool(true),
						},
					},
				},
				BackupImages: pointer.Bool(false),
			},
			WantError: false,
		}, nil),
		ginkgov2.Entry("AWS Without Region No S3ForcePathStyle with BackupImages false should succeed", ginkgov2.Label("aws", "ibmcloud"), InstallCase{
			Name:         "default-no-region-no-s3forcepathstyle",
			BRestoreType: lib.RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupImages: pointer.Bool(false),
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
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
		ginkgov2.Entry("AWS With Region And S3ForcePathStyle should succeed", ginkgov2.Label("aws", "ibmcloud"), InstallCase{
			Name:         "default-with-region-and-s3forcepathstyle",
			BRestoreType: lib.RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config: map[string]string{
								"region":           bslConfig["region"],
								"s3ForcePathStyle": "true",
								"profile":          bslConfig["profile"],
							},
							Default: true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
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
		ginkgov2.Entry("AWS Without Region And S3ForcePathStyle true should fail", ginkgov2.Label("aws", "ibmcloud"), InstallCase{
			Name:         "default-with-region-and-s3forcepathstyle",
			BRestoreType: lib.RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config: map[string]string{
								"s3ForcePathStyle": "true",
							},
							Default: true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
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
		}, fmt.Errorf("region for AWS backupstoragelocation not automatically discoverable. Please set the region in the backupstoragelocation config")),
		ginkgov2.Entry("unsupportedOverrides should succeed", ginkgov2.Label("aws", "ibmcloud"), InstallCase{
			Name:         "valid-unsupported-overrides",
			BRestoreType: lib.RESTIC,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config: map[string]string{
								"profile":          bslConfig["profile"],
								"region":           bslConfig["region"],
								"s3ForcePathStyle": "true",
							},
							Default: true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: lib.VeleroPrefix,
								},
							},
							Credential: &bslCredential,
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
				UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
					"awsPluginImageFqin": "quay.io/konveyor/velero-plugin-for-aws:latest",
				},
			},
			WantError: false,
		}, nil),
	)

	ginkgov2.DescribeTable("DPA / Restic Deletion test",
		func(installCase deletionCase) {
			log.Printf("Building dpa with restic")
			err := dpaCR.Build(lib.RESTIC)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			log.Printf("Creating dpa with restic")
			err = dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, &dpaCR.CustomResource.Spec)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			log.Printf("Waiting for velero pod with restic to be running")
			gomega.Eventually(lib.AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			log.Printf("Deleting dpa with restic")
			err = dpaCR.Delete(runTimeClientForSuiteRun)
			if installCase.WantError {
				gomega.Expect(err).To(gomega.HaveOccurred())
			} else {
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				log.Printf("Checking no velero pods with restic are running")
				gomega.Eventually(lib.AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).ShouldNot(gomega.BeTrue())
			}
		},
		ginkgov2.Entry("Should succeed", deletionCase{WantError: false}),
	)

	ginkgov2.DescribeTable("DPA / Kopia Deletion test",
		func(installCase deletionCase) {
			log.Printf("Building dpa with kopia")
			err := dpaCR.Build(lib.KOPIA)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			log.Printf("Creating dpa with kopia")
			err = dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, &dpaCR.CustomResource.Spec)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			log.Printf("Waiting for velero pod with kopia to be running")
			gomega.Eventually(lib.AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			log.Printf("Deleting dpa with kopia")
			err = dpaCR.Delete(runTimeClientForSuiteRun)
			if installCase.WantError {
				gomega.Expect(err).To(gomega.HaveOccurred())
			} else {
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				log.Printf("Checking no velero pods with kopia are running")
				gomega.Eventually(lib.AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).ShouldNot(gomega.BeTrue())
			}
		},
		ginkgov2.Entry("Should succeed", deletionCase{WantError: false}),
	)
})

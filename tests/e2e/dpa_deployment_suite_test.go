package e2e_test

import (
	"fmt"
	"log"
	"os"
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
	providerFromDPA := Dpa.Spec.BackupLocations[0].Velero.Provider
	bucket := Dpa.Spec.BackupLocations[0].Velero.ObjectStorage.Bucket
	bslConfig := Dpa.Spec.BackupLocations[0].Velero.Config
	bslCredential := corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: "bsl-cloud-credentials-" + provider,
		},
		Key: "cloud",
	}

	type InstallCase struct {
		Name               string
		BRestoreType       BackupRestoreType
		DpaSpec            *oadpv1alpha1.DataProtectionApplicationSpec
		TestCarriageReturn bool
		WantError          bool
	}
	type deletionCase struct {
		WantError bool
	}

	var lastInstallingApplicationNamespace string
	var lastInstallTime time.Time
	var _ = AfterEach(func(ctx SpecContext) {
		report := ctx.SpecReport()
		if report.Failed() {
			baseReportDir := artifact_dir + "/" + report.LeafNodeText
			err := os.MkdirAll(baseReportDir, 0755)
			Expect(err).NotTo(HaveOccurred())
			// print namespace error events for app namespace
			if lastInstallingApplicationNamespace != "" {
				PrintNamespaceEventsAfterTime(kubernetesClientForSuiteRun, lastInstallingApplicationNamespace, lastInstallTime)
			}
			err = SavePodLogs(kubernetesClientForSuiteRun, lastInstallingApplicationNamespace, baseReportDir)
			Expect(err).NotTo(HaveOccurred())
			log.Printf("Running must gather for failed deployment test - " + report.LeafNodeText)
			err = RunMustGather(oc_cli, baseReportDir+"/must-gather")
			if err != nil {
				log.Printf("Failed to run must gather: " + err.Error())
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
			Expect(err).ToNot(HaveOccurred())
			// sleep to accommodate throttled CI environment
			// TODO this should be a function, not an arbitrary sleep
			time.Sleep(20 * time.Second)
			// Capture logs right after DPA is reconciled for diffing after one minute.
			Eventually(dpaCR.GetNoErr(runTimeClientForSuiteRun).Status.Conditions[0].Type, timeoutMultiplier*time.Minute*3, time.Second*5).Should(Equal("Reconciled"))
			if installCase.WantError {
				log.Printf("Test case expected to error. Waiting for the error to show in DPA Status")
				Eventually(dpaCR.GetNoErr(runTimeClientForSuiteRun).Status.Conditions[0].Status, timeoutMultiplier*time.Minute*3, time.Second*5).Should(Equal(metav1.ConditionFalse))
				Eventually(dpaCR.GetNoErr(runTimeClientForSuiteRun).Status.Conditions[0].Reason, timeoutMultiplier*time.Minute*3, time.Second*5).Should(Equal("Error"))
				Eventually(dpaCR.GetNoErr(runTimeClientForSuiteRun).Status.Conditions[0].Message, timeoutMultiplier*time.Minute*3, time.Second*5).Should(Equal(expectedErr.Error()))
				return
			}
			timeReconciled := time.Now()
			adpLogsAtReconciled, err := GetOpenShiftADPLogs(kubernetesClientForSuiteRun, dpaCR.Namespace)
			Expect(err).NotTo(HaveOccurred())
			log.Printf("Waiting for velero pod to be running")
			Eventually(AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			dpa, err := dpaCR.Get(runTimeClientForSuiteRun)
			Expect(err).NotTo(HaveOccurred())
			if len(dpa.Spec.BackupLocations) > 0 {
				log.Printf("Checking for bsl spec")
				for _, bsl := range dpa.Spec.BackupLocations {
					// Check if bsl matches the spec
					Expect(DoesBSLSpecMatchesDpa(namespace, *bsl.Velero, installCase.DpaSpec)).To(BeTrue())
				}
			} else {
				log.Println("Checking no BSLs are deployed")
				_, err = dpaCR.ListBSLs()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(fmt.Sprintf("no BSL in %s namespace", namespace)))
			}
			if len(dpa.Spec.SnapshotLocations) > 0 {
				log.Printf("Checking for vsl spec")
				for _, vsl := range dpa.Spec.SnapshotLocations {
					Expect(DoesVSLSpecMatchesDpa(namespace, *vsl.Velero, installCase.DpaSpec)).To(BeTrue())
				}
			} else {
				log.Println("Checking no VSLs are deployed")
				_, err = dpaCR.ListVSLs()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(fmt.Sprintf("no VSL in %s namespace", namespace)))
			}

			// Check for velero tolerances
			if len(dpa.Spec.Configuration.Velero.PodConfig.Tolerations) > 0 {
				log.Printf("Checking for velero tolerances")
				Eventually(VerifyVeleroTolerations(kubernetesClientForSuiteRun, namespace, dpa.Spec.Configuration.Velero.PodConfig.Tolerations), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			// check for velero resource allocations
			if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests != nil {
				log.Printf("Checking for velero resource allocation requests")
				Eventually(VerifyVeleroResourceRequests(kubernetesClientForSuiteRun, namespace, dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits != nil {
				log.Printf("Checking for velero resource allocation limits")
				Eventually(VerifyVeleroResourceLimits(kubernetesClientForSuiteRun, namespace, dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			//restic installation with new and deprecated options
			if dpa.Spec.Configuration.Restic != nil && *dpa.Spec.Configuration.Restic.Enable {
				log.Printf("Waiting for restic pods to be running")
				Eventually(AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*4, time.Second*5).Should(BeTrue())
			} else if dpa.Spec.Configuration.NodeAgent != nil && *dpa.Spec.Configuration.NodeAgent.Enable {
				log.Printf("Waiting for NodeAgent pods to be running")
				Eventually(AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*4, time.Second*5).Should(BeTrue())
			} else {
				log.Printf("Waiting for NodeAgent daemonset to be deleted")
				Eventually(IsNodeAgentDaemonsetDeleted(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*4, time.Second*5).Should(BeTrue())
			}

			// check defaultPlugins
			log.Printf("Waiting for velero deployment to have expected plugins")
			if len(dpa.Spec.Configuration.Velero.DefaultPlugins) > 0 {
				log.Printf("Checking for default plugins")
				for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
					Eventually(DoesPluginExist(kubernetesClientForSuiteRun, namespace, plugin), timeoutMultiplier*time.Minute*6, time.Second*5).Should(BeTrue())
				}
			}

			// check customPlugins
			log.Printf("Waiting for velero deployment to have expected custom plugins")
			if len(dpa.Spec.Configuration.Velero.CustomPlugins) > 0 {
				log.Printf("Checking for custom plugins")
				for _, plugin := range dpa.Spec.Configuration.Velero.CustomPlugins {
					Eventually(DoesCustomPluginExist(kubernetesClientForSuiteRun, namespace, plugin), timeoutMultiplier*time.Minute*6, time.Second*5).Should(BeTrue())
				}
			}

			log.Printf("Waiting for restic daemonSet to have nodeSelector")
			if dpa.Spec.Configuration.Restic != nil && dpa.Spec.Configuration.Restic.PodConfig != nil {
				for key, value := range dpa.Spec.Configuration.Restic.PodConfig.NodeSelector {
					log.Printf("Waiting for restic daemonSet to get node selector")
					Eventually(NodeAgentDaemonSetHasNodeSelector(kubernetesClientForSuiteRun, namespace, key, value), timeoutMultiplier*time.Minute*6, time.Second*5).Should(BeTrue())
				}
			}
			log.Printf("Waiting for nodeAgent daemonSet to have nodeSelector")
			if dpa.Spec.Configuration.NodeAgent != nil && dpa.Spec.Configuration.NodeAgent.PodConfig != nil {
				for key, value := range dpa.Spec.Configuration.NodeAgent.PodConfig.NodeSelector {
					log.Printf("Waiting for NodeAgent daemonSet to get node selector")
					Eventually(NodeAgentDaemonSetHasNodeSelector(kubernetesClientForSuiteRun, namespace, key, value), timeoutMultiplier*time.Minute*6, time.Second*5).Should(BeTrue())
				}
			}
			// wait at least 1 minute after reconciled
			Eventually(func() bool {
				//has it been at least 1 minute since reconciled?
				log.Printf("Waiting for 1 minute after reconciled: %v elapsed", time.Since(timeReconciled).String())
				return time.Now().After(timeReconciled.Add(time.Minute))
			}, timeoutMultiplier*time.Minute*5, time.Second*5).Should(BeTrue())
			adpLogsAfterOneMinute, err := GetOpenShiftADPLogs(kubernetesClientForSuiteRun, dpaCR.Namespace)
			Expect(err).NotTo(HaveOccurred())
			// We expect adp logs to be the same after 1 minute
			adpLogsDiff := cmp.Diff(adpLogsAtReconciled, adpLogsAfterOneMinute)
			// If registry deployment were deleted after CR update, we expect to see a new log entry, ignore that.
			// We also ignore case where deprecated restic entry was used
			if !strings.Contains(adpLogsDiff, "Registry Deployment deleted") && !strings.Contains(adpLogsDiff, "(Deprecation Warning) Use nodeAgent instead of restic, which is deprecated and will be removed with the OADP 1.4") {
				Expect(adpLogsDiff).To(Equal(""))
			}
		},
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
									Prefix: VeleroPrefix,
								},
							},
							Credential: &bslCredential,
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
									Prefix: VeleroPrefix,
								},
							},
							Credential: &bslCredential,
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
									Prefix: VeleroPrefix,
								},
							},
							Credential: &bslCredential,
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
									Prefix: VeleroPrefix,
								},
							},
							Credential: &bslCredential,
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
									Prefix: VeleroPrefix,
								},
							},
							Credential: &bslCredential,
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
						NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
							PodConfig: &oadpv1alpha1.PodConfig{},
							Enable:    pointer.Bool(true),
						},
					},
				},
				SnapshotLocations: Dpa.Spec.SnapshotLocations,
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
							Provider: providerFromDPA,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: VeleroPrefix,
								},
							},
							Credential: &bslCredential,
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
									Prefix: VeleroPrefix,
								},
							},
							Credential: &bslCredential,
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
									Prefix: VeleroPrefix,
								},
							},
							Credential: &bslCredential,
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
							Provider: providerFromDPA,
							Config:   bslConfig,
							Default:  true,
							StorageType: velero.StorageType{
								ObjectStorage: &velero.ObjectStorageLocation{
									Bucket: bucket,
									Prefix: VeleroPrefix,
								},
							},
							Credential: &bslCredential,
						},
					},
				},
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:      &oadpv1alpha1.PodConfig{},
						DefaultPlugins: Dpa.Spec.Configuration.Velero.DefaultPlugins,
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
		Entry("Enable tolerations", InstallCase{
			Name:         "default-cr-tolerations",
			BRestoreType: RESTIC,
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
									Prefix: VeleroPrefix,
								},
							},
							Credential: &bslCredential,
						},
					},
				},
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:      &oadpv1alpha1.PodConfig{},
						DefaultPlugins: Dpa.Spec.Configuration.Velero.DefaultPlugins,
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
		Entry("AWS Without Region No S3ForcePathStyle with BackupImages false should succeed", Label("aws", "ibmcloud"), InstallCase{
			Name:         "default-no-region-no-s3forcepathstyle",
			BRestoreType: RESTIC,
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
									Prefix: VeleroPrefix,
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
		Entry("AWS With Region And S3ForcePathStyle should succeed", Label("aws", "ibmcloud"), InstallCase{
			Name:         "default-with-region-and-s3forcepathstyle",
			BRestoreType: RESTIC,
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
									Prefix: VeleroPrefix,
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
		Entry("AWS Without Region And S3ForcePathStyle true should fail", Label("aws", "ibmcloud"), InstallCase{
			Name:         "default-with-region-and-s3forcepathstyle",
			BRestoreType: RESTIC,
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
									Prefix: VeleroPrefix,
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
		Entry("unsupportedOverrides should succeed", Label("aws", "ibmcloud"), InstallCase{
			Name:         "valid-unsupported-overrides",
			BRestoreType: RESTIC,
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
									Prefix: VeleroPrefix,
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

	DescribeTable("DPA / Restic Deletion test",
		func(installCase deletionCase) {
			log.Printf("Building dpa with restic")
			err := dpaCR.Build(RESTIC)
			Expect(err).NotTo(HaveOccurred())
			log.Printf("Creating dpa with restic")
			err = dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, &dpaCR.CustomResource.Spec)
			Expect(err).NotTo(HaveOccurred())
			log.Printf("Waiting for velero pod with restic to be running")
			Eventually(AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			log.Printf("Deleting dpa with restic")
			err = dpaCR.Delete(runTimeClientForSuiteRun)
			if installCase.WantError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				log.Printf("Checking no velero pods with restic are running")
				Eventually(AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).ShouldNot(BeTrue())
			}
		},
		Entry("Should succeed", deletionCase{WantError: false}),
	)

	DescribeTable("DPA / Kopia Deletion test",
		func(installCase deletionCase) {
			log.Printf("Building dpa with kopia")
			err := dpaCR.Build(KOPIA)
			Expect(err).NotTo(HaveOccurred())
			log.Printf("Creating dpa with kopia")
			err = dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, &dpaCR.CustomResource.Spec)
			Expect(err).NotTo(HaveOccurred())
			log.Printf("Waiting for velero pod with kopia to be running")
			Eventually(AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			log.Printf("Deleting dpa with kopia")
			err = dpaCR.Delete(runTimeClientForSuiteRun)
			if installCase.WantError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				log.Printf("Checking no velero pods with kopia are running")
				Eventually(AreVeleroPodsRunning(kubernetesClientForSuiteRun, namespace), timeoutMultiplier*time.Minute*3, time.Second*5).ShouldNot(BeTrue())
			}
		},
		Entry("Should succeed", deletionCase{WantError: false}),
	)
})

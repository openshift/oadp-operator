package e2e_test

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	. "github.com/openshift/oadp-operator/tests/e2e/lib"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

type TestDPASpec struct {
	BSLSecretName           string
	CustomPlugins           []oadpv1alpha1.CustomPlugin
	SnapshotLocations       []oadpv1alpha1.SnapshotLocation
	VeleroPodConfig         oadpv1alpha1.PodConfig
	ResticPodConfig         oadpv1alpha1.PodConfig
	NodeAgentPodConfig      oadpv1alpha1.PodConfig
	EnableRestic            bool
	EnableNodeAgent         bool
	NoDefaultBackupLocation bool
	s3ForcePathStyle        bool
	NoS3ForcePathStyle      bool
	NoRegion                bool
	DoNotBackupImages       bool
	UnsupportedOverrides    map[oadpv1alpha1.UnsupportedImageKey]string
}

func createTestDPASpec(testSpec TestDPASpec) *oadpv1alpha1.DataProtectionApplicationSpec {
	dpaSpec := &oadpv1alpha1.DataProtectionApplicationSpec{
		BackupLocations: []oadpv1alpha1.BackupLocation{
			{
				Velero: &velero.BackupStorageLocationSpec{
					Config: dpaCR.BSLConfig,
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: testSpec.BSLSecretName,
						},
						Key: "cloud",
					},
					Default: true,
					StorageType: velero.StorageType{
						ObjectStorage: &velero.ObjectStorageLocation{
							Bucket: dpaCR.BSLBucket,
							Prefix: dpaCR.BSLBucketPrefix,
						},
					},
					Provider: dpaCR.BSLProvider,
				},
			},
		},
		Configuration: &oadpv1alpha1.ApplicationConfig{
			Velero: &oadpv1alpha1.VeleroConfig{
				CustomPlugins:  testSpec.CustomPlugins,
				DefaultPlugins: dpaCR.VeleroDefaultPlugins,
				PodConfig:      &testSpec.VeleroPodConfig,
			},
		},
		SnapshotLocations:    testSpec.SnapshotLocations,
		UnsupportedOverrides: testSpec.UnsupportedOverrides,
	}
	if testSpec.EnableNodeAgent {
		dpaSpec.Configuration.NodeAgent = &oadpv1alpha1.NodeAgentConfig{
			NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
				Enable:    ptr.To(testSpec.EnableNodeAgent),
				PodConfig: &testSpec.NodeAgentPodConfig,
			},
			UploaderType: "kopia",
		}
	} else {
		dpaSpec.Configuration.Restic = &oadpv1alpha1.ResticConfig{
			NodeAgentCommonFields: oadpv1alpha1.NodeAgentCommonFields{
				Enable:    ptr.To(testSpec.EnableRestic),
				PodConfig: &testSpec.ResticPodConfig,
			},
		}
	}
	if len(testSpec.SnapshotLocations) > 0 {
		dpaSpec.SnapshotLocations[0].Velero.Credential = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: vslSecretName,
			},
			Key: "cloud",
		}
	}
	if testSpec.NoDefaultBackupLocation {
		dpaSpec.Configuration.Velero.NoDefaultBackupLocation = true
		dpaSpec.BackupLocations = []oadpv1alpha1.BackupLocation{}
	}
	if testSpec.s3ForcePathStyle {
		configWithS3ForcePathStyle := map[string]string{}
		for key, value := range dpaSpec.BackupLocations[0].Velero.Config {
			configWithS3ForcePathStyle[key] = value
		}
		configWithS3ForcePathStyle["s3ForcePathStyle"] = "true"
		dpaSpec.BackupLocations[0].Velero.Config = configWithS3ForcePathStyle
	}
	if testSpec.NoRegion {
		configWithoutRegion := map[string]string{}
		for key, value := range dpaSpec.BackupLocations[0].Velero.Config {
			if key != "region" {
				configWithoutRegion[key] = value
			}
		}
		dpaSpec.BackupLocations[0].Velero.Config = configWithoutRegion
	}
	if testSpec.NoS3ForcePathStyle {
		configWithoutRegion := map[string]string{}
		for key, value := range dpaSpec.BackupLocations[0].Velero.Config {
			if key != "s3ForcePathStyle" {
				configWithoutRegion[key] = value
			}
		}
		dpaSpec.BackupLocations[0].Velero.Config = configWithoutRegion
	}
	if testSpec.DoNotBackupImages {
		dpaSpec.BackupImages = ptr.To(false)
	}
	return dpaSpec
}

var _ = Describe("Configuration testing for DPA Custom Resource", func() {
	type InstallCase struct {
		DpaSpec *oadpv1alpha1.DataProtectionApplicationSpec
	}

	var lastInstallTime time.Time
	var _ = AfterEach(func(ctx SpecContext) {
		report := ctx.SpecReport()
		if report.Failed() {
			getFailedTestLogs(namespace, "", lastInstallTime, report)
		}
	})
	DescribeTable("DPA reconciled to true",
		func(installCase InstallCase) {
			lastInstallTime = time.Now()
			err := dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, installCase.DpaSpec)
			Expect(err).ToNot(HaveOccurred())

			Eventually(dpaCR.IsReconciledTrue(), time.Minute*2, time.Second*5).Should(BeTrue())
			// TODO do not use Consistently, using because no field in DPA is updated telling when it was last reconciled
			Consistently(dpaCR.IsReconciledTrue(), time.Minute*1, time.Second*15).Should(BeTrue())

			timeReconciled := time.Now()
			adpLogsAtReconciled, err := GetManagerPodLogs(kubernetesClientForSuiteRun, dpaCR.Namespace)
			Expect(err).NotTo(HaveOccurred())

			log.Printf("Waiting for velero Pod to be running")
			// TODO do not use Consistently
			Consistently(VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*1, time.Second*15).Should(BeTrue())
			timeAfterVeleroIsRunning := time.Now()

			if installCase.DpaSpec.Configuration.Restic != nil && *installCase.DpaSpec.Configuration.Restic.Enable {
				log.Printf("Waiting for restic Pods to be running")
				Eventually(AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(BeTrue())
				if installCase.DpaSpec.Configuration.Restic.PodConfig != nil {
					log.Printf("Waiting for restic DaemonSet to have nodeSelector")
					for key, value := range installCase.DpaSpec.Configuration.Restic.PodConfig.NodeSelector {
						log.Printf("Waiting for restic DaemonSet to get node selector")
						Eventually(NodeAgentDaemonSetHasNodeSelector(kubernetesClientForSuiteRun, namespace, key, value), time.Minute*6, time.Second*5).Should(BeTrue())
					}
				}
			} else if installCase.DpaSpec.Configuration.NodeAgent != nil && *installCase.DpaSpec.Configuration.NodeAgent.Enable {
				log.Printf("Waiting for NodeAgent Pods to be running")
				Eventually(AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(BeTrue())
				if installCase.DpaSpec.Configuration.NodeAgent.PodConfig != nil {
					log.Printf("Waiting for NodeAgent DaemonSet to have nodeSelector")
					for key, value := range installCase.DpaSpec.Configuration.NodeAgent.PodConfig.NodeSelector {
						log.Printf("Waiting for NodeAgent DaemonSet to get node selector")
						Eventually(NodeAgentDaemonSetHasNodeSelector(kubernetesClientForSuiteRun, namespace, key, value), time.Minute*6, time.Second*5).Should(BeTrue())
					}
				}
			} else {
				log.Printf("Waiting for NodeAgent DaemonSet to be deleted")
				Eventually(IsNodeAgentDaemonSetDeleted(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(BeTrue())
			}

			if len(installCase.DpaSpec.BackupLocations) > 0 {
				log.Print("Checking if BSLs are available")
				Eventually(dpaCR.BSLsAreUpdated(timeAfterVeleroIsRunning), time.Minute*3, time.Second*5).Should(BeTrue())
				Eventually(dpaCR.BSLsAreAvailable(), time.Minute*3, time.Second*5).Should(BeTrue())
				for _, bsl := range installCase.DpaSpec.BackupLocations {
					log.Printf("Checking for BSL spec")
					Expect(dpaCR.DoesBSLSpecMatchesDpa(namespace, *bsl.Velero)).To(BeTrue())
				}
			} else {
				log.Println("Checking no BSLs are deployed")
				_, err = dpaCR.ListBSLs()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(fmt.Sprintf("no BSL in %s namespace", namespace)))
			}

			if len(installCase.DpaSpec.SnapshotLocations) > 0 {
				// TODO Check if VSLs are available creating new backup/restore test with VSL
				for _, vsl := range installCase.DpaSpec.SnapshotLocations {
					log.Printf("Checking for VSL spec")
					Expect(dpaCR.DoesVSLSpecMatchesDpa(namespace, *vsl.Velero)).To(BeTrue())
				}
			} else {
				log.Println("Checking no VSLs are deployed")
				_, err = dpaCR.ListVSLs()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(fmt.Sprintf("no VSL in %s namespace", namespace)))
			}

			if len(installCase.DpaSpec.Configuration.Velero.PodConfig.Tolerations) > 0 {
				log.Printf("Checking for velero tolerances")
				Eventually(VerifyVeleroTolerations(kubernetesClientForSuiteRun, namespace, installCase.DpaSpec.Configuration.Velero.PodConfig.Tolerations), time.Minute*3, time.Second*5).Should(BeTrue())
			}

			if installCase.DpaSpec.Configuration.Velero.PodConfig.ResourceAllocations.Requests != nil {
				log.Printf("Checking for velero resource allocation requests")
				Eventually(VerifyVeleroResourceRequests(kubernetesClientForSuiteRun, namespace, installCase.DpaSpec.Configuration.Velero.PodConfig.ResourceAllocations.Requests), time.Minute*3, time.Second*5).Should(BeTrue())
			}

			if installCase.DpaSpec.Configuration.Velero.PodConfig.ResourceAllocations.Limits != nil {
				log.Printf("Checking for velero resource allocation limits")
				Eventually(VerifyVeleroResourceLimits(kubernetesClientForSuiteRun, namespace, installCase.DpaSpec.Configuration.Velero.PodConfig.ResourceAllocations.Limits), time.Minute*3, time.Second*5).Should(BeTrue())
			}

			if len(installCase.DpaSpec.Configuration.Velero.DefaultPlugins) > 0 {
				log.Printf("Waiting for velero Deployment to have expected default plugins")
				for _, plugin := range installCase.DpaSpec.Configuration.Velero.DefaultPlugins {
					log.Printf("Checking for %s default plugin", plugin)
					Eventually(DoesPluginExist(kubernetesClientForSuiteRun, namespace, plugin), time.Minute*6, time.Second*5).Should(BeTrue())
				}
			}

			if len(installCase.DpaSpec.Configuration.Velero.CustomPlugins) > 0 {
				log.Printf("Waiting for velero Deployment to have expected custom plugins")
				for _, plugin := range installCase.DpaSpec.Configuration.Velero.CustomPlugins {
					log.Printf("Checking for %s custom plugin", plugin.Name)
					Eventually(DoesCustomPluginExist(kubernetesClientForSuiteRun, namespace, plugin), time.Minute*6, time.Second*5).Should(BeTrue())
				}
			}

			// wait at least 1 minute after reconciled
			Eventually(
				func() bool {
					//has it been at least 1 minute since reconciled?
					log.Printf("Waiting for 1 minute after reconciled: %v elapsed", time.Since(timeReconciled).String())
					return time.Now().After(timeReconciled.Add(time.Minute))
				},
				time.Minute*5, time.Second*5,
			).Should(BeTrue())
			adpLogsAfterOneMinute, err := GetManagerPodLogs(kubernetesClientForSuiteRun, dpaCR.Namespace)
			Expect(err).NotTo(HaveOccurred())
			// We expect OADP logs to be the same after 1 minute
			adpLogsDiff := cmp.Diff(adpLogsAtReconciled, adpLogsAfterOneMinute)
			// If registry deployment were deleted after CR update, we expect to see a new log entry, ignore that.
			// We also ignore case where deprecated restic entry was used
			if !strings.Contains(adpLogsDiff, "Registry Deployment deleted") && !strings.Contains(adpLogsDiff, "(Deprecation Warning) Use nodeAgent instead of restic, which is deprecated and will be removed in the future") {
				Expect(adpLogsDiff).To(Equal(""))
			}
		},
		Entry("Default DPA CR", InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{BSLSecretName: bslSecretName}),
		}),
		Entry("DPA CR with BSL secret with carriage return", InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{BSLSecretName: bslSecretNameWithCarriageReturn}),
		}),
		Entry("DPA CR with Velero custom plugin", InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName: bslSecretName,
				CustomPlugins: []oadpv1alpha1.CustomPlugin{
					{
						Name:  "encryption-plugin",
						Image: "quay.io/konveyor/openshift-velero-plugin:oadp-1.4",
					},
				},
			}),
		}),
		Entry("DPA CR with Velero resource allocations", InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName: bslSecretName,
				VeleroPodConfig: oadpv1alpha1.PodConfig{
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
			}),
		}),
		Entry("DPA CR with Velero toleration", InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName: bslSecretName,
				VeleroPodConfig: oadpv1alpha1.PodConfig{
					Tolerations: []corev1.Toleration{
						{
							Key:               "node.kubernetes.io/unreachable",
							Operator:          "Exists",
							Effect:            "NoExecute",
							TolerationSeconds: ptr.To(int64(6000)),
						},
					},
				},
			}),
		}),
		Entry("DPA CR with VSL", Label("aws", "azure", "gcp"), InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName:     bslSecretName,
				SnapshotLocations: dpaCR.SnapshotLocations,
			}),
		}),
		Entry("DPA CR with restic enabled with node selector", InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName: bslSecretName,
				EnableRestic:  true,
				ResticPodConfig: oadpv1alpha1.PodConfig{
					NodeSelector: map[string]string{
						"foo": "bar",
					},
				},
			}),
		}),
		Entry("DPA CR with kopia enabled with node selector", InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName:   bslSecretName,
				EnableNodeAgent: true,
				NodeAgentPodConfig: oadpv1alpha1.PodConfig{
					NodeSelector: map[string]string{
						"foo": "bar",
					},
				},
			}),
		}),
		Entry("DPA CR with NoDefaultBackupLocation and with BackupImages false", InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName:           bslSecretName,
				NoDefaultBackupLocation: true,
				DoNotBackupImages:       true,
			}),
		}),
		Entry("DPA CR with S3ForcePathStyle true", Label("aws"), InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName:    bslSecretName,
				s3ForcePathStyle: true,
			}),
		}),
		// TODO bug https://github.com/vmware-tanzu/velero/issues/8022
		// Entry("DPA CR without Region, without S3ForcePathStyle and with BackupImages false", Label("aws"), InstallCase{
		// 	DpaSpec: createTestDPASpec(TestDPASpec{
		// 		BSLSecretName:      bslSecretName,
		// 		NoRegion:           true,
		// 		NoS3ForcePathStyle: true,
		// 		DoNotBackupImages:  true,
		// 	}),
		// }),
		Entry("DPA CR with unsupportedOverrides", Label("aws", "ibmcloud"), InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName: bslSecretName,
				UnsupportedOverrides: map[oadpv1alpha1.UnsupportedImageKey]string{
					"awsPluginImageFqin": "quay.io/konveyor/velero-plugin-for-aws:oadp-1.4",
				},
			}),
		}),
	)

	DescribeTable("DPA reconciled to false",
		func(installCase InstallCase, message string) {
			lastInstallTime = time.Now()
			err := dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, installCase.DpaSpec)
			Expect(err).ToNot(HaveOccurred())

			log.Printf("Test case expected to error. Waiting for the error to show in DPA Status")
			Eventually(dpaCR.IsReconciledFalse(message), time.Minute*3, time.Second*5).Should(BeTrue())
		},
		Entry("DPA CR without Region and with S3ForcePathStyle true", Label("aws", "ibmcloud"), InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName:    bslSecretName,
				NoRegion:         true,
				s3ForcePathStyle: true,
			}),
		}, "region for AWS backupstoragelocation not automatically discoverable. Please set the region in the backupstoragelocation config"),
	)

	DescribeTable("DPA Deletion test",
		func() {
			log.Printf("Creating DPA")
			err := dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, dpaCR.Build(KOPIA))
			Expect(err).NotTo(HaveOccurred())
			log.Printf("Waiting for velero Pod to be running")
			Eventually(VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(BeTrue())
			log.Printf("Deleting DPA")
			err = dpaCR.Delete()
			Expect(err).NotTo(HaveOccurred())
			log.Printf("Waiting for velero to be deleted")
			Eventually(VeleroIsDeleted(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(BeTrue())
		},
		Entry("Should succeed"),
	)
})

package e2e_test

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/tests/e2e/lib"
)

type TestDPASpec struct {
	BSLSecretName           string
	DefaultPlugins          []oadpv1alpha1.DefaultPlugin // Overrides default plugins loaded from config
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
		UnsupportedOverrides: dpaCR.UnsupportedOverrides,
	}
	if len(testSpec.DefaultPlugins) > 0 {
		dpaSpec.Configuration.Velero.DefaultPlugins = testSpec.DefaultPlugins
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

var _ = ginkgo.Describe("Configuration testing for DPA Custom Resource", func() {
	type InstallCase struct {
		DpaSpec *oadpv1alpha1.DataProtectionApplicationSpec
	}
	var lastInstallTime time.Time
	var _ = ginkgo.AfterEach(func(ctx ginkgo.SpecContext) {
		report := ctx.SpecReport()
		if report.Failed() {
			getFailedTestLogs(namespace, "", lastInstallTime, report)
		}
	})
	ginkgo.DescribeTable("DPA reconciled to true",
		func(installCase InstallCase) {
			lastInstallTime = time.Now()
			err := dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, installCase.DpaSpec)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(dpaCR.IsReconciledTrue(), time.Minute*2, time.Second*5).Should(gomega.BeTrue())
			// TODO do not use Consistently, using because no field in DPA is updated telling when it was last reconciled
			gomega.Consistently(dpaCR.IsReconciledTrue(), time.Minute*1, time.Second*15).Should(gomega.BeTrue())

			timeReconciled := time.Now()
			adpLogsAtReconciled, err := lib.GetManagerPodLogs(kubernetesClientForSuiteRun, dpaCR.Namespace)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			log.Printf("Waiting for velero Pod to be running")
			// TODO do not use Consistently
			gomega.Consistently(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*1, time.Second*15).Should(gomega.BeTrue())
			timeAfterVeleroIsRunning := time.Now()

			if installCase.DpaSpec.Configuration.Restic != nil && *installCase.DpaSpec.Configuration.Restic.Enable {
				log.Printf("Waiting for restic Pods to be running")
				gomega.Eventually(lib.AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
				if installCase.DpaSpec.Configuration.Restic.PodConfig != nil {
					log.Printf("Waiting for restic DaemonSet to have nodeSelector")
					for key, value := range installCase.DpaSpec.Configuration.Restic.PodConfig.NodeSelector {
						log.Printf("Waiting for restic DaemonSet to get node selector")
						gomega.Eventually(lib.NodeAgentDaemonSetHasNodeSelector(kubernetesClientForSuiteRun, namespace, key, value), time.Minute*6, time.Second*5).Should(gomega.BeTrue())
					}
				}
			} else if installCase.DpaSpec.Configuration.NodeAgent != nil && *installCase.DpaSpec.Configuration.NodeAgent.Enable {
				log.Printf("Waiting for NodeAgent Pods to be running")
				gomega.Eventually(lib.AreNodeAgentPodsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
				if installCase.DpaSpec.Configuration.NodeAgent.PodConfig != nil {
					log.Printf("Waiting for NodeAgent DaemonSet to have nodeSelector")
					for key, value := range installCase.DpaSpec.Configuration.NodeAgent.PodConfig.NodeSelector {
						log.Printf("Waiting for NodeAgent DaemonSet to get node selector")
						gomega.Eventually(lib.NodeAgentDaemonSetHasNodeSelector(kubernetesClientForSuiteRun, namespace, key, value), time.Minute*6, time.Second*5).Should(gomega.BeTrue())
					}
				}
			} else {
				log.Printf("Waiting for NodeAgent DaemonSet to be deleted")
				gomega.Eventually(lib.IsNodeAgentDaemonSetDeleted(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			}

			if len(installCase.DpaSpec.BackupLocations) > 0 {
				log.Print("Checking if BSLs are available")
				gomega.Eventually(dpaCR.BSLsAreUpdated(timeAfterVeleroIsRunning), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
				gomega.Eventually(dpaCR.BSLsAreAvailable(), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
				for _, bsl := range installCase.DpaSpec.BackupLocations {
					log.Printf("Checking for BSL spec")
					gomega.Expect(dpaCR.DoesBSLSpecMatchesDpa(namespace, *bsl.Velero)).To(gomega.BeTrue())
				}
			} else {
				log.Println("Checking no BSLs are deployed")
				_, err = dpaCR.ListBSLs()
				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(err.Error()).To(gomega.Equal(fmt.Sprintf("no BSL in %s namespace", namespace)))
			}

			if len(installCase.DpaSpec.SnapshotLocations) > 0 {
				// Velero does not change status of VSL objects. Users can only confirm if VSLs are correct configured when running a native snapshot backup/restore
				for _, vsl := range installCase.DpaSpec.SnapshotLocations {
					log.Printf("Checking for VSL spec")
					gomega.Expect(dpaCR.DoesVSLSpecMatchesDpa(namespace, *vsl.Velero)).To(gomega.BeTrue())
				}
			} else {
				log.Println("Checking no VSLs are deployed")
				_, err = dpaCR.ListVSLs()
				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(err.Error()).To(gomega.Equal(fmt.Sprintf("no VSL in %s namespace", namespace)))
			}

			if len(installCase.DpaSpec.Configuration.Velero.PodConfig.Tolerations) > 0 {
				log.Printf("Checking for velero tolerances")
				gomega.Eventually(lib.VerifyVeleroTolerations(kubernetesClientForSuiteRun, namespace, installCase.DpaSpec.Configuration.Velero.PodConfig.Tolerations), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			}

			if installCase.DpaSpec.Configuration.Velero.PodConfig.ResourceAllocations.Requests != nil {
				log.Printf("Checking for velero resource allocation requests")
				gomega.Eventually(lib.VerifyVeleroResourceRequests(kubernetesClientForSuiteRun, namespace, installCase.DpaSpec.Configuration.Velero.PodConfig.ResourceAllocations.Requests), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			}

			if installCase.DpaSpec.Configuration.Velero.PodConfig.ResourceAllocations.Limits != nil {
				log.Printf("Checking for velero resource allocation limits")
				gomega.Eventually(lib.VerifyVeleroResourceLimits(kubernetesClientForSuiteRun, namespace, installCase.DpaSpec.Configuration.Velero.PodConfig.ResourceAllocations.Limits), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			}

			if len(installCase.DpaSpec.Configuration.Velero.DefaultPlugins) > 0 {
				log.Printf("Waiting for velero Deployment to have expected default plugins")
				for _, plugin := range installCase.DpaSpec.Configuration.Velero.DefaultPlugins {
					log.Printf("Checking for %s default plugin", plugin)
					gomega.Eventually(lib.DoesPluginExist(kubernetesClientForSuiteRun, namespace, plugin), time.Minute*6, time.Second*5).Should(gomega.BeTrue())
				}
			}

			if len(installCase.DpaSpec.Configuration.Velero.CustomPlugins) > 0 {
				log.Printf("Waiting for velero Deployment to have expected custom plugins")
				for _, plugin := range installCase.DpaSpec.Configuration.Velero.CustomPlugins {
					log.Printf("Checking for %s custom plugin", plugin.Name)
					gomega.Eventually(lib.DoesCustomPluginExist(kubernetesClientForSuiteRun, namespace, plugin), time.Minute*6, time.Second*5).Should(gomega.BeTrue())
				}
			}

			// wait at least 1 minute after reconciled
			gomega.Eventually(
				func() bool {
					//has it been at least 1 minute since reconciled?
					log.Printf("Waiting for 1 minute after reconciled: %v elapsed", time.Since(timeReconciled).String())
					return time.Now().After(timeReconciled.Add(time.Minute))
				},
				time.Minute*5, time.Second*5,
			).Should(gomega.BeTrue())
			adpLogsAfterOneMinute, err := lib.GetManagerPodLogs(kubernetesClientForSuiteRun, dpaCR.Namespace)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			// We expect OADP logs to be the same after 1 minute
			adpLogsDiff := cmp.Diff(adpLogsAtReconciled, adpLogsAfterOneMinute)
			// If registry deployment were deleted after CR update, we expect to see a new log entry, ignore that.
			// We also ignore case where deprecated restic entry was used
			if !strings.Contains(adpLogsDiff, "Registry Deployment deleted") && !strings.Contains(adpLogsDiff, "(Deprecation Warning) Use nodeAgent instead of restic, which is deprecated and will be removed in the future") {
				gomega.Expect(adpLogsDiff).To(gomega.Equal(""))
			}
		},
		ginkgo.Entry("Default DPA CR", InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{BSLSecretName: bslSecretName}),
		}),
		ginkgo.Entry("DPA CR with BSL secret with carriage return", InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{BSLSecretName: bslSecretNameWithCarriageReturn}),
		}),
		ginkgo.Entry("DPA CR with Velero custom plugin", InstallCase{
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
		ginkgo.Entry("DPA CR with Velero resource allocations", InstallCase{
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
		ginkgo.Entry("DPA CR with Velero toleration", InstallCase{
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
		ginkgo.Entry("DPA CR with VSL", ginkgo.Label("aws", "azure", "gcp"), InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName:     bslSecretName,
				SnapshotLocations: dpaCR.SnapshotLocations,
			}),
		}),
		ginkgo.Entry("DPA CR with restic enabled with node selector", InstallCase{
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
		ginkgo.Entry("DPA CR with kopia enabled with node selector", InstallCase{
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
		ginkgo.Entry("DPA CR with NoDefaultBackupLocation and with BackupImages false", InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName:           bslSecretName,
				NoDefaultBackupLocation: true,
				DoNotBackupImages:       true,
			}),
		}),
		ginkgo.Entry("DPA CR with legacy-aws plugin", ginkgo.Label("aws", "ibmcloud"), InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName:  bslSecretName,
				DefaultPlugins: []oadpv1alpha1.DefaultPlugin{oadpv1alpha1.DefaultPluginOpenShift, oadpv1alpha1.DefaultPluginLegacyAWS},
			}),
		}),
		ginkgo.Entry("DPA CR with S3ForcePathStyle true", ginkgo.Label("aws"), InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName:    bslSecretName,
				s3ForcePathStyle: true,
			}),
		}),
		ginkgo.Entry("DPA CR without Region, without S3ForcePathStyle and with BackupImages false", ginkgo.Label("aws"), InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName:      bslSecretName,
				NoRegion:           true,
				NoS3ForcePathStyle: true,
				DoNotBackupImages:  true,
			}),
		}),
	)

	ginkgo.DescribeTable("DPA reconciled to false",
		func(installCase InstallCase, message string) {
			lastInstallTime = time.Now()
			err := dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, installCase.DpaSpec)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			log.Printf("Test case expected to error. Waiting for the error to show in DPA Status")
			gomega.Eventually(dpaCR.IsReconciledFalse(message), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
		},
		ginkgo.Entry("DPA CR without Region and with S3ForcePathStyle true", ginkgo.Label("aws", "ibmcloud"), InstallCase{
			DpaSpec: createTestDPASpec(TestDPASpec{
				BSLSecretName:    bslSecretName,
				NoRegion:         true,
				s3ForcePathStyle: true,
			}),
		}, "region for AWS backupstoragelocation not automatically discoverable. Please set the region in the backupstoragelocation config"),
	)

	ginkgo.DescribeTable("DPA Deletion test",
		func() {
			log.Printf("Creating DPA")
			err := dpaCR.CreateOrUpdate(runTimeClientForSuiteRun, dpaCR.Build(lib.KOPIA))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			log.Printf("Waiting for velero Pod to be running")
			gomega.Eventually(lib.VeleroPodIsRunning(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
			log.Printf("Deleting DPA")
			err = dpaCR.Delete()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			log.Printf("Waiting for velero to be deleted")
			gomega.Eventually(lib.VeleroIsDeleted(kubernetesClientForSuiteRun, namespace), time.Minute*3, time.Second*5).Should(gomega.BeTrue())
		},
		ginkgo.Entry("Should succeed"),
	)
})

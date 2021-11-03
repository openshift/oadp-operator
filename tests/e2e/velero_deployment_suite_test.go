package e2e

import (
	"log"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("Configuration testing for Velero Custom Resource", func() {

	type InstallCase struct {
		Name       string
		VeleroSpec *oadpv1alpha1.VeleroSpec
		WantError  bool
	}

	DescribeTable("Updating custom resource with new configuration",
		func(installCase InstallCase, expectedErr error) {
			err := vel.CreateOrUpdate(installCase.VeleroSpec)
			Expect(err).ToNot(HaveOccurred())
			log.Printf("Waiting for velero pod to be running")
			Eventually(isVeleroPodRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			velero, err := vel.Get()
			Expect(err).NotTo(HaveOccurred())
			if len(velero.Spec.BackupStorageLocations) > 0 {
				log.Printf("Checking for bsl spec")
				for _, bsl := range velero.Spec.BackupStorageLocations {
					// Check if bsl matches the spec
					Eventually(doesBSLExist(namespace, bsl, installCase.VeleroSpec), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}
			if len(velero.Spec.VolumeSnapshotLocations) > 0 {
				log.Printf("Checking for vsl spec")
				for _, vsl := range velero.Spec.VolumeSnapshotLocations {
					Eventually(doesVSLExist(namespace, vsl, installCase.VeleroSpec), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}

			// Check for velero tolerations
			if len(velero.Spec.VeleroTolerations) > 0 {
				log.Printf("Checking for velero tolerations")
				Eventually(verifyVeleroTolerations(namespace, velero.Spec.VeleroTolerations), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}

			// TODO check for custom velero plugins

			//restic installation
			if *velero.Spec.EnableRestic {
				log.Printf("Waiting for restic pods to be running")
				Eventually(areResticPodsRunning(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			} else {
				log.Printf("Waiting for restic daemonset to be deleted")
				Eventually(isResticDaemonsetDeleted(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			// check defaultplugins
			log.Printf("Waiting for velero deployment to have expected plugins")
			if len(velero.Spec.DefaultVeleroPlugins) > 0 {
				log.Printf("Checking for default plugins")
				for _, plugin := range velero.Spec.DefaultVeleroPlugins {
					Eventually(doesPluginExist(namespace, plugin), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
				}
			}

			for key, value := range velero.Spec.ResticNodeSelector {
				log.Printf("Waiting for restic daemonset to get node selector")
				Eventually(resticDaemonSetHasNodeSelector(namespace, key, value), timeoutMultiplier*time.Minute*3, time.Second*5).Should(BeTrue())
			}
			if velero.Spec.BackupImages == nil || *installCase.VeleroSpec.BackupImages {
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
						Config: map[string]string{
							"region": region,
						},
						Default: true,
						StorageType: velero.StorageType{
							ObjectStorage: &velero.ObjectStorageLocation{
								Bucket: s3Bucket,
								Prefix: veleroPrefix,
							},
						},
					},
				},
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
					oadpv1alpha1.DefaultPluginAWS,
				},
			},
			WantError: false,
		}, nil),
		Entry("Velero CR with bsl and vsl", InstallCase{
			Name: "default-cr-bsl-vsl",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
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
								Prefix: veleroPrefix,
							},
						},
					},
				},
				VolumeSnapshotLocations: []velero.VolumeSnapshotLocationSpec{
					{
						Provider: "aws",
						Config: map[string]string{
							"Region": "us-east-1",
						},
					},
				},
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
					oadpv1alpha1.DefaultPluginAWS,
				},
			},
			WantError: false,
		}, nil),
		Entry("Velero CR with bsl and multiple vsl", InstallCase{
			Name: "default-cr-bsl-vsl",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
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
								Prefix: veleroPrefix,
							},
						},
					},
				},
				VolumeSnapshotLocations: []velero.VolumeSnapshotLocationSpec{
					{
						Provider: "aws",
						Config: map[string]string{
							"Region": "us-east-1",
						},
					},
					{
						Provider: "azure",
						Config: map[string]string{
							"Region": "us-east-1",
						},
					},
				},
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
					oadpv1alpha1.DefaultPluginAWS,
				},
			},
			WantError: false,
		}, nil),
		Entry("Velero CR with no bsl and multiple vsl", InstallCase{
			Name: "default-cr-multiple-vsl",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
				EnableRestic: pointer.Bool(true),
				VolumeSnapshotLocations: []velero.VolumeSnapshotLocationSpec{
					{
						Provider: "aws",
						Config: map[string]string{
							"Region": "us-east-1",
						},
					},
					{
						Provider: "azure",
						Config: map[string]string{
							"Region": "us-east-1",
						},
					},
				},
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
					oadpv1alpha1.DefaultPluginAWS,
				},
			},
			WantError: false,
		}, nil),
		Entry("Default velero CR with restic disabled", InstallCase{
			Name: "default-cr-no-restic",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
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
								Prefix: veleroPrefix,
							},
						},
					},
				},
				DefaultVeleroPlugins: []oadpv1alpha1.DefaultPlugin{
					oadpv1alpha1.DefaultPluginOpenShift,
					oadpv1alpha1.DefaultPluginAWS,
				},
			},
			WantError: false,
		}, nil),
		Entry("Adding CSI plugin", InstallCase{
			Name: "default-cr-csi",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
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
								Prefix: veleroPrefix,
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
			WantError: false,
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
						Config: map[string]string{
							"region": region,
						},
						Default: true,
						StorageType: velero.StorageType{
							ObjectStorage: &velero.ObjectStorageLocation{
								Bucket: s3Bucket,
								Prefix: veleroPrefix,
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
			WantError: false,
		}, nil),
		Entry("Enable tolerations", InstallCase{
			Name: "default-cr-tolerations",
			VeleroSpec: &oadpv1alpha1.VeleroSpec{
				EnableRestic: pointer.Bool(true),
				VeleroTolerations: []corev1.Toleration{
					{
						Key:               "node.kubernetes.io/unreachable",
						Operator:          "Exists",
						Effect:            "NoExecute",
						TolerationSeconds: func(i int64) *int64 { return &i }(6000),
					},
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
								Prefix: veleroPrefix,
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
			WantError: false,
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
			WantError: false,
		}, nil),
	)
})

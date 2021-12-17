package e2e

import (
	"fmt"
	"log"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("Configuration testing for DPA Custom Resource", func() {

	type InstallCase struct {
		Name         string
		BRestoreType BackupRestoreType
		DpaSpec      *oadpv1alpha1.DataProtectionApplicationSpec
	}

	DescribeTable("Updating custom resource with new configuration",
		func(installCase InstallCase, expectedErr bool) {
			//TODO: Calling vel.build() is the old pattern.
			//Change it later to make sure all the spec values are passed for every test case,
			// instead of assigning the values in advance to the DPA CR

			fmt.Println("starting vel.Build()")
			err := vel.Build(installCase.BRestoreType)
			if(expectedErr){
				Expect(err).To(BeNil()) 
			}
			// else{
			// 	Expect(err).ToNot(BeNil()) 
			// }
			fmt.Println("Done with vel.Build()")

			fmt.Println("starting vel.createorupdate")
			err = vel.CreateOrUpdate(installCase.DpaSpec)
			if(expectedErr && err != nil){
				Expect(err).To(HaveOccurred())
			}else{
				Expect(err).NotTo(HaveOccurred())
			}
			fmt.Println("Done with vel.CreateOrUpdate()")

			fmt.Println("starting vel.createorupdate nil check")
			err = vel.CreateOrUpdate(installCase.DpaSpec)
			if(expectedErr && err == nil ){
				Expect(err).To(BeNil()) 
			}
			// else{
			// 	Expect(err).ToNot(BeNil()) 
			// }
			fmt.Println("Done with vel.CreateOrUpdate() nil check")


			expectedBool := BeTrue()
			if(expectedErr){
				expectedBool= BeFalse()
			}

			log.Printf("Waiting for velero pod to be running")
			Eventually(isVeleroPodRunning(namespace, vel), timeoutMultiplier*time.Minute*3, time.Second*5).Should(expectedBool)
			dpa, err := vel.Get()
			log.Printf("Done with call for velero pod to be running")
			if(expectedErr && dpa == nil){
				Expect(err).To(HaveOccurred())
			}else{
				Expect(err).NotTo(HaveOccurred())
			}

			if(expectedErr && dpa != nil ){
				Expect(err).To(BeNil()) 
			 }
			//  else{
			// 	Expect(err).ToNot(BeNil()) 
			// }

			//if dpa is not equal to nil, then we can check the test cases
			if dpa != nil{

				//checking for backup locations
				if len(dpa.Spec.BackupLocations) > 0 {
					log.Printf("Checking for bsl spec")
					for _, bsl := range dpa.Spec.BackupLocations {
						// Check if bsl matches the spec
						Eventually(doesBSLExist(namespace, *bsl.Velero, installCase.DpaSpec), timeoutMultiplier*time.Minute*3, time.Second*5).Should(expectedBool)
					}
				}
				
				
				//checking for snapshot locations
				if len(dpa.Spec.SnapshotLocations) > 0 {
					log.Printf("Checking for vsl spec")
					for _, vsl := range dpa.Spec.SnapshotLocations {
						Eventually(doesVSLExist(namespace, *vsl.Velero, installCase.DpaSpec), timeoutMultiplier*time.Minute*3, time.Second*5).Should(expectedBool)
					}
				}


				//Check for velero tolerations
				if len(dpa.Spec.Configuration.Velero.PodConfig.Tolerations) > 0 {
					log.Printf("Checking for velero tolerations")
					Eventually(verifyVeleroTolerations(namespace, dpa.Spec.Configuration.Velero.PodConfig.Tolerations), timeoutMultiplier*time.Minute*3, time.Second*5).Should(expectedBool)
				}

				

				//check for velero resource allocations
				//resource allocation - requests
				if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests != nil {
					log.Printf("Checking for velero resource allocation requests")
					Eventually(verifyVeleroResourceRequests(namespace, dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests), timeoutMultiplier*time.Minute*3, time.Second*5).Should(expectedBool)
				}

				//resource allocation - limits
				if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits != nil {
					log.Printf("Checking for velero resource allocation limits")
					Eventually(verifyVeleroResourceLimits(namespace, dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits), timeoutMultiplier*time.Minute*3, time.Second*5).Should(expectedBool)
				}

				

				//restic installation
				if dpa.Spec.Configuration.Restic != nil && *dpa.Spec.Configuration.Restic.Enable {
					log.Printf("Waiting for restic pods to be running")
					Eventually(areResticPodsRunning(namespace,vel), timeoutMultiplier*time.Minute*3, time.Second*5).Should(expectedBool)
				} else {
					log.Printf("Waiting for restic daemonset to be deleted")
					Eventually(isResticDaemonsetDeleted(namespace), timeoutMultiplier*time.Minute*3, time.Second*5).Should(expectedBool)
				}
				// check defaultplugins
				log.Printf("Waiting for velero deployment to have expected plugins")
				if len(dpa.Spec.Configuration.Velero.DefaultPlugins) > 0 {
					log.Printf("Checking for default plugins")
					for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
						Eventually(doesPluginExist(namespace, plugin,vel), timeoutMultiplier*time.Minute*3, time.Second*5).Should(expectedBool)
					}
				}

				//check customplugins
				log.Printf("Waiting for velero deployment to have expected custom plugins")
				if len(dpa.Spec.Configuration.Velero.CustomPlugins) > 0 {
					log.Printf("Checking for custom plugins")
					for _, plugin := range dpa.Spec.Configuration.Velero.CustomPlugins {
						Eventually(doesCustomPluginExist(namespace, plugin), timeoutMultiplier*time.Minute*3, time.Second*5).Should(expectedBool)
					}
				}

				for key, value := range dpa.Spec.Configuration.Restic.PodConfig.NodeSelector {
					log.Printf("Waiting for restic daemonset to get node selector")
					Eventually(resticDaemonSetHasNodeSelector(namespace, key, value), timeoutMultiplier*time.Minute*3, time.Second*5).Should(expectedBool)
				}
				if dpa.Spec.BackupImages == nil || *installCase.DpaSpec.BackupImages {
					log.Printf("Waiting for registry pods to be running")
					Eventually(areRegistryDeploymentsAvailable(namespace,vel), timeoutMultiplier*time.Minute*3, time.Second*5).Should(expectedBool)
				}

			}else{
				if(err != nil){
					Expect(err).To(HaveOccurred())
				}else{
					Expect(err).To(BeNil()) 
				}
			}

		},
		Entry("Default velero CR", InstallCase{
			Name:         "default-cr",
			BRestoreType: restic,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginAWS,
						},
						PodConfig: &oadpv1alpha1.PodConfig{},
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
				},
			},
		}, false),
		Entry("Adding Velero custom plugin", InstallCase{
			Name:         "default-cr-velero-custom-plugin",
			BRestoreType: "restic",
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginCSI,
							oadpv1alpha1.DefaultPluginAWS,
							oadpv1alpha1.DefaultPluginOpenShift,
						},
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
				},
			},
		}, false),
		Entry("Adding Velero resource allocations", InstallCase{
			Name:         "default-cr-velero-resource-alloc",
			BRestoreType: "restic",
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
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginCSI,
							oadpv1alpha1.DefaultPluginAWS,
							oadpv1alpha1.DefaultPluginOpenShift,
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
				},
			},
		}, false),
		Entry("Adding AWS plugin", InstallCase{
			Name:         "default-cr-aws-plugin",
			BRestoreType: "restic",
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
				},
			},
		}, false),
		Entry("DPA CR with bsl and vsl", InstallCase{
			Name:         "default-cr-bsl-vsl",
			BRestoreType: "restic",
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginAWS,
						},
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(true),
					},
				},
				SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
					{
						Velero: &velero.VolumeSnapshotLocationSpec{
							Provider: "aws",
							Config: map[string]string{
								"region": "us-east-1",
							},
						},
					},
				},
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
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
				},
			},
		}, false),
		/*Entry("DPA CR with bsl and multiple vsl", InstallCase{
			Name:         "default-cr-bsl-vsl",
			BRestoreType: "restic",
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginAWS,
						},
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(true),
					},
				},
				SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
					{
						Velero: &velero.VolumeSnapshotLocationSpec{
							Provider: "aws",
							Config: map[string]string{
								"region": "us-east-1",
							},
						},
					},
					{
						Velero: &velero.VolumeSnapshotLocationSpec{
							Provider: "aws",
							Config: map[string]string{
								"region": "us-east-2",
							},
						},
					},
				},
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
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
				},
			},
			WantError: false,
		}, nil),*/
		/*Entry("DPA CR with no bsl and multiple vsl", InstallCase{
			Name:         "default-cr-multiple-vsl",
			BRestoreType: "restic",
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginAWS,
						},
						NoDefaultBackupLocation: true,
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(true),
					},
				},
				SnapshotLocations: []oadpv1alpha1.SnapshotLocation{
					{
						Velero: &velero.VolumeSnapshotLocationSpec{
							Provider: "aws",
							Config: map[string]string{
								"region": "us-east-1",
							},
						},
					},
					{
						Velero: &velero.VolumeSnapshotLocationSpec{
							Provider: "aws",
							Config: map[string]string{
								"region": "us-east-2",
							},
						},
					},
				},
			},
			WantError: false,
		}, nil),*/
		Entry("Default velero CR with restic disabled", InstallCase{
			Name:         "default-cr-no-restic",
			BRestoreType: "restic",
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginAWS,
						},
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
				},
			},
		}, false),
		Entry("Adding CSI plugin", InstallCase{
			Name:         "default-cr-csi",
			BRestoreType: "restic",
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginCSI,
							oadpv1alpha1.DefaultPluginAWS,
						},
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
				},
			},
		}, false),
		Entry("Set restic node selector", InstallCase{
			Name:         "default-cr-node-selector",
			BRestoreType: "restic",
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
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
				},
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginAWS,
						},
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
		}, false),
		Entry("Enable tolerations", InstallCase{
			Name:         "default-cr-tolerations",
			BRestoreType: "restic",
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				BackupLocations: []oadpv1alpha1.BackupLocation{
					{
						Velero: &velero.BackupStorageLocationSpec{
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
				},
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginAWS,
						},
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
		}, false),
		Entry("NoDefaultBackupLocation", InstallCase{
			Name:         "default-cr-node-selector",
			BRestoreType: "restic",
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:               &oadpv1alpha1.PodConfig{},
						NoDefaultBackupLocation: true,
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginAWS,
						},
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(true),
					},
				},
			},
		}, true),

	
		Entry("Empty BackupLocation config", InstallCase{
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						PodConfig:               &oadpv1alpha1.PodConfig{},
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginAWS,
						},
					},
					Restic: &oadpv1alpha1.ResticConfig{
						PodConfig: &oadpv1alpha1.PodConfig{},
						Enable:    pointer.Bool(true),
					},
				},
			},
		}, true),

		Entry("Empty StorageType in BSL", InstallCase{
			Name:         "default-cr",
			BRestoreType: restic,
			DpaSpec: &oadpv1alpha1.DataProtectionApplicationSpec{
				Configuration: &oadpv1alpha1.ApplicationConfig{
					Velero: &oadpv1alpha1.VeleroConfig{
						DefaultPlugins: []oadpv1alpha1.DefaultPlugin{
							oadpv1alpha1.DefaultPluginOpenShift,
							oadpv1alpha1.DefaultPluginAWS,
						},
						PodConfig: &oadpv1alpha1.PodConfig{},
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
							Config: map[string]string{
								"region": region,
							},
							Default: true,
						},
					},
				},
			},
		}, true),
	)
})

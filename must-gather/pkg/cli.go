package pkg

import (
	"fmt"
	"slices"
	"time"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	nac1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	openshiftconfigv1 "github.com/openshift/api/config/v1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	ocadminspect "github.com/openshift/oc/pkg/cli/admin/inspect"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/spf13/cobra"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/mateusoliveira43/oadp-must-gather/pkg/gather"
	"github.com/mateusoliveira43/oadp-must-gather/pkg/templates"
)

const (
	mustGatherVersion = "dev-mar-14-2025"
	// TODO remove var, not applicable anymore
	oadpOpenShiftVersion = "4.18"
	// TODO <this-image> const
)

// TODO which errors should make must-gather exit earlier?

var (
	LogsSince time.Duration
	Timeout   time.Duration
	SkipTLS   bool
	// essentialOnly bool

	CLI = &cobra.Command{
		Use: "oc adm must-gather --image=<this-image> -- /usr/bin/gather",
		Long: `OADP Must-gather

TODO`,
		Args: cobra.NoArgs,
		Example: `  # TODO
  oc adm must-gather --image=<this-image>

  # TODO
  oc adm must-gather --image=<this-image> -- /usr/bin/gather --essential-only --logs-since <time>

  # TODO
  oc adm must-gather --image=<this-image> -- /usr/bin/gather --timeout <time>

  # TODO
  oc adm must-gather --image=<this-image> -- /usr/bin/gather --skip-tls --timeout <time>

  # TODO metrics dump`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
			// TODO test flags
			// fmt.Printf("logsSince %#v\n", LogsSince)

			clusterConfig := config.GetConfigOrDie()
			// https://github.com/openshift/oc/blob/46db7c2bce5a57e3c3d9347e7e1e107e61dbd306/pkg/cli/admin/inspect/inspect.go#L142
			clusterConfig.QPS = 999999
			clusterConfig.Burst = 999999

			clusterClient, err := client.New(clusterConfig, client.Options{})
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while creating Go client: %v\n", err)
				return err
			}

			// in what versions of OCP must must-gather work? be careful about API versions update?
			err = openshiftconfigv1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while adding to scheme: %v\n", err)
				return err
			}
			err = operatorsv1alpha1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while adding to scheme: %v\n", err)
				return err
			}
			err = storagev1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while adding to scheme: %v\n", err)
				return err
			}
			err = volumesnapshotv1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while adding to scheme: %v\n", err)
				return err
			}
			err = corev1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while adding to scheme: %v\n", err)
				return err
			}
			// OADP CRDs
			err = oadpv1alpha1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while adding to scheme: %v\n", err)
				return err
			}
			err = nac1alpha1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while adding to scheme: %v\n", err)
				return err
			}
			err = velerov1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while adding to scheme: %v\n", err)
				return err
			}
			err = velerov2alpha1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while adding to scheme: %v\n", err)
				return err
			}

			clusterVersionList := &openshiftconfigv1.ClusterVersionList{}
			err = gather.AllResources(clusterClient, clusterVersionList)
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while gathering ClusterVersion: %v\n", err)
				return err
			}
			if len(clusterVersionList.Items) == 0 {
				err = fmt.Errorf("no ClusterVersion found in cluster")
				fmt.Printf("Exiting OADP must-gather, an error happened while gathering ClusterVersion: %v\n", err)
				return err
			}
			clusterVersion := &clusterVersionList.Items[0]
			clusterID := string(clusterVersion.Spec.ClusterID[:8])

			// for now, lest keep the folder structure as it is
			//     must-gather/clusters/<id>/cluster-scoped-resources/apiextensions.k8s.io/customresourcedefinitions
			//     must-gather/clusters/<id>/namespaces/<name>/velero.io/<name>
			//     must-gather/clusters/<id>/namespaces/<name>/oadp.openshift.io/<name>
			// otherwise may break `omg` usage. ref https://github.com/openshift/oadp-operator/pull/1269
			outputPath := fmt.Sprintf("must-gather/clusters/%s/", clusterID)

			var resourcesToGather []client.ObjectList
			infrastructureList := &openshiftconfigv1.InfrastructureList{}
			nodeList := &corev1.NodeList{}
			clusterServiceVersionList := &operatorsv1alpha1.ClusterServiceVersionList{}
			// TODO when Velero/OADP API updates, how to handle? use dynamic client instead?
			dataProtectionApplicationList := &oadpv1alpha1.DataProtectionApplicationList{}
			cloudStorageList := &oadpv1alpha1.CloudStorageList{}
			backupStorageLocationList := &velerov1.BackupStorageLocationList{}
			volumeSnapshotLocationList := &velerov1.VolumeSnapshotLocationList{}
			backupList := &velerov1.BackupList{}
			restoreList := &velerov1.RestoreList{}
			scheduleList := &velerov1.ScheduleList{}
			backupRepositoryList := &velerov1.BackupRepositoryList{}
			dataUploadList := &velerov2alpha1.DataUploadList{}
			dataDownloadList := &velerov2alpha1.DataDownloadList{}
			podVolumeBackupList := &velerov1.PodVolumeBackupList{}
			podVolumeRestoreList := &velerov1.PodVolumeRestoreList{}

			deleteBackupRequestList := &velerov1.DeleteBackupRequestList{}
			serverStatusRequestList := &velerov1.ServerStatusRequestList{}
			nonAdminBackupStorageLocationRequestList := &nac1alpha1.NonAdminBackupStorageLocationRequestList{}
			nonAdminBackupStorageLocationList := &nac1alpha1.NonAdminBackupStorageLocationList{}
			nonAdminBackupList := &nac1alpha1.NonAdminBackupList{}
			nonAdminRestoreList := &nac1alpha1.NonAdminRestoreList{}
			nonAdminDownloadRequestList := &nac1alpha1.NonAdminDownloadRequestList{}

			storageClassList := &storagev1.StorageClassList{}
			volumeSnapshotClassList := &volumesnapshotv1.VolumeSnapshotClassList{}
			csiDriverList := &storagev1.CSIDriverList{}
			resourcesToGather = append(resourcesToGather,
				infrastructureList,
				nodeList,
				clusterServiceVersionList,

				dataProtectionApplicationList,
				cloudStorageList,
				backupStorageLocationList,
				volumeSnapshotLocationList,
				backupList,
				restoreList,
				scheduleList,
				backupRepositoryList,
				dataUploadList,
				dataDownloadList,
				podVolumeBackupList,
				podVolumeRestoreList,

				deleteBackupRequestList,
				serverStatusRequestList,
				nonAdminBackupStorageLocationRequestList,
				nonAdminBackupStorageLocationList,
				nonAdminBackupList,
				nonAdminRestoreList,
				nonAdminDownloadRequestList,

				storageClassList,
				volumeSnapshotClassList,
				csiDriverList,
			)
			for _, resource := range resourcesToGather {
				// TODO  do this part in parallel?
				err = gather.AllResources(clusterClient, resource)
				if err != nil {
					fmt.Println(err)
				}
			}

			if len(infrastructureList.Items) == 0 {
				fmt.Println(fmt.Errorf("no Infrastructure found in cluster"))
			}
			infrastructure := &infrastructureList.Items[0]

			if len(nodeList.Items) == 0 {
				fmt.Println(fmt.Errorf("no Node found in cluster"))
			}

			// get namespaces with OADP installs

			// subscriptionList := &operatorsv1alpha1.SubscriptionList{}
			// err = clusterClient.List(context.Background(), subscriptionList)
			// if err != nil {
			// 	fmt.Println(err)
			// }
			// for _, sub := range subscriptionList.Items {
			// 	// prod? "redhat-oadp-operator"
			// 	// other packages that should be important for us?
			// 	// dev? "oadp-operator" https://github.com/openshift/oadp-operator/blob/5601dcfd0a07468f496ddb70ab570ccff1b4f0cc/bundle/metadata/annotations.yaml#L6
			// 	fmt.Printf("Found '%v' operator version '%v' installed in '%v' namespace\n", sub.Spec.Package, sub.Spec.StartingCSV, sub.Namespace)
			// }

			if len(clusterServiceVersionList.Items) == 0 {
				fmt.Println(fmt.Errorf("no ClusterServiceVersion found in cluster"))
			}
			oadpOperatorsText := ""
			foundOADP := false
			foundRelatedProducts := false
			importantCSVsByNamespace := map[string][]operatorsv1alpha1.ClusterServiceVersion{}

			// ?Managed Velero operator? only available in ROSA? https://github.com/openshift/managed-velero-operator
			//
			// ?IBM Fusion?
			//
			// ?Dell Power Protect?
			//
			// upstream velero?
			relatedProducts := []string{"OpenShift Virtualization", "Advanced Cluster Management for Kubernetes", "Submariner"}
			communityProducts := []string{"KubeVirt HyperConverged Cluster Operator"}

			for _, csv := range clusterServiceVersionList.Items {
				// OADP dev, community and prod operators have same spec.displayName
				if csv.Spec.DisplayName == "OADP Operator" {
					oadpOperatorsText += fmt.Sprintf("Found **%v** version **%v** installed in **%v** namespace\n\n", csv.Spec.DisplayName, csv.Spec.Version, csv.Namespace)
					foundOADP = true
					importantCSVsByNamespace[csv.Namespace] = append(importantCSVsByNamespace[csv.Namespace], csv)
				}
				if slices.Contains(relatedProducts, csv.Spec.DisplayName) {
					oadpOperatorsText += fmt.Sprintf("Found related product **%v** version **%v** installed in **%v** namespace\n\n", csv.Spec.DisplayName, csv.Spec.Version, csv.Namespace)
					foundRelatedProducts = true
					importantCSVsByNamespace[csv.Namespace] = append(importantCSVsByNamespace[csv.Namespace], csv)
				}
				if slices.Contains(communityProducts, csv.Spec.DisplayName) {
					oadpOperatorsText += fmt.Sprintf("⚠️ Found related product **%v (Community)** version **%v** installed in **%v** namespace\n\n", csv.Spec.DisplayName, csv.Spec.Version, csv.Namespace)
					foundRelatedProducts = true
					importantCSVsByNamespace[csv.Namespace] = append(importantCSVsByNamespace[csv.Namespace], csv)
				}
			}

			// oc adm inspect --dest-dir must-gather/clusters/${clusterID} ns/${ns}
			if len(importantCSVsByNamespace) != 0 {
				ocAdmInspect := ocadminspect.NewInspectOptions(genericiooptions.NewTestIOStreamsDiscard())
				ocAdmInspect.DestDir = outputPath
				ocAdmInspectNamespaces := []string{}
				for namespace := range importantCSVsByNamespace {
					ocAdmInspectNamespaces = append(ocAdmInspectNamespaces, "ns/"+namespace)
				}

				// https://github.com/openshift/oc/blob/ae1bd9e4a75b8ab617a569e5c8e1a0d7285a16f6/pkg/cli/admin/inspect/inspect.go#L108
				err = ocAdmInspect.Complete(ocAdmInspectNamespaces)
				if err != nil {
					fmt.Println(err)
				}
				err = ocAdmInspect.Validate()
				if err != nil {
					fmt.Println(err)
				}
				err = ocAdmInspect.Run()
				if err != nil {
					fmt.Println(err)
				}
			}

			// gather_logs

			// gather_metrics
			// Find problem with velero metrics (port?) and kill html, add to summary.md file

			// gather_versions https://github.com/openshift/oadp-operator/pull/994
			if len(storageClassList.Items) == 0 {
				fmt.Println(fmt.Errorf("no StorageClass found in cluster"))
			}

			if len(volumeSnapshotClassList.Items) == 0 {
				fmt.Println(fmt.Errorf("no VolumeSnapshotClass found in cluster"))
			}

			if len(csiDriverList.Items) == 0 {
				fmt.Println(fmt.Errorf("no CSIDriver found in cluster"))
			}

			// TODO do processes in parallel!?
			// https://gobyexample.com/waitgroups
			// https://github.com/konveyor/analyzer-lsp/blob/main/engine/engine.go
			templates.ReplaceMustGatherVersion(mustGatherVersion)
			templates.ReplaceClusterInformationSection(outputPath, clusterID, clusterVersion, infrastructure, nodeList)
			templates.ReplaceOADPOperatorInstallationSection(outputPath, importantCSVsByNamespace, foundOADP, foundRelatedProducts, oadpOperatorsText)
			templates.ReplaceDataProtectionApplicationsSection(outputPath, dataProtectionApplicationList)
			templates.ReplaceCloudStoragesSection(outputPath, cloudStorageList)
			templates.ReplaceBackupStorageLocationsSection(outputPath, backupStorageLocationList)
			templates.ReplaceVolumeSnapshotLocationsSection(outputPath, volumeSnapshotLocationList)
			// this creates DownloadRequests CRs
			templates.ReplaceBackupsSection(outputPath, backupList, clusterClient, deleteBackupRequestList, podVolumeBackupList)
			templates.ReplaceRestoresSection(outputPath, restoreList, clusterClient, podVolumeRestoreList)

			downloadRequestList := &velerov1.DownloadRequestList{}
			err = gather.AllResources(clusterClient, downloadRequestList)
			if err != nil {
				fmt.Println(err)
			}

			templates.ReplaceSchedulesSection(outputPath, scheduleList)
			templates.ReplaceBackupRepositoriesSection(outputPath, backupRepositoryList)
			templates.ReplaceDataUploadsSection(outputPath, dataUploadList)
			templates.ReplaceDataDownloadsSection(outputPath, dataDownloadList)
			templates.ReplacePodVolumeBackupsSection(outputPath, podVolumeBackupList)
			templates.ReplacePodVolumeRestoresSection(outputPath, podVolumeRestoreList)
			templates.ReplaceDownloadRequestsSection(outputPath, downloadRequestList)
			templates.ReplaceDeleteBackupRequestsSection(outputPath, deleteBackupRequestList)
			templates.ReplaceServerStatusRequestsSection(outputPath, serverStatusRequestList)
			templates.ReplaceNonAdminBackupStorageLocationRequestsSection(outputPath, nonAdminBackupStorageLocationRequestList)
			templates.ReplaceNonAdminBackupStorageLocationsSection(outputPath, nonAdminBackupStorageLocationList)
			templates.ReplaceNonAdminBackupsSection(outputPath, nonAdminBackupList)
			templates.ReplaceNonAdminRestoresSection(outputPath, nonAdminRestoreList)
			templates.ReplaceNonAdminDownloadRequestsSection(outputPath, nonAdminDownloadRequestList)
			templates.ReplaceAvailableStorageClassesSection(outputPath, storageClassList)
			templates.ReplaceAvailableVolumeSnapshotClassesSection(outputPath, volumeSnapshotClassList)
			templates.ReplaceAvailableCSIDriversSection(outputPath, csiDriverList, oadpOpenShiftVersion)
			templates.ReplaceCustomResourceDefinitionsSection(outputPath, clusterConfig)
			// do not tar!
			err = templates.Write(outputPath)
			if err != nil {
				fmt.Printf("Error occurred: %v\n", err)
				return err
			}
			return nil
			// TODO Should / Can must-gather collect node and node-agent /dev/ and /host_pods files info.
		},
	}
)

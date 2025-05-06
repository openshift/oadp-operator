package pkg

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
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

	"github.com/openshift/oadp-operator/must-gather/pkg/gather"
	"github.com/openshift/oadp-operator/must-gather/pkg/templates"
)

const (
	mustGatherVersion = "1.5.0"
	mustGatherImage   = "registry.redhat.io/oadp/oadp-mustgather-rhel9:v1.5"

	addToSchemeError = "Exiting OADP must-gather, an error happened while adding %s to scheme: %v\n"

	DefaultRequestTimeout = 5 * time.Second
)

var (
	Timeout        time.Duration
	RequestTimeout time.Duration
	SkipTLS        bool

	CLI = &cobra.Command{
		Use: fmt.Sprintf("oc adm must-gather --image=%[1]s -- /usr/bin/gather", mustGatherImage),
		Long: `OADP Must-gather: a tool to collect information about OADP installation in a cluster, along with information about its custom resources and cluster storage.

For more information, check OADP must-gather documentation: https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/backup_and_restore/oadp-application-backup-and-restore#using-the-must-gather-tool`,
		Args: cobra.NoArgs,
		Example: fmt.Sprintf(`  # running OADP Must-gather with default configuration
  oc adm must-gather --image=%[1]s

  # running OADP Must-gather with timeout of 1 minute per OADP server request
  oc adm must-gather --image=%[1]s -- /usr/bin/gather --request-timeout 1m

  # running OADP Must-gather with timeout of 15 seconds per OADP server request and with insecure TLS connections
  oc adm must-gather --image=%[1]s -- /usr/bin/gather --request-timeout 15s --skip-tls`, mustGatherImage),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if RequestTimeout <= 0 {
				err := fmt.Errorf("--request-timeout value must be greater than zero")
				fmt.Printf("Exiting OADP must-gather: %v\n", err)
				return err
			}

			clusterConfig := config.GetConfigOrDie()
			// https://github.com/openshift/oc/blob/46db7c2bce5a57e3c3d9347e7e1e107e61dbd306/pkg/cli/admin/inspect/inspect.go#L142
			clusterConfig.QPS = 999999
			clusterConfig.Burst = 999999

			clusterClient, err := client.New(clusterConfig, client.Options{})
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while creating Go client: %v\n", err)
				return err
			}

			err = openshiftconfigv1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf(addToSchemeError, "github.com/openshift/api/config/v1", err)
				return err
			}
			err = operatorsv1alpha1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf(addToSchemeError, "github.com/operator-framework/api/pkg/operators/v1alpha1", err)
				return err
			}
			err = storagev1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf(addToSchemeError, "k8s.io/api/storage/v1", err)
				return err
			}
			err = volumesnapshotv1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf(addToSchemeError, "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1", err)
				return err
			}
			err = corev1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf(addToSchemeError, "k8s.io/api/core/v1", err)
				return err
			}
			// OADP CRDs
			err = oadpv1alpha1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf(addToSchemeError, "github.com/openshift/oadp-operator/api/v1alpha1", err)
				return err
			}
			err = nac1alpha1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf(addToSchemeError, "github.com/migtools/oadp-non-admin/api/v1alpha1", err)
				return err
			}
			err = velerov1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf(addToSchemeError, "github.com/vmware-tanzu/velero/pkg/apis/velero/v1", err)
				return err
			}
			err = velerov2alpha1.AddToScheme(clusterClient.Scheme())
			if err != nil {
				fmt.Printf(addToSchemeError, "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1", err)
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
			versionParts := strings.Split(clusterVersion.Status.Desired.Version, ".")
			major, err := strconv.Atoi(versionParts[0])
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while parsing OpenShift major version: %v\n", err)
				return err
			}
			minor, err := strconv.Atoi(versionParts[1])
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while parsing OpenShift minor version: %v\n", err)
				return err
			}

			// be careful about folder structure, otherwise may break `omg` usage
			outputPath := fmt.Sprintf("must-gather/clusters/%s/", clusterID)

			var resourcesToGather []client.ObjectList
			infrastructureList := &openshiftconfigv1.InfrastructureList{}
			nodeList := &corev1.NodeList{}
			clusterServiceVersionList := &operatorsv1alpha1.ClusterServiceVersionList{}
			subscriptionList := &operatorsv1alpha1.SubscriptionList{}

			dataProtectionApplicationList := &oadpv1alpha1.DataProtectionApplicationList{}
			dataProtectionTestList := &oadpv1alpha1.DataProtectionTestList{}
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
				subscriptionList,

				dataProtectionApplicationList,
				dataProtectionTestList,
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

			// get namespaces with OADP installs
			if len(clusterServiceVersionList.Items) == 0 {
				fmt.Println(fmt.Errorf("no ClusterServiceVersion found in cluster"))
			}
			oadpOperatorsText := ""
			foundOADP := false
			foundRelatedProducts := false
			importantCSVsByNamespace := map[string][]operatorsv1alpha1.ClusterServiceVersion{}
			importantSubscriptionsByNamespace := map[string][]operatorsv1alpha1.Subscription{}
			oldOADPError := ""

			// ?Managed Velero operator? only available in ROSA? https://github.com/openshift/managed-velero-operator
			//
			// ?Dell Power Protect?
			// labels:
			//       app: ppdm-controller
			//       app.kubernetes.io/name: powerprotect
			// name: powerprotect-controller-c8dcf8648-nlg85
			//
			// upstream velero?
			relatedProducts := []string{
				"OpenShift Virtualization",
				"Advanced Cluster Management for Kubernetes",
				"Submariner",
				"IBM Storage Fusion",
			}
			communityProducts := []string{"KubeVirt HyperConverged Cluster Operator"}

			for _, csv := range clusterServiceVersionList.Items {
				// OADP dev, community and prod operators have same spec.displayName
				if csv.Spec.DisplayName == "OADP Operator" {
					oadpOperatorsText += fmt.Sprintf("Found **%v** version **%v** installed in **%v** namespace\n\n", csv.Spec.DisplayName, csv.Spec.Version, csv.Namespace)
					if (csv.Spec.Version.Major < 1 || (csv.Spec.Version.Major == 1 && csv.Spec.Version.Minor < 5)) && major >= 4 && minor >= 19 {
						oldOADPError += "❌ OADP 1.4 and lower is not supported in OpenShift 4.19 and higher\n\n"
					}
					foundOADP = true
					importantCSVsByNamespace[csv.Namespace] = append(importantCSVsByNamespace[csv.Namespace], csv)
					for _, subscription := range subscriptionList.Items {
						if subscription.Status.InstalledCSV == csv.Name {
							importantSubscriptionsByNamespace[subscription.Namespace] = append(importantSubscriptionsByNamespace[subscription.Namespace], subscription)
						}
					}
				}
				if slices.Contains(relatedProducts, csv.Spec.DisplayName) {
					oadpOperatorsText += fmt.Sprintf("Found related product **%v** version **%v** installed in **%v** namespace\n\n", csv.Spec.DisplayName, csv.Spec.Version, csv.Namespace)
					foundRelatedProducts = true
					importantCSVsByNamespace[csv.Namespace] = append(importantCSVsByNamespace[csv.Namespace], csv)
					for _, subscription := range subscriptionList.Items {
						if subscription.Status.InstalledCSV == csv.Name {
							importantSubscriptionsByNamespace[subscription.Namespace] = append(importantSubscriptionsByNamespace[subscription.Namespace], subscription)
						}
					}
				}
				if slices.Contains(communityProducts, csv.Spec.DisplayName) {
					oadpOperatorsText += fmt.Sprintf("⚠️ Found related product **%v (Community)** version **%v** installed in **%v** namespace\n\n", csv.Spec.DisplayName, csv.Spec.Version, csv.Namespace)
					foundRelatedProducts = true
					importantCSVsByNamespace[csv.Namespace] = append(importantCSVsByNamespace[csv.Namespace], csv)
					for _, subscription := range subscriptionList.Items {
						if subscription.Status.InstalledCSV == csv.Name {
							importantSubscriptionsByNamespace[subscription.Namespace] = append(importantSubscriptionsByNamespace[subscription.Namespace], subscription)
						}
					}
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

			// TODO do processes in parallel!?
			// https://gobyexample.com/waitgroups
			// https://github.com/konveyor/analyzer-lsp/blob/main/engine/engine.go
			templates.ReplaceMustGatherVersion(mustGatherVersion)
			templates.ReplaceClusterInformationSection(outputPath, clusterID, clusterVersion, infrastructureList, nodeList)
			templates.ReplaceOADPOperatorInstallationSection(outputPath, importantCSVsByNamespace, importantSubscriptionsByNamespace, foundOADP, foundRelatedProducts, oldOADPError, oadpOperatorsText)
			templates.ReplaceDataProtectionApplicationsSection(outputPath, dataProtectionApplicationList)
			templates.ReplaceDataProtectionTestsSection(outputPath, dataProtectionTestList)
			templates.ReplaceCloudStoragesSection(outputPath, cloudStorageList)
			templates.ReplaceBackupStorageLocationsSection(outputPath, backupStorageLocationList)
			templates.ReplaceVolumeSnapshotLocationsSection(outputPath, volumeSnapshotLocationList)
			// this creates DownloadRequests CRs
			templates.ReplaceBackupsSection(outputPath, backupList, clusterClient, deleteBackupRequestList, podVolumeBackupList, RequestTimeout, SkipTLS)
			templates.ReplaceRestoresSection(outputPath, restoreList, clusterClient, podVolumeRestoreList, RequestTimeout, SkipTLS)

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
			templates.ReplaceAvailableCSIDriversSection(outputPath, csiDriverList)
			templates.ReplaceCustomResourceDefinitionsSection(outputPath, clusterConfig)
			err = templates.WriteVersion(mustGatherVersion)
			if err != nil {
				fmt.Printf("Error occurred: %v\n", err)
				return err
			}
			// do not tar!
			err = templates.Write(outputPath)
			if err != nil {
				fmt.Printf("Error occurred: %v\n", err)
				return err
			}
			return nil
		},
	}
)

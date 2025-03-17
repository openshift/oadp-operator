package templates

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	nac1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	openshiftconfigv1 "github.com/openshift/api/config/v1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/downloadrequest"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/output"
	"github.com/vmware-tanzu/velero/pkg/label"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/mateusoliveira43/oadp-must-gather/pkg/gvk"
)

var (
	summaryTemplateReplacesKeys = []string{
		"MUST_GATHER_VERSION",
		"ERRORS",
		"CLUSTER_ID", "OCP_VERSION", "CLOUD", "ARCH", "CLUSTER_VERSION",
		"OADP_VERSIONS",
		"DATA_PROTECTION_APPLICATIONS",
		"CLOUD_STORAGES",
		"BACKUP_STORAGE_LOCATIONS",
		"VOLUME_SNAPSHOT_LOCATIONS",
		"BACKUPS",
		"RESTORES",
		"SCHEDULES",
		"BACKUPS_REPOSITORIES",
		"DATA_UPLOADS",
		"DATA_DOWNLOADS",
		"POD_VOLUME_BACKUPS",
		"POD_VOLUME_RESTORES",
		"DOWNLOAD_REQUESTS",
		"DELETE_BACKUP_REQUESTS",
		"SERVER_STATUS_REQUESTS",
		"NON_ADMIN_BACKUP_STORAGE_LOCATION_REQUESTS",
		"NON_ADMIN_BACKUP_STORAGE_LOCATIONS",
		"NON_ADMIN_BACKUPS",
		"NON_ADMIN_RESTORES",
		"NON_ADMIN_DOWNLOAD_REQUESTS",
		"STORAGE_CLASSES",
		"VOLUME_SNAPSHOT_CLASSES",
		"CSI_DRIVERS", "OADP_OCP_VERSION",
		"CUSTOM_RESOURCE_DEFINITION",
	}
	summaryTemplateReplaces = map[string]string{}
)

// TODO https://stackoverflow.com/a/31742265
// TODO https://github.com/kubernetes-sigs/kubebuilder/blob/master/pkg/plugins/golang/v4/scaffolds/internal/templates/readme.go
// https://deploy-preview-4185--kubebuilder.netlify.app/plugins/extending/extending_cli_features_and_plugins#example-bollerplate
// https://github.com/kubernetes-sigs/kubebuilder/tree/master/pkg/machinery
const summaryTemplate = `# OADP must-gather summary version <<MUST_GATHER_VERSION>>

# Table of Contents

- [Errors](#errors)
- [Cluster information](#cluster-information)
- [OADP operator installation information](#oadp-operator-installation-information)
    - [DataProtectionApplications (DPAs)](#dataprotectionapplications-dpas)
    - [CloudStorages](#cloudstorages)
    - [BackupStorageLocations (BSLs)](#backupstoragelocations-bsls)
    - [VolumeSnapshotLocations (VSLs)](#volumesnapshotlocations-vsls)
    - [Backups](#backups)
    - [Restores](#restores)
    - [Schedules](#schedules)
    - [BackupRepositories](#backuprepositories)
    - [DataUploads](#datauploads)
    - [DataDownloads](#datadownloads)
    - [PodVolumeBackups](#podvolumebackups)
    - [PodVolumeRestores](#podvolumerestores)
    - [DownloadRequests](#downloadrequests)
    - [DeleteBackupRequests](#deletebackuprequests)
    - [ServerStatusRequests](#serverstatusrequests)
    - [NonAdminBackupStorageLocationRequests](#nonadminbackupstoragelocationrequests)
    - [NonAdminBackupStorageLocations](#nonadminbackupstoragelocations)
    - [NonAdminBackups](#nonadminbackups)
    - [NonAdminRestores](#nonadminrestores)
    - [NonAdminDownloadRequests](#nonadmindownloadrequests)
- Storage
    - [Available StorageClasses in cluster](#available-storageclasses-in-cluster)
    - [Available VolumeSnapshotClasses in cluster](#available-volumesnapshotclasses-in-cluster)
    - [Available CSIDrivers in cluster](#available-csidrivers-in-cluster)
- [CustomResourceDefinitions](#customresourcedefinitions)

## Errors

<<ERRORS>>

## Cluster information

| Cluster ID | OpenShift version | Cloud provider | Architecture |
| ---------- | ----------------- | -------------- | ------------ |
| <<CLUSTER_ID>> | <<OCP_VERSION>> | <<CLOUD>> | <<ARCH>> |

<<CLUSTER_VERSION>>

## OADP operator installation information

<<OADP_VERSIONS>>

### DataProtectionApplications (DPAs)

<<DATA_PROTECTION_APPLICATIONS>>

### CloudStorages

<<CLOUD_STORAGES>>

### BackupStorageLocations (BSLs)

<<BACKUP_STORAGE_LOCATIONS>>

### VolumeSnapshotLocations (VSLs)

<<VOLUME_SNAPSHOT_LOCATIONS>>

### Backups

<<BACKUPS>>

### Restores

<<RESTORES>>

### Schedules

<<SCHEDULES>>

### BackupRepositories

<<BACKUPS_REPOSITORIES>>

### DataUploads

<<DATA_UPLOADS>>

### DataDownloads

<<DATA_DOWNLOADS>>

### PodVolumeBackups

<<POD_VOLUME_BACKUPS>>

### PodVolumeRestores

<<POD_VOLUME_RESTORES>>

### DownloadRequests

<<DOWNLOAD_REQUESTS>>

### DeleteBackupRequests

<<DELETE_BACKUP_REQUESTS>>

### ServerStatusRequests

<<SERVER_STATUS_REQUESTS>>

### NonAdminBackupStorageLocationRequests

<<NON_ADMIN_BACKUP_STORAGE_LOCATION_REQUESTS>>

### NonAdminBackupStorageLocations

<<NON_ADMIN_BACKUP_STORAGE_LOCATIONS>>

### NonAdminBackups

<<NON_ADMIN_BACKUPS>>

### NonAdminRestores

<<NON_ADMIN_RESTORES>>

### NonAdminDownloadRequests

<<NON_ADMIN_DOWNLOAD_REQUESTS>>

## Available StorageClasses in cluster

<<STORAGE_CLASSES>>

## Available VolumeSnapshotClasses in cluster

<<VOLUME_SNAPSHOT_CLASSES>>

## Available CSIDrivers in cluster

<<CSI_DRIVERS>>

> **Note:** check [supported Container Storage Interface drivers for OpenShift <<OADP_OCP_VERSION>>](https://docs.openshift.com/container-platform/<<OADP_OCP_VERSION>>/storage/container_storage_interface/persistent-storage-csi.html#csi-drivers-supported_persistent-storage-csi)

## CustomResourceDefinitions

<<CUSTOM_RESOURCE_DEFINITION>>
`

func init() {
	for _, key := range summaryTemplateReplacesKeys {
		summaryTemplateReplaces[key] = ""
	}
}

func ReplaceMustGatherVersion(version string) {
	summaryTemplateReplaces["MUST_GATHER_VERSION"] = "`" + version + "`"
}

func ReplaceClusterInformationSection(outputPath string, clusterID string, clusterVersion *openshiftconfigv1.ClusterVersion, infrastructure *openshiftconfigv1.Infrastructure, nodeList *corev1.NodeList) {
	summaryTemplateReplaces["CLUSTER_ID"] = clusterID

	if clusterVersion != nil {
		// nil check
		summaryTemplateReplaces["OCP_VERSION"] = clusterVersion.Status.Desired.Version
		summaryTemplateReplaces["CLUSTER_VERSION"] = createYAML(outputPath, "cluster-scoped-resources/config.openshift.io/clusterversions.yaml", clusterVersion)
	} else {
		// this is code is unreachable?
		summaryTemplateReplaces["OCP_VERSION"] = "❌ error"
		summaryTemplateReplaces["OCP_CAPABILITIES"] = "❌ error"
		summaryTemplateReplaces["ERRORS"] += "⚠️ No ClusterVersion found in cluster\n\n"
	}

	if infrastructure != nil {
		cloudProvider := string(infrastructure.Spec.PlatformSpec.Type)
		summaryTemplateReplaces["CLOUD"] = cloudProvider
	} else {
		summaryTemplateReplaces["CLOUD"] = "❌ error"
		summaryTemplateReplaces["ERRORS"] += "⚠️ No Infrastructure found in cluster\n\n"
	}

	if nodeList != nil && len(nodeList.Items) != 0 {
		architectureText := ""
		for _, node := range nodeList.Items {
			arch := node.Status.NodeInfo.OperatingSystem + "/" + node.Status.NodeInfo.Architecture
			if len(architectureText) == 0 {
				architectureText += arch
			} else {
				if !strings.Contains(architectureText, arch) {
					architectureText += " | " + arch
				}
			}
		}
		summaryTemplateReplaces["ARCH"] = architectureText
	} else {
		summaryTemplateReplaces["ARCH"] = "❌ error"
		summaryTemplateReplaces["ERRORS"] += "⚠️ No Node found in cluster\n\n"
	}
	// TODO maybe nil case can be simplified by initializing everything with an error state/message
}

func ReplaceOADPOperatorInstallationSection(
	outputPath string,
	importantCSVsByNamespace map[string][]operatorsv1alpha1.ClusterServiceVersion,
	foundOADP bool,
	foundRelatedProducts bool,
	oadpOperatorsText string,
) {
	if len(importantCSVsByNamespace) == 0 {
		summaryTemplateReplaces["OADP_VERSIONS"] = "❌ No OADP Operator was found installed in the cluster\n\nNo related product was found installed in the cluster"
		summaryTemplateReplaces["ERRORS"] += "🚫 No OADP Operator was found installed in the cluster\n\n"
	} else {
		for namespace, csvs := range importantCSVsByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)
			for _, csv := range csvs {
				csv.GetObjectKind().SetGroupVersionKind(gvk.ClusterServiceVersionGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &csv})
			}
			folder := fmt.Sprintf("namespaces/%s/operators.coreos.com/clusterserviceversions", namespace)
			oadpOperatorsText += createYAML(outputPath, folder+"/clusterserviceversions.yaml", list)
		}
		if !foundOADP {
			summaryTemplateReplaces["OADP_VERSIONS"] += "❌ No OADP Operator was found installed in the cluster\n\n"
			summaryTemplateReplaces["ERRORS"] += "🚫 No OADP Operator was found installed in the cluster\n\n"
		}
		summaryTemplateReplaces["OADP_VERSIONS"] += oadpOperatorsText
		if !foundRelatedProducts {
			summaryTemplateReplaces["OADP_VERSIONS"] += "No related product was found installed in the cluster\n\n"
		}
		summaryTemplateReplaces["OADP_VERSIONS"] += fmt.Sprintf("For information about all objects collected in each namespace, check [`%[1]s/namespaces`](%[1]s/namespaces) folder", outputPath)
	}
}

func ReplaceDataProtectionApplicationsSection(outputPath string, dataProtectionApplicationList *oadpv1alpha1.DataProtectionApplicationList) {
	if dataProtectionApplicationList != nil && len(dataProtectionApplicationList.Items) != 0 {
		dataProtectionApplicationsByNamespace := map[string][]oadpv1alpha1.DataProtectionApplication{}

		for _, dataProtectionApplication := range dataProtectionApplicationList.Items {
			dataProtectionApplicationsByNamespace[dataProtectionApplication.Namespace] = append(dataProtectionApplicationsByNamespace[dataProtectionApplication.Namespace], dataProtectionApplication)
		}

		summaryTemplateReplaces["DATA_PROTECTION_APPLICATIONS"] += "| Namespace | Name | spec.unsupportedOverrides | status.conditions[0] | yaml |\n| --- | --- | --- | --- | --- |\n"
		for namespace, dataProtectionApplications := range dataProtectionApplicationsByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/oadp.openshift.io/dataprotectionapplications", namespace)
			file := folder + "/dataprotectionapplications.yaml"
			for _, dataProtectionApplication := range dataProtectionApplications {
				dataProtectionApplication.GetObjectKind().SetGroupVersionKind(gvk.DataProtectionApplicationGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &dataProtectionApplication})

				unsupportedOverridesText := "false"
				if dataProtectionApplication.Spec.UnsupportedOverrides != nil {
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ DataProtectionApplication **%v** in **%v** namespace is using **unsupportedOverrides**\n\n",
						dataProtectionApplication.Name, namespace,
					)
					unsupportedOverridesText = "⚠️ true"
				}

				dpaStatus := ""
				if len(dataProtectionApplication.Status.Conditions) == 0 {
					dpaStatus = "⚠️ no status"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ DataProtectionApplication **%v** with **no status** in **%v** namespace\n\n",
						dataProtectionApplication.Name, namespace,
					)
				} else {
					condition := dataProtectionApplication.Status.Conditions[0]
					if condition.Status == v1.ConditionTrue {
						dpaStatus = fmt.Sprintf("✅ status %s: %s", condition.Type, condition.Status)
					} else {
						dpaStatus = fmt.Sprintf("❌ status %s: %s", condition.Type, condition.Status)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ DataProtectionApplication **%v** with **status %s: %s** in **%v** namespace\n\n",
							dataProtectionApplication.Name, condition.Type, condition.Status, namespace,
						)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["DATA_PROTECTION_APPLICATIONS"] += fmt.Sprintf(
					"| %v | %v | %v | %v | %s |\n",
					namespace, dataProtectionApplication.Name, unsupportedOverridesText, dpaStatus, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["DATA_PROTECTION_APPLICATIONS"] = "❌ No DataProtectionApplication was found in the cluster"
		summaryTemplateReplaces["ERRORS"] += "⚠️ No DataProtectionApplication was found in the cluster\n\n"
	}
}

func ReplaceCloudStoragesSection(outputPath string, cloudStorageList *oadpv1alpha1.CloudStorageList) {
	if cloudStorageList != nil && len(cloudStorageList.Items) != 0 {
		cloudStorageByNamespace := map[string][]oadpv1alpha1.CloudStorage{}

		for _, cloudStorage := range cloudStorageList.Items {
			cloudStorageByNamespace[cloudStorage.Namespace] = append(cloudStorageByNamespace[cloudStorage.Namespace], cloudStorage)
		}

		summaryTemplateReplaces["CLOUD_STORAGES"] += "| Namespace | Name | yaml |\n| --- | --- | --- |\n"
		for namespace, cloudStorages := range cloudStorageByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/oadp.openshift.io/cloudstorages", namespace)
			file := folder + "/cloudstorages.yaml"
			for _, cloudStorage := range cloudStorages {
				cloudStorage.GetObjectKind().SetGroupVersionKind(gvk.CloudStorageGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &cloudStorage})

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["BACKUPS"] += fmt.Sprintf(
					"| %v | %v | %s |\n",
					namespace, cloudStorage.Name, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["CLOUD_STORAGES"] = "❌ No CloudStorage was found in the cluster"
	}
}

func ReplaceBackupStorageLocationsSection(outputPath string, backupStorageLocationList *velerov1.BackupStorageLocationList) {
	if backupStorageLocationList != nil && len(backupStorageLocationList.Items) != 0 {
		backupStorageLocationsByNamespace := map[string][]velerov1.BackupStorageLocation{}

		for _, backupStorageLocation := range backupStorageLocationList.Items {
			backupStorageLocationsByNamespace[backupStorageLocation.Namespace] = append(backupStorageLocationsByNamespace[backupStorageLocation.Namespace], backupStorageLocation)
		}

		summaryTemplateReplaces["BACKUP_STORAGE_LOCATIONS"] += "| Namespace | Name | spec.default | status.phase | yaml |\n| --- | --- | --- | --- | --- |\n"
		for namespace, backupStorageLocations := range backupStorageLocationsByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/velero.io/backupstoragelocations", namespace)
			file := folder + "/backupstoragelocations.yaml"
			for _, backupStorageLocation := range backupStorageLocations {
				backupStorageLocation.GetObjectKind().SetGroupVersionKind(gvk.BackupStorageLocationGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &backupStorageLocation})

				bslStatus := ""
				bslStatusPhase := backupStorageLocation.Status.Phase
				if len(bslStatusPhase) == 0 {
					bslStatus = "⚠️ no status phase"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ BackupStorageLocation **%v** with **no status phase** in **%v** namespace\n\n",
						backupStorageLocation.Name, namespace,
					)
				} else {
					if bslStatusPhase == velerov1.BackupStorageLocationPhaseAvailable {
						bslStatus = fmt.Sprintf("✅ status phase %s", bslStatusPhase)
					} else {
						bslStatus = fmt.Sprintf("❌ status phase %s", bslStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ BackupStorageLocation **%v** with **status phase %s** in **%v** namespace\n\n",
							backupStorageLocation.Name, bslStatusPhase, namespace,
						)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["BACKUP_STORAGE_LOCATIONS"] += fmt.Sprintf(
					"| %v | %v | %t | %v | %s |\n",
					namespace, backupStorageLocation.Name, backupStorageLocation.Spec.Default, bslStatus, link,
				)
				// velero get backup-locations
				// NAME              PROVIDER   BUCKET/PREFIX           PHASE         LAST VALIDATED                  ACCESS MODE   DEFAULT
				// velero-sample-1   aws        my-bucket-name/velero   Unavailable   2024-10-21 17:27:45 +0000 UTC   ReadWrite     true

				// oc get bsl -n openshift-adp
				// NAME              PHASE         LAST VALIDATED   AGE    DEFAULT
				// velero-sample-1   Unavailable   22s              112s   true
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["BACKUP_STORAGE_LOCATIONS"] = "❌ No BackupStorageLocation was found in the cluster"
		summaryTemplateReplaces["ERRORS"] += "⚠️ No BackupStorageLocation was found in the cluster\n\n"
	}
}

func ReplaceVolumeSnapshotLocationsSection(outputPath string, volumeSnapshotLocationList *velerov1.VolumeSnapshotLocationList) {
	if volumeSnapshotLocationList != nil && len(volumeSnapshotLocationList.Items) != 0 {
		volumeSnapshotLocationsByNamespace := map[string][]velerov1.VolumeSnapshotLocation{}

		for _, volumeSnapshotLocation := range volumeSnapshotLocationList.Items {
			volumeSnapshotLocationsByNamespace[volumeSnapshotLocation.Namespace] = append(volumeSnapshotLocationsByNamespace[volumeSnapshotLocation.Namespace], volumeSnapshotLocation)
		}

		summaryTemplateReplaces["VOLUME_SNAPSHOT_LOCATIONS"] += "| Namespace | Name | yaml |\n| --- | --- | --- |\n"
		for namespace, volumeSnapshotLocations := range volumeSnapshotLocationsByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/velero.io/volumesnapshotlocations", namespace)
			file := folder + "/volumesnapshotlocations.yaml"
			for _, volumeSnapshotLocation := range volumeSnapshotLocations {
				volumeSnapshotLocation.GetObjectKind().SetGroupVersionKind(gvk.VolumeSnapshotLocationGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &volumeSnapshotLocation})

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["VOLUME_SNAPSHOT_LOCATIONS"] += fmt.Sprintf(
					"| %v | %v | %s |\n",
					namespace, volumeSnapshotLocation.Name, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["VOLUME_SNAPSHOT_LOCATIONS"] = "❌ No VolumeSnapshotLocation was found in the cluster"
	}
}

func ReplaceBackupsSection(outputPath string, backupList *velerov1.BackupList, clusterClient client.Client, deleteBackupRequestList *velerov1.DeleteBackupRequestList, podVolumeBackupList *velerov1.PodVolumeBackupList) {
	if backupList != nil && len(backupList.Items) != 0 {
		backupsByNamespace := map[string][]velerov1.Backup{}

		for _, backup := range backupList.Items {
			backupsByNamespace[backup.Namespace] = append(backupsByNamespace[backup.Namespace], backup)
		}

		summaryTemplateReplaces["BACKUPS"] += "| Namespace | Name | status.phase | describe | logs | yaml |\n| --- | --- | --- | --- | --- | ---|\n"
		for namespace, backups := range backupsByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/velero.io/backups", namespace)
			file := folder + "/backups.yaml"
			for _, backup := range backups {
				backup.GetObjectKind().SetGroupVersionKind(gvk.BackupGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &backup})

				backupStatus := ""
				backupStatusPhase := backup.Status.Phase
				if len(backupStatusPhase) == 0 {
					backupStatus = "⚠️ no status phase"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ Backup **%v** with **no status phase** in **%v** namespace\n\n",
						backup.Name, namespace,
					)
				} else {
					failedStates := []velerov1.BackupPhase{
						velerov1.BackupPhaseFailed,
						velerov1.BackupPhasePartiallyFailed,
						velerov1.BackupPhaseFinalizingPartiallyFailed,
						velerov1.BackupPhaseWaitingForPluginOperationsPartiallyFailed,
						velerov1.BackupPhaseFailedValidation,
					}
					if backupStatusPhase == velerov1.BackupPhaseCompleted {
						backupStatus = fmt.Sprintf("✅ status phase %s", backupStatusPhase)
					} else if slices.Contains(failedStates, backupStatusPhase) {
						backupStatus = fmt.Sprintf("❌ status phase %s", backupStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ Backup **%v** with **status phase %s** in **%v** namespace\n\n",
							backup.Name, backupStatusPhase, namespace,
						)
					} else {
						backupStatus = fmt.Sprintf("⚠️ status phase %s", backupStatusPhase)
					}
				}

				var relatedDeleteBackupRequests []velerov1.DeleteBackupRequest
				for _, deleteBackupRequest := range deleteBackupRequestList.Items {
					if deleteBackupRequest.Labels[velerov1.BackupNameLabel] == label.GetValidName(backup.Name) &&
						deleteBackupRequest.Labels[velerov1.BackupUIDLabel] == string(backup.UID) {
						relatedDeleteBackupRequests = append(relatedDeleteBackupRequests, deleteBackupRequest)
					}
				}
				var relatedPodVolumeBackupLists []velerov1.PodVolumeBackup
				for _, podVolumeBackup := range podVolumeBackupList.Items {
					if podVolumeBackup.Labels[velerov1.BackupNameLabel] == label.GetValidName(backup.Name) {
						relatedPodVolumeBackupLists = append(relatedPodVolumeBackupLists, podVolumeBackup)
					}
				}

				// TODO when to use insecureSkipTLSVerify and caCertFile?
				describeOutput := func(ctx context.Context) string {
					ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
					defer cancel()
					return output.DescribeBackup(ctx, clusterClient, &backup, relatedDeleteBackupRequests, relatedPodVolumeBackupLists, true, false, "")
				}(context.Background())

				writeTo := &bytes.Buffer{}
				// TODO when to use insecureSkipTLSVerify and caCertFile?
				// TODO user input on timeout?
				err := downloadrequest.Stream(context.Background(), clusterClient, backup.Namespace, backup.Name, velerov1.DownloadTargetKindBackupLog, writeTo, 5*time.Second, false, "")
				var logs string
				if err != nil {
					fmt.Println(err)
					logs = fmt.Sprintf("❌ %s", err)
				} else {
					logs = createFile(
						outputPath,
						folder+"/"+backup.Name+".log",
						writeTo.String(),
						"logs",
					)
				}
				yamlLink := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["BACKUPS"] += fmt.Sprintf(
					"| %v | %v | %s | %s | %s | %s |\n",
					namespace, backup.Name,
					backupStatus,
					createFile(
						outputPath,
						folder+"/describe-"+backup.Name+".txt",
						describeOutput,
						"describe",
					),
					logs,
					yamlLink,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["BACKUPS"] = "❌ No Backup was found in the cluster"
	}
}

func ReplaceRestoresSection(outputPath string, restoreListList *velerov1.RestoreList, clusterClient client.Client, podVolumeRestoreList *velerov1.PodVolumeRestoreList) {
	if restoreListList != nil && len(restoreListList.Items) != 0 {
		restoresByNamespace := map[string][]velerov1.Restore{}

		for _, restore := range restoreListList.Items {
			restoresByNamespace[restore.Namespace] = append(restoresByNamespace[restore.Namespace], restore)
		}

		summaryTemplateReplaces["RESTORES"] += "| Namespace | Name | status.phase | describe | logs | yaml |\n| --- | --- | --- | --- | --- | --- |\n"
		for namespace, restores := range restoresByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/velero.io/restores", namespace)
			file := folder + "/restores.yaml"
			for _, restore := range restores {
				restore.GetObjectKind().SetGroupVersionKind(gvk.RestoreGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &restore})

				restoreStatus := ""
				restoreStatusPhase := restore.Status.Phase
				if len(restoreStatusPhase) == 0 {
					restoreStatus = "⚠️ no status phase"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ Restore **%v** with **no status phase** in **%v** namespace\n\n",
						restore.Name, namespace,
					)
				} else {
					failedStates := []velerov1.RestorePhase{
						velerov1.RestorePhaseFailed,
						velerov1.RestorePhasePartiallyFailed,
						velerov1.RestorePhaseFinalizingPartiallyFailed,
						velerov1.RestorePhaseWaitingForPluginOperationsPartiallyFailed,
						velerov1.RestorePhaseFailedValidation,
					}
					if restoreStatusPhase == velerov1.RestorePhaseCompleted {
						restoreStatus = fmt.Sprintf("✅ status phase %s", restoreStatusPhase)
					} else if slices.Contains(failedStates, restoreStatusPhase) {
						restoreStatus = fmt.Sprintf("❌ status phase %s", restoreStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ Restore **%v** with **status phase %s** in **%v** namespace\n\n",
							restore.Name, restoreStatusPhase, namespace,
						)
					} else {
						restoreStatus = fmt.Sprintf("⚠️ status phase %s", restoreStatusPhase)
					}
				}

				var relatedPodVolumeRestoreLists []velerov1.PodVolumeRestore
				for _, podVolumeRestore := range podVolumeRestoreList.Items {
					if podVolumeRestore.Labels[velerov1.RestoreNameLabel] == label.GetValidName(restore.Name) {
						relatedPodVolumeRestoreLists = append(relatedPodVolumeRestoreLists, podVolumeRestore)
					}
				}

				// TODO when to use insecureSkipTLSVerify and caCertFile?
				describeOutput := func(ctx context.Context) string {
					ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
					defer cancel()
					return output.DescribeRestore(ctx, clusterClient, &restore, relatedPodVolumeRestoreLists, true, false, "")
				}(context.Background())

				writeTo := &bytes.Buffer{}
				// TODO when to use insecureSkipTLSVerify and caCertFile?
				// TODO user input on timeout?
				err := downloadrequest.Stream(context.Background(), clusterClient, restore.Namespace, restore.Name, velerov1.DownloadTargetKindRestoreLog, writeTo, 5*time.Second, false, "")
				var logs string
				if err != nil {
					fmt.Println(err)
					logs = fmt.Sprintf("❌ %s", err)
				} else {
					logs = createFile(
						outputPath,
						folder+"/"+restore.Name+".log",
						writeTo.String(),
						"logs",
					)
				}

				yamllink := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["RESTORES"] += fmt.Sprintf(
					"| %v | %v | %s | %s | %s | %s |\n",
					namespace, restore.Name,
					restoreStatus,
					createFile(
						outputPath,
						folder+"/describe-"+restore.Name+".txt",
						describeOutput,
						"describe",
					),
					logs,
					yamllink,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["RESTORES"] = "❌ No Restore was found in the cluster"
	}
}

func ReplaceSchedulesSection(outputPath string, scheduleList *velerov1.ScheduleList) {
	if scheduleList != nil && len(scheduleList.Items) != 0 {
		schedulesByNamespace := map[string][]velerov1.Schedule{}

		for _, schedule := range scheduleList.Items {
			schedulesByNamespace[schedule.Namespace] = append(schedulesByNamespace[schedule.Namespace], schedule)
		}

		summaryTemplateReplaces["SCHEDULES"] += "| Namespace | Name | status.phase | yaml |\n| --- | --- | --- | --- |\n"
		for namespace, schedules := range schedulesByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/velero.io/schedules", namespace)
			file := folder + "/schedules.yaml"
			for _, schedule := range schedules {
				schedule.GetObjectKind().SetGroupVersionKind(gvk.ScheduleGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &schedule})

				scheduleStatus := ""
				scheduleStatusPhase := schedule.Status.Phase
				if len(scheduleStatusPhase) == 0 {
					scheduleStatus = "⚠️ no status phase"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ Schedule **%v** with **no status phase** in **%v** namespace\n\n",
						schedule.Name, namespace,
					)
				} else {
					if scheduleStatusPhase == velerov1.SchedulePhaseEnabled {
						scheduleStatus = fmt.Sprintf("✅ status phase %s", scheduleStatusPhase)
					} else if scheduleStatusPhase == velerov1.SchedulePhaseFailedValidation {
						scheduleStatus = fmt.Sprintf("❌ status phase %s", scheduleStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ Schedule **%v** with **status phase %s** in **%v** namespace\n\n",
							schedule.Name, scheduleStatusPhase, namespace,
						)
					} else {
						scheduleStatus = fmt.Sprintf("⚠️ status phase %s", scheduleStatusPhase)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["SCHEDULES"] += fmt.Sprintf(
					"| %v | %v | %s | %s |\n",
					namespace, schedule.Name, scheduleStatus, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["SCHEDULES"] = "❌ No Schedule was found in the cluster"
	}
}

func ReplaceBackupRepositoriesSection(outputPath string, backupRepositoryList *velerov1.BackupRepositoryList) {
	if backupRepositoryList != nil && len(backupRepositoryList.Items) != 0 {
		backupRepositoriesByNamespace := map[string][]velerov1.BackupRepository{}

		for _, backupRepository := range backupRepositoryList.Items {
			backupRepositoriesByNamespace[backupRepository.Namespace] = append(backupRepositoriesByNamespace[backupRepository.Namespace], backupRepository)
		}

		summaryTemplateReplaces["BACKUPS_REPOSITORIES"] += "| Namespace | Name | status.phase | yaml |\n| --- | --- | --- | --- |\n"
		for namespace, backupRepositories := range backupRepositoriesByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/velero.io/backuprepositories", namespace)
			file := folder + "/backuprepositories.yaml"
			for _, backupRepository := range backupRepositories {
				backupRepository.GetObjectKind().SetGroupVersionKind(gvk.BackupRepositoryGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &backupRepository})

				backupRepositoryStatus := ""
				backupRepositoryStatusPhase := backupRepository.Status.Phase
				if len(backupRepositoryStatusPhase) == 0 {
					backupRepositoryStatus = "⚠️ no status phase"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ BackupRepository **%v** with **no status phase** in **%v** namespace\n\n",
						backupRepository.Name, namespace,
					)
				} else {
					if backupRepositoryStatusPhase == velerov1.BackupRepositoryPhaseReady {
						backupRepositoryStatus = fmt.Sprintf("✅ status phase %s", backupRepositoryStatusPhase)
					} else if backupRepositoryStatusPhase == velerov1.BackupRepositoryPhaseNotReady {
						backupRepositoryStatus = fmt.Sprintf("❌ status phase %s", backupRepositoryStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ BackupRepository **%v** with **status phase %s** in **%v** namespace\n\n",
							backupRepository.Name, backupRepositoryStatusPhase, namespace,
						)
					} else {
						backupRepositoryStatus = fmt.Sprintf("⚠️ status phase %s", backupRepositoryStatusPhase)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["BACKUPS_REPOSITORIES"] += fmt.Sprintf(
					"| %v | %v | %s | %s |\n",
					namespace, backupRepository.Name, backupRepositoryStatus, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["BACKUPS_REPOSITORIES"] = "❌ No BackupRepository was found in the cluster"
	}
}

func ReplaceDataUploadsSection(outputPath string, dataUploadList *velerov2alpha1.DataUploadList) {
	if dataUploadList != nil && len(dataUploadList.Items) != 0 {
		dataUploadByNamespace := map[string][]velerov2alpha1.DataUpload{}

		for _, dataUpload := range dataUploadList.Items {
			dataUploadByNamespace[dataUpload.Namespace] = append(dataUploadByNamespace[dataUpload.Namespace], dataUpload)
		}

		summaryTemplateReplaces["DATA_UPLOADS"] += "| Namespace | Name | status.phase | yaml |\n| --- | --- | --- | --- |\n"
		for namespace, dataUploads := range dataUploadByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/velero.io/datauploads", namespace)
			file := folder + "/datauploads.yaml"
			for _, dataUpload := range dataUploads {
				dataUpload.GetObjectKind().SetGroupVersionKind(gvk.DataUploadGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &dataUpload})

				dataUploadStatus := ""
				dataUploadStatusPhase := dataUpload.Status.Phase
				if len(dataUploadStatusPhase) == 0 {
					dataUploadStatus = "⚠️ no status phase"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ DataUpload **%v** with **no status phase** in **%v** namespace\n\n",
						dataUpload.Name, namespace,
					)
				} else {
					failedStates := []velerov2alpha1.DataUploadPhase{
						velerov2alpha1.DataUploadPhaseCanceling,
						velerov2alpha1.DataUploadPhaseCanceled,
						velerov2alpha1.DataUploadPhaseFailed,
					}
					if dataUploadStatusPhase == velerov2alpha1.DataUploadPhaseCompleted {
						dataUploadStatus = fmt.Sprintf("✅ status phase %s", dataUploadStatusPhase)
					} else if slices.Contains(failedStates, dataUploadStatusPhase) {
						dataUploadStatus = fmt.Sprintf("❌ status phase %s", dataUploadStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ DataUpload **%v** with **status phase %s** in **%v** namespace\n\n",
							dataUpload.Name, dataUploadStatusPhase, namespace,
						)
					} else {
						dataUploadStatus = fmt.Sprintf("⚠️ status phase %s", dataUploadStatusPhase)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["DATA_UPLOADS"] += fmt.Sprintf(
					"| %v | %v | %s | %s |\n",
					namespace, dataUpload.Name, dataUploadStatus, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["DATA_UPLOADS"] = "❌ No DataUpload was found in the cluster"
	}
}

func ReplaceDataDownloadsSection(outputPath string, dataDownloadList *velerov2alpha1.DataDownloadList) {
	if dataDownloadList != nil && len(dataDownloadList.Items) != 0 {
		dataDownloadByNamespace := map[string][]velerov2alpha1.DataDownload{}

		for _, dataDownload := range dataDownloadList.Items {
			dataDownloadByNamespace[dataDownload.Namespace] = append(dataDownloadByNamespace[dataDownload.Namespace], dataDownload)
		}

		summaryTemplateReplaces["DATA_DOWNLOADS"] += "| Namespace | Name | status.phase | yaml |\n| --- | --- | --- | --- |\n"
		for namespace, dataDownloads := range dataDownloadByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/velero.io/datadownloads", namespace)
			file := folder + "/datadownloads.yaml"
			for _, dataDownload := range dataDownloads {
				dataDownload.GetObjectKind().SetGroupVersionKind(gvk.DataDownloadGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &dataDownload})

				dataDownloadStatus := ""
				dataDownloadStatusPhase := dataDownload.Status.Phase
				if len(dataDownloadStatusPhase) == 0 {
					dataDownloadStatus = "⚠️ no status phase"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ DataDownload **%v** with **no status phase** in **%v** namespace\n\n",
						dataDownload.Name, namespace,
					)
				} else {
					failedStates := []velerov2alpha1.DataDownloadPhase{
						velerov2alpha1.DataDownloadPhaseCanceling,
						velerov2alpha1.DataDownloadPhaseCanceled,
						velerov2alpha1.DataDownloadPhaseFailed,
					}
					if dataDownloadStatusPhase == velerov2alpha1.DataDownloadPhaseCompleted {
						dataDownloadStatus = fmt.Sprintf("✅ status phase %s", dataDownloadStatusPhase)
					} else if slices.Contains(failedStates, dataDownloadStatusPhase) {
						dataDownloadStatus = fmt.Sprintf("❌ status phase %s", dataDownloadStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ DataDownload **%v** with **status phase %s** in **%v** namespace\n\n",
							dataDownload.Name, dataDownloadStatusPhase, namespace,
						)
					} else {
						dataDownloadStatus = fmt.Sprintf("⚠️ status phase %s", dataDownloadStatusPhase)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["DATA_DOWNLOADS"] += fmt.Sprintf(
					"| %v | %v | %s | %s |\n",
					namespace, dataDownload.Name, dataDownloadStatus, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["DATA_DOWNLOADS"] = "❌ No DataDownload was found in the cluster"
	}
}

func ReplacePodVolumeBackupsSection(outputPath string, podVolumeBackupList *velerov1.PodVolumeBackupList) {
	if podVolumeBackupList != nil && len(podVolumeBackupList.Items) != 0 {
		podVolumeBackupsByNamespace := map[string][]velerov1.PodVolumeBackup{}

		for _, podVolumeBackup := range podVolumeBackupList.Items {
			podVolumeBackupsByNamespace[podVolumeBackup.Namespace] = append(podVolumeBackupsByNamespace[podVolumeBackup.Namespace], podVolumeBackup)
		}

		summaryTemplateReplaces["POD_VOLUME_BACKUPS"] += "| Namespace | Name | status.phase | yaml |\n| --- | --- | --- | --- |\n"
		for namespace, podVolumeBackups := range podVolumeBackupsByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/velero.io/podvolumebackups", namespace)
			file := folder + "/podvolumebackups.yaml"
			for _, podVolumeBackup := range podVolumeBackups {
				podVolumeBackup.GetObjectKind().SetGroupVersionKind(gvk.PodVolumeBackupGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &podVolumeBackup})

				podVolumeBackupStatus := ""
				podVolumeBackupStatusPhase := podVolumeBackup.Status.Phase
				if len(podVolumeBackupStatusPhase) == 0 {
					podVolumeBackupStatus = "⚠️ no status phase"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ PodVolumeBackup **%v** with **no status phase** in **%v** namespace\n\n",
						podVolumeBackup.Name, namespace,
					)
				} else {
					if podVolumeBackupStatusPhase == velerov1.PodVolumeBackupPhaseCompleted {
						podVolumeBackupStatus = fmt.Sprintf("✅ status phase %s", podVolumeBackupStatusPhase)
					} else if podVolumeBackupStatusPhase == velerov1.PodVolumeBackupPhaseFailed {
						podVolumeBackupStatus = fmt.Sprintf("❌ status phase %s", podVolumeBackupStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ PodVolumeBackup **%v** with **status phase %s** in **%v** namespace\n\n",
							podVolumeBackup.Name, podVolumeBackupStatusPhase, namespace,
						)
					} else {
						podVolumeBackupStatus = fmt.Sprintf("⚠️ status phase %s", podVolumeBackupStatusPhase)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["POD_VOLUME_BACKUPS"] += fmt.Sprintf(
					"| %v | %v | %s | %s |\n",
					namespace, podVolumeBackup.Name, podVolumeBackupStatus, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["POD_VOLUME_BACKUPS"] = "❌ No PodVolumeBackup was found in the cluster"
	}
}

func ReplacePodVolumeRestoresSection(outputPath string, podVolumeRestoreList *velerov1.PodVolumeRestoreList) {
	if podVolumeRestoreList != nil && len(podVolumeRestoreList.Items) != 0 {
		podVolumeRestoresByNamespace := map[string][]velerov1.PodVolumeRestore{}

		for _, podVolumeRestore := range podVolumeRestoreList.Items {
			podVolumeRestoresByNamespace[podVolumeRestore.Namespace] = append(podVolumeRestoresByNamespace[podVolumeRestore.Namespace], podVolumeRestore)
		}

		summaryTemplateReplaces["POD_VOLUME_RESTORES"] += "| Namespace | Name | status.phase | yaml |\n| --- | --- | --- | --- |\n"
		for namespace, podVolumeRestores := range podVolumeRestoresByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/velero.io/podvolumerestores", namespace)
			file := folder + "/podvolumerestores.yaml"
			for _, podVolumeRestore := range podVolumeRestores {
				podVolumeRestore.GetObjectKind().SetGroupVersionKind(gvk.PodVolumeRestoreGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &podVolumeRestore})

				podVolumeRestoreStatus := ""
				podVolumeRestoreStatusPhase := podVolumeRestore.Status.Phase
				if len(podVolumeRestoreStatusPhase) == 0 {
					podVolumeRestoreStatus = "⚠️ no status phase"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ PodVolumeRestore **%v** with **no status phase** in **%v** namespace\n\n",
						podVolumeRestore.Name, namespace,
					)
				} else {
					if podVolumeRestoreStatusPhase == velerov1.PodVolumeRestorePhaseCompleted {
						podVolumeRestoreStatus = fmt.Sprintf("✅ status phase %s", podVolumeRestoreStatusPhase)
					} else if podVolumeRestoreStatusPhase == velerov1.PodVolumeRestorePhaseFailed {
						podVolumeRestoreStatus = fmt.Sprintf("❌ status phase %s", podVolumeRestoreStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ PodVolumeRestore **%v** with **status phase %s** in **%v** namespace\n\n",
							podVolumeRestore.Name, podVolumeRestoreStatusPhase, namespace,
						)
					} else {
						podVolumeRestoreStatus = fmt.Sprintf("⚠️ status phase %s", podVolumeRestoreStatusPhase)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["POD_VOLUME_RESTORES"] += fmt.Sprintf(
					"| %v | %v | %s | %s |\n",
					namespace, podVolumeRestore.Name, podVolumeRestoreStatus, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["POD_VOLUME_RESTORES"] = "❌ No PodVolumeRestore was found in the cluster"
	}
}

func ReplaceDownloadRequestsSection(outputPath string, downloadRequestList *velerov1.DownloadRequestList) {
	if downloadRequestList != nil && len(downloadRequestList.Items) != 0 {
		downloadRequestsByNamespace := map[string][]velerov1.DownloadRequest{}

		for _, downloadRequest := range downloadRequestList.Items {
			downloadRequestsByNamespace[downloadRequest.Namespace] = append(downloadRequestsByNamespace[downloadRequest.Namespace], downloadRequest)
		}

		summaryTemplateReplaces["DOWNLOAD_REQUESTS"] += "| Namespace | Name | status.phase | yaml |\n| --- | --- | --- | --- |\n"
		for namespace, downloadRequests := range downloadRequestsByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/velero.io/downloadrequests", namespace)
			file := folder + "/downloadrequests.yaml"
			for _, downloadRequest := range downloadRequests {
				downloadRequest.GetObjectKind().SetGroupVersionKind(gvk.DownloadRequestGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &downloadRequest})

				downloadRequestStatus := ""
				downloadRequestStatusPhase := downloadRequest.Status.Phase
				if len(downloadRequestStatusPhase) == 0 {
					downloadRequestStatus = "⚠️ no status"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ DownloadRequest **%v** with **no status** in **%v** namespace\n\n",
						downloadRequest.Name, namespace,
					)
				} else {
					if downloadRequestStatusPhase == velerov1.DownloadRequestPhaseProcessed {
						downloadRequestStatus = fmt.Sprintf("✅ status phase %s", downloadRequestStatusPhase)
					} else {
						downloadRequestStatus = fmt.Sprintf("⚠️ status phase %s", downloadRequestStatusPhase)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["DOWNLOAD_REQUESTS"] += fmt.Sprintf(
					"| %v | %v | %s | %s |\n",
					namespace, downloadRequest.Name, downloadRequestStatus, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["DOWNLOAD_REQUESTS"] = "❌ No DownloadRequest was found in the cluster"
	}
}

func ReplaceDeleteBackupRequestsSection(outputPath string, deleteBackupRequestList *velerov1.DeleteBackupRequestList) {
	if deleteBackupRequestList != nil && len(deleteBackupRequestList.Items) != 0 {
		deleteBackupRequestsByNamespace := map[string][]velerov1.DeleteBackupRequest{}

		for _, deleteBackupRequest := range deleteBackupRequestList.Items {
			deleteBackupRequestsByNamespace[deleteBackupRequest.Namespace] = append(deleteBackupRequestsByNamespace[deleteBackupRequest.Namespace], deleteBackupRequest)
		}

		summaryTemplateReplaces["DELETE_BACKUP_REQUESTS"] += "| Namespace | Name | status.phase | yaml |\n| --- | --- | --- | --- |\n"
		for namespace, deleteBackupRequests := range deleteBackupRequestsByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/velero.io/deletebackuprequests", namespace)
			file := folder + "/deletebackuprequests.yaml"
			for _, deleteBackupRequest := range deleteBackupRequests {
				deleteBackupRequest.GetObjectKind().SetGroupVersionKind(gvk.DeleteBackupRequestGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &deleteBackupRequest})

				deleteBackupRequestStatus := ""
				deleteBackupRequestStatusPhase := deleteBackupRequest.Status.Phase
				if len(deleteBackupRequestStatusPhase) == 0 {
					deleteBackupRequestStatus = "⚠️ no status"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ DeleteBackupRequest **%v** with **no status** in **%v** namespace\n\n",
						deleteBackupRequest.Name, namespace,
					)
				} else {
					if deleteBackupRequestStatusPhase == velerov1.DeleteBackupRequestPhaseProcessed {
						deleteBackupRequestStatus = fmt.Sprintf("✅ status phase %s", deleteBackupRequestStatusPhase)
					} else {
						deleteBackupRequestStatus = fmt.Sprintf("⚠️ status phase %s", deleteBackupRequestStatusPhase)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["DELETE_BACKUP_REQUESTS"] += fmt.Sprintf(
					"| %v | %v | %s | %s |\n",
					namespace, deleteBackupRequest.Name, deleteBackupRequestStatus, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["DELETE_BACKUP_REQUESTS"] = "❌ No DeleteBackupRequest was found in the cluster"
	}
}

func ReplaceServerStatusRequestsSection(outputPath string, serverStatusRequestList *velerov1.ServerStatusRequestList) {
	if serverStatusRequestList != nil && len(serverStatusRequestList.Items) != 0 {
		serverStatusRequestsByNamespace := map[string][]velerov1.ServerStatusRequest{}

		for _, serverStatusRequest := range serverStatusRequestList.Items {
			serverStatusRequestsByNamespace[serverStatusRequest.Namespace] = append(serverStatusRequestsByNamespace[serverStatusRequest.Namespace], serverStatusRequest)
		}

		summaryTemplateReplaces["SERVER_STATUS_REQUESTS"] += "| Namespace | Name | status.phase | yaml |\n| --- | --- | --- | --- |\n"
		for namespace, serverStatusRequests := range serverStatusRequestsByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/velero.io/serverstatusrequests", namespace)
			file := folder + "/serverstatusrequests.yaml"
			for _, serverStatusRequest := range serverStatusRequests {
				serverStatusRequest.GetObjectKind().SetGroupVersionKind(gvk.ServerStatusRequestGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &serverStatusRequest})

				serverStatusRequestStatus := ""
				serverStatusRequestStatusPhase := serverStatusRequest.Status.Phase
				if len(serverStatusRequestStatusPhase) == 0 {
					serverStatusRequestStatus = "⚠️ no status"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ ServerStatusRequest **%v** with **no status** in **%v** namespace\n\n",
						serverStatusRequest.Name, namespace,
					)
				} else {
					if serverStatusRequestStatusPhase == velerov1.ServerStatusRequestPhaseProcessed {
						serverStatusRequestStatus = fmt.Sprintf("✅ status phase %s", serverStatusRequestStatusPhase)
					} else {
						serverStatusRequestStatus = fmt.Sprintf("⚠️ status phase %s", serverStatusRequestStatusPhase)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["SERVER_STATUS_REQUESTS"] += fmt.Sprintf(
					"| %v | %v | %s | %s |\n",
					namespace, serverStatusRequest.Name, serverStatusRequestStatus, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["SERVER_STATUS_REQUESTS"] = "❌ No ServerStatusRequest was found in the cluster"
	}
}

func ReplaceNonAdminBackupStorageLocationRequestsSection(outputPath string, nonAdminBackupStorageLocationRequestList *nac1alpha1.NonAdminBackupStorageLocationRequestList) {
	if nonAdminBackupStorageLocationRequestList != nil && len(nonAdminBackupStorageLocationRequestList.Items) != 0 {
		nonAdminBackupStorageLocationRequestsByNamespace := map[string][]nac1alpha1.NonAdminBackupStorageLocationRequest{}

		for _, request := range nonAdminBackupStorageLocationRequestList.Items {
			nonAdminBackupStorageLocationRequestsByNamespace[request.Namespace] = append(nonAdminBackupStorageLocationRequestsByNamespace[request.Namespace], request)
		}

		summaryTemplateReplaces["NON_ADMIN_BACKUP_STORAGE_LOCATION_REQUESTS"] += "| Namespace | Name | status.phase | yaml |\n| --- | --- | --- | --- |\n"
		for namespace, requests := range nonAdminBackupStorageLocationRequestsByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/oadp.openshift.io/nonadminbackupstoragelocationrequests", namespace)
			file := folder + "/nonadminbackupstoragelocationrequests.yaml"
			for _, request := range requests {
				request.GetObjectKind().SetGroupVersionKind(gvk.NonAdminBackupStorageLocationRequestGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &request})

				requestStatus := ""
				requestStatusPhase := request.Status.Phase
				if len(requestStatusPhase) == 0 {
					requestStatus = "⚠️ no status phase"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ NonAdminBackupStorageLocationRequest **%v** with **no status phase** in **%v** namespace\n\n",
						request.Name, namespace,
					)
				} else {
					if requestStatusPhase == nac1alpha1.NonAdminBSLRequestPhaseApproved {
						requestStatus = fmt.Sprintf("✅ status phase %s", requestStatusPhase)
					} else if requestStatusPhase == nac1alpha1.NonAdminBSLRequestPhaseRejected {
						requestStatus = fmt.Sprintf("❌ status phase %s", requestStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ NonAdminBackupStorageLocationRequest **%v** with **status phase %s** in **%v** namespace\n\n",
							request.Name, requestStatusPhase, namespace,
						)
					} else {
						requestStatus = fmt.Sprintf("⚠️ status phase %s", requestStatusPhase)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["NON_ADMIN_BACKUP_STORAGE_LOCATION_REQUESTS"] += fmt.Sprintf(
					"| %v | %v | %v | %s |\n",
					namespace, request.Name, requestStatus, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["NON_ADMIN_BACKUP_STORAGE_LOCATION_REQUESTS"] = "❌ No NonAdminBackupStorageLocationRequest was found in the cluster"
	}
}

func ReplaceNonAdminBackupStorageLocationsSection(outputPath string, nonAdminBackupStorageLocationList *nac1alpha1.NonAdminBackupStorageLocationList) {
	if nonAdminBackupStorageLocationList != nil && len(nonAdminBackupStorageLocationList.Items) != 0 {
		nonAdminBackupStorageLocationsByNamespace := map[string][]nac1alpha1.NonAdminBackupStorageLocation{}

		for _, backupStorageLocation := range nonAdminBackupStorageLocationList.Items {
			nonAdminBackupStorageLocationsByNamespace[backupStorageLocation.Namespace] = append(nonAdminBackupStorageLocationsByNamespace[backupStorageLocation.Namespace], backupStorageLocation)
		}

		summaryTemplateReplaces["NON_ADMIN_BACKUP_STORAGE_LOCATIONS"] += "| Namespace | Name | Approved | status.phase | yaml |\n| --- | --- | --- | --- | --- |\n"
		for namespace, backupStorageLocations := range nonAdminBackupStorageLocationsByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/oadp.openshift.io/nonadminbackupstoragelocations", namespace)
			file := folder + "/nonadminbackupstoragelocations.yaml"
			for _, backupStorageLocation := range backupStorageLocations {
				backupStorageLocation.GetObjectKind().SetGroupVersionKind(gvk.NonAdminBackupStorageLocationGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &backupStorageLocation})

				bslStatus := ""
				bslStatusPhase := backupStorageLocation.Status.Phase
				if len(bslStatusPhase) == 0 {
					bslStatus = "⚠️ no status phase"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ NonAdminBackupStorageLocation **%v** with **no status phase** in **%v** namespace\n\n",
						backupStorageLocation.Name, namespace,
					)
				} else {
					if bslStatusPhase == nac1alpha1.NonAdminPhaseCreated {
						bslStatus = fmt.Sprintf("✅ status phase %s", bslStatusPhase)
					} else if bslStatusPhase == nac1alpha1.NonAdminPhaseBackingOff {
						bslStatus = fmt.Sprintf("❌ status phase %s", bslStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ NonAdminBackupStorageLocation **%v** with **status phase %s** in **%v** namespace\n\n",
							backupStorageLocation.Name, bslStatusPhase, namespace,
						)
					} else {
						bslStatus = fmt.Sprintf("⚠️ status phase %s", bslStatusPhase)
					}
				}
				bslStatusApproved := ""
				conditionInNABSL := meta.FindStatusCondition(backupStorageLocation.Status.Conditions, string(nac1alpha1.NonAdminBSLConditionApproved))
				if conditionInNABSL == nil {
					bslStatusApproved = "⚠️ no status condition approved"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ NonAdminBackupStorageLocation **%v** with **no status condition approved** in **%v** namespace\n\n",
						backupStorageLocation.Name, namespace,
					)
				} else {
					if conditionInNABSL.Status == v1.ConditionTrue {
						bslStatusApproved = fmt.Sprintf("✅ status condition approved %s", conditionInNABSL.Status)
					} else {
						bslStatusApproved = fmt.Sprintf("❌ status condition approved %s", conditionInNABSL.Status)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["NON_ADMIN_BACKUP_STORAGE_LOCATIONS"] += fmt.Sprintf(
					"| %v | %v | %s | %v | %s |\n",
					namespace, backupStorageLocation.Name, bslStatusApproved, bslStatus, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["NON_ADMIN_BACKUP_STORAGE_LOCATIONS"] = "❌ No NonAdminBackupStorageLocation was found in the cluster"
	}
}

func ReplaceNonAdminBackupsSection(outputPath string, nonAdminBackupList *nac1alpha1.NonAdminBackupList) {
	if nonAdminBackupList != nil && len(nonAdminBackupList.Items) != 0 {
		backupsByNamespace := map[string][]nac1alpha1.NonAdminBackup{}

		for _, backup := range nonAdminBackupList.Items {
			backupsByNamespace[backup.Namespace] = append(backupsByNamespace[backup.Namespace], backup)
		}

		summaryTemplateReplaces["NON_ADMIN_BACKUPS"] += "| Namespace | Name | status.phase | yaml |\n| --- | --- | --- | --- |\n"
		for namespace, backups := range backupsByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/oadp.openshift.io/nonadminbackups", namespace)
			file := folder + "/nonadminbackups.yaml"
			for _, backup := range backups {
				backup.GetObjectKind().SetGroupVersionKind(gvk.NonAdminBackupGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &backup})

				backupStatus := ""
				backupStatusPhase := backup.Status.Phase
				if len(backupStatusPhase) == 0 {
					backupStatus = "⚠️ no status phase"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ NonAdminBackup **%v** with **no status phase** in **%v** namespace\n\n",
						backup.Name, namespace,
					)
				} else {
					if backupStatusPhase == nac1alpha1.NonAdminPhaseCreated {
						backupStatus = fmt.Sprintf("✅ status phase %s", backupStatusPhase)
					} else if backupStatusPhase == nac1alpha1.NonAdminPhaseBackingOff {
						backupStatus = fmt.Sprintf("❌ status phase %s", backupStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ NonAdminBackup **%v** with **status phase %s** in **%v** namespace\n\n",
							backup.Name, backupStatusPhase, namespace,
						)
					} else {
						backupStatus = fmt.Sprintf("⚠️ status phase %s", backupStatusPhase)
					}
				}

				yamlLink := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["NON_ADMIN_BACKUPS"] += fmt.Sprintf(
					"| %v | %v | %s | %s |\n",
					namespace, backup.Name,
					backupStatus,
					yamlLink,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["NON_ADMIN_BACKUPS"] = "❌ No NonAdminBackup was found in the cluster"
	}
}

func ReplaceNonAdminRestoresSection(outputPath string, nonAdminRestoreList *nac1alpha1.NonAdminRestoreList) {
	if nonAdminRestoreList != nil && len(nonAdminRestoreList.Items) != 0 {
		restoresByNamespace := map[string][]nac1alpha1.NonAdminRestore{}

		for _, restore := range nonAdminRestoreList.Items {
			restoresByNamespace[restore.Namespace] = append(restoresByNamespace[restore.Namespace], restore)
		}

		summaryTemplateReplaces["NON_ADMIN_RESTORES"] += "| Namespace | Name | status.phase | describe | logs | yaml |\n| --- | --- | --- | --- | --- | --- |\n"
		for namespace, restores := range restoresByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/oadp.openshift.io/nonadminrestores", namespace)
			file := folder + "/nonadminrestores.yaml"
			for _, restore := range restores {
				restore.GetObjectKind().SetGroupVersionKind(gvk.NonAdminRestoreGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &restore})

				restoreStatus := ""
				restoreStatusPhase := restore.Status.Phase
				if len(restoreStatusPhase) == 0 {
					restoreStatus = "⚠️ no status phase"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ NonAdminRestore **%v** with **no status phase** in **%v** namespace\n\n",
						restore.Name, namespace,
					)
				} else {
					if restoreStatusPhase == nac1alpha1.NonAdminPhaseCreated {
						restoreStatus = fmt.Sprintf("✅ status phase %s", restoreStatusPhase)
					} else if restoreStatusPhase == nac1alpha1.NonAdminPhaseBackingOff {
						restoreStatus = fmt.Sprintf("❌ status phase %s", restoreStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ NonAdminRestore **%v** with **status phase %s** in **%v** namespace\n\n",
							restore.Name, restoreStatusPhase, namespace,
						)
					} else {
						restoreStatus = fmt.Sprintf("⚠️ status phase %s", restoreStatusPhase)
					}
				}

				yamllink := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["NON_ADMIN_RESTORES"] += fmt.Sprintf(
					"| %v | %v | %s | %s |\n",
					namespace, restore.Name,
					restoreStatus,
					yamllink,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["NON_ADMIN_RESTORES"] = "❌ No NonAdminRestore was found in the cluster"
	}
}

func ReplaceNonAdminDownloadRequestsSection(outputPath string, nonAdminDownloadRequestList *nac1alpha1.NonAdminDownloadRequestList) {
	if nonAdminDownloadRequestList != nil && len(nonAdminDownloadRequestList.Items) != 0 {
		downloadRequestsByNamespace := map[string][]nac1alpha1.NonAdminDownloadRequest{}

		for _, downloadRequest := range nonAdminDownloadRequestList.Items {
			downloadRequestsByNamespace[downloadRequest.Namespace] = append(downloadRequestsByNamespace[downloadRequest.Namespace], downloadRequest)
		}

		summaryTemplateReplaces["NON_ADMIN_DOWNLOAD_REQUESTS"] += "| Namespace | Name | status.phase | yaml |\n| --- | --- | --- | --- |\n"
		for namespace, downloadRequests := range downloadRequestsByNamespace {
			list := &corev1.List{}
			list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

			folder := fmt.Sprintf("namespaces/%s/oadp.openshift.io/nonadmindownloadrequests", namespace)
			file := folder + "/nonadmindownloadrequests.yaml"
			for _, downloadRequest := range downloadRequests {
				downloadRequest.GetObjectKind().SetGroupVersionKind(gvk.NonAdminDownloadRequestGVK)
				list.Items = append(list.Items, runtime.RawExtension{Object: &downloadRequest})

				downloadRequestStatus := ""
				downloadRequestStatusPhase := downloadRequest.Status.Phase
				if len(downloadRequestStatusPhase) == 0 {
					downloadRequestStatus = "⚠️ no status"
					summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
						"⚠️ NonAdminDownloadRequest **%v** with **no status** in **%v** namespace\n\n",
						downloadRequest.Name, namespace,
					)
				} else {
					if downloadRequestStatusPhase == nac1alpha1.NonAdminPhaseCreated {
						downloadRequestStatus = fmt.Sprintf("✅ status phase %s", downloadRequestStatusPhase)
					} else if downloadRequestStatusPhase == nac1alpha1.NonAdminPhaseBackingOff {
						downloadRequestStatus = fmt.Sprintf("❌ status phase %s", downloadRequestStatusPhase)
						summaryTemplateReplaces["ERRORS"] += fmt.Sprintf(
							"❌ NonAdminDownloadRequest **%v** with **status phase %s** in **%v** namespace\n\n",
							downloadRequest.Name, downloadRequestStatusPhase, namespace,
						)
					} else {
						downloadRequestStatus = fmt.Sprintf("⚠️ status phase %s", downloadRequestStatusPhase)
					}
				}

				link := fmt.Sprintf("[`yaml`](%s)", file)
				summaryTemplateReplaces["NON_ADMIN_DOWNLOAD_REQUESTS"] += fmt.Sprintf(
					"| %v | %v | %s | %s |\n",
					namespace, downloadRequest.Name, downloadRequestStatus, link,
				)
			}

			createYAML(outputPath, file, list)
		}
	} else {
		summaryTemplateReplaces["NON_ADMIN_DOWNLOAD_REQUESTS"] = "❌ No NonAdminDownloadRequest was found in the cluster"
	}
}

// TODO was not able to create generic replace section function

// TODO this function writes summary and cluster files
// break into 2
func ReplaceAvailableStorageClassesSection(outputPath string, storageClassList *storagev1.StorageClassList) {
	if storageClassList != nil && len(storageClassList.Items) != 0 {
		list := &corev1.List{}
		list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

		for _, storageClass := range storageClassList.Items {
			storageClass.GetObjectKind().SetGroupVersionKind(gvk.StorageClassGVK)
			list.Items = append(list.Items, runtime.RawExtension{Object: &storageClass})
		}
		// TODO could not create generic function, type/interface/pointer error
		// createYAMLList(storageClassList, gvk.StorageClassGVK)
		summaryTemplateReplaces["STORAGE_CLASSES"] = createYAML(outputPath, "cluster-scoped-resources/storage.k8s.io/storageclasses/storageclasses.yaml", list)
	} else {
		summaryTemplateReplaces["STORAGE_CLASSES"] = "❌ No StorageClass was found in the cluster"
		summaryTemplateReplaces["ERRORS"] += "⚠️ No StorageClass was found in the cluster\n\n"
	}
}

func ReplaceAvailableVolumeSnapshotClassesSection(outputPath string, volumeSnapshotClassList *volumesnapshotv1.VolumeSnapshotClassList) {
	if volumeSnapshotClassList != nil && len(volumeSnapshotClassList.Items) != 0 {
		list := &corev1.List{}
		list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

		for _, volumeSnapshotClass := range volumeSnapshotClassList.Items {
			volumeSnapshotClass.GetObjectKind().SetGroupVersionKind(gvk.VolumeSnapshotClassGVK)
			list.Items = append(list.Items, runtime.RawExtension{Object: &volumeSnapshotClass})
		}
		summaryTemplateReplaces["VOLUME_SNAPSHOT_CLASSES"] = createYAML(outputPath, "cluster-scoped-resources/snapshot.storage.k8s.io/volumesnapshotclasses/volumesnapshotclasses.yaml", list)
	} else {
		summaryTemplateReplaces["VOLUME_SNAPSHOT_CLASSES"] = "❌ No VolumeSnapshotClass was found in the cluster"
		summaryTemplateReplaces["ERRORS"] += "⚠️ No VolumeSnapshotClass was found in the cluster\n\n"
	}
}

func ReplaceAvailableCSIDriversSection(outputPath string, csiDriverList *storagev1.CSIDriverList, oadpOpenShiftVersion string) {
	if csiDriverList != nil && len(csiDriverList.Items) != 0 {
		list := &corev1.List{}
		list.GetObjectKind().SetGroupVersionKind(gvk.ListGVK)

		for _, csiDriver := range csiDriverList.Items {
			csiDriver.GetObjectKind().SetGroupVersionKind(gvk.CSIDriverGVK)
			list.Items = append(list.Items, runtime.RawExtension{Object: &csiDriver})
		}
		summaryTemplateReplaces["CSI_DRIVERS"] = createYAML(outputPath, "cluster-scoped-resources/storage.k8s.io/csidrivers/csidrivers.yaml", list)
	} else {
		summaryTemplateReplaces["CSI_DRIVERS"] = "❌ No CSIDriver was found in the cluster"
		summaryTemplateReplaces["ERRORS"] += "⚠️ No CSIDriver was found in the cluster\n\n"
	}
	summaryTemplateReplaces["OADP_OCP_VERSION"] = oadpOpenShiftVersion
}

func ReplaceCustomResourceDefinitionsSection(outputPath string, clusterConfig *rest.Config) {
	// TODO error!!!
	client, _ := apiextensionsclientset.NewForConfig(clusterConfig)

	crdsPath := "cluster-scoped-resources/apiextensions.k8s.io/customresourcedefinitions"

	// CRD spec.names.plural : CRD spec.group
	crds := map[string]string{
		"dataprotectionapplications":            gvk.DataProtectionApplicationGVK.Group,
		"cloudstorages":                         gvk.CloudStorageGVK.Group,
		"backupstoragelocations":                gvk.BackupStorageLocationGVK.Group,
		"volumesnapshotlocations":               gvk.VolumeSnapshotLocationGVK.Group,
		"backups":                               gvk.BackupGVK.Group,
		"restores":                              gvk.RestoreGVK.Group,
		"schedules":                             gvk.ScheduleGVK.Group,
		"backuprepositories":                    gvk.BackupRepositoryGVK.Group,
		"datauploads":                           gvk.DataUploadGVK.Group,
		"datadownloads":                         gvk.DataDownloadGVK.Group,
		"podvolumebackups":                      gvk.PodVolumeBackupGVK.Group,
		"podvolumerestores":                     gvk.PodVolumeRestoreGVK.Group,
		"downloadrequests":                      gvk.DownloadRequestGVK.Group,
		"deletebackuprequests":                  gvk.DeleteBackupRequestGVK.Group,
		"serverstatusrequests":                  gvk.ServerStatusRequestGVK.Group,
		"nonadminbackupstoragelocationrequests": gvk.NonAdminBackupStorageLocationRequestGVK.Group,
		"nonadminbackupstoragelocations":        gvk.NonAdminBackupStorageLocationGVK.Group,
		"nonadminbackups":                       gvk.NonAdminBackupGVK.Group,
		"nonadminrestores":                      gvk.NonAdminRestoreGVK.Group,
		"nonadmindownloadrequests":              gvk.NonAdminDownloadRequestGVK.Group,
		"clusterserviceversions":                gvk.ClusterServiceVersionGVK.Group,
	}

	for crdName, crdGroup := range crds {
		crd, _ := client.ApiextensionsV1().CustomResourceDefinitions().Get(context.Background(), crdName+"."+crdGroup, v1.GetOptions{})
		crd.GetObjectKind().SetGroupVersionKind(gvk.CustomResourceDefinitionGVK)
		// TODO check error
		createYAML(outputPath, crdsPath+fmt.Sprintf("/%s.yaml", crdName), crd)
	}

	summaryTemplateReplaces["CUSTOM_RESOURCE_DEFINITION"] = fmt.Sprintf("For more information, check [`%s`](%s)\n\n", crdsPath, crdsPath)
}

// TODO move to another folder?
func createYAML(outputPath string, yamlPath string, obj runtime.Object) string {
	objFilePath := outputPath + yamlPath
	dir := path.Dir(objFilePath)
	// TODO permission
	// TODO need defer somewhere?
	err := os.MkdirAll(dir, 0777)
	if err != nil {
		return "❌ Unable to create dir " + dir
	}
	result := ""
	newFile, err := os.Create(objFilePath)
	if err != nil {
		fmt.Println(err)
		result = "❌ Unable to create file " + objFilePath
	} else {
		printer := printers.YAMLPrinter{}
		err = printer.PrintObj(obj, newFile)
		if err != nil {
			fmt.Println(err)
			result = "❌ Unable to write " + objFilePath
		} else {
			result = fmt.Sprintf("For more information, check [`%s`](%s)\n\n", yamlPath, yamlPath)
		}
	}
	defer newFile.Close()
	return result
}

func createFile(outputPath string, describePath string, describeOutput string, describeTitle string) string {
	describeFilePath := outputPath + describePath
	dir := path.Dir(describeFilePath)
	// TODO permission
	// TODO need defer somewhere?
	err := os.MkdirAll(dir, 0777)
	if err != nil {
		return "❌ Unable to create dir " + dir
	}
	result := ""
	newFile, err := os.Create(describeFilePath)
	if err != nil {
		fmt.Println(err)
		result = "❌ Unable to create file " + describeFilePath
	} else {
		err := os.WriteFile(describeFilePath, []byte(describeOutput), 0644)
		if err != nil {
			fmt.Println(err)
			result = "❌ Unable to write " + describeFilePath
		} else {
			result = fmt.Sprintf("[`"+describeTitle+"`](%s)", describePath)
		}
	}
	defer newFile.Close()
	return result
}

func Write(outputPath string) error {
	if len(summaryTemplateReplaces["ERRORS"]) == 0 {
		summaryTemplateReplaces["ERRORS"] += "No errors happened or were found while running OADP must-gather\n\n"
	}

	summary := summaryTemplate
	for _, key := range summaryTemplateReplacesKeys {
		value, ok := summaryTemplateReplaces[key]
		if !ok {
			return fmt.Errorf("key '%s' not set in SummaryTemplateReplaces", key)
		}
		if len(value) == 0 {
			return fmt.Errorf("value for key '%s' not set in SummaryTemplateReplaces", key)
		}
		summary = strings.ReplaceAll(
			summary,
			fmt.Sprintf("<<%s>>", key),
			value,
		)
	}

	summaryPath := outputPath + "oadp-must-gather-summary.md"
	// TODO permission
	// TODO need defer somewhere?
	err := os.WriteFile(summaryPath, []byte(summary), 0644)
	if err != nil {
		return err
	}

	return nil
}

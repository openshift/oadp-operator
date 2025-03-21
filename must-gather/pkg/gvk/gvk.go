package gvk

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	CustomResourceDefinitionGVK = schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	}
	ListGVK = schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "List",
	}
	ClusterServiceVersionGVK = schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "ClusterServiceVersion",
	}
	DataProtectionApplicationGVK = schema.GroupVersionKind{
		Group:   "oadp.openshift.io",
		Version: "v1alpha1",
		Kind:    "DataProtectionApplication",
	}
	CloudStorageGVK = schema.GroupVersionKind{
		Group:   "oadp.openshift.io",
		Version: "v1alpha1",
		Kind:    "CloudStorage",
	}
	BackupStorageLocationGVK = schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v1",
		Kind:    "BackupStorageLocation",
	}
	VolumeSnapshotLocationGVK = schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v1",
		Kind:    "VolumeSnapshotLocation",
	}
	BackupGVK = schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v1",
		Kind:    "Backup",
	}
	RestoreGVK = schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v1",
		Kind:    "Restore",
	}
	ScheduleGVK = schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v1",
		Kind:    "Schedule",
	}
	BackupRepositoryGVK = schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v1",
		Kind:    "BackupRepository",
	}
	DataUploadGVK = schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v2alpha1",
		Kind:    "DataUpload",
	}
	DataDownloadGVK = schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v2alpha1",
		Kind:    "DataDownload",
	}
	PodVolumeBackupGVK = schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v1",
		Kind:    "PodVolumeBackup",
	}
	PodVolumeRestoreGVK = schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v1",
		Kind:    "PodVolumeRestore",
	}
	DownloadRequestGVK = schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v1",
		Kind:    "DownloadRequest",
	}
	DeleteBackupRequestGVK = schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v1",
		Kind:    "DeleteBackupRequest",
	}
	ServerStatusRequestGVK = schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v1",
		Kind:    "ServerStatusRequest",
	}
	NonAdminBackupStorageLocationRequestGVK = schema.GroupVersionKind{
		Group:   "oadp.openshift.io",
		Version: "v1alpha1",
		Kind:    "NonAdminBackupStorageLocationRequest",
	}
	NonAdminBackupStorageLocationGVK = schema.GroupVersionKind{
		Group:   "oadp.openshift.io",
		Version: "v1alpha1",
		Kind:    "NonAdminBackupStorageLocation",
	}
	NonAdminBackupGVK = schema.GroupVersionKind{
		Group:   "oadp.openshift.io",
		Version: "v1alpha1",
		Kind:    "NonAdminBackup",
	}
	NonAdminRestoreGVK = schema.GroupVersionKind{
		Group:   "oadp.openshift.io",
		Version: "v1alpha1",
		Kind:    "NonAdminRestore",
	}
	NonAdminDownloadRequestGVK = schema.GroupVersionKind{
		Group:   "oadp.openshift.io",
		Version: "v1alpha1",
		Kind:    "NonAdminDownloadRequest",
	}
	StorageClassGVK = schema.GroupVersionKind{
		Group:   "storage.k8s.io",
		Version: "v1",
		Kind:    "StorageClass",
	}
	VolumeSnapshotClassGVK = schema.GroupVersionKind{
		Group:   "snapshot.storage.k8s.io",
		Version: "v1",
		Kind:    "VolumeSnapshotClass",
	}
	CSIDriverGVK = schema.GroupVersionKind{
		Group:   "storage.k8s.io",
		Version: "v1",
		Kind:    "CSIDriver",
	}
)

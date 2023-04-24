<h1>API References</h1>

Pre-requisites: Install OADP to your cluster. before proceeding to the next steps.
```
❯ ls  bundle/manifests/*oadp.openshift.io* bundle/manifests/velero.io* | xargs -I {} sh -c 'export FILE={} && yq .metadata.name $FILE && echo kind: $(yq .spec.names.kind $FILE) shortName: $(yq .spec.names.shortNames $FILE )'
volumesnapshotbackups.datamover.oadp.openshift.io
kind: VolumeSnapshotBackup shortName: - vsb
volumesnapshotrestores.datamover.oadp.openshift.io
kind: VolumeSnapshotRestore shortName: - vsr
cloudstorages.oadp.openshift.io
kind: CloudStorage shortName: null
dataprotectionapplications.oadp.openshift.io
kind: DataProtectionApplication shortName: - dpa
backuprepositories.velero.io
kind: BackupRepository shortName: null
backups.velero.io
kind: Backup shortName: null
backupstoragelocations.velero.io
kind: BackupStorageLocation shortName: - bsl
deletebackuprequests.velero.io
kind: DeleteBackupRequest shortName: null
downloadrequests.velero.io
kind: DownloadRequest shortName: null
podvolumebackups.velero.io
kind: PodVolumeBackup shortName: null
podvolumerestores.velero.io
kind: PodVolumeRestore shortName: null
restores.velero.io
kind: Restore shortName: null
schedules.velero.io
kind: Schedule shortName: null
serverstatusrequests.velero.io
kind: ServerStatusRequest shortName: - ssr
volumesnapshotlocations.velero.io
kind: VolumeSnapshotLocation shortName: - vsl
```

You can use `oc explain <full-name|kind|short-name>.<fields>` to explore available APIs

eg.
```
❯ oc explain dpa.spec.features
KIND:     DataProtectionApplication
VERSION:  oadp.openshift.io/v1alpha1

RESOURCE: features <Object>

DESCRIPTION:
     features defines the configuration for the DPA to enable the OADP tech
     preview features

FIELDS:
   dataMover	<Object>
     Contains data mover specific configurations
```

See also [![Go Reference](https://pkg.go.dev/badge/github.com/openshift/oadp-operator.svg)](https://pkg.go.dev/github.com/openshift/oadp-operator@master) for a deeper dive.

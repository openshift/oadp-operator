<h1>API References</h1>

Pre-requisites: Install OADP to your cluster. before proceeding to the next steps.

Run `oc api-resources | grep -e 'oadp\|velero'` to get available APIs

Example output (subject to change, depending on the version of OADP installed):
```
veleroinstalls                                            managed.openshift.io/v1alpha2                   true         VeleroInstall
cloudstorages                                             oadp.openshift.io/v1alpha1                      true         CloudStorage
dataprotectionapplications            dpa                 oadp.openshift.io/v1alpha1                      true         DataProtectionApplication
backuprepositories                                        velero.io/v1                                    true         BackupRepository
backups                                                   velero.io/v1                                    true         Backup
backupstoragelocations                bsl                 velero.io/v1                                    true         BackupStorageLocation
deletebackuprequests                                      velero.io/v1                                    true         DeleteBackupRequest
downloadrequests                                          velero.io/v1                                    true         DownloadRequest
podvolumebackups                                          velero.io/v1                                    true         PodVolumeBackup
podvolumerestores                                         velero.io/v1                                    true         PodVolumeRestore
resticrepositories                                        velero.io/v1                                    true         ResticRepository
restores                                                  velero.io/v1                                    true         Restore
schedules                                                 velero.io/v1                                    true         Schedule
serverstatusrequests                  ssr                 velero.io/v1                                    true         ServerStatusRequest
volumesnapshotlocations                                   velero.io/v1                                    true         VolumeSnapshotLocation
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

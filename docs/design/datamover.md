---
title: datamover_crd_design
authors:
  - "@savitharaghunathan"
reviewers:
  - "@shawn_hurley"
  - "@alaypatel07"
  - "@dymurray"
  - "@eemcmullan"
approvers:
  - "@shawn_hurley"
  - "@alaypatel07"
creation-date: 2022-03-16
status: implementable
---

# Data Mover CRD design

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] User-facing documentation is created

## Open questions

* PVC/VolumeSnapshot mover - Should the Datamover Backup process be triggered off a PVC or a snapshot? 
    * Should we support both types and provide user an option to pick either PVC or snapshot?


## Summary
OADP operator currently supports backup and restore of applications backed by CSI volumes by leveraging the Velero CSI plugin. The problem with CSI snapshots on some providers such as ODF is that these snapshots are local to the Openshift cluster and cannot be recovered if the cluster gets deleted accidentally or if there is a disaster. In order to overcome this issue, DataMover is made available for users to save the snapshots in a remote storage. 

## Motivation

Create an extensible design to support various data movers that can be integrated with OADP operator. Vendors should be able to bring their own data mover controller and implementation, and use that with OADP operator.

## Goals
* Create an extensible data mover solution
* Supply a default data mover option 
* Supply APIs for DataMover CRs (eg: VolumeSnapshotBackup, VolumeSnapshotRestore)
* Supply a sample codebase for the Data Mover plugin and controller implementation


## Non Goals
* Maintain 3rd party data mover implementations
* Adding a status watch controller to Velero

## User stories

Story 1: 
As an application developer, I would like to save the CSI snaphots in a S3 bucket. 

Story 2:
As a cluster admin, I would like to be able to restore CSI snapshots if disaster happens.

## Design & Implementation details

This design supports adding the data mover feature to the OADP operator and facilitates integrating various vendor implemented data movers. 

![DataMover CRD](../images/datamovercrd.png)

Note: We will be supporting VolSync as the default data mover. 

The VolumeSnapshotBackup Controller will watch for VolumeSnapshotBackup CR. Likewise, VolumeSnapshotRestore Controller will watch for VolumeSnapshotRestore CR. 

### Volume Snapshot Backup

Assuming that the `DataMover Enable (spec: feature.dataMover.enable)` flag is set to true in the DPA config, when a velero backup is created, it triggers the custom velero CSI plugin (velero BackupItemAction plugin) to create the `VolumeSnapshotBackup` CR in the app namespace. The extended plugin looks up for the PVCs in the user namespace mentioned in the velero backup and creates a `VolumeSnapshotBackup` CR for every `VolumeSnapshotContent` encountered during the backup process.

`VolumeSnapshotBackup` CR supports a `volumesnapshotcontent` as the type of the backup object. Velero backup will wait for the `VolumeSnapshotBackup` to be complete. Once all the `VolumeSnapshotBackup` gets completed, Velero backup's status get updated accordingly.

```
apiVersion: datamover.oadp.openshift.io/v1alpha1
kind: VolumeSnapshotBackup
metadata:
  annotations:
    datamover.io/restic-repository: <restic_repo>
    datamover.io/source-pvc-name: <src_pvc_name>
    datamover.io/source-pvc-size: <src_pvc_size>
    datamover.io/source-pvc-storageclass: <src_pvc_sc>
    datamover.io/source-pvc-volumesnapshotclass: <src_vsc_class>
  labels:
    velero.io/backup-name: <backup_name>
  name: <vsb_name>
  namespace: <ns>
spec:
  protectedNamespace: <oadp_ns>
  resticSecretRef:
    name: <bsl_restic_secret>
  volumeSnapshotContent:
    name: <vsc_name>
status:
  phase: <vsb_status>
  resticrepository: <restic_repo>
  sourcePVCData:
    name: <src_pvc_name>
    size: <src_pvc_size>
    storageClassName: <sc_name>
  volumeSnapshotClassName: <volumesnapshotclass_name>
```
### Volume Snapshot Restore
When a velero restore is triggered, the custom Velero CSI plugin looks for `VolumeSnapshotBackup` CR in the backup resources. If it encounters a `VolumeSnapshotBackup` resource, then the extended plugin (velero RestoreItemAction plugin) will create a `VolumeSnapshotRestore` CR in the app namespace. It will populate the CR with the details obtained from the `VolumeSnapshotBackup` resource. 

The velero CSI plugin waits for  `VolumeSnapshotRestore (VSR)` to complete, which is after the VSR controller waits for VolSync's `ReplicationDestination` CR to complete, before proceeding with the Velero restore process.

```
apiVersion: datamover.oadp.openshift.io/v1alpha1
kind: VolumeSnapshotRestore
metadata:
  labels:
    velero.io/restore-name: <restore_name>
  name: <name>
  namespace: <namespace>
spec:
  protectedNamespace: <OADP namespace>
  resticSecretRef:
    name: <restic_secret_ref>
  volumeSnapshotMoverBackupRef:
    resticrepository: <restic_repo>
    sourcePVCData:
      name: <src_pvc_name>
      size: <src_pvc_size>
status:
  phase: <vsr_phase>
  snapshotHandle: <vsc_snaphandle>
```

We will provide a sample codebase which the vendors will be able to extend and implement their own data movers. 


### Default OADP Data Mover controller

VolSync will be used as the default Data Mover for OADP and `restic` will be the supported method for backup & restore of PVCs. VolSync will be installed through operator hub. Restic repository details are configured in a `secret` object which gets used by the VolSync's resources. This design takes advantage of VolSync's two resources - `ReplicationSource` & `ReplicationDestination`. `ReplicationSource` object helps with taking a backup of the PVCs and using restic to move it to the storage specified in the restic secret. `ReplicationDestination` object takes care of restoring the backup from the restic repository. There will be a 1:1 relationship between the replication src/dest CRs and PVCs.

The user will provide the restic password in a secret and the controller will create restic secret using BSL config. During backup time, the secret corresponding to the given bsl will be added to the VSB/VSR CR by the CSI Plugin. Using that secret as source, the controller will create on-demand secrets for every backup/restore request.
```
...
spec:
  features:
    dataMover: 
      enable: true
      credentialName: <dm-restic-secret-name>

...
```

If the DataMover flag is enabled, then the user creates a restic secret with all the following details,
```
apiVersion: v1
kind: Secret
metadata:
  name: restic-config
type: Opaque
stringData:
  # The repository encryption key
  RESTIC_PASSWORD: <password>
```
*Note: More details for installing restic secret in [here](https://volsync.readthedocs.io/en/stable/usage/restic/index.html#specifying-a-repository)*


Custom velero CSI plugin will be responsible for creating `VolumeSnapshotBackup` & `VolumeSnapshotRestore` CRs. 

Once a VolumeSnapshotBackup CR gets created, the controller will create the corresponding `ReplicationSource` CR in the protected namespace. VolSync watches for the creation of `ReplicationSource` CR and copies the PVC data to the restic repository mentioned in the `restic-config`.  
```
apiVersion: volsync.backube/v1alpha1
kind: ReplicationSource
metadata:
  name: database-source
  namespace: <protected_ns>
spec:
  sourcePVC: <pvc_name>
  trigger:
    manual: <trigger_name>
  restic:
    pruneIntervalDays: 15
    repository: <restic-config>
    retain:
      hourly: 1
      daily: 1
      weekly: 1
      monthly: 1
      yearly: 1
    copyMethod: None
```

Similarly, when a VolumeSnapshotRestore CR gets created, controller will create a `ReplicationDestination` CR in the protected namespace. VolSync controller copies the PVC data from the restic repository to the protected namespace. The `snaphandle` from the volumesnapshotcontent created by VolSync controller will be passed back to Velero CSI plugin. The plugin passes this `snaphandle` to the restore process and continues with the CSI restore operation. 

```
apiVersion: volsync.backube/v1alpha1
kind: ReplicationDestination
metadata:
  name: <protected_namespace>
spec:
  trigger:
    manual: <trigger_name>
  restic:
    destinationPVC: <pvc_name>
    repository: restic-config
    copyMethod: None
```

Data mover controller will clean up all controller-created resources after the process is complete.

## Alternate Design ideas

### Support for multiple data mover plugins
`DataMoverClass` spec will support the following field,
    `selector: <tagname>`
PVC must be labelled with the `<tagname>`, to be moved by the specific `DataMoverClass`. User/Admin of the cluster must label the PVCs with the required `<tagname>` and map it to a `DataMoverClass`. If the PVCs are not labelled, it will be moved by the default datamover.

#### Alternate options
PVCs can be annotated with the `DataMoverClass`, and when a backup is created, the controller will look at the DataMoverClass and add it to the `VolumeSnapshotBackup` CR. 


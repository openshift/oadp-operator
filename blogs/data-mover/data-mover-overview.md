Data Mover (OADP 1.2 or below)

# A Technical Overview of VolumeSnapshotMover

## Table of Contents
- [Introduction](#introduction)
- [What is CSI](#what-is-csi)
- [Why We Need VolumeSnapshotMover](#why-we-need-volumesnapshotmover)
- [Components](#components)
  - [VolSync](#volsync)
  - [Velero](#velero)
  - [VolumeSnapshotMover Controller](#volumesnapshotmover-controller)
  - [VolumeSnapshotMover CustomResourceDefinitions](#volumesnapshotmover-customresourcedefinitions)
- [Backup Process](#backup-process)
- [Restore Process](#restore-process)
- [VolumeSnapshotMover's Future](#volumesnapshotmovers-future)


## Introduction

VolumeSnapshotMover provides portability and durability of CSI volume snapshots 
by relocating snapshots into an object storage location during backup of a 
stateful application. These snapshots are then available for restore during 
instances of disaster scenarios. This blog will discuss the different 
VolumeSnapshotMover components and how they work together to complete this 
process.


## What Is CSI?

One of the more important components of VolumeSnapshotMover to understand is CSI, 
or Container Storage Interface. CSI provides a layer of abstraction between container 
orchestration tools and storage systems such that users do not need to be 
informed on the differences between storage provider's needs and requirements.
It also provides point-in-time snapshotting of volumes.

CSI volumes are now the industry standard and are the storage backing for most 
Cloud Native applications. 
However, issues concerning CSI volumes still remain. Some volumes have 
vendor-specific requirements, and can prevent proper portability and durability. 
VolumeSnapshotMover works to solve this case, which will be
discussed more in the next section.

You can read more about CSI [here](https://kubernetes-csi.github.io/docs/). 


## Why We Need VolumeSnapshotMover

During a backup using Velero with CSI, CSI snapshotting is performed. This 
snapshot is created on the storage provider where the snapshot was taken. 
This means that for some providers, such as ODF, the snapshot lives on the 
cluster. Due to this poor durability, in the case of a disaster scenario, the 
snapshot is also subjected to disaster.  

With volumeSnapshotMover, snapshots are relocated off of the cluster to the 
targeted backupStorageLocation (generally object storage), providing additional safety. 


## Components

### [OADP Operator](https://github.com/openshift/oadp-operator): 
OADP is the OpenShift API for Data Protection operator. This open source operator sets up and installs Velero on the OpenShift platform, allowing users to backup and restore applications. 
We will be installing Velero alongside the CSI plugin (modified version).

### [Modified CSI plugin (M-CSI)](https://github.com/openshift/velero-plugin-for-csi/tree/data-mover):  
The upstream Velero plugin for CSI is modified to facilitate CSI volumesnapshot data movement from an OpenShift cluster to object storage and vice versa.

### [VolSync](https://volsync.readthedocs.io/en/stable/):
VolSync is a Kubernetes operator that performs asynchronous replication of persistent volumes within, or across, clusters. The replication provided by VolSync is independent of the storage system. This allows replication to and from storage types that don’t normally support remote replication. 
We will be using Volsync’s restic datamover.

### [VolumeSnapshotMover (VSM) Controller](https://github.com/migtools/volume-snapshot-mover):
The VSM controller is the CSI data movement orchestrator, it is deployed via the OADP Operator once the datamover feature is enabled. This controller has the following responsibilities:
- Validates the VolumeSnapshotBackup/VolumeSnapshotRestore Custom Resources.
- Makes sure that the data movement workflow has the appropriate storage credentials
- Performs the copy of VolumeSnapshotContent, CSI VolumeSnapshot and PersistentVolumeClaims from application namespace to OADP Operator namespace
- Triggers the data movement process and subsequently performs the cleanup of extraneous resources created.

### [VolumeSnapshotMover CustomResourceDefinitions (CRDs)](https://github.com/migtools/volume-snapshot-mover/tree/master/config/crd/bases):
The data mover process will be based on two Custom Resource Definitions:
- VolumeSnapshotBackup (VSB):
```
Spec:
  Volumesnapshotcontent:
  ProtectedNamespace:
  ResticSecretRef:
Status:
  Completed:
  SourcePVCData:
  Conditions:
  ResticRepository:
  Phase:
  VolumeSnapshotClassName:  
```
- VolumeSnapshotRestore (VSR):
```
Spec:
  ResticSecretRef:
  VolumeSnapshotMoverBackupRef:
  ProtectedNamespace:
Status:
  Conditions:
  Phase:
  Snapshothandle:
```

## Backup Process

- The M-CSI plugin is extended to facilitate the data movement of CSI VolumeSnapshots(VS) from cluster to object storage.
- When the Velero Backup is triggered, the M-CSI plugin creates a VS for each PersistentVolumeClaim (PVC) to be backed up.
- Now for the created VS, the M-CSI plugin fetches the associated VolumeSnapshotContent (VSC) and adds it as an additional item to be backed up.
- Subsequently, the M-CSI plugin then checks whether there is a VolumeSnapshotBackup (VSB) instance associated with the VSC that was added as an additional item, if there isn't one then the M-CSI plugin creates a VSB for each VSC.
- The creation of a VSB triggers the data movement process as the VolumeSnapshotMover (VSM) controller begins to reconcile on this VSB instance.
- VSM first validates the VSB, then copies the VSC, followed by VS and finally the PVC into the namespace where OADP Operator resides. Once this is done the VSM controller uses the PVC as a datasource and creates a Volsync ReplicationSource CR.
- Volsync reconciles on ReplicationSource CR and then Volsync’s restic mover begins the transfer of data from cluster to the target object storage.
- Since the time when VSB is created and data movement is started, Velero backup waits for Volsync to complete the data movement, once that's done VSB is marked complete and consequently the backup is marked complete by Velero.
- One point to note is that, VSM controller deletes all the extraneous resources that were created during the data mover backup process.


![VSMBackup](data-mover-backup.png)



## Restore Process

- During restore, the M-CSI plugin is extended to support volumeSnapshotMover 
functionality. As mentioned previously, during backup, a VSB custom 
resource is stored as a backup object. This CR contains details pertinent to 
performing a volumeSnapshotMover restore. 

- Once a VSB CR is encountered, a VSR CR is created by the M-CSI plugin. The VSM controller 
then begins to reconcile on the VSR CR. Here, a VolSync ReplicationDestination is created by the VSM controller in the 
OADP Operator namespace. This CR will recover the VolumeSnapshot that was 
stored in the object storage location during backup. 

- After the VolSync restore step completes, the Velero restore continues as usual. 
However, the M-CSI plugin uses the VolSync VolumeSnapshot's `snapHandle` 
as the data source for its associated PVC.  

- The stateful application data is then restored, and disaster is averted.


![VSMRestore](data-mover-restore.png)

## VolumeSnapshotMover's Future

In the near future, we plan to improve the performance of VolumeSnapshotMover. 
A new Velero ItemAction plugin will be introduced to allow for asynchronous 
operations during backup and restore. This will vastly improve the performance of 
VolumeSnapshot data movement.

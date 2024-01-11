<h1 align="center"> E2E Backup/Restore with CSI Flowchart - Mongo</h1>

Steps below explain E2E backup/restore Test suite of App [mongo-persistent](/tests/e2e/sample-applications/mongo-persistent/mongo-persistent-csi.yaml) using CSI in AWS cluster.


## 1. Pre-Backup:


### 1.1 Create VolumeSnapshotClass with Driver which is similar to Provider of available csi-storage class.

<img width="1614" alt="Screen Shot 2022-08-02 at 5 36 48 PM" src="https://user-images.githubusercontent.com/83228833/182479725-228d4d39-6b04-4128-a8ba-ec409e3bfeb3.png">

Note: Storage Class’s Provider and VolumeSnapshotClass’s Driver should match.

VolumeSnapshotClass:
```
apiVersion: snapshot.storage.k8s.io/v1
deletionPolicy: Retain
driver: ebs.csi.aws.com
kind: VolumeSnapshotClass
metadata:
 annotations:
   snapshot.storage.kubernetes.io/is-default-class: 'true'
 labels:
   velero.io/csi-volumesnapshot-class: 'true'
 name: oadp-example-snapclass
```

### 1.2 Installing Application and checking if application is ready for the backup by Running preBackup Step

<img width="1366" alt="Screen Shot 2022-08-02 at 6 11 10 PM" src="https://user-images.githubusercontent.com/83228833/182482216-ed0d9d51-0d5c-4c6d-8c37-5fcdefd1d5c9.png">


## 2. Backup:

### 2.1 For Backup, create volumeSnapshot CR that points to mongo-pvc.

Path to volume after creating volumeSnapshot CR:

	VolumeSnapshot → mongo-pvc → pv → volume

volumeSnapshot is created with **readyToUse: nil**

<img width="1419" alt="Screen Shot 2022-08-02 at 6 12 35 PM" src="https://user-images.githubusercontent.com/83228833/182482356-6b1319c5-5178-41c5-b8c6-f148f33fd636.png">

As described in the image, source of the volumeSnapshot is mongo-pvc. Here is the example of VolumeSnapshot:
```
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
 generateName: velero-mongo-
 name: velero-mongo-6sg7g
 namespace: mongo-persistent
spec:
 source:
   persistentVolumeClaimName: mongo-pvc
 volumeSnapshotClassName: oadp-example
status:
 boundVolumeSnapshotContentName: snapcontent
 creationTime: '2022-07-13T15:28:28Z'
 readyToUse: nil
 restoreSize: 1Gi

```

### 2.2 **CSI snapshotter** makes gRPC call to **provider’s Driver** to create snapShot, and update volumeSnapshot’s readyToUse nil to false

<img width="1120" alt="Screen Shot 2022-08-02 at 6 14 17 PM" src="https://user-images.githubusercontent.com/83228833/182482626-33d1be48-b86a-4b75-ba38-0de631d0dfdd.png">


### 2.3 CSI-snapshotter create volumeSnapshotContent with snapshotHandle returned by Driver

<img width="1096" alt="Screen Shot 2022-08-02 at 6 16 40 PM" src="https://user-images.githubusercontent.com/83228833/182482958-5fd8d50c-1961-4c33-b76e-abc7e7cbfcdd.png">

Source of volumeSnapshotContent is volumeHandle, a unique string that identifies volume. Similarly, snapshotHandle is the unique identifier of the volume snapshot created on the storage backend. Here is the example of volumeSnapshotContent:
```
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotContent
metadata:
 annotations:
   snapshot.storage.kubernetes.io/volumesnapshot-being-deleted: 'yes'
 name: snapcontent
spec:
 deletionPolicy: Retain
 driver: ebs.csi.aws.com
 source:
   volumeHandle: vol-081e4301172d6fa52
 volumeSnapshotClassName: oadp-example-snapclass
 volumeSnapshotRef:
   apiVersion: snapshot.storage.k8s.io/v1
   kind: VolumeSnapshot
   name: velero-mongo-6sg7g
   namespace: mongo-persistent
status:
 readyToUse: true
 restoreSize: 1073741824
 snapshotHandle: snap-083de977db057e37c

```
### 2.4 Update volumeSnapshot and volumeSnapshotContent **readyToUse: true**

<img width="1095" alt="Screen Shot 2022-08-02 at 6 18 04 PM" src="https://user-images.githubusercontent.com/83228833/182483084-1262419f-9a04-40e4-9ecb-70506634db65.png">


### 2.5 End of Backup

<img width="1545" alt="Screen Shot 2022-08-02 at 6 19 14 PM" src="https://user-images.githubusercontent.com/83228833/182483254-eeba12d9-3c12-47e2-8216-ae2d8cd2a58c.png">

## 3. Uninstall the application

<img width="1348" alt="Screen Shot 2022-08-02 at 7 06 52 PM" src="https://user-images.githubusercontent.com/83228833/182489471-55d1933c-0f8a-4972-b84d-02148a7912e0.png">

Delete VolumeSnapshot created during the backup. Otherwise deleting namespace in the cluster during uninstallation will trigger the VolumeSnapshot deletion, which will cause snapshot deletion on the cloud providers, then backup cannot restore the PV.

## 4. Restore:


### 4.1  Backup resources from S3 bucket

resources such as namespace, services, routes, secrets e.t.c.


### 4.2 Create PVC

Velero creates PVC with DataSource: VolumeSnapshot that leads to snapshot. (PVC, VS, and, VSC configuration files are available at the end)

	VolumeSnapshotContent→VolumeSnapshot→ PVC’s dataSource → PV → Volume

<img width="1454" alt="Screen Shot 2022-08-02 at 6 31 07 PM" src="https://user-images.githubusercontent.com/83228833/182484702-589b5a45-8b73-4335-86cb-366ad0562f43.png">


Note: Backup will re-create `VolumeSnapshotContent,`because some parameter in the `VolumeSnapshotContent` Spec is immutable, e.g. `VolumeSnapshotRef` and `Source`.


### 4.3 Create PV, CSI-provisioner observes PVC and creates PV as per claim

<img width="1155" alt="Screen Shot 2022-08-02 at 6 32 44 PM" src="https://user-images.githubusercontent.com/83228833/182484879-990f53d5-7829-4290-bd78-d2ce4acfc78a.png">


### 4.4 Create volume, CSI-provisioner asks provider’s driver to create volume and return volume-ID

<img width="1167" alt="Screen Shot 2022-08-02 at 6 38 20 PM" src="https://user-images.githubusercontent.com/83228833/182485506-611a5d62-7123-434f-9e17-7a7a343c5582.png">

### 4.5 PV→ Volume, CSI-provisioner provides volume-ID to PV and bounds PV to volume

<img width="1155" alt="Screen Shot 2022-08-02 at 6 38 42 PM" src="https://user-images.githubusercontent.com/83228833/182485615-73c8cbe5-421e-4242-a5f2-83a8d45943a5.png">


### 4.6 End of the restore

<img width="1568" alt="Screen Shot 2022-08-02 at 6 39 07 PM" src="https://user-images.githubusercontent.com/83228833/182485630-8b92670d-28e0-4fe8-bd2d-9260e4277304.png">






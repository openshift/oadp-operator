# Velero Restic Troubleshooting Tips
This document contains commands for both Velero Restic Filesystem copy and for OADP's datamover feature.

## Additional information can be found in the restic documentation

https://restic.readthedocs.io/en/latest/077_troubleshooting.html

## setup cli clients
```
alias velero='oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero'
alias restic='oc -n openshift-adp exec deployment/velero -c velero -it -- /usr/bin/restic'
```

## restic repository info
```
sh-4.4$ ./velero repo get
NAME                                         STATUS   LAST MAINTENANCE
mysql-persistent-dpa-sample-1-restic-bb9mz   Ready    2023-06-19 19:35:40 +0000 UTC
```
```
sh-4.4$ ./velero repo get mysql-persistent-dpa-sample-1-restic-bb9mz -o yaml
apiVersion: velero.io/v1
kind: BackupRepository
metadata:
  creationTimestamp: "2023-06-19T19:35:38Z"
  generateName: mysql-persistent-dpa-sample-1-restic-
  generation: 3
  labels:
    velero.io/repository-type: restic
    velero.io/storage-location: dpa-sample-1
    velero.io/volume-namespace: mysql-persistent
  managedFields:
  - apiVersion: velero.io/v1
    fieldsType: FieldsV1
    fieldsV1:
      f:metadata:
        f:generateName: {}
        f:labels:
          .: {}
          f:velero.io/repository-type: {}
          f:velero.io/storage-location: {}
          f:velero.io/volume-namespace: {}
      f:spec:
        .: {}
        f:backupStorageLocation: {}
        f:maintenanceFrequency: {}
        f:repositoryType: {}
        f:resticIdentifier: {}
        f:volumeNamespace: {}
      f:status:
        .: {}
        f:lastMaintenanceTime: {}
        f:phase: {}
    manager: velero-server
    operation: Update
    time: "2023-06-19T19:35:40Z"
  name: mysql-persistent-dpa-sample-1-restic-bb9mz
  namespace: openshift-adp
  resourceVersion: "27163692"
  uid: ce3dcf98-c2b0-441b-92c4-677c2ead6012
spec:
  backupStorageLocation: dpa-sample-1
  maintenanceFrequency: 168h0m0s
  repositoryType: restic
  resticIdentifier: s3:<REPOSITORY-URL>/<BUCKET>/velero/restic/mysql-persistent
  volumeNamespace: mysql-persistent
status:
  lastMaintenanceTime: "2023-06-19T19:35:40Z"
  phase: Ready
```

## restic repo password 
```
[whayutin@thinkdoe SETUP]$ oc get  secret  velero-repo-credentials -n openshift-adp 
NAME                      TYPE     DATA   AGE
velero-repo-credentials   Opaque   1      5d23h
[whayutin@thinkdoe SETUP]$ oc get  secret  velero-repo-credentials -n openshift-adp -o yaml
apiVersion: v1
data:
  repository-password: c3RhdGljLXBhc3N3MHJk
kind: Secret
metadata:
  creationTimestamp: "2023-06-14T17:43:37Z"
  name: velero-repo-credentials
  namespace: openshift-adp
  resourceVersion: "22449264"
  uid: b75d5f8c-9263-445e-b1a3-167a95c07cdf
type: Opaque

echo "c3RhdGljLXBhc3N3MHJk" | base64 -d
static-passw0rd
```
Alternatively:
```
oc get  secret  velero-repo-credentials -n openshift-adp -o jsonpath='{.data.repository-password}'|base64 -d
```

## restic commands:

#### stats
``` 
restic stats  --cache-dir /tmp/.cache   -r s3:<REPOSITORY-URL>/<BUCKET>/velero/restic/mysql-persistent
enter password for repository: 
repository 2464cd5d opened (version 2, compression level auto)
scanning...
Stats in restore-size mode:
     Snapshots processed:  2
        Total File Count:  108
              Total Size:  102.652 MiB
```

#### list locks
```
restic  --cache-dir /tmp/.cache -r s3:<REPOSITORY-URL>/<BUCKET>/velero/restic/mysql-persistent list locks
enter password for repository: 
repository 2464cd5d opened (version 2, compression level auto)

```

#### unlock
```
sh-4.4$ restic  --cache-dir /tmp/.cache   -r s3:<REPOSITORY-URL>/<BUCKET>/velero/restic/mysql-persistent unlock    
enter password for repository: 
repository 2464cd5d opened (version 2, compression level auto)
sh-4.4$ 
```

#### list blobs
```
sh-4.4$ restic  --cache-dir /tmp/.cache   -r s3:<REPOSITORY-URL>/<BUCKET>/velero/restic/mysql-persistent list blobs
enter password for repository: 
repository 2464cd5d opened (version 2, compression level auto)
data c2017654d859475a2ee546d693a2bb12886eec94edb5cac737ea573f3ef8d0ae
tree 159821e90934b136b8c7c355eec08074a66ba7d7db20b9cfe6c98c8c9253dd3f
data a76bde70b2db6e17474b375c1746f0f75a7e4d62f48754780f0fd1c39ac4f0b5
data 7cbf59062c5944d940b95609497d215a1e606bb48551fa488bd91e2aeb9355eb
data 369b06f024be1a9f192efaeff32612bd0f89d280743fcb0df60216fbd097f943
```

#### prune
```
restic  --cache-dir /tmp/.cache   -r s3:<REPOSITORY-URL>/<BUCKET>/velero/restic/mysql-persistent prune  
enter password for repository: 
repository 2464cd5d opened (version 2, compression level auto)
loading indexes...
loading all snapshots...
finding data that is still in use for 2 snapshots
[0:00] 100.00%  2 / 2 snapshots
searching used packs...
collecting packs for deletion and repacking

to repack:             0 blobs / 0 B
this removes:          0 blobs / 0 B
to delete:             0 blobs / 0 B
total prune:           0 blobs / 0 B
remaining:           100 blobs / 405.382 KiB
unused size after prune: 0 B (0.00% of remaining size)

done
```

#### retain policy - keep
```
restic  --cache-dir /tmp/.cache   -r s3:<REPOSITORY-URL>/<BUCKET>/velero/restic/mysql-persistent forget --keep-last 1 --prune
enter password for repository: 
repository 2464cd5d opened (version 2, compression level auto)
Applying Policy: keep 1 latest snapshots
keep 1 snapshots:
ID        Time                 Host        Tags                                                                                                                                                                                                          Reasons        Paths
------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
40b42d78  2023-06-19 19:35:46  velero      pvc-uid=28a884c1-6df4-411d-90ce-b800338a10f8,volume=applog,backup=hay1,backup-uid=ad20f725-6f83-4290-ae79-fe6d0b85cd9c,ns=mysql-persistent,pod=todolist-1-74w69,pod-uid=67369e22-57d8-434a-9de7-47446121ade0  last snapshot  /host_pods/67369e22-57d8-434a-9de7-47446121ade0/volumes/kubernetes.io~csi/pvc-28a884c1-6df4-411d-90ce-b800338a10f8/mount
------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
1 snapshots

keep 1 snapshots:
ID        Time                 Host        Tags                                                                                                                                                                                                                    Reasons        Paths
----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
af9a6651  2023-06-19 19:35:43  velero      pod=mysql-7bc95589b4-zr7c4,pod-uid=87dda243-6e5e-4030-a1e1-60cc394677e8,pvc-uid=966cca8f-9648-40ab-812e-a711500acf57,volume=mysql-data,backup=hay1,backup-uid=ad20f725-6f83-4290-ae79-fe6d0b85cd9c,ns=mysql-persistent  last snapshot  /host_pods/87dda243-6e5e-4030-a1e1-60cc394677e8/volumes/kubernetes.io~csi/pvc-966cca8f-9648-40ab-812e-a711500acf57/mount
----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
1 snapshots
```

#### In case of restic backup failure 

* check PodVolumeBackup/PodVolumeRestore CR status. Check is there any useful information in restic daemonSet pod.
```
oc -n openshift-adp get podvolumebackups -l velero.io/backup-name=<backup-name> 
oc -n openshift-adp get podvolumerestore -l velero.io/restore-name=<restore-name>
oc logs -n openshift-adp <restic-pod>
```

## Data Mover (OADP 1.2 or below) + Restic

#### get replicationsource info
```
oc get replicationsource -A
NAMESPACE       NAME                SOURCE                                                 LAST SYNC              DURATION        NEXT SYNC
openshift-adp   vsb-7rkn6-rep-src   snapcontent-993aabe2-8170-4661-984e-00a560f486cd-pvc   2023-06-20T20:16:55Z   33.274853286s   
openshift-adp   vsb-vpqzd-rep-src   snapcontent-a751884d-b148-4a7d-9f5d-90da7a522be7-pvc   2023-06-20T20:17:51Z   24.452515994s   
```

```
oc get replicationsource vsb-7rkn6-rep-src -n openshift-adp -o yaml
apiVersion: volsync.backube/v1alpha1
kind: ReplicationSource
metadata:
  creationTimestamp: "2023-06-20T20:16:22Z"
  generation: 1
  labels:
    datamover.oadp.openshift.io/vsb: vsb-7rkn6
  name: vsb-7rkn6-rep-src
  namespace: openshift-adp
  resourceVersion: "28136883"
  uid: 1b6b4f33-41b2-4159-a396-545208742208
spec:
  restic:
    accessModes:
    - ReadWriteOnce
    copyMethod: Direct
    customCA: {}
    moverServiceAccount: velero
    repository: vsb-7rkn6-secret
    retain: {}
    storageClassName: gp2-csi
    volumeSnapshotClassName: csi-aws-vsc-test
  sourcePVC: snapcontent-993aabe2-8170-4661-984e-00a560f486cd-pvc
  trigger:
    manual: vsb-7rkn6-trigger
status:
  conditions:
  - lastTransitionTime: "2023-06-20T20:16:55Z"
    message: Waiting for manual trigger
    reason: WaitingForManual
    status: "False"
    type: Synchronizing
  lastManualSync: vsb-7rkn6-trigger
  lastSyncDuration: 33.274853286s
  lastSyncTime: "2023-06-20T20:16:55Z"
  latestMoverStatus:
    logs: |-
      no parent snapshot found, will read all files
      Added to the repository: 8.102 MiB (408.500 KiB stored)
      processed 101 files, 102.651 MiB in 0:00
      snapshot dcec01b1 saved
      Restic completed in 4s
    result: Successful
  restic: {}
```

#### get restic repo information for data mover
```
oc get secret dpa-sample-1-volsync-restic -n openshift-adp -o yaml
apiVersion: v1
data:
  AWS_ACCESS_KEY_ID: QUtJQVZCUsnip
  AWS_DEFAULT_REGION: dXMtdsnip
  AWS_SECRET_ACCESS_KEY: ZGZQsnip
  RESTIC_PASSWORD: cmVzdGljcGFzc3dvcmQ=
  RESTIC_REPOSITORY: czM6czMuYW1hem9uYXdzLmNvbS9jdnBidWNrZXR1c3dlc3Qy
  restic-prune-interval: MQ==
kind: Secret
metadata:
  creationTimestamp: "2023-06-14T17:53:41Z"
  labels:
    openshift.io/oadp: "True"
    openshift.io/oadp-bsl-name: dpa-sample-1
    openshift.io/oadp-bsl-provider: aws
  name: dpa-sample-1-volsync-restic
  namespace: openshift-adp
  ownerReferences:
  - apiVersion: oadp.openshift.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: DataProtectionApplication
    name: dpa-sample
    uid: 66568a80-778a-4478-bca1-d8ff7720b129
  resourceVersion: "28139203"
  uid: 192bc903-e754-4cd3-9173-2af805c2b0d0
type: Opaque
```

#### decode the restic passwd 
```
cho "cmVzdGljcGFzc3dvcmQ=" | base64 -d
resticpassword
```

#### datamover restic path 

The path in 1.2.0 is
`$bucket/openshift-adp/$snapcontent_name`

The snapcontent_name = sourcePVC

```
spec:
  restic:
    accessModes:
    - ReadWriteOnce
    copyMethod: Direct
    customCA: {}
    moverServiceAccount: velero
    pruneIntervalDays: 1
    repository: vsb-zg6gg-secret
    retain: {}
    storageClassName: gp2-csi
    volumeSnapshotClassName: csi-aws-vsc-test
  sourcePVC: snapcontent-2044fb64-253d-461b-93f3-1ce8d6b67ebe-pvc
```

#### list snapshots for DataMover restic snapshot

```
restic  --cache-dir /tmp/.cache -r s3:<REPOSITORY-URL>/<BUCKET>/openshift-adp/snapcontent-993aabe2-8170-4661-984e-00a560f486cd-pvc snapshots
enter password for repository: 
repository 85c55159 opened (version 2, compression level auto)
created new cache in /tmp/.cache
ID        Time                 Host        Tags        Paths
------------------------------------------------------------
dcec01b1  2023-06-20 20:16:46  volsync                 /data
------------------------------------------------------------
1 snapshots
```

##  Update DPA for retain policy - restic forget
```
  features:
    dataMover:
      credentialName: restic-secret
      enable: true
      pruneInterval: "1"
      snapshotRetainPolicy:
        hourly: "1"
```

## Run a new backup and check replicationsource

```
oc get replicationsource vsb-zg6gg-rep-src -n openshift-adp -o yaml
apiVersion: volsync.backube/v1alpha1
kind: ReplicationSource
metadata:
  creationTimestamp: "2023-06-20T21:04:28Z"
  generation: 1
  labels:
    datamover.oadp.openshift.io/vsb: vsb-zg6gg
  name: vsb-zg6gg-rep-src
  namespace: openshift-adp
  resourceVersion: "28168858"
  uid: 53dc160a-d0c1-416a-95fb-77f316e8e0c1
spec:
  restic:
    accessModes:
    - ReadWriteOnce
    copyMethod: Direct
    customCA: {}
    moverServiceAccount: velero
    pruneIntervalDays: 1
    repository: vsb-zg6gg-secret
```

#### get snapshots
```
restic  --cache-dir /tmp/.cache -r s3:<REPOSITORY-URL>/<BUCKET>/openshift-adp/snapcontent-2044fb64-253d-461b-93f3-1ce8d6b67ebe-pvc snapshots
enter password for repository: 
repository 83b7f53a opened (version 2, compression level auto)
ID        Time                 Host        Tags        Paths
------------------------------------------------------------
ab60e48b  2023-06-20 21:04:41  volsync                 /data
------------------------------------------------------------
1 snapshots
```

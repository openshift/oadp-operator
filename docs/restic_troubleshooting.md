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

## Maintenance

* Upstream Documentation:
  * Velero - https://velero.io/docs/v1.15/repository-maintenance/
  * Restic - https://restic.readthedocs.io/en/latest/060_forget.html#

Restic Repository Maintenance in OADP:

In the namespace where OADP is installed repo-maintain-job's are executed

```shell=
pod/repo-maintain-job-1730739882527-2nbls                             0/1     Completed   0          168m
pod/repo-maintain-job-1730743482536-fl9tm                             0/1     Completed   0          108m
pod/repo-maintain-job-1730747082545-55ggx                             0/1     Completed   0          48m
pod/repo-maintain-job-1730749183178-5jqf2                             0/1     Completed   0          13m
pod/repo-maintain-job-1730749483183-mvrzw                             0/1     Completed   0          8m57s
pod/repo-maintain-job-1730749783183-8vtjh                             0/1     Completed   0          3m57s
```

* It is recommended to capture the logs from all the repo-maintain-jobs to understand the state of the repository and the maintenance tasks. Capturing the logs should be done on an ongoing basis.

* A user can check the logs of the repo-maintain-jobs for details of restic repo maintenance cleanup and the removal of artifacts in s3 storage.
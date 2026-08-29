# get this party started

1. ensure you have a default storage class selected
```
ocs-storagecluster-ceph-rbd (default)        openshift-storage.rbd.csi.ceph.com      Delete          Immediate              true                   7h50m
```

2. Create the VM's
Note.. these vm's already have the label for changeBlockTracking: true

```
oc create -f cirros-test-1.yaml , can also include test-2,3
```

3. Create the pv/pvc that will store the qocw2 snapshots

```
oc create -f pushBackupPV.yaml
```

4. Create an optional filebrowsers container to inspect files on the above pv/pvc

```
oc create -f filebrowser.yaml
```

5. Create the virtualMachineBackupTracker instance

```
oc create -f virtualMachineBackupTracker.yaml
```

6. Take a full andn incremental backup

Full: the first backup will always be a full backup
```
oc create -f virtualMachineBackupFull.yaml
```
Incremetnal: any new backup using the same tracker will be incremental
```
oc create -f virtualMachineBackupInc1.yaml
```

See my results in RESULTS



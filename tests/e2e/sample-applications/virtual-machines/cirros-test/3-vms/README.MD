# Various test cases for backing up / restoring vms using selectors

## DataVolume precreated: backup all, restore one at a time
#### setup:
```
alias velero='oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero'
cd cirros-test-with-dataVolumePV 
oc create namespace cirros-test-ds-pv
oc create -f cirros-datasource-datavolume.yaml
oc create -f cirros-test-1.yaml -f cirros-test-2.yaml -f cirros-test-3.yaml
velero backup create backup-cirros-test-pv-all --include-namespaces cirros-test-ds-pv --snapshot-move-data=true
oc delete -f cirros-test-1.yaml -f cirros-test-2.yaml -f cirros-test-3.yaml
```

#### to restore:
```
alias velero='oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero'
velero restore create cirros-test-pv-1  --from-backup backup-cirros-test-pv-all --selector app=cirros-test-pv-1
velero restore create cirros-test-pv-2  --from-backup backup-cirros-test-pv-all --selector app=cirros-test-pv-2
velero restore create cirros-test-pv-3  --from-backup backup-cirros-test-pv-all --selector app=cirros-test-pv-3
```

**The above use case should all work as expected. \0/**
See the results in [cirros-test-with-dataVolumePV/RESULTS](cirros-test-with-dataVolumePV/RESULTS)

**To restore the datasource and datavolume**
You may want to test the restore of the vm's after also deleting the datasource and datavolume.
Include the `openshift-virtualization-os-images` namespace in the original backup.

```
spec:
  defaultVolumesToFsBackup: false
  storageLocation: velero-sample-1
  includedNamespaces:
    - cirros-test-ds-pv
    - openshift-virtualization-os-images
  snapshotMoveData: true
```

* note: restoring by label without the datavolume and datasource previously restored or present will not work unless tagged appropriately.

## DataVolume not precreated, container disk: backup all, restore one at a time
#### setup:
```
alias velero='oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero'
cd cirros-test-with-dataVolumeContainer

oc create namespace cirros-test-cont
oc create -f cirros-test-1.yaml -f cirros-test-2.yaml -f cirros-test-3.yaml
velero backup create backup-cirros-test-cont-all --include-namespaces cirros-test-cont --snapshot-move-data=true
oc delete -f cirros-test-1.yaml -f cirros-test-2.yaml -f cirros-test-3.yaml
```
#### to restore scenario 1: (does NOT work)
See the results in [cirros-test-with-dataVolumeContainer/RESULTS](cirros-test-with-dataVolumeContainer/RESULTS)

```
velero restore create cirros-test-cont-1  --from-backup backup-cirros-test-cont-all --selector app=cirros-test-cont-1
velero restore create cirros-test-cont-2  --from-backup backup-cirros-test-cont-all --selector app=cirros-test-cont-2
velero restore create cirros-test-cont-3  --from-backup backup-cirros-test-cont-all --selector app=cirros-test-cont-3
```

#### to restore scenario 2: (works)
```
velero restore create cirros-test-1  --from-backup backup-cirros-test-cont-all
```

**Velero selectors in some cases are not working, The datavolume PV/PVC is not a restore resource**

[visual diff of restore's resources](cirros-test-with-dataVolumeContainer/RESULTS/visual_diff_of_restore.png)
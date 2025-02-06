# Various test cases for backing up / restoring vms

## backup all, restore one at a time
setup:
oc create -f cirros-datasource-datavolume.yaml
oc create -f cirros-test-1.yaml -f cirros-test-2.yaml -f cirros-test-3.yaml
oc create -f backup-cirros-test-all.yaml

to restore:
velero restore create cirros-test-1  --from-backup 3-cirros-vms-all --selector app=cirros-test-1
velero restore create cirros-test-2  --from-backup 3-cirros-vms-all --selector app=cirros-test-2
velero restore create cirros-test-3  --from-backup 3-cirros-vms-all --selector app=cirros-test-3


## backup 2 vms, restore 1
velero backup create namespace3vms-backup-2-vms --include-namespaces cirros-test --snapshot-move-data=true --or-selector "app=cirros-test-1 or app=cirros-test-2

velero restore create namespace3vms-backup-2-vms-backup1  --from-backup namespace3vms-backup-2-vms --selector app=cirros-test-1


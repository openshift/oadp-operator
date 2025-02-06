setup:
oc create -f cirros-datasource-datavolume.yaml
oc create -f cirros-test-1.yaml -f cirros-test-2.yaml -f cirros-test-3.yaml
oc create -f backup-cirros-test-all.yaml

to restore:
velero restore create cirros-test-1  --from-backup 3-cirros-vms-all --selector app=cirros-test-1
velero restore create cirros-test-2  --from-backup 3-cirros-vms-all --selector app=cirros-test-2
velero restore create cirros-test-3  --from-backup 3-cirros-vms-all --selector app=cirros-test-3

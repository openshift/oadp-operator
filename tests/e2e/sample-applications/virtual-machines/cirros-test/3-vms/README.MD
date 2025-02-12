# Various test cases for backing up / restoring vms using selectors

## DataVolume precreated: backup all, restore one at a time
#### setup:
```
cd cirros-test-with-dataVolumePV 
oc create namespace cirros-test-ds-pv
oc create -f cirros-datasource-datavolume.yaml
oc create -f cirros-test-1.yaml -f cirros-test-2.yaml -f cirros-test-3.yaml
oc create -f backup-cirros-test-pv-all.yaml
oc delete -f cirros-test-1.yaml -f cirros-test-2.yaml -f cirros-test-3.yaml
```

#### to restore:
```
alias velero='oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero'
velero restore create cirros-test-1  --from-backup 3-cirros-vms-all --selector app=cirros-test-pv-1
velero restore create cirros-test-2  --from-backup 3-cirros-vms-all --selector app=cirros-test-pv-2
velero restore create cirros-test-3  --from-backup 3-cirros-vms-all --selector app=cirros-test-pv-3
```


## backup 2 vms, restore 1:
```
velero backup create namespace3vms-backup-2-vms --include-namespaces cirros-test --snapshot-move-data=true --or-selector "app=cirros-test-1 or app=cirros-test-2
```
```
velero restore create namespace3vms-backup-2-vms-backup1  --from-backup namespace3vms-backup-2-vms --selector app=cirros-test-1
```

**The above use case should all work as expected. \0/**

## DataVolume not precreated, container disk: backup all, restore one at a time
#### setup:
```
cd cirros-test-dataVolumeContainer

oc create namespace cirros-test-cont
oc create -f cirros-test-1.yaml -f cirros-test-2.yaml -f cirros-test-3.yaml
oc create -f backup-cirros-test-cont-all.yaml
oc delete -f cirros-test-1.yaml -f cirros-test-2.yaml -f cirros-test-3.yaml
```
#### to restore scenario 1: (does NOT work)
```
alias velero='oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero'
velero restore create cirros-test-1  --from-backup 3-cirros-vms-all --selector app=cirros-test-cont-1
velero restore create cirros-test-2  --from-backup 3-cirros-vms-all --selector app=cirros-test-cont-2
velero restore create cirros-test-3  --from-backup 3-cirros-vms-all --selector app=cirros-test-cont-3
```

#### to restore scenario 2: (works)
```
velero restore create cirros-test-1  --from-backup 3-cirros-vms-all 
```

**Velero selectors in some cases are not working**

## Velero describe from a restore w/o selectors
```
Name:         [1mcirros-test-cont-restore-all[22m
Namespace:    openshift-adp
Labels:       <none>
Annotations:  <none>

Phase:                       [32mCompleted[0m
Total items to be restored:  48
Items restored:              48

Started:    2025-02-12 20:18:54 +0000 UTC
Completed:  2025-02-12 20:19:32 +0000 UTC

Warnings:
  Velero:     <none>
  Cluster:  could not restore, CustomResourceDefinition "datavolumes.cdi.kubevirt.io" already exists. Warning: the in-cluster version is different than the backed-up version
            could not restore, CustomResourceDefinition "reclaimspacecronjobs.csiaddons.openshift.io" already exists. Warning: the in-cluster version is different than the backed-up version
            could not restore, CustomResourceDefinition "virtualmachineinstances.kubevirt.io" already exists. Warning: the in-cluster version is different than the backed-up version
            could not restore, CustomResourceDefinition "virtualmachines.kubevirt.io" already exists. Warning: the in-cluster version is different than the backed-up version
  Namespaces:
    cirros-test-cont:  could not restore, RoleBinding "system:deployers" already exists. Warning: the in-cluster version is different than the backed-up version
                       could not restore, RoleBinding "system:image-builders" already exists. Warning: the in-cluster version is different than the backed-up version
                       could not restore, RoleBinding "system:image-pullers" already exists. Warning: the in-cluster version is different than the backed-up version

Backup:  cirros-test-cont-1

Namespaces:
  Included:  all namespaces found in the backup
  Excluded:  <none>

Resources:
  Included:        *
  Excluded:        nodes, events, events.events.k8s.io, backups.velero.io, restores.velero.io, resticrepositories.velero.io, csinodes.storage.k8s.io, volumeattachments.storage.k8s.io, backuprepositories.velero.io
  Cluster-scoped:  auto

Namespace mappings:  <none>

Label selector:  <none>

Or label selector:  <none>

Restore PVs:  auto

CSI Snapshot Restores:
  cirros-test-cont/cirros-test-cont-1-dv:
    Data Movement:
      Operation ID: dd-b87516bb-8253-406c-81c3-d94bc1fe2efc.b41e016a-79e0-4d1360739
      Data Mover: velero
      Uploader Type: kopia
  cirros-test-cont/cirros-test-cont-2-dv:
    Data Movement:
      Operation ID: dd-b87516bb-8253-406c-81c3-d94bc1fe2efc.2caca578-da22-4126a5405
      Data Mover: velero
      Uploader Type: kopia
  cirros-test-cont/cirros-test-cont-3-dv:
    Data Movement:
      Operation ID: dd-b87516bb-8253-406c-81c3-d94bc1fe2efc.f7ea1235-8fa0-482f7a446
      Data Mover: velero
      Uploader Type: kopia

Existing Resource Policy:   <none>
ItemOperationTimeout:       4h0m0s

Preserve Service NodePorts:  auto

Uploader config:

Restore Item Operations:
  Operation for persistentvolumeclaims cirros-test-cont/cirros-test-cont-1-dv:
    Restore Item Action Plugin:  velero.io/csi-pvc-restorer
    Operation ID:                dd-b87516bb-8253-406c-81c3-d94bc1fe2efc.b41e016a-79e0-4d1360739
    Phase:                       Completed
    Progress:                    157286400 of 157286400 complete (Bytes)
    Progress description:        Completed
    Created:                     2025-02-12 20:18:57 +0000 UTC
    Started:                     2025-02-12 20:19:18 +0000 UTC
    Updated:                     2025-02-12 20:19:23 +0000 UTC
  Operation for persistentvolumeclaims cirros-test-cont/cirros-test-cont-2-dv:
    Restore Item Action Plugin:  velero.io/csi-pvc-restorer
    Operation ID:                dd-b87516bb-8253-406c-81c3-d94bc1fe2efc.2caca578-da22-4126a5405
    Phase:                       Completed
    Progress:                    157286400 of 157286400 complete (Bytes)
    Progress description:        Completed
    Created:                     2025-02-12 20:18:57 +0000 UTC
    Started:                     2025-02-12 20:19:23 +0000 UTC
    Updated:                     2025-02-12 20:19:29 +0000 UTC
  Operation for persistentvolumeclaims cirros-test-cont/cirros-test-cont-3-dv:
    Restore Item Action Plugin:  velero.io/csi-pvc-restorer
    Operation ID:                dd-b87516bb-8253-406c-81c3-d94bc1fe2efc.f7ea1235-8fa0-482f7a446
    Phase:                       Completed
    Progress:                    157286400 of 157286400 complete (Bytes)
    Progress description:        Completed
    Created:                     2025-02-12 20:18:57 +0000 UTC
    Started:                     2025-02-12 20:19:07 +0000 UTC
    Updated:                     2025-02-12 20:19:13 +0000 UTC

HooksAttempted:   0
HooksFailed:      0

Resource List:
  apiextensions.k8s.io/v1/CustomResourceDefinition:
    - datavolumes.cdi.kubevirt.io(failed)
    - reclaimspacecronjobs.csiaddons.openshift.io(failed)
    - virtualmachineinstances.kubevirt.io(failed)
    - virtualmachines.kubevirt.io(failed)
  apps/v1/ControllerRevision:
    - cirros-test-cont/revision-start-vm-1a057a7f-e0b4-4630-8ee9-02088b8cbdf4-1(created)
    - cirros-test-cont/revision-start-vm-259a4db7-c616-4744-98dd-56feaf2d4e70-1(created)
    - cirros-test-cont/revision-start-vm-bc988d77-5ac4-4c48-a26f-a6afda4e9402-1(created)
  authorization.openshift.io/v1/RoleBinding:
    - cirros-test-cont/system:deployers(failed)
    - cirros-test-cont/system:image-builders(failed)
    - cirros-test-cont/system:image-pullers(failed)
  cdi.kubevirt.io/v1beta1/DataVolume:
    - cirros-test-cont/cirros-test-cont-1-dv(created)
    - cirros-test-cont/cirros-test-cont-2-dv(created)
    - cirros-test-cont/cirros-test-cont-3-dv(created)
  csiaddons.openshift.io/v1alpha1/ReclaimSpaceCronJob:
    - cirros-test-cont/cirros-test-cont-1-dv-1739390971(created)
    - cirros-test-cont/cirros-test-cont-2-dv-1739391033(created)
    - cirros-test-cont/cirros-test-cont-3-dv-1739391169(created)
  kubevirt.io/v1/VirtualMachine:
    - cirros-test-cont/cirros-test-cont-1(created)
    - cirros-test-cont/cirros-test-cont-2(created)
    - cirros-test-cont/cirros-test-cont-3(created)
  kubevirt.io/v1/VirtualMachineInstance:
    - cirros-test-cont/cirros-test-cont-1(skipped)
    - cirros-test-cont/cirros-test-cont-2(skipped)
    - cirros-test-cont/cirros-test-cont-3(skipped)
  policy/v1/PodDisruptionBudget:
    - cirros-test-cont/kubevirt-disruption-budget-6475b(created)
    - cirros-test-cont/kubevirt-disruption-budget-cjsx5(created)
    - cirros-test-cont/kubevirt-disruption-budget-f75qd(created)
  rbac.authorization.k8s.io/v1/RoleBinding:
    - cirros-test-cont/system:deployers(skipped)
    - cirros-test-cont/system:image-builders(skipped)
    - cirros-test-cont/system:image-pullers(skipped)
  v1/ConfigMap:
    - cirros-test-cont/kube-root-ca.crt(skipped)
    - cirros-test-cont/openshift-service-ca.crt(skipped)
  v1/PersistentVolume:
    - pvc-39526aee-9019-4332-ab19-11634e2c0578(skipped)
    - pvc-8ba45cac-6c22-4d89-a6e1-5dac166cb48f(skipped)
    - pvc-a03ead6e-216f-484d-90fa-99ff8e31fef9(skipped)
  v1/PersistentVolumeClaim:
    - cirros-test-cont/cirros-test-cont-1-dv(created)
    - cirros-test-cont/cirros-test-cont-2-dv(created)
    - cirros-test-cont/cirros-test-cont-3-dv(created)
  v1/Pod:
    - cirros-test-cont/virt-launcher-cirros-test-cont-1-ld2ff(skipped)
    - cirros-test-cont/virt-launcher-cirros-test-cont-2-ttjjc(skipped)
    - cirros-test-cont/virt-launcher-cirros-test-cont-3-s5m2d(skipped)
  v1/Secret:
    - cirros-test-cont/builder-dockercfg-cqlj4(skipped)
    - cirros-test-cont/default-dockercfg-rdf58(skipped)
    - cirros-test-cont/deployer-dockercfg-lhkmz(skipped)
  v1/ServiceAccount:
    - cirros-test-cont/builder(skipped)
    - cirros-test-cont/default(skipped)
    - cirros-test-cont/deployer(skipped)
  velero.io/v2alpha1/DataUpload:
    - openshift-adp/cirros-test-cont-1-pg9zl(skipped)
    - openshift-adp/cirros-test-cont-1-tjtc6(skipped)
    - openshift-adp/cirros-test-cont-1-xgxt2(skipped)
```

## Velero describe from a restore w/ selectors
```
Name:         [1mcirros-test-cont-restore-3rdvm-2[22m
Namespace:    openshift-adp
Labels:       <none>
Annotations:  <none>

Phase:                       [32mCompleted[0m
Total items to be restored:  5
Items restored:              5

Started:    2025-02-12 20:22:39 +0000 UTC
Completed:  2025-02-12 20:22:40 +0000 UTC

Backup:  cirros-test-cont-1

Namespaces:
  Included:  all namespaces found in the backup
  Excluded:  <none>

Resources:
  Included:        *
  Excluded:        nodes, events, events.events.k8s.io, backups.velero.io, restores.velero.io, resticrepositories.velero.io, csinodes.storage.k8s.io, volumeattachments.storage.k8s.io, backuprepositories.velero.io
  Cluster-scoped:  auto

Namespace mappings:  <none>

Label selector:  app=cirros-test-cont-3

Or label selector:  <none>

Restore PVs:  auto

CSI Snapshot Restores: <none included>

Existing Resource Policy:   <none>
ItemOperationTimeout:       4h0m0s

Preserve Service NodePorts:  auto

Uploader config:


HooksAttempted:   0
HooksFailed:      0

Resource List:
  cdi.kubevirt.io/v1beta1/DataVolume:
    - cirros-test-cont/cirros-test-cont-3-dv(created)
  kubevirt.io/v1/VirtualMachine:
    - cirros-test-cont/cirros-test-cont-3(created)
  velero.io/v2alpha1/DataUpload:
    - openshift-adp/cirros-test-cont-1-pg9zl(skipped)
    - openshift-adp/cirros-test-cont-1-tjtc6(skipped)
    - openshift-adp/cirros-test-cont-1-xgxt2(skipped)
```







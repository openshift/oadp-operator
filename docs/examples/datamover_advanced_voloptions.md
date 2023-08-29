# <h2 align="center">OADP Data Mover 1.2 Advanced Volume Options</h2>


- The official OpenShift OADP Data Mover documentation can be found [here](https://docs.openshift.com/container-platform/4.13/backup_and_restore/application_backup_and_restore/backing_up_and_restoring/backing-up-applications.html#oadp-using-data-mover-for-csi-snapshots_backing-up-applications)
- We maintain an up to date FAQ page [here](https://access.redhat.com/articles/5456281)

<h2>Background Information:<a id="pre-reqs"></a></h2>


OADP Data Mover 1.2 leverages some of the recently added features of Ceph to be 
performant in large scale environments, one being the 
[shallow copy](https://github.com/ceph/ceph-csi/blob/devel/docs/design/proposals/cephfs-snapshot-shallow-ro-vol.md) 
method, which is available > OCP 4.11. This feature requires use of the Data Mover
1.2 feature for volumeOptions so that other storageClasses and accessModes can be
used other than what is found on the source PVC. 

1. [Prerequisites](#pre-reqs)
2. [CephFS with ShallowCopy](#shallowcopy)
3. [CephFS and CephRBD Split Volumes](#fsrbd)

<h2>Prerequisites:<a id="pre-reqs"></a></h2>

- OCP > 4.11

- OADP operator and a credentials secret are created. Follow 
  [these steps](/docs/install_olm.md) for installation instructions.

- A CephFS and a CephRBD `StorageClass` and a `VolumeSnapshotClass` 
    - Installing ODF will create these in your cluster:

### CephFS VolumeSnapshotClass and StorageClass:

**Note:** The deletionPolicy, annotations, and labels

```yml
apiVersion: snapshot.storage.k8s.io/v1
deletionPolicy: Retain # <--- Note the Retain Policy
driver: openshift-storage.cephfs.csi.ceph.com
kind: VolumeSnapshotClass
metadata:
  annotations:
    snapshot.storage.kubernetes.io/is-default-class: 'true' # <--- Note the default
  labels:
    velero.io/csi-volumesnapshot-class: 'true'   # <--- Note the velero label 
  name: ocs-storagecluster-cephfsplugin-snapclass
parameters:
  clusterID: openshift-storage
  csi.storage.k8s.io/snapshotter-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/snapshotter-secret-namespace: openshift-storage
```

**Note:** The annotations
```yml
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: ocs-storagecluster-cephfs
  annotations:
    description: Provides RWO and RWX Filesystem volumes
    storageclass.kubernetes.io/is-default-class: 'true'  # <--- Note the default
provisioner: openshift-storage.cephfs.csi.ceph.com
parameters:
  clusterID: openshift-storage
  csi.storage.k8s.io/controller-expand-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/controller-expand-secret-namespace: openshift-storage
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-cephfs-node
  csi.storage.k8s.io/node-stage-secret-namespace: openshift-storage
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: openshift-storage
  fsName: ocs-storagecluster-cephfilesystem
reclaimPolicy: Delete
allowVolumeExpansion: true
volumeBindingMode: Immediate
```

### CephRBD VolumeSnapshotClass and StorageClass: 

**Note:** The deletionPolicy, and labels
```yml
apiVersion: snapshot.storage.k8s.io/v1
deletionPolicy: Retain # <--- Note: the Retain Policy
driver: openshift-storage.rbd.csi.ceph.com
kind: VolumeSnapshotClass
metadata:
  labels:
    velero.io/csi-volumesnapshot-class: 'true' # <--- Note velero 
  name: ocs-storagecluster-rbdplugin-snapclass
parameters:
  clusterID: openshift-storage
  csi.storage.k8s.io/snapshotter-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/snapshotter-secret-namespace: openshift-storage
```

```yml
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: ocs-storagecluster-ceph-rbd
  annotations:
    description: 'Provides RWO Filesystem volumes, and RWO and RWX Block volumes'
provisioner: openshift-storage.rbd.csi.ceph.com
parameters:
  csi.storage.k8s.io/fstype: ext4
  csi.storage.k8s.io/provisioner-secret-namespace: openshift-storage
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-rbd-node
  csi.storage.k8s.io/controller-expand-secret-name: rook-csi-rbd-provisioner
  imageFormat: '2'
  clusterID: openshift-storage
  imageFeatures: layering
  csi.storage.k8s.io/controller-expand-secret-namespace: openshift-storage
  pool: ocs-storagecluster-cephblockpool
  csi.storage.k8s.io/node-stage-secret-namespace: openshift-storage
reclaimPolicy: Delete
allowVolumeExpansion: true
volumeBindingMode: Immediate
```

- Create an additional CephFS `StorageClass` to make use of the `shallowCopy` feature:

```yml
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: ocs-storagecluster-cephfs-shallow
  annotations:
    description: Provides RWO and RWX Filesystem volumes
    storageclass.kubernetes.io/is-default-class: 'false'
provisioner: openshift-storage.cephfs.csi.ceph.com
parameters:
  csi.storage.k8s.io/provisioner-secret-namespace: openshift-storage
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-cephfs-node
  csi.storage.k8s.io/controller-expand-secret-name: rook-csi-cephfs-provisioner
  clusterID: openshift-storage
  fsName: ocs-storagecluster-cephfilesystem
  csi.storage.k8s.io/controller-expand-secret-namespace: openshift-storage
  backingSnapshot: 'true'     # <--- shallowCopy
  csi.storage.k8s.io/node-stage-secret-namespace: openshift-storage
reclaimPolicy: Delete
allowVolumeExpansion: true
volumeBindingMode: Immediate
```

- **Notes**: 
    - Make sure the default `VolumeSnapshotClass` and `StorageClass` are the same provisioner
    - The `VolumeSnapshotClass` must have the `deletionPloicy` set to Retain
    - The `VolumeSnapshotClasses` must have the label `velero.io/csi-volumesnapshot-class: 'true'`

- Install the latest VolSync operator using OLM.

![Volsync_install](/docs/images/volsync_install.png)

- We will be using VolSync's Restic option, hence configure a restic secret:

```yml
apiVersion: v1
kind: Secret
metadata:
  name: <secret-name>
type: Opaque
stringData:
  # The repository encryption key
  RESTIC_PASSWORD: my-secure-restic-password
```

<h1 align="center">Backup/Restore with CephFS ShallowCopy<a id="shallowcopy"></a></h1>

- Please ensure that a stateful application is running in a separate namespace with PVCs using 
  CephFS as the provisioner

- Please ensure the default `StorageClass` and `VolumeSnapshotClass` as cephFS, as shown
    in the [prerequisites](#pre-reqs)

- **Helpful Commands**:
    
    Check the VolumeSnapshotClass retain policy:
    ```
    oc get volumesnapshotclass -A  -o jsonpath='{range .items[*]}{"Name: "}{.metadata.name}{"  "}{"Retention Policy: "}{.deletionPolicy}{"\n"}{end}'
    ```
    Check the VolumeSnapShotClass lables:
    ```
    oc get volumesnapshotclass -A  -o jsonpath='{range .items[*]}{"Name: "}{.metadata.name}{"  "}{"labels: "}{.metadata.labels}{"\n"}{end}' 
    ```
    Check the StorageClass annotations:
    ```
    oc get storageClass -A  -o jsonpath='{range .items[*]}{"Name: "}{.metadata.name}{"  "}{"annotations: "}{.metadata.annotations}{"\n"}{end}' 
    ```

- Create a DPA similar to below:
  - Add the restic secret name from the previous step to your DPA CR 
    in `spec.features.dataMover.credentialName`. If this step is not completed 
    then it will default to the secret name `dm-credential`.


```yml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
  namespace: openshift-adp
spec:
  backupLocations:
    - velero:
        config:
          profile: default
          region: us-east-1
        credential:
          key: cloud
          name: cloud-credentials
        default: true
        objectStorage:
          bucket: <my-bucket>
          prefix: velero
        provider: aws
  configuration:
    nodeAgent:
      enable: false # [true, false]
      uploaderType: restic # [restic, kopia]
    velero:
      defaultPlugins:
        - openshift
        - aws
        - csi
        - vsm
  features:
    dataMover:
      credentialName: <restic-secret-name>
      enable: true
      volumeOptionsForStorageClasses:
        ocs-storagecluster-cephfs:
          sourceVolumeOptions:
            accessMode: ReadOnlyMany
            cacheAccessMode: ReadWriteMany
            cacheStorageClassName: ocs-storagecluster-cephfs
            storageClassName: ocs-storagecluster-cephfs-shallow
```

<hr style="height:1px;border:none;color:#333;">

<h4> For Backup <a id="backup"></a></h4>

- Create a backup CR:

```yml
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: <backup-name>
  namespace: <protected-ns>
spec:
  includedNamespaces:
  - <app-ns>
  storageLocation: velero-sample-1
```

- Monitor the datamover backup and artifacts via [a debug script](/docs/examples/debug.md)

OR
- Check the progress of the `volumeSnapshotBackup`(s):

```
oc get vsb -n <app-ns>
oc get vsb <vsb-name> -n <app-ns> -ojsonpath="{.status.phase}` 
```

- Wait several minutes and check the VolumeSnapshotBackup CR status for `completed`: 

- There should now be a snapshot(s) in the object store that was given in the restic secret.
- You can check for this snapshot in your targeted `backupStorageLocation` with a
prefix of `/<OADP-namespace>`

<h4> For Restore <a id="restore"></a></h4>

- Make sure the application namespace is deleted, as well as any volumeSnapshotContents
  that were created during backup.

- Create a restore CR:

```yml
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: <restore-name>
  namespace: <protected-ns>
spec:
  backupName: <previous-backup-name>
```
- Monitor the datamover backup and artifacts via [a debug script](/docs/examples/debug.md)
OR
- Check the `VolumeSnapshotRestore`(s) progress: 

```
oc get vsr -n <app-ns>
oc get vsr <vsr-name> -n <app-ns> -ojsonpath="{.status.phase}
```

- Check that your application data has been restored:

`oc get route <route-name> -n <app-ns> -ojsonpath="{.spec.host}"`


<h1 align="center">Backup/Restore with Split Volumes: CephFS and CephRBD<a id="fsrbd"></a></h1>

- Ensure a stateful application is running in a separate namespace with PVCs provisioned
  by both CephFS and CephRBD

- This assumes cephFS is being used as the default `StorageClass` and 
    `VolumeSnapshotClass`

- Create a DPA similar to below:
  - Add the restic secret name from the prerequisites to your DPA CR in 
  `spec.features.dataMover.credentialName`. If this step is not completed then 
    it will default to the secret name `dm-credential`
  - Note: `volumeOptionsForStorageClass` can be defined for multiple storageClasses,
    thus allowing a backup to complete with volumes with different providers.

```yml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
  namespace: openshift-adp
spec:
  backupLocations:
    - velero:
        config:
          profile: default
          region: us-east-1
        credential:
          key: cloud
          name: cloud-credentials
        default: true
        objectStorage:
          bucket: <my-bucket>
          prefix: velero
        provider: aws
  configuration:
    nodeAgent:
      enable: false
      uploaderType: restic
    velero:
      defaultPlugins:
        - openshift
        - aws
        - csi
        - vsm
  features:
    dataMover:
      credentialName: <restic-secret-name>
      enable: true
      volumeOptionsForStorageClasses:
        ocs-storagecluster-cephfs:
          sourceVolumeOptions:
            accessMode: ReadOnlyMany
            cacheAccessMode: ReadWriteMany
            cacheStorageClassName: ocs-storagecluster-cephfs
            storageClassName: ocs-storagecluster-cephfs-shallow
        ocs-storagecluster-ceph-rbd:
          sourceVolumeOptions:
            storageClassName: ocs-storagecluster-ceph-rbd
            cacheStorageClassName: ocs-storagecluster-ceph-rbd
          destinationVolumeOptions:
            storageClassName: ocs-storagecluster-ceph-rbd
            cacheStorageClassName: ocs-storagecluster-ceph-rbd
```
Note: The CephFS ShallowCopy feature can only be used for datamover backup operation, the ShallowCopy volume options are not supported for restore.

- Now follow the backup and restore steps from the previous example

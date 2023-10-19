<h1 align="center">Stateful Application Backup/Restore - VolumeSnapshotMover (OADP 1.2 or below)</h1>
<h2 align="center">Relocate Snapshots into your Object Storage Location</h2>

<h2>Background Information:<a id="pre-reqs"></a></h2>
<hr style="height:1px;border:none;color:#333;">

OADP Data Mover enables customers to back up container storage interface (CSI) volume snapshots to a remote object store. When Data Mover is enabled, you can restore stateful applications from the store if a failure, accidental deletion, or corruption of the cluster occurs. OADP Data Mover solution uses the Restic option of VolSync.<br><br>

- The official OpenShift OADP Data Mover documentation can be found [here](https://docs.openshift.com/container-platform/4.12/backup_and_restore/application_backup_and_restore/backing_up_and_restoring/backing-up-applications.html#oadp-using-data-mover-for-csi-snapshots_backing-up-applications)
- We maintain an up to date FAQ page [here](https://access.redhat.com/articles/5456281)
- <b>Note:</b> Data Mover is a tech preview feature in OADP 1.1.x.  Data Mover is planned to be fully supported by Red Hat in the OADP 1.2.0 release.
- <b>Note:</b> We recommend customers using OADP 1.2.x Data Mover to backup and restore ODF CephFS volumes, upgrade or install OCP 4.12 for improved performance.  OADP Data Mover can leverage CephFS shallow volumes in OCP 4.12+ which based on our testing improves the performance of backup times.
  - [CephFS ROX details](https://issues.redhat.com/browse/RHSTOR-4287)
  - [Provisioning and mounting CephFS snapshot-backed volumes](https://github.com/ceph/ceph-csi/blob/devel/docs/cephfs-snapshot-backed-volumes.md)

<h2>Prerequisites:<a id="pre-reqs"></a></h2>

<hr style="height:1px;border:none;color:#333;">

- Have a stateful application running in a separate namespace. 

- Follow instructions for installing the OADP operator and creating an 
appropriate `volumeSnapshotClass` and `storageClass`found [here](/docs/examples/CSI/csi_example.md).

- Install the VolSync operator using OLM.

Note: For OADP 1.2 you are not required to annotate the openshift-adp namespace (OADP Operator install namespace) with `volsync.backube/privileged-movers='true'`. This action
will be automatically performed by the Operator when the datamover feature is enabled.

![Volsync_install](/docs/images/volsync_install.png)

- We will be using VolSync's Restic option, hence configure a restic secret:

```
apiVersion: v1
kind: Secret
metadata:
  name: <secret-name>
type: Opaque
stringData:
  # The repository encryption key
  RESTIC_PASSWORD: my-secure-restic-password
```

- Create a DPA similar to below:
  - Add the restic secret name from the previous step to your DPA CR in `spec.features.dataMover.credentialName`.  
    If this step is not completed then it will default to the secret name `dm-credential`.

  - Note the CSI and VSM as `defaultPlugins` and `dataMover.enable` flag.


```
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
  namespace: openshift-adp
spec:
  features:
    dataMover: 
      enable: true
      credentialName: <secret-name>
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
          bucket: <bucket-name>
          prefix: <bucket-prefix>
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
```

<hr style="height:1px;border:none;color:#333;">

<h4> For Backup <a id="backup"></a></h4>

- Create a backup CR:

```
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

- Wait several minutes and check the VolumeSnapshotBackup CR status for `completed`: 

`oc get vsb -n <app-ns>`

`oc get vsb <vsb-name> -n <app-ns> -ojsonpath="{.status.phase}` 

- There should now be a snapshot in the object store that was given in the restic secret.
- You can check for this snapshot in your targeted `backupStorageLocation` with a
prefix of `/<OADP-namespace>`

<h4> For Restore <a id="restore"></a></h4>

- Make sure the application namespace is deleted, as well as the volumeSnapshotContent
  that was created by the Velero CSI plugin.

- Create a restore CR:

```
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: <restore-name>
  namespace: <protected-ns>
spec:
  backupName: <previous-backup-name>
```

- Wait several minutes and check the VolumeSnapshotRestore CR status for `completed`: 

`oc get vsr -n <app-ns>`

`oc get vsr <vsr-name> -n <app-ns> -ojsonpath="{.status.phase}` 

- Check that your application data has been restored:

`oc get route <route-name> -n <app-ns> -ojsonpath="{.spec.host}"`

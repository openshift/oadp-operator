<h1 align="center">Stateful Application Backup/Restore - VolumeSnapshotMover</h1>
<h2 align="center">Relocate Snapshots into your Object Storage Location</h2>

<h2>Prerequisites:<a id="pre-reqs"></a></h2>

<hr style="height:1px;border:none;color:#333;">

- Have a stateful application running in a separate namespace. 

- Follow instructions for installing the OADP operator and creating an 
appropriate `volumeSnapshotClass` and `storageClass`found [here](/docs/examples/csi_example.md).

- Install the [VolSync operator](https://volsync.readthedocs.io/en/stable/installation/index.html) using OLM.

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

  - Note the CSI `defaultPlugin` and `dataMover.enable` flag.


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
    restic:
      enable: false
    velero:
      defaultPlugins:
        - openshift
        - aws
        - csi
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

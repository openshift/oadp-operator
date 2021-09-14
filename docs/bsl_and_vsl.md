<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Backup Storage Locations and Volume Snapshot Locations Customization</h1>
<hr style="height:1px;border:none;color:#333;">

### Configure Backup Storage Locations and Volume Snapshot Locations

For configuring the `backupStorageLocations` and the `volumeSnapshotLocations` 
we will be using the `backupStorageLocations` and the `volumeSnapshotLocations` 
specs respectively in the `oadp_v1alpha1_velero_cr.yaml` file during the deployement. 

For instance, If we want to configure `aws` for `backupStorageLocations` as 
well as `volumeSnapshotLocations` pertaining to velero, our 
`oadp_v1alpha1_velero_cr.yaml` file should look something like this:

```
apiVersion: oadp.openshift.io/v1alpha1
kind: Velero
metadata:
  name: velero-sample
spec:
  defaultVeleroPlugins:
  - aws
  backupStorageLocations:
  - name: default
    provider: aws
    objectStorage:
      bucket: myBucket
      prefix: "velero"
    config:
      region: us-east-1
      profile: "default"
    credential:
      name: cloud-credentials
      namespace: oadp-operator-system
  volumeSnapshotLocations:
  - name: default
    provider: aws
    config:
      region: us-west-2
      profile: "default"
```
<b>Note:</b> 
- Be sure to use the same `secret` name you used while creating the cloud 
credentials secret in the Operator installation.
- Another thing to consider are the CR file specs; they should be tailored in 
accordance to your own cloud provider accounts. For instance, `bucket` spec value should be according to your own bucket name, and so on.

- Do not configure more than one `backupStorageLocations` per cloud provider; 
the velero installation will fail.
- Parameter reference for [backupStorageLocations](https://velero.io/docs/main/api-types/backupstoragelocation/) 
and [volumeSnapshotLocations](https://velero.io/docs/main/api-types/volumesnapshotlocation/)

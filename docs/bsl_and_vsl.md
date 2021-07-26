***
## Backup Storage Locations and Volume Snapshot Locations Customization
***

### Configure Backup Storage Locations and Volume Snapshot Locations

For configuring the `backupStorageLocations` and the `volumeSnapshotLocations` we will be using the `backup_storage_locations` and the `volume_snapshot_locations` specs respectively in the `konveyor.openshift.io_v1alpha1_velero_cr.yaml` file during the deployement. 

For instance, If we want to configure `aws` for `backupStorageLocations` as well as `volumeSnapshotLocations` pertaining to velero, our `konveyor.openshift.io_v1alpha1_velero_cr.yaml` file should look something like this:

```
apiVersion: konveyor.openshift.io/v1alpha1
kind: Velero
metadata:
  name: example-velero
spec:
  default_velero_plugins:
  - aws
  backup_storage_locations:
  - name: default
    provider: aws
    object_storage:
      bucket: myBucket
      prefix: "velero"
    config:
      region: us-east-1
      profile: "default"
    credentials_secret_ref:
      name: cloud-credentials
      namespace: oadp-operator
  volume_snapshot_locations:
  - name: default
    provider: aws
    config:
      region: us-west-2
      profile: "default"
```
<b>Note:</b> 
- Be sure to use the same `secret` name you used while creating the cloud credentials secret in step 3 of Operator   installation section.
- Another thing to consider are the CR file specs, they should be tailored in accordance to your own cloud provider accounts, for instance `bucket` spec value should be according to your own bucket name and so on.
- Do not configure more than one `backupStorageLocations` per cloud provider, the velero installation will fail.
- bsl/vsl parameters in the OADP Velero CR must be specified using `snake_case` rather than `camelCase`.
  Keep this in mind when using parameters from the below parameter reference. For example, use `s3_url` rather than `s3Url`.
- Parameter reference for [backupStorageLocations](https://velero.io/docs/main/api-types/backupstoragelocation/) and [volumeSnapshotLocations](https://velero.io/docs/main/api-types/volumesnapshotlocation/)

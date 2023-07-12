<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Backup Storage Locations and Volume Snapshot Locations Customization</h1>

### Configure Backup Storage Locations and Volume Snapshot Locations


For configuring the `backupStorageLocations` and the `volumeSnapshotLocations` 
we will be using the `backupLocations.Velero` and the `snapshotLocations.Velero` 
specs respectively in the `oadp_v1alpha1_dpa.yaml` file during the deployment. 

For instance, If we want to configure `aws` for `backupLocations` as 
well as `snapshotLocations` pertaining to Velero, our 
`oadp_v1alpha1_dpa.yaml` file should look something like this:

```
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: dpa-sample
spec:
  configuration:
    velero:
      defaultPlugins:
      - openshift
      - aws
    nodeAgent:
      enable: true
      uploaderType: restic
  backupLocations:
    - name: default
      velero:
        provider: aws
        default: true
        objectStorage:
          bucket: my-bucket
          prefix: my-prefix
        config:
          region: us-east-1
          profile: "default"
        credential:
          name: cloud-credentials
          key: cloud
  snapshotLocations:
    - name: default
      velero:
        provider: aws
        config:
          region: us-west-2
          profile: "default"

```

<b>Note:</b> 

- Be sure to use the same `secret` name you used while creating the cloud 
credentials secret in the Operator installation.

- Another thing to consider are the CR file specs; they should be tailored in 
accordance to your own cloud provider accounts. 
For instance, `bucket` spec value should be according to your own bucket name, and so on.

- Your `backupLocations` region should point to your bucket's region, and your 
`snapshotLocations` region should point to your cluster's region.

- Do not configure more than one `backupLocations` per cloud provider; 
the velero installation will fail.

- Parameter reference for [backupLocations](https://velero.io/docs/main/api-types/backupstoragelocation/) 
and [snapshotLocations](https://velero.io/docs/main/api-types/volumesnapshotlocation/)

- Please add the spec `spec.backupLocations.default: true` if you see recurring
warnings in velero logs with the message `"There is no existing backup storage location set as default."`. 
Similarly, you can add `default: true` for `snapshotLocations`.

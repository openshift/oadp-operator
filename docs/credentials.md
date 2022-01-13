<h1 align="center">Using Credentials with the OADP Operator</h1>


### Creating a Secret

- Use command `oc create secret generic <SECRET_NAME> --namespace openshift-adp --from-file cloud=<CREDENTIALS_FILE_PATH>`


### Credentials for BackupStorageLocation:

- If the secret name is not specified, OADP will use the default value.
    - AWS default secret name is `cloud-credentials`
    - GCP default secret name is `cloud-credentials-gcp`
    - Azure default secret name is `cloud-credentials-azure`

- If the `credential` spec has a custom name, this name will be used by the
registry, and the default name will not be expected. An example is shown below:

```
spec:
  ...
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
          name: my-custom-name
          key: cloud
```

    
### Credentials for VolumeSnapshotLocation:

- `VolumeSnapshotLocation` will **always** expect the default secret name to exist, 
which are described above.

- To use seperate credentials for `BackupStorageLocation` and `VolumeSnapshotLocation`, 
your `BackupStorageLocation` must provide a custom secret name.


### Use Cases

1. #### `BackupStorageLocation` and `VolumeSnapshotLocation` share credentials for one provider:

    - Use the default secret name for the provider.
    - Example for AWS:

    `oc create secret generic cloud-credentials --namespace openshift-adp --from-file cloud=<CREDENTIALS_FILE_PATH>`

```
spec:
  ...
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
  volumeSnapshots:
    - name: default
      velero:
        provider: aws
        config:
          region: us-west-2
          profile: "default"
```

<hr style="height:1px;border:none;color:#333;">

2. #### `BackupStorageLocation` and `VolumeSnapshotLocation` use the same provider but use different credentials:

    - an example of this could be Using AWS S3 for your BSL, and also using an S3 
    compatible provider, such as Minio or Noobaa.
                
    - To do so, the VSL credentials **must** be the default secret name, as shown 
    above. BSL credentials must have a custom secret name, and be provided in 
    the `credential` spec field.

    - Example for AWS:

    `oc create secret generic <CUSTOM_NAME> --namespace openshift-adp --from-file cloud=<CREDENTIALS_FILE_PATH>`

    `oc create secret generic cloud-credentials --namespace openshift-adp --from-file cloud=<CREDENTIALS_FILE_PATH>`

```
spec:
  ...
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
          name: my-custom-name
          key: cloud
  volumeSnapshots:
    - name: default
      velero:
        provider: aws
        config:
          region: us-west-2
          profile: "default"
```

<hr style="height:1px;border:none;color:#333;">

3. #### No `BackupStorageLocation` or `VolumeSnapshotLocation` specified, but the plugin for the provider exists:

    - The default secret name still needs to exist in order for the Velero installation
    to be successful, but the secret can be empty.
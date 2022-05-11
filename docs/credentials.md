<h1 align="center">Using Credentials with the OADP Operator</h1>

### Creating a Secret

- Use command `oc create secret generic <SECRET_NAME> --namespace openshift-adp --from-file cloud=<CREDENTIALS_FILE_PATH>`

<h3>Default Secret Names<a id="defaultsecrets"></a></h3>

  - AWS default secret name is `cloud-credentials`
  - GCP default secret name is `cloud-credentials-gcp`
  - Azure default secret name is `cloud-credentials-azure`

### Credentials for BackupStorageLocation:

- If the secret name is not specified, OADP will use the default value.
- If the `credential` spec has a custom name, this name will be used by the
  registry, and the default name will not be expected. Example:

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

- `VolumeSnapshotLocation` will **always** expect the [default secret name](#defaultsecrets) 
to exist, as described above.

### Separate Credentials for BSL and VSL:

- To use separate credentials for `BackupStorageLocation` and `VolumeSnapshotLocation`, 
your `BackupStorageLocation` must provide a custom secret name.

#### AWS Plugin Exception:

  - *If you are using the AWS plugin, you can use the `profile` config key
    to use one secret for separate credentials.* 
    Example AWS credentials and DPA:

  ```
    [backupStorage]
    aws_access_key_id=...
    aws_secret_access_key=...

    [volumeSnapshot]
    aws_access_key_id=...
    aws_secret_access_key=...
  ```

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
              profile: "backupStorage"
            credential:
              name: cloud-credentials
              key: cloud
      snapshotLocations:
        - name: default
          velero:
            provider: aws
            config:
              region: us-west-2
              profile: "volumeSnapshot"
  ```


## Use Cases

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
  snapshotLocations:
    - name: default
      velero:
        provider: aws
        config:
          region: us-west-2
          profile: "default"
```

<hr style="height:1px;border:none;color:#333;">

2. #### `BackupStorageLocation` and `VolumeSnapshotLocation` use the same provider but use different credentials:

    - An example of this could be Using AWS S3 for your BSL, and also using an S3 
    compatible provider, such as Minio or Noobaa.

    - As mentioned previously, if you are using the AWS plugin, you can use one
      secret with separate credentials. Further information [here](#separatecreds).
                
    - Otherwise, the VSL credentials **must** be the [default secret name](#defaultsecrets) and 
    BSL credentials must have a custom secret name, and be provided in the `credential` spec field.

    - Example:

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
  snapshotLocations:
    - name: default
      velero:
        provider: aws
        config:
          region: us-west-2
          profile: "default"
```

<hr style="height:1px;border:none;color:#333;">

1. #### No `BackupStorageLocation` specified, but the plugin for the provider exists:
Specify .spec.configuration.noDefaultBackupLocation like so:
```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
spec:
  configuration:
    velero:
      defaultPlugins:
      - openshift
      - aws
      noDefaultBackupLocation: true
    restic:
      enable: true
```
If you don't need volumesnapshotlocation, you will not need to create a VSL credentials.

If you need `VolumeSnapshotLocation`, regardless of the `noDefaultBackupLocation` setting, you will need a to create VSL credentials.

VSL credentials **must** be the [default secret name](#defaultsecrets)

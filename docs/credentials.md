<h1 align="center">Using Credentials with the OADP Operator</h1>


1. [Creating a Secret: OADP](#creating-a-secret-for-oadp)
    1. [Credentials for BackupStorageLocation](#credentials-for-backupstoragelocation)
    2. [Credentials for VolumeSnapshotLocation](#credentials-for-volumesnapshotlocation)
2. [Separate Credentials for BSL and VSL](#separate-credentials-for-bsl-and-vsl)
3. [AWS Plugin Exception](#aws-plugin-exception)
4. [Use Cases](#use-cases)
    1. [BSL and VSL share credentials for one provider](#backupstoragelocation-and-volumesnapshotlocation-share-credentials-for-one-provider)
    2. [BSL and VSL use the same provider but use different credentials](#backupstoragelocation-and-volumesnapshotlocation-use-the-same-provider-but-use-different-credentials)
    3. [No BSL specified but the plugin for the provider exists](#no-backupstoragelocation-specified-but-the-plugin-for-the-provider-exists)
5. [Creating a Secret: OADP with VolumeSnapshotMover](#creating-a-secret-for-volumesnapshotmover)

### Creating a Secret for OADP

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

- If the secret name is not specified, OADP will use the default value.
- If the `credential` spec has a custom name, this name will be used by the
  registry, and the default name will not be expected. Example:

```
spec:
  ...
  snapshotLocations:
    - velero:
        config:
          profile: default
          region: us-east-1
        provider: aws
        credential:
          name: my-custom-name
          key: cloud
```

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
        - velero:
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
    - velero:
        provider: aws
        config:
          region: us-west-2
          profile: "default"
        credential:
          name: cloud-credentials
          key: cloud
```

<hr style="height:1px;border:none;color:#333;">

2. #### `BackupStorageLocation` and `VolumeSnapshotLocation` use the same provider but use different credentials:

    - An example of this could be Using AWS S3 for your BSL, and also using an S3 
    compatible provider, such as Minio or Noobaa.

    - As mentioned previously, if you are using the AWS plugin, you can use one
      secret with separate credentials. Further information [here](#separatecreds).
                
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
    - velero:
        provider: aws
        config:
          region: us-west-2
          profile: "default"
        credential:
          name: my-custom-name2
          key: cloud
```

<hr style="height:1px;border:none;color:#333;">

3. #### No `BackupStorageLocation` specified but the plugin for the provider exists:

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
    nodeAgent:
      enable: true
      uploaderType: restic
```
If you don't need volumesnapshotlocation, you will not need to create a VSL credentials.

If you need `VolumeSnapshotLocation`, regardless of the `noDefaultBackupLocation` setting, you will need a to create VSL credentials.


### Creating a Secret for volumeSnapshotMover (OADP 1.2 or below)

VolumeSnapshotMover requires a restic secret. It can be configured as so:

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

- *Note:* `dpa.spec.features.dataMover.credentialName` must match the name of the secret. 
  Otherwise it will default to the name `dm-credential`.

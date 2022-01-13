<h1 align="center">Using Credentials with the OADP Operator</h1>


### Credentials for BackupStorageLocation:

- If the secret name is not specified, OADP will use the default value.
    - AWS default secret name is `cloud-credentials`
    - GCP default secret name is `cloud-credentials-gcp`
    - Azure default secret name is `cloud-credentials-azure`

- If the `credential` spec has a custom name, this name will be used by the
registry, and the default name will not be expected.

    
### Credentials for VolumeSnapshotLocation:

- `VolumeSnapshotLocation` will **always** expect the default secret name to exist.

- To use seperate credentials for `BackupStorageLocation` and `VolumeSnapshotLocation`, 
your `BackupStorageLocation` must provide a custom secret name.


### Use Cases

1. `BackupStorageLocation` and `VolumeSnapshotLocation` share credentials for one provider:
    - Use the default secret name for the provider.

2. `BackupStorageLocation` and `VolumeSnapshotLocation` use the same provider but
    use different credentials:
    - an example of this could be Using AWS S3 for your BSL, and also using an S3 
    compatible provider, such as Minio or Noobaa.
                
    - To do so, the VSL credentials **must** be the default secret name, as shown 
    above. BSL credentials must have a custom secret name, and be provided in 
    the `credential` spec field.

3. No `BackupStorageLocation` or `VolumeSnapshotLocation` specified, but the plugin
    for the provider exists:
    - The default secret name still needs to exist in order for the Velero installation
    to be successful, but the secret can be empty.
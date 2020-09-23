***
# Noobaa + Self-Hosted S3 Providers
***

Velero allows a user to configure the BackupStorageLocation object with any
valid s3 provider. This can include tools like Noobaa & Minio. OADP Operator
allows you to integrate with Noobaa in a couple of ways. The first option is to
manually install Noobaa somewhere, and then configure the BackupStorageLocation
configuration on the `Velero` CR to use it. The second option is to allow OADP
to discover an existing OCS operator installation and attempt to create the
Noobaa bucket automatically for the user and configure the BSL for OADP without
any user intervention. (*NOTE*: This feature has known bugs which need to be
addressed. We recommend installing Noobaa/Minio manually and following the
first set of instructions below).

## Configure OADP with precreated Noobaa or Minio S3 Bucket

With an existing bucket created inside of Noobaa or Minio, a user can configure
OADP to setup a `BackupStorageLocation` object for Velero. The bucket
credentials will still need to be created as a secret in the `oadp-operator`
namespace.

The credentials file should follow this AWS credentials
[template](https://github.com/konveyor/velero-examples/blob/master/velero-install/aws-credentials)

```
oc create secret generic cloud-credentials --namespace oadp-operator --from-file cloud=<CREDENTIALS_FILE_PATH>
```

With the secret created, make sure you have the URL of the s3 service and set
the following `backup_storage_locations` spec in the `Velero` CR:
```
  backup_storage_locations:
    - config:
        profile: default
        region: noobaa                   # could be different for Minio depending on server configuration
        s3_url: <S3_URL_ROUTE>           # s3 URL
        s3_force_path_style: true        # force velero to use path-style convention
        insecure_skip_tls_verify: true   # insecure connections
      credentials_secret_ref:
        name: cloud-credentials
        namespace: oadp-operator
      name: default
      object_storage:
        bucket: noobaa-bucket-name       # Bucket name
        prefix: velero
      provider: aws                      # aws provider means use s3 client               
```

*NOTE*: For Minio, the default region is `minio` and can change depending on
server configuration.


## OADP Operator with NooBaa automatic install

This set of instructions explains how to use OADP to install and configure
Noobaa automatically. Please note this work is still being tested and is prone
to bugs.

Please follow the following steps in order to install OADP Operator with NooBaa:

1. Create a namespace named `oadp-operator`.
2. Do not create any cloud credentials secret as the secret comes out of the box for NooBaa.
3. Now install the OCS (OpenShift Container Storage) operator from the OperatorHub in the `openshift-storage` namespace, so that the requisite NooBaa CRDs get deployed on the cluster and wait till the OCS operator pods are in running state.
4. Make sure the Velero CR file specifically has the following:
   - `noobaa: true`
   - `enable_restic: true`
   - `use_upstream_images: true`
   - `default_velero_plugins` list should only consist of `aws` plugin
   - No data pertaining to Volume Snapshot Locations and Backup Storage Locations.
 
      The CR file may look somewhat like this:
      ```
        apiVersion: konveyor.openshift.io/v1alpha1
        kind: Velero
        metadata:
        name: example-velero
        spec:
          use_upstream_images: true
          noobaa: true
          default_velero_plugins:
          - aws
          enable_restic: true
      ```
  
5. Now for deployment of velero use the following commands in sequence:
```
oc create -f deploy/
oc create -f deploy/crds/konveyor.openshift.io_veleros_crd.yaml
oc create -f deploy/crds/konveyor.openshift.io_v1alpha1_velero_cr.yaml
```

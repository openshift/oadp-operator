<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Noobaa + Self-Hosted S3 Providers</h1>
<hr style="height:1px;border:none;color:#333;">


1. [Overview](#overview)
2. [Configure Noobaa or Minio Bucket](#configure-oadp-with-precreated-noobaa-or-minio-s3-bucket)
3. [Get Noobaa Route and Credentials](#get-noobaa-s3-route-and-credentials)
4. [Automatic Install](#oadp-operator-with-noobaa-automatic-install)
5. [Disaster Scenarios](#noobaa-and-disaster-scenarios)


## Overview

Velero allows a user to configure the `BackupStorageLocation` object with any
valid s3 provider. This can include tools like Noobaa & Minio. OADP Operator
allows you to integrate with Noobaa in a couple of ways: 

The first option is to manually install Noobaa somewhere, and then configure 
the BackupStorageLocation configuration on the DataProtectionApplication (DPA) CR. 

The second option is to allow OADP to discover an existing OCS operator 
installation and attempt to create the Noobaa bucket automatically for the user, 
and configure the BSL for OADP without any user intervention. 

*NOTE*: This feature has known bugs which need to be
addressed. We recommend installing Noobaa/Minio manually and following the
first set of instructions below.

## Configure OADP with precreated Noobaa or Minio S3 Bucket

With an existing bucket created inside of Noobaa or Minio, a user can configure
OADP to setup a `BackupStorageLocation` object for Velero. The bucket
credentials will still need to be created as a secret in the `openshift-adp`
namespace.

The credentials file should follow this AWS credentials
[template](https://github.com/konveyor/velero-examples/blob/master/velero-install/aws-credentials)

### Get NooBaa s3 route and credentials
To get the required information from NooBaa, you can use the NooBaa CLI to get
the s3 route, bucket name, and credentials. Optionally, you can grab this
information from OCP. To get the s3 route (assuming NooBaa/OCS is installed in
`openshift-storage`):

```
$ oc get route s3 -n openshift-storage
```

To get the bucket name for a given `ObjectBucketClaim`:

```
$ oc get obc <obc_name> -o yaml -n openshift-storage | grep bucketName
```

To get the credentials for the bucket, find the associated secret in the NooBaa
namespace with the same name as the `ObjectBucketClaim`.

```
$ oc create secret generic cloud-credentials --namespace openshift-adp --from-file cloud=<CREDENTIALS_FILE_PATH>
```

With the secret created, make sure you have the URL of the s3 service and set
the following `backupStorageLocations` spec in the Velero CR:

```
  backupLocations:
    - name: default
      velero:
       config:
         profile: "default"
         region: noobaa                  # could be different for Minio depending on server configuration
         s3Url: <S3_URL_ROUTE>           # s3 URL
         s3ForcePathStyle: "true"        # force velero to use path-style convention
         insecureSkipTLSVerify: "true"   # insecure connections
       credential:
         name: cloud-credentials
         key: cloud
       objectStorage:
         bucket: noobaa-bucket-name       # Bucket name
         prefix: velero
       provider: aws                      # aws provider means use s3 client               

```

*NOTE*: For Minio, the default region is `minio`, and can change depending on
server configuration.

## OADP Operator with NooBaa automatic install

This set of instructions explains how to use OADP to install and configure
Noobaa automatically. Please note this work is still being tested, and is prone
to bugs.

Please follow the these steps in order to install OADP Operator with NooBaa:

1. Create a namespace named `openshift-adp`.
2. Do not create any cloud credentials secret, as the secret comes out of the 
box for NooBaa.
3. Now install the OCS (OpenShift Container Storage) operator from the 
OperatorHub in the `openshift-storage` namespace, so that the requisite NooBaa 
CRDs get deployed on the cluster, and wait untill the OCS operator pods are in 
running state.
4. Make sure the Velero CR file specifically has the following:
   - `configuration.nodeAgent.enable: true`
   - `configuration.nodeAgent.uploaderType` is set to `restic` or `kopia`
   - `defaultPlugins` list should only consist of `aws` plugin
   - No data pertaining to Volume Snapshot Locations and Backup Storage Locations.
 
The DPA CR file may look somewhat like this:

  ```
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
    nodeAgent:
      enable: true
      uploaderType: restic
  ```
  
5. Now for deployment of Velero use the following:

```
oc create -f config/samples/oadp_v1alpha1_dpa.yaml
```


## NooBaa and disaster scenarios:

- If you are using cluster storage for your NooBaa bucket backupStorageLocation, 
then backups will be subjected to disaster.
- To avoid such case, you will need to configure Noobaa as an external object store: 
  - [Configuring the backing store](https://access.redhat.com/documentation/en-us/red_hat_openshift_data_foundation/4.10/html/managing_hybrid_and_multicloud_resources/adding-storage-resources-for-hybrid-or-multicloud_rhodf)

<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Authentication Methods for Azure Plugin </h1>

### Using Service Principal Credentials
1. To use Service Principal as authentication mechanism, create a credential file using the following command,
```
cat << EOF  > ./credentials-velero
AZURE_SUBSCRIPTION_ID=${AZURE_SUBSCRIPTION_ID}
AZURE_TENANT_ID=${AZURE_TENANT_ID}
AZURE_CLIENT_ID=${AZURE_CLIENT_ID}
AZURE_CLIENT_SECRET=${AZURE_CLIENT_SECRET}
AZURE_RESOURCE_GROUP=${AZURE_RESOURCE_GROUP}
AZURE_CLOUD_NAME=AzurePublicCloud
EOF
```

<b>Note:</b> 
- Servical Principal credentials does not support backing up of images. 
- If you are looking for that feature, please add storage access key to the credentials as follows,
 ```
cat << EOF  > ./credentials-velero
AZURE_SUBSCRIPTION_ID=${AZURE_SUBSCRIPTION_ID}
AZURE_TENANT_ID=${AZURE_TENANT_ID}
AZURE_CLIENT_ID=${AZURE_CLIENT_ID}
AZURE_CLIENT_SECRET=${AZURE_CLIENT_SECRET}
AZURE_RESOURCE_GROUP=${AZURE_RESOURCE_GROUP}
AZURE_STORAGE_ACCOUNT_ACCESS_KEY=${AZURE_STORAGE_ACCOUNT_ACCESS_KEY}
AZURE_CLOUD_NAME=AzurePublicCloud
EOF
```

2. Once you have the credentials file, create the secret using the following command,

```
oc create secret generic cloud-credentials-azure --namespace openshift-adp --from-file cloud=./credentials-velero
```

3. Create a DataProtectionApplication (DPA) CR with the appropriate values. For example, the DPA CR would look like,
```
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: dpa-sample
spec:
  backupLocations:
    - velero:
        config:
          resourceGroup: <resource_group_name>
          storageAccount: <storage_account>
          subscriptionId: <subscription_id>
        credential:
          key: cloud
          name: cloud-credentials-azure
        default: true
        objectStorage:
          bucket: <bucket_name>
          prefix: velero
        provider: azure
  configuration:
    nodeAgent:
      enable: true
      uploaderType: restic
    velero:
      defaultPlugins:
        - openshift
        - azure
  snapshotLocations:
    - velero:
        config:
          resourceGroup: <resource_group_name>
          subscriptionId: <subscription_id>
        provider: azure
```

### Using Storage Account Access Key Credentials

1. create a credential file using the following command,
```
cat << EOF  > ./credentials-velero
AZURE_STORAGE_ACCOUNT_ACCESS_KEY=${AZURE_STORAGE_ACCOUNT_ACCESS_KEY}
AZURE_CLOUD_NAME=AzurePublicCloud
EOF
```

2. Once you have the credentials file, create the secret using the following command,

```
oc create secret generic cloud-credentials-azure --namespace openshift-adp --from-file cloud=./credentials-velero
```

3. Create a DataProtectionApplication (DPA) CR with the appropriate values. For example, the DPA CR would look like,
```
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: dpa-sample
spec:
  backupLocations:
    - velero:
        config:
          resourceGroup: <resource_group_name>
          storageAccount: <storage_account>
          subscriptionId: <subscription_id>
          storageAccountKeyEnvVar: AZURE_STORAGE_ACCOUNT_ACCESS_KEY
        credential:
          key: cloud
          name: cloud-credentials-azure
        default: true
        objectStorage:
          bucket: <bucket_name>
          prefix: velero
        provider: azure
  configuration:
    nodeAgent:
      enable: true
      uploaderType: restic
    velero:
      defaultPlugins:
        - openshift
        - azure
  snapshotLocations:
    - velero:
        config:
          resourceGroup: <resource_group_name>
          subscriptionId: <subscription_id>
        provider: azure
```

<b>Note:</b> 
If you would like to take backups to the specified VolumeSnapshotLocation, make sure to include Service Principal credentials. 

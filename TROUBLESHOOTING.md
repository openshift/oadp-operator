<h1 align="center">Troubleshooting<a id="troubleshooting"></a></h1>

  If you need help, first search if there is 
    [already an issue filed](https://github.com/openshift/oadp-operator/issues) 
    or [create a new issue](https://github.com/openshift/oadp-operator/issues/new). 

<hr style="height:1px;border:none;color:#333;">

## Debugging Failed Backups:
 - 


### Debugging failed volume backups:
  - Restic:


  - Cloud Snapshots:


  - CSI Snapshots:

<hr style="height:1px;border:none;color:#333;">

## Debugging Failed Restores:
 - 


### Debugging failed volume restores:
  - Restic:


  - Cloud Snapshots:


  - CSI Snapshots:

<hr style="height:1px;border:none;color:#333;">

## Common Misconfigurations:

### Credentials secret is not properly formatted:
  - AWS:
    - An example of correct AWS credentials:

    ```
    [<INSERT_PROFILE_NAME>]
    aws_access_key_id=<INSERT_VALUE>
    aws_secret_access_key=<INSERT_VALUE>
    ```

    *Note:* Do not use quotes while putting values in place of INSERT_VALUE Placeholders

  - GCP:

  - Azure:
   

### Errors in the Velero pod:

-  **Error:** `Backup store contains invalid top-level directories: [someDirName]`

    **Problem:** your object storage root/prefix directory contains directories not 
    from velero's [approved list](https://github.com/vmware-tanzu/velero/blob/6f64052e94ef71c9d360863f341fe3c11e319f08/pkg/persistence/object_store_layout.go#L37-L43)

    **Solutions:**
    1. Define prefix directory inside a storage bucket where backups are to be uploaded instead of object storage root. In your Velero CR set a prefix for velero to use in `Velero.spec.backupStorageLocations[*].objectStorage.prefix`

    ```
        objectStorage:
          bucket: your-bucket-name
          prefix: <DirName>
    ```

    2. Delete the offending directories from your object storage location.


### Known issue with Backup/Restore using Restic:

-  **Error:** `Using Restic as backup method causes PartiallyFailed/Failed errors in the Restore/Backup`

    **Solution:**
    
    The changes in the backup/restore process for mitigating this error would be a two step restore process where, in the first step we would perform a restore excluding the replicationcontroller and deploymentconfig resources, and the second step would involve a restore including these resources. The backup and restore commands are given below for more clarity. (The examples given below are a use case for backup/restore of a target namespace, for other cases a similar strategy can be followed).

    Please note that this is a temporary fix for this issue and there are ongoing discussions to solve it.

    Step 1: Initiate the backup as any normal backup for restic.
    ```
    velero create backup <backup-name> -n openshift-adp --include-namespaces=<TARGET_NAMESPACE>
    ```

    Step 2: Initiate a restore excluding the replicationcontroller and deploymentconfig resources.
    ```
    velero restore create --from-backup=<BACKUP_NAME> -n openshift-adp --include-namespaces <TARGET_NAMESPACE> --exclude-resources replicationcontroller,deploymentconfig --restore-volumes=true
    ```

    Step 3: Initiate a restore including the replicationcontroller and deploymentconfig resources.
    ```
    velero restore create --from-backup=<BACKUP_NAME> -n openshift-adp --include-namespaces <TARGET_NAMESPACE> --include-resources replicationcontroller,deploymentconfig --restore-volumes=true
    ```


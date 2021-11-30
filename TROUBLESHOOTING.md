<h1 align="center">Troubleshooting<a id="troubleshooting"></a></h1>

  If you need help, first search if there is 
    [already an issue filed](https://github.com/openshift/oadp-operator/issues) 
    or [create a new issue](https://github.com/openshift/oadp-operator/issues/new). 

<hr style="height:1px;border:none;color:#333;">

## Debugging Failed Backups:
 This section includes how to debug a failed backup. For more specific issues related to restic/CSI/Volume snapshots check out the section <link to section>.

1. Check for validation errors in the backup by running the following command,
`oc describe backup <backupName>`
Alternatively, if you have a local velero installation, you can also run `velero describe backup <backupName> -n <namespace>` and `velero backup logs <backupName> -n <namespace>`.
2. Run `oc logs pod/<veleroPodName>` to check if there are any errors.
3. Fix errors if any. 

If the issue still persists, [create a new issue](https://github.com/openshift/oadp-operator/issues/new) if [an issue doesnt exist already](https://github.com/openshift/oadp-operator/issues)

### Debugging failed volume backups:
  - Restic: 
    1. Obtain the Restic pod logs by running the following command,
  `oc logs -l name=restic`. Check for errors.  

  - Cloud Snapshots:
  

  - CSI Snapshots:

<hr style="height:1px;border:none;color:#333;">

## Debugging Failed Restores:
 This section includes how to debug a failed restore. For more specific issues related to restic/CSI/Volume snapshots check out the section <link to section>.

1. Check for validation errors in the restore by running the following command,
`oc describe restore <restoreName>`
Alternatively, if you have a local velero installation, you can also run `velero describe restore <restoreName> -n <namespace>` and `velero restore logs <restoreName> -n <namespace>`.
2. Run `oc logs pod/<veleroPodName>` to check if there are any errors.
3. Fix errors if any. 

If the issue still persists, [create a new issue](https://github.com/openshift/oadp-operator/issues/new) if [an issue doesnt exist already](https://github.com/openshift/oadp-operator/issues)

### Debugging failed volume restores:
  - Restic:
  1. Obtain the Restic pod logs by running the following command,
  `oc logs -l name=restic`. Check for errors.  

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
### Errors in backup logs:

-   **Error:** 
    `error getting volume info: rpc error: code = Unknown desc = InvalidVolume.NotFound: The volume ‘vol-xxxx’ does not exist.\n\tstatus code: 400`

    **Problem** 
    AWS PV and Volume snaphot location are in different region.

    **Solution**
    Edit Velero `volume_snapshot_location` to the region specified in PV, change region in VolumeSnapshotLocation resource to the region mentioned in the PV, and then create a new Backup.


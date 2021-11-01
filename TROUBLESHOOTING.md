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

<hr style="height:1px;border:none;color:#333;">

### Restic - NFS volumes and `root_squash`:

If using NFS volumes while `root_squash` is enabled, Restic will be mapped to 
`nfsnobody` and not have the proper permissions to perform a backup/restore. 

#### To solve this issue:
  - Use supplemental groups, and apply this same supplemental group to the Restic
    daemonset.
  - Set `no_root_squash`.

  <hr style="height:1px;border:none;color:#333;"> 

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



-  


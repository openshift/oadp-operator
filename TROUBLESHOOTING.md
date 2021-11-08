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



-  


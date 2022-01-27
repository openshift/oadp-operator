<h1 align="center">Troubleshooting<a id="troubleshooting"></a></h1>

1. [Debugging Failed Backups](#backup)
    1. [Debugging Failed Volume Backups](#volbackup)
2. [Debugging Failed Restores](#restore)
    1. [Debugging Failed Volume Restores](#volrestore)
3. Common Issues and Misconfigurations
    1. [Credentials Not Properly Formatted](#creds)
    2. [Errors in the Velero Pod](#velpod)
    3. [Errors in Backup Logs](#backuplogs)
    4. [Backup/Restore is Stuck In Progress](#stuck)
    5. [Restic - NFS Volumes and rootSquash](#rootsquash)
    6. [Issue with Backup/Restore of DeploymentConfig using Restic](#deployconfig)
    7. [New Restic Backup Partially Failing After Clearing Bucket](#resbackup)


If you need help, first search if there is [already an issue filed](https://github.com/openshift/oadp-operator/issues) 
  or [create a new issue](https://github.com/openshift/oadp-operator/issues/new).


<hr style="height:1px;border:none;color:#333;">

<h1 align="center">Debugging Failed Backups<a id="backup"></a></h1>

This section includes steps to debug a failed backup. For more specific issues related to Restic/CSI/volume snapshots check out the following section. 

1. Check for validation errors in the backup by running the following command,
`oc describe backup <backupName>`
Alternatively, if you have a local Velero installation, you can also run `velero describe backup <backupName> -n <namespace>` and `velero backup logs <backupName> -n <namespace>`.

2. Run `oc logs pod/<veleroPodName>` to check if there are any errors.

3. Fix errors if any. 

If the issue still persists, [create a new issue](https://github.com/openshift/oadp-operator/issues/new) if [an issue doesnt exist already](https://github.com/openshift/oadp-operator/issues)


<h3 align="center">Debugging Failed Volume Backups<a id="volbackup"></a></h3>

  - Restic: 
    1. Obtain the Restic pod logs by running the following command,
  `oc logs -l name=restic`. Check for errors.  

  - Cloud Snapshots:
  

  - CSI Snapshots:

<hr style="height:1px;border:none;color:#333;">


<h1 align="center">Debugging Failed Restores<a id="restore"></a></h1>

This section includes how to debug a failed restore. For more specific issues related to restic/CSI/Volume snapshots check out the following section.

1. Check for validation errors in the restore by running the following command,
`oc describe restore <restoreName>`
Alternatively, if you have a local velero installation, you can also run `velero describe restore <restoreName> -n <namespace>` and `velero restore logs <restoreName> -n <namespace>`.

2. Run `oc logs pod/<veleroPodName>` to check if there are any errors.

3. Fix errors if any. 


If the issue still persists, [create a new issue](https://github.com/openshift/oadp-operator/issues/new) if [an issue doesnt exist already](https://github.com/openshift/oadp-operator/issues)

<h3 align="center">Debugging Failed Volume Restores<a id="volrestore"></a></h3>

  - Restic:
    1. Obtain the Restic pod logs by running the following command,
    `oc logs -l name=restic`. Check for errors.  

  - Cloud Snapshots:


  - CSI Snapshots:



<hr style="height:1px;border:none;color:#333;">

<h1 align="center">Common Issues and Misconfigurations<a id="misconfig"></a></h1>

<h3 align="center">Credentials Secret Not Properly Formatted<a id="creds"></a></h3>

  - AWS:
    - An example of correct AWS credentials:

    ```
    [<INSERT_PROFILE_NAME>]
    aws_access_key_id=<INSERT_VALUE>
    aws_secret_access_key=<INSERT_VALUE>
    ```

    *Note:* Do not use quotes while putting values in place of INSERT_VALUE Placeholders


<hr style="height:1px;border:none;color:#333;"> 

<h3 align="center">Errors in the Velero Pod<a id="velpod"></a></h3>

-  **Error:** `Backup storage contains invalid top-level directories: [someDirName]`

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

  
<hr style="height:1px;border:none;color:#333;"> 

<h3 align="center">Errors in Backup Logs<a id="backuplogs"></a></h3>

-   **Error:** 
    `error getting volume info: rpc error: code = Unknown desc = InvalidVolume.NotFound: The volume ‘vol-xxxx’ does not exist.\n\tstatus code: 400`

    **Problem** 
    AWS PV and Volume snaphot location are in different regions.

    **Solution**
    Edit Velero `volume_snapshot_location` to the region specified in PV, and 
    change region in VolumeSnapshotLocation resource to the region mentioned in the PV, and then create a new backup.


<hr style="height:1px;border:none;color:#333;"> 

<h3 align="center">Backup/Restore is Stuck In Progress<a id="stuck"></a></h3>

  - If a backup or restore is stuck as "In Progress," then it is likely that the backup 
  or restore was interrupted. If this is the case, it cannot resume. 

  - For further details on your backup, run the command `velero backup describe <backup-name>`.
  And for more details on your restore, run `velero restore describe <backup-name>`.

  - You can delete the backup with the command `oc delete backup <backup-name> -n openshift-adp`,
  and delete the restore with the command `oc delete restore <restore-name> -n openshift-adp`



<hr style="height:1px;border:none;color:#333;">

<h3 align="center">Restic - NFS Volumes and rootSquash<a id="rootsquash"></a></h3>

- If using NFS volumes while `rootSquash` is enabled, Restic will be mapped to 
`nfsnobody` and not have the proper permissions to perform a backup/restore. 

#### To solve this issue:
  - Use supplemental groups, and apply this same supplemental group to the Restic
    daemonset.

### An example of using Restic supplemental groups in the Velero CR could look like this:

```
    apiVersion: oadp.openshift.io/v1alpha1
    kind: DataProtectionApplication
    metadata:
      name: dpa-sample
    spec:
      configuration:
        velero:
          defaultPlugins:
          - openshift
        restic:
          enable: true
          supplementalGroups:
            - 1234
```


<hr style="height:1px;border:none;color:#333;"> 

<h3 align="center">Issue with Backup/Restore of DeploymentConfig using Restic<a id="deployconfig"></a></h3>

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


<hr style="height:1px;border:none;color:#333;"> 

<h3 align="center">New Restic Backup Partially Failing After Clearing Bucket<a id="resbackup"></a></h3>

  After creating a backup for a stateful app using Restic on a given namespace, 
  clearing the bucket, and then creating a new backup again using Restic, the 
  backup will partially fail. 
  
  - Velero pod logs after attempting to backup the Mssql app using the steps 
  defined above:

  ```
  level=error msg="Error checking repository for stale locks" controller=restic-repo error="error running command=restic unlock --repo=s3:s3-us-east-1.amazonaws.com/<bucketname>/<prefix>/restic/mssql-persistent --password-file=/tmp/credentials/openshift-adp/velero-restic-credentials-repository-password --cache-dir=/scratch/.cache/restic, stdout=, stderr=Fatal: unable to open config file: Stat: The specified key does not exist.\nIs there a repository at the following location?\ns3:s3-us-east-1.amazonaws.com/<bucketname>/<prefix>/restic/mssql-persistent\n: exit status 1" error.file="/go/src/github.com/vmware-tanzu/velero/pkg/restic/repository_manager.go:293" error.function="github.com/vmware-tanzu/velero/pkg/restic.(*repositoryManager).exec" logSource="pkg/controller/restic_repository_controller.go:144" name=mssql-persistent-velero-sample-1-ckcj4 namespace=openshift-adp
  ```

  - Running the command `velero backup describe <backup-name> --details -n openshift-adp` 
  results in:

  ```
  Restic Backups:
  Failed:
    mssql-persistent/mssql-deployment-1-l7482: mssql-vol
  ```

  This is a known Velero [issue](https://github.com/vmware-tanzu/velero/issues/4421) 
  which appears to be in the process of deciding expected behavior. 
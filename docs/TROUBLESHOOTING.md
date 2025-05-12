# Troubleshooting 

If you need help, first search if there is [already an issue filed](https://issues.redhat.com/issues/?jql=project%20%3D%20OADP) 
  or please log into jira and create a new issue in the `OADP` project.

1. [OADP Cheat Sheet](oadp_cheat_sheet.md)
1. [OADP FAQ](https://access.redhat.com/articles/5456281)
1. [OADP Official Troubleshooting Documentation](https://docs.openshift.com/container-platform/latest/backup_and_restore/application_backup_and_restore/troubleshooting.html)
1. [OADP must-gather](https://docs.openshift.com/container-platform/latest/backup_and_restore/application_backup_and_restore/troubleshooting.html#migration-using-must-gather_oadp-troubleshooting)
1. [Debugging Failed Backups](#debugging-failed-backups)
1. [Debugging Failed Restores](#debugging-failed-restores)
1. [Debugging OpenShift Virtualization backup/restore](virtualization_troubleshooting.md)
1. [Deleting Backups](#deleting-backups)
1. [Debugging Data Mover (OADP 1.2 or below)](https://github.com/migtools/volume-snapshot-mover/blob/master/docs/troubleshooting.md)
1. [OpenShift ROSA STS and OADP installation](https://github.com/rh-mobb/documentation/blob/main/content/docs/misc/oadp/rosa-sts/_index.md)
1. [Common Issues and Misconfigurations](#common-issues-and-misconfigurations)
    - [Credentials Not Properly Formatted](#credentials-secret-not-properly-formatted)
    - [Errors in the Velero Pod](#errors-in-the-velero-pod)
    - [Errors in Backup Logs](#errors-in-backup-logs)
    - [Backup/Restore is Stuck In Progress](#backuprestore-is-stuck-in-progress)
    - [Restic - NFS Volumes and rootSquash](#restic---nfs-volumes-and-rootsquash)
    - [Issue with Backup/Restore of DeploymentConfig using Restic](#issue-with-backuprestore-of-deploymentconfig-using-restic)
    - [New Restic Backup Partially Failing After Clearing Bucket](#new-restic-backup-partially-failing-after-clearing-bucket)
    - [Restic Restore Partially Failing on OCP 4.14 Due to Changed PSA Policy](#restic-restore-partially-failing-on-ocp-414-due-to-changed-psa-policy)


## Debugging Failed Backups

1. OpenShift commands
    - Check for validation errors in the backup by running the following command,
    ```
    oc describe backup <backupName> -n openshift-adp
    ```
    - Check the Velero logs
    ```
    oc logs -f deploy/velero -n openshift-adp
    ```
    - If Data Mover (OADP 1.2 or below) is enabled, check the volume-snapshot-logs
    ```
    oc logs -f deployment.apps/volume-snapshot-mover -n openshift-adp
    ```
    
1. Velero commands
    -  Alias the velero command: 
    ```
    alias velero='oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero'
    ```
    - Get the backup details: 
    ```
    velero backup describe <backupName> --details
    ```
    - Get the backup logs: 
    ```
    velero backup logs <backupName>
    ```
1. Restic backup debug
    - Please refer to the [restic troubleshooting tips page](restic_troubleshooting.md)

1. Volume Snapshots debug
    - This guide has not yet been published

1. CSI Snapshots debug
    - This guide has not yet been published
    
    
## Debugging Failed Restores

This section includes how to debug a failed restore. For more specific issues related to restic/CSI/Volume snapshots check out the following section.

1. OpenShift commands
    - Check for validation errors in the backup by running the following command,
    ```
    oc describe restore <restoreName> -n openshift-adp
    ```
    - Check the Velero logs
    ```
    oc logs -f deployment.apps/velero -n openshift-adp
    ```
    If Data Mover (OADP 1.2 or below) is enabled, check the volume-snapshot-logs
    ```
    oc logs -f deployment.apps/volume-snapshot-mover -n openshift-adp
    ```
    
1. Velero commands
    - Alias the velero command: 
    ```
    alias velero='oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero'
    ```
    - Get the restore details: 
    ```
    velero restore describe <restoreName> --details
    ```
    - Get the backup logs: 
    ```
    velero backup logs <restoreName>
    ```
 
## Deleting Backups

A common misunderstanding is that `oc delete backup $backup_name` command will delete the backup and artifacts from object storage.  **This is not the case**.  The backup object will be deleted temporarily but will be recreated via Velero's sync controller.

To delete an OADP backup, the related objects and off cluster artifacts.

 1. OpenShift commands
    
    Create a DeleteBackupRequest object:
    ```yaml
    apiVersion: velero.io/v1
    kind: DeleteBackupRequest
    metadata:
      name: deletebackuprequest
      namespace: openshift-adp
    spec:
      backupName: <backupName>
    ```

  1. Velero commands:
      ```
      velero backup delete --help
      ```

      Delete the backup:  
      ```
      velero backup delete <backupName>
      ```

The related artifacts will be deleted at different times depending on the backup method:
* Restic:  Artifacts are deleted in the next full maintentance cycle after the backup is deleted.
* CSI: Artifacts are deleted immediately when the backup is deleted.
* Kopia: Artifacts are deleted after two full maintentance cycles after the backup is deleted.

**Note:** In OADP 1.3.x and OADP 1.4.x the full maintenance cycle executes once every 24 hours.

For more information on Restic and Kopia please refer to the following pages:
* [Restic Maintenance](restic_troubleshooting.md#maintenance)
* [Kopia Maintenance](kopia_troubleshooting.md#kopia-repository-maintenance)

Once a backup and related artifacts are deleted and no active backups related to the related namespace it is recommended to delete the backupRepository object.

```
oc get backuprepositories.velero.io -n openshift-adp
oc delete backuprepository <backupRepositoryName> -n openshift-adp
```

### Deleting completed maintenance jobs

```
oc delete pod --field-selector=status.phase==Succeeded -n openshift-adp
```

## Common Issues and Misconfigurations

### Credentials Secret Not Properly Formatted

  - Credentials:
    An example of correct AWS credentials:

    ```
    [<INSERT_PROFILE_NAME>]
    aws_access_key_id=<INSERT_VALUE>
    aws_secret_access_key=<INSERT_VALUE>
    ```

    *Note:* Do not use quotes while putting values in place of INSERT_VALUE Placeholders


### Errors in the Velero Pod

-  **Error:** `Backup storage contains invalid top-level directories: [someDirName]`

    **Problem:** your object storage root/prefix directory contains directories not 
    from velero's [approved list](https://github.com/vmware-tanzu/velero/blob/6f64052e94ef71c9d360863f341fe3c11e319f08/pkg/persistence/object_store_layout.go#L37-L43)

    **Solutions:**
    1. Define prefix directory inside a storage bucket where backups are to be uploaded instead of object storage root. In your Velero CR set a prefix for velero to use in:

    ```
    oc explain backupstoragelocations.velero.io.spec.objectStorage
    ```
    Example:
    ```
    objectStorage:
      bucket: your-bucket-name
      prefix: <DirName>
    ```

    2. Delete the offending directories from your object storage location.

  

### Errors in Backup Logs

-   **Error:** 
    `error getting volume info: rpc error: code = Unknown desc = InvalidVolume.NotFound: The volume ‘vol-xxxx’ does not exist.\n\tstatus code: 400`

    **Problem** 
    AWS PV and Volume snaphot location are in different regions.

    **Solution**
    Edit Velero `volume_snapshot_location` to the region specified in PV, and 
    change region in VolumeSnapshotLocation resource to the region mentioned in the PV, and then create a new backup.


### Backup/Restore is Stuck In Progress

  - If a backup or restore is stuck as "In Progress," then it is likely that the backup 
  or restore was interrupted. If this is the case, it cannot resume. 

  - For further details on your backup, run the command:
  ```
  velero backup describe <backup-name> --details  # --details is optional
  ```
  - For more details on your restore, run:
  ```
  velero restore describe <backup-name> --details  # --details is optional
  ```

  - You can delete the backup with the command: 
  ```
  velero backup delete <backupName>
  ```
  - You can delete the restore with the command: 
  ```
  velero delete restore <restoreName> 
  ```


### Restic - NFS Volumes and rootSquash

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
        nodeAgent:
          enable: true
          uploaderType: restic
          supplementalGroups:
            - 1234
```

### Issue with Backup/Restore of DeploymentConfig with volumes or restore hooks

-  (OADP 1.3+) **Error:** `DeploymentConfigs restore with spec.Replicas==0 or DC pods fail to restart if they crash if using DC with volumes or restore hooks`

    **Solution:**

    Solution is the same as in the (OADP 1.1+), except it applies to the use case if you are restoring DeploymentConfigs and have either volumes or post-restore hooks regardless of the backup method.

-  (OADP 1.1+) **Error:** `DeploymentConfigs restore with spec.Replicas==0 or DC pods fail to restart if they crash if using Restic/Kopia restores or restore hooks`

    **Solution:**
    
    This is expected behavior on restore if you are restoring DeploymentConfigs and are either using Restic or Kopia for volume restore or you have post-restore hooks. The pod and DC plugins make these modifications to ensure that Restic or Kopia and hooks work properly, and [dc-post-restore.sh](../docs/scripts/dc-post-restore.sh) should have been run immediately after a successful restore. Usage for this script is `dc-post-restore.sh <restore-name>`

-  (OADP 1.0.z) **Error:** `Using Restic as backup method causes PartiallyFailed/Failed errors in the Restore or post-restore hooks fail to execute`

    **Solution:**

    The changes in the backup/restore process for mitigating this error would be a two step restore process where, in the first step we would perform a restore excluding the replicationcontroller and deploymentconfig resources, and the second step would involve a restore including these resources. The backup and restore commands are given below for more clarity. (The examples given below are a use case for backup/restore of a target namespace, for other cases a similar strategy can be followed).

    Please note that this is a temporary fix for this issue and there are ongoing discussions to solve it.

    Step 1: Initiate the backup as any normal backup for restic.
    ```
    velero create backup <backup-name> -n openshift-adp --include-namespaces=<TARGET_NAMESPACE>
    ```

    Step 2: Initiate a restore excluding the replicationcontroller and deploymentconfig resources.
    ```
    velero restore create --from-backup=<BACKUP_NAME> -n openshift-adp --include-namespaces <TARGET_NAMESPACE> --exclude-resources replicationcontroller,deploymentconfig,templateinstances.template.openshift.io --restore-volumes=true
    ```

    Step 3: Initiate a restore including the replicationcontroller and deploymentconfig resources.
    ```
    velero restore create --from-backup=<BACKUP_NAME> -n openshift-adp --include-namespaces <TARGET_NAMESPACE> --include-resources replicationcontroller,deploymentconfig,templateinstances.template.openshift.io --restore-volumes=true
    ```

### New Restic Backup Partially Failing After Clearing Bucket

  After creating a backup for a stateful app using Restic on a given namespace, 
  clearing the bucket, and then creating a new backup again using Restic, the 
  backup will partially fail. 
  
  - Velero pod logs after attempting to backup the Mssql app using the steps 
  defined above:

  ```
  level=error msg="Error checking repository for stale locks" controller=restic-repo error="error running command=restic unlock --repo=s3:s3-us-east-1.amazonaws.com/<bucketname>/<prefix>/restic/mssql-persistent --password-file=/tmp/credentials/openshift-adp/velero-restic-credentials-repository-password --cache-dir=/scratch/.cache/restic, stdout=, stderr=Fatal: unable to open config file: Stat: The specified key does not exist.\nIs there a repository at the following location?\ns3:s3-us-east-1.amazonaws.com/<bucketname>/<prefix>/restic/mssql-persistent\n: exit status 1" error.file="/go/src/github.com/vmware-tanzu/velero/pkg/restic/repository_manager.go:293" error.function="github.com/vmware-tanzu/velero/pkg/restic.(*repositoryManager).exec" logSource="pkg/controller/restic_repository_controller.go:144" name=mssql-persistent-velero-sample-1-ckcj4 namespace=openshift-adp
  ```

  - Running the command:
  ```
  velero backup describe <backup-name> --details 
  ```
  results in:
  ```
  Restic Backups:
  Failed:
    mssql-persistent/mssql-deployment-1-l7482: mssql-vol
  ```

  This is a known Velero [issue](https://github.com/vmware-tanzu/velero/issues/4421) 
  which appears to be in the process of deciding expected behavior. 


### Restic Restore Partially Failing on OCP 4.14 Due to Changed PSA Policy 

 **Issue:** 
 OCP 4.14 enforces a Pod Security Admission (PSA) policy that can hinder the readiness of pods during a Restic restore process. If a Security Context Constraints (SCC) resource is not found during the creation of a pod, and the PSA policy on the pod is not aligned with the required standards, pod admission is denied. This issue arises due to the resource restore order of Velero.  
  - Sample error: 
  ```
  \"level=error\" in line#2273: time=\"2023-06-12T06:50:04Z\" level=error msg=\"error restoring mysql-869f9f44f6-tp5lv: pods \\\"mysql-869f9f44f6-tp5lv\\\" is forbidden: violates PodSecurity \\\"restricted:v1.24\\\": privileged (container \\\"mysql\\\" must not set securityContext.privileged=true), allowPrivilegeEscalation != false (containers \\\"restic-wait\\\", \\\"mysql\\\" must set securityContext.allowPrivilegeEscalation=false), unrestricted capabilities (containers \\\"restic-wait\\\", \\\"mysql\\\" must set securityContext.capabilities.drop=[\\\"ALL\\\"]), seccompProfile (pod or containers \\\"restic-wait\\\", \\\"mysql\\\" must set securityContext.seccompProfile.type to \\\"RuntimeDefault\\\" or \\\"Localhost\\\")\" logSource=\"/remote-source/velero/app/pkg/restore/restore.go:1388\" restore=openshift-adp/todolist-backup-0780518c-08ed-11ee-805c-0a580a80e92c\n velero container contains \"level=error\" in line#2447: time=\"2023-06-12T06:50:05Z\" level=error msg=\"Namespace todolist-mariadb, resource restore error: error restoring pods/todolist-mariadb/mysql-869f9f44f6-tp5lv: pods \\\"mysql-869f9f44f6-tp5lv\\\" is forbidden: violates PodSecurity \\\"restricted:v1.24\\\": privileged (container \\\"mysql\\\" must not set securityContext.privileged=true), allowPrivilegeEscalation != false (containers \\\"restic-wait\\\", \\\"mysql\\\" must set securityContext.allowPrivilegeEscalation=false), unrestricted capabilities (containers \\\"restic-wait\\\", \\\"mysql\\\" must set securityContext.capabilities.drop=[\\\"ALL\\\"]), seccompProfile (pod or containers \\\"restic-wait\\\", \\\"mysql\\\" must set securityContext.seccompProfile.type to \\\"RuntimeDefault\\\" or \\\"Localhost\\\")\" logSource=\"/remote-source/velero/app/pkg/controller/restore_controller.go:510\" restore=openshift-adp/todolist-backup-0780518c-08ed-11ee-805c-0a580a80e92c\n]", 
  ```

 **Solution:**
 Restore SCC before Pod. Set restore priority field on velero server to restore SCC before Pod.

  - Example DPA setting:
  ```
  $ oc get dpa -o yaml
   configuration:
      restic:
        enable: true
      velero:
        args:
          restore-resource-priorities: 'securitycontextconstraints,customresourcedefinitions,namespaces,storageclasses,volumesnapshotclass.snapshot.storage.k8s.io,volumesnapshotcontents.snapshot.storage.k8s.io,volumesnapshots.snapshot.storage.k8s.io,datauploads.velero.io,persistentvolumes,persistentvolumeclaims,serviceaccounts,secrets,configmaps,limitranges,pods,replicasets.apps,clusterclasses.cluster.x-k8s.io,endpoints,services,-,clusterbootstraps.run.tanzu.vmware.com,clusters.cluster.x-k8s.io,clusterresourcesets.addons.cluster.x-k8s.io'
        defaultPlugins:
        - gcp
        - openshift
  ```
  Please note that this is a temporary fix for this issue, and ongoing discussions are in progress to address it. Also, note that if have an existing restore resource priority list, make sure you combine that existing list with the complete list presented in the example above.
  
  - This error can occur regardless of the SCC if the application is not aligned with the security standards. Please ensure that the security standards for the application pods are aligned, as provided in the link below, to prevent deployment warnings.  
  https://access.redhat.com/solutions/7002730

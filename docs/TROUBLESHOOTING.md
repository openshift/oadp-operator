<h1 align="center">Troubleshooting<a id="troubleshooting"></a></h1>

If you need help, first search if there is [already an issue filed](https://issues.redhat.com/issues/?jql=project%20%3D%20OADP) 
  or please log into jira and create a new issue in the `OADP` project.

1. [OADP Cheat Sheet](oadp_cheat_sheet.md)
2. [Debugging Failed Backups](#backup)
3. [Debugging Failed Restores](#restore)
4. [Debugging Data Mover (OADP 1.2 or below)](https://github.com/migtools/volume-snapshot-mover/blob/master/docs/troubleshooting.md)
5. [Common Issues and Misconfigurations](#misconfig)
    1. [Credentials Not Properly Formatted](#creds)
    2. [Errors in the Velero Pod](#velpod)
    3. [Errors in Backup Logs](#backuplogs)
    4. [Backup/Restore is Stuck In Progress](#stuck)
    5. [Restic - NFS Volumes and rootSquash](#rootsquash)
    6. [Issue with Backup/Restore of DeploymentConfig using Restic](#deployconfig)
    7. [New Restic Backup Partially Failing After Clearing Bucket](#resbackup)
    8. [Restic Restore Partially Failing on OCP 4.14 Due to Changed PSA Policy](#psapolicy)
6. [OADP FAQ](#faq)
7. [OpenShift ROSA STS and OADP installation](https://github.com/rh-mobb/documentation/blob/main/content/docs/misc/oadp/rosa-sts/_index.md)


<hr style="height:1px;border:none;color:#333;">

<h1 align="center">Debugging Failed Backups<a id="backup"></a></h1>

This section includes steps to debug a failed backup. For more specific issues related to Restic/CSI/volume snapshots check out the following section. Both the OpenShift commands and native Velero commands are useful in troubleshooting.
1. OpenShift commands
    1. Check for validation errors in the backup by running the following command,
    ```
    oc describe backup <backupName> -n openshift-adp
    ```
    2. Check the Velero logs
    ```
    oc logs -f deploy/velero -n openshift-adp
    ```
    If Data Mover (OADP 1.2 or below) is enabled, check the volume-snapshot-logs
    ```
    oc logs -f deployment.apps/volume-snapshot-mover -n openshift-adp
    ```
    
2. Velero commands
    1. Alias the velero command: 
    ```
    alias velero='oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero'
    ```
    2. Get the backup details: 
    ```
    velero backup describe <backupName> --details
    ```
    3. Get the backup logs: 
    ```
    velero backup logs <backupName>
    ```
3. Restic backup debug
    1. Please refer to the [restic troubleshooting tips page](restic_troubleshooting.md)

4. Volume Snapshots debug
    1. This guide has not yet been published

5. CSI Snapshots debug
    1. This guide has not yet been published
    
    

<hr style="height:1px;border:none;color:#333;">

<h1 align="center">Debugging Failed Restores<a id="restore"></a></h1>

This section includes how to debug a failed restore. For more specific issues related to restic/CSI/Volume snapshots check out the following section.

1. OpenShift commands
    1. Check for validation errors in the backup by running the following command,
    ```
    oc describe restore <restoreName> -n openshift-adp
    ```
    2. Check the Velero logs
    ```
    oc logs -f deployment.apps/velero -n openshift-adp
    ```
    If Data Mover (OADP 1.2 or below) is enabled, check the volume-snapshot-logs
    ```
    oc logs -f deployment.apps/volume-snapshot-mover -n openshift-adp
    ```
    
2. Velero commands
    1. Alias the velero command: 
    ```
    alias velero='oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero'
    ```
    2. Get the restore details: 
    ```
    velero restore describe <restoreName> --details
    ```
    3. Get the backup logs: 
    ```
    velero backup logs <restoreName>
    ```
 
<hr style="height:1px;border:none;color:#333;">

<h1 align="center">Common Issues and Misconfigurations<a id="misconfig"></a></h1>

<h3 align="center">Credentials Secret Not Properly Formatted<a id="creds"></a></h3>

  - Credentials:
    An example of correct AWS credentials:

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
        nodeAgent:
          enable: true
          uploaderType: restic
          supplementalGroups:
            - 1234
```


<hr style="height:1px;border:none;color:#333;"> 

<h3 align="center">Issue with Backup/Restore of DeploymentConfig with volumes or restore hooks<a id="deployconfig"></a></h3>

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


<hr style="height:1px;border:none;color:#333;">

<h3 align="center">Restic Restore Partially Failing on OCP 4.14 Due to Changed PSA Policy<a id="psapolicy"></a></h3>

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

<hr style="height:1px;border:none;color:#333;"> 
<h1 align="center">OADP FAQ<a id="faq"></a></h1>

The OADP team maintains a [FAQ page](https://access.redhat.com/articles/5456281)

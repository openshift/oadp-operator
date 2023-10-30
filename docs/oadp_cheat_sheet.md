## Cheat sheet
# OADP commands
This cheat sheet presents a list of command-line executables that are frequently used by OpenShift Administrators using OADP. The commands are organized by category.

You may also find OpenShift cheat [here](https://access.cdn.redhat.com/content/origin/files/sha256/e1/e1410185092472c9a943b85cd6b60196f3938ffa8d650026818d5456e66e01c1/openshift_cheat_sheet_r5v1.pdf?_auth_=1687811315_27797f50bfbfc2691c084156de530f76) helpful.

## OpenShift commands:

#### List your DPA - DataProtectionApplication configuration
List:
```
oc get dpa -n openshift-adp
```
Get the details:
```
oc get dpa dpaname -n openshift-adp -o yaml
```

#### Create a backup
Update the following backup.yaml CR
```yaml
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: replace_me_backup_name
  namespace: openshift-adp
spec:
  includedNamespaces:
  - replace_me_namespace_1
  - replace_me_namespace_2
  storageLocation: dpa_replace_me
  ttl: 720h0m0s
```
Create the backup
```
oc create -f backup.yaml
```

#### Create a restore
Update the following restore.yaml CR
```yaml
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: replace_me_restore_name
  namespace: openshift-adp
spec:
  backupName: replace_me_backup_name
  restorePVs: true
```

#### Get the definitions, parameters and options
Get OADP CRD's
```
oc get crd | grep oadp
```

Example with the `dataprotectionapplications.oadp.openshift.io` CRD
```
oc explain dataprotectionapplications.oadp.openshift.io
oc explain dataprotectionapplications.oadp.openshift.io.spec.features.dataMover
```

## Velero commands:

## Enable Velero and Restic Cli
```
alias velero='oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero'
alias restic='oc -n openshift-adp exec deployment/velero -c velero -it -- /usr/bin/restic'
```

#### Enable Velero Shell completion

##### Bash
Linux:
```
velero completion bash > /etc/bash_completion.d/velero
```
MacOS:
```
velero completion bash > /usr/local/etc/bash_completion.d/velero
```

### List Backups
```
velero backup get
```

## Create a Backup 

#### Backup with Defaults
```
velero backup create backup $backup_name --include-namespaces $namespace
```

#### Backup using restic for PV data
```
velero backup create backup $backup_name --include-namespaces $namespace --default-volumes-to-fs-backup
```

#### Create a backup excluding the velero and default namespaces.
```  
velero backup create $backup_name --exclude-namespaces velero,default
```

#### Create a backup based on a schedule named daily-backup.
```
velero backup create --from-schedule daily-backup
```

#### View the YAML for a backup that doesn't snapshot volumes, without sending it to the server.
```
velero backup create $backup_name --snapshot-volumes=false -o yaml
```

#### Wait for a backup to complete before returning from the command.
```
velero backup create $backup_name --wait
```

#### Delete a Backup
```
velero backup delete $backup_name
```

## Debug a Failed or Partially Failed Backup

Two simple steps should provide the information required

#### Logs
```
velero backup logs $backup_name
```

#### Describe
```
velero backup describe $backup_name --details
```

```
Name:         mssql-persistent
Namespace:    openshift-adp
Labels:       velero.io/storage-location=velero-sample-1
Annotations:  velero.io/source-cluster-k8s-gitversion=v1.23.5+b0357ed
              velero.io/source-cluster-k8s-major-version=1
              velero.io/source-cluster-k8s-minor-version=23

Phase:  (run `velero backup logs mssql-persistent` for more information)

Errors:    2
Warnings:  0

Namespaces:
  Included:  mssql-persistent
  Excluded:  <none>

Resources:
  Included:        *
  Excluded:        <none>
  Cluster-scoped:  auto

Label selector:  <none>

Storage Location:  velero-sample-1

Velero-Native Snapshot PVs:  auto

TTL:  720h0m0s

Hooks:  <none>

Backup Format Version:  1.1.0

Started:    2023-06-21 23:56:51 -0700 PDT
Completed:  2023-06-22 00:07:51 -0700 PDT

Expiration:  2023-07-21 23:56:50 -0700 PDT

Resource List:
  apiextensions.k8s.io/v1/CustomResourceDefinition:
    - clusterserviceversions.operators.coreos.com
    <SNIP>
    - mssql-persistent/mssql-app-service
    - mssql-persistent/mssql-service
  v1/ServiceAccount:
    - mssql-persistent/builder
    - mssql-persistent/default
    - mssql-persistent/deployer
    - mssql-persistent/mssql-persistent-sa

Velero-Native Snapshots: <none included>
```



## Data Mover (OADP 1.2 or below) Specific commands

#### Clean up datamover related objects
**WARNING** Do not run this command on production systems.  This is a remove *ALL* command.
```
oc delete vsb -A --all; oc delete vsr -A --all; oc delete vsc -A --all; oc delete vs -A --all; oc delete replicationsources.volsync.backube -A --all; oc delete replicationdestination.volsync.backube -A --all
```
Details:
```
--all=false:
	Delete all resources, in the namespace of the specified resource types.
```
```
-A, --all-namespaces=false:
	If present, list the requested object(s) across all namespaces. Namespace in current context is ignored even
	if specified with --namespace.
```
A safer to execute a cleanup is to limit the delete to a namespace or a specific object.
* namespaced objecs: VSB, VSR, VSC, VS
* protected namespace (openshift-adp): replicationsources.volsync.backube, replicationdestination.volsync.backube

```
oc delete vsb -n <namespace> --all
```



#### Remove finalizers
```
for i in `oc get vsc -A -o custom-columns=NAME:.metadata.name`; do echo $i; oc patch vsc $i -p '{"metadata":{"finalizers":null}}' --type=merge; done
```

#### Watch datamover resources while backup in progress
```
curl -o ~/.local/bin/datamover_resources.sh https://raw.githubusercontent.com/openshift/oadp-operator/master/docs/examples/datamover_resources.sh
```
###### Backups
```
watch -n 5 datamover_resources.sh -b -d
```
###### Restore
```
watch -n 5 datamover_resources.sh -r -d
```

#### Watch the VSM plugin logs
```
oc logs -f deployment.apps/volume-snapshot-mover -n openshift-adp
```

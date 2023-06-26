## GOAL
use as needed, publish to https://developers.redhat.com/cheat-sheets/

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
```
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
```
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

#### Backup with [Restic, Kopia]
```
velero backup create backup $backup_name --include-namespaces $namespace --default-volumes-to-fs-backup
```

#### Create a backup excluding the velero and default namespaces.
```  
velero backup create $backup_name --exclude-namespaces velero,default
```

#### Create a backup based on a schedule named daily-backup.
```
velero backup create --from-schedule $backup_name
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
velero backup logs $backup
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

Phase:  [31mPartiallyFailed[0m (run `velero backup logs mssql-persistent` for more information)

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



## Data Mover Specific commands

#### Clean up datamover related objects
```
oc delete vsb -A --all; oc delete vsr -A --all; oc delete vsc -A --all; oc delete vs -A --all; oc delete replicationsources.volsync.backube -A --all; oc delete replicationdestination.volsync.backube -A --all
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
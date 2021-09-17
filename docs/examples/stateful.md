<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Stateful Application Backup/Restore - MSSQL</h1>
<h2 align="center">Using an AWS s3 Bucket and AWS EBS Snapshot</h2>
<hr style="height:1px;border:none;color:#333;">

### Prerequisites
* Make sure the OADP operator is installed:

    `make deploy`

* Create a credentials secret for AWS:

   `oc create secret generic cloud-credentials --namespace oadp-operator-system --from-file cloud=<CREDENTIALS_FILE_PATH>`

* Make sure your Velero CR is similar to this:

    ```
    apiVersion: oadp.openshift.io/v1alpha1
    kind: Velero
    metadata:
      name: velero-sample
    spec:
      olmManaged: false
      backupStorageLocations:
      - provider: aws
        default: true
        objectStorage:
          bucket: my-bucket
        credential:
          name: cloud-credentials
          key: cloud    
        config:
          region: us-east-1
          profile: default
      volumeSnapshotLocations:
      - provider: aws
        config:
          region: us-west-2
      enableRestic: true
      defaultVeleroPlugins:
      - openshift
      - aws
    ```
    *Note*: Your BSL region should be the same as your s3 bucket, and your
            VSL region should be your cluster's region. 

* Install Velero + Restic:

  `oc create -n oadp-operator-system -f config/samples/oadp_v1alpha1_velero.yaml`

<hr style="height:1px;border:none;color:#333;">

### Create the Mssql deployment config:

`oc create -f docs/examples/manifests/mssql-template.yaml`

This example will create the following resources:
* **Namespace:** 
* **Secret:** 
* **Service:** 
* **Route:** 
* **PersistentVolumeClaim:** 
* **Deployment:** 

### Verify application resources:

`oc get all -n mssql-persistent`

Should look similar to this:

```
NAME                                        READY   STATUS      RESTARTS   AGE
pod/mssql-app-deployment-6dbc8d5b64-nlmhc   1/1     Running     0          85s
pod/mssql-deployment-1-deploy               0/1     Completed   0          85s
pod/mssql-deployment-1-qfh7c                1/1     Running     0          79s

NAME                                       DESIRED   CURRENT   READY   AGE
replicationcontroller/mssql-deployment-1   1         1         1       85s

NAME                        TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
service/mssql-app-service   ClusterIP   172.30.8.249     <none>        5000/TCP   85s
service/mssql-service       ClusterIP   172.30.200.254   <none>        1433/TCP   85s

NAME                                   READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/mssql-app-deployment   1/1     1            1           85s

NAME                                              DESIRED   CURRENT   READY   AGE
replicaset.apps/mssql-app-deployment-6dbc8d5b64   1         1         1       85s

NAME                                                  REVISION   DESIRED   CURRENT   TRIGGERED BY
deploymentconfig.apps.openshift.io/mssql-deployment   1          1         1         config
```

### Add data to application

Visit the route location provided in the `HOST/PORT` section following this command:

`oc get routes -n mssql-persistent`

Here you will see a table of data. Enter additional data and save.
Once completed, it's time to begin a backup.

### Create application backup

`oc create -f docs/examples/manifests/mssql-backup.yaml`

### Verify the backup is completed

`oc get backup -n oadp-operator-system mssql-persistent -o jsonpath='{.status.phase}'`

should result in `Completed`

### Delete the application

Once we have ensured the backup is completed, we want to test the restore 
process. First, delete the `mssql-persistent` project:

`oc delete namespace mssql-persistent`

### Create the restore for the application

`oc create -f docs/examples/manifests/mssql-restore.yaml`

### Verify the restore is completed

`oc get restore -n oadp-operator-system mssql-persistent -o jsonpath='{.status.phase}'`

Should result in `Completed`

### Verify all resources have been recreated in the restore process

`oc get all -n mssql-persistent`

Should look similar to this:

```
NAME                                        READY   STATUS      RESTARTS   AGE
pod/mssql-app-deployment-6dbc8d5b64-mx22j   1/1     Running     0          7m3s
pod/mssql-deployment-1-deploy               0/1     Completed   0          7m3s
pod/mssql-deployment-1-pzx6k                1/1     Running     0          7m

NAME                                       DESIRED   CURRENT   READY   AGE
replicationcontroller/mssql-deployment-1   1         1         1       7m3s

NAME                        TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
service/mssql-app-service   ClusterIP   172.30.81.38     <none>        5000/TCP   7m
service/mssql-service       ClusterIP   172.30.221.119   <none>        1433/TCP   7m

NAME                                   READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/mssql-app-deployment   1/1     1            1           7m3s

NAME                                              DESIRED   CURRENT   READY   AGE
replicaset.apps/mssql-app-deployment-6dbc8d5b64   1         1         1       7m4s

NAME                                                  REVISION   DESIRED   CURRENT   TRIGGERED BY
deploymentconfig.apps.openshift.io/mssql-deployment   1          1         1         config

```

### Verify the data previously entered in the mssql table 

`oc get routes -n mssql-persistent`

Check the data in the table previously entered is present.

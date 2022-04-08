<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Stateful Application Backup/Restore - MySQL</h1>
<h2 align="center">Using an AWS s3 Bucket and AWS EBS Snapshot</h2>

### Prerequisites
* OADP operator, a credentials secret, and a DataProtectionApplication (DPA) CR
  are all created. Follow [these steps](/docs/install_olm.md) for installation instructions.

  - Make sure your DPA CR is similar to below in the install step.

* Information on `backupLocations` and `snapshotLocations` specs
  can be found [here](/docs/config/bsl_and_vsl.md).


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
          - aws
        restic:
          enable: true
      backupLocations:
        - name: default
          velero:
            provider: aws
            default: true
            objectStorage:
              bucket: my-bucket
              prefix: my-prefix
            config:
              region: us-east-1
              profile: "default"
            credential:
              name: cloud-credentials
              key: cloud
      snapshotLocations:
        - name: default
          velero:
            provider: aws
            config:
              region: us-west-2
              profile: "default"

    ```

<hr style="height:1px;border:none;color:#333;">

### Create the MySQL deployment config:

`oc create -f docs/examples/manifests/mysql/mysql-persistent-template.yaml`

This example will create the following resources:
* **Namespace**
* **Secret**
* **Service**
* **Route**
* **PersistentVolumeClaim**
* **Deployment**

### Verify application resources:

`oc get all -n mysql-persistent`

Should look similar to this:

```
NAME                         READY     STATUS      RESTARTS   AGE
pod/mysql-6bb6964964-x4s8d   1/1       Running     0          54s
pod/todolist-1-59jqk         1/1       Running     0          51s
pod/todolist-1-deploy        0/1       Completed   0          54s

NAME                               DESIRED   CURRENT   READY     AGE
replicationcontroller/todolist-1   1         1         1         54s

NAME               TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
service/mysql      ClusterIP   172.30.73.117   <none>        3306/TCP   55s
service/todolist   ClusterIP   172.30.92.118   <none>        8000/TCP   55s

NAME                    READY     UP-TO-DATE   AVAILABLE   AGE
deployment.apps/mysql   1/1       1            1           55s

NAME                               DESIRED   CURRENT   READY     AGE
replicaset.apps/mysql-6bb6964964   1         1         1         55s

NAME                                          REVISION   DESIRED   CURRENT   TRIGGERED BY
deploymentconfig.apps.openshift.io/todolist   1          1         1         config
```

### Add data to application

Visit the route location provided in the `HOST/PORT` section following this command:

`oc get routes -n mysql-persistent`

Here you will see a table of data. Enter additional data and save.
Once completed, it's time to begin a backup.

### Create application backup

`oc create -f docs/examples/manifests/mysql/mysql-backup.yaml`

### Verify the backup is completed

`oc get backup -n openshift-adp mysql-persistent -o jsonpath='{.status.phase}'`

should result in `Completed`

### Delete the application

Once we have ensured the backup is completed, we want to test the restore
process. First, delete the `mysql-persistent` project:

`oc delete namespace mysql-persistent`

### Create the restore for the application

`oc create -f docs/examples/manifests/mysql/mysql-restore.yaml`

### Verify the restore is completed

`oc get restore -n openshift-adp mysql-persistent -o jsonpath='{.status.phase}'`

Should result in `Completed`

### Verify all resources have been recreated in the restore process

`oc get all -n mysql-persistent`

Should look similar to this:

```
NAME                         READY     STATUS      RESTARTS   AGE
pod/mysql-6bb6964964-x4s8d   1/1       Running     0          54s
pod/todolist-1-59jqk         1/1       Running     0          51s
pod/todolist-1-deploy        0/1       Completed   0          54s

NAME                               DESIRED   CURRENT   READY     AGE
replicationcontroller/todolist-1   1         1         1         54s

NAME               TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
service/mysql      ClusterIP   172.30.73.117   <none>        3306/TCP   55s
service/todolist   ClusterIP   172.30.92.118   <none>        8000/TCP   55s

NAME                    READY     UP-TO-DATE   AVAILABLE   AGE
deployment.apps/mysql   1/1       1            1           55s

NAME                               DESIRED   CURRENT   READY     AGE
replicaset.apps/mysql-6bb6964964   1         1         1         55s

NAME                                          REVISION   DESIRED   CURRENT   TRIGGERED BY
deploymentconfig.apps.openshift.io/todolist   1          1         1         config
```

### Verify the data previously entered in the mysql table

`oc get routes -n mysql-persistent`

Check the data in the table previously entered is present.

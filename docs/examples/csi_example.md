<h1 align="center">Stateful Application Backup/Restore - MSSQL</h1>
<h2 align="center">CSI Volume Snapshotting with AWS EBS</h2>

### Prerequisites
* OADP operator, a credentials secret, and a DataProtectionApplication (DPA) CR 
  are all created. Follow [these steps](/docs/install_olm.md) for installation instructions.

  - Make sure your DPA CR is similar to below in the install step. 
    Note the `EnableCSI` feature flag and the `csi` default plugin.

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
          - csi
        restic:
          enable: false
        featureFlags:
          - EnableCSI
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

### Create a StorageClass and VolumeShapshotClass:

- A `StorageClass` and a `VolumeSnapshotClass` are needed before the Mssql application 
is created. The app will map to the `StorageClass`, which contains information about the CSI driver. 

- Include a label in `VolumeSnapshotClass` to let 
Velero know which to use, and set `deletionPolicy` to  `Retain` in order for
`VolumeSnapshotContent` to remain after the application namespace is deleted.

`oc create -f docs/examples/manifests/mssql/VolumeSnapshotClass.yaml`

```
apiVersion: v1
kind: List
items:
  - apiVersion: snapshot.storage.k8s.io/v1
    kind: VolumeSnapshotClass
    metadata:
      name: example-snapclass
      labels:
        velero.io/csi-volumesnapshot-class: 'true'
      annotations:
        snapshot.storage.kubernetes.io/is-default-class: 'true'
    driver: ebs.csi.aws.com
    deletionPolicy: Retain
```

`gp2-csi` comes as a default `StorageClass` with OpenShift clusters. 

`oc get storageclass` 

If this is not found, create a `StorageClass` like below:

```
apiVersion: storage.k8s.io/v1 
kind: StorageClass 
metadata: 
  name: gp2-csi
  annotations:
    storageclass.kubernetes.io/is-default-class: 'true'
provisioner: ebs.csi.aws.com
parameters: 
  type: gp2
reclaimPolicy: Delete 
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
```

<hr style="height:1px;border:none;color:#333;">

### Create the Mssql deployment config:

`oc create -f docs/examples/manifests/mssql/csi-mssql-template.yaml`

This example will create the following resources:
* **Namespace** 
* **Secret** 
* **Service** 
* **Route** 
* **PersistentVolumeClaim** 
* **Deployment** 


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

`oc create -f docs/examples/manifests/mssql/mssql-backup.yaml`


### Verify the backup is completed

`oc get backup -n openshift-adp mssql-persistent -o jsonpath='{.status.phase}'`
should result in `Completed`

Once completed, you should now be able to see a namespace-scoped `VolumeSnapshot`:

`oc get volumesnapshot -n mssql-persistent`

```
NAME                     READYTOUSE   SOURCEPVC   SOURCESNAPSHOTCONTENT   RESTORESIZE   SNAPSHOTCLASS       SNAPSHOTCONTENT                                    CREATIONTIME   AGE
velero-mssql-pvc-kxhqc   true         mssql-pvc                           10Gi          example-snapclass   snapcontent-ec0b296b-550d-4669-9eff-8fcc44f46ae2   80s            81s
```


### Delete the application

Because `VolumeSnapshotContent` is cluster-scoped, it will remain after the
application is deleted since we set the `deletionPolicy` to `Retain` in the
`VolumeSnapshotClass`. We can make sure the `VolumeSnapshotContent` is ready first:

`oc get volumesnapshotcontent`

```
NAME                                               READYTOUSE   RESTORESIZE   DELETIONPOLICY   DRIVER            VOLUMESNAPSHOTCLASS   VOLUMESNAPSHOT           AGE
snapcontent-28527e2d-21bf-471a-ac62-044ecf8113e3   true         10737418240   Retain           ebs.csi.aws.com   example-snapclass     velero-mssql-pvc-vnvvr   103m
velero-velero-mssql-pvc-vnvvr-4q5jl                true         10737418240   Retain           ebs.csi.aws.com   example-snapclass     velero-mssql-pvc-vnvvr   87m
```

Once we have ensured the backup is completed and `VolumeSnapshotContent` is 
ready, we want to test the restore process. First, delete the `mssql-persistent` project:

`oc delete namespace mssql-persistent`


### Create the restore for the application

`oc create -f docs/examples/manifests/mssql/mssql-restore.yaml`


### Verify the restore is completed

`oc get restore -n openshift-adp mssql-persistent -o jsonpath='{.status.phase}'`

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
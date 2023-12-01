<h1 align="center">Stateful Application Backup/Restore - MySQL</h1>
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
        nodeAgent:
          enable: false
          uploaderType: restic #[restic, kopia]
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

### StorageClass and VolumeShapshotClass Requirements:

- A `StorageClass` and a `VolumeSnapshotClass` are needed before the mysql or any application
with persistent data is created. The app will map to the `StorageClass`, which contains information about the CSI driver.

- Most VolumeSnapshotClass specifications can not be modified as they are managed by an operator. 
  - A new VolumeSnapshotClass can be created with the correct labels and annotations.
  - A label and annotation can be added to an existing vsc via the following command: 
    - `oc label volumesnapshotclass <vsc> velero.io/csi-volumesnapshot-class=true`
    - `oc annotate volumesnapshotclass <vsc> snapshot.storage.kubernetes.io/is-default-class=true --overwrite`

- Include a label in `VolumeSnapshotClass` to let
Velero know which to use, and set `deletionPolicy` to  `Retain` in order for
`VolumeSnapshotContent` to remain after the application namespace is deleted.

`oc create -f docs/examples/manifests/mysql/VolumeSnapshotClass.yaml`

```
apiVersion: snapshot.storage.k8s.io/v1
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

In most cases storage classes would have already been created in the cluster by default.  If that is not the case, please create a `StorageClass` like below:

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

### Create the mysql deployment config:

`oc create -f docs/examples/manifests/mysql/mysql-persistent-csi-template.yaml`
`oc create -f docs/examples/manifests/mysql/pvc/aws.yaml`

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

Once completed, you should now be able to see a namespace-scoped `VolumeSnapshot`:

`oc get volumesnapshot -n mysql-persistent`

```
NAME                     READYTOUSE   SOURCEPVC   SOURCESNAPSHOTCONTENT   RESTORESIZE   SNAPSHOTCLASS       SNAPSHOTCONTENT                                    CREATIONTIME   AGE
velero-mysql-pvc-kxhqc   true         mysql-pvc                           10Gi          example-snapclass   snapcontent-ec0b296b-550d-4669-9eff-8fcc44f46ae2   80s            81s
```


### Delete the application

Because `VolumeSnapshotContent` is cluster-scoped, it will remain after the
application is deleted since we set the `deletionPolicy` to `Retain` in the
`VolumeSnapshotClass`. We can make sure the `VolumeSnapshotContent` is ready first:

`oc get volumesnapshotcontent`

```
NAME                                               READYTOUSE   RESTORESIZE   DELETIONPOLICY   DRIVER            VOLUMESNAPSHOTCLASS   VOLUMESNAPSHOT           AGE
snapcontent-28527e2d-21bf-471a-ac62-044ecf8113e3   true         10737418240   Retain           ebs.csi.aws.com   example-snapclass     velero-mysql-pvc-vnvvr   103m
velero-velero-mysql-pvc-vnvvr-4q5jl                true         10737418240   Retain           ebs.csi.aws.com   example-snapclass     velero-mysql-pvc-vnvvr   87m
```

Once we have ensured the backup is completed and `VolumeSnapshotContent` is
ready, we want to test the restore process. First, delete the `mysql-persistent` project:

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

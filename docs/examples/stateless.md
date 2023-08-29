<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Stateless Application Backup/Restore - Nginx</h1>
<h2 align="center">Using an AWS s3 Bucket</h2>

### Prerequisites
* OADP operator, a credentials secret, and a DataProtectionApplication (DPA) CR 
  are all created. Follow [these steps](/docs/install_olm.md) for installation instructions.

  - Make sure your DPA CR is similar to below in the install step. 

* Information on `backupLocations` spec can be found [here](/docs/config/bsl_and_vsl.md). 

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
    nodeAgent:
      enable: true
      uploaderType: restic
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
```

<hr style="height:1px;border:none;color:#333;">

### Create the Nginx deployment:

`oc create -f docs/examples/manifests/nginx/nginx-deployment.yaml`

This will create the following resources:
* **Namespace**
* **Deployment**
* **Service**
* **Route**

### Verify Nginx deployment resources:

`oc get all -n nginx-example`

Should look similar to this:

```
$ oc get all -n nginx-example
NAME                                    READY   STATUS    RESTARTS   AGE
pod/nginx-deployment-55ddb59f4c-bls2x   1/1     Running   0          2m9s
pod/nginx-deployment-55ddb59f4c-cqjw8   1/1     Running   0          2m9s

NAME               TYPE           CLUSTER-IP      EXTERNAL-IP                                                               PORT(S)          AGE
service/my-nginx   LoadBalancer   172.30.46.198   aef02efae2e95444eaeef61c92dbc441-1447257770.us-east-2.elb.amazonaws.com   8080:30193/TCP   2m10s

NAME                               READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/nginx-deployment   2/2     2            2           2m10s

NAME                                          DESIRED   CURRENT   READY   AGE
replicaset.apps/nginx-deployment-55ddb59f4c   2         2         2       2m10s

NAME                                HOST/PORT                                                              PATH   SERVICES   PORT   WILDCARD
route.route.openshift.io/my-nginx   my-nginx-nginx-example.apps.cluster-da0d.da0d.sandbox591.opentlc.com          my-nginx   8080   None
```

### Create application backup

`oc create -f docs/examples/manifests/nginx/nginx-stateless-backup.yaml`

### Verify backup is completed

`oc get backup -n openshift-adp nginx-stateless -o jsonpath='{.status.phase}'`

should result in `Completed`


### Delete the Nginx application

Once we have ensured the backup is completed, we want to test the restore
process. First, delete the `nginx-example` project:

`oc delete namespace nginx-example`

### Create the restore for the application

`oc create -f docs/examples/manifests/nginx/nginx-stateless-restore.yaml`

### Ensure the restore has completed

`oc get restore -n openshift-adp nginx-stateless -o jsonpath='{.status.phase}'`

Should result in `Completed`

### Verify resources have been recreated in the restore process

`oc get all -n nginx-example`

Should look similar to this:

```
NAME                                    READY   STATUS    RESTARTS   AGE
pod/nginx-deployment-55ddb59f4c-7dbw7   1/1     Running   0          77s
pod/nginx-deployment-55ddb59f4c-gldml   1/1     Running   0          77s

NAME               TYPE           CLUSTER-IP       EXTERNAL-IP                                                               PORT(S)          AGE
service/my-nginx   LoadBalancer   172.30.158.248   ab58ecf4d417a432792de1219cd3f054-1995587190.us-east-2.elb.amazonaws.com   8080:32036/TCP   76s

NAME                               READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/nginx-deployment   2/2     2            2           77s

NAME                                          DESIRED   CURRENT   READY   AGE
replicaset.apps/nginx-deployment-55ddb59f4c   2         2         2       77s

NAME                                HOST/PORT                                                              PATH   SERVICES   PORT   WILDCARD
route.route.openshift.io/my-nginx   my-nginx-nginx-example.apps.cluster-da0d.da0d.sandbox591.opentlc.com          my-nginx   8080   None
```

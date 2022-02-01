<h1 align="center">Install OADP Operator using OperatorHub</h1>

### Install the OADP Operator
You can install the OADP Operator from the Openshift's OperatorHub. 
You can search for the operator using keywords such as `oadp` or `velero`.

![OADP-OLM-1](/docs/images/OADP-OLM-1.png)

Now click on `Install`

![OADP-OLM-1](/docs/images/click-install.png)

Finally, click on `Install` again. This will create namespace `openshift-adp` 
if it does not exist, and install the OADP operator in it.

![OADP-OLM-1](/docs/images/click-install-again.png)

### Create credentials secret
Before creating a DataProtectionApplication (DPA) CR, ensure you have created a secret
 `cloud-credentials` in namespace `openshift-adp`.

 Make sure your credentials file is in the proper format. For example, if using
 AWS, it should look like:

  ```
  [<INSERT_PROFILE_NAME>]
  aws_access_key_id=<INSERT_VALUE>
  aws_secret_access_key=<INSERT_VALUE>
  ```
  *Note:* Do not use quotes while putting values in place of INSERT_VALUE Placeholders

#### Create the secret:

 ```
$ oc create secret generic cloud-credentials --namespace openshift-adp --from-file cloud=<CREDENTIALS_FILE_PATH>
```

### Create the DataProtectionApplication Custom Resource

Create an instance of the DataProtectionApplication (DPA) CR by clicking on `Create Instance` as highlighted below:

![Velero-CR-1](/docs/images/dpa-cr.png)

The Velero instance can be created by selecting configurations using the OCP Web UI or by using a YAML file as mentioned below.

Finally, set the CR spec values appropriately, and click on `Create`.

The CR values are mentioned for ease of use. Please remember to mention `default: true` in backupStorageLocations if you intend on using the default backup storage location as shown below.

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

![Velero-CR-2](/docs/images/create-dpa-cr-yaml.png)

### Verify install

To verify all of the correct resources have been created, the following command
`oc get all -n openshift-adp` should look similar to:

```
NAME                                                     READY   STATUS    RESTARTS   AGE
pod/oadp-operator-controller-manager-67d9494d47-6l8z8    2/2     Running   0          2m8s
pod/oadp-velero-sample-1-aws-registry-5d6968cbdd-d5w9k   1/1     Running   0          95s
pod/restic-9cq4q                                         1/1     Running   0          94s
pod/restic-m4lts                                         1/1     Running   0          94s
pod/restic-pv4kr                                         1/1     Running   0          95s
pod/velero-588db7f655-n842v                              1/1     Running   0          95s

NAME                                                       TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
service/oadp-operator-controller-manager-metrics-service   ClusterIP   172.30.70.140    <none>        8443/TCP   2m8s
service/oadp-velero-sample-1-aws-registry-svc              ClusterIP   172.30.130.230   <none>        5000/TCP   95s

NAME                    DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
daemonset.apps/restic   3         3         3       3            3           <none>          96s

NAME                                                READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/oadp-operator-controller-manager    1/1     1            1           2m9s
deployment.apps/oadp-velero-sample-1-aws-registry   1/1     1            1           96s
deployment.apps/velero                              1/1     1            1           96s

NAME                                                           DESIRED   CURRENT   READY   AGE
replicaset.apps/oadp-operator-controller-manager-67d9494d47    1         1         1       2m9s
replicaset.apps/oadp-velero-sample-1-aws-registry-5d6968cbdd   1         1         1       96s
replicaset.apps/velero-588db7f655                              1         1         1       96s
```

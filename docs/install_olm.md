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
    nodeAgent:
      enable: true
      uploaderType: restic #[restic, kopia]
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

* Verify the DPA has been reconciled successfully:

  ```
  oc get dpa dpa-sample -n openshift-adp -o jsonpath='{.status}'
  ```

  Example Output:
  ```
  {"conditions":[{"lastTransitionTime":"2023-10-27T01:23:57Z","message":"Reconcile complete","reason":"Complete","status":"True","type":"Reconciled"}]}
  ```

  **Note**: the `type` is set to `Reconciled` and `status` is set to `True`.

* To verify all of the correct resources have been created, the following command
`oc get all -n openshift-adp` should look similar to:
  ```
  NAME                                                    READY   STATUS    RESTARTS   AGE
  pod/node-agent-9pjz9                                    1/1     Running   0          3d17h
  pod/node-agent-fmn84                                    1/1     Running   0          3d17h
  pod/node-agent-xw2dg                                    1/1     Running   0          3d17h
  pod/openshift-adp-controller-manager-76b8bc8d7b-kgkcw   1/1     Running   0          3d17h
  pod/velero-64475b8c5b-nh2qc                             1/1     Running   0          3d17h

  NAME                                                       TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
  service/openshift-adp-controller-manager-metrics-service   ClusterIP   172.30.194.192   <none>        8443/TCP   3d17h
  service/openshift-adp-velero-metrics-svc                   ClusterIP   172.30.190.174   <none>        8085/TCP   3d17h

  NAME                        DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
  daemonset.apps/node-agent   3         3         3       3            3           <none>          3d17h

  NAME                                               READY   UP-TO-DATE   AVAILABLE   AGE
  deployment.apps/openshift-adp-controller-manager   1/1     1            1           3d17h
  deployment.apps/velero                             1/1     1            1           3d17h

  NAME                                                          DESIRED   CURRENT   READY   AGE
  replicaset.apps/openshift-adp-controller-manager-76b8bc8d7b   1         1         1       3d17h
  replicaset.apps/openshift-adp-controller-manager-85fff975b8   0         0         0       3d17h
  replicaset.apps/velero-64475b8c5b                             1         1         1       3d17h
  replicaset.apps/velero-8b5bc54fd                              0         0         0       3d17h
  replicaset.apps/velero-f5c9ffb66                              0         0         0       3d17h
  ```

  **Note**: The node-agent Pods are created only if using `restic` or `kopia` in DPA.

  **Note**: The node-agent Pods are labeled as `restic` in older installations.

* Verify the BackupStorageLocations
  ```
  oc get backupStorageLocation -n openshift-adp
  NAME           PHASE       LAST VALIDATED   AGE     DEFAULT
  dpa-sample-1   Available   1s               3d16h   true
  ```

  **Note**: the `PHASE` set to `Available`.

<hr style="height:1px;border:none;color:#333;">
<h1 align="center">OLM Integration</h1>
<hr style="height:1px;border:none;color:#333;">

Create `oadp-operator-source.yaml` file like below in the oadp-operator directory:

```
apiVersion: operators.coreos.com/v1
kind: OperatorSource
metadata:
  name: oadp-operator
  namespace: openshift-marketplace
spec:
  type: appregistry
  endpoint: https://quay.io/cnr
  registryNamespace: deshah
  displayName: "OADP Operator"
  publisher: "deshah@redhat.com"
```

<b>Note:</b> All commands should be run in the root directory of this repository.

Run the following commands below:

```
oc create namespace oadp-operator-system
oc project oadp-operator-system
oc create secret generic <SECRET_NAME> --namespace oadp-operator-system --from-file cloud=<CREDENTIALS_FILE_PATH>
oc create -f oadp-operator-source.yaml
```
- After running these commands, install OADP Operator from the `README` 
instructions.

When the installation is succeeded, create a Velero CRD instance from the 
`README` instructions.

Post completion of all the above steps, you can check if the operator was 
successfully installed. The expected result for the command 
`oc get all -n oadp-operator-system` is as follows:

```
NAME                                             READY   STATUS    RESTARTS   AGE
pod/oadp-default-aws-registry-568978c9dc-glpfj   1/1     Running   0          10h
pod/oadp-operator-64f79d9bf4-4lzl9               1/1     Running   0          10h
pod/restic-bc5tm                                 1/1     Running   0          10h
pod/restic-dzrkh                                 1/1     Running   0          10h
pod/restic-z4mhx                                 1/1     Running   0          10h
pod/velero-779f785b7d-5z6qf                      1/1     Running   0          10h

NAME                                    TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)             AGE
service/oadp-default-aws-registry-svc   ClusterIP   172.30.155.164   <none>        5000/TCP            10h
service/oadp-operator-metrics           ClusterIP   172.30.58.121    <none>        8383/TCP,8686/TCP   10h

NAME                    DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
daemonset.apps/restic   3         3         3       3            3           <none>          10h

NAME                                        READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/oadp-default-aws-registry   1/1     1            1           10h
deployment.apps/oadp-operator               1/1     1            1           10h
deployment.apps/velero                      1/1     1            1           10h

NAME                                                   DESIRED   CURRENT   READY   AGE
replicaset.apps/oadp-default-aws-registry-568978c9dc   1         1         1       10h
replicaset.apps/oadp-operator-64f79d9bf4               1         1         1       10h
replicaset.apps/velero-779f785b7d                      1         1         1       10h

NAME                                                       HOST/PORT                                                                                        PATH   SERVICES                        PORT       TERMINATION   WILDCARD
route.route.openshift.io/oadp-default-aws-registry-route   oadp-default-aws-registry-route-oadp-operator.apps.cluster-dshah-4-5.dshah-4-5.mg.dog8code.com          oadp-default-aws-registry-svc   5000-tcp                 None
``` 

- For cleaning up the deployed resources, remove the Velero CR instance, 
and then uninstall the operator from the `README` instructions. To check if the 
resources are removed, run:

```
$ oc get all -n oadp-operator
No resources found in oadp-operator-system namespace.
```


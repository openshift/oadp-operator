<h1 align="center">Install & Build from Source (Non-OLM)</h1>

### Prerequisites

- Docker/Podman
- OpenShift CLI
- Access to OpenShift cluster

### Cloning the Repository

Checkout this OADP Operator repository:

```
git clone https://github.com/openshift/oadp-operator.git
cd oadp-operator
```

### Installing the Operator

To install CRDs and deploy the OADP operator to the `openshift-adp`
 namespace, run:

```
$ make deploy-olm
```

After testing, uninstall CRDs and undeploy the OADP operator from `openshift-adp` namespace, running
```
$ make undeploy-olm
```

### Installing Velero + Restic

#### Creating credentials secret
Before creating a DataProtectionApplication (DPA) CR, ensure you have created a secret
 `cloud-credentials` in namespace `openshift-adp`

 Make sure your credentials file is in the proper format. For example, if using
 AWS, it should look like:

  ```
  [<INSERT_PROFILE_NAME>]
  aws_access_key_id=<INSERT_VALUE>
  aws_secret_access_key=<INSERT_VALUE>
  ```
  *Note:* Do not use quotes while putting values in place of INSERT_VALUE Placeholders

Create the secret:

 ```
$ oc create secret generic cloud-credentials --namespace openshift-adp --from-file cloud=<CREDENTIALS_FILE_PATH>
```

#### Creating a DataProtectionApplication custom resource to install Velero
You can specify your DataProtectionApplication (DPA) CR config values here: `congig/samples/oadp_v1alpha1_dpa.yaml`

Create the DPA CR:

```
$ oc create -n openshift-adp -f config/samples/oadp_v1alpha1_dpa.yaml
```

### Verify Installation

Post completion of all the above steps, you can check if the
operator was successfully installed if the expected result for the command
`oc get all -n openshift-adp` is as follows:

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

### Uninstall Operator

`$ oc delete namespace openshift-adp`

<div align="center">
  <h1> OADP Operator </h1>
  <p>  OADP is OpenShift API for Data Protection operator. This operator sets up and 
installs <a href="https://velero.io/">Velero</a> on the OpenShift platform.</p>

  [![Go Report Card](https://goreportcard.com/badge/github.com/openshift/oadp-operator)](https://goreportcard.com/report/github.com/openshift/oadp-operator) [![codecov](https://codecov.io/gh/openshift/oadp-operator/branch/master/graph/badge.svg?token=qLM0hAzjpD)](https://codecov.io/gh/openshift/oadp-operator) [![License](https://img.shields.io/:license-apache-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0.html) [![Go Reference](https://pkg.go.dev/badge/github.com/openshift/oadp-operator.svg)](https://pkg.go.dev/github.com/openshift/oadp-operator)

  AWS: [![AWS builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-operator-e2e-aws-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-operator-e2e-aws-periodic-slack)
  GCP: [![GCP builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-operator-e2e-gcp-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-operator-e2e-gcp-periodic-slack)
  Azure: [![Azure builds](https://prow.ci.openshift.org/badge.svg?jobs=periodic-ci-openshift-oadp-operator-master-operator-e2e-azure-periodic-slack)](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/periodic-ci-openshift-oadp-operator-master-operator-e2e-azure-periodic-slack)
</div>

# Table of Contents

1. [Getting Started](#get-started)
    1. [Prerequisites](#prerequisites)
    2. [Cloning the Repository](#clone-repo)
    3. [Installing the Operator](#operator-install)
    4. [Installing Velero + Restic](#velero-restic-install)
    5. [Verify Installation](#verify-install)
    6. [Uninstall Operator](#uninstall)
2. Custom Installation
    1. [Configure Plugins](docs/plugins.md)
    2. [Backup Storage Locations and Volume Snapshot Locations](docs/bsl_and_vsl.md)
    3. [Resource Requests and Limits](docs/resource_req_limits.md)
    4. [Self-Signed Certificate](docs/self_signed_certs.md)
3. [OLM Integration](docs/olm.md)
4. [Use NooBaa as a Backup Storage Location](docs/noobaa/install_oadp_noobaa.md) 
5. [Use Velero --features flag](docs/features_flag.md)
6. Examples
    1. [Stateless App Backup/Restore](docs/examples/stateless.md)
    2. [Stateful App Backup/Restore](docs/examples/stateful.md)
7. [Velero Version Relationship](#version)
8. [Troubleshooting](./TROUBLESHOOTING.md)


<hr style="height:1px;border:none;color:#333;">

<h1 align="center">Getting Started<a id="get-started"></a></h1>

### Prerequisites <a id="prerequisites"></a> 

- Docker/Podman  
- OpenShift CLI  
- Access to OpenShift cluster  

### Cloning the Repository <a id="clone-repo"></a>

Checkout this OADP Operator repository:

```
git clone https://github.com/openshift/oadp-operator.git
cd oadp-operator
```

### Installing the Operator <a id="operator-install"></a>

To install CRDs and deploy the OADP operator to the `oadp-operator-system`
 namespace, run:
```
$ make deploy
```

### Installing Velero + Restic <a id="velero-restic-install"></a>

#### Creating credentials secret
Before creating a Velero CR, ensure you have created a secret
 `cloud-credentials` in namespace `oadp-operator-system`

 ```
$ oc create secret generic cloud-credentials --namespace oadp-operator-system --from-file cloud=<CREDENTIALS_FILE_PATH>
```

#### Creating a Velero custom resource to install Velero
```
$ oc create -n oadp-operator-system -f config/samples/oadp_v1alpha1_velero.yaml
```

### Verify Installation <a id="verify-install"></a>

Post completion of all the above steps, you can check if the 
operator was successfully installed if the expected result for the command 
`oc get all -n oadp-operator-system` is as follows:
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

### Uninstall Operator <a id="uninstall"></a>

`$ make undeploy`

<hr style="height:1px;border:none;color:#333;">

<h1 align="center">Velero Version Relationship<a id="version"></a></h1>

By default, OADP will install the forked versions of Velero that exist under the `konveyor` organization. These images have minor tweaks to support the OpenShift specific use cases of using Velero with OCP. The `konveyor` images tend to lag behind Velero upstream releases as we are more cautious about supporting older versions. Here is the default mapping of versions:

| OADP Version   | Velero Version |
| :------------- |   -----------: |
|  v0.1.1        | v1.4.1         |
|  v0.1.2        | v1.4.2         |
|  v0.1.3        | v1.4.2         |
|  v0.1.4        | v1.4.2         |
|  v0.2.0        | v1.5.2         |
|  v0.2.1        | v1.5.2         |
|  v0.2.3        | v1.6.0         |
|  v0.2.4        | v1.6.0         |
|  v0.2.5        | v1.6.0         |
|  v0.2.6        | v1.6.0         |


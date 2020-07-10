# OADP Operator

## Overview

OADP is OpenShift Application Data Protection operator. This operator sets up and installs [Velero](https://velero.io/) on the OpenShift platform.

## Prerequisites

- Docker/Podman
- OpenShift CLI
- Access to OpenShift cluster

## Getting Started

### Cloning the Repository

Checkout this OADP Operator repository:

```
git clone git@github.com:konveyor/oadp-operator.git
cd oadp-operator
```

### Building the Operator

Build the OADP operator image and push it to a public registry (quay.io or [dockerhub](https://hub.docker.com/))

There are two ways to build the operator image:

- Using operator-sdk
  ```
  operator-sdk build oadp-operator
  ```
- Using Podman/Docker
  ```
  podman build -f build/Dockerfile . -t oadp-operator:latest
  ```
After successfully building the operator image, push it to a public registry.

### Using the Image

In order to use a locally built image of the operator, please update the `operator.yaml` file. Update the `image` of the `oadp-operator` container with the image registry URL. You can edit the file manually or use the following command( `<REGISTRY_URL>` is the placeholder for your own registry url in the command):
```
sed -i 's|quay.io/konveyor/oadp-operator:latest|<REGISTRY_URL>|g' deploy/operator.yaml
```
For OSX, use the following command:
```
sed -i "" 's|quay.io/konveyor/oadp-operator:latest|<REGISTRY_URL>|g' deploy/operator.yaml
```

Before proceeding further make sure the `image` is updated in the `operator.yaml` file as discussed above.

### Operator installation

To install OADP operator and the essential Velero components follow the steps given below:

- Create a new namespace named `oadp-operator`
  ```
  oc create namespace oadp-operator
  ```
- Switch to the `oadp-operator` namespace
  ```
  oc project oadp-operator
  ```
- Create secret for the cloud provider credentials to be used. Also, the credentials file present at `CREDENTIALS_FILE_PATH` shoud be in proper format, for instance if the provider is AWS it should follow this AWS credentials [template](https://github.com/konveyor/velero-examples/blob/master/velero-install/aws-credentials)
  ```
  oc create secret generic <SECRET_NAME> --namespace oadp-operator --from-file cloud=<CREDENTIALS_FILE_PATH>
  ```
- Now to create the deployment, role, role binding, service account and the cluster role binding, use the following command:
  ```
  oc create -f deploy/
  ```
- Deploy the Velero custom resource definition:
  ```
  oc create -f deploy/crds/konveyor.openshift.io_veleros_crd.yaml   
  ```
- Finally, deploy the Velero CR:
  ```
  oc create -f deploy/crds/konveyor.openshift.io_v1alpha1_velero_cr.yaml
  ```

Post completion of all the above steps, you can check if the operator was successfully installed or not, the expected result for the command `oc get all -n oadp-operator` is as follows:
```
NAME                                 READY     STATUS    RESTARTS   AGE
pod/oadp-operator-7749f885f6-9nm9w   1/1       Running   0          6m6s
pod/restic-48s5r                     1/1       Running   0          2m16s
pod/restic-5sr4c                     1/1       Running   0          2m16s
pod/restic-bs5p2                     1/1       Running   0          2m16s
pod/velero-76546b65c8-tm9vv          1/1       Running   0          2m16s

NAME                            TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)             AGE
service/oadp-operator-metrics   ClusterIP   172.30.21.118   <none>        8383/TCP,8686/TCP   5m51s

NAME                    DESIRED   CURRENT   READY     UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
daemonset.apps/restic   3         3         3         3            3           <none>          2m17s

NAME                            READY     UP-TO-DATE   AVAILABLE   AGE
deployment.apps/oadp-operator   1/1       1            1           6m7s
deployment.apps/velero          1/1       1            1           2m17s

NAME                                       DESIRED   CURRENT   READY     AGE
replicaset.apps/oadp-operator-7749f885f6   1         1         1         6m7s
replicaset.apps/velero-76546b65c8          1         1         1         2m17s

``` 
<b>Note:</b> For using the `velero` CLI directly configured for the `oadp-operator` namespace, you may want to use the following command:
```
velero client config set namespace=oadp-operator
```

### Configure Velero Plugins

There are mainly two categories of velero plugins that can be specified while installing Velero:

1. `default-velero-plugins`:<br>
   4 types of default velero plugins can be installed - AWS, GCP, Azure and OpenShift. For installation, you need to specify them in the `konveyor.openshift.io_v1alpha1_velero_cr.yaml` file during deployment.
   ```
    apiVersion: konveyor.openshift.io/v1alpha1
    kind: Velero
    metadata:
      name: example-velero
    spec:
      default_velero_plugins:
      - azure
      - gcp
      - aws
      - openshift    
   ```
   The above specification will install Velero with all the 4 default plugins.
   
2. `custom-velero-plugin`:<br>
   For installation of custom velero plugins, you need to specify the plugin `image` and plugin `name` in the `konveyor.openshift.io_v1alpha1_velero_cr.yaml` file during deployment.

   For instance, 
   ```
    apiVersion: konveyor.openshift.io/v1alpha1
    kind: Velero
    metadata:
      name: example-velero
    spec:
      default_velero_plugins:
      - azure
      - gcp
      custom_velero_plugins:
      - name: custom-plugin-example
        image: quay.io/example-repo/custom-velero-plugin   
   ```
   The above specification will install Velero with 3 plugins (azure, gcp and custom-plugin-example).

### Enable CSI plugin for Velero

By default the CSI plugin is not enabled, in order to enable the [CSI plugin](https://github.com/vmware-tanzu/velero-plugin-for-csi/) for velero, you need to specify a flag `enable_csi_plugin` and set it to `true` in the `konveyor.openshift.io_v1alpha1_velero_cr.yaml` file during the installation.

### Configure Backup Storage Locations and Volume Snapshot Locations

For configuring the `backupStorageLocations` and the `volumeSnapshotLocations` we will be using the `backup_storage_locations` and the `volume_snapshot_locations` specs respectively in the `konveyor.openshift.io_v1alpha1_velero_cr.yaml` file during the deployement. 

For instance, If we want to configure `aws` for `backupStorageLocations` as well as `volumeSnapshotLocations` pertaining to velero, our `konveyor.openshift.io_v1alpha1_velero_cr.yaml` file should look something like this:

```
apiVersion: konveyor.openshift.io/v1alpha1
kind: Velero
metadata:
  name: example-velero
spec:
  default_velero_plugins:
  - aws
  backup_storage_locations:
  - name: default
    provider: aws
    object_storage:
      bucket: myBucket
      prefix: "velero"
    config:
      region: us-east-1
      profile: "default"
    credentials_secret_ref:
      name: cloud-credentials
      namespace: oadp-operator
  volume_snapshot_locations:
  - name: default
    provider: aws
    config:
      region: us-west-2
      profile: "default"
```
<b>Note:</b> 
- Be sure to use the same `secret` name you used while creating the cloud credentials secret in step 3 of Operator   installation section.
- Another thing to consider are the CR file specs, they should be tailored in accordance to your own cloud provider accouts, for instance `bucket` spec value should be accoring to your own bucket name and so on.
- Do not configure more than one `backupStorageLocations` per cloud provider, the velero installation will fail.  
- Parameter reference for [backupStorageLocations](https://velero.io/docs/master/api-types/backupstoragelocation/) and [volumeSnapshotLocations](https://velero.io/docs/master/api-types/volumesnapshotlocation/)

### Using upstream images

In order use the upstream images for Velero deployment as well as its plugins, you need to specify a flag `use_upstream_images` in the `konveyor.openshift.io_v1alpha1_velero_cr.yaml` during installation of the operator.

For instance the `konveyor.openshift.io_v1alpha1_velero_cr.yaml` file might look something like this:

```
apiVersion: konveyor.openshift.io/v1alpha1
kind: Velero
metadata:
  name: example-velero
spec:
  use_upstream_images: true
  default_velero_plugins:
  - aws
  backup_storage_locations:
  - name: default
    provider: aws
    object_storage:
      bucket: shubbam-6109f5e9711c8c58131acdd2f490f451
      prefix: "velero"
    config:
      region: us-east-1
      profile: "default"
    credentials_secret_ref:
      name: cloud-credentials
      namespace: oadp-operator
  volume_snapshot_locations:
  - name: default
    provider: aws
    config:
      region: us-west-2
      profile: "default"
  enable_restic: true
```
Such a CR specification will use the upstream images for deployment.

<b>Note:</b> If the flag `use_upstream_images` is set, the registry will be switched from `quay.io` to `docker.io` and v1.4.0 (current upstream version) image tag will be used for `Velero` and `latest` image tag will be used for the `plugins`.  

### Setting resource limits and requests for Velero and Restic Pods

In order to set specific resource(cpu, memory) `limits` and `requests` for the Velero pod, you need use the `velero_resource_allocation` specification field in the `konveyor.openshift.io_v1alpha1_velero_cr.yaml` file during the deployment.

For instance, the `velero_resource_allocation` can look somewhat similar to:
```
velero_resource_allocation:
  limits:
    cpu: "2"
    memory: 512Mi
  requests:
    cpu: 500m
    memory: 256Mi
```

Similarly, you can use the `restic_resource_allocation` specification field for setting specific resource `limits` and `requests` for the Restic pods.

```
restic_resource_allocation:
  limits:
    cpu: "2"
    memory: 512Mi
  requests:
    cpu: 500m
    memory: 256Mi
```

<b>Note:</b> 
- The values for the resource requests and limits flags follow the same format as [Kubernetes resource requirements](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/)
- Also, if the `velero_resource_allocation`/`restic_resource_allocation` is not defined by the user then the default resources specification for Velero/Restic pod(s) is 
  ```
  resources:
    limits:
      cpu: "1"
      memory: 256Mi
    requests:
      cpu: 500m
      memory: 128Mi
  ```

### Use Velero with a storage provider secured by a self-signed certificate

If you are using an S3-Compatible storage provider that is secured with a self-signed certificate, connections to the object store may fail with a `certificate signed by unknown authority` message. In order to proceed, you will have to specify the a base64 encoded certificate string as a value of the `caCert` spec under the `object_storage` configuration in the velero CR.

Your CR might look somewhat like this:

```
apiVersion: konveyor.openshift.io/v1alpha1
kind: Velero
metadata:
  name: example-velero
spec:
  use_upstream_images: true
  default_velero_plugins:
  - aws
  - openshift
  backup_storage_locations:
  - name: default
    provider: aws
    object_storage:
      bucket: velero
      caCert: <base64_encoded_cert_string>
    config:
      region: us-east-1
      profile: "default"
      insecure_skip_tls_verify: "false"
      signature_version: "1"
      public_url: "https://m-oadp.apps.cluster-sdpampat0519.sdpampat0519.mg.dog8code.com"
      s3_url: "https://m-oadp.apps.cluster-sdpampat0519.sdpampat0519.mg.dog8code.com"
      s3_force_path_style: "true"
    credentials_secret_ref:
      name: cloud-credentials
      namespace: oadp-operator
  enable_restic: true
```
<b>Note:</b> Ensure that `insecure_skip_tls_verify` is set to `false` so that TLS is used.

### OLM Installation
Make sure operator-sdk and olm is properly installed, for instance the output of the command `operator-sdk olm status` should look like this:
```
I0708 14:23:09.267307  211635 request.go:621] Throttling request took 1.049514278s, request: GET:https://api.cluster-dshah0518.dshah0518.mg.dog8code.com:6443/apis/crunchydata.com/v1?timeout=32s
INFO[0002] Fetching CRDs for version "0.14.1"           
INFO[0003] Fetching resources for version "0.14.1"      
INFO[0004] Successfully got OLM status for version "0.14.1" 

NAME                                            NAMESPACE    KIND                        STATUS
olm                                                          Namespace                   Installed
operatorgroups.operators.coreos.com                          CustomResourceDefinition    Installed
catalogsources.operators.coreos.com                          CustomResourceDefinition    Installed
subscriptions.operators.coreos.com                           CustomResourceDefinition    Installed
installplans.operators.coreos.com                            CustomResourceDefinition    Installed
aggregate-olm-edit                                           ClusterRole                 Installed
catalog-operator                                olm          Deployment                  Installed
olm-operator                                    olm          Deployment                  Installed
operatorhubio-catalog                           olm          CatalogSource               Installed
olm-operators                                   olm          OperatorGroup               Installed
aggregate-olm-view                                           ClusterRole                 Installed
operators                                                    Namespace                   Installed
global-operators                                operators    OperatorGroup               Installed
olm-operator-serviceaccount                     olm          ServiceAccount              Installed
packageserver                                   olm          ClusterServiceVersion       Installed
system:controller:operator-lifecycle-manager                 ClusterRole                 Installed
clusterserviceversions.operators.coreos.com                  CustomResourceDefinition    Installed
olm-operator-binding-olm                                     ClusterRoleBinding          Installed
```
To install operator-sdk, select compiling and installing from source option from this [link](https://docs.openshift.com/container-platform/4.2/operators/operator_sdk/osdk-getting-started.html).

To install OLM, use the following command `operator-sdk olm install --version 0.14.1`.

### OLM Integration
<b>Note:</b> Run all the commands at the root of the directory.

To publish the operator on operatorhub in your cluster, run `operator-sdk run packagemanifests --olm-namespace olm --operator-namespace oadp-operator --operator-version 0.1.0`.


### Cleanup
For cleaning up the deployed resources, use the following commands:
```
oc delete -f deploy/crds/konveyor.openshift.io_v1alpha1_velero_cr.yaml
oc delete -f deploy/crds/konveyor.openshift.io_veleros_crd.yaml   
oc delete -f deploy/
oc delete namespace oadp-operator
oc delete crd $(oc get crds | grep velero.io | awk -F ' ' '{print $1}')
```



# OADP Operator

## Overview

OADP is OpenShift Application Data Protection operator. This operator sets up and installs [Velero](https://velero.io/) on the OpenShift platform.

## Prerequisites

- Docker/Podman
- OpenShift CLI
- Access to OpenShift cluster

***
## Getting started with basic install
***

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
- Now to create the deployment, role, role binding, service account, use the following command:
  ```
  oc create -f deploy/
  ```
- To create the cluster role binding, run the following command:
  ```
  oc create -f deploy/non-olm
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

***
## Customize Installation
***

### Configure Velero Plugins

There are mainly two categories of velero plugins that can be specified while installing Velero:

1. `default-velero-plugins`:<br>
   Five types of default velero plugins can be installed - AWS, GCP, Azure, OpenShift, and CSI. For installation, you need to specify them in the `konveyor.openshift.io_v1alpha1_velero_cr.yaml` file during deployment.
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
      - csi
   ```
   The above specification will install Velero with all five default plugins. 

   [This](https://github.com/vmware-tanzu/velero-plugin-for-csi/) repository has more information about the CSI plugin.
   
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


### Backup Storage Locations and Volume Snapshot Locations Customization

Velero supports backup storage locations and volume snapshot locations from a number of cloud providers (AWS, Azure and GCP). Please refer the section [configure Backup Storage Locations and Volume Snapshot Locations](docs/bsl_and_vsl.md). 

### Using upstream images

In order to use the upstream images for Velero deployment as well as its plugins, you need to set a flag `use_upstream_images` as `true` in the `konveyor.openshift.io_v1alpha1_velero_cr.yaml` during installation of the operator.

<b>Note:</b> If the flag `use_upstream_images` is set, the registry will be switched from `quay.io` to `docker.io` and v1.4.0 (current upstream version) image tag will be used for `Velero` and `latest` image tag will be used for the `plugins`.  

### Resource requests and limits customization

By default, the Velero deployment requests 500m CPU, 128Mi memory and sets a limit of 1000m CPU, 256Mi. Customization of these resource requests and limits may be performed using steps specified in the [Resource requests and limits customization](docs/resource_req_limits.md) section.

### Use self-sigend certificate

If you intend to use Velero with a storage provider that is secured by a self-signed certificate, you may need to instruct Velero to trust that certificate. See [Use self-sigend certificate](docs/self_signed_certs.md) section for details.

***
## OLM Integration
***

For installing/uninstalling the OADP operator directly from OperatorHub, follow this document [OLM Integration](docs/olm.md) for details.

***
## OADP Operator with NooBaa
***

[Install OADP Operator and use NooBaa as a BackupStoraeLocation](docs/noobaa/install_oadp_noobaa.md) 

[NooBaa debugging scenarios](docs/noobaa/debug_noobaa.md)

[Cleanup OADP Operator with NooBaa](docs/noobaa/cleanup_oadp_noobaa.md)

***
## Cleanup
***
For cleaning up the deployed resources, use the following commands:
```
oc delete -f deploy/crds/konveyor.openshift.io_v1alpha1_velero_cr.yaml
oc delete -f deploy/crds/konveyor.openshift.io_veleros_crd.yaml   
oc delete -f deploy/
oc delete namespace oadp-operator
oc delete crd $(oc get crds | grep velero.io | awk -F ' ' '{print $1}')
```


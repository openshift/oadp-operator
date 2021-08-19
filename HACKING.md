# Installing OADP Operator v2

## Install CRDs + operator pod
To install CRDs and deploy OADP operator to `oadp-operator-system` namespace, run:
```
$ make deploy
```

## Install Velero + Restic
First, ensure you have created a secret `cloud-credentials` in namespace `oadp-operator-system`
```
$ oc create secret generic cloud-credentials --namespace oadp-operator-system --from-file cloud=<CREDENTIALS_FILE_PATH>
```

Create a `Velero` Custom Resource to install Velero
```
$ oc create -n oadp-operator-system -f config/samples/oadp_v1alpha1_velero.yaml
```

## Local Development Environment
You can test your changes by creating your own images and running your builds locally

```
1. podman build . -t quay.io/<CONTAINER_REGISTRY_USERNAME>/oadp-operator:<IMAGE_TAG>
```
Note: The above command for `podman build` is to be executed from the root directory of the operator.
```
2. podman push <IMAGE_ID> quay.io/<CONTAINER_REGISTRY_USERNAME>/oadp-operator:<IMAGE_TAG>
```
    <IMAGE_ID> can be found out by running `podman images` after the image has been built.
    <IMAGE_TAG> can be any tag that you would would like to assign to the image.
    
```
3.IMG=quay.io/<CONTAINER_REGISTRY_USERNAME>/oadp-operator:<IMAGE_TAG> make deploy
```

# Uninstall OADP Operator
```
$ make undeploy
```


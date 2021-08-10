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

# Uninstall OADP Operator
```
$ make undeploy
```

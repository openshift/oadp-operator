***
## Install OADP Operator with NooBaa
***

Please follow the following steps in order to install OADP Operator with NooBaa:

1. Create a namespace named `oadp-operator`.
2. Do not create any cloud credentials secret as the secret comes out of the box for NooBaa.
3. Now install the OCS (OpenShift Container Storage) operator from the OperatorHub in the `oadp-operator` namespace, so that the requisite NooBaa CRDs get deployed on the cluster and wait till the OCS operator pods are in running state.
4. Make sure the Velero CR file specifically has the following:
   - `noobaa: true`
   - `enable_restic: true`
   - `use_upstream_images: true`
   - `default_velero_plugins` list should only consist of `aws` plugin
   - No data pertaining to Volume Snapshot Locations and Backup Storage Locations.
 
      The CR file may look somewhat like this:
      ```
        apiVersion: konveyor.openshift.io/v1alpha1
        kind: Velero
        metadata:
        name: example-velero
        spec:
          use_upstream_images: true
          noobaa: true
          default_velero_plugins:
          - aws
          enable_restic: true
      ```
  
5. Now for deployment of velero use the following commands in sequence:
```
oc create -f deploy/
oc create -f deploy/crds/konveyor.openshift.io_veleros_crd.yaml
oc create -f deploy/crds/konveyor.openshift.io_v1alpha1_velero_cr.yaml
```

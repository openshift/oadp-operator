<h1 align="center">Upgrading from OADP 0.2</h1>

#### There are a few differences between OADP 0.2 and  0.3:

1. The `apiVersion` has changed from `konveyor.openshift.io/v1alpha1` 
to `oadp.openshift.io/v1alpha1`.

2. The `spec` values have changed from `snake_case` to `camelCase` with 
*nearly* a 1:1 mapping. 

*Important!!* The only config name that has changed is for the credentials secret
used in the `backupStorageLocations` spec: 
`credentials_secret_ref` to `credential`.

<hr style="height:1px;border:none;color:#333;">

## Upgrade

### Copy/save old Velero CR definitions to another location
Save your current Velero CR config as to be sure to remember the values.

### Convert your CR config to the new version
As mentioned above, the `spec` values have changed from `snake_case` to `camelCase` with 
*nearly* a 1:1 mapping. The only config name that has changed is for the credentials secret
used in the `backupStorageLocations` spec: 
`credentials_secret_ref` to `credential`.

For example, here is a sample Velero CR for the old version:

```
apiVersion: konveyor.openshift.io/v1alpha1
kind: Velero
metadata:
  name: example-velero
spec:
  olm_managed: false
  default_velero_plugins:
  - aws
  - openshift
  - csi
  backup_storage_locations:
  - name: default
    provider: aws
    object_storage:
      bucket: my-bucket-name
      prefix: my-prefix
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

And a sample Velero CR for the new version:

```
apiVersion: oadp.openshift.io/v1alpha1
kind: Velero
metadata:
  name: velero-sample
spec:
  olmManaged: false
  defaultVeleroPlugins:
  - openshift
  - aws
  backupStorageLocations:
  - provider: aws
    default: true
    objectStorage:
      bucket: my-bucket-name
      prefix: my-prefix
    config:
      region: us-east-1
      profile: "default"
    credential:
      name: cloud-credentials
      key: cloud
  volumeSnapshotLocations:
    - provider: aws
      config:
        region: us-west-2
        profile: "default"
  enableRestic: true
```

### Uninstall the OADP operator
Use the web console to uninstall the OADP operator by clicking on 
`Install Operators` under the `Operators` tab on the left-side menu. 
Then click on `OADP Operator`, as shown below.

![](/docs/images/installed_op.png)

After clicking on `OADP Operator` under `Installed Operators`, navigate to the
right side of the page, where the `Actions` drop-down menu is. Click on that, 
and select `Uninstall Operator`, as shown below.

![](/docs/images/uninstall_op.png)

### Delete the remaining resources
To delete the remaining resources that are deployed, use the following commands:

```
oc delete -f deploy/crds/konveyor.openshift.io_v1alpha1_velero_cr.yaml
oc delete -f deploy/crds/konveyor.openshift.io_veleros_crd.yaml   
oc delete -f deploy/non-olm/
oc delete namespace oadp-operator
oc delete crd $(oc get crds | grep velero.io | awk -F ' ' '{print $1}')
```

### Install OADP Operator 0.3.x
Follow theses [basic install](../docs/install_olm.md) instructions to install the 
new OADP operator version, create the Velero CR, and verify correct installation.

<h1 align="center">Upgrading from OADP 0.2</h1>

#### There are a few differences between OADP 0.2 and  0.3:

1. The `apiVersion` has changed from `konveyor.openshift.io/v1alpha1` 
to `oadp.openshift.io/v1alpha1`.

2. The `spec` values have changed from `snake_case` to `camelCase` with 
*nearly* a 1:1 mapping. 

*Note:* 
  - The only config name that will be different is for the credentials secret
  used in the `backupLocations.velero` spec: 
  `credentials_secret_ref` should now be `credential`.
  
  - `oc get velero` may not return correct results if the old CRDs are installed, 
    due to the old CRD being picked up first. If this occurs, you can delete
    the old CRD, or run `oc get velero.oadp.openshift.io`
      - For example: 
      ```
    ❯ oc get velero.oadp.openshift.io -n openshift-adp
      NAME             AGE
      example-velero   94s

      ❯ oc get velero -n openshift-adp
      No resources found in oadp-operator-system namespace.
      ```

<hr style="height:1px;border:none;color:#333;">

## Upgrade

### Copy/save old Velero CR definitions to another location
Save your current Velero (Now DataProtectionApplication - DPA) CR config as to be sure to remember the values.

### Convert your CR config to the new version
As mentioned above, the `spec` values have changed from `snake_case` to `camelCase` with 
*nearly* a 1:1 mapping. The only config value that will be different is for the credentials secret
used in the `backupLocations.velero` spec: 
`credentials_secret_ref` to `credential`. You can browse available fields in our [API reference](API_ref.md).

For example, here is a sample Velero CR (Now DataProtectionApplication - DPA) for the 0.2 version:

```
apiVersion: konveyor.openshift.io/v1alpha1
kind: Velero
metadata:
  name: example-velero
spec:
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

Sample Velero CR for version 0.3-0.4
```
apiVersion: oadp.openshift.io/v1alpha1
kind: Velero
metadata:
  name: velero-sample
spec:
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

A sample DataProtectionApplication (previously Velero) for version 0.5 or later:

```
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
spec:
  configuration:
    velero:
      defaultPlugins:
      - openshift
      - aws
    restic:
      enable: true
  backupLocations:
    - name: default
      velero:
        provider: aws
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
  snapshotLocations:
    - name: default
      velero:
        provider: aws
        config:
          region: us-west-2
          profile: "default"
```

Sample DataProtectionApplication (deprecated `restic` and replaced by `nodeAgent`) for version 1.3 or later:

```
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
spec:
  configuration:
    velero:
      defaultPlugins:
      - openshift
      - aws
    nodeAgent:
      enable: true
      uploaderType: restic
  backupLocations:
    - name: default
      velero:
        provider: aws
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
  snapshotLocations:
    - name: default
      velero:
        provider: aws
        config:
          region: us-west-2
          profile: "default"
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
oc delete namespace oadp-operator
oc delete crd veleros.konveyor.openshift.io
```

### Install OADP Operator 0.3.x
Follow theses [basic install](../docs/install_olm.md) instructions to install the 
new OADP operator version, create the Velero (DPA) CR, and verify correct installation.

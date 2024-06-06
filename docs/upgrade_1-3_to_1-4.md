# Upgrading from OADP 1.3

> **NOTE:** Always upgrade to next minor version, do NOT skip versions. To update to higher version, please upgrade one channel at a time. Example: to upgrade from 1.1 to 1.3, upgrade first to 1.2, then to 1.3.

## Changes from OADP 1.3 to 1.4

- Velero was updated from version 1.12 to 1.14 (Changes reference: https://velero.io/docs/v1.13/upgrade-to-1.13/ https://velero.io/docs/v1.14/upgrade-to-1.14/)

    From this update:

    - TODO CSI?

    - Velero changed client Burst and QPS defaults from 30 and 20 to 100 and 100, respectively. If you want to change these values, use new `spec.configuration.velero.client-burst` and `spec.configuration.velero.client-qps` fields.

    - Non AWS object storages that use AWS plugin need a new `spec.backupLocations[].velero.config.checksumAlgorithm` field to be set in DPA to them to work in OADP 1.4.

- TODO image pull policy override

## Upgrade steps

### Backup the DPA configuration

Save your current DataProtectionApplication (DPA) CustomResource config, be sure to remember the values.

For example:
```
oc get dpa -n openshift-adp -o yaml > dpa.orig.backup
```

### Upgrade the OADP Operator

For general operator upgrade instructions please review the [OpenShift documentation](https://docs.openshift.com/container-platform/4.13/operators/admin/olm-upgrading-operators.html)
* Change the Subscription for the OADP Operator from `stable-1.3` to `stable-1.4`
* Allow time for the operator and containers to update and restart

### Convert your DPA to the new version

If you are using an object storage compatible with AWS (different then AWS itself) and using AWS plugin, you need to add new `spec.backupLocations[].velero.config.checksumAlgorithm` field with an empty string as value to your DPA. Example
```diff
 spec:
   backupLocations:
     - velero:
         config:
           profile: default
           region: <region>
           s3ForcePathStyle: 'true'
           s3Url: <url>
+          checksumAlgorithm: ""
```

### Verify the upgrade

Follow theses [basic install verification](../docs/install_olm.md#verify-install) to verify the installation.

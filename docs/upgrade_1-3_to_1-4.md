# Upgrading from OADP 1.3

> **NOTE:** Always upgrade to next minor version, do NOT skip versions. To update to higher version, please upgrade one channel at a time. Example: to upgrade from 1.1 to 1.3, upgrade first to 1.2, then to 1.3.

## Changes from OADP 1.3 to 1.4

- Velero was updated from version 1.12 to 1.14 (Changes reference: https://velero.io/docs/v1.13/upgrade-to-1.13/ https://velero.io/docs/v1.14/upgrade-to-1.14/)

    From this update:

    - velero-plugin-for-csi code is now inside Velero code, which means, no init container is needed for the plugin anymore. No changes needed in DPA.

    - Velero changed client Burst and QPS defaults from 30 and 20 to 100 and 100, respectively.

    - velero-plugin-for-aws updated default value of `spec.config.checksumAlgorithm` field in BackupStorageLocations (BSLs) from `""` (no checksum calculation) to `CRC32` algorithm (reference https://github.com/vmware-tanzu/velero-plugin-for-aws/blob/release-1.10/backupstoragelocation.md). The checksum algorithm types are known to work only with AWS. Several S3 providers require the md5sum to be disabled (setting checksum algorithm to `""`). Please confirm with your storage provider with regards to the md5sum algorithm support and configuration.

        OADP 1.4 default value for BSLs created within DPA for this configuration is `""`, meaning the md5sum is not checked (which is consistent with OADP 1.3). For BSLs created within DPA, this can be updated by using `spec.backupLocations[].velero.config.checksumAlgorithm` field in the DPA. If your BSLs are created outside DPA, this configuration can be updated by using `spec.config.checksumAlgorithm` field in the BSLs.

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

No DPA changes are required to upgrade from OADP 1.3 to 1.4.

### Verify the upgrade

Follow theses [basic install verification](../docs/install_olm.md#verify-install) to verify the installation.

## Changes from OADP 1.4.0 to 1.4.1

- If you want to change client Burst and QPS values, use new `spec.configuration.velero.client-burst` and `spec.configuration.velero.client-qps` fields.

- TODO image pull policy override and default behavior
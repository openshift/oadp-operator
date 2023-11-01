# Upgrading from OADP 1.1

> **NOTE:** Always upgrade to next minor version, do NOT skip versions. To update to higher version, please upgrade one channel at a time. Example: to upgrade from 1.1 to 1.3, upgrade first to 1.2, then to 1.3.
## Changes from OADP 1.1 to 1.2

- Velero was updated from version 1.9 to 1.11 (Changes reference: https://velero.io/docs/v1.11/upgrade-to-1.11/#upgrade-from-version-lower-than-v1100)

    From this update, in the DPA's configuration `spec.configuration.velero.args` have changed:

    - The `default-volumes-to-restic` field was renamed `default-volumes-to-fs-backup`, **if you are using `spec.velero`, you need to add it back, with the new name, to your DPA after upgrading OADP**

    - The `default-restic-prune-frequency` field was renamed `default-repo-maintain-frequency`, **if you are using `spec.velero`, you need to add it back, with the new name, to your DPA after upgrading OADP**

    - The `restic-timeout` field was renamed `fs-backup-timeout`, **if you are using `spec.velero`, you need to add it back, with the new name, to your DPA after upgrading OADP**

- The `restic` DaemonSet was renamed to `node-agent`.  OADP will automatically update the name of the DaemonSet

- The CustomResourceDefinition `resticrepositories.velero.io` was renamed to `backuprepositories.velero.io` 
  * The CustomResourceDefinition `resticrepositories.velero.io` can optionally be removed from the cluster

## Upgrade steps

### Backup the DPA configuration

Save your current DataProtectionApplication (DPA) CustomResource config, be sure to remember the values.

For example:
```
oc get dpa -n openshift-adp -o yaml > dpa.orig.backup 
```

### Upgrade the OADP Operator

For general operator upgrade instructions please review the [OpenShift documentation](https://docs.openshift.com/container-platform/4.13/operators/admin/olm-upgrading-operators.html)
* Change the Subscription for the OADP Operator from `stable-1.1` to `stable-1.2`
* Allow time for the operator and containers to update and restart

### Convert your DPA to the new version

If you are using fields that were updated in `spec.configuration.velero.args`, you need to update there new names. Example
```diff
 spec:
   configuration:
     velero:
       args:
-        default-volumes-to-restic: true
+        default-volumes-to-fs-backup: true
-        default-restic-prune-frequency: 6000
+        default-repo-maintain-frequency: 6000
-        restic-timeout: 600
+        fs-backup-timeout: 600
```

### Verify the upgrade 

Follow theses [basic install verification](../docs/install_olm.md#verify-install) to verify the installation.

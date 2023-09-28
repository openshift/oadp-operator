# Upgrading from OADP 1.1

> **NOTE:** Always upgrade to next minor version, do NOT skip versions. To update to higher version, do one upgrade at a time. Example: to upgrade from 1.1 to 1.3, upgrade first to 1.2, then to 1.3.
## Changes from OADP 1.1 to 1.2

- Velero was updated from version 1.9 to 1.11 (Changes reference: https://velero.io/docs/v1.11/upgrade-to-1.11/#upgrade-from-version-lower-than-v1100)

    From this update, in `spec.configuration.velero.args` these were changed:

    - `default-volumes-to-restic` was renamed `default-volumes-to-fs-backup`, **if you are using it, you need to add it back, with the new name, to your DPA after upgrading OADP**

    - `default-restic-prune-frequency` was renamed `default-repo-maintain-frequency`, **if you are using it, you need to add it back, with the new name, to your DPA after upgrading OADP**

    - `restic-timeout` was renamed `fs-backup-timeout`, **if you are using it, you need to add it back, with the new name, to your DPA after upgrading OADP**

- `restic` DaemonSet was renamed to `node-agent` (no changes required, OADP code handles this change)

- `resticrepositories.velero.io` CustomResourceDefinition was renamed to `backuprepositories.velero.io` (you can delete `resticrepositories.velero.io` CRD from your cluster, if you want)

## Upgrade steps

### Copy old DPA

Save your current DataProtectionApplication (DPA) CustomResource config, be sure to remember the values.

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

### Uninstall the OADP operator

Use the web console to uninstall the OADP operator by clicking on `Install Operators` under the `Operators` tab on the left-side menu. Then click on `OADP Operator`.

After clicking on `OADP Operator` under `Installed Operators`, navigate to the right side of the page, where the `Actions` drop-down menu is. Click on that, and select `Uninstall Operator`.

### Install OADP Operator 1.2.x

Follow theses [basic install](../docs/install_olm.md) instructions to install the new OADP operator version, create DPA, and verify correct installation.

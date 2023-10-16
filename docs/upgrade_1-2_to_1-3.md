# Upgrading from OADP 1.2

> **NOTE:** Always upgrade to next minor version, do NOT skip versions. To update to higher version, do one upgrade at a time. Example: to upgrade from 1.1 to 1.3, upgrade first to 1.2, then to 1.3.
## Changes from OADP 1.2 to 1.3

- Velero was updated from version 1.11 to 1.12 (Changes reference: https://velero.io/docs/v1.12/upgrade-to-1.12/#upgrade-from-v110-or-higher)

    From this update, OADP now uses Velero Built-in DataMover instead of VSM/Volsync DataMover. This change the following:

    - `spec.features.dataMover` was removed (you can delete secret for `spec.features.dataMover.credentialName` from your cluster, if you want)

    - `vsm` plugin was removed

    - `volsync` operator is not necessary anymore (you can uninstall it from your cluster, if you want)

    - `volumesnapshotbackups.datamover.oadp.openshift.io` and `volumesnapshotrestores.datamover.oadp.openshift.io` CustomResourceDefinitions are not necessary anymore (you can delete `volumesnapshotbackups.datamover.oadp.openshift.io` and `volumesnapshotrestores.datamover.oadp.openshift.io` CRDs from your cluster, if you want)

    Also, OADP now supports a new file system backup software: Kopia.

    - To use it, use the new `spec.configuration.nodeAgent` field. Example

        ```yaml
        spec:
          configuration:
            nodeAgent:
              enable: true
              uploaderType: kopia
        ```

`spec.configuration.restic` field is being deprecated in OADP 1.3, and will be removed in OADP 1.4. To avoid seeing deprecating warnings about it, use the new syntax:
```diff
 spec:
   configuration:
-    restic:
-      enable: true
+    nodeAgent:
+      enable: true
+      uploaderType: restic
```

> **Note:** OADP will be favoring Kopia over Restic in the near future.

## Upgrade steps

### Create new backup

If you are using DataMover, you need to create new backups before upgrade, because the 1.2 DataMover backups will not work after upgrade. We suggest to create Restic backups.

### Copy old DPA

Save your current DataProtectionApplication (DPA) CustomResource config, be sure to remember the values.

### Convert your DPA to the new version

If you are using DataMover, you need to update with the new configuration. Example
```diff
 spec:
   configuration:
-    features:
-      dataMover:
-      enable: true
-      credentialName: dm-credentials
+    nodeAgent:
+      enable: true
+      uploaderType: kopia
     velero:
       defaultPlugins:
-      - vsm
       - csi
       - openshift
```

### Uninstall the OADP operator

Use the web console to uninstall the OADP operator by clicking on `Install Operators` under the `Operators` tab on the left-side menu. Then click on `OADP Operator`.

After clicking on `OADP Operator` under `Installed Operators`, navigate to the right side of the page, where the `Actions` drop-down menu is. Click on that, and select `Uninstall Operator`.

### Install OADP Operator 1.3.x

Follow theses [basic install](../docs/install_olm.md) instructions to install the new OADP operator version, create DPA, and verify correct installation.

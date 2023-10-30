# Upgrading from OADP 1.2

> **NOTE:** Always upgrade to next minor version, do NOT skip versions. To update to higher version, do one upgrade at a time. Example: to upgrade from 1.1 to 1.3, upgrade first to 1.2, then to 1.3.
## Changes from OADP 1.2 to 1.3

- Velero was updated from version 1.11 to 1.12 (Changes reference: https://velero.io/docs/v1.12/upgrade-to-1.12/#upgrade-from-v110-or-higher)

    From this update, OADP now uses Velero Built-in DataMover instead of VSM/Volsync DataMover. This change the following:

    - `spec.features.dataMover` (you can delete secret for `spec.features.dataMover.credentialName` from your cluster, if you want) and `vsm` plugin are not necessary anymore and you need to remove them from your DPA

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

> **Note:** In the next version of OADP the Restic option will be deprecated and Kopia will be come the default uploaderType.

## Upgrade steps

### If the OADP 1.2 tech-preview DataMover feature is in use, please read the following.

OADP 1.2 DataMover backups can NOT be restored with OADP 1.3. To prevent a gap in the data protection of your applications we recommend the following.

* If on cluster backups are sufficient and CSI storage is available
  * Backup the applications with a CSI backup

* If off cluster backups are required
  * Backup the applications with a filesystem backup using the `--default-volumes-to-fs-backup=true` option.
  * Backup the applications with your CloudStorage plugins e.g. velero-plugin-for-aws

* If for any reason an OADP 1.2 DataMover backup must be restored, OADP must be fully uninstalled and OADP 1.2 reinstalled and configured.

### Backup the DPA configuration

Save your current DataProtectionApplication (DPA) CustomResource config, be sure to remember the values.

For example:
```
oc get dpa -n openshift-adp -o yaml > dpa.orig.backup 
```

For general operator upgrade instructions please review the [OpenShift documentation](https://docs.openshift.com/container-platform/4.13/operators/admin/olm-upgrading-operators.html)
* Change the Subscription for the OADP Operator from `stable-1.2` to `stable-1.3`
* Allow time for the operator and containers to update and restart

### Convert your DPA to the new version

If you are using DataMover, you need to update with the new configuration. 

* remove the features.dataMover key and values from DPA
* remove the VSM plugin 

Example
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

### Verify the upgrade 

Follow theses [basic install verification](../docs/install_olm.md#verify-install) to verify the installation.

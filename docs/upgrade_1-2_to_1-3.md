# Upgrading from OADP 1.2

> **NOTE:** Always upgrade to next minor version, do NOT skip versions. To update to higher version, please upgrade one channel at a time. Example: to upgrade from 1.1 to 1.3, upgrade first to 1.2, then to 1.3.
## Changes from OADP 1.2 to 1.3

- The Velero server has been updated from version 1.11 to 1.12 (Changes reference: https://velero.io/docs/v1.12/upgrade-to-1.12/#upgrade-from-v110-or-higher)

    From this update, OADP 1.3 now uses the Velero Built-in Data Mover instead of the VSM/Volsync Data Mover. This changes the following:

    - The `spec.features.dataMover` field and the `vsm` plugin are not compatible with 1.3 and must be removed from the DPA configuration.

    - The Volsync operator is no longer required and can optionally be removed.

    - The CustomResourceDefinitions `volumesnapshotbackups.datamover.oadp.openshift.io` and `volumesnapshotrestores.datamover.oadp.openshift.io` are no longer required and can optionally be removed.

    - The secrets used for the OADP-1.2 Data Mover are no longer required and can optionally be removed.
    
- OADP now supports Kopia, an alternative file system backup tool to Restic.

    - To employ Kopia, use the new `spec.configuration.nodeAgent` field. For example:

        ```yaml
        spec:
          configuration:
            nodeAgent:
              enable: true
              uploaderType: kopia
        ```

- The `spec.configuration.restic` field is being deprecated in OADP 1.3, and will be removed in OADP 1.4. To avoid seeing deprecating warnings about it, use the new syntax:
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

### If the OADP 1.2 tech-preview Data Mover feature is in use, please read the following

OADP 1.2 Data Mover backups can **NOT** be restored with OADP 1.3. To prevent a gap in the data protection of your applications we recommend the following to be **completed prior to the OADP upgrade**.

* If on cluster backups are sufficient and CSI storage is available
  * Backup the applications with a CSI backup

* If off cluster backups are required
  * Backup the applications with a filesystem backup using the `--default-volumes-to-fs-backup=true` or `backup.spec.defaultVolumesToFsBackup` options.
  * Backup the applications with your object storage  plugins e.g. velero-plugin-for-aws

* If for any reason an OADP 1.2 Data Mover backup must be restored, OADP must be fully uninstalled and OADP 1.2 reinstalled and configured.

### Backup the DPA configuration

Save your current DataProtectionApplication (DPA) CustomResource config, be sure to remember the values.

For example:
```
oc get dpa -n openshift-adp -o yaml > dpa.orig.backup 
```

### Upgrade the OADP Operator

For general operator upgrade instructions please review the [OpenShift documentation](https://docs.openshift.com/container-platform/4.13/operators/admin/olm-upgrading-operators.html)
* Change the Subscription for the OADP Operator from `stable-1.2` to `stable-1.3`
* Allow time for the operator and containers to update and restart

### Convert your DPA to the new version

If relocating backups off cluster is required (Data Mover), please reconfigure the DPA with the following:

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

* Wait for the DPA to reconcile successfully.

### Verify the upgrade 

Follow theses [basic install verification](../docs/install_olm.md#verify-install) to verify the installation.

**NOTE**: Invoking data movement off cluster in OADP 1.3.0 is now an option per backup vs. a DPA configuration.

For example:

```
velero backup create example-backup --include-namespaces mysql-persistent --snapshot-move-data=true
```
or
```yaml
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: example-backup
  namespace: openshift-adp
spec:
  snapshotMoveData: true
  includedNamespaces:
  - mysql-persistent
  storageLocation: dpa-sample-1
  ttl: 720h0m0s
```
# Upgrading from OADP 1.4

> **NOTE:** Always upgrade to next minor version, do NOT skip versions. To update to higher version, please upgrade one channel at a time. Example: to upgrade from 1.1 to 1.3, upgrade first to 1.2, then to 1.3.

## Changes from OADP 1.4 to 1.5

- Velero was updated from version 1.14 to 1.16 (Changes reference: TODO https://velero.io/docs/v1.13/upgrade-to-1.13/ https://velero.io/docs/v1.14/upgrade-to-1.14/)

    From this update:

    TODO

## Upgrade steps

### Backup the DPA configuration

Save your current DataProtectionApplication (DPA) CustomResource config, be sure to remember the values.

For example:
```
oc get dpa -n openshift-adp -o yaml > dpa.orig.backup
```

### Upgrade the OADP Operator

For general operator upgrade instructions please review the [OpenShift documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/operators/administrator-tasks#olm-upgrading-operators)
* Change the Subscription for the OADP Operator from `stable-1.4` to `stable`
* Allow time for the operator and containers to update and restart

> **NOTE:** You need to be at least on OCP 4.19 to be able to upgrade to OADP 1.5. On previous versions of OCP, OADP 1.5 will not be available for installation.

### Convert your DPA to the new version

If you are using `spec.configuration.restic` field, you need to use `spec.configuration.nodeAgent` now. Example
```diff
 spec:
   configuration:
-    restic:
+    nodeAgent:
       enable: true
+      uploaderType: restic
```

### Verify the upgrade

Follow theses [basic install verification](../docs/install_olm.md#verify-install) to verify the installation.

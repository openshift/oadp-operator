<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Usage of Velero <code>--features</code> flag</h1>
<hr style="height:1px;border:none;color:#333;">

Some of the new features in Velero are released as beta features, behind feature 
flags, which are not enabled by default during the Velero installation. In order 
to provide `--features` flag values, you need to use the specify the flags under 
`configuration.velero.featureFlags:` in the `oadp.openshift.io/v1alpha1_dpa.yaml` file
during deployment.

Some of the usage instances of the `--features` flag are as follows:
- Enabling Velero plugin for CSI: To enable CSI plugin you need to add two 
  things in the `oadp.openshift.io/v1alpha1_dpa.yaml` file during deployment.
  - First, add `csi` under the `configuration.velero.defaultPlugins` 
  - Second, add `EnableCSI` under `configuration.velero.featureFlags`
```
defaultPlugins:
- csi
featureFlags: EnableCSI
```

> **Note:** Starting with OADP 1.4, the CSI plugin is built into Velero core and `EnableCSI` is enabled automatically. You no longer need to add `csi` to `defaultPlugins` or `EnableCSI` to `featureFlags` for CSI snapshot support.
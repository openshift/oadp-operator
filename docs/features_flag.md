<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Usage of Velero <code>--features</code> flag</h1>
<hr style="height:1px;border:none;color:#333;">

Some of the new features in Velero are released as beta features, behind feature 
flags, which are not enabled by default during the Velero installation. In order 
to provide `--features` flag values, you need to use the specify the flags under 
`veleroFeatureFlags:` in the `oadp.openshift.io/v1alpha1_velero.yaml` file
during deployment.

Some of the usage instances of the `--features` flag are as follows:
- Enabling Velero plugin for CSI: To enable CSI plugin you need to add two 
  things in the `oadp.openshift.io/v1alpha1_velero.yaml` file during deployment.
  - First, add `csi` under the `defaultVeleroPlugins` 
  - Second, add `EnableCSI` under `veleroFeatureFlags`
```
defaultVeleroPlugins:
- csi
veleroFeatureFlags: EnableCSI
```
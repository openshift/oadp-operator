<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Plugins Customization</h1>
<hr style="height:1px;border:none;color:#333;">

### Configure Velero Plugins

There are mainly two categories of Velero plugins that can be specified while 
installing Velero:

1. `defaultVeleroPlugins`:<br>
   There are five types of default Velero plugins can be installed: 
   `AWS`, `GCP`, `Azure` and `OpenShift`, and `CSI`. For installation, 
   you need to specify them in the `oadp_v1alpha1_velero_cr.yaml` file 
   during deployment.

   ```
    apiVersion: oadp.openshift.io/v1alpha1
    kind: Velero
    metadata:
      name: velero-sample
    spec:
      defaultVeleroPlugins:
      - azure
      - gcp
      - aws
      - openshift    
   ```
   The above specification will install Velero with four of the default plugins.
   
2. `customVeleroPlugins`:<br>
   For installation of custom Velero plugins, you need to specify the plugin 
   `image` and plugin `name` in the `oadp_v1alpha1_velero_cr.yaml` file during 
   deployment.

   For instance, 
   ```
    apiVersion: oadp.openshift.io/v1alpha1
    kind: Velero
    metadata:
      name: velero-sample
    spec:
      defaultVeleroPlugins:
      - azure
      - gcp
      customVeleroPlugins:
      - name: custom-plugin-example
        image: quay.io/example-repo/custom-velero-plugin   
   ```
   The above specification will install Velero with three plugins: 
   `azure`, `gcp`, and `custom-plugin-example`.

***
## Plugin customization
***

### Configure Velero Plugins

There are mainly two categories of velero plugins that can be specified while installing Velero:

1. `default-velero-plugins`:<br>
   4 types of default velero plugins can be installed - AWS, GCP, Azure and OpenShift. For installation, you need to specify them in the `konveyor.openshift.io_v1alpha1_velero_cr.yaml` file during deployment.
   ```
    apiVersion: konveyor.openshift.io/v1alpha1
    kind: Velero
    metadata:
      name: example-velero
    spec:
      default_velero_plugins:
      - azure
      - gcp
      - aws
      - openshift    
   ```
   The above specification will install Velero with all the 4 default plugins.
   
2. `custom-velero-plugin`:<br>
   For installation of custom velero plugins, you need to specify the plugin `image` and plugin `name` in the `konveyor.openshift.io_v1alpha1_velero_cr.yaml` file during deployment.

   For instance, 
   ```
    apiVersion: konveyor.openshift.io/v1alpha1
    kind: Velero
    metadata:
      name: example-velero
    spec:
      default_velero_plugins:
      - azure
      - gcp
      custom_velero_plugins:
      - name: custom-plugin-example
        image: quay.io/example-repo/custom-velero-plugin   
   ```
   The above specification will install Velero with 3 plugins (azure, gcp and custom-plugin-example).

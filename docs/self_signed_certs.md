***
## Use self-sigend certificate
***

### Use Velero with a storage provider secured by a self-signed certificate

If you are using an S3-Compatible storage provider that is secured with a self-signed certificate, connections to the object store may fail with a `certificate signed by unknown authority` message. In order to proceed, you will have to specify the a base64 encoded certificate string as a value of the `caCert` spec under the `object_storage` configuration in the velero CR.

Your CR might look somewhat like this:

```
apiVersion: konveyor.openshift.io/v1alpha1
kind: Velero
metadata:
  name: example-velero
spec:
  use_upstream_images: true
  default_velero_plugins:
  - aws
  - openshift
  backup_storage_locations:
  - name: default
    provider: aws
    object_storage:
      bucket: velero
      caCert: <base64_encoded_cert_string>
    config:
      region: us-east-1
      profile: "default"
      insecure_skip_tls_verify: "false"
      signature_version: "1"
      public_url: "https://m-oadp.apps.cluster-sdpampat0519.sdpampat0519.mg.dog8code.com"
      s3_url: "https://m-oadp.apps.cluster-sdpampat0519.sdpampat0519.mg.dog8code.com"
      s3_force_path_style: "true"
    credentials_secret_ref:
      name: cloud-credentials
      namespace: oadp-operator
  enable_restic: true
```
<b>Note:</b> Ensure that `insecure_skip_tls_verify` is set to `false` so that TLS is used.

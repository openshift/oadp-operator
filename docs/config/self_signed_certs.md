<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Use Self-Sigend Certificate</h1>
<hr style="height:1px;border:none;color:#333;">

### Use Velero with a storage provider secured by a self-signed certificate

If you are using an S3-Compatible storage provider that is secured with a 
self-signed certificate, connections to the object store may fail with a 
`certificate signed by unknown authority` message. In order to proceed, you will 
have to specify a base64 encoded certificate string as a value of the `caCert` 
spec, under the `objectStorage` configuration in the DataProtectionApplication (DPA) CR.

Your DPA CR might look somewhat like this:

```
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: velero-sample
spec:
  configuration:
    velero:
      defaultPlugins:
      - openshift
      - aws
    nodeAgent:
      enable: true
      uploaderType: restic
  backupLocations:
    - name: default
      velero:
        provider: aws
        default: true
        objectStorage:
          bucket: my-bucket
          prefix: my-prefix
          caCert: <base64_encoded_cert_string>
        config:
          region: us-east-1
          profile: "default"
          insecureSkipTLSVerify: "false"
          signatureVersion: "1"
          public_url: "https://m-oadp.apps.cluster-uid-123.uid-123.mg.dog8code.com"
          s3Url: "https://m-oadp.apps.cluster-uid-123.uid-123.mg.dog8code.com"
          s3ForcePathStyle: "true"
        credential:
          name: cloud-credentials
          key: cloud
  snapshotLocations:
    - name: default
      velero:
        provider: aws
        config:
          region: us-west-2
          profile: "default"

```
<b>Note:</b> Ensure that `insecureSkipTLSVerify` is set to `false` so that TLS 
is used.

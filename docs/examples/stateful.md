<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Stateful Application Backup/Restore - MySQL</h1>
<h2 align="center">Using an AWS s3 Bucket</h2>
<hr style="height:1px;border:none;color:#333;">

### Prerequisites
* Make sure the OADP operator is installed:

    `make deploy`

* Create a credentials secret for your aws bucket:

   `oc create secret generic cloud-credentials --namespace oadp-operator-system --from-file cloud=<CREDENTIALS_FILE_PATH>`

* Make sure your Velero CR is similar to this in `config/samples/oadp_v1alpha1_velero.yaml`

    ```
    apiVersion: oadp.openshift.io/v1alpha1
    kind: Velero
    metadata:
    name: velero-sample
    spec:
    # Add fields here
    olmManaged: false
    backupStorageLocations:
    - provider: aws
        default: true
        objectStorage:
        bucket: my-bucket-name
        credential:
        name: cloud-credentials
        key: cloud    
        config:
        region: us-east-1
        profile: default
    enableRestic: true
    defaultVeleroPlugins:
    - openshift
    - aws
    ```

* Install Velero + Restic:

  `oc create -n oadp-operator-system -f config/samples/oadp_v1alpha1_velero.yaml`

<hr style="height:1px;border:none;color:#333;">

### Create the Nginx deployment:
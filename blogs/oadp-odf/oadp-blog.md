# Guide: Backup and Restore Stateful Applications on OpenShift using OADP and ODF
<!--
We want to publish a blog that contains a guided example of backing up and restoring a CSI-based workload. This should contain:

-   OADP Overview
-   Installation
-   Application Overview
-   Backup/Restore of app showing CSI functionality
-   Upcoming features (Data Mover)
-->

OpenShift API for Data Protection (OADP) enables backup, restore, and disaster recovery of applications on an OpenShift cluster. Data that can be protected with OADP include k8s resource objects, persistent volumes, and internal images.
The OpenShift API for Data Protection (OADP) is designed to protect Application Workloads on a single OpenShift cluster.

Red Hat® OpenShift® Data Foundation is software-defined storage for containers. Engineered as the data and storage services platform for Red Hat OpenShift, Red Hat OpenShift Data Foundation helps teams develop and deploy applications quickly and efficiently across clouds.

## Table of Content
- [Guide: Backup and Restore Stateful Applications on OpenShift using OADP and ODF](#guide-backup-and-restore-stateful-applications-on-openshift-using-oadp-and-odf)
  - [Table of Content](#table-of-content)
  - [Pre-requisites](#pre-requisites)
  - [Installing OpenShift Data Foundation Operator](#installing-openshift-data-foundation-operator)
    - [Creating StorageSystem](#creating-storagesystem)
    - [Creating Object Bucket Claim](#creating-object-bucket-claim)
    - [Gathering information from Object Bucket](#gathering-information-from-object-bucket)
  - [Deploying an application](#deploying-an-application)
  - [Installing OpenShift API for Data Protection Operator](#installing-openshift-api-for-data-protection-operator)
    - [Create credentials secret for OADP Operator to use](#create-credentials-secret-for-oadp-operator-to-use)
  - [Back up application](#back-up-application)
  - [Uhh what? Disasters?](#uhh-what-disasters)
  - [Restore application](#restore-application)
  - [Conclusion](#conclusion)
    - [Remove workloads from this guide](#remove-workloads-from-this-guide)


<!-- In this guide, we will cover:
- Operators installation -->
  <!-- - Local Storage Operator -->
  <!-- - OpenShift Data Foundation
  - OpenShift API for Data Protection Operator
- Application deployment
- Application protection
- A disaster scenario
- Application recovery from disaster -->

The term *Project* and *namespace* maybe used interchangeably in this guide.
## Pre-requisites
- Terminal environment
  - Your terminal has the following commands
    - [oc](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.9/html/cli_tools/openshift-cli-oc) binary
    - [git](https://git-scm.com/downloads) binary
    - velero
      - Set alias to use command from cluster (preferred)
        - `alias velero='oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero'`
      - [Download velero from Github Release](https://velero.io/docs/v1.8/basic-install/#option-2-github-release)
  - Alternatively enter prepared environment in your terminal with `docker run -it ghcr.io/kaovilai/oadp-cli:v1.0.1 bash`
    - source can be found at https://github.com/kaovilai/oadp-cli
- [Authenticate as Cluster Admin inside your environment](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.9/html/cli_tools/openshift-cli-oc#cli-logging-in_cli-developer-commands) of an OpenShift 4.9 Cluster.
- Your cluster meets the minimum requirement for [OpenShift Data Foundation](https://access.redhat.com/documentation/en-us/red_hat_openshift_data_foundation/4.9/html/planning_your_deployment/infrastructure-requirements_rhodf#minimum-deployment-resource-requirements) in Internal Mode deployment
  - 3 worker nodes, each with at least:
    - 8 logical CPU
    - 24 GiB memory
    - 1+ storage devices

## Installing OpenShift Data Foundation Operator
We will be using OpenShift Data Foundation to simplify application deployment across cloud providers which will be covered in the next section.

1. Open the OpenShift Web Console by navigating to the url below, make sure you are in Administrator view, not Developer.

   ```sh
   oc get route console -n openshift-console -ojsonpath="{.spec.host}"
   ```
   Authenticate with your credentials if necessary.

<!-- 2. Navigate to *OperatorHub* from the side menu and install **Local Storage Operator**
   
   ![Local Storage](localStorage.png)
   
   Leaving everything as default, click through until the installation finished. -->
2. Navigate to *OperatorHub*, search for and install **OpenShift Data Foundation**

   ![OpenShift Data Foundation Installation](odfInstall.png)

### Creating StorageSystem

![OpenShift Data Foundation Installation finished](ODFfinishedInstall.png)

1. Click *Create StorageSystem* button after it turned blue.
    
<!-- 4. Select *Create a new StorageClass using local storage devices* and click *Next*.
    ![](ODFlocalStorageDev.png) -->
2. Select *Create a new StorageClass* and follow the *Creating an OpenShift Data Foundation cluster* steps for your cloud provider.
   - [Amazon Web Services(AWS)](https://access.redhat.com/documentation/en-us/red_hat_openshift_data_foundation/4.9/html/deploying_openshift_data_foundation_using_amazon_web_services/deploy-using-dynamic-storage-devices-aws#creating-an-openshift-data-foundation-service_cloud-storage)
   - [Google Cloud (GCP)](https://access.redhat.com/documentation/en-us/red_hat_openshift_data_foundation/4.9/html/deploying_and_managing_openshift_data_foundation_using_google_cloud/deploying_openshift_data_foundation_on_google_cloud#creating-an-openshift-data-foundation-service_gcp)
   - [Microsoft Azure](https://access.redhat.com/documentation/en-us/red_hat_openshift_data_foundation/4.9/html/deploying_openshift_data_foundation_using_microsoft_azure_and_azure_red_hat_openshift/deploying-openshift-data-foundation-on-microsoft-azure_azure#creating-an-openshift-data-foundation-service_azure)
   - [VMware vSphere](https://access.redhat.com/documentation/en-us/red_hat_openshift_data_foundation/4.9/html/deploying_openshift_data_foundation_on_vmware_vsphere/deploy-using-dynamic-storage-devices-vmware#creating-an-openshift-data-foundation-service_cloud-storage)

### Creating Object Bucket Claim
Object Bucket Claim creates a persistent storage bucket for Velero to store backed up kubernetes manifests.

1. Navigate to *Storage* > *Object Bucket Claim* and click *Create Object Bucket Claim*
   ![](ObjectBucketClaimCreate.png)
   Note the Project you are currently in. You can create a new Project or leave as *default*

2. set the following values:
   - ObjectBucketClaim Name:  `oadp-bucket`
   - StorageClass: `openshift-storage.noobaa.io`
   - BucketClass: `noobaa-default-bucket-class`

   ![](ObjectBucketClaimFields.png)

3. Click *Create*

   ![](ObjectBucketClaimReady.png)
   When the *Status* is *Bound*, the bucket is ready.

### Gathering information from Object Bucket
1. Click on Object Bucket *obc-default-oadp-bucket* at local endpoint are using is an S3 storage provided by OpenShift Data Foundation with local endpoint at 
   ![](obc-default-oadp-bucket.png)
   Take note of the following information which may differ from the guide:
    - `.spec.endpoint.bucketName`. Seen in my screenshot as `oadp-bucket-c21e8d02-4d0b-4d19-a295-cecbf247f51f`
    - `.spec.endpoint.bucketHost`: Seen in my screenshot as `s3.openshift-storage.svc`

2. Navigate to *Storage* > *Object Bucket Claim* > *oadp-bucket*. Ensure you are in the same *Project* used to create *oadp-bucket*.
3. Click on oadp-secret in the bottom left to view bucket secrets
4. Click Reveal values to see the bucket secret values. Copy data from *AWS_ACCESS_KEY_ID* and *AWS_SECRET_ACCESS_KEY* and save it as we'll need it later when installing the OADP Operator.
   
   Note: regardless of the cloud provider, the secret field names seen here may contain *AWS_\**.
5. Now you should have the following information:
   - *bucketName*
   - *bucketHost*
   - *AWS_ACCESS_KEY_ID*
   - *AWS_SECRET_ACCESS_KEY*
## Deploying an application
Since we are using OpenShift Data Foundation, we can use common application definition across cloud providers regardless of available storage class.

Clone our demo apps repository and enter the cloned repository.
```sh
git clone https://github.com/kaovilai/mig-demo-apps --single-branch -b oadp-blog-rocketchat
cd mig-demo-apps
```

Apply rocket chat manifests.
```sh
oc apply -f apps/rocket-chat/manifests/
```

Navigate to rocket-chat setup wizard url obtained by this command into your browser.
```sh
oc get route rocket-chat -n rocket-chat -ojsonpath="{.spec.host}"
```

Enter your setup information. remember it as we may need it later.

Skip to step 4, select "Keep standalone", and "continue".

Press "Go to your workspace"

![First message!](readyToUse.png)

"Enter"

Go to Channel #general and type some message

![First message!](firstMessage.png)

## Installing OpenShift API for Data Protection Operator
You can install the OADP Operator from the Openshift's OperatorHub. 
You can search for the operator using keywords such as `oadp` or `velero`.

![OADP-OLM-1](OADP-OLM-1.png)

Now click on `Install`

<!-- ![OADP-OLM-1](/docs/images/click-install.png) -->

Finally, click on `Install` again. This will create *Project* `openshift-adp` 
if it does not exist, and install the OADP operator in it.

<!-- ![OADP-OLM-1](/docs/images/click-install-again.png) -->

### Create credentials secret for OADP Operator to use
We will now create secret `cloud-credentials` using values obtained from Object Bucket Claim in *Project* `openshift-adp`.

From OpenShift Web Console side bar navigate to *Workloads* > *Secrets* and click *Create* > Key/value secret
![](secretKeyValCreate.png)

Fill out the following fields:
- Secret name: `cloud-credentials`
- Key: `cloud`
- Value:
  - Replace the values with your own values from earlier steps and enter it in the value field.
      ```
      [default]
      aws_access_key_id=<INSERT_VALUE>
      aws_secret_access_key=<INSERT_VALUE>
      ```
      *Note:* Do not use quotes while putting values in place of INSERT_VALUE Placeholders

![](secretKeyValFields.png)

<!-- ```
$ oc create secret generic cloud-credentials --namespace openshift-adp --from-file cloud=<CREDENTIALS_FILE_PATH>
``` -->

### Create the DataProtectionApplication Custom Resource
From side bars navigate to *Operators* > *Installed Operators* 

Create an instance of the DataProtectionApplication (DPA) CR by clicking on `Create Instance` as highlighted below:

![Velero-CR-1](/docs/images/dpa-cr.png)

Select *Configure via*: `YAML view`

Finally, copy the values provided below and update fields with comments with information obtained earlier.

The CR values are mentioned for ease of use. Please remember to mention `default: true` in backupStorageLocations if you intend on using the default backup storage location as shown below.

```
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: example-dpa
  namespace: openshift-adp
spec:
  configuration:
    velero:
      featureFlags:
        - EnableCSI
      defaultPlugins:
      - openshift
      - aws
      - csi
  backupLocations:
    - velero:
        default: true
        provider: aws
        credential:
            name: cloud-credentials
            key: cloud
        objectStorage:
            bucket: "oadp-bucket-c21e8d02-4d0b-4d19-a295-cecbf247f51f" #update this
            prefix: velero
        config:
            profile: default
            region: "localstorage"
            s3ForcePathStyle: "true"
            s3Url: "http://s3.openshift-storage.svc/" #update this if necessary
```
![Velero-CR-2](create-dpa-cr-yaml.png)

The object storage we are using is an S3 compatible storage provided by OpenShift Data Foundation. We are using custom s3Url capability of the aws velero plugin to access *OpenShift Data Foundation* local endpoint in velero.

Click *Create*
### Verify install

To verify all of the correct resources have been created, the following command
`oc get all -n openshift-adp` should look similar to:

```
NAME                                                     READY   STATUS    RESTARTS   AGE
pod/oadp-operator-controller-manager-67d9494d47-6l8z8    2/2     Running   0          2m8s
pod/oadp-velero-sample-1-aws-registry-5d6968cbdd-d5w9k   1/1     Running   0          95s
pod/velero-588db7f655-n842v                              1/1     Running   0          95s

NAME                                                       TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
service/oadp-operator-controller-manager-metrics-service   ClusterIP   172.30.70.140    <none>        8443/TCP   2m8s
service/oadp-velero-sample-1-aws-registry-svc              ClusterIP   172.30.130.230   <none>        5000/TCP   95s

NAME                    DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
daemonset.apps/restic   3         3         3       3            3           <none>          96s

NAME                                                READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/oadp-operator-controller-manager    1/1     1            1           2m9s
deployment.apps/oadp-velero-sample-1-aws-registry   1/1     1            1           96s
deployment.apps/velero                              1/1     1            1           96s

NAME                                                           DESIRED   CURRENT   READY   AGE
replicaset.apps/oadp-operator-controller-manager-67d9494d47    1         1         1       2m9s
replicaset.apps/oadp-velero-sample-1-aws-registry-5d6968cbdd   1         1         1       96s
replicaset.apps/velero-588db7f655                              1         1         1       96s
```

### Modifying VolumeSnapshotClass
Navigate to *Storage* > *VolumeSnapshotClasses* and click *ocs-storagecluster-rbdplugin-snapclass*

Click YAML view to modify values `deletionPolicy` and `labels` as shown below:

```diff
  apiVersion: snapshot.storage.k8s.io/v1
- deletionPolicy: Delete
+ deletionPolicy: Retain
  driver: openshift-storage.rbd.csi.ceph.com
  kind: VolumeSnapshotClass
  metadata:
    name: ocs-storagecluster-rbdplugin-snapclass
+   labels:
+     velero.io/csi-volumesnapshot-class: "true"
```

Setting a `DeletionPolicy` of `Retain` on the *VolumeSnapshotClass* will preserve the volume snapshot in the storage system for the lifetime of the Velero backup and will prevent the deletion of the volume snapshot, in the storage system, in the event of a disaster where the namespace with the *VolumeSnapshot* object may be lost.

The Velero CSI plugin, to backup CSI backed PVCs, will choose the VolumeSnapshotClass in the cluster that has the same driver name and also has the `velero.io/csi-volumesnapshot-class: "true"` label set on it.
## Back up application
From side menu, navigate to *Operators* > *Installed Operators*
Under *Project* `openshift-adp`, click on *OADP Operator*.
Under *Provided APIs* > *Backup*, click on *Create instance*

![](backupCreateInstance.png)

In IncludedNamespaces, add `rocket-chat`

![](backupRocketChat.png)

Click *Create*.

The status of `restore` should eventually show `Phase: Completed`
## Uhh what? Disasters?
Someone forgot their breakfast and their brain is deprived of minerals. They proceeded to delete `rocket-chat` namespace.

Navigate to *Home* > *Projects* > `rocket-chat`
![](deleteRocketChat.png)

Confirm deletion by typing `rocket-chat` and click *Delete*.

Wait until Project `rocket-chat` is deleted.

Rocket Chat application URL should no longer work.
## Restore application
An eternity of time has passed.

You finally had breakfast and your brain is working again. Realizing the chat application is down, you decided to restore it.

From side menu, navigate to *Operators* > *Installed Operators*
Under *Project* `openshift-adp`, click on *OADP Operator*.
Under *Provided APIs* > *Restore*, click on *Create instance*
![](createRestoreInstance.png)

Under Backup Name, type `backup`

In IncludedNamespaces, add `rocket-chat`
check `restorePVs`

![](restoreRocketChat.png)

Click *Create*.

The status of `restore` should eventually show `Phase: Completed`.

After a few minutes, you should see the chat application up and running.
You can check via Workloads > Pods > Project: `rocket-chat` and see the following
![](rocketChatReady.png)

Try to access the chat application via URL:
```sh
oc get route rocket-chat -n rocket-chat -ojsonpath="{.spec.host}"
```
Check previous message exists.
![First message!](firstMessage.png)
## Conclusion
Phew.. what a ride. We have covered the basic usage of OpenShift API for Data Protection (OADP) Operator, Velero, and the OpenShift Data Foundation.

Data is protected! Good bye data loss! Oh, and eat your breakfast people!


### Remove workloads from this guide
```sh
oc delete ns openshift-adp rocket-chat openshift-storage
```

If openshift-storage *Project* is stuck, follow [troubleshooting guide](https://access.redhat.com/documentation/en-us/red_hat_openshift_data_foundation/4.9/html/troubleshooting_openshift_data_foundation/troubleshooting-and-deleting-remaining-resources-during-uninstall_rhodf).

If you have set velero alias per this guide, you can remove it by running the following command:
```sh
unalias velero
```
# Create a project and user with non-admin access that can execute an OADP backup

## Background
The purpose of this demonstration is to provide an example for OpenShift administrators and users how an admistrator may configure OADP and OpenShift Pipelines to provide non-adminstrators access to trigger an OADP backup and restore workflow.

This example uses [OpenShift Pipelines](https://cloud.redhat.com/blog/introducing-openshift-pipelines) and configures a tekton pipeline for a non-admin user that has access to OADP resources to trigger a backup.  The non-admin user can execute the pipeline but can not edit the pipeline.  The administrator is allowed to configure OADP for their users, and users can execute a backup or restore as needed with out the administrator intervention.

## Project Architecture
OpenShift Administrators can utilize OpenShift pipelines and OADP to best fit their own needs.  An example architecture is show below that provides OpenShift pipelines for backing up and restoring projects with limited roles for non-admins.  The non-admin in this case Joe or Sarah are allowed to trigger OADP backup and an OADP restore but neither Joe or Sarah can edit the OpenShift pipelines or accidently restore a backup to the wrong namespace.  Joe and Sarah will have a full history of all the executions of the backups and restores in the pipeline-runs section of the pipeline.  The OpenShift administrator may also create tekton [pipeline triggers](https://cloud.redhat.com/blog/guide-to-openshift-pipelines-part-6-triggering-pipeline-execution-from-github) to schedule a backup of a namespace based on a specific event.

![oadp-non-admin-diagram1](https://user-images.githubusercontent.com/138787/226448245-68712098-38c7-4b46-aaae-bba910f8dfc0.png)

An example with just one application
![Screenshot from 2023-03-21 09-15-02](https://user-images.githubusercontent.com/138787/226651959-b698bf0a-998f-4bfa-b2e1-46e012aa4442.png)

## Technical Details of this demonstration
A user may want to change the backup custom resource, or other aspects of this demo. Simply fork this git repository and update the settings and configuration. The following provides a more in depth technical specification.

* To change the backup or restore custom resource, update the [crd's in](oadp-tekton-container/)
* The oauth and some of the user settings can be found in the [demo_users](demo_users) directory
* Some of the templates used in this demonstration are templated and found in [install_templates/templates](install_templates). The [install.sh](install.sh) script executes `oc process` to substitute variables and renders to the directory of the users choice or by default to `/tmp/oadp_non_admin` 

* The parameters that users are allowed to set in the tekton pipeline are defined in [05-build-and-deploy.yaml](install_templates/templates/05-build-and-deploy.yaml).



## Known Issues
* Advanced backup and restore options are not included in the templates.

## Steps

### Prerequisites
* Install [OpenShift Pipelines](https://docs.openshift.com/container-platform/4.12/cicd/pipelines/installing-pipelines.html) 
* Check that [OADP is installed](https://docs.openshift.com/container-platform/4.12/backup_and_restore/application_backup_and_restore/installing/about-installing-oadp.html) and [configured with a DPA named dpa-sample](https://github.com/openshift/oadp-operator/blob/master/docs/install_olm.md#create-the-dataprotectionapplication-custom-resource)
* Check that the [nginx-example sample application](https://github.com/openshift/oadp-operator/blob/master/docs/examples/stateless.md) is installed

```
./check_prerequisites.sh -h
Check the prerequisites [-h|-i]
options:
h     Print this Help.
i     Install nginx-example

[whayutin@thinkdoe tekton-oadp-nonadmin]$
```


### First create non-admin users 
* **NOTE** Note this script is for demo purposes only, there has been very limited validation of this script. 
  *  This step can easily be done manually and the script skipped by executing the steps documented [here](https://www.redhat.com/sysadmin/openshift-htpasswd-oauth)
  *  If a user has been created manually, the created user requires the view role 
  ```
  oc create namespace $PROJECT
  oc adm policy add-role-to-user view $USER -n $PROJECT
  ```
* logged in as the kubeadmin user, execute the following:
```
cd demo_users
./create_demousers.sh -h
Create the OADP non-admin users

Syntax: scriptTemplate [-h|-n|-c|-p|-x|-d]
options:
h     Print this Help.
n     demouser base name
x     the project name
c     the number of users to be created
p     the common password
d     The directory where the htpasswd file will be saved
```

Example:
```
./create_demousers.sh -n buzz -c 2
```
This will create two new users in openshift called buzz1 and buzz2 with a default password of `passw0rd`.

* Please first confirm that you can log in as the demo user.  **NOTE:** It may take a few moments for the OCP oauth settings to reconcile. 

* If you are logged in as the admin, please log into OCP with the buzz user in another browser.
  * Please note your permissions once logged in.
  * Also note there are no pipelines created.

### Login as the non-admin user
* Test the non-admin user first before moving on.
* The user should be able to be logged in, and have view access to the created project/namespace.

### Setup the Tekton pipelines 

* logged in as the kubeadmin user, execute the following:

Using an example with a user called buzz1 in a project called buzz1
```
./install.sh -h
Create the OADP non-admin templates

Syntax: scriptTemplate [-h|-p|-u|-d]
options:
h     Print this Help.
p     Name of the Project or Namespace
u     Name of the non-admin user
d     The directory where the templates will be saved


./install.sh -p buzz1 -u buzz1 -d /tmp/buzz1
```

The project will be created and the user updated.

* Navigate to the pipelines menu as the buzz1 user

![Screenshot from 2023-03-17 10-36-42](https://user-images.githubusercontent.com/138787/225965236-3f78ea35-ef11-40ce-8c31-349c32cc3e56.png)


### Trigger a backup as a non-admin user
Log into Openshift as the non-admin user buzz1, and click `Pipelines`

* You should now see a new tekton pipeline created call `backup-pipeline`
  * Click `Actions`
    * **NOTE** you should see the user only has permissions to `Start` the pipeline
  * Click `Start`
    * Update the GIT_URL and GIT_BRANCH 
      * This is for demonstration purposes only and will later be removed.
      * The git repo should be a clone of oadp-operator and contain the directory `docs/tekton-oadp-nonadmin`
    * Give your backup an unique name, e.g. mybackuptest1
    * The `workspace` should be:
      * A VolumeClaimTemplate
      * In this demo a volume claim using the gp2-csi storage class was created.

![Screenshot from 2023-03-17 10-38-06](https://user-images.githubusercontent.com/138787/225965457-bd7fca53-9b71-45e8-a4a9-96739769b356.png)


* Watch and wait for the backup to complete

![Screenshot from 2023-03-17 10-39-19](https://user-images.githubusercontent.com/138787/225965741-71d82e2d-95a5-4f00-8ae1-ec5ffb83626b.png)


* Check the logs of the Tekton tasks, below is an example of a previous execution.


### Delete the application
Now that you have backed up an application, delete the application's namespace and we'll proceed to the restore pipeline.
**NOTE:** Deleting the namespace may require admin access
```
oc delete namespace nginx-example
```

### Restore the application
In the buzz1 project, click on `Pipelines` and the `restore-pipeline`

![Screenshot from 2023-03-21 08-38-26](https://user-images.githubusercontent.com/138787/226641215-40e03147-3690-47f2-89e1-9e8e171ba7bd.png)


Follow the same steps and the same `backup name` used in the backup pipeline.
* The backup name in the example was `mybackuptest1`

![Screenshot from 2023-03-21 08-39-47](https://user-images.githubusercontent.com/138787/226641262-7c97cfb3-ffa6-4bf3-893f-854cd3f70ec2.png)

The restore should run to completion.
![Screenshot from 2023-03-21 08-49-48](https://user-images.githubusercontent.com/138787/226644387-2320656a-fd6e-47c3-9a4e-fad71f2bf430.png)


The nginx-example application should be created and running:
```
[whayutin@thinkdoe docs]$ oc get all -n nginx-example
NAME                                    READY   STATUS    RESTARTS   AGE
pod/nginx-deployment-7754cc8446-pf4rd   1/1     Running   0          30m
pod/nginx-deployment-7754cc8446-tm2hg   1/1     Running   0          30m

NAME               TYPE           CLUSTER-IP       EXTERNAL-IP                                                               PORT(S)          AGE
service/my-nginx   LoadBalancer   172.30.196.113   a9640ce3a586744148859f932b41e851-1614434757.us-west-2.elb.amazonaws.com   8080:31746/TCP   30m

NAME                               READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/nginx-deployment   2/2     2            2           30m

NAME                                          DESIRED   CURRENT   READY   AGE
replicaset.apps/nginx-deployment-7754cc8446   2         2         2       30m

NAME                                HOST/PORT                                                                       PATH   SERVICES   PORT   TERMINATION   WILDCARD
route.route.openshift.io/my-nginx   my-nginx-nginx-example.apps.cluster-wdh02152023a.wdh02152023a.mg.dog8code.com          my-nginx   8080                 None

```

#### Complete
Thank you for walking through this OADP demonstration.

### Logs from the backup execution 
```
﻿import-images

step-oc

imagestream.image.openshift.io/toolbox imported

Name:			toolbox
Namespace:		buzz1
Created:		Less than a second ago
Labels:			<none>
Annotations:		openshift.io/image.dockerRepositoryCheck=2023-03-17T16:21:54Z
Image Repository:	image-registry.openshift-image-registry.svc:5000/buzz1/toolbox
Image Lookup:		local=false
Unique Images:		1
Tags:			1

latest
  tagged from registry.access.redhat.com/ubi9/toolbox:latest

  * registry.access.redhat.com/ubi9/toolbox@sha256:8d3c5489b5cb4c37d7b402a43adb4e8ac87c84b63c59f418ef42943786b5d783
      Less than a second ago

Image Name:	toolbox:latest
Docker Image:	registry.access.redhat.com/ubi9/toolbox@sha256:8d3c5489b5cb4c37d7b402a43adb4e8ac87c84b63c59f418ef42943786b5d783
Name:		sha256:8d3c5489b5cb4c37d7b402a43adb4e8ac87c84b63c59f418ef42943786b5d783
Created:	Less than a second ago
Annotations:	image.openshift.io/dockerLayersOrder=ascending
Image Size:	202.7MB in 2 layers
Layers:		79.17MB	sha256:2a625e4afab51b49edb0e5f4ff37d8afbb20ec644ed1e68641358a6305557de3
		123.5MB	sha256:d58ac2930bccf7a92710daa578557736aae56e265127f342cf6f700582782b22
Image Created:	3 weeks ago
Author:		<none>
Arch:		amd64
Command:	/bin/sh -c /bin/sh
Working Dir:	<none>
User:		<none>
Exposes Ports:	<none>
Docker Labels:	architecture=x86_64
		build-date=2023-02-22T14:02:03
		com.github.containers.toolbox=true
		com.redhat.component=toolbox-container
		com.redhat.license_terms=https://www.redhat.com/en/about/red-hat-end-user-license-agreements#UBI
		description=The Universal Base Image is designed and engineered to be the base layer for all of your containerized applications, middleware and utilities. This base image is freely redistributable, but Red Hat only supports Red Hat technologies through subscriptions for Red Hat products. This image is maintained by Red Hat and updated regularly.
		distribution-scope=public
		io.buildah.version=1.27.3
		io.k8s.description=The Universal Base Image is designed and engineered to be the base layer for all of your containerized applications, middleware and utilities. This base image is freely redistributable, but Red Hat only supports Red Hat technologies through subscriptions for Red Hat products. This image is maintained by Red Hat and updated regularly.
		io.k8s.display-name=Red Hat Universal Base Image 9
		io.openshift.expose-services=
		io.openshift.tags=base rhel9
		maintainer=Oliver Gutiérrez <ogutierrez@redhat.com>
		name=toolbox-container
		release=11
		summary=Base image for creating UBI toolbox containers
		url=https://access.redhat.com/containers/#/registry.access.redhat.com/toolbox-container/images/9.1.0-11
		usage=This image is meant to be used with the toolbox command
		vcs-ref=5581af47138aa1a57cc352d44f1d338280828ca2
		vcs-type=git
		vendor=Red Hat, Inc.
		version=9.1.0
Environment:	PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
		container=oci
		NAME=toolbox-container
		VERSION=9.1.0




checkout

step-clone

+ '[' false = true ']'
+ '[' false = true ']'
+ '[' false = true ']'
+ CHECKOUT_DIR=/workspace/output/
+ '[' true = true ']'
+ cleandir
+ '[' -d /workspace/output/ ']'
+ rm -rf /workspace/output//lost+found
+ rm -rf '/workspace/output//.[!.]*'
+ rm -rf '/workspace/output//..?*'
+ test -z ''
+ test -z ''
+ test -z ''
+ /ko-app/git-init -url=https://github.com/weshayutin/oadp-operator.git -revision=tekton-non-admin -refspec= -path=/workspace/output/ -sslVerify=true -submodules=true -depth=1 -sparseCheckoutDirectories=
{"level":"info","ts":1679070132.658233,"caller":"git/git.go:176","msg":"Successfully cloned https://github.com/weshayutin/oadp-operator.git @ 92a7a898baffd77e65ddbf0a1454eb2f080e2687 (grafted, HEAD, origin/tekton-non-admin) in path /workspace/output/"}
{"level":"info","ts":1679070132.744676,"caller":"git/git.go:215","msg":"Successfully initialized and updated submodules in path /workspace/output/"}
+ cd /workspace/output/
++ git rev-parse HEAD
+ RESULT_SHA=92a7a898baffd77e65ddbf0a1454eb2f080e2687
+ EXIT_CODE=0
+ '[' 0 '!=' 0 ']'
+ printf %s 92a7a898baffd77e65ddbf0a1454eb2f080e2687
+ printf %s https://github.com/weshayutin/oadp-operator.git


listfiles

step-list-workspace-files

+ cd /workspace/debug/docs/tekton-oadp-nonadmin/backup_cr/
+ ls -la
total 12
drwxrwsr-x. 2 65532 1000740000 4096 Mar 17 16:22 .
drwxrwsr-x. 4 65532 1000740000 4096 Mar 17 16:22 ..
-rw-rw-r--. 1 65532 1000740000  189 Mar 17 16:22 backup.yaml


triggerbackup

step-oc

echo the BACKUP_NAME parameter
buzz1backup1

cat the original backup cr
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: BACKUP_NAME
  namespace: openshift-adp
spec:
  includedNamespaces:
  - nginx-example
  storageLocation: dpa-sample-1
  ttl: 720h0m0s

Update the backup cr's name

cat the updated backup cr
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: buzz1backup1
  namespace: openshift-adp
spec:
  includedNamespaces:
  - nginx-example
  storageLocation: dpa-sample-1
  ttl: 720h0m0s

Finally create the backup
backup.velero.io/buzz1backup1 created

Get the details and status of the backup
apiVersion: velero.io/v1
kind: Backup
metadata:
  annotations:
    velero.io/source-cluster-k8s-gitversion: v1.25.4+a34b9e9
    velero.io/source-cluster-k8s-major-version: "1"
    velero.io/source-cluster-k8s-minor-version: "25"
  creationTimestamp: "2023-03-17T16:22:27Z"
  generation: 2
  labels:
    velero.io/storage-location: dpa-sample-1
  name: buzz1backup1
  namespace: openshift-adp
  resourceVersion: "23218594"
  uid: 451756d7-721a-4a07-be88-e6e255cea58c
spec:
  csiSnapshotTimeout: 10m0s
  defaultVolumesToRestic: false
  includedNamespaces:
  - nginx-example
  storageLocation: dpa-sample-1
  ttl: 720h0m0s
  volumeSnapshotLocations:
  - dpa-sample-1
status:
  expiration: "2023-04-16T16:22:27Z"
  formatVersion: 1.1.0
  phase: InProgress
  startTimestamp: "2023-03-17T16:22:27Z"
  version: 1



checkbackupstatus

step-oc

echo the BACKUP_NAME parameter
buzz1backup1

InProgressInProgress
InProgress
InProgress
InProgress
InProgress
InProgress
InProgress
InProgress
Completed


finalstatus

step-oc

echo the BACKUP_NAME parameter
buzz1backup1

apiVersion: velero.io/v1
kind: Backup
metadata:
  annotations:
    velero.io/source-cluster-k8s-gitversion: v1.25.4+a34b9e9
    velero.io/source-cluster-k8s-major-version: "1"
    velero.io/source-cluster-k8s-minor-version: "25"
  creationTimestamp: "2023-03-17T16:22:27Z"
  generation: 5
  labels:
    velero.io/storage-location: dpa-sample-1
  name: buzz1backup1
  namespace: openshift-adp
  resourceVersion: "23219258"
  uid: 451756d7-721a-4a07-be88-e6e255cea58c
spec:
  csiSnapshotTimeout: 10m0s
  defaultVolumesToRestic: false
  includedNamespaces:
  - nginx-example
  storageLocation: dpa-sample-1
  ttl: 720h0m0s
  volumeSnapshotLocations:
  - dpa-sample-1
status:
  completionTimestamp: "2023-03-17T16:23:11Z"
  expiration: "2023-04-16T16:22:27Z"
  formatVersion: 1.1.0
  phase: Completed
  progress:
    itemsBackedUp: 42
    totalItems: 42
  startTimestamp: "2023-03-17T16:22:27Z"
  version: 1


```
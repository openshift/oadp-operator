# Create a project and user with non-admin access that can execute an OADP backup

## steps

Using an example with a user called test05 in a project call test05
```
./install.sh -p test05 -u test05 -d /tmp/test05
```

The user and project will be created, however the user needs a password.

* follow the documented steps here: https://www.redhat.com/sysadmin/openshift-htpasswd-oauth

```
htpasswd  -B -b ~/OADP/passwd_file test05 passw0rd
```

* Typically I destroy the old htpasswd config in OCP and recreate
* Upload the file or paste creds


## In another browser
* log in as the new user and kick off the pipeline 


![Screenshot from 2023-03-16 14-29-23](https://user-images.githubusercontent.com/138787/225745231-b056152c-b115-4e89-809a-ac36613bb886.png)



## Logs from all runs 
```
﻿import-images

step-oc

imagestream.image.openshift.io/toolbox imported

Name:			toolbox
Namespace:		test06
Created:		Less than a second ago
Labels:			<none>
Annotations:		openshift.io/image.dockerRepositoryCheck=2023-03-16T20:26:17Z
Image Repository:	image-registry.openshift-image-registry.svc:5000/test06/toolbox
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
{"level":"info","ts":1678998395.5686452,"caller":"git/git.go:176","msg":"Successfully cloned https://github.com/weshayutin/oadp-operator.git @ 7d669c4b360714ea24df4a942721b5b4ef7fa343 (grafted, HEAD, origin/tekton-non-admin) in path /workspace/output/"}
{"level":"info","ts":1678998395.6642838,"caller":"git/git.go:215","msg":"Successfully initialized and updated submodules in path /workspace/output/"}
+ cd /workspace/output/
++ git rev-parse HEAD
+ RESULT_SHA=7d669c4b360714ea24df4a942721b5b4ef7fa343
+ EXIT_CODE=0
+ '[' 0 '!=' 0 ']'
+ printf %s 7d669c4b360714ea24df4a942721b5b4ef7fa343
+ printf %s https://github.com/weshayutin/oadp-operator.git


listfiles

step-list-workspace-files

+ cd /workspace/debug
+ ls -la
total 312
drwxrwsr-x. 16 root  1000890000   4096 Mar 16 20:26 .
drwxrwsrwx.  3 root  1000890000     19 Mar 16 20:26 ..
-rw-rw-r--.  1 65532 1000890000    105 Mar 16 20:26 .ci-operator.yaml
drwxrwsr-x.  8 65532 1000890000   4096 Mar 16 20:26 .git
drwxrwsr-x.  4 65532 1000890000   4096 Mar 16 20:26 .github
-rw-rw-r--.  1 65532 1000890000    153 Mar 16 20:26 .gitignore
-rw-rw-r--.  1 65532 1000890000   2355 Mar 16 20:26 .travis.yml
-rw-rw-r--.  1 65532 1000890000    849 Mar 16 20:26 Dockerfile
-rw-rw-r--.  1 65532 1000890000  10759 Mar 16 20:26 LICENSE
-rw-rw-r--.  1 65532 1000890000  20735 Mar 16 20:26 Makefile
-rw-rw-r--.  1 65532 1000890000    266 Mar 16 20:26 OWNERS
-rw-rw-r--.  1 65532 1000890000    190 Mar 16 20:26 OWNERS_ALIASES
-rw-rw-r--.  1 65532 1000890000    225 Mar 16 20:26 PROJECT
-rw-rw-r--.  1 65532 1000890000   7050 Mar 16 20:26 README.md
drwxrwsr-x.  3 65532 1000890000   4096 Mar 16 20:26 api
drwxrwsr-x.  4 65532 1000890000   4096 Mar 16 20:26 blogs
drwxrwsr-x.  2 65532 1000890000   4096 Mar 16 20:26 build
drwxrwsr-x.  5 65532 1000890000   4096 Mar 16 20:26 bundle
-rw-rw-r--.  1 65532 1000890000    985 Mar 16 20:26 bundle.Dockerfile
-rw-rw-r--.  1 65532 1000890000    111 Mar 16 20:26 codecov.yml
drwxrwsr-x. 11 65532 1000890000   4096 Mar 16 20:26 config
drwxrwsr-x.  2 65532 1000890000   4096 Mar 16 20:26 controllers
drwxrwsr-x.  2 65532 1000890000   4096 Mar 16 20:26 deploy
drwxrwsr-x.  9 65532 1000890000   4096 Mar 16 20:26 docs
-rw-rw-r--.  1 65532 1000890000   7147 Mar 16 20:26 go.mod
-rw-rw-r--.  1 65532 1000890000 157055 Mar 16 20:26 go.sum
drwxrwsr-x.  2 65532 1000890000   4096 Mar 16 20:26 hack
-rw-rw-r--.  1 65532 1000890000   7135 Mar 16 20:26 main.go
drwxrwsr-x.  3 65532 1000890000   4096 Mar 16 20:26 must-gather
drwxrwsr-x.  5 65532 1000890000   4096 Mar 16 20:26 pkg
drwxrwsr-x.  3 65532 1000890000   4096 Mar 16 20:26 tests


triggerbackup

step-oc

echo the BACKUP_NAME parameter
asdfasdfasf

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
  name: asdfasdfasf
  namespace: openshift-adp
spec:
  includedNamespaces:
  - nginx-example
  storageLocation: dpa-sample-1
  ttl: 720h0m0s

Finally create the backup
backup.velero.io/asdfasdfasf created

Get the details and status of the backup
apiVersion: velero.io/v1
kind: Backup
metadata:
  annotations:
    velero.io/source-cluster-k8s-gitversion: v1.25.4+a34b9e9
    velero.io/source-cluster-k8s-major-version: "1"
    velero.io/source-cluster-k8s-minor-version: "25"
  creationTimestamp: "2023-03-16T20:26:49Z"
  generation: 2
  labels:
    velero.io/storage-location: dpa-sample-1
  name: asdfasdfasf
  namespace: openshift-adp
  resourceVersion: "22244019"
  uid: 694e2c70-e7f7-45a5-8ad0-4eaf6b8059bd
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
  expiration: "2023-04-15T20:26:49Z"
  formatVersion: 1.1.0
  phase: InProgress
  startTimestamp: "2023-03-16T20:26:49Z"
  version: 1

```

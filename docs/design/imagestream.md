# Internal Image backup design

Our new approach to backing up images will not require use of a registry.
OADP Operator will contain a controller that will watch for ImageStreamsBackup Custom Resource.

Alternative approach:
- Writing to S3 from within the Velero Plugin
  - Why we have decided against this approach?
    - Plugins in velero can only process one backupItem at a time
      - Makes it difficult to deduplicate images when you only see one image at a time
    - Post Backup plugin is not yet available in Velero
      - This might be a good option if we want to write to S3 from within the Velero Plugin if and when it is available
- Backup internal registry's PV directly
  - We don't have granular control over what images will be backed up and snapshots are not movable until data mover is available

Why new controller?:
- We want to not be limited by the velero plugin system for backing up OpenShift internal images which is specific to OpenShift distribution of Kubernetes
- The deduplication benefits outweighs the benefits from staying within the velero plugin system

```yaml
apiVersion: v1alpha1
kind: ImageStreamsBackup
metadata:
  name: my-backup
spec:
  startBackup: false
  veleroBackupReference:
    name: my-backup
    namespace: my-namespace
    startOnCompletion: true
  backupLocation:
    name: default
    velero:
      provider: aws
      default: true
      objectStorage:
        bucket: my-bucket
        prefix: my-prefix
      config:
        region: us-east-1
        profile: "default"
      credential:
        name: my-custom-name
        key: cloud

  imageStreams:
    - name: my-image-stream
      dockerImageReferences:
        - dockerImageReference: image-registry.openshift-image-registry.svc:5000/openshift/apicast-gateway@sha256:313df5722ddd866d43758af161e5932dfd4648b99a5c57acfddfc2a955669fe8
          tags:
            - latest
            - v1.0.0
            - v1.0.1
```
`startBackup: false` will prevent the operator from starting a backup. This is useful if you are waiting for velero backup to complete.
When the user is ready to start backing up, they will mark `startBackup: true` on the custom resource.

`veleroBackupReference:` (optional) is the name of a velero backup which operator will check for completion before allowing the backup to start.

`veleroBackupReference.startOnCompletion:` (optional) if `true`, operator will start the backup if the velero backup is complete.

`backupLocation:` (optional) is the name of the DPA compatible backup location to use.

`imageStreams` is a list of imagestreams to include in the backup and can be created and appended to by a human operator, or by velero plugin when scanning a backupItem.

`imageStreams[].dockerImageReferences` is a list of dockerImageReferences for the imagestream. Each dockerImageReference must be a digest type to be accepted by the operator.

`imageStreams[].dockerImageReferences[].tags` is a list of tags to create from this dockerImageReference upon restore.

ImageStreamsRestore will most likely be performed by a human operator and will be used to restore imagestreams from a ImageStreamsBackup before restoring a dependent velero backup.

```yaml
apiVersion: v1alpha1
kind: ImageStreamsRestore
metadata:
  name: my-restore
spec:
  ImageStreamsBackupReference:
    name: my-backup
  backupLocation:
    name: default
    velero:
      provider: aws
      default: true
      objectStorage:
        bucket: my-bucket
        prefix: my-prefix
      config:
        region: us-east-1
        profile: "default"
      credential:
        name: my-custom-name
        key: cloud
  imageStreams:
    - name: my-image-stream
      dockerImageReferences:
        - dockerImageReference: image-registry.openshift-image-registry.svc:5000/openshift/apicast-gateway@sha256:313df5722ddd866d43758af161e5932dfd4648b99a5c57acfddfc2a955669fe8
          tags:
            - latest
            - v1.0.0
            - v1.0.1
```

The spec fields in the ImageStreamsRestore are identical to the ImageStreamsBackup except:
- startBackup
- veleroBackupReference

Source registry path in dockerImageReference will be ignored if inaccessble (ie. from another cluster's internal registry) and only the sha256 digest will be used when looking up image from object storage to restore to internal registry.

`ImageStreamsBackupReference` is the reference of the ImageStreamsBackup to restore from. The name is used when looking up backup from backup location.

To simplify the restore process, there is no startRestore field, and the operator will automatically start the restore.

How this solves previous problems:
- We no longer need a running registry to copy images to.
- We can process all the intended images in one go, allowing us to tar up without duplicating the images to object storage.
- Velero BackupItemAction can easily plug in to the system and create or update this CR.

You will create ImageStreamsRestore prior to restoring from a velero backup.


# How we backup internal images from ImageStreams today:

Through use of openshift-velero-plugin added to velero server and a registry deployment by OADP when `BackupImages: true` is set in DataProtectionApplication the backup workflow is as follows:
- User creates Backup covering imagestreams
- [imagestreams/backup.go/BackupPlugin.Execute()](https://github.com/openshift/openshift-velero-plugin//blob/004e1f89e04e9f422d55e59c5caa07471f96d0f5/velero-plugins/imagestream/backup.go#L32) checks for annotations to determine if image should be backed up.
- BackupItemAction velero-plugins/common/backup.go checks for deployed OADP registry and add annotations to backup item.

Plugins registered with velero server:
- newImageStreamTagBackupPlugin
  - sets annotations on imagestreamtags that depend on others to be restored first
- newImageStreamTagRestorePlugin
  - Restore tag if tag is a reference or external image
  - Search for the tag corresponding to a particular imagestream to check if an image is present in the new namespace 
  - If the tag is not present, look it up in the old, backup namespace and use that tag to pull the particular image required
- newImageStreamBackupPlugin
  - Retrive internal registry and migration registry from annotaions.
  - For all the tags check imagestream has any associated imagestreamtags so that we know we need to restore the tags as well.
  - For all the Items in al the tags, fetch `dockerImageReference`, constructs source and destination path from `dockerImageReference` and `migrationRegistry`. Fetches all the images referenced by namespace from internal image registry of openshift, `image-registry.openshift-image-registry.svc:5000/`,  and push the same to to defined docker registry, `oadp-default-aws-registry-route-oadp-operator.apps.<route>`.
- newImageStreamRestorePlugin
  - Retrive `backupInternalRegistry`, `internalRegistry`, and `migrationRegistry`.
  - For all the tags check imagestream has any associated imagestreamtags, if so then, use the tag if it references an ImageStreamImage in the current namespace.
  - For all the Items in al the tags, fetch `dockerImageReference`, constructs source and destination path from `migrationRegistry` and `internalRegistry`. Fetches all the images that were pushed into registry initialized at backup time and pushes the same to internal openshift image registry.
- newImageTagRestorePlugin
  - Set SkipRestore to true, so that Image Tags are not restored

Notes:
Image streams provide a means of creating and updating container images in an on-going way. As improvements are made to an image, tags can be used to assign new version numbers and keep track of changes. This document describes how image streams are managed.


<h1 align="center">Troubleshooting<a id="troubleshooting"></a></h1>

Look at container logs from pods in project `oadp-operator-system`.

Problems are often identified in the following container logs
 - oadp-operator-controller-manager-*/manager
 - velero-*/velero

If you need help, first search if there is [already an issue filed](https://github.com/openshift/oadp-operator/issues) or [create a new issue](https://github.com/openshift/oadp-operator/issues/new). Attach pod logs if there are relevant information.

## Common Problems and Solutions
### Errors in velero-* pod
`Backup store contains invalid top-level directories: [someDirName]`

**Problem**: your object storage root/prefix directory contains directories not from velero's [approved list](https://github.com/vmware-tanzu/velero/blob/6f64052e94ef71c9d360863f341fe3c11e319f08/pkg/persistence/object_store_layout.go#L37-L43)

**Solution**:
1. Define prefix directory inside a storage bucket where backups are to be uploaded instead of object storage root. In your Velero CR set a prefix for velero to use in `Velero.spec.backupStorageLocations[*].objectStorage.prefix`
```yaml
      objectStorage:
        bucket: your-bucket-name
        prefix: <DirName>
```
2. Delete the offending directories from your object storage location.
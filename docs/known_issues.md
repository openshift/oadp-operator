<h1 align="center">Known Issues</h1>

- [[Documentation] Failed/PartiallyFailed Orphaned backup will not be removed by ObjectStorageSync](https://github.com/vmware-tanzu/velero/issues/4483)

- When using Azure as a provider, if the provider secret originally pointed to Service Principal credentials and then changed to use Storagekey Account credentials, it can create a blended view of credentials within the velero pod. 
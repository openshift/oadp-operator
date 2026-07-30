<h1 align="center">Known Issues</h1>

> **Note:** This document is not actively maintained. For up-to-date known issues, see the [OADP Jira project](https://issues.redhat.com/issues/?jql=project%20%3D%20OADP) and [OpenShift documentation](https://docs.openshift.com/container-platform/latest/backup_and_restore/application_backup_and_restore/troubleshooting.html).

- [[Documentation] Failed/PartiallyFailed Orphaned backup will not be removed by ObjectStorageSync](https://github.com/vmware-tanzu/velero/issues/4483)

- When using Azure as a provider, if the provider secret originally pointed to Service Principal credentials and then changed to use Storagekey Account credentials, it can create a blended view of credentials within the velero pod. 
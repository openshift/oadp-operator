# More frequent CSI polling for Windows workloads

When backing up volumes using the CSI plugin, Velero polls every five seconds to check for snaphandle creation. However, with windows workloads using Microsoft VSS, any freezing is automatically unfrozen after ten seconds. This means that if either 1) the volume takes more than 5 seconds for snapshot creation, or 2) there are 2 volumes for the same pod or VM, then the unfreeze will happen prematurely, risking backup integrity.

More information on the VSS 10-second freeze can be found at [Overview of Processing a Backup Under VSS](https://learn.microsoft.com/en-us/windows/win32/vss/overview-of-processing-a-backup-under-vss)

To address this, Velero has introduced an optional early frequent polling period. When enabled, the Velero CSI plugin will poll for snapshot completion once per second for the first 10 seconds and then fall back to the prior five-second polling interval until the CSI snapshot timeout has been reached.

To enable this feature in OADP, set `spec.configuration.velero.enableCSISnapshotEarlyFrequentPolling` to `true` in the DPA.

Note that this will increase the number of APIServer calls made by up to 8 for each volume. It may be possible that enabling this in conjunction with using a large value for `spec.configuration.velero.itemBlockWorkerCount` or `spec.configuration.velero.concurrentBackups` will result in rate limiter errors like the following:
```
time="2026-05-21T18:27:29Z" level=error msg="Failed to wait for VolumeSnapshot test-oadp-479/velero-mysql-data1-kq25b to become ReadyToUse within timeout 10m0s: failed to get volumesnapshot test-oadp-479/velero-mysql-data1-kq25b: client rate limiter Wait returned an error: rate: Wait(n=1) would exceed context deadline" backup=openshift-adp/mysql-63300dca-5542-11f1-8ae1-0a58ac1309fd cmd=/velero logSource="/workspace/pkg/backup/actions/csi/pvc_action.go:352" pluginName=velero
```

If this happens, try reducing the value for `spec.configuration.velero.itemBlockWorkerCount` or `spec.configuration.velero.concurrentBackups`.

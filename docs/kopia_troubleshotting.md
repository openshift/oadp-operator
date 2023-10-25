# Velero Kopia Troubleshooting Tips

## known error: Velero CPU is pegged at 100%

At this time the error is associated with kopia maintenance running on the backup repository. 
Users may also find the following type of errors in the logs while maintenance is executed:
  * `unable to create memory-mapped segment: unable to create memory-mapped file: open : no such file or directory`
  * `Error getting backup store for this location`
  * `Error getting a backup store`
  * `BackupStorageLocation is invalid`

     
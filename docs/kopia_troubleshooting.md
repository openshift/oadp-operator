# Velero Kopia Troubleshooting Tips

## Documentation
* kopia client: https://kopia.io/docs/reference/command-line/
* kopia common commands: https://kopia.io/docs/reference/command-line/common/
* kopia advanced commands: https://kopia.io/docs/reference/command-line/advanced/

## Use the kopia client from OpenShift

```
apiVersion: v1
kind: Pod
metadata:
  name: oadp-mustgather-pod
  labels:
    purpose: user-interaction
spec:
  containers:
  - name: oadp-mustgather-container
    image: registry.redhat.io/oadp/oadp-mustgather-rhel9:v1.4
    command: ["sleep"]
    args: ["infinity"] 
```

Connect to the pod and execute kopia commands

```
oc -n openshift-adp rsh pod/oadp-mustgather-pod

sh-5.1# which kopia
/usr/bin/kopia
sh-5.1# kopia --help

usage: kopia [<flags>] <command> [<args> ...]
Kopia - Fast And Secure Open-Source Backup

Flags:
      --[no-]help             Show context-sensitive help (also try --help-long and --help-man).
      --[no-]version          Show application version.
      --log-file=LOG-FILE     Override log file.
      --log-dir="/root/.cache/kopia"  
                              Directory where log files should be written. ($KOPIA_LOG_DIR)
```

## Connect to a kopia repository

```
export S3_BUCKET=<your bucket name>
export S3_REPOSITORY_PATH=<path without S3_BUCKET>
export S3_ACCESS_KEY=<s3 access key>
export S3_SECRET_ACCESS_KEY=<s3 secret access key>

# Use static-passw0rd as it is hardcoded

kopia repository connect s3 \
    --bucket="$S3_BUCKET" \
    --prefix="$S3_REPOSITORY_PATH" \
    --access-key="$S3_ACCESS_KEY" \
    --secret-access-key="$S3_SECRET_ACCESS_KEY" \
    --password=static-passw0rd
```

## Basic commands

* status and info
```
kopia repository status
kopia repository info
```

* content and size
```
kopia content stats
```

```
Count: 116
Total Bytes: 37.2 MB
Total Packed: 37.1 MB (compression 0.1%)
By Method:
  (uncompressed)         count: 102 size: 37.1 MB
  zstd-fastest           count: 14 size: 50.4 KB packed: 13.6 KB compression: 73.1%
Average: 320.4 KB
Histogram:

        0 between 0 B and 10 B (total 0 B)
        8 between 10 B and 100 B (total 506 B)
       19 between 100 B and 1 KB (total 9 KB)
       49 between 1 KB and 10 KB (total 201.6 KB)
       30 between 10 KB and 100 KB (total 0.9 MB)
        1 between 100 KB and 1 MB (total 114.7 KB)
        9 between 1 MB and 10 MB (total 35.9 MB)
        0 between 10 MB and 100 MB (total 0 B)
```

* statistics
```
kopia snapshot ls --all --storage-stats
```

* benchmark
```
kopia benchmark hashing
kopia benchmark encryption
kopia benchmark splitter
```

## known error: Velero CPU is pegged at 100%

At this time the error is associated with kopia maintenance running on the backup repository. 
Users may also find the following type of errors in the logs while maintenance is executed:
  * `unable to create memory-mapped segment: unable to create memory-mapped file: open : no such file or directory`
  * `Error getting backup store for this location`
  * `Error getting a backup store`
  * `BackupStorageLocation is invalid`

## Kopia Repository Maintenance

* repository maintenance commands:
```
kopia maintenance info
kopia maintenance run 
kopia maintenance run --full
```

#### Upstream Documentation
* Velero - https://velero.io/docs/v1.15/repository-maintenance/
* Kopia - https://kopia.io/docs/advanced/maintenance/

#### List of RFE's, bug fix and enhancements
* https://github.com/vmware-tanzu/velero/issues/8364

#### There are two types of kopia repository maintenance:

* Quick maintenance manages and optimizes indexes and q blobs that store metadata (directory listings, manifests such as snapshots, policies, acls, etc.).

* Full maintenance manages both q and p data blobs (which store contents of all files).
Maintenance is composed of individual tasks grouped into two sets:

#### Quick Maintenance
This runs frequently (hourly) with the goal of of keeping the number of index blobs (n) small, as high number of indexes negatively affects the performance of all kopia operations. This is because every write session (snapshot command, any policy manipulation, etc.) adds at least one n blob and usually one q blob so it’s very important to aggressively compact them:

* quick-rewrite-contents - looks for contents in short q packs that utilize less than 80% of the target pack size (currently around 20MB) and rewrites them to a new, larger q pack, effectively orphaning the original packs and making them eligible for deletion after some time.
* quick-delete-blobs - looks for orphaned q packs (that are not referenced by any index) and deletes them after enough time has passed for those contents to be no longer referenced by any cache.
* index-compaction - merges multiple smaller index blobs (n) into larger ones


#### Full maintenance
The main purpose of full maintenance is to perform garbage collection of contents that are no longer needed after snapshots they belong to get deleted or age out of the system.

* snapshot-gc - finds all contents (files and directory listings) that are no longer reachable from snapshot manifests and marks them as deleted. It also undeletes contents that are in use and have been marked as deleted before (due to unavoidable race between snapshot gc and snapshot create possible when multiple machines are involved).
NOTE: This is the most costly operation as it requires scanning all directories in all snapshots that are active in the system. The good news is that all this data is in q blobs and thanks to the quick maintenance it was kept nice and compact and quick to access, so this phase does not usually take that long (e.g. currently ~25 seconds on my 720 GB repository with >1.5M contents).

* full-drop-deleted-content - removes contents that have been marked for deletion long enough from the index. This creates “holes” in pack blobs and/or makes blobs completely unused and subject to deletion.

* full-rewrite-contents - same as quick-rewrite-contents but acts on all blobs (p and q)

* full-delete-blobs - same as quick-delete-blobs but acts on all blobs (p and q)

There are additional safety measures built into the maintenance routine to make it safe to run even when other kopia clients on other machines are executing snapshots concurrently. For example {quick}-delete-blobs will not run if less than X amount of time has passed since last content rewrite and full-drop-deleted-content will only drop contents if enough time has passed between full maintenance cycles.

The recommendation is to run quick maintenance as frequently as it makes sense for your repository (hourly is typically fine). The entire quick cycle should take <10 seconds, even for big repositories.

Full maintenance cycle runs every 24h by default and can be spread apart further (weekly or even monthly is probably fine) or stopped completely if somebody does not want or care to reclaim unused space.

## Kopia Repository Maintenance in OADP

In the namespace where OADP is installed repo-maintain-job's are executed

```shell=
pod/repo-maintain-job-1730739882527-2nbls                             0/1     Completed   0          168m
pod/repo-maintain-job-1730743482536-fl9tm                             0/1     Completed   0          108m
pod/repo-maintain-job-1730747082545-55ggx                             0/1     Completed   0          48m
pod/repo-maintain-job-1730749183178-5jqf2                             0/1     Completed   0          13m
pod/repo-maintain-job-1730749483183-mvrzw                             0/1     Completed   0          8m57s
pod/repo-maintain-job-1730749783183-8vtjh                             0/1     Completed   0          3m57s
```

* It is recommended to capture the logs from all the repo-maintain-jobs to understand the state of the repository and the maintenance tasks. Capturing the logs should be done on an ongoing basis.

* A user can check the logs of the repo-maintain-jobs for details for kopia or restic repo maintenance cleanup and the removal of artifacts in s3 storage.

* Users can expect a full 72 hour cycle (three executions of a full maintenance) before artifacts in s3 are deleted with OADP-1.[3,4].x
  * A user can find a note in the repo-maintain-job when the next full cycle maintenance will occur:
    ```
    not due for full maintenance cycle until 2024-11-01 18:29:4
    ```
#### What to look for in the repo-maintain-job logs:

  * A full maintenance job begins with:
    
    ```
    Running full maintenance...
    ```
  * If kopia's safety measures have not yet been reached a user may see:
    ```
    "Skipping blob deletion because not enough time has passed yet (59m59s left)
    ```
    * This indicates the s3 files are not old enough to be deleted, and users will have to wait for a future full maintenance cycle.
    
  * If files in S3 are no longer associated with an OADP backup they will be marked unreferenced.  The total unreferenced files will be summarized with.
    ```
    found $number_of_file_blobs pack blobs not in use"
    ```
    
    ```
    msg="GC found $num_files unused contents ($total_size_of_files GB)"
    ```
  * Finally after 72 hours, kopia will delete the s3 artifacts. Users can look in the logs for:
    ```
    Found $number blobs to delete ($size GB)
    ```

#### A full example of the repo-maintain-job logs when files are deleted:

```shell=
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:44Z" level=debug msg="scanning prefixes [p0 p1 p2 p3 p4 p5 p6 p7 p8 p9 pa pb pc pd pe pf q0 q1 q2 q3 q4 q5 q6 q7 q8 q9 qa qb qc qd qe qf s0 s1 s2 s3 s4 s5 s6 s7 s8 s9 sa sb sc sd se sf]" logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:92" logger name="[writer-1:UdmRepoMaintenance]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=debug msg="found 93 pack blobs not in use" logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:92" logger name="[writer-1:UdmRepoMaintenance]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=debug msg="Found 93 blobs to delete (2 GB)" logModule=kopia/maintenance logSource="pkg/kopia/kopia_log.go:92" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=debug msg="Found 93 blobs to delete (2 GB)" logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:92" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=info msg="Deleted total 93 unreferenced blobs (2 GB)" logModule=kopia/maintenance logSource="pkg/kopia/kopia_log.go:94" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=info msg="Deleted total 93 unreferenced blobs (2 GB)" logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:94" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=debug msg="Extending object lock retention-period is disabled." logModule=kopia/maintenance logSource="pkg/kopia/kopia_log.go:92" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=debug msg="Extending object lock retention-period is disabled." logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:92" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=info msg="Compacting an eligible uncompacted epoch..." logModule=kopia/maintenance logSource="pkg/kopia/kopia_log.go:94" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=info msg="Compacting an eligible uncompacted epoch..." logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:94" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=debug msg="there are no uncompacted epochs eligible for compaction" logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:92" logger name="[epoch-manager]" oldestUncompactedEpoch=0
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=info msg="Cleaning up no-longer-needed epoch markers..." logModule=kopia/maintenance logSource="pkg/kopia/kopia_log.go:94" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=info msg="Cleaning up no-longer-needed epoch markers..." logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:94" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=info msg="Attempting to compact a range of epoch indexes ..." logModule=kopia/maintenance logSource="pkg/kopia/kopia_log.go:94" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=info msg="Attempting to compact a range of epoch indexes ..." logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:94" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=debug msg="not generating range checkpoint" logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:92" logger name="[epoch-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=info msg="Cleaning up unneeded epoch markers..." logModule=kopia/maintenance logSource="pkg/kopia/kopia_log.go:94" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=info msg="Cleaning up unneeded epoch markers..." logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:94" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=info msg="Cleaning up old index blobs which have already been compacted..." logModule=kopia/maintenance logSource="pkg/kopia/kopia_log.go:94" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=info msg="Cleaning up old index blobs which have already been compacted..." logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:94" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=debug msg="Cleaning up superseded index blobs..." logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:92" logger name="[epoch-manager]" maxReplacementTime="2024-11-01 14:29:45 +0000 UTC"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=debug msg="Keeping 77 logs of total size 158.2 KB" logModule=kopia/maintenance logSource="pkg/kopia/kopia_log.go:92" logger name="[shared-manager]"
repo-maintain-job-1730572782098-fr42k velero-repo-maintenance-container time="2024-11-02T18:39:45Z" level=debug msg="Keeping 77 logs of total size 158.2 KB" logModule=kopia/kopia/format logSource="pkg/kopia/kopia_log.go:92" logger name="[shared-manager]"
```

## Kopia safety features

#### Kopia safety features:

* https://github.com/kopia/kopia/blob/master/repo/maintenance/maintenance_safety.go#L56C1-L68C1

```
// Supported safety levels.
//
//nolint:gochecknoglobals
var (
	// SafetyNone has safety parameters which allow full garbage collection without unnecessary
	// delays, but it is safe only if no other kopia clients are running and storage backend is
	// strongly consistent.
	SafetyNone = SafetyParameters{
		BlobDeleteMinAge:                 0,
		DropContentFromIndexExtraMargin:  0,
		MarginBetweenSnapshotGC:          0,
		MinContentAgeSubjectToGC:         0,
		RewriteMinAge:                    0,
		SessionExpirationAge:             0,
		RequireTwoGCCycles:               false,
		DisableEventualConsistencySafety: true,
	}

	// SafetyFull has default safety parameters which allow safe GC concurrent with snapshotting
	// by other Kopia clients.
	SafetyFull = SafetyParameters{
		BlobDeleteMinAge:                24 * time.Hour, //nolint:mnd
		DropContentFromIndexExtraMargin: time.Hour,
		MarginBetweenSnapshotGC:         4 * time.Hour,  //nolint:mnd
		MinContentAgeSubjectToGC:        24 * time.Hour, //nolint:mnd
		RewriteMinAge:                   2 * time.Hour,  //nolint:mnd
		SessionExpirationAge:            96 * time.Hour, //nolint:mnd
		RequireTwoGCCycles:              true,
		MinRewriteToOrphanDeletionDelay: time.Hour,
	}
)
```

## Full maintenance with safety=None ( Emergency use only ) - not recommended.

WARNING: As the name implies, the --safety=none flag disables all safety features, so the user must ensure that no concurrent operations are happening and repository storage is properly in sync before attempting it. Failure to do so can introduce repository corruption.

```
kopia maintenance run --full --safety=none
```

First, safety=none is only something we can run if we can guarantee that nothing will access the repository during that time. This means that before running a manual full maintenance with safety off, users must first shut down all Velero instances which reference the repository. 

This is not something we want to recommend that users do under normal circumstances – it should be limited to exceptional circumstances – something like "We accidentally backed up a 10TB volume we didn't intend to back up, so we deleted the Velero backup and need to immediately get rid of this in our bucket." Or also "We have found a bug in Velero and full maintenance is not working properly. The data should have been removed days ago but has not."


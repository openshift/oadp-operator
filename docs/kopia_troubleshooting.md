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
    image: registry.redhat.io/oadp/oadp-mustgather-rhel9:v1.3
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

* maintenance 
```
kopia maintenance run 
# use the following with caution 
kopia maintenance run --full --force --safety=none
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

     
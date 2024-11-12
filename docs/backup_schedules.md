# Backup Schedules

## Additional Documentation
* Upstream Documentation: https://velero.io/docs/main/api-types/schedule/

## Example Schedule CR
```yaml
apiVersion: velero.io/v1
kind: Schedule
metadata:
  name: schedule-backup-10pm
  namespace: openshift-adp
spec:
  schedule: "0 22 * * ?"  
  template:
    includedNamespaces:
      - test
    ttl: 720h  # Retain backups for 30 days
    snapshotMoveData: true
    snapshotVolumes: true
    storageLocation: velero-sample-1
    volumeSnapshotLocations:
      - velero-sample-1
```

## Example k8s cronjob to delete scheduled backups (retain latest 10 backups)
```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: velero-backup-cleanup
  namespace: openshift-adp
spec:
  schedule: "0 23 * * *"  # 11:00 PM
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: cleanup
            image: velero/velero:latest  
            command:
            - /bin/sh
            - -c
            - |
              BACKUP_JOBS=("schedule-backup-10pm-ed-no-pvc-nisdok" "schedule-backup-10pm-ed-no-pvc-nisdok-ext" "schedule-backup-10pm-ed-no-pvc-qvistorp")  # List of backup jobs to manage

              for JOB in "${BACKUP_JOBS[@]}"; do
                echo "Processing backups for job: $JOB"

                # Get backups for the job, ordered by creation date
                BACKUPS=$(velero backup get --namespace=openshift-adp --selector=velero.io/job-name=$JOB --sort-by=.metadata.creationTimestamp -o name | awk -F'/' '{print $2}')

                BACKUP_COUNT=$(echo "$BACKUPS" | wc -l)  
                DELETE_COUNT=$((BACKUP_COUNT - 10))  # Calculate how many to delete, keeping last 10

                if [ "$DELETE_COUNT" -gt 0 ]; then
                  BACKUPS_TO_DELETE=$(echo "$BACKUPS" | head -n "$DELETE_COUNT")

                  for BACKUP in $BACKUPS_TO_DELETE; do
                    echo "Deleting backup: $BACKUP"
                    velero backup delete "$BACKUP" --confirm --namespace=openshift-adp
                  done
                else
                  echo "No backups to delete for job: $JOB"
                fi
              done
          restartPolicy: OnFailure
```
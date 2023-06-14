FROM quay.io/openshift/origin-must-gather:4.10 as builder

# Add the Backup and Restore CR's
FROM registry.access.redhat.com/ubi9/toolbox:latest
WORKDIR /

COPY --from=builder /usr/bin/oc /usr/bin/oc
COPY backup.yaml .
COPY restore.yaml . 
COPY check_backup_namespace.sh /usr/bin/
COPY get_restore_namespace.sh /usr/bin/

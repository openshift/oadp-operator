# Add the Backup and Restore CR's
FROM registry.access.redhat.com/ubi9/toolbox:latest

WORKDIR /
COPY backup.yaml .
COPY restore.yaml .
COPY check_namespace.sh .
COPY check_namespace.sh /usr/bin

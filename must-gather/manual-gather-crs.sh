#!/bin/bash
# gather CRs if must-gather do not work.
# logs are not collected here because they are collected by must-gather and must-gather fails normally due to inability to collect logs from velero server.
# We can introduce a separate script to collect logs from velero server if needed.
mkdir manual-gather-crs
oc get volumesnapshotbackups.datamover.oadp.openshift.io --all-namespaces -oyaml > manual-gather-crs/volumesnapshotbackups.datamover.oadp.openshift.io.yaml
oc get volumesnapshotrestores.datamover.oadp.openshift.io --all-namespaces -oyaml > manual-gather-crs/volumesnapshotrestores.datamover.oadp.openshift.io.yaml
oc get cloudstorages.oadp.openshift.io --all-namespaces -oyaml > manual-gather-crs/cloudstorages.oadp.openshift.io.yaml
oc get dataprotectionapplications.oadp.openshift.io --all-namespaces -oyaml > manual-gather-crs/dataprotectionapplications.oadp.openshift.io.yaml
oc get backuprepositories.velero.io --all-namespaces -oyaml > manual-gather-crs/backuprepositories.velero.io.yaml
oc get backups.velero.io --all-namespaces -oyaml > manual-gather-crs/backups.velero.io.yaml
oc get backupstoragelocations.velero.io --all-namespaces -oyaml > manual-gather-crs/backupstoragelocations.velero.io.yaml
oc get deletebackuprequests.velero.io --all-namespaces -oyaml > manual-gather-crs/deletebackuprequests.velero.io.yaml
oc get downloadrequests.velero.io --all-namespaces -oyaml > manual-gather-crs/downloadrequests.velero.io.yaml
oc get podvolumebackups.velero.io --all-namespaces -oyaml > manual-gather-crs/podvolumebackups.velero.io.yaml
oc get podvolumerestores.velero.io --all-namespaces -oyaml > manual-gather-crs/podvolumerestores.velero.io.yaml
oc get resticrepositories.velero.io --all-namespaces -oyaml > manual-gather-crs/resticrepositories.velero.io.yaml
oc get restores.velero.io --all-namespaces -oyaml > manual-gather-crs/restores.velero.io.yaml
oc get schedules.velero.io --all-namespaces -oyaml > manual-gather-crs/schedules.velero.io.yaml
oc get serverstatusrequests.velero.io --all-namespaces -oyaml > manual-gather-crs/serverstatusrequests.velero.io.yaml
oc get volumesnapshotlocations.velero.io --all-namespaces -oyaml > manual-gather-crs/volumesnapshotlocations.velero.io.yaml
zip -r manual-gather.zip manual-gather-crs

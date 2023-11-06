#!/bin/bash

BACKUP="${BACKUP:-false}"
RESTORE="${RESTORE:-false}"
DETAILS="${DETAILS:-false}"
############################################################
# Help                                                     #
############################################################
Help()
{
   # Display Help
   echo "Check the OADP CR's in real time"
   echo
   echo "options:"
   echo "h     Print this Help."
   echo "b     Check a backup"
   echo "r     Check a restore"
   echo "d     Print a list of the relevant backup/restore cr's"
}

############################################################
############################################################
# Main program                                             #
############################################################
############################################################
# Process the input options. Add options as needed.        #
############################################################
# Get the options
while getopts ":hbrd" option; do
   case $option in
      h) # display Help
         Help
         exit;;
      b) # Check backup resources 
	     BACKUP="true";;
      r) # Check restore resources 
	     RESTORE="true";;
      d) # Print a list of the relevant backup/restore cr's 
	     DETAILS="true";;
     \?) # Invalid option
         echo "Error: Invalid option"
	 Help
         exit;;
   esac
done

if [ "$BACKUP" = "false" ] && [ "$RESTORE" = "false" ]; then
  Help
fi 

function backup_summary () {
    echo -e "Get Backups:\n"
    oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero get backup ;
    echo -e "\nTotal Snapshots: " `oc get volumesnapshot -A | sed 1d | wc -l` ; 
    echo "Total OADP Snapshots: " `oc get volumesnapshot -n openshift-adp | sed 1d | wc -l` ;
    echo "Total SnapshotContents: " `oc get volumesnapshotcontents -A | sed 1d | wc -l` ; 
    echo -e "\nTotal VSB: " `oc get vsb -A 2>/dev/null | sed 1d | wc -l` ;
    echo "Completed: " `oc get vsb -A 2>/dev/null | grep -c Completed` ;
    echo "InProgress: " `oc get vsb -A 2>/dev/null | grep -c InProgress` ;
    echo "SnapshotBackupDone: " `oc get vsb -A 2>/dev/null | grep -c SnapshotBackupDone` ;
    echo -e "\nVSB STATUS" ;
    echo "Completed: " `oc get vsb -A -oyaml 2>/dev/null | grep batching | grep -c Completed` ;
    echo "Processing: " `oc get vsb -A -oyaml 2>/dev/null | grep batching | grep -c Processing` ;
    echo "Queued: " `oc get vsb -A -oyaml 2>/dev/null | grep batching | grep -c Queued` ;
    echo -e "\nTotal ReplicationSources: " `oc get replicationsources -A | sed 1d | wc -l` ;
}

function restore_summary () {
    echo -e "Get Restores:\n"
    oc -n openshift-adp exec deployment/velero -c velero -it -- ./velero get restore ;
    echo -e "\nTotal VSR: " `oc get vsr -A 2>/dev/null | sed 1d | wc -l` ;
    echo "Completed: " `oc get vsr -A 2>/dev/null | grep -c Completed` ;
    echo "InProgress: " `oc get vsr -A 2>/dev/null | grep -c InProgress` ;
    echo "SnapshotRestoreDone: " `oc get vsr -A 2>/dev/null | grep -c SnapshotRestoreDone` ;
    echo -e "\nVSR STATUS" ;
    echo "Completed: " `oc get vsr -A -oyaml 2>/dev/null | grep batching | grep -c Completed` ;
    echo "Processing: " `oc get vsr -A -oyaml 2>/dev/null | grep batching | grep -c Processing` ;
    echo "Queued: " `oc get vsr -A -oyaml 2>/dev/null | grep batching | grep -c Queued` ;
    echo -e "\nTotal ReplicationDestinations: " `oc get replicationdestinations -A | sed 1d | wc -l` ;
}

function vsc_details () {
    echo -e "\n***** VOLUME SNAPSHOT CONTENTS ******"
    for i in $(oc get vsc -A -o=custom-columns=:.metadata.name); do
        oc get vsc $i -o jsonpath='{"Name: "}{@.metadata.name}{" ReadyToUse: "}{@.status.readyToUse}{" creationTime: " }{@.metadata.creationTimestamp}' | column -t
    done
}

function replicationsources_details () {
    echo -e "\n***** REPLICATION SOURCE ******"
    for i in $(oc -n openshift-adp get replicationsources -A -o=custom-columns=:.metadata.name); do
        oc -n openshift-adp get replicationsources $i -o jsonpath='{"Name: "}{@.metadata.name}{" SyncDuration: "}{@.status.lastSyncDuration}{" sourcePVC: " }{@.spec.sourcePVC}' | column -t
    done
}

function replicationdestination_details () {
    echo -e "\n***** REPLICATION DESTINATION ******"
    for i in $(oc -n openshift-adp get replicationdestination -A -o=custom-columns=:.metadata.name); do
        oc -n openshift-adp get replicationdestination $i -o jsonpath='{"Name: "}{@.metadata.name}{" SyncDuration: "}{@.status.lastSyncDuration}{" sourcePVC: " }{@.spec.sourcePVC}' | column -t
    done
}


if [ "$BACKUP" = "true" ]; then 
    backup_summary
    # Show Details
    if [ "$DETAILS" = "true" ]; then
        vsc_details
        replicationsources_details
    fi
fi

if [ "$RESTORE" = "true" ]; then 
    restore_summary
    # Show Details
    if [ "$DETAILS" = "true" ]; then
        vsc_details
        replicationdestination_details
    fi
fi
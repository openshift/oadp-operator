#!/bin/bash

INSTALLSAMPLE="${INSTALLSAMPLE:-false}"
############################################################
# Help                                                     #
############################################################
Help()
{
   # Display Help
   echo "Check the prerequisites for the OADP non-admin demo"
   echo
   echo "Check the prerequisites [-h|-i]"
   echo "options:"
   echo "h     Print this Help."
   echo
}

############################################################
############################################################
# Main program                                             #
############################################################
############################################################
# Process the input options. Add options as needed.        #
############################################################
# Get the options
while getopts "h:i" option; do
   case $option in
      h) # display Help
         Help
         exit;;
      i) # Enter a demo user base name 
	 INSTALLSAMPLE="true";;
     \?) # Invalid option
         echo "Error: Invalid option"
	 Help
         exit;;
   esac
done

TKN_INSTALLED=0
OADP_INSTALLED=0
OADP_CONFIGURED=0

if ! type oc > /dev/null 2>&1; then
    echo "oc command not found, please install it in your PATH and log in to the OpenShift cluster to continue..."
    exit 1
fi

if ! type jq > /dev/null 2>&1; then
    echo "jq command not found, please install it in your PATH to continue..."
    exit 1
fi

if ! oc auth can-i '*' '*' --all-namespaces >/dev/null 2>&1; then
    echo "You must be logged in to a OpenShift cluster as a user with cluster-admin to continue..."
    exit 1
fi

printf "Checking if the Openshift Pipelines operator is installed...\n"
printf "$ oc get crds tektonpipelines.operator.tekton.dev\n"
oc get crds tektonpipelines.operator.tekton.dev && TKN_INSTALLED=1

printf "Checking if OADP is installed and configured....\n"
printf "$ oc get operator oadp-operator.openshift-adp\n"
if oc get operator  | grep -q oadp-operator.openshift-adp; then
  OADP_INSTALLED=1
  printf "Checking if there is configured at least one DataProtectionApplication Custom Resource...\n"
  printf "$ oc get dpa -n openshift-adp -o jsonpath={.items} | jq '. | length'\n"
  drc=$(oc get dpa -n openshift-adp -o jsonpath={.items} | jq '. | length')
  if [ $drc -ge 1 ]; then
     OADP_CONFIGURED=1
  fi
fi

function print_tkn_not_installed() {
   printf "\n\t* Install OpenShift Pipelines (Tekton):\n"
   printf "\n\t  https://docs.openshift.com/container-platform/latest/cicd/pipelines/installing-pipelines.html\n"
}

function print_oadp_not_installed() {
   printf "\n\t* Install OADP Operator:\n"
   printf "\n\t  https://docs.openshift.com/container-platform/4.12/backup_and_restore/application_backup_and_restore/installing/about-installing-oadp.html\n"
}

function print_oadp_not_configured() {
     printf "\n\t* Configure OADP DataProtectionApplication Custom Resource named 'dpa-sample':\n"
     printf "\n\t  https://github.com/openshift/oadp-operator/blob/master/docs/install_olm.md#create-the-dataprotectionapplication-custom-resource"
}

printf "\n######################"
printf "\nSummary:"
printf "\n\t Tekton installed:\t%s" "$([ "$TKN_INSTALLED" -eq 1 ] && echo "True" || echo "False")"
printf "\n\t OADP installed:\t%s" "$([ "$OADP_INSTALLED" -eq 1 ] && echo "True" || echo "False")"
printf "\n\t OADP configured:\t%s" "$([ "$OADP_CONFIGURED" -eq 1 ] && echo "True" || echo "False")"
[ "$TKN_INSTALLED" -eq 1 ] && [ "$OADP_INSTALLED" -eq 1 ] && [ "$OADP_CONFIGURED" -eq 1 ] || printf "\n\nActions needed:"
[ "$TKN_INSTALLED" -eq 1 ] || print_tkn_not_installed
[ "$OADP_INSTALLED" -eq 1 ] || print_oadp_not_installed
[ "$OADP_CONFIGURED" -eq 1 ] || print_oadp_not_configured

printf "\n######################\n"

[ "$TKN_INSTALLED" -eq 1 ] && [ "$OADP_INSTALLED" -eq 1 ] && [ "$OADP_CONFIGURED" -eq 1 ]
exit $?

#!/bin/bash

INSTALL_SAMPLE="${INSTALL_SAMPLE:-false}"
############################################################
# Help                                                     #
############################################################
Help()
{
   # Display Help
   echo "Check the prerequisites for the OADP non-admin demo"
   echo
   echo "Syntax: scriptTemplate [-h|-p|-u|-d]"
   echo "options:"
   echo "h     Print this Help."
   echo "i     Install nginx-example"
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
while getopts ":h:i:" option; do
   case $option in
      h) # display Help
         Help
         exit;;
      i) # Enter a project name 
	 INSTALL_SAMPLE=true;;
     \?) # Invalid option
         echo "Error: Invalid option"
	 Help
         exit;;
   esac
done


printf "Checking if OADP is installed and configured....\n"
orc=`oc get operator oadp-operator.openshift-adp > /dev/null && echo $?`
drc=`oc get dpa -n openshift-adp > /dev/null && echo $?`
dparc=`oc get dpa dpa-sample -n openshift-adp -o jsonpath='{.metadata.name}'` || true
if [ $orc -eq 0 ] && [ $drc -eq 0 ] && [ $dparc == "dpa-sample" ]; then 
  printf "OADP is installed and configured correctly\n" 
elif [ $orc -ne 0 ]; then
  printf "OADP is NOT installed, please install OADP\n"
  printf "https://docs.openshift.com/container-platform/4.12/backup_and_restore/application_backup_and_restore/installing/about-installing-oadp.html\n"
  exit 1
elif [ $drc -ne 0 ]; then
  printf "OADP is not configured with a DPA\n"
  printf "Please configure a DPA named dpa-sample\n"
  printf "https://github.com/openshift/oadp-operator/blob/master/docs/install_olm.md#create-the-dataprotectionapplication-custom-resource"
  exit 1
elif [ $dparc != "dpa-sample" ]; then
  printf "At this time the DPA name must be configured to be dpa-sample"
  exit 1
fi

printf "Checking for a sample application nginx-example\n"
nrc=`oc get namespace nginx-example > /dev/null && echo $?`
drc=`oc get deployment -n nginx-example | grep "2/2" > /dev/null && echo $?`

if [ $nrc -eq 0 ] && [ $drc -eq 0 ]; then
  printf "The nginx-example sample application is deployed and running\n"
elif [ $INSTALL_SAMPLE == "true" ]; then
  printf "Installing the nginx sample application\n"
  oc create -f ../examples/manifests/nginx/nginx-deployment.yaml 
elif [ $INSTALL_SAMPLE == "false" ]; then
  printf "Please set the flag to install the sample application or\n"
  printf "alternatively follow the instructions:\n"
  printf "https://github.com/openshift/oadp-operator/blob/master/docs/examples/stateless.md\n"
fi


printf "done"

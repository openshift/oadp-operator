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

printf "\n"
printf "Checking if the Openshift Pipelines operator is installed....\n"
prc=`oc get operator | grep -c openshift-pipelines-operator-rh.openshift-operators` 
if [ $prc -eq 1 ]; then
  printf "The Openshift Pipelines operator is installed\n"
else
  printf "Please install the Openshift Pipelines operator\n"
  exit
fi

printf "\n"
printf "Checking if OADP is installed and configured....\n"
orc=`oc get operator | grep -c oadp-operator.openshift-adp`
if [ $orc -eq 1 ]; then
   drc=`oc get dpa -n openshift-adp | grep -c dpa-sample`
   dparc=`oc get dpa dpa-sample -n openshift-adp -o jsonpath='{.metadata.name}'` || true
   if [ $drc -eq 1 ] && [ $dparc == "dpa-sample" ]; then 
   printf "OADP is installed and configured correctly\n" 
   elif [ $drc -ne 1 ]; then
   printf "OADP is not configured with a DPA\n"
   printf "Please configure a DPA named dpa-sample\n"
   printf "https://github.com/openshift/oadp-operator/blob/master/docs/install_olm.md#create-the-dataprotectionapplication-custom-resource"
   printf "\n"
   exit 1
   elif [ $dparc != "dpa-sample" ]; then
   printf "At this time the DPA name must be configured to be dpa-sample"
   exit 1
   fi
else 
   printf "OADP is NOT installed, please install OADP\n"
   printf "https://docs.openshift.com/container-platform/4.12/backup_and_restore/application_backup_and_restore/installing/about-installing-oadp.html\n"
   printf "\n"
   exit 1
fi
printf "\n\n"

printf "Checking for a sample application nginx-example\n"
nrc=`oc get namespace | grep -c nginx-example`
ndrc=`oc get deployment -n nginx-example | grep -c "2\/2"` || true

if [ $nrc -eq 1 ]; then
  printf "The nginx-example namespace found\n"
   if [ $ndrc -eq 1 ]; then
      printf "The nginx-example deployment was found\n"
   fi
fi
if [ $INSTALLSAMPLE == "true" ] && [ $nrc -eq 0 ]; then
  printf "Installing the nginx sample application\n"
  oc create -f ../examples/manifests/nginx/nginx-deployment.yaml 
elif [ $INSTALLSAMPLE == "false" ] && [ $nrc -eq 0 ]; then
  printf "Please set the flag to install the sample application or\n"
  printf "alternatively follow the instructions:\n"
  printf "https://github.com/openshift/oadp-operator/blob/master/docs/examples/stateless.md\n"
fi


printf "done"

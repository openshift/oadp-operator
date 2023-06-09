#!/bin/bash

OUTPUT_DIR="${OUTPUT_DIR:-/tmp/oadp_non_admin}"
PASSWORD="${PASSWORD:-passw0rd}" #CHANGEME?
############################################################
# Help                                                     #
############################################################
Help()
{
   # Display Help
   echo "Create the OADP non-admin users for demonstrations only"
   echo
   echo "Syntax: scriptTemplate [-h|-n|-c|-p|-x|-d]"
   echo "options:"
   echo "h     Print this Help."
   echo "n     demouser base name"
   echo "x     the project name"
   echo "c     the number of users to be created"
   echo "p     the common password"
   echo "d     The directory where the htpasswd file will be saved"
   echo
}

############################################################
############################################################
# This script is for demonstration purposes only           #
############################################################
############################################################
# Process the input options. Add options as needed.        #
############################################################
# Get the options
while getopts ":h:n:c:p:x:d:" option; do
   case $option in
      h) # display Help
         Help
         exit;;
      n) # Enter a demo user base name 
	 BASENAME=$OPTARG;;
      c) # Enter the number of users to be created
	 COUNT=$OPTARG;;
      d) # The output directory
	 OUTPUT_DIR=$OPTARG;;
      p) # The common password for the httpdpasswd file
	 PASSWORD=$OPTARG;;
      x) # Enter the project name
    PROJECT=$OPTARG;;
     \?) # Invalid option
         echo "Error: Invalid option"
	 Help
         exit;;
   esac
done

if [ -z "$BASENAME" ];then Help; exit; fi
if [ -z "$COUNT" ];then Help; exit; fi
if [ -z "$PROJECT" ];then Help; exit; fi

# create the templates
mkdir -p $OUTPUT_DIR || true
cp oauth.yaml $OUTPUT_DIR
pushd $OUTPUT_DIR
touch htpasswd || true

COUNTER=1
while [[ $COUNTER -le $COUNT ]]; do
  echo "htpasswd -B -b $OUTPUT_DIR/htpasswd $BASENAME$COUNTER $PASSWORD"
  htpasswd -B -b $OUTPUT_DIR/htpasswd $BASENAME$COUNTER $PASSWORD
  ((COUNTER++))
done
printf "\n"

printf "create the OCP secret w/ htpasswd creds\n"
oc create secret generic oadp-nonadmin-$BASENAME --from-file=htpasswd=htpasswd -n openshift-config || printf "WARNING: A secret with this name already exists\n"
oc get secret/oadp-nonadmin-$BASENAME -n openshift-config -oyaml

printf "Create the OCP oauth entry"
sed -i '' -e "s/REPLACEME/$BASENAME/g" oauth.yaml


# oc get oauth cluster -o jsonpath='{.spec.identityProviders}' | yq -P
# oc patch oauth cluster  --type merge --patch-file oauth.yaml

# get the currently configured identity providers
oc get oauth cluster -o jsonpath='{.spec.identityProviders}' | yq -P > current_ident.yaml
# add two spaces
sed -i '' 's/^/  /' current_ident.yaml
cat current_ident.yaml >> oauth.yaml
printf "\n\n"
cat oauth.yaml
printf "This script will merge this oauth.yaml file in 10 seconds\n"
printf "ctl-c to cancel"
sleep 10
oc patch oauth cluster  --type merge --patch-file oauth.yaml || exit 1

printf "\n\n"
printf "WARNING: it may take a few minutes for the oauth settings to reconcile\n"

printf "\nThe following is the htpasswd file\n"
cat htpasswd
printf "sleeping for 60 seconds\n"

console=`oc get routes --all-namespaces | grep -i console-openshift | awk '{print$3}'`
auth_window="k8s/cluster/config.openshift.io~v1~ClusterOperator/authentication"
printf "\n !!! Please check the following for details !!!\n"
printf "https://${console}/${auth_window}\n\n"
sleep 60

popd
pwd
# create the project template
printf "PROJECT = $PROJECT\n"
oc process -f 01-new-project-request_template.yaml -p PROJECT=$PROJECT -o yaml > "${OUTPUT_DIR}/01-new-project-request_template.yaml"

# create the user templates
COUNTER=1
while [[ $COUNTER -le $COUNT ]]; do
  oc process -f user.yaml -p PROJECT=$PROJECT -p BASENAME=$BASENAME -p USER=$BASENAME$COUNTER -o yaml > "${OUTPUT_DIR}/user-${BASENAME}${COUNTER}.yaml"
  ((COUNTER++))
done

pushd ${OUTPUT_DIR}
# apply the project template
oc create -f 01-new-project-request_template.yaml

# create the users
for i in `ls user*.yaml`; do
  oc create -f $i
done
popd

printf "\n **** DONE! ****\n\n"
printf "Created users based on the input BASENAME: $BASENAME and COUNT: $COUNT\n"
printf "Common PASSWORD: $PASSWORD\n"
printf "htpasswd file saved to: $OUTPUT_DIR\n\n"
printf "The following manifests in ${OUTPUT_DIR} have been applied...\n"
ls -la ${OUTPUT_DIR}

printf "\n\nPlease ensure the OAuth Server has reconciled...\n"
printf "https://${console}/${auth_window}\n\n"







#!/bin/bash

OUTPUT_DIR="${OUTPUT_DIR:-/tmp/oadp_non_admin}"
PASSWORD="${PASSWORD:-passw0rd}" #CHANGEME?
############################################################
# Help                                                     #
############################################################
Help()
{
   # Display Help
   echo "Create the OADP non-admin users"
   echo
   echo "Syntax: scriptTemplate [-h|-n|-c|-p|-d]"
   echo "options:"
   echo "h     Print this Help."
   echo "n     demouser base name"
   echo "c     the number of users to be created"
   echo "p     the common password"
   echo "d     The directory where the htpasswd file will be saved"
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
while getopts ":h:n:c:p:d:" option; do
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
     \?) # Invalid option
         echo "Error: Invalid option"
	 Help
         exit;;
   esac
done

if [ -z "$BASENAME" ];then Help; exit; fi
if [ -z "$COUNT" ];then Help; exit; fi

printf "Creating the users based on the input BASENAME: $BASENAME and COUNT: $COUNT\n"
printf "Common PASSWORD: $PASSWORD\n"
printf "htpasswd file saved to: $OUTPUT_DIR\n\n"

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
oc create secret generic htpass-secret-$BASENAME --from-file=htpasswd=htpasswd -n openshift-config || printf "WARNING: A secret with this name already exists\n"
oc get secret/htpass-secret-$BASENAME -n openshift-config -oyaml

printf "Create the OCP oauth entry"
sed -e "s/REPLACEME/$BASENAME/" oauth.yaml > oauth.yaml.tmp
mv oauth.yaml.tmp oauth.yaml
cat oauth.yaml
oc apply  -f oauth.yaml

printf "WARNING: it may take a few minutes for the oauth settings to reconcile\n"
printf "Once the oauth settings have reconciled you may login w/ the following users:\n"
cat htpasswd

popd





#!/bin/bash

OUTPUT_DIR="${OUTPUT_DIR:-/tmp/oadp_non_admin}"
############################################################
# Help                                                     #
############################################################
Help()
{
   # Display Help
   echo "Create the OADP non-admin templates"
   echo
   echo "Syntax: scriptTemplate [-h|-p|-u|-d]"
   echo "options:"
   echo "h     Print this Help."
   echo "p     Project or Namespace for the tekton pipeline"
   echo "u     Name of the non-admin user"
   echo "d     The directory where the templates will be saved"
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
while getopts ":h:p:u:d:" option; do
   case $option in
      h) # display Help
         Help
         exit;;
      p) # Enter a project name 
	 PROJECT=$OPTARG;;
      u) # Enter a user name
	 USER=$OPTARG;;
      d) # The output directory
	 OUTPUT_DIR=$OPTARG;;
     \?) # Invalid option
         echo "Error: Invalid option"
	 Help
         exit;;
   esac
done

if [ -z "$PROJECT" ];then Help; exit; fi
if [ -z "$USER" ];then Help; exit; fi

printf "Creating the templates based on the input project: $PROJECT and user: $USER\n"
printf "Saved to: $OUTPUT_DIR\n\n"

# create the templates
mkdir -p $OUTPUT_DIR || true
pushd install_templates
pwd
cp -v *.yaml $OUTPUT_DIR/
pushd templates
pwd

FILES="03-rbac-pipeline-role.yaml
04-service-account_template.yaml"

for i in $FILES; do
  oc process -f $i -p PROJECT=$PROJECT -p USER=$USER -o yaml > $OUTPUT_DIR/$i
done

ALLOWED_NAMESPACES=`oc --as buzz1 get projects -o jsonpath='{range .items[*]}{.metadata.name}{","}{end}'`

FILES="05-build-and-deploy.yaml"
for i in $FILES; do
  oc process -f $i -p PROJECT=$PROJECT -p USER=$USER -p ALLOWED_NAMESPACES=$ALLOWED_NAMESPACES -o yaml > $OUTPUT_DIR/$i
done
popd


FILES="03-rbac-pipeline-role.yaml
04-service-account_template.yaml
05-build-and-deploy.yaml"

# apply the templates
pushd $OUTPUT_DIR
for i in $FILES; do
  printf "\nExecuting oc create -f $i\n"
  oc create -f $i
done

# oc adm policy add-role-to-user view $USER -n $PROJECT


printf "done"

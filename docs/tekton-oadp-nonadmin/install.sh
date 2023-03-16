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
   echo "Syntax: scriptTemplate [-h|-p|-u]"
   echo "options:"
   echo "h     Print this Help."
   echo "p     Name of the Project or Namespace"
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
cp -Rv *.yaml $OUTPUT_DIR/
pushd templates
for i in `ls`;do
  oc process -f $i -p PROJECT=$PROJECT -p USER=$USER -o yaml > $OUTPUT_DIR/$i
done
popd

# apply the templates
pushd $OUTPUT_DIR
for i in `ls`;do
  oc create -f $i
done

oc adm policy add-role-to-user view $USER -n $PROJECT


printf "done"

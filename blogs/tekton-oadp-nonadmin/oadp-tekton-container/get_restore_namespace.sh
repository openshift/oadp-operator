#!/bin/bash

#$1 = backup name
#$2 = allowed projects string
#

namespace=`oc get backup $1 -o jsonpath='{.spec.includedNamespaces[0]}' -n openshift-adp`
if [ "$namespace" = "" ]; then
  echo "Error: backup not found or no namespace associated with backup"
  exit 1
fi

if grep -q "$namespace" <<< "$2"; then
  echo "Found project $namespace in the allowed list of projects for the user"
else
  echo "The project $namespace is not allowed for this user"
  exit 1
fi

#!/bin/bash

#$1 = project name
#$2 = allowed projects string

if grep -q "$1" <<< "$2"; then
  echo "Found project $1 in the allowed list of projects for the user"
else
  echo "The project $1 is not allowed for this user"
  exit 1
fi

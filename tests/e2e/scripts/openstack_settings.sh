#!/bin/bash

cat > $TMP_DIR/oadpcreds <<EOF
{
  "spec": {
      "configuration":{
        "velero":{
          "defaultPlugins": [
            "openshift", "aws"
          ]
        }
      },
      "backupLocations": [
        {
          "velero": {
            "provider": "aws",
            "config": {
             "profile": "$BSL_AWS_PROFILE",
              "region": "$BSL_REGION"
            },
            "objectStorage":{
              "bucket": "$BUCKET"
            }
          }
        }
      ],
    "credential":{
      "name": "$SECRET",
      "key": "cloud"
    },
     "snapshotLocations": [
       {
         "velero": {
           "provider": "aws",
           "config": { 
             "profile": "default",
             "region": "$VSL_REGION"
           }
         }
       }
     ]
  }
}
EOF

x=$(cat $TMP_DIR/oadpcreds); echo "$x" | grep -o '^[^#]*'  > $TMP_DIR/oadpcreds

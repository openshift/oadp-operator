#!/bin/bash

cat > $TMP_DIR/oadpcreds <<EOF
{
  "spec": {
      "configuration":{
        "velero":{
          "defaultPlugins": [
            "openshift", "$PROVIDER"
          ]
        }
      },
      "backupLocations": [
        {
          "velero": {
            "provider": "$PROVIDER",
            "config": {
            },
            "objectStorage":{
              "bucket": "$BUCKET"
            }
          }
        }
      ]
#     ,"credential":{
#       "name": "$SECRET",
#       "key": "cloud"
#     },
     "snapshotLocations": [
       {
         "velero": {
           "provider": "$PROVIDER",
           "config": { 
             "snapshotLocation": "$VSL_REGION"
           }
         }
       }
     ]
  }
}
EOF

x=$(cat $TMP_DIR/oadpcreds); echo "$x" | grep -o '^[^#]*'  > $TMP_DIR/oadpcreds

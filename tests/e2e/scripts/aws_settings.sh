#!/bin/bash

cat > $TMP_DIR/awscreds <<EOF
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
              "region": "$REGION"
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
#      "snapshotLocations": [
#        {
#          "velero": {
#            "provider": "$PROVIDER",
#            "config": { 
#              "profile": "snapshot",
#              "region": "$REGION"
#            }
#          }
#        }
#      ]
  }
}
EOF

x=$(cat $TMP_DIR/awscreds); echo "$x" | grep -o '^[^#]*'  > $TMP_DIR/awscreds

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
              "subscriptionId": "",
              "storageAccount": "",
              "resourceGroup": "",
              "storageAccountKeyEnvVar": "",
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
              "subscriptionId": "",
						  "resourceGroup": "",
           }
         }
       }
     ]
  }
}
EOF

x=$(cat $TMP_DIR/oadpcreds); echo "$x" | grep -o '^[^#]*'  > $TMP_DIR/oadpcreds

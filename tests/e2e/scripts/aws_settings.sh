#!/bin/bash

cat > $TMP_DIR/oadpcreds <<EOF
{
  "metadata": {
    "namespace": "$NAMESPACE"
  },
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
           "provider": "$PROVIDER",
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

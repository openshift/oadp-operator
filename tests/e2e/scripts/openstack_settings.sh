#!/bin/bash

cat > $TMP_DIR/oadpcreds <<EOF
{
  "spec": {
      "unsupportedOverrides": {
        "veleroImageFqin": "$VELERO_IMAGE",
        "awsPluginImageFqin": "$AWS_PLUGIN_IMAGE",
        "openshiftPluginImageFqin": "$OPENSHIFT_PLUGIN_IMAGE",
        "azurePluginImageFqin": "$AZURE_PLUGIN_IMAGE",
        "gcpPluginImageFqin": "$GCP_PLUGIN_IMAGE",
        "resticRestoreImageFqin": "$RESTORE_IMAGE",
        "kubevirtPluginImageFqin": "$KUBEVIRT_PLUGIN_IMAGE",
        "nonAdminControllerImageFqin": "$NON_ADMIN_IMAGE"
      },
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

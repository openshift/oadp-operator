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
      ],
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

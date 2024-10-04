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
              "profile": "$BSL_AWS_PROFILE",
              "region": "$BSL_REGION",
              "s3ForcePathStyle": "true",
              "s3Url": "https://s3.$BSL_REGION.cloud-object-storage.appdomain.cloud"
            },
            "objectStorage":{
              "bucket": "$BUCKET"
            }
          }
        }
      ]
  }
}
EOF

x=$(cat $TMP_DIR/oadpcreds); echo "$x" | grep -o '^[^#]*'  > $TMP_DIR/oadpcreds

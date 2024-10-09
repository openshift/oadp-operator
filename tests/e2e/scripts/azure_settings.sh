#!/bin/bash

CI_AZURE_SUBSCRIPTION_ID=$(cat $CI_JSON_CRED_FILE | awk '/subscriptionId/ { gsub(/["]|,.*/,""); print $2}')
CI_AZURE_CLIENT_ID=$(cat $CI_JSON_CRED_FILE | awk '/clientId/ { gsub(/["]|,.*/,""); print $2}')
CI_AZURE_CLIENT_SECRET=$(cat $CI_JSON_CRED_FILE | awk '/clientSecret/ { gsub(/["]|,.*/,""); print $2}')
CI_AZURE_TENANT_ID=$(cat $CI_JSON_CRED_FILE | awk '/tenantId/ { gsub(/["]|,.*/,""); print $2}')
CI_AZURE_RESOURCE_GROUP=$(cat $AZURE_RESOURCE_FILE | awk '{ gsub(/["\n \{\}:]|.*infraID/,"");gsub(/,/," "); print $1}')

if [ "$OPENSHIFT_CI" == "true" ]; then\
  CI_AZURE_RESOURCE_GROUP="${CI_AZURE_RESOURCE_GROUP}-rg"; \
fi

cat > $TARGET_CI_CRED_FILE <<EOF
AZURE_SUBSCRIPTION_ID=${CI_AZURE_SUBSCRIPTION_ID}
AZURE_TENANT_ID=${CI_AZURE_TENANT_ID}
AZURE_CLIENT_ID=${CI_AZURE_CLIENT_ID}
AZURE_CLIENT_SECRET=${CI_AZURE_CLIENT_SECRET}
AZURE_RESOURCE_GROUP=${CI_AZURE_RESOURCE_GROUP}
AZURE_CLOUD_NAME=AzurePublicCloud
EOF

AZURE_SUBSCRIPTION_ID=$(cat $OADP_JSON_CRED_FILE | awk '/subscriptionId/ { gsub(/["]|,.*/,""); print $2}')
AZURE_CLIENT_ID=$(cat $OADP_JSON_CRED_FILE | awk '/clientId/ { gsub(/["]|,.*/,""); print $2}')
AZURE_CLIENT_SECRET=$(cat $OADP_JSON_CRED_FILE | awk '/clientSecret/ { gsub(/["]|,.*/,""); print $2}')
AZURE_TENANT_ID=$(cat $OADP_JSON_CRED_FILE | awk '/tenantId/ { gsub(/["]|,.*/,""); print $2}')
AZURE_RESOURCE_GROUP=$(cat $OADP_JSON_CRED_FILE | awk '/resourceGroup/ { gsub(/["]|,.*/,""); print $2}')
AZURE_STORAGE_ACCOUNT_ACCESS_KEY=$(cat $OADP_JSON_CRED_FILE | awk '/storageAccountAccessKey/ { gsub(/["]|,.*/,""); print $2}')
AZURE_STORAGE_ACCOUNT=$(cat $OADP_JSON_CRED_FILE | awk '/"storageAccount"/ { gsub(/["]|,.*/,""); print $2}')

cat > $OADP_CRED_FILE <<EOF
AZURE_SUBSCRIPTION_ID=${AZURE_SUBSCRIPTION_ID}
AZURE_TENANT_ID=${AZURE_TENANT_ID}
AZURE_CLIENT_ID=${AZURE_CLIENT_ID}
AZURE_CLIENT_SECRET=${AZURE_CLIENT_SECRET}
AZURE_RESOURCE_GROUP=${AZURE_RESOURCE_GROUP}
AZURE_STORAGE_ACCOUNT_ACCESS_KEY=${AZURE_STORAGE_ACCOUNT_ACCESS_KEY} 
AZURE_CLOUD_NAME=AzurePublicCloud
EOF

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
              "subscriptionId": "$AZURE_SUBSCRIPTION_ID",
              "storageAccount": "$AZURE_STORAGE_ACCOUNT",
              "resourceGroup": "$AZURE_RESOURCE_GROUP",
              "storageAccountKeyEnvVar": "AZURE_STORAGE_ACCOUNT_ACCESS_KEY"
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
              "subscriptionId": "$CI_AZURE_SUBSCRIPTION_ID",
              "resourceGroup": "$CI_AZURE_RESOURCE_GROUP"
           }
         }
       }
     ]
  }
}
EOF

x=$(cat $TMP_DIR/oadpcreds); echo "$x" | grep -o '^[^#]*'  > $TMP_DIR/oadpcreds

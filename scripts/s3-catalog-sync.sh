#!/bin/bash
# s3-catalog-sync.sh - Sync S3 backup catalog locally using DPA configuration
#
# Reads BSL credentials, bucket, prefix, and region from a DataProtectionApplication
# CR and syncs the S3 contents to a local directory. Safe to re-run — only new or
# changed objects are downloaded.
#
# Usage: s3-catalog-sync.sh <local-catalog-dir> [dpa-name] [namespace]

set -euo pipefail

LOCAL_DIR="${1:-}"
DPA_NAME="${2:-}"
NAMESPACE="${3:-openshift-adp}"

if [[ -z "$LOCAL_DIR" ]]; then
    echo "Usage: $0 <local-catalog-dir> [dpa-name] [namespace]"
    echo ""
    echo "Arguments:"
    echo "  local-catalog-dir Local directory to sync S3 contents into"
    echo "  dpa-name          Name of the DPA CR (auto-detected if only one exists)"
    echo "  namespace         Namespace of the DPA (default: openshift-adp)"
    echo ""
    echo "Example:"
    echo "  $0 ./my-catalog"
    echo "  $0 ./my-catalog dpa-test"
    echo "  $0 /tmp/s3-catalog dpa-test openshift-adp"
    exit 1
fi

# Auto-detect DPA name if not provided
if [[ -z "$DPA_NAME" ]]; then
    DPA_NAMES=$(oc get dpa -n "$NAMESPACE" -o jsonpath='{.items[*].metadata.name}')
    DPA_COUNT=$(echo "$DPA_NAMES" | wc -w)

    if [[ "$DPA_COUNT" -eq 0 ]]; then
        echo "Error: No DataProtectionApplication found in namespace '$NAMESPACE'" >&2
        exit 1
    elif [[ "$DPA_COUNT" -gt 1 ]]; then
        echo "Error: Multiple DPAs found in namespace '$NAMESPACE': $DPA_NAMES" >&2
        echo "Please specify which one: $0 <local-catalog-dir> <dpa-name>" >&2
        exit 1
    fi

    DPA_NAME="$DPA_NAMES"
    echo "Auto-detected DPA: $DPA_NAME"
fi

echo "Reading DPA '$DPA_NAME' in namespace '$NAMESPACE'..."

DPA_JSON=$(oc get dpa "$DPA_NAME" -n "$NAMESPACE" -o json)

# Extract BSL configuration from the first backupLocation
BUCKET=$(echo "$DPA_JSON" | jq -r '.spec.backupLocations[0].velero.objectStorage.bucket')
PREFIX=$(echo "$DPA_JSON" | jq -r '.spec.backupLocations[0].velero.objectStorage.prefix // empty')
REGION=$(echo "$DPA_JSON" | jq -r '.spec.backupLocations[0].velero.config.region // "us-east-1"')
SECRET_NAME=$(echo "$DPA_JSON" | jq -r '.spec.backupLocations[0].velero.credential.name')
SECRET_KEY=$(echo "$DPA_JSON" | jq -r '.spec.backupLocations[0].velero.credential.key')
S3_URL=$(echo "$DPA_JSON" | jq -r '.spec.backupLocations[0].velero.config.s3Url // empty')

if [[ -z "$BUCKET" || "$BUCKET" == "null" ]]; then
    echo "Error: Could not extract bucket from DPA '$DPA_NAME'" >&2
    exit 1
fi

if [[ -z "$SECRET_NAME" || "$SECRET_NAME" == "null" ]]; then
    echo "Error: Could not extract credential secret name from DPA '$DPA_NAME'" >&2
    exit 1
fi

echo "Extracting credentials from secret '$SECRET_NAME' (key: '$SECRET_KEY')..."

CREDS=$(oc get secret "$SECRET_NAME" -n "$NAMESPACE" -o jsonpath="{.data.${SECRET_KEY}}" | base64 -d)

export AWS_ACCESS_KEY_ID=$(echo "$CREDS" | grep aws_access_key_id | awk '{print $3}')
export AWS_SECRET_ACCESS_KEY=$(echo "$CREDS" | grep aws_secret_access_key | awk '{print $3}')
export AWS_DEFAULT_REGION="$REGION"

if [[ -z "$AWS_ACCESS_KEY_ID" || -z "$AWS_SECRET_ACCESS_KEY" ]]; then
    echo "Error: Could not parse AWS credentials from secret '$SECRET_NAME'" >&2
    exit 1
fi

S3_BASE="s3://${BUCKET}"
S3_PATH="${S3_BASE}/${PREFIX}"
S3_PATH_DATAMOVER="${S3_BASE}/${PREFIX}-kubevirt-datamover"

ENDPOINT_ARGS=()
if [[ -n "$S3_URL" ]]; then
    ENDPOINT_ARGS+=(--endpoint-url "$S3_URL")
fi

mkdir -p "$LOCAL_DIR"

echo ""
echo "BSL configuration:"
echo "  Bucket:    $BUCKET"
echo "  Prefix:    ${PREFIX:-<none>}"
echo "  Region:    $REGION"
echo "  S3 paths:"
echo "    - $S3_PATH"
echo "    - $S3_PATH_DATAMOVER"
if [[ -n "$S3_URL" ]]; then
    echo "  Endpoint:  $S3_URL"
fi
echo "  Local dir: $LOCAL_DIR"
echo ""

echo "Syncing ${PREFIX}..."
aws s3 sync "$S3_PATH" "$LOCAL_DIR/${PREFIX}" "${ENDPOINT_ARGS[@]}" 2>&1

echo ""
echo "Syncing ${PREFIX}-kubevirt-datamover..."
aws s3 sync "$S3_PATH_DATAMOVER" "$LOCAL_DIR/${PREFIX}-kubevirt-datamover" "${ENDPOINT_ARGS[@]}" 2>&1

echo ""
echo "Sync complete. Contents:"
echo ""
# Show top-level directory summary
du -sh "$LOCAL_DIR"/*/ 2>/dev/null || echo "  (empty or flat structure)"
echo ""
echo "Re-run the same command to pick up new backups."

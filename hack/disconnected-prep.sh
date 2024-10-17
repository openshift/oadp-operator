#!/bin/bash

#Find most recent version
export OADPTAG=$(git branch --show-current)
git checkout origin/$(git branch --show-current) -- Dockerfile
git checkout origin/$(git branch --show-current) -- .gitignore
for file in $(git status --porcelain -- render_templates* | awk '{print $2}'); do
  git checkout origin/$(git branch --show-current) -- "${file}"
done


#Declare image information
IMAGES=(
  "operator"
  "plugin"
  "velero"
  "helper"
  "gcpplugin"
  "awsplugin"
  "legacyawsplugin"
  "azureplugin"
  "registry"
)

declare -A IMG_MAP
IMG_MAP[operator_repo]="oadp-operator"
IMG_MAP[plugin_repo]="openshift-velero-plugin"
IMG_MAP[velero_repo]="velero"
IMG_MAP[helper_repo]="velero-restore-helper"
IMG_MAP[gcpplugin_repo]="velero-plugin-for-gcp"
IMG_MAP[awsplugin_repo]="velero-plugin-for-aws"
IMG_MAP[legacyawsplugin_repo]="velero-plugin-for-legacy-aws"
IMG_MAP[azureplugin_repo]="velero-plugin-for-microsoft-azure"
IMG_MAP[registry_repo]="registry"

#Get latest images
for i in ${IMAGES[@]}; do
  docker pull quay.io/konveyor/${IMG_MAP[${i}_repo]}:${OADPTAG} >/dev/null 2>&1
  DOCKER_STAT=$?
  RETRIES=10
  while [ "$DOCKER_STAT" -ne 0 ] && [ $RETRIES -gt 0 ]; do
    docker pull quay.io/konveyor/${IMG_MAP[${i}_repo]}:${OADPTAG} >/dev/null 2>&1
    DOCKER_STAT=$?
    let RETRIES=RETRIES-1
  done

  if [ $RETRIES -le 0 ]; then
    echo "Failed to pull new images"
    exit 1
  fi
done

#oc mirror images to get correct shas
for i in ${IMAGES[@]}; do
  RETRIES=10
  while [ -z "${IMG_MAP[${i}_sha]}" ] && [ $RETRIES -gt 0 ]; do
    IMG_MAP[${i}_sha]=$(oc image mirror --keep-manifest-list=true --dry-run=true quay.io/konveyor/${IMG_MAP[${i}_repo]}:${OADPTAG}=quay.io/foobar/${IMG_MAP[${i}_repo]}:${OADPTAG} 2>&1 | grep "\->" | awk -F'[: ]' '{ print $8 }')
    let RETRIES=RETRIES-1
  done

  if [ $RETRIES -le 0 ]; then
    echo "Failed to mirror images to obtain SHAs"
    exit 1
  fi
done


# Make CSV Changes check for OADP version inf csv file name
for f in bundle/manifests/oadp-operator.clusterserviceversion.yaml
  do
  sed -i "s,oadp-operator:.*,oadp-operator@sha256:${IMG_MAP[operator_sha]},g"                                                              ${f}
  sed -i "s,/velero:.*,/velero@sha256:${IMG_MAP[velero_sha]},g"                                                                            ${f}
  sed -i "s,/velero-restore-helper:.*,/velero-restore-helper@sha256:${IMG_MAP[helper_sha]},g"                                ${f}
  sed -i "s,/openshift-velero-plugin:.*,/openshift-velero-plugin@sha256:${IMG_MAP[plugin_sha]},g"                                          ${f}
  sed -i "s,/velero-plugin-for-aws:.*,/velero-plugin-for-aws@sha256:${IMG_MAP[awsplugin_sha]},g"                                           ${f}
  sed -i "s,/velero-plugin-for-legacy-aws:.*,/velero-plugin-for-legacy-aws@sha256:${IMG_MAP[legacyawsplugin_sha]},g"                                           ${f}
  sed -i "s,/velero-plugin-for-microsoft-azure:.*,/velero-plugin-for-microsoft-azure@sha256:${IMG_MAP[azureplugin_sha]},g"                 ${f}
  sed -i "s,/velero-plugin-for-gcp:.*,/velero-plugin-for-gcp@sha256:${IMG_MAP[gcpplugin_sha]},g"                                           ${f}
  sed -i "s,/registry:.*,/registry@sha256:${IMG_MAP[registry_sha]},g"                                                                      ${f}
  sed -i 's,value: velero-restore-helper,value: velero-restore-helper@sha256,g'                                              ${f}
  sed -i 's,value: velero-plugin-for-gcp,value: velero-plugin-for-gcp@sha256,g'                                                            ${f}
  sed -i 's,value: velero-plugin-for-aws,value: velero-plugin-for-aws@sha256,g'                                                            ${f}
  sed -i 's,value: velero-plugin-for-legacy-aws,value: velero-plugin-for-legacy-aws@sha256,g'                                                            ${f}
  sed -i 's,value: velero-plugin-for-microsoft-azure,value: velero-plugin-for-microsoft-azure@sha256,g'                                    ${f}
  sed -i 's,value: velero$,value: velero@sha256,g'                                                                                         ${f}
  sed -i 's,value: openshift-velero-plugin$,value: openshift-velero-plugin@sha256,g'                                                       ${f}
  sed -i 's,value: registry$,value: registry@sha256,g'                                                                                     ${f}
  sed -i "/VELERO_OPENSHIFT_PLUGIN_TAG/,/^ *[^:]*:/s/value: .*/value: ${IMG_MAP[plugin_sha]}/"                                                                       ${f}
  sed -i "/VELERO_TAG/,/^ *[^:]*:/s/value: .*/value: ${IMG_MAP[velero_sha]}/"                                                                                        ${f}
  sed -i "/VELERO_RESTORE_HELPER_TAG/,/^ *[^:]*:/s/value: .*/value: ${IMG_MAP[helper_sha]}/"                                                                  ${f}
  sed -i "/VELERO_GCP_PLUGIN_TAG/,/^ *[^:]*:/s/value: .*/value: ${IMG_MAP[gcpplugin_sha]}/"                                                                          ${f}
  sed -i "/VELERO_AWS_PLUGIN_TAG/,/^ *[^:]*:/s/value: .*/value: ${IMG_MAP[awsplugin_sha]}/"                                                                          ${f}
  sed -i "/VELERO_LEGACY_AWS_PLUGIN_TAG/,/^ *[^:]*:/s/value: .*/value: ${IMG_MAP[legacyawsplugin_sha]}/"                                                                          ${f}
  sed -i "/VELERO_AZURE_PLUGIN_TAG/,/^ *[^:]*:/s/value: .*/value: ${IMG_MAP[azureplugin_sha]}/"                                                                      ${f}
  sed -i "/VELERO_REGISTRY_TAG/,/^ *[^:]*:/s/value: .*/value: ${IMG_MAP[registry_sha]}/"                                                                             ${f}
if [[ "$f" =~ .*clusterserviceversion.* ]] && ! grep -q infrastructure-features ${f}; then
  sed -i '/^spec:/i\ \ \ \ operators.openshift.io/infrastructure-features: \x27[\"Disconnected\"]\x27'                                                               ${f}
fi
done

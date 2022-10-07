#!/bin/bash
# gather CRs if must-gather do not work.
# logs are not collected here because they are collected by must-gather and must-gather fails normally due to inability to collect logs from velero server.
# We can introduce a separate script to collect logs from velero server if needed.
mkdir manual-gather
cd manual-gather
for apiresource in $(oc api-resources -oname | grep -e 'oadp\.openshift\.io\|velero\.io'); do
    echo "Collecting $apiresource"
    mkdir -p $(dirname $apiresource) && oc get $apiresource -Ao yaml > $apiresource.yaml
done
for crd in $(oc get crd -o name | grep -e 'oadp\|velero'); do \
  echo "Collecting $crd"
  mkdir -p $(dirname $crd) && oc get $crd -Ao yaml > $crd.yaml; \
done
cd ..
zip -r manual-gather.zip manual-gather

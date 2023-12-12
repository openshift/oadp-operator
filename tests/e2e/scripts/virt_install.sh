#!/bin/bash

export CSV="${CSV:=kubevirt-hyperconverged-operator.v4.13.6}"
export VIRT_NAMESPACE="${VIRT_NAMESPACE:=openshift-cnv}"
export OC_CLI="${OC_CLI:=oc}"


# Check for HCO, if it exists assume virt is already installed.
function cnv_hco_exists() {
    echo "Checking for HCO..."
    $OC_CLI get hco -n $VIRT_NAMESPACE kubevirt-hyperconverged
}
export -f cnv_hco_exists

if cnv_hco_exists
then
    echo "CNV already installed with HCO, nothing to do here..."
    exit 0
else
    echo "No HCO found, installing virt operator..."
fi


# Check for virt operator subscription, and install if needed.
function create_cnv_subscription() {
    $OC_CLI apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: $VIRT_NAMESPACE
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: kubevirt-hyperconverged-group
  namespace: $VIRT_NAMESPACE
spec:
  targetNamespaces:
    - $VIRT_NAMESPACE
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: hco-operatorhub
  namespace: $VIRT_NAMESPACE
spec:
  source: redhat-operators
  sourceNamespace: openshift-marketplace
  name: kubevirt-hyperconverged
  startingCSV: $CSV
  installPlanApproval: Automatic
  channel: "stable"
EOF
}
function cnv_subscription_exists() {
    echo "Checking CNV subscription..."
    $OC_CLI get subscription -n $VIRT_NAMESPACE hco-operatorhub
}
export -f cnv_subscription_exists

if cnv_subscription_exists
then
    echo "CNV subscription already present."
else
    echo "Installing CNV subscription..."
    create_cnv_subscription
fi


# Wait for subscription
function wait_cnv_subscription_exists() {
    echo "Waiting for CNV subscription..."
    until cnv_subscription_exists
    do
        sleep 1
    done
}
export -f wait_cnv_subscription_exists

if timeout 30s bash -c wait_cnv_subscription_exists
then
    echo "Found subscription."
else
    echo "Failed to find CNV subscription!"
    exit 1
fi


# Wait for virt ClusterServiceVersion
function cnv_csv_succeeded() {
    echo "Checking CNV CSV..."
    PHASE=$($OC_CLI get csv -n $VIRT_NAMESPACE $CSV -o jsonpath='{.status.phase}')
    echo "CSV phase is $PHASE"
    test "$PHASE" == "Succeeded"
}
export -f cnv_csv_succeeded

function wait_cnv_csv_succeeded() {
    echo "Waiting for virt CSV to install..."
    until cnv_csv_succeeded
    do
        sleep 5
    done
}
export -f wait_cnv_csv_succeeded
if timeout 2m bash -c wait_cnv_csv_succeeded
then
    echo "CSV succeeded."
else
    echo "Failed installing CSV after two minutes!"
    exit 1
fi



# Create a HyperConvergedOperator instance
function create_cnv_hco() {
    echo "Installing HCO..."
    $OC_CLI apply -f - <<EOF
apiVersion: hco.kubevirt.io/v1beta1
kind: HyperConverged
metadata:
  name: kubevirt-hyperconverged
  namespace: $VIRT_NAMESPACE
spec:
EOF
}

function wait_cnv_hco_exists() {
    echo "Waiting for HCO..."
    until cnv_hco_exists
    do
        sleep 5
    done
}
export -f wait_cnv_hco_exists

create_cnv_hco
if timeout 5m bash -c wait_cnv_hco_exists
then
    echo "Found HCO."
else
    echo "Failed to create HCO!"
    exit 1
fi

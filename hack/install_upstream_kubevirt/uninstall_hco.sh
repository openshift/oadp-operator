#!/bin/bash
# Uninstall upstream HyperConverged Cluster Operator (HCO) and KubeVirt from OpenShift.
#
# Handles finalizers and ordered teardown carefully to avoid stuck resources.
#
# Usage:
#   ./uninstall_hco.sh
#
# Environment variables:
#   HCO_NAMESPACE     Namespace HCO was installed into.
#                     Auto-detected from hco-operator deployment if not set.
#                     (default: kubevirt-hyperconverged)
#
#   OC_TOOL           oc or kubectl binary.
#                     (default: oc)

set -euo pipefail

# ---------------------------------------------------------------------------
# Defaults
# ---------------------------------------------------------------------------
OC_TOOL="${OC_TOOL:-oc}"

# Auto-detect namespace from the running hco-operator deployment, fall back to default
HCO_NAMESPACE="${HCO_NAMESPACE:-}"
if [[ -z "${HCO_NAMESPACE}" ]]; then
    HCO_NAMESPACE=$(
        ${OC_TOOL} get deployments --all-namespaces \
            --field-selector='metadata.name=hco-operator' \
            -o jsonpath='{$.items[0].metadata.namespace}' \
            --ignore-not-found 2>/dev/null || true
    )
    HCO_NAMESPACE="${HCO_NAMESPACE:-kubevirt-hyperconverged}"
fi

readonly HCO_NAMESPACE OC_TOOL

CATALOGSOURCE_NAME="kubevirt-hyperconverged"
CATALOGSOURCE_NS="openshift-marketplace"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
log()  { echo "[INFO]  $(date +%H:%M:%S)  $*"; }
warn() { echo "[WARN]  $(date +%H:%M:%S)  $*"; }
err()  { echo "[ERROR] $(date +%H:%M:%S)  $*" >&2; }

oc_delete() { ${OC_TOOL} delete --ignore-not-found "$@"; }

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    log "============================================================"
    log " HCO / KubeVirt Upstream Uninstall"
    log "============================================================"
    log "  HCO_NAMESPACE = ${HCO_NAMESPACE}"
    log "============================================================"

    if ! ${OC_TOOL} whoami &>/dev/null; then
        err "Not logged in to a cluster. Run 'oc login' first."
        exit 1
    fi

    # -----------------------------------------------------------------------
    # 1. Set uninstallStrategy=RemoveWorkloads on KubeVirt and CDI so that
    #    HCO does not block deletion due to running workloads.
    # -----------------------------------------------------------------------
    log "Setting uninstallStrategy=RemoveWorkloads on KubeVirt/CDI..."
    ${OC_TOOL} get cdis,kubevirts -n "${HCO_NAMESPACE}" \
        --output='name' --ignore-not-found 2>/dev/null \
        | xargs -r ${OC_TOOL} patch -n "${HCO_NAMESPACE}" \
            --type='merge' \
            --patch='{"spec":{"uninstallStrategy":"RemoveWorkloads"}}' \
        || true

    # -----------------------------------------------------------------------
    # 2. Disable common boot image import to stop background DataVolume creation
    # -----------------------------------------------------------------------
    log "Disabling HCO boot image import..."
    if ${OC_TOOL} get hyperconverged kubevirt-hyperconverged \
            -n "${HCO_NAMESPACE}" &>/dev/null 2>&1; then
        ${OC_TOOL} patch hyperconverged kubevirt-hyperconverged \
            -n "${HCO_NAMESPACE}" \
            --type='merge' \
            --patch='{"spec":{"enableCommonBootImageImport":false}}' \
            || true
        sleep 5
    fi

    # -----------------------------------------------------------------------
    # 3. Delete VMs, VMIs, DataVolumes, DataImportCrons
    #    (order matters: VM → VMI → DV → DataImportCron)
    # -----------------------------------------------------------------------
    log "Removing VM workloads..."
    for kind in vm vmi dv dataimportcron; do
        if ${OC_TOOL} get "${kind}" --all-namespaces &>/dev/null 2>&1; then
            oc_delete "${kind}" --all-namespaces --all || true
            ${OC_TOOL} wait "${kind}" --all-namespaces --all \
                --for=delete --timeout=3m || true
        fi
    done

    # -----------------------------------------------------------------------
    # 4. Delete HPP (HostPath Provisioner) and related StorageClasses
    # -----------------------------------------------------------------------
    log "Removing HostPath Provisioner..."
    if ${OC_TOOL} get hostpathprovisioners --all-namespaces &>/dev/null 2>&1; then
        oc_delete hostpathprovisioners --all-namespaces --all || true
    fi
    for sc in hostpath-provisioner hostpath-csi hostpath-csi-basic hostpath-csi-pvc-block; do
        oc_delete storageclass "${sc}" || true
    done
    ${OC_TOOL} wait daemonsets -n "${HCO_NAMESPACE}" \
        --selector='k8s-app=hostpath-provisioner' \
        --for=delete --timeout=3m || true

    # -----------------------------------------------------------------------
    # 5. Delete image streams created by the OS image namespace
    # -----------------------------------------------------------------------
    log "Removing image streams..."
    oc_delete imagestream -n openshift-virtualization-os-images --all || true

    # -----------------------------------------------------------------------
    # 6. Delete the HyperConverged CR and wait for operator-driven teardown
    # -----------------------------------------------------------------------
    log "Deleting HyperConverged CR..."
    if ${OC_TOOL} get hyperconverged kubevirt-hyperconverged \
            -n "${HCO_NAMESPACE}" &>/dev/null 2>&1; then
        oc_delete hyperconverged --all-namespaces --all || true
    fi
    log "Waiting for HCO-managed deployments/daemonsets to be removed (2m)..."
    ${OC_TOOL} wait deployments,daemonsets -n "${HCO_NAMESPACE}" \
        --selector='!olm.owner' \
        --for=delete --timeout=2m || true

    # -----------------------------------------------------------------------
    # 7. Remove OLM resources: CSV → Subscription → OperatorGroup → CatalogSource
    # -----------------------------------------------------------------------
    log "Removing OLM resources..."
    oc_delete clusterserviceversions -n "${HCO_NAMESPACE}" \
        --selector="operators.coreos.com/community-kubevirt-hyperconverged.${HCO_NAMESPACE}" \
        || true
    # Wait for operator-owned deployments to go away
    ${OC_TOOL} wait deployments -n "${HCO_NAMESPACE}" \
        --selector='olm.owner' \
        --for=delete --timeout=3m || true

    oc_delete subscriptions.operators -n "${HCO_NAMESPACE}" --all || true
    oc_delete operatorgroups -n "${HCO_NAMESPACE}" --all || true
    oc_delete catalogsource "${CATALOGSOURCE_NAME}" \
        -n "${CATALOGSOURCE_NS}" || true

    # -----------------------------------------------------------------------
    # 8. Remove cluster-scoped leftovers: CRDs, webhooks, RBAC, API services
    #    Use awk to match kubevirt-owned resources safely (avoids false matches)
    # -----------------------------------------------------------------------
    log "Removing cluster-scoped kubevirt resources..."
    for kind in \
        clusterroles \
        clusterrolebindings \
        validatingwebhookconfigurations \
        mutatingwebhookconfigurations \
        apiservices \
        customresourcedefinitions
    do
        ${OC_TOOL} get "${kind}" --no-headers --ignore-not-found 2>/dev/null \
            | awk '$1 ~ /[.\-]kubevirt[.\-]?/ || $1 ~ /^kubevirt/ { print $1 }' \
            | xargs -r ${OC_TOOL} delete "${kind}" --ignore-not-found \
            || true
    done

    # -----------------------------------------------------------------------
    # 9. Delete the HCO namespace and the OS images namespace
    # -----------------------------------------------------------------------
    log "Deleting namespaces..."
    oc_delete namespace "${HCO_NAMESPACE}" || true
    oc_delete namespace openshift-virtualization-os-images || true

    # Wait for namespace termination
    log "Waiting for namespace '${HCO_NAMESPACE}' to terminate (5m)..."
    ${OC_TOOL} wait namespace "${HCO_NAMESPACE}" \
        --for=delete --timeout=5m || true

    # -----------------------------------------------------------------------
    # 10. Reclaim any HPP LSO PVs
    # -----------------------------------------------------------------------
    log "Reclaiming HPP local storage PVs (if any)..."
    ${OC_TOOL} get pv \
        -l storage.openshift.com/local-volume-owner-name=local-block-hpp \
        -o name --ignore-not-found 2>/dev/null \
        | xargs -r -I{} ${OC_TOOL} patch {} \
            --type='merge' \
            --patch='{"spec":{"claimRef":null}}' \
        || true

    log "============================================================"
    log " Uninstall Complete"
    log "============================================================"

    # Final check for lingering kubevirt CRDs
    local remaining
    remaining=$(${OC_TOOL} get crd --no-headers --ignore-not-found 2>/dev/null \
        | awk '$1 ~ /kubevirt|hco\.kubevirt|cdi\.kubevirt|ssp\.kubevirt/' \
        | wc -l || true)
    if (( remaining > 0 )); then
        warn "${remaining} kubevirt-related CRD(s) still present — may need manual removal:"
        ${OC_TOOL} get crd --no-headers 2>/dev/null \
            | awk '$1 ~ /kubevirt|hco\.kubevirt|cdi\.kubevirt|ssp\.kubevirt/ { print "  " $1 }' \
            || true
    else
        log "No kubevirt CRDs remaining. Cluster is clean."
    fi
}

main "$@"

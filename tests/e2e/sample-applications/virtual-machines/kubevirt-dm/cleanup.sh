#!/usr/bin/env bash
# cleanup.sh - Clean up kubevirt-datamover resources left behind after a backup run.
#
# Removes:
#   - Datamover uploader pods          (kubevirt-dm-du-*        in OADP namespace)
#   - Datamover staging PVCs           (kubevirt-dm-pvc-du-*    in OADP namespace)
#   - VMB source PVCs                  (kubevirt-backup-du-*    in APP namespace)
#   - Orphaned / Released Retain PVs   (backing any of the above)
#   - VirtualMachineBackup             (vmb-*                   in APP namespace)
#   - VirtualMachineBackupTracker      (vmbt-*                  in APP namespace)
#   - DataUpload objects               (du-kubevirt-dm-*        in OADP namespace)
#
# Usage:
#   ./cleanup.sh [APP_NAMESPACE] [OADP_NAMESPACE]
#
# Defaults:
#   APP_NAMESPACE  - auto-detected from VirtualMachineBackup resources
#   OADP_NAMESPACE - openshift-adp

set -euo pipefail

APP_NS="${1:-}"
OADP_NS="${2:-openshift-adp}"

# ── colours ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'; YELLOW='\033[1;33m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; NC='\033[0m'
info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()      { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
deleted() { echo -e "${GREEN}[DEL]${NC}   $*"; }

# ── helpers ───────────────────────────────────────────────────────────────────
delete_if_exists() {
    local kind="$1" name="$2" ns_flag="${3:-}"
    if oc get "$kind" "$name" $ns_flag &>/dev/null; then
        oc delete "$kind" "$name" $ns_flag --ignore-not-found
        deleted "$kind/$name"
    fi
}

wait_for_deletion() {
    local kind="$1" name="$2" ns_flag="${3:-}" timeout="${4:-60}"
    local elapsed=0
    while oc get "$kind" "$name" $ns_flag &>/dev/null; do
        if (( elapsed >= timeout )); then
            warn "Timed out waiting for $kind/$name to be deleted"
            return 1
        fi
        sleep 2; (( elapsed += 2 ))
    done
}

# ── auto-detect app namespace ─────────────────────────────────────────────────
if [[ -z "$APP_NS" ]]; then
    info "No APP_NAMESPACE specified, auto-detecting from VirtualMachineBackup resources..."
    APP_NS=$(oc get virtualmachinebackup -A --no-headers 2>/dev/null \
             | awk '{print $1}' | sort -u | head -1 || true)
    if [[ -z "$APP_NS" ]]; then
        # Fall back to VirtualMachineBackupTracker
        APP_NS=$(oc get virtualmachinebackuptracker -A --no-headers 2>/dev/null \
                 | awk '{print $1}' | sort -u | head -1 || true)
    fi
    if [[ -z "$APP_NS" ]]; then
        warn "Could not auto-detect application namespace. Set APP_NAMESPACE as first argument."
        warn "Skipping VirtualMachineBackup/Tracker cleanup."
    else
        info "Auto-detected application namespace: ${APP_NS}"
    fi
fi

echo ""
info "=== kubevirt-datamover cleanup ==="
info "  OADP namespace : ${OADP_NS}"
info "  App  namespace : ${APP_NS:-<skipped>}"
echo ""

# ── 1. Datamover uploader pods ────────────────────────────────────────────────
info "--- Datamover uploader pods (${OADP_NS}) ---"
pods=$(oc get pods -n "${OADP_NS}" --no-headers 2>/dev/null \
       | awk '/^kubevirt-dm-du-/{print $1}' || true)
if [[ -z "$pods" ]]; then
    ok "No datamover uploader pods found."
else
    for pod in $pods; do
        oc delete pod "$pod" -n "${OADP_NS}" --ignore-not-found
        deleted "pod/$pod"
    done
fi

# ── 2. DataUpload objects ─────────────────────────────────────────────────────
info "--- DataUpload objects (${OADP_NS}) ---"
dus=$(oc get dataupload -n "${OADP_NS}" --no-headers 2>/dev/null \
      | awk '/^du-kubevirt-dm-/{print $1}' || true)
if [[ -z "$dus" ]]; then
    ok "No kubevirt-dm DataUpload objects found."
else
    for du in $dus; do
        oc delete dataupload "$du" -n "${OADP_NS}" --ignore-not-found
        deleted "dataupload/$du"
    done
fi

# ── 3. VirtualMachineBackup + VirtualMachineBackupTracker ────────────────────
if [[ -n "$APP_NS" ]]; then
    info "--- VirtualMachineBackup (${APP_NS}) ---"
    vmbs=$(oc get virtualmachinebackup -n "${APP_NS}" --no-headers 2>/dev/null \
           | awk '{print $1}' || true)
    if [[ -z "$vmbs" ]]; then
        ok "No VirtualMachineBackup objects found."
    else
        for vmb in $vmbs; do
            # Remove finalizer first - VMBs can get stuck with one
            oc patch virtualmachinebackup "$vmb" -n "${APP_NS}" \
               --type=merge -p '{"metadata":{"finalizers":[]}}' &>/dev/null || true
            oc delete virtualmachinebackup "$vmb" -n "${APP_NS}" --ignore-not-found
            deleted "virtualmachinebackup/$vmb"
        done
    fi

    info "--- VirtualMachineBackupTracker (${APP_NS}) ---"
    vmbts=$(oc get virtualmachinebackuptracker -n "${APP_NS}" --no-headers 2>/dev/null \
            | awk '{print $1}' || true)
    if [[ -z "$vmbts" ]]; then
        ok "No VirtualMachineBackupTracker objects found."
    else
        for vmbt in $vmbts; do
            oc patch virtualmachinebackuptracker "$vmbt" -n "${APP_NS}" \
               --type=merge -p '{"metadata":{"finalizers":[]}}' &>/dev/null || true
            oc delete virtualmachinebackuptracker "$vmbt" -n "${APP_NS}" --ignore-not-found
            deleted "virtualmachinebackuptracker/$vmbt"
        done
    fi

    info "--- VMB source PVCs (kubevirt-backup-du-* in ${APP_NS}) ---"
    vmbpvcs=$(oc get pvc -n "${APP_NS}" --no-headers 2>/dev/null \
              | awk '/^kubevirt-backup-du-/{print $1}' || true)
    if [[ -z "$vmbpvcs" ]]; then
        ok "No VMB source PVCs found."
    else
        for pvc in $vmbpvcs; do
            oc delete pvc "$pvc" -n "${APP_NS}" --ignore-not-found
            deleted "pvc/$pvc (${APP_NS})"
        done
    fi
fi

# ── 4. Staging PVCs in OADP namespace ────────────────────────────────────────
info "--- Datamover staging PVCs (kubevirt-dm-pvc-du-* in ${OADP_NS}) ---"
staging_pvcs=$(oc get pvc -n "${OADP_NS}" --no-headers 2>/dev/null \
               | awk '/^kubevirt-dm-pvc-du-/{print $1}' || true)
if [[ -z "$staging_pvcs" ]]; then
    ok "No staging PVCs found."
else
    # Collect backing PV names before deleting PVCs
    declare -a pvs_to_delete=()
    for pvc in $staging_pvcs; do
        pv=$(oc get pvc "$pvc" -n "${OADP_NS}" \
             -o jsonpath='{.spec.volumeName}' 2>/dev/null || true)
        [[ -n "$pv" ]] && pvs_to_delete+=("$pv")
        oc delete pvc "$pvc" -n "${OADP_NS}" --ignore-not-found
        deleted "pvc/$pvc (${OADP_NS})"
    done

    # Wait briefly for PVCs to clear before touching PVs
    sleep 3

    # ── 5. Backing PVs (Retain policy → won't auto-delete) ───────────────────
    info "--- Backing PVs (Retain policy) ---"
    for pv in "${pvs_to_delete[@]+"${pvs_to_delete[@]}"}"; do
        policy=$(oc get pv "$pv" -o jsonpath='{.spec.persistentVolumeReclaimPolicy}' 2>/dev/null || true)
        if [[ "$policy" == "Retain" ]]; then
            oc delete pv "$pv" --ignore-not-found
            deleted "pv/$pv (Retain)"
        else
            ok "pv/$pv has ${policy} policy — will self-delete."
        fi
    done
fi

# ── 6. Any remaining Released/orphaned Retain PVs for kubevirt-dm ────────────
info "--- Orphaned Released Retain PVs (kubevirt-dm-pvc-du-*) ---"
orphan_pvs=$(oc get pv --no-headers 2>/dev/null \
             | awk '/Retain.*Released.*kubevirt-dm-pvc-du-/{print $1}' || true)
if [[ -z "$orphan_pvs" ]]; then
    ok "No orphaned Released PVs found."
else
    for pv in $orphan_pvs; do
        oc delete pv "$pv" --ignore-not-found
        deleted "pv/$pv (orphaned Released)"
    done
fi

echo ""
echo "bounce the oadp-kubevirt-datamover controller"
oc delete deployment.apps/oadp-kubevirt-datamover-controller-manager
echo "bounce the virt controlers in openshift-cnv"
oc delete deployment virt-controller -n openshift-cnv


ok "=== Cleanup complete ==="
echo ""
info "Remaining kubevirt-dm resources:"
echo "  Pods:    $(oc get pods -n "${OADP_NS}" --no-headers 2>/dev/null | grep -c kubevirt-dm || echo 0)"
echo "  PVCs:    $(oc get pvc  -n "${OADP_NS}" --no-headers 2>/dev/null | grep -c kubevirt-dm || echo 0)"
echo "  PVs:     $(oc get pv          --no-headers 2>/dev/null | grep -c kubevirt-dm || echo 0)"
if [[ -n "$APP_NS" ]]; then
echo "  VMB:     $(oc get virtualmachinebackup        -n "${APP_NS}" --no-headers 2>/dev/null | wc -l | tr -d ' ')"
echo "  VMBT:    $(oc get virtualmachinebackuptracker -n "${APP_NS}" --no-headers 2>/dev/null | wc -l | tr -d ' ')"
fi

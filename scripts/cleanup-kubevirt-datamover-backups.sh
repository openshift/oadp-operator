#!/bin/bash
# cleanup-backups.sh - Remove all OADP kubevirt-datamover backup objects from the cluster
#
# Removes: Velero backups, DataUploads, VirtualMachineBackups, VirtualMachineBackupTrackers,
#          and ONLY PVCs created by the datamover (identified by name prefix and labels).
#
# Usage: cleanup-backups.sh [namespace]

set -u

NAMESPACE="${1:-openshift-adp}"
DRY_RUN="${DRY_RUN:-true}"

RED=$'\033[1;31m'
GREEN=$'\033[1;32m'
YELLOW=$'\033[1;33m'
CYAN=$'\033[1;36m'
DIM=$'\033[2m'
RST=$'\033[0m'

deleted=0

run() {
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "  ${DIM}[dry-run]${RST} $*"
    else
        "$@" 2>&1
    fi
}

header() {
    printf "\n${CYAN}== %s ==${RST}\n" "$1"
}

# Strip finalizers from a resource so delete doesn't hang
remove_finalizers() {
    local resource="$1" name="$2" ns="$3"
    local finalizers
    finalizers=$(oc get "$resource" "$name" -n "$ns" -o jsonpath='{.metadata.finalizers}' 2>/dev/null || echo "")
    if [[ -n "$finalizers" && "$finalizers" != "[]" && "$finalizers" != "null" ]]; then
        echo "    ${DIM}removing finalizers from $name${RST}"
        run oc patch "$resource" "$name" -n "$ns" --type=merge -p '{"metadata":{"finalizers":[]}}' 2>/dev/null
    fi
}

# ── Velero Backups ────────────────────────────────────────────────────────────
header "Velero Backups"
backups=$(oc get backup.velero.io -n "$NAMESPACE" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || echo "")
if [[ -z "$backups" ]]; then
    echo "  ${DIM}No backups found${RST}"
else
    for b in $backups; do
        echo "  ${RED}Deleting${RST} backup ${YELLOW}$b${RST}"
        run velero backup delete "$b" -n "$NAMESPACE" --confirm
        (( deleted++ ))
    done
fi

# ── VirtualMachineBackups (before DataUploads — they depend on DU) ────────────
header "VirtualMachineBackups"
vmbs=$(oc get virtualmachinebackup -A -o jsonpath='{range .items[*]}{.metadata.namespace}{" "}{.metadata.name}{"\n"}{end}' 2>/dev/null || echo "")
if [[ -z "$vmbs" ]]; then
    echo "  ${DIM}No VirtualMachineBackups found${RST}"
else
    while IFS=' ' read -r ns name; do
        [[ -z "$name" ]] && continue
        echo "  ${RED}Deleting${RST} vmb ${YELLOW}$ns/$name${RST}"
        remove_finalizers "virtualmachinebackup" "$name" "$ns"
        run oc delete virtualmachinebackup "$name" -n "$ns" --wait=false
        (( deleted++ ))
    done <<< "$vmbs"
fi

# ── VirtualMachineBackupTrackers ──────────────────────────────────────────────
header "VirtualMachineBackupTrackers"
vmbts=$(oc get virtualmachinebackuptracker -A -o jsonpath='{range .items[*]}{.metadata.namespace}{" "}{.metadata.name}{"\n"}{end}' 2>/dev/null || echo "")
if [[ -z "$vmbts" ]]; then
    echo "  ${DIM}No VirtualMachineBackupTrackers found${RST}"
else
    while IFS=' ' read -r ns name; do
        [[ -z "$name" ]] && continue
        echo "  ${RED}Deleting${RST} vmbt ${YELLOW}$ns/$name${RST}"
        remove_finalizers "virtualmachinebackuptracker" "$name" "$ns"
        run oc delete virtualmachinebackuptracker "$name" -n "$ns" --wait=false
        (( deleted++ ))
    done <<< "$vmbts"
fi

# ── DataUploads ───────────────────────────────────────────────────────────────
header "DataUploads"
dus=$(oc get dataupload.velero.io -n "$NAMESPACE" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || echo "")
if [[ -z "$dus" ]]; then
    echo "  ${DIM}No DataUploads found${RST}"
else
    for du in $dus; do
        echo "  ${RED}Deleting${RST} dataupload ${YELLOW}$du${RST}"
        remove_finalizers "dataupload.velero.io" "$du" "$NAMESPACE"
        run oc delete dataupload.velero.io "$du" -n "$NAMESPACE" --wait=false
        (( deleted++ ))
    done
fi

# ── Datamover PVCs ────────────────────────────────────────────────────────────
# ONLY delete PVCs created by the datamover, identified by:
#   1. Name prefix: kubevirt-backup-  (temp PVCs created in VM namespace)
#   2. Name prefix: kubevirt-dm-pvc-  (rebound PVCs created in OADP namespace)
#   3. Label: velero.io/dataupload-name  (all datamover-created PVCs carry this)
# This ensures we NEVER touch VM disk PVCs or any other PVCs on the cluster.
header "Datamover PVCs (safe cleanup)"

# Method 1: by label (most reliable — catches all datamover PVCs regardless of name)
labeled_pvcs=$(oc get pvc -A -l "velero.io/dataupload-name" \
    -o jsonpath='{range .items[*]}{.metadata.namespace}{" "}{.metadata.name}{"\n"}{end}' 2>/dev/null || echo "")

# Method 2: by name prefix (fallback — catches any that might be missing the label)
prefix_pvcs=""
for prefix in "kubevirt-backup-" "kubevirt-dm-pvc-"; do
    matches=$(oc get pvc -A -o jsonpath='{range .items[*]}{.metadata.namespace}{" "}{.metadata.name}{"\n"}{end}' 2>/dev/null \
        | grep " ${prefix}" || echo "")
    if [[ -n "$matches" ]]; then
        prefix_pvcs="${prefix_pvcs}${matches}"$'\n'
    fi
done

# Combine and deduplicate
all_pvcs=$(printf '%s\n%s' "$labeled_pvcs" "$prefix_pvcs" | sort -u | grep -v '^$' || echo "")

if [[ -z "$all_pvcs" ]]; then
    echo "  ${DIM}No datamover PVCs found${RST}"
else
    while IFS=' ' read -r ns name; do
        [[ -z "$name" ]] && continue
        echo "  ${RED}Deleting${RST} pvc ${YELLOW}$ns/$name${RST}"
        remove_finalizers "pvc" "$name" "$ns"
        run oc delete pvc "$name" -n "$ns" --wait=false
        (( deleted++ ))
    done <<< "$all_pvcs"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
printf "\n${GREEN}Done.${RST} Deleted ${YELLOW}%d${RST} resources.\n" "$deleted"
if [[ "$DRY_RUN" == "true" ]]; then
    printf "${DIM}(dry-run mode — nothing was actually deleted. Run with DRY_RUN=false to delete.)${RST}\n"
fi

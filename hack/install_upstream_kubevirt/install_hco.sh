#!/bin/bash
# Install upstream HyperConverged Cluster Operator (HCO) and KubeVirt on OpenShift via OLM.
#
# Implements the kustomize-based installation approach documented at:
# https://github.com/kubevirt/hyperconverged-cluster-operator#installing-hco-using-kustomize-openshift-olm-only
#
# All images are 100% upstream (quay.io/kubevirt). No Red Hat IIB / internal images are used.
#
# Usage:
#   ./install_hco.sh
#
# Environment variables (all optional — defaults shown):
#
#   HCO_CHANNEL       OLM subscription channel.
#                     (default: candidate-v1.18)
#
#   HCO_INDEX_IMAGE   CatalogSource grpc index image. Use:
#                       Unstable (latest from main): quay.io/kubevirt/hyperconverged-cluster-index:1.18.0-unstable
#                       Stable tagged release:       quay.io/kubevirt/hyperconverged-cluster-index:1.18.0
#                     (default: quay.io/kubevirt/hyperconverged-cluster-index:1.18.0-unstable)
#
#   HCO_VERSION       Pin a specific startingCSV version, e.g. "1.18.0" or "1.18.0-202604211450".
#                     Leave empty (default) to use the channel head (recommended for unstable index).
#                     (default: "" — no startingCSV pinning)
#
#   HCO_GIT_REF       Git ref to download kustomize manifests from.
#                     Use "main" for latest or "v1.18.0" for a tagged release.
#                     (default: main)
#
#   HCO_NAMESPACE     Namespace to install HCO into.
#                     (default: kubevirt-hyperconverged)
#
#   KVM_EMULATION     true = patch HCO CR with KVM software emulation (for nodes without HW virt).
#                     (default: false)
#
#   TIMEOUT           Timeout for oc wait readiness checks, e.g. "15m" or "30m".
#                     (default: 15m)
#
#   OC_TOOL           oc or kubectl binary.
#                     (default: oc)
#
# Examples:
#   # Install latest unstable build from main branch (no version pinning):
#   ./install_hco.sh
#
#   # Install a specific stable tagged release:
#   HCO_CHANNEL=stable-v1.18 \
#   HCO_INDEX_IMAGE=quay.io/kubevirt/hyperconverged-cluster-index:1.18.0 \
#   HCO_GIT_REF=v1.18.0 \
#   ./install_hco.sh
#
#   # Pin to the exact CSV shown by: oc get packagemanifest community-kubevirt-hyperconverged
#   HCO_VERSION=1.18.0-202604211450 ./install_hco.sh
#
#   # Enable KVM software emulation on bare-metal without hardware virt:
#   KVM_EMULATION=true ./install_hco.sh
#
#   # Extend timeout for slow clusters:
#   TIMEOUT=30m ./install_hco.sh

set -euo pipefail

# ---------------------------------------------------------------------------
# Defaults
# ---------------------------------------------------------------------------
HCO_CHANNEL="${HCO_CHANNEL:-candidate-v1.18}"
HCO_INDEX_IMAGE="${HCO_INDEX_IMAGE:-quay.io/kubevirt/hyperconverged-cluster-index:1.18.0-unstable}"
HCO_VERSION="${HCO_VERSION:-}"        # empty = no startingCSV pin; use channel head
HCO_GIT_REF="${HCO_GIT_REF:-main}"
HCO_NAMESPACE="${HCO_NAMESPACE:-kubevirt-hyperconverged}"
KVM_EMULATION="${KVM_EMULATION:-false}"
TIMEOUT="${TIMEOUT:-15m}"
OC_TOOL="${OC_TOOL:-oc}"

CATALOGSOURCE_NAME="kubevirt-hyperconverged"
CATALOGSOURCE_NS="openshift-marketplace"
SUBSCRIPTION_NAME="hco-operatorhub"
PACKAGE_NAME="community-kubevirt-hyperconverged"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
log()  { echo "[INFO]  $(date +%H:%M:%S)  $*"; }
warn() { echo "[WARN]  $(date +%H:%M:%S)  $*"; }
err()  { echo "[ERROR] $(date +%H:%M:%S)  $*" >&2; }

WORK_DIR=""
cleanup() {
    if [[ -n "${WORK_DIR}" && -d "${WORK_DIR}" ]]; then
        log "Cleaning up temp directory: ${WORK_DIR}"
        rm -rf "${WORK_DIR}"
    fi
}
trap cleanup EXIT

require_tool() {
    if ! command -v "$1" &>/dev/null; then
        err "Required tool '$1' not found in PATH."
        exit 1
    fi
}

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------
preflight() {
    log "Running pre-flight checks..."
    require_tool "${OC_TOOL}"
    require_tool curl
    require_tool tar

    if ! ${OC_TOOL} whoami &>/dev/null; then
        err "Not logged in to a cluster. Run 'oc login' first."
        exit 1
    fi
    log "Logged in as: $(${OC_TOOL} whoami)"
    log "Cluster:      $(${OC_TOOL} whoami --show-server)"

    if ${OC_TOOL} get namespace "${HCO_NAMESPACE}" &>/dev/null; then
        warn "Namespace '${HCO_NAMESPACE}' already exists."
        if ${OC_TOOL} get hyperconverged kubevirt-hyperconverged \
                -n "${HCO_NAMESPACE}" &>/dev/null 2>&1; then
            err "HyperConverged CR already exists. Run uninstall_hco.sh first."
            exit 1
        fi
        # Clean up any leftover OLM resources from a previous partial install
        # (e.g. a subscription with a stale startingCSV that would block resolution)
        log "Cleaning up any leftover subscriptions from previous install attempt..."
        ${OC_TOOL} delete subscription "${SUBSCRIPTION_NAME}" \
            -n "${HCO_NAMESPACE}" --ignore-not-found 2>/dev/null || true
        ${OC_TOOL} delete clusterserviceversions \
            -n "${HCO_NAMESPACE}" --all --ignore-not-found 2>/dev/null || true
    fi
}

# ---------------------------------------------------------------------------
# Download the kustomize directory from the HCO GitHub repo
#
# GitHub tarball structure:
#   kubevirt-hyperconverged-cluster-operator-<sha>/deploy/kustomize/...
#
# With --strip-components=1 and cd into WORK_DIR, files land at:
#   ${WORK_DIR}/deploy/kustomize/...
# ---------------------------------------------------------------------------
download_kustomize() {
    WORK_DIR="$(mktemp -d -t hco-kustomize-XXXXXX)"
    local tarball_url="https://api.github.com/repos/kubevirt/hyperconverged-cluster-operator/tarball/${HCO_GIT_REF}"

    log "Downloading HCO kustomize manifests (git ref: '${HCO_GIT_REF}')..."
    (
        cd "${WORK_DIR}"
        curl -fsSL "${tarball_url}" \
            | tar --strip-components=1 -xzf - \
                  --wildcards \
                  "kubevirt-hyperconverged-cluster-operator-*/deploy/kustomize"
    )

    KUSTOMIZE_DIR="${WORK_DIR}/deploy/kustomize"
    if [[ ! -f "${KUSTOMIZE_DIR}/deploy_kustomize.sh" ]]; then
        err "deploy_kustomize.sh not found after extraction."
        err "Expected: ${KUSTOMIZE_DIR}/deploy_kustomize.sh"
        exit 1
    fi
    log "Kustomize dir ready: ${KUSTOMIZE_DIR}"
}

# ---------------------------------------------------------------------------
# Step 1: Create the CatalogSource pointing to the upstream index image
# ---------------------------------------------------------------------------
apply_catalogsource() {
    log "Creating CatalogSource '${CATALOGSOURCE_NAME}' → ${HCO_INDEX_IMAGE} ..."

    ${OC_TOOL} apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: ${CATALOGSOURCE_NAME}
  namespace: ${CATALOGSOURCE_NS}
spec:
  sourceType: grpc
  image: "${HCO_INDEX_IMAGE}"
  displayName: KubeVirt HyperConverged (upstream)
  publisher: KubeVirt Project
  updateStrategy:
    registryPoll:
      interval: 8h
EOF

    log "Waiting for CatalogSource to become READY (timeout: 10m)..."
    local deadline=$(( $(date +%s) + 600 ))
    while true; do
        local state
        state=$(${OC_TOOL} get catalogsource "${CATALOGSOURCE_NAME}" \
            -n "${CATALOGSOURCE_NS}" \
            -o jsonpath='{.status.connectionState.lastObservedState}' 2>/dev/null || true)
        if [[ "${state}" == "READY" ]]; then
            log "CatalogSource is READY."
            break
        fi
        if [[ $(date +%s) -gt ${deadline} ]]; then
            err "CatalogSource did not become READY within 10m."
            ${OC_TOOL} describe catalogsource "${CATALOGSOURCE_NAME}" \
                -n "${CATALOGSOURCE_NS}" || true
            exit 1
        fi
        log "  CatalogSource state: '${state:-<pending>}' — retrying in 10s..."
        sleep 10
    done
}

# ---------------------------------------------------------------------------
# Step 2: Discover the actual CSV name in the channel (informational).
#         Uses --selector=catalog= to target our specific CatalogSource.
#         Critical for unstable builds that use date-stamped version suffixes.
# ---------------------------------------------------------------------------
discover_csv() {
    log "Discovering current CSV in channel '${HCO_CHANNEL}' of '${PACKAGE_NAME}'..."
    local retries=0 max_retries=18  # 18 × 10s = 3 minutes
    local csv=""
    while [[ -z "${csv}" ]]; do
        if (( retries == max_retries )); then
            warn "Timed out waiting for PackageManifest. Proceeding with channel head."
            DISCOVERED_CSV=""
            return 0
        fi
        sleep 10
        # Use --selector=catalog to target our specific CatalogSource
        csv=$(${OC_TOOL} get packagemanifest "${PACKAGE_NAME}" \
            --selector="catalog=${CATALOGSOURCE_NAME}" \
            -o jsonpath="{.status.channels[?(@.name==\"${HCO_CHANNEL}\")].currentCSV}" \
            2>/dev/null || true)
        # Fall back to any catalog if our custom one hasn't propagated yet
        if [[ -z "${csv}" ]]; then
            csv=$(${OC_TOOL} get packagemanifest "${PACKAGE_NAME}" \
                -o jsonpath="{.status.channels[?(@.name==\"${HCO_CHANNEL}\")].currentCSV}" \
                2>/dev/null || true)
        fi
        (( retries += 1 ))
    done
    DISCOVERED_CSV="${csv}"
    log "Discovered CSV: ${DISCOVERED_CSV}"
}

# ---------------------------------------------------------------------------
# Step 3: Create Namespace, OperatorGroup, Subscription
# ---------------------------------------------------------------------------
apply_olm_resources() {
    log "Creating Namespace '${HCO_NAMESPACE}'..."
    ${OC_TOOL} apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${HCO_NAMESPACE}
  labels:
    openshift.io/cluster-monitoring: "true"
EOF

    log "Creating OperatorGroup..."
    ${OC_TOOL} apply -f - <<EOF
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: kubevirt-hyperconverged-group
  namespace: ${HCO_NAMESPACE}
spec: {}
EOF

    # Build the Subscription YAML — only set startingCSV if explicitly requested
    local starting_csv_line=""
    if [[ -n "${HCO_VERSION}" ]]; then
        starting_csv_line="  startingCSV: kubevirt-hyperconverged-operator.v${HCO_VERSION}"
        log "Subscription: pinning startingCSV to kubevirt-hyperconverged-operator.v${HCO_VERSION}"
    else
        log "Subscription: using channel head (${DISCOVERED_CSV})"
    fi

    log "Creating Subscription (channel: ${HCO_CHANNEL})..."
    ${OC_TOOL} apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: ${SUBSCRIPTION_NAME}
  namespace: ${HCO_NAMESPACE}
spec:
  source: ${CATALOGSOURCE_NAME}
  sourceNamespace: ${CATALOGSOURCE_NS}
  name: ${PACKAGE_NAME}
  channel: "${HCO_CHANNEL}"
  installPlanApproval: Automatic
${starting_csv_line}
EOF
}

# ---------------------------------------------------------------------------
# Step 4: Wait for the InstallPlan to be created and complete
# ---------------------------------------------------------------------------
wait_for_installplan() {
    log "Waiting for InstallPlan to be created (up to 5m)..."
    local install_plan="" retries=0 max_retries=20  # 20 × 15s = 5 minutes
    while [[ -z "${install_plan}" ]]; do
        if (( retries == max_retries )); then
            err "Timed out waiting for InstallPlan."
            ${OC_TOOL} get subscription "${SUBSCRIPTION_NAME}" \
                -n "${HCO_NAMESPACE}" -o yaml || true
            exit 1
        fi
        sleep 15
        install_plan=$(${OC_TOOL} get subscription "${SUBSCRIPTION_NAME}" \
            -n "${HCO_NAMESPACE}" \
            -o jsonpath='{.status.installplan.name}' 2>/dev/null || true)
        (( retries += 1 ))
    done
    log "InstallPlan: ${install_plan}"

    log "Waiting for InstallPlan ${install_plan} to complete (timeout: 5m)..."
    ${OC_TOOL} wait installplan "${install_plan}" \
        -n "${HCO_NAMESPACE}" \
        --for=condition=Installed \
        --timeout=5m
    log "InstallPlan complete."
}

# ---------------------------------------------------------------------------
# Step 5: Wait for the HCO operator deployment to be Available
# ---------------------------------------------------------------------------
wait_for_hco_operator() {
    log "Waiting for HCO operator deployment to be Available (timeout: 5m)..."
    ${OC_TOOL} wait deployments \
        --selector="operators.coreos.com/${PACKAGE_NAME}.${HCO_NAMESPACE}" \
        -n "${HCO_NAMESPACE}" \
        --for=condition=Available \
        --timeout=5m
    log "HCO operator deployment is Available."
}

# ---------------------------------------------------------------------------
# Step 6: Create the HyperConverged CR
#
# NOTE on featureGates: HCO v1.18 nightly builds (e.g. v1.18.0-202604*) have a
# CRD/webhook mismatch — the CRD schema declares featureGates as []Object{name,state}
# but the running webhook Go struct still expects the old map/struct format.  Any
# featureGates entry (even an empty list) causes the webhook to reject the CR with
# "failed to parse the HyperConverged".  Omit featureGates entirely; the two gates we
# care about (decentralizedLiveMigration, videoConfig) are beta-phase and on by default.
# incrementalBackup (alpha) can be re-added once the nightly build stabilises.
#
# NOTE on KVM emulation: spec.virtualization has no developerConfiguration field.
# useEmulation is set on the KubeVirt CR (spec.configuration.developerConfiguration)
# via a jsonpatch annotation on the HyperConverged CR after initial creation.
#
# API structure for v1.18:
#   - virtualization settings live under spec.virtualization.*
#   - cert config lives under spec.security.certConfig
#   - workloadSources.enableCommonBootImageImport replaces spec.enableCommonBootImageImport
# ---------------------------------------------------------------------------
apply_hco_cr() {
    log "Creating HyperConverged CR..."

    # KVM emulation annotation: HCO forwards this jsonpatch to the KubeVirt CR
    local kvm_annotation=""
    if [[ "${KVM_EMULATION}" == "true" ]]; then
        log "  KVM_EMULATION=true: will patch KubeVirt CR for software emulation"
        kvm_annotation='  annotations:
    kubevirt.kubevirt.io/jsonpatch: >
      [{"op":"add","path":"/spec/configuration/developerConfiguration","value":{"useEmulation":true}}]'
    fi

    ${OC_TOOL} apply -f - <<EOF
apiVersion: hco.kubevirt.io/v1beta1
kind: HyperConverged
metadata:
  name: kubevirt-hyperconverged
  namespace: ${HCO_NAMESPACE}
${kvm_annotation}
spec:
  security:
    certConfig:
      ca:
        duration: 48h0m0s
        renewBefore: 24h0m0s
      server:
        duration: 24h0m0s
        renewBefore: 12h0m0s
  deployment:
    uninstallStrategy: BlockUninstallIfWorkloadsExist
  virtualization:
    evictionStrategy: None
    workloadUpdateStrategy:
      batchEvictionInterval: 1m0s
      batchEvictionSize: 10
      workloadUpdateMethods: []
  workloadSources:
    enableCommonBootImageImport: true
EOF
}

# ---------------------------------------------------------------------------
# Step 7: Wait for HyperConverged to become Available
# ---------------------------------------------------------------------------
wait_for_hco() {
    log "Waiting for HyperConverged CR to become Available (timeout: ${TIMEOUT})..."
    if ! ${OC_TOOL} wait hyperconverged kubevirt-hyperconverged \
            -n "${HCO_NAMESPACE}" \
            --for=condition=Available \
            --timeout="${TIMEOUT}"; then
        err "HyperConverged CR did not become Available within ${TIMEOUT}."
        ${OC_TOOL} describe hyperconverged kubevirt-hyperconverged \
            -n "${HCO_NAMESPACE}" || true
        exit 1
    fi
    log "HyperConverged is Available."
}

# ---------------------------------------------------------------------------
# Print final status summary
# ---------------------------------------------------------------------------
print_status() {
    log "============================================================"
    log " Installation Complete!"
    log "============================================================"

    echo ""
    echo "=== HyperConverged ==="
    ${OC_TOOL} get hyperconverged -n "${HCO_NAMESPACE}" 2>/dev/null || true

    echo ""
    echo "=== KubeVirt ==="
    ${OC_TOOL} get kubevirt -n "${HCO_NAMESPACE}" 2>/dev/null || true

    echo ""
    echo "=== CDI ==="
    ${OC_TOOL} get cdi -n "${HCO_NAMESPACE}" 2>/dev/null || true

    echo ""
    echo "=== Pods ==="
    ${OC_TOOL} get pods -n "${HCO_NAMESPACE}" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    log "============================================================"
    log " HCO / KubeVirt Upstream Install"
    log "============================================================"
    log "  HCO_CHANNEL       = ${HCO_CHANNEL}"
    log "  HCO_INDEX_IMAGE   = ${HCO_INDEX_IMAGE}"
    log "  HCO_VERSION       = ${HCO_VERSION:-<channel head>}"
    log "  HCO_GIT_REF       = ${HCO_GIT_REF}"
    log "  HCO_NAMESPACE     = ${HCO_NAMESPACE}"
    log "  KVM_EMULATION     = ${KVM_EMULATION}"
    log "  TIMEOUT           = ${TIMEOUT}"
    log "============================================================"

    preflight
    download_kustomize   # validates connectivity; WORK_DIR set for cleanup
    apply_catalogsource
    discover_csv
    apply_olm_resources
    wait_for_installplan
    wait_for_hco_operator
    apply_hco_cr
    wait_for_hco
    print_status
}

main "$@"

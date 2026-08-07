# Design proposal: ResourceQuota for the OADP namespace

## Abstract

Ship a default namespace-scoped `ResourceQuota` for the OADP install namespace (typically `openshift-adp`) so compliance checks that require a ResourceQuota pass on install, while allowing cluster admins to adjust hard limits without the operator overwriting their changes on reconcile or upgrade.

## Background

Compliance scans commonly flag pods running in namespaces without a `ResourceQuota` applied. OADP installs into a suggested namespace (`openshift-adp` via `operatorframework.io/suggested-namespace`) and also supports other namespaces (for example ACM’s `open-cluster-management-backup`).

Today OADP sets per-container resource requests/limits (operator Deployment, Velero/node-agent defaults, DPA `resourceAllocations`) but does **not** create a namespace-level `ResourceQuota`. There is no existing ResourceQuota manifest in `config/` or the OLM bundle.

A fixed quota is sensitive to cluster size because node-agent is a DaemonSet: pod count and aggregate CPU/memory scale with node count. Defaults must therefore be a generous ceiling, not a tight right-size.

## Goals

- Clear the compliance finding: the OADP namespace has a `ResourceQuota` applied after install / when a DPA is reconciled.
- Ship product defaults (not docs-only).
- Allow admins to adjust the quota (`oc edit` / `oc patch`) with changes preserved across operator reconciles and upgrades (create-if-absent).
- Apply in whatever namespace the DPA lives in (not hardcoded only to `openshift-adp`).

## Non Goals

- `LimitRange` (separate finding / follow-up if required).
- `ClusterResourceQuota`.
- DPA API fields to configure quota (admins edit the ResourceQuota object directly).
- Automatic quota scaling based on node count.
- Per-cloud or per-profile default matrices.
- Deleting or resetting admin-modified quotas.

## Decision summary

| Choice | Decision |
|--------|----------|
| Delivery | Ship defaults with the product; operator ensures object exists |
| Admin adjustability | Create-if-absent; never update `.spec` if the named quota exists |
| Default sizing | Generous mid-size ceiling (node-agent + Velero + operator + helpers + some backup/restore pods) |
| Hard resources | `pods`, `requests.cpu`, `requests.memory` only — **no** `limits.*` (see below) |
| Configuration surface | Direct edit of `ResourceQuota`; no new DPA fields |
| Object name | `oadp-resource-quota` |

## High-level design

### Shipped object

A single namespaced ResourceQuota:

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: oadp-resource-quota
  namespace: openshift-adp   # or DPA namespace
  labels:
    app.kubernetes.io/name: oadp-operator
    app.kubernetes.io/managed-by: oadp-operator
  annotations:
    oadp.openshift.io/quota-policy: create-if-absent
spec:
  hard:
    pods: "200"
    requests.cpu: "50"
    requests.memory: 64Gi
```

Defaults are a **ceiling** for a typical mid-size cluster. Large clusters or heavy concurrent data-mover workloads may need higher hard limits; small clusters may lower them.

Canonical default values live in operator code (or a single shared constant/source used by the operator). A matching YAML under `config/` documents the same content for review and examples.

### Comparison to established OADP pod resources

Default per-container resources today:

| Component | Requests | Limits |
|-----------|----------|--------|
| Velero | 500m / 128Mi | *none* |
| Node-agent (per pod) | 500m / 128Mi | *none* |
| Operator manager | 500m / 128Mi | 1 / 512Mi |
| VM file restore / kubevirt datamover | 10m / 64Mi | 500m / 128Mi |
| CLI download | 50m / 32Mi | 100m / 64Mi |

**Quota vs defaults:** The namespace hard limits are aggregate ceilings, not “barely above one Velero pod.”

- Steady-state (operator + Velero, no node-agent): ~1 CPU / ~256Mi requests → well under `requests.cpu: 50` / `requests.memory: 64Gi`.
- With node-agent (DaemonSet): each node adds 500m / 128Mi requests. Rough headroom before hitting the ceiling is on the order of ~90+ node-agent pods for `requests.cpu: 50`, or ~190+ nodes for `pods: 200` (plus operator/Velero/helpers).

**Why `limits.*` are omitted from the default quota:** Default Velero and node-agent pods set **requests only** (no limits). If a ResourceQuota tracks `limits.cpu` / `limits.memory`, admission can reject pods that omit limits (unless a LimitRange supplies defaults). Shipping `limits.*` would risk breaking the default Velero/node-agent stack. Admins who set container limits everywhere may add `limits.*` to the quota themselves via `oc edit`.

### Lifecycle (create-if-absent)

1. On DPA reconcile, look up `ResourceQuota/oadp-resource-quota` in the DPA namespace.
2. If **missing** → create with shipped defaults.
3. If **present** → do nothing (no patch of `.spec`).
4. Quota is **not** a DPA-owned child for deletion purposes; namespace deletion removes it. DPA teardown does not delete the quota.
5. If an admin **deletes** the quota, the next DPA reconcile recreates defaults.

### OLM / bundle interaction

Pure CSV-owned bundle objects are reconciled by OLM and can reset admin edits on upgrade. To honor create-if-absent:

- The **operator** is authoritative: it creates `oadp-resource-quota` if absent and never updates `.spec` when present.
- Keep a matching example under `config/` (and optional docs sample) for review; do **not** add the ResourceQuota as a CSV-owned bundle manifest, so OLM cannot clobber admin edits on upgrade.
- Practical day-1 timing: the quota appears when a DPA is reconciled (normal install path creates a DPA shortly after operator install). That is sufficient to clear the compliance finding for pods that run because of the DPA.

### RBAC

Extend manager permissions (kubebuilder markers → `make manifests` / bundle):

```text
//+kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch;create
```

No `update`, `patch`, or `delete` on `resourcequotas`, so the operator cannot overwrite or remove admin edits even if a bug attempts Update.

### Admin workflow

```bash
# Inspect
oc describe resourcequota oadp-resource-quota -n openshift-adp

# Adjust (survives reconcile/upgrade)
oc edit resourcequota oadp-resource-quota -n openshift-adp
# or
oc patch resourcequota oadp-resource-quota -n openshift-adp --type merge -p '{"spec":{"hard":{"pods":"400"}}}'
```

Document that node-agent is a DaemonSet and large clusters may need higher `pods` / `requests.cpu` / `requests.memory`. If pods are Pending due to quota, raise hard limits. Note that default Velero/node-agent have no container limits, so the shipped quota intentionally omits `limits.*`.

## Edge cases

| Case | Behavior |
|------|----------|
| Admin deletes the quota | Next DPA reconcile recreates defaults |
| Admin creates an additional ResourceQuota | Both apply; effective limit is the intersection (most restrictive). Operator only manages `oadp-resource-quota` |
| ACM / custom install namespace | Same ensure logic in the DPA’s namespace |
| Multiple DPAs in one namespace | Idempotent ensure; still one `oadp-resource-quota` |
| Quota too tight → pods Pending | Docs/support: raise hard limits; no auto-scale |
| No DPA yet | Quota is not ensured until a DPA is reconciled (expected shortly after install) |

## Testing

- **Unit:** missing → created with defaults; existing custom `.spec` → unchanged (operator does not patch).
- **E2E (light/optional):** after deploy + DPA, `ResourceQuota/oadp-resource-quota` exists in the test namespace.

## Documentation

Add a short admin note under `docs/config/` covering defaults, inspect/adjust commands, recreate-on-delete behavior, and node-agent / large-cluster sizing guidance.

## Alternatives considered

1. **Bundle-only OLM-owned ResourceQuota** — Simple, but upgrades reset admin edits; rejected for adjustability requirement.
2. **Docs/sample only** — No upgrade conflict, but does not ship an applied quota; rejected.
3. **Second “override” ResourceQuota** — Cannot loosen a tight default (intersection); rejected as the primary adjust mechanism.
4. **DPA API for quota** — Extra API surface; admins can already edit ResourceQuota; deferred/out of scope.
5. **Include `limits.cpu` / `limits.memory` in default hard list** — Rejected for v1 because default Velero/node-agent pods lack container limits and would risk admission failure; admins can add these fields manually if their workloads all set limits.

## Implementation sketch (for planning)

1. Add default constants + `ensureResourceQuota` helper (get → create if not found).
2. Call ensure from DPA reconcile in the DPA namespace.
3. Add kubebuilder RBAC; regenerate manifests/bundle.
4. Add `config/` example YAML matching defaults (not CSV-owned in the bundle).
5. Unit tests for create-if-absent.
6. Admin docs under `docs/config/`.

## Open questions

None for v1 of this design. Default hard-limit numbers may be tuned during implementation review if QE/support have preferred ceilings.

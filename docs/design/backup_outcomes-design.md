# Backup outcomes design

_First-principles design for **per-item backup outcomes**: what happened to each resource in a single Velero backup run._

**Upstream:** [velero-io/velero#10503](https://github.com/velero-io/velero/issues/10503)

Related: [backup resource rollup](backup_resource_rollup-design.md) (downstream grouping), [vm backup status](vm_backup_status-design.md) (VM case study).

## Problem

After a Velero backup finishes — especially `PartiallyFailed` — operators need to know:

**For this backup, which resources failed, warned, or were skipped, and why?**

Today Velero only answers at job level:

```yaml
status:
  phase: PartiallyFailed
  errors: 2
```

There is no single, machine-readable **outcome per resource** for one backup.

## What exists today

**Machine-readable** = explicit `{GVR, namespace, name, outcome, message}` — not regex on `results.gz`.

Before adding a new file or log line, map what each backup artifact already records:

### In-cluster: `Backup.status`

| Field | Per-item? | Use |
|-------|-----------|-----|
| `phase`, `failureReason` | Job | Whole backup failed |
| `errors`, `warnings` | Count only | Points to BSL logs/results |
| `validationErrors` | Pre-run strings | Not backup outcomes |
| `hookStatus` | Count only | No per-hook detail |
| `backupItemOperations*` | Counts | Points to `itemoperations` |

### Object storage: `backups/<name>/`

| File | Per-item? | Outcome signal | Limitation |
|------|-----------|----------------|------------|
| `resource-list.json.gz` | Yes (inventory) | Denominator only | No pass/fail |
| `results.gz` | Errors only | Sync fail/warn strings | Unstructured; `name:` ambiguous; async gaps ([#9377](https://github.com/vmware-tanzu/velero/issues/9377)) |
| `volumeinfo.json.gz` | PVC | Structured succeeded/failed | PVC-only; can be stale |
| `itemoperations.json.gz` | Async ops | Structured async status | Partial coverage; may disagree with `results` |
| `<backup>.tar.gz` | Yes (manifests) | Content only | No outcome; large |
| `velero-backup.json` | Job | Status snapshot | May predate async finalize |
| `logs.gz` | No | Debug | Not structured |
| `volumesnapshots.json.gz`, `podvolumebackups.json.gz` | Volume path | Snapshot/PVB metadata | Not generic item outcomes |

### Merge vs new capture

| Approach | When |
|----------|------|
| **Merge at finalize** | Data already exists in `results` + `volumeinfo` + `itemoperations` + `resource-list` — unify after async completes ([#9377](https://github.com/vmware-tanzu/velero/issues/9377)) |
| **Structure at write time** | Velero records a failure — store GVR/ns/name as fields, not only inside a string |
| **New capture** | No file records it today — e.g. per-hook outcome, explicit `skipped` with reason |

**#10503 is mostly merge + structure**, not more `logs.gz` text. Success = in resource-list, absent from failure sets, volumeinfo OK for PVCs.

### Next step

Walk one real `PartiallyFailed` backup: for each failed object, note which file(s) hold truth. That matrix drives the finalize merge rules and the short list of gaps that need new fields at backup time.


## Scope

**In scope:** one backup run → one outcome view.

**Out of scope (for this document):**

- Restore outcomes
- Rollup to logical units (VM, StatefulSet, Helm app) — see [backup resource rollup](backup_resource_rollup-design.md)
- Cross-backup history (“what failed across last week’s dailies”)
- Changing Velero backup phase semantics or retry behavior

## Model

### One backup, one snapshot

Each `velero backup create` (or schedule tick) produces a **new** `Backup` CR and a new object-storage prefix. Outcomes describe **that run only**. They are written once at finalize and are not updated by later backups.

### Per-item outcome

An outcome is the result for one Kubernetes object that was part of this backup:

```
{ group, resource, version, namespace, name, outcome, message? }
```

`outcome` is one of:

| Value | Meaning |
|-------|---------|
| `succeeded` | Backed up without error |
| `failed` | Error prevented successful backup of this item |
| `warning` | Backed up with a non-fatal issue |
| `skipped` | Explicitly not backed up (selector, hook, policy) — semantics TBD |

A single object may accumulate messages from multiple steps (e.g. metadata OK, volume async fail). The outcome is `failed` if any step failed; messages should be deduplicated or listed.

### Sparse storage, implicit success

At scale (thousands of objects, few failures), persist **only non-success items** plus a **summary**:

```json
{
  "backup": "daily-2026-09-08",
  "summary": {
    "totalItems": 8420,
    "succeeded": 8408,
    "failed": 10,
    "warnings": 2,
    "skipped": 0
  },
  "items": [
    {
      "group": "kubevirt.io",
      "resource": "virtualmachines",
      "version": "v1",
      "namespace": "app-ns",
      "name": "vm-broken",
      "outcome": "failed",
      "message": "persistentvolumeclaims \"vm-broken-disk\" not found"
    }
  ]
}
```

**Rule:** if an object is in the backup’s resource list (and `volumeinfo` is OK for PVCs) and does not appear in `items` → treat as `succeeded`.

Do not store one row per succeeded object. Do not put the full list on the `Backup` CR.

### Where outcomes come from

Finalize merge (after async work):

1. `resource-list` — what was in scope
2. `results` — sync errors (parse or prefer structured fields if written at failure time)
3. `volumeinfo` — PVC results
4. `itemoperations` — async failures (wins over stale `results`/`volumeinfo` when they disagree)

Output = sparse non-success rows + summary. Do not duplicate `resource-list` with one success row per object.

### Error string parsing (interim)

Two common shapes in `results.gz`:

- `error executing custom action (groupResource=..., namespace=..., name=...): ...` → structured fields present
- `name: /X message: /...` → resolve kind by cross-reference to resource list, not by assuming `name`’s type

Unrecognized patterns → keep as unstructured message; never drop.

## Open questions

1. **Skipped vs succeeded** — when does Velero omit an item vs mark it skipped with a reason?
2. **PVC vs parent object** — one outcome row per API object, or collapse volume failure onto a parent? (Recommendation: per API object; rollup is downstream.)
3. **Warnings** — separate outcome or modifier on `succeeded`?
4. **Artifact vs API only** — persist `<backup>-item-outcomes.json.gz`, or expose only via `pkg/` and let callers persist if needed?
5. **Summary on `Backup.status`** — counts only (no per-item rows)?

## Consumers

| Consumer | Uses outcomes for |
|----------|-------------------|
| `velero backup describe` | Human-readable failure list |
| oadp-cli `backup resource-status` | Input to owner/label rollup |
| Portals / automation | Selective retry, ticketing, reports |

Consumers that need “did VM X succeed?” combine sparse outcomes + resource list + optional owner graph — that is rollup, not part of this layer.

## Success criteria

- [ ] For a named backup, list every **failed** and **warned** item with explicit GVR, namespace, name, and message
- [ ] Async failures included in the same view as sync failures
- [ ] Size bounded by failure count, not total object count
- [ ] Stable enough for integrators to depend on without parsing `results.gz` ad hoc

## Non-goals

- Per-VM or per-app status (downstream)
- Reason-code taxonomy (`MissingPVC`, etc.) — downstream
- Restore outcomes
- Mutating or amending outcomes after finalize

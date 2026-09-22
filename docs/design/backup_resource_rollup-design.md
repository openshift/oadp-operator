# Generalized backup resource rollup design

_A generalization of `docs/design/vm_backup_status-design.md` (tracking [OADP-8697](https://redhat.atlassian.net/browse/OADP-8697)), after finding the underlying mechanism is not KubeVirt-specific._

## Abstract
Velero reports backup outcomes per raw Kubernetes object, as a flat list plus an unstructured error string, with no concept of the logical or composite unit (a VM, a database cluster, a Helm release) that object belongs to.
This proposal describes a general mechanism, structured error parsing plus `ownerReference` graph walking, that rolls per-item results up to whatever logical unit a user cares about, so that a namespace with thousands of such units stays legible when a backup partially fails.
VM support (the original motivation, see `vm_backup_status-design.md`) becomes one built-in convenience on top of this generic engine rather than bespoke logic.

## Background
`docs/design/vm_backup_status-design.md` set out to solve per-VM backup visibility specifically, and validated the problem against a real cluster: a `PartiallyFailed` backup's only detail is an unstructured string (`<backup>-results.gz`) where the same `name:` field holds a VM name in one failure and a PVC name in another, with no `kind` field to tell them apart.

While designing the correlation logic for that document, we checked whether the VM-to-PVC link required KubeVirt-specific knowledge (parsing `VirtualMachine.spec.template.spec.volumes[]`) or whether it was already expressed through standard Kubernetes mechanisms.
It's the latter.
Inspecting the actual backed-up manifests from our test namespace:

```json
// DataVolume vm-good-1-disk
"ownerReferences": [{"kind": "VirtualMachine", "name": "vm-good-1", "controller": true}]
```
```json
// PVC vm-good-1-disk
"ownerReferences": [{"kind": "DataVolume", "name": "vm-good-1-disk", "controller": true}]
```

`VirtualMachine → DataVolume → PVC` is a standard `controller: true` ownership chain, the same mechanism used by `Deployment → ReplicaSet → Pod`, `StatefulSet → Pod`, `Job → Pod`, or any CRD written with controller-runtime's `SetControllerReference` (which covers most operators, including most database operators).
Walking that chain to find "which logical unit does this failed item belong to" requires no VM-specific code at all.

Separately, re-examining our two reproduced failures, both were already attributed by Velero directly to the VM item itself:
```
error executing custom action (groupResource=virtualmachines.kubevirt.io, namespace=oadp-8697-multi-vm,
  name=vm-broken-orphan-pvc): rpc error: code = Unknown desc = persistentvolumeclaims "..." not found
```
This string already contains a structured `groupResource=`, `namespace=`, `name=` triple, it is simply serialized as free text rather than as fields.
The only failure mode from our testing that actually required walking up from a child item was the original single-VM repro, where the error landed on the PVC item (`PVC ... has no volume backing this claim`), and that case is exactly what `ownerReferences` walking solves generically.

So the two things that make the VM case work are both workload-agnostic:
1. Parse Velero's own error/warning text into structured `{groupResource, namespace, name, message}` tuples.
2. Walk `ownerReferences` from a failed item up to whatever unit the user considers "the thing they care about."

## Goals
- Provide one general mechanism to roll up raw per-item backup results to a logical/composite unit, for any workload that uses standard Kubernetes ownership or labeling, not only VMs.
- Make VM support (and any other grouping) a thin configuration of this engine, not a separate implementation, so future groupings (StatefulSet, a specific operator's CR, Helm releases) cost close to nothing to add.
- Preserve the failures-first, summary-line-first CLI presentation already validated for the VM case, now parameterized by grouping key instead of hardcoded to VirtualMachine.

## Non Goals
- Full semantic modeling of every possible spec-level reference (e.g. a Pod mounting a ConfigMap by name, which has no `ownerReference`) as a first-class grouping mechanism in v1. Our evidence shows Velero already attributes these failures to the referencing item directly, so structured error parsing alone covers the cases that motivated OADP-8697; a "well-known reference path" table is a possible v2 enrichment, not a launch requirement.
- Replacing Velero's own resource list or error reporting. This stays a derived, read-only enrichment view.
- Cross-namespace or cross-backup grouping in v1. Scope is one backup's included namespace(s) at a time.

## High-Level Design
A small generic engine with two building blocks:

1. **Error/warning structurer**: parses Velero's `results.gz` (and any similarly formatted plugin errors) into `{groupResource, namespace, name, message}` tuples wherever the format allows, and falls back to an `Unstructured` bucket (never silently dropped) otherwise.
2. **Ownership graph walker**: builds an item graph for the backup from the resource list plus each item's `ownerReferences` (read live from cluster when the source namespace still exists, or from the backup tarball otherwise), and walks `controller: true` references upward from any failed/warned item to a target.

A **grouping strategy** decides what "target" means:
- `owner:<Kind>` — stop walking when a `Kind` match is reached (e.g. `owner:VirtualMachine`, `owner:StatefulSet`).
- `owner` (no kind) — walk all the way to the topmost object with no further owner.
- `label:<key>` — skip the graph walk, group flatly by a label value (for Helm-style apps that share `app.kubernetes.io/instance` without using `ownerReferences`).

VM support becomes `--group-by=owner:VirtualMachine` shipped as a default preset; nothing about the engine itself knows what a VM is.

## Detailed Design

### Command shape
```
kubectl oadp backup resource-status <backup> --group-by=owner:VirtualMachine
kubectl oadp backup resource-status <backup> --group-by=owner:StatefulSet
kubectl oadp backup resource-status <backup> --group-by=owner              # walk to topmost owner
kubectl oadp backup resource-status <backup> --group-by=label:app.kubernetes.io/instance
```
Shared flags across all grouping strategies (same as the VM-specific design): `--status=succeeded|failed|skipped`, `--group-by-reason`, `-o table|wide|json|yaml`, with `table` defaulting to a failures-first view plus a one-line summary count, so the output size tracks the failure count, not the namespace's total object count.

### Error structuring rules
Two known Velero error shapes observed in practice:
- `error executing custom action (groupResource=X, namespace=Y, name=Z): <message>` → parses directly into `{groupResource: X, namespace: Y, name: Z, message}`.
- `name: /Z message: /<message>` (the older item-backup-error shape, seen when a PVC's own backup step fails rather than a plugin custom action) → `Z` is the failing item's name but its `kind` is not present in the string; the structurer resolves `kind` by cross-referencing `Z` against the backup's resource list (which kind's name list contains `Z`) rather than guessing from field order, which is what made naive `name:` grepping unreliable in the first place.
- Anything that matches neither pattern is kept as `{message, groupResource: Unstructured}` and always surfaced, never hidden, since silently dropping unrecognized errors would recreate the exact visibility gap this proposal fixes.

### Ownership graph walk
Given the resource list for the backup, fetch each object's `metadata.ownerReferences` (live cluster lookup preferred; tarball fallback for backups whose source namespace no longer exists, exactly as in the VM-specific design).
For a structured error tuple `{groupResource, namespace, name}`, look up that object, and if it has a `controller: true` owner reference, follow it, repeating until the grouping strategy's stop condition is met or no further owner exists.
This is a small, generic, iterative graph walk, bounded by the depth of real ownership chains (rarely more than 2-3 hops in practice: `PVC → DataVolume → VirtualMachine`, `Pod → ReplicaSet → Deployment`).

### Rollup semantics
A group (e.g. one VM, one StatefulSet, one label value) is:
- `Failed` if any item under it produced a structured or unstructured error, carrying the (deduplicated) set of reasons up.
- `Succeeded` if every item under it backed up cleanly.
- `Skipped` if explicitly excluded (selector/filter), a case that needs further definition, see Open Issues.

### Relationship to the VM-specific design
`vm_backup_status-design.md`'s Phase 1 CLI and Phase 2 CRD both become thin instances of this engine:
- Its correlation algorithm (steps 1-4) is superseded by the generic structurer + graph walker described here; notably, the VM-specific design's step 2 ("read `spec.template.spec.volumes[]`") turns out to be unnecessary for the failure modes we tested, since ownership chains already carry that information.
- Its reason-code taxonomy (`MissingPVC`, `MissingDataSource`, etc.) still applies, now as an optional classification layer applied to the generic `{groupResource, namespace, name, message}` tuples, rather than being VM-specific parsing logic.
- Its Phase 2 CRD sketch (`VirtualMachineBackupStatus`) would generalize to something like `BackupResourceRollup` with a `spec.groupBy` field, if a persisted, in-cluster version is pursued later.

## Alternatives Considered

**Ship the VM-specific version first, generalize afterward.**
This was the original plan. Deprioritized once we found the generic mechanism (structured errors + ownerRef walk) is not meaningfully more work than the VM-specific version would have been, and is simpler, since it avoids writing and maintaining bespoke `VirtualMachine.spec` parsing at all.

**Structured error parsing without the ownership graph walk.**
Insufficient on its own: it correctly labels which raw item failed, but doesn't answer "which VM does this failed PVC belong to" for the failure mode where the error lands on a child item rather than the top-level object.

**Ownership graph walk without structured error parsing.**
Insufficient on its own: still need the message text for the human-readable "why," and the orphan-reference failure mode (referencing an object that was never created, so there's nothing to walk from) has no graph to walk at all, only structured error parsing catches it.

**A full table of well-known spec-level reference paths (Pod→ConfigMap, VM→PVC-by-name, etc.) as a core v1 requirement.**
Deferred. Higher effort and ongoing maintenance burden (one entry per Kind/field), and our evidence shows it is not needed for the failure modes that motivated OADP-8697, since Velero already attributes those to the referencing item directly. Worth revisiting as a v2 enrichment once real-world usage shows it's needed.

## Security Considerations
Read-only, same as the VM-specific design: no writes to Backups, VMs, or any other cluster resource.
Needs read RBAC on whatever resource kinds appear in a given backup (broader than the VM-specific design, since the grouping target is now user-chosen), plus BSL object storage read access for historical backups whose source namespace no longer exists.

## Compatibility
Works retroactively against any existing backup, since `ownerReferences` and Velero's error text are already captured and stored today; no changes to Velero, the kubevirt-velero-plugin, or any other plugin are required to ship v1.

## Implementation
1. Implement the error structurer and ownership graph walker as a standalone, reusable library (not VM-aware), with unit tests built from the fixtures already captured while validating the VM-specific design (resource-list, results, and tarball files from the 5-VM repro).
2. Implement `kubectl oadp backup resource-status` in `migtools/oadp-cli` on top of that library, with `owner:VirtualMachine` as the first shipped preset, directly superseding the VM-specific design's Phase 1 plan.
3. Add `owner:StatefulSet` and `label:app.kubernetes.io/instance` presets to validate the engine generalizes beyond the motivating VM case before calling v1 done.
4. Revisit `vm_backup_status-design.md`'s Phase 2 CRD idea, generalized to a `groupBy`-parameterized CRD, only if in-cluster persistence proves necessary after the CLI ships.

## Open Issues
- **Stop condition for `owner` grouping.** When walking to "the topmost owner" with no explicit Kind given, is the first Kind with no further owner always the right unit, or should some common Kinds (e.g. `ReplicaSet` between `Pod` and `Deployment`) be skipped by default since they're rarely what a user means by "the app"?
- **Combining `owner` and `label` grouping.** Some workloads are partially expressed via ownership and partially via shared labels (e.g. a Helm release where only some resources have ownerRefs). Need to decide whether groupings can be combined or whether users must pick one strategy per invocation.
- **Skip semantics**, carried over unresolved from the VM-specific design: Velero doesn't always explicitly mark an item "skipped," so we need a clear definition before that status value means something consistent.
- **Command naming.** `resource-status` vs. `rollup` vs. `summary` vs. `grouped-status`, needs a decision before implementation.

## References
- `docs/design/vm_backup_status-design.md` (the VM-specific case this generalizes, including the original cluster evidence and reproduction steps)
- [OADP-8697](https://redhat.atlassian.net/browse/OADP-8697)
- `docs/design/kubectl-oadp.md` (existing kubectl-oadp plugin design, where this command would live)
- [migtools/oadp-cli](https://github.com/migtools/oadp-cli)

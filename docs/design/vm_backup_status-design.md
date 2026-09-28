# Per-VM backup status visibility design

_Tracks [OADP-8697](https://redhat.atlassian.net/browse/OADP-8697): Expose per-VM backup status in PartiallyFailed namespace backups._

## Abstract
When a namespace-scoped OADP/Velero backup of OpenShift Virtualization VMs finishes `PartiallyFailed`, there is no way to tell which individual VMs succeeded and which failed without manually correlating item-level errors back to their owning VM.
This proposal makes that correlation visible, first through a `kubectl oadp` CLI command, and optionally later through an in-cluster status object, so the result stays legible even when a namespace has thousands of VMs.

## Background
Velero reports backup problems at the Kubernetes-resource level: PVC, DataVolume, VirtualMachine, VirtualMachineInstance, Pod.
Nothing rolls those errors up to the VirtualMachine that owns the affected disk.
Cohesity has reported this gap operating OADP-backed VM backups at scale, where a subset of VMs in large namespaces routinely have stale or broken configurations (missing DataVolume/PVC, orphaned VM/VMI, CSI mount issues, snapshot timeouts).

To ground this proposal in real behavior rather than speculation, we reproduced the problem locally on a live OpenShift Virtualization cluster (CNV 4.22, OADP built from `oadp-dev`).
Using the `cirros-test` DataVolume-clone pattern from `tests/e2e/sample-applications/virtual-machines/cirros-test/3-vms/`, we deployed 5 VMs in one namespace:

- `vm-good-1`, `vm-good-2`, `vm-good-3`: normal clones of a `cirros` DataSource. All backed up successfully via CSI snapshot + `velero-fs` data movement.
- `vm-broken-orphan-pvc`: disk volume references a PVC that was never created (stale/dangling reference).
- `vm-broken-baddatasource`: `dataVolumeTemplate` clones from a nonexistent `DataSource` (broken config, the DataVolume/PVC never materializes).

A real `velero.io/v1` Backup (`snapshotMoveData: true`, `kubevirt` + `csi` + `openshift` plugins) against that namespace produced `status.phase: PartiallyFailed`, `status.errors: 2`, with detail only available as:

```
name: /vm-broken-baddatasource message: /Error backing up item error: /error executing custom
  action (groupResource=virtualmachines.kubevirt.io, namespace=oadp-8697-multi-vm,
  name=vm-broken-baddatasource): rpc error: code = Unknown desc = persistentvolumeclaims
  "vm-broken-baddatasource-disk" not found
name: /vm-broken-orphan-pvc message: /Error backing up item error: /error executing custom
  action (groupResource=virtualmachines.kubevirt.io, namespace=oadp-8697-multi-vm,
  name=vm-broken-orphan-pvc): rpc error: code = Unknown desc = persistentvolumeclaims
  "vm-broken-orphan-pvc-disk-does-not-exist" not found
```

This is the `Backup.status.errors` / `<backup>-results.gz` structure Velero already writes, and it is not a JSON error list, it is a flattened string with ad hoc `key: /value` segments.
Critically, we found the `name:` field is not stable in what it names.
In this run it happened to be the VM name (the failure occurred in the VM's custom backup action).
In an earlier single-VM repro on the same cluster (`vmfr-test-backup-202608070835`, preserved in the backup storage from a prior session), the exact same field held a **PVC** name instead, because that failure happened one step later, during the PVC's own item backup:

```
name: /vmfr-test-vm-disk-1 message: /Error backing up item error: /PVC vmfr-test-ns/vmfr-test-vm-disk-1
  has no volume backing this claim
```

So a naive grep for `name: /` cannot reliably tell you whether you are looking at a VM or a PVC, let alone which VM owns a failed PVC.
At 5 VMs this is merely annoying.
At 1,000+ VMs with a mix of failure modes it is not something an operator, or a script, can reliably act on.

We also checked every other artifact Velero writes to object storage for this backup, to see what correlation data already exists:

| File | Scope | Contents |
|---|---|---|
| `<backup>-resource-list.json.gz` | per-Kind | flat `namespace/name` lists per GVR, no cross-references between kinds |
| `<backup>-results.gz` | backup-wide | unstructured error/warning strings (see above), no `kind` field |
| `<backup>-volumeinfo.json.gz` | per-PVC | structured `result: succeeded/failed`, but keyed by PVC name only, no VM reference |
| `<backup>-itemoperations.json.gz` | per-PVC datamover op | structured, PVC-scoped only |
| VM manifest inside `<backup>.tar.gz` | per-VM | `spec.template.spec.volumes[]`, the only place that links a VM to the PVC/DataVolume names it owns |

We also confirmed the kubevirt-velero-plugin already labels backed-up PVCs with `velero.kubevirt.io/pvc-uid` (used for VMFR's selective restore, see `docs/design/vm_file_restore-design.md`), but that label identifies the PVC, not the owning VM's name or namespace.
So today, the only reliable VM-to-disk join key is the VM's own backed-up manifest, and nothing pre-computes that join for you.

## Goals
- Make per-VM backup outcome (succeeded / failed / skipped) and failure reason visible without manual cross-referencing, in a form that stays legible when a namespace has thousands of VMs.
- Ship a first usable version quickly, without requiring new CRDs, webhooks, or changes to Velero or the kubevirt-velero-plugin release cycle.
- Leave a clear path to a more integrated, cluster-native status object if the CLI proves the approach is worth persisting.

## Non Goals
- Changing Velero's core backup orchestration or phase semantics.
- Automatic retry of failed VMs (separate RFE, per OADP-8697's acceptance criteria).
- Automatic cleanup or remediation of stale VM configurations.
- Per-VM status rollup for non-KubeVirt workloads.
- Replacing Velero's own error reporting; this is a derived, read-only enrichment view on top of it.

## High-Level Design
Two phases, deliberately decoupled so the first phase can ship without waiting on the second.

**Phase 1 (primary deliverable): `kubectl oadp backup vm-status <backup>`**
A new subcommand in the existing `migtools/oadp-cli` plugin (which already ships `backup create/describe/logs/delete` using Velero's client libraries, see `docs/design/kubectl-oadp.md`).
It reads the data sources listed above, resolves the VM-to-PVC/DataVolume join client-side, classifies failures into a small set of reason codes, and renders a table.
Default output is failures-first: a one-line summary count plus a table of only the non-succeeded VMs, so a 4,000-VM namespace with 12 failures renders as 12 rows, not 4,000.

**Phase 2 (optional follow-on): in-cluster status object**
If Phase 1 proves useful, an OADP-operator controller (this repo) watches Backup completion and performs the same correlation server-side once, persisting the result as a small CRD (`VirtualMachineBackupStatus`, following the exact pattern already established by `VirtualMachineBackupsDiscovery`, see `docs/design/vm_file_restore-design.md`).
This makes the result visible via `oc get`/`oc describe` without the CLI, computed once instead of on every query, and consumable by other automation (selective retry, selective data backup) mentioned in OADP-8697's business justification.

## Detailed Design

### Correlation algorithm
For a given backup, build the per-VM view in four steps:

1. **Enumerate VMs and PVCs in the backup.** From the live Backup's resource list (or `<backup>-resource-list.json.gz` for backups whose source namespace no longer exists), collect every `kubevirt.io/v1/VirtualMachine` and `v1/PersistentVolumeClaim` name.
2. **Extract each VM's expected disks.** For every VM, read `spec.template.spec.volumes[]` (from the live cluster if the VM still exists there, otherwise from the VM manifest inside `<backup>.tar.gz`) and collect `persistentVolumeClaim.claimName` and `dataVolume.name` references, plus `dataVolumeTemplates[].metadata.name` for cloned disks. This is the VM's declared disk set, and it is present even for VMs whose backup ultimately failed.
3. **Determine each disk's outcome.** Cross-reference the declared disk names against:
   - the PVC resource list (was it backed up at all?),
   - `<backup>-volumeinfo.json.gz` (`result: succeeded/failed/skipped` for CSI/datamover-backed disks),
   - `<backup>-results.gz` error strings, matched against the declared disk names and the VM name (not just `name:`, since we proved that field is ambiguous) to attribute an error and classify it.
4. **Roll up to VM status.** A VM is `Succeeded` if all its declared disks succeeded, `Failed` if any disk is missing or failed (carrying the first matched failure reason), `Skipped` if excluded by selector/filter. VMs with zero declared disks are `Succeeded` trivially (VM metadata backed up, nothing else expected).

### Failure reason classification
Raw error strings get mapped to a small, stable set of reason codes so that thousands of VMs collapse into a handful of buckets instead of thousands of free-text messages:

| Reason code | Matches |
|---|---|
| `MissingPVC` | `persistentvolumeclaims "X" not found`, `PVC ... has no volume backing this claim` |
| `MissingDataVolume` | `datavolumes.cdi.kubevirt.io "X" not found` |
| `MissingDataSource` | `datasources.cdi.kubevirt.io "X" not found` |
| `CSISnapshotTimeout` | timeout errors from `volumeinfo` / `itemoperations` |
| `DataUploadFailed` | `snapshotDataMovementInfo.Phase: Failed` in `volumeinfo` |
| `Unknown` | anything that doesn't match a known pattern (always shown verbatim, never silently dropped) |

This taxonomy is a starting point based on the failure modes we could reproduce locally; it should be revisited once we have more real Cohesity-style stale-config samples (see Open Issues).

### CLI shape and example output
```
$ kubectl oadp backup vm-status oadp-8697-multi-vm-backup
Backup: oadp-8697-multi-vm-backup   Phase: PartiallyFailed
VMs: 5 total   3 succeeded   2 failed   0 skipped

FAILED:
NAME                      NAMESPACE            REASON              DETAIL
vm-broken-baddatasource   oadp-8697-multi-vm   MissingDataSource   dataVolumeTemplate "vm-broken-baddatasource-disk" clones from DataSource "does-not-exist" (not found)
vm-broken-orphan-pvc      oadp-8697-multi-vm   MissingPVC          volume "rootdisk" references PVC "vm-broken-orphan-pvc-disk-does-not-exist" (not found)

Use --status=succeeded|failed|skipped or -o wide to see all VMs.
Use -o json / -o yaml for scripting.
```

At scale, `--group-by-reason` collapses the failure list further:
```
$ kubectl oadp backup vm-status ns-scale-backup --group-by-reason
Backup: ns-scale-backup   Phase: PartiallyFailed
VMs: 4000 total   3971 succeeded   29 failed   0 skipped

REASON               COUNT   EXAMPLE VM
MissingPVC             18    vm-4021
CSISnapshotTimeout      7    vm-1187
DataUploadFailed        4    vm-2755
```

Flags:
- `--status=succeeded|failed|skipped` (repeatable) filters rows.
- `--group-by-reason` clusters failed VMs by reason code with a count and one example, instead of one row per VM.
- `-o table` (default, failures-first), `-o wide` (every VM regardless of status), `-o json`/`-o yaml` (full structured output for scripting, this is the actual "grep optimization": once the data is structured, `jq`/`grep` against it is reliable in a way that scraping `results.gz` is not).

### Phase 2 CRD sketch
```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: VirtualMachineBackupStatus
metadata:
  name: oadp-8697-multi-vm-backup
  namespace: openshift-adp
spec:
  backupName: oadp-8697-multi-vm-backup
status:
  phase: Completed              # discovery/computation phase, distinct from the Backup's own phase
  summary:
    total: 5
    succeeded: 3
    failed: 2
    skipped: 0
  vms:
  - name: vm-broken-baddatasource
    namespace: oadp-8697-multi-vm
    result: Failed
    reason: MissingDataSource
    detail: 'dataVolumeTemplate "vm-broken-baddatasource-disk" clones from DataSource "does-not-exist" (not found)'
    disks:
    - name: rootdisk
      pvcName: vm-broken-baddatasource-disk
      result: Failed
  - name: vm-good-1
    namespace: oadp-8697-multi-vm
    result: Succeeded
    disks:
    - name: rootdisk
      pvcName: vm-good-1-disk
      result: Succeeded
```
A controller reconciling this CRD would watch Backups for completion (`Completed`/`PartiallyFailed`/`Failed`), run the same correlation algorithm as Phase 1, and write the result once. This is intentionally the same shape of design as `VirtualMachineBackupsDiscovery` (build candidate list, validate, incrementally update status), so it can reuse that controller's patterns directly.

### Where the code lives
- Phase 1: `migtools/oadp-cli`, alongside the existing `backup describe`/`backup logs` commands, using the same Velero client libraries.
- Phase 2 (if pursued): this repo, `api/v1alpha1/` for the new CRD type and `internal/controller/` for the reconciler, mirroring `virtualmachinebackupsdiscovery_controller.go`.

## Alternatives Considered

**Extend Velero's own Backup CRD/status upstream.**
Rejected as the primary approach: it is out of OADP's control, Velero is intentionally workload-agnostic and unlikely to accept KubeVirt-specific rollup logic in core, and the upstream release cycle is far slower than what this feature needs.

**Push the correlation entirely into the kubevirt-velero-plugin (label PVCs with the owning VM's name/namespace at backup time, extending the existing `velero.kubevirt.io/pvc-uid` labeling).**
This is a good complementary improvement: it would turn the VM-to-PVC join from "parse the VM manifest" into an O(1) label lookup, benefiting this feature and VMFR's existing discovery logic. But it lives in a separate repo with its own release cadence, and it does not retroactively help with backups already taken (like the ones we inspected in this session). Recommended as a fast-follow, not the initial fix.

**Document a grep/jq recipe against `results.gz` instead of building tooling.**
Rejected as the primary solution. We demonstrated directly that the same field (`name:`) can hold either a VM name or a PVC name depending on where in the pipeline a failure occurred, with no `kind` discriminator. A recipe built on that assumption will silently misattribute errors. A short-lived stopgap doc could still be written, but it should not be the answer to the acceptance criteria.

**Ship the CRD/controller (Phase 2) as the only deliverable, skip the CLI.**
Rejected as the starting point: it requires new API types, RBAC, and a bundle/CSV change before any user sees value, and it is slower to iterate on the reason-code taxonomy (see Open Issues) than a CLI command is. The CLI can ship, be used, and be refined immediately against real customer backups; the CRD is worth doing later once the correlation logic is validated.

## Security Considerations
Both phases are read-only with respect to cluster state; neither writes to or mutates Backups, VMs, PVCs, or DataVolumes.
Phase 1 needs the same RBAC a user already needs for `velero backup describe`/`logs` (get/list Backups, DataUploads, PVCs, VirtualMachines in the target namespace), plus read access to the Backup Storage Location's object storage credentials when a backup's source namespace no longer exists and the tool must fall back to downloading the tarball.
Phase 2 reuses the operator's own existing BSL access; the new CRD's status is read/list-only for regular users, and only the controller's service account gets write access to it.

## Compatibility
Phase 1 works against any existing or historical backup on this cluster without any cluster-side changes, we validated this directly against a real `PartiallyFailed` backup from a prior session as well as the fresh 5-VM repro; no version gating is needed beyond what the kubevirt-velero-plugin already emits today.
Phase 2 introduces a new optional CRD and controller, which should be gated the same way `kubevirt-datamover` and VM File Restore are, an opt-in component enabled via the DPA, not installed by default.

## Implementation
1. Implement the correlation algorithm and reason-code classifier as a small, independently testable Go package, using the artifacts captured during this session (`resource-list.json.gz`, `results.gz`, `volumeinfo.json.gz`, and the 5-VM tarball) as recorded test fixtures.
2. Wire it into `migtools/oadp-cli` as `kubectl oadp backup vm-status`.
3. Validate against a larger synthetic namespace (dozens to low-hundreds of VMs, mixing more failure modes: CSI snapshot timeout, DataUpload failure, orphaned VMI) to stress-test the failures-first / group-by-reason output before calling Phase 1 done.
4. Gather feedback (ideally against a real Cohesity-scale sample or a closer approximation) before committing to Phase 2.
5. If Phase 2 is warranted, implement `VirtualMachineBackupStatus` and its controller in this repo, reusing the `VirtualMachineBackupsDiscovery` controller as a structural template.

## Open Issues
- **Live cluster vs. tarball for VM manifests.** Reading the VM's `spec.template.spec.volumes[]` from the live cluster is cheap and fast, but only works while the source VM still exists. Our own `vmfr-test-ns` repro namespace was deleted after its backups were taken, which is exactly the case where the tool must fall back to downloading and parsing `<backup>.tar.gz`. Phase 1 needs to support both paths, live-cluster first, tarball fallback.
- **Reason-code taxonomy completeness.** The table above covers the failure modes we could reproduce locally (missing PVC, missing DataVolume, missing DataSource). Cohesity's reported stale-config cases at scale (orphan VM/VMI, CSI mount issues, snapshot timeouts) should be used to validate and extend this list before Phase 1 is considered done.
- **Skip semantics.** Velero does not always explicitly mark an item as "skipped" the way it marks failures; we need to decide what counts as `Skipped` for a VM (e.g., excluded by backup label selector) versus simply absent from the backup's included namespaces.
- **Ownership of Phase 1.** The command belongs in `migtools/oadp-cli`, a separate repository from this operator; this design doc lives here because the correlation logic and Phase 2 CRD are OADP-operator concerns, but Phase 1 implementation work needs to happen (or be coordinated) against that repo.

## References
- [OADP-8697](https://redhat.atlassian.net/browse/OADP-8697)
- `docs/design/vm_file_restore-design.md` (VirtualMachineBackupsDiscovery/VirtualMachineFileRestore, the controller pattern Phase 2 would reuse)
- `docs/design/kubectl-oadp.md` (existing kubectl-oadp plugin design, the CLI Phase 1 would extend)
- [migtools/oadp-cli](https://github.com/migtools/oadp-cli)
- [migtools/kubevirt-velero-plugin](https://github.com/migtools/kubevirt-velero-plugin)

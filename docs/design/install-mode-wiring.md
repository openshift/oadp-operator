# OADP Install-Mode Wiring

_Status: Draft — 2026-11-20_

This document shows how OLM, the OperatorGroup, the CSV, ServiceAccounts, RBAC, the operator manager cache, DataProtectionApplications (DPAs), and Velero fit together in each install mode.
It is a public-safe reference; customer migration mechanics and release timelines are out of scope.

## Legend

- `[ns]` — namespace-scoped object
- `[cluster]` — cluster-scoped object
- `[watch]` — manager cache watch scope
- Arrows show control or data flow, not network traffic.

## OwnNamespace

```text
+-- OWNNAMESPACE - OLMv0, namespace-scoped -----------------------------------+
|                                                                            |
|  CONTROL PLANE (OLM install time)                                          |
|                                                                            |
|  +----------------+   +----------------+   +------------------+   +------------------+
|  | OperatorGroup  |-->| CSV            |-->| OLM creates      |-->| Operator         |
|  | [ns]           |   | [ns]           |   | SAs + RBAC       |   | Deployment [ns]  |
|  | targetNS:      |   | installModes:  |   | permissions [ns] |   | WATCH_NAMESPACE= |
|  |  - openshift   |   |  OwnNS: true   |   |  -> Role + RB    |   |   openshift-adp  |
|  |    -adp        |   |  AllNS: false  |   | clusterPerm [cl] |   | OPERATOR_NS=     |
|  +----------------+   +----------------+   |  -> CR + CRB     |   |   openshift-adp  |
|                                            | SAs: controller, |   +--------+---------+
|                                            |      velero, ... |            |
|                                            +------------------+            v
|                                                                   +------------------+
|                                                                   | Manager cache    |
|  +-- CLUSTER-SCOPED RBAC -------------------------------------+   | watch:           |
|  | CSV.clusterPermissions -> ClusterRole -> ClusterRoleBinding|   |   openshift-adp  |
|  |   -> operator SA: openshift-adp-controller-manager         |   +--------+---------+
|  |   -> velero SA:   openshift-adp/velero                     |            |
|  | Change 5: ClusterRole oadp-velero                          |            v
|  |           CRB oadp-velero-openshift-adp                    |   +------------------+
|  +------------------------------------------------------------+   | DPA: oadp-sample |
|                                                                   | [ns: openshift-  |
|                                                                   |      adp]        |
|                                                                   +--------+---------+
|                                                                            |
|                                                                            v
|                                                                   +------------------+
|                                                                   | Operator         |
|                                                                   | reconciler       |
|                                                                   |  +-> Velero      |
|                                                                   |  |   Deployment  |
|                                                                   |  |   SA: velero  |
|                                                                   |  |   VELERO_NS=  |
|                                                                   |  |   openshift-  |
|                                                                   |  |     adp       |
|                                                                   |  +-> BSL / VSL / |
|                                                                   |      NodeAgent   |
|                                                                   +------------------+
|                                                                            |
|  +-- NAMESPACE tenant-a --------------------------------------------------+
|  | DPA: tenant-dpa [ns]                                                   |
|  |   -> not in manager cache                                              |
|  |   -> not reconciled in OwnNamespace                                    |
|  +------------------------------------------------------------------------+
|                                                                            |
+----------------------------------------------------------------------------+
```

Notes:

- OwnNamespace scopes the manager cache and the CSV `permissions` RoleBindings.
- `clusterPermissions` remains cluster-scoped in both install modes.
- The CSV is not copied to tenant namespaces.
- The CSV box shows the mode selected by the OperatorGroup. A dual-mode CSV also declares `AllNamespaces: true`; the OperatorGroup determines the active mode.
- The diagram assumes the manager cache is gated to `watchNamespace`. If the cache is unconditional, DPAs in `tenant-a` are visible to the manager even under OwnNamespace.

## AllNamespaces

```text
+-- ALLNAMESPACES - OLMv0, cluster-scoped ------------------------------------+
|                                                                            |
|  CONTROL PLANE (OLM install time)                                          |
|                                                                            |
|  +----------------+   +----------------+   +------------------+   +------------------+
|  | OperatorGroup  |-->| CSV            |-->| OLM creates      |-->| Operator         |
|  | [ns]           |   | [ns]           |   | SAs + RBAC       |   | Deployment [ns]  |
|  | spec: {}       |   | installModes:  |   | permissions [ns] |   | WATCH_NAMESPACE= |
|  | global         |   |  AllNS: true   |   |  -> ClusterRole  |   |   ""             |
|  | status.ns: [""]|   |  OwnNS: true   |   |     + CRB        |   | OPERATOR_NS=     |
|  +----------------+   |  (dual-mode)   |   | clusterPerm [cl] |   |   openshift-adp  |
|                       +----------------+   |  -> CR + CRB     |   +--------+---------+
|                                            | SAs in openshift-|            |
|                                            | adp ONLY         |            v
|                                            +------------------+   +------------------+
|                                                                   | Manager cache    |
|  +-- CLUSTER-SCOPED RBAC -------------------------------------+   | watch:           |
|  | CSV.permissions -> ClusterRole + CRB (promoted)            |   |   all namespaces |
|  | CSV.clusterPermissions -> ClusterRole + CRB                |   +--------+---------+
|  |   -> operator SA: openshift-adp-controller-manager         |            |
|  |   -> velero SA:   openshift-adp/velero                     |            |
|  | Change 5: SA velero in each DPA namespace                  |            |
|  |           ClusterRole oadp-velero                          |            |
|  |           CRB oadp-velero-<DPA namespace>                  |            |
|  +------------------------------------------------------------+            |
|                                                                            |
|                 +----------------------------------------------------------+
|                 |                                                          |
|                 v                                                          v
|        +------------------+                                      +------------------+
|        | DPA: oadp-sample |                                      | DPA: tenant-dpa  |
|        | [ns: openshift-  |                                      | [ns: tenant-a]   |
|        |      adp]        |                                      |                  |
|        +--------+---------+                                      +--------+---------+
|                 |                                                          |
|                 v                                                          v
|        +------------------+                                      +------------------+
|        | Velero Deployment|                                      | Velero Deployment|
|        | [ns: openshift-  |                                      | [ns: tenant-a]   |
|        |      adp]        |                                      |                  |
|        | SA: velero       |                                      | SA: velero       |
|        | VELERO_NS=       |                                      | VELERO_NS=       |
|        |   openshift-adp  |                                      |   tenant-a       |
|        | watch: Backups / |                                      | watch: Backups / |
|        | Restores / BSLs  |                                      | Restores / BSLs  |
|        +------------------+                                      +------------------+
|                                                                            |
|  +-- NAMESPACE tenant-a --------------------------------------------------+
|  | DPA: tenant-dpa [ns]                                                   |
|  |   -> now in the manager cache (all namespaces)                         |
|  |   -> Velero SA + RBAC created in tenant-a by the operator              |
|  +------------------------------------------------------------------------+
|                                                                            |
|  NOTES                                                                     |
|  - WATCH_NAMESPACE="" is OLM's all-namespaces signal.                      |
|  - OPERATOR_NAMESPACE=openshift-adp still drives PSA/CLI/VMDP setup.       |
|  - OLM creates SAs only in openshift-adp; tenant SAs are operator-created. |
|  - OLMv0 aggregate admin/edit/view roles can permit DPA creation by users  |
|    in other namespaces; restrict DPA-create RBAC if self-service is not    |
|    desired.                                                                |
|                                                                            |
+----------------------------------------------------------------------------+
```

Notes:

- `WATCH_NAMESPACE=""` is OLM's all-namespaces signal.
- `OPERATOR_NAMESPACE=openshift-adp` still drives PSA labeling and CLI/VMDP setup.
- The CSV box shows the dual-mode bridge. An AllNamespaces-only CSV sets `OwnNamespace: false`; the runtime wiring is otherwise the same.
- OLM creates the CSV-declared ServiceAccounts only in `openshift-adp`; the operator creates the `velero` ServiceAccount in each tenant namespace.
- OLMv0 promotes CSV `permissions` to ClusterRoles/ClusterRoleBindings.
- OLMv0's aggregated `admin`/`edit`/`view` ClusterRoles can allow users in other namespaces to create DPAs. Restrict DPA-create RBAC if self-service is not intended.

## OLMv1

```text
+-- OLMv1 - ClusterExtension, no install modes --------------------------------+
|                                                                            |
|  CONTROL PLANE (OLMv1 install time)                                        |
|                                                                            |
|  +----------------+   +----------------+   +------------------+   +------------------+
|  | ClusterCatalog |-->| ClusterExtension|-->| Bundle CSV       |-->| Operator         |
|  | [cluster]      |   | [cluster]      |   | (inert metadata) |   | Deployment [ns]  |
|  |                |   | spec.namespace:|   | only install.    |   | WATCH_NAMESPACE= |
|  |                |   |   openshift-adp|   | spec.deployments |   |   unset/empty    |
|  |                |   | serviceAccount:|   | is rendered      |   | OPERATOR_NS=     |
|  |                |   |   deprecated   |   | installModes /   |   |   openshift-adp  |
|  |                |   |   ignored      |   | permissions are  |   +--------+---------+
|  |                |   | source: Catalog|   | ignored          |            |
|  +----------------+   +----------------+   +------------------+            v
|                                                                   +------------------+
|  +-- NO OPERATORGROUP / NO INSTALLMODES -----------------------+   | Manager cache    |
|  | OLMv1 operator-controller uses its own cluster-admin SA.    |   | watch:           |
|  | ClusterExtension is cluster-scoped.                         |   |   all namespaces |
|  | spec.namespace is the install namespace, not a watch scope. |   +--------+---------+
|  +------------------------------------------------------------+            |
|                                                                            |
|                 +----------------------------------------------------------+
|                 |                                                          |
|                 v                                                          v
|        +------------------+                                      +------------------+
|        | DPA: oadp-sample |                                      | DPA: tenant-dpa  |
|        | [ns: openshift-  |                                      | [ns: tenant-a]   |
|        |      adp]        |                                      |                  |
|        +--------+---------+                                      +--------+---------+
|                 |                                                          |
|                 v                                                          v
|        +------------------+                                      +------------------+
|        | Velero Deployment|                                      | Velero Deployment|
|        | [ns: openshift-  |                                      | [ns: tenant-a]   |
|        |      adp]        |                                      |                  |
|        | SA: velero       |                                      | SA: velero       |
|        | (operator-       |                                      | (operator-       |
|        |  created)        |                                      |  created)        |
|        | VELERO_NS=       |                                      | VELERO_NS=       |
|        |   openshift-adp  |                                      |   tenant-a       |
|        | watch: Backups / |                                      | watch: Backups / |
|        | Restores / BSLs  |                                      | Restores / BSLs  |
|        +------------------+                                      +------------------+
|                                                                            |
|  +-- OPERATOR-MANAGED WORKLOAD RBAC --------------------------------------+
|  | SA velero [ns: each DPA namespace]                                     |
|  | ClusterRole oadp-velero [cluster]                                      |
|  | ClusterRoleBinding oadp-velero-<DPA namespace> [cluster]               |
|  |   -> velero SA [ns: <DPA namespace>]                                   |
|  +------------------------------------------------------------------------+
|                                                                            |
|  +-- OLMv1 NOTES ---------------------------------------------------------+
|  | No OperatorGroup, no installModes, no CSV-generated RBAC.              |
|  | No OLM-created workload SAs; the operator must create them.            |
|  | Uninstall is cascading: deleting the ClusterExtension removes          |
|  | deployments, RBAC, CRDs, and their CRs. Use --cascade=orphan to retain |
|  | CRDs and data. PSA labels are not managed by OLMv1.                    |
|  +------------------------------------------------------------------------+
|                                                                            |
+----------------------------------------------------------------------------+
```

Notes:

- OLMv1 has no OperatorGroup and no install-mode selection.
- `ClusterExtension` is cluster-scoped; `spec.namespace` is the install namespace, not a watch scope.
- `spec.serviceAccount` is deprecated and ignored; OLMv1 uses its own cluster-admin ServiceAccount.
- The operator Deployment's ServiceAccount comes from the CSV deployment spec; OLMv1 does not generate RBAC from the CSV.
- The bundle CSV is inert metadata: only `install.spec.deployments` is rendered; `installModes`, `permissions`, and `clusterPermissions` are ignored.
- The operator must create the `velero` ServiceAccount and RBAC in each DPA namespace.
- Uninstall follows Kubernetes cascading deletion. Use `oc delete clusterextension <name> --cascade=orphan` to retain CRDs and data.
- PSA labels are not managed by OLMv1; the operator must handle them explicitly.

## Comparison

| Aspect | OwnNamespace | AllNamespaces | OLMv1 |
|---|---|---|---|
| OperatorGroup | namespaced target | global (`spec: {}`) | none |
| installModes | `OwnNamespace: true` | `AllNamespaces: true` | none |
| `WATCH_NAMESPACE` | `openshift-adp` | `""` | unset/empty |
| Manager cache | `openshift-adp` | all namespaces | all namespaces |
| OLM-created SAs | all in `openshift-adp` | only `openshift-adp` | none from CSV; operator-managed |
| CSV RBAC | `permissions` + `clusterPermissions` | `permissions` promoted | ignored |
| CSV copies | none | every namespace | not applicable |
| Uninstall | OLMv0 | OLMv0 | cascading deletion |

## References

- [OLM OperatorGroup design](https://github.com/operator-framework/operator-lifecycle-manager/blob/master/doc/design/operatorgroups.md)
- [OLM OperatorGroup documentation](https://olm.operatorframework.io/docs/concepts/crds/operatorgroup)
- [OLMv1 design decisions](https://operator-framework.github.io/operator-controller/project/olmv1_design_decisions/)
- [OLMv1 limitations](https://operator-framework.github.io/operator-controller/project/olmv1_limitations/)

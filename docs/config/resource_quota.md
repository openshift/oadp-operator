# ResourceQuota for the OADP namespace

When a DataProtectionApplication (DPA) is reconciled, the OADP operator ensures a
namespace-scoped `ResourceQuota` named `oadp-resource-quota` exists in the DPA
namespace (typically `openshift-adp`).

## Defaults

| Hard resource | Default |
|---------------|---------|
| `pods` | `200` |
| `requests.cpu` | `50` |
| `requests.memory` | `64Gi` |

These values are a mid-size cluster ceiling (operator, Velero, node-agent DaemonSet,
optional helpers, and some concurrent backup/restore pods). The default quota does
**not** set `limits.cpu` or `limits.memory` because default Velero and node-agent
pods specify requests only; tracking limits in the quota can cause admission to
reject those pods.

## Create-if-absent

- If the quota is **missing**, the operator creates it with the defaults above.
- If the quota **exists**, the operator leaves it unchanged (admin edits survive
  reconcile and upgrades).
- If an admin **deletes** the quota, the next DPA reconcile recreates the defaults.

## Inspect and adjust

```bash
oc describe resourcequota oadp-resource-quota -n openshift-adp

oc edit resourcequota oadp-resource-quota -n openshift-adp

oc patch resourcequota oadp-resource-quota -n openshift-adp --type merge \
  -p '{"spec":{"hard":{"pods":"400","requests.cpu":"100","requests.memory":"128Gi"}}}'
```

Node-agent is a DaemonSet: large clusters may need higher `pods` / `requests.cpu` /
`requests.memory`. If pods are Pending because of quota, raise the hard limits.

Admins who set container limits on all workloads may optionally add `limits.cpu` /
`limits.memory` to the quota via `oc edit`.

## Design

See [docs/design/2026-07-16-resourcequota-design.md](../design/2026-07-16-resourcequota-design.md).

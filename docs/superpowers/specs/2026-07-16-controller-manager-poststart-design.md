# Design: Controller Manager `lifecycle.postStart` Hook

**Date:** 2026-07-16  
**Status:** Draft — awaiting user review  
**Scope:** `openshift-adp-controller-manager` container only  
**Related:** `docs/container_lifecycle_hooks_exception.md` (Best Practice #76 / #90)

---

## 1. Goal

Satisfy platform / compliance expectations that containers configure lifecycle management so they can initialize before servicing traffic and react correctly to platform events (e.g. SIGTERM / SIGKILL, PostStart, PreStop).

For this change, configure a **`lifecycle.postStart`** hook on the **`openshift-adp-controller-manager`** (`manager`) container that waits until the manager’s `/healthz` endpoint is healthy.

---

## 2. Current State

| Item | Today |
|------|--------|
| Deployment source | `config/manager/manager.yaml` (container name `manager`; OLM CSV name `openshift-adp-controller-manager`) |
| Probes | `startupProbe`, `livenessProbe`, `readinessProbe` on `:8081` (`/healthz`, `/readyz`) |
| Graceful shutdown | `ctrl.SetupSignalHandler()` in `cmd/main.go`; `terminationGracePeriodSeconds: 10` |
| `lifecycle.postStart` | Not set |
| `lifecycle.preStop` | Not set |
| Documented exception | `docs/container_lifecycle_hooks_exception.md` — intentional omission of postStart (#76) and preStop (#90) |
| Runtime image | `registry.access.redhat.com/ubi9-minimal` with only `/manager` copied in (`Dockerfile`) |

Graceful shutdown for SIGTERM is already handled by controller-runtime. This work is specifically about adding **PostStart** for compliance plus a useful health wait—not about replacing signal handling.

---

## 3. Requirements (agreed)

1. **Add** `lifecycle.postStart` so scanners / best-practice checks pass for the controller-manager container.
2. **Do not** add `preStop` in this change (preStop exception remains valid).
3. **postStart behavior:** readiness-style wait until `http://127.0.0.1:8081/healthz` succeeds (not a no-op).
4. **Surface area:** manager Deployment / CSV only; Velero, node-agent, and other workloads are out of scope.

---

## 4. Kubernetes Semantics (constraints)

These constraints drive which options are safe:

1. **postStart is not a reliable “before ENTRYPOINT” gate.** The kubelet may run it concurrently with the main process. It must not assume the process has not started; it should **wait** until health is ready.
2. **A failing postStart kills the container.** One-shot checks against `/healthz` at the moment the hook starts often fail (manager not listening yet) and will crash-loop the pod.
3. **Lifecycle handlers** support `exec`, `httpGet`, and (on newer clusters) `sleep`. `httpGet` for lifecycle is **one-shot**—no built-in retry—so it is a poor fit for waiting on a just-started process.
4. **UBI9-minimal** does not ship `curl`/`wget` by default. Any HTTP client used in `exec` must already exist or be added to the image.

---

## 5. Options

### Option A — `exec` retry loop with `curl` (recommended)

**What:** Add `lifecycle.postStart.exec` that loops (e.g. up to ~120s) calling `curl -sf http://127.0.0.1:8081/healthz`, then exits 0/1. Install `curl` in the manager image via `microdnf` in `Dockerfile`.

**Example sketch:**

```yaml
lifecycle:
  postStart:
    exec:
      command:
        - /bin/sh
        - -c
        - |
          for i in $(seq 1 60); do
            curl -sf http://127.0.0.1:8081/healthz && exit 0
            sleep 2
          done
          exit 1
```

**Pros**

- Satisfies compliance (field present).
- Real HTTP health check aligned with existing probes.
- Retry avoids postStart race / crash-loop.
- Timeout can align with `startupProbe` budget (12 × 10s = 120s).

**Cons**

- Slightly larger image / attack surface (`curl`).
- Shell script in manifest needs careful quoting / CSV regeneration via `make bundle`.

**Touches:** `Dockerfile`, `config/manager/manager.yaml`, regenerated bundle CSV, exception doc update.

---

### Option B — `exec` retry loop with bash `/dev/tcp` (no new packages)

**What:** Same retry structure, but open `127.0.0.1:8081` via bash `/dev/tcp` instead of HTTP GET.

**Pros**

- No Dockerfile package change.
- Still provides a wait-until-listening postStart.

**Cons**

- Only checks TCP accept, **not** HTTP 200 on `/healthz`.
- Depends on bash + `/dev/tcp` remaining available in UBI9-minimal.
- Weaker match to the “wait for healthz” intent.

---

### Option C — Lifecycle `httpGet` to `/healthz` (not recommended)

**What:**

```yaml
lifecycle:
  postStart:
    httpGet:
      path: /healthz
      port: 8081
```

**Pros**

- Declarative; no curl or shell.

**Cons**

- **One-shot:** if the manager is not listening when the hook runs, postStart fails and the container is killed.
- Race with manager startup makes this unreliable in practice.
- Does not implement a wait loop.

**Verdict:** Reject for this use case.

---

### Option D — Manager binary as postStart checker

**What:** Add a small mode (e.g. `/manager --wait-for-healthz`) or a tiny sidecar binary used only as the postStart command, looping until `/healthz` returns success.

**Pros**

- No `curl`; exact HTTP semantics; controlled by Go code / tests.
- Works on a minimal image with only the manager binary.

**Cons**

- More product code and test surface for a compliance-driven hook.
- ENTRYPOINT is `/manager`; args/flags must not confuse the main process vs the hook (postStart uses its own command, so this is solvable but easy to get wrong in docs).

---

### Option E — Keep exception (no postStart); strengthen documentation only

**What:** Do not add the hook; rely on `docs/container_lifecycle_hooks_exception.md` for audit exceptions.

**Pros**

- No runtime / image change; matches historical rationale (controller-runtime retries + probes).

**Cons**

- Does **not** meet the agreed goal (user chose to add a compliance-oriented postStart).
- Listed here for completeness / if scanners later accept the exception.

**Verdict:** Out of scope for the chosen direction; keep as fallback if product decides not to ship A–D.

---

## 6. Recommendation

**Ship Option A:**

1. Install `curl` in the UBI9-minimal manager image.
2. Add retrying `lifecycle.postStart.exec` against `http://127.0.0.1:8081/healthz` in `config/manager/manager.yaml`.
3. Regenerate OLM bundle so the CSV deployment for `openshift-adp-controller-manager` includes the hook.
4. Update `docs/container_lifecycle_hooks_exception.md`:
   - Exception 1 (postStart): mark as **implemented** for the controller-manager (describe the healthz wait).
   - Exception 2 (preStop): **unchanged** (still intentionally omitted).

**Timeout guidance:** Cap the loop at roughly the existing startupProbe window (~120s) so a permanently unhealthy manager fails clearly rather than hanging indefinitely.

---

## 7. Implementation Touchpoints

| Area | Change |
|------|--------|
| `Dockerfile` | Install `curl` (and clean microdnf cache) before `USER 65532:65532` |
| `config/manager/manager.yaml` | Add `lifecycle.postStart` on the `manager` container |
| Bundle / CSV | `make bundle` (or project equivalent) so `bundle/manifests/oadp-operator.clusterserviceversion.yaml` picks up the Deployment change |
| Docs | Update `docs/container_lifecycle_hooks_exception.md`; this design lives under `docs/superpowers/specs/` |
| Tests | Prefer unit/manifest assertions if the repo already validates Deployment YAML; otherwise smoke via `make deploy-olm` that the pod becomes Ready and the Deployment shows `lifecycle.postStart` |

No Go controller logic changes required for Option A.

---

## 8. Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| postStart fails before `/healthz` is up | Retry loop with sleep; align timeout with startupProbe |
| `curl` not present | Explicit install in Dockerfile; verify in image smoke test |
| Hook hangs forever | Hard exit after N attempts |
| Duplicate “readiness” with probes | Acceptable overlap: probes remain source of truth for kubelet traffic; postStart satisfies lifecycle policy and early init wait |
| CSV drift | Always regenerate bundle after manager YAML change; run `make bundle-isupdated` in CI if available |
| Confusing exception doc | Rewrite Exception 1 so auditors see postStart as present for controller-manager |

---

## 9. Out of Scope

- `lifecycle.preStop` on the manager
- Velero Deployment / DaemonSet / node-agent / non-admin / VMFR containers
- Changing `terminationGracePeriodSeconds` or signal-handling code in `cmd/main.go`
- Cluster-wide compliance policy objects (only the container spec)

---

## 10. Success Criteria

1. Deployed `openshift-adp-controller-manager` container spec includes `lifecycle.postStart`.
2. postStart waits until `/healthz` on port 8081 succeeds (Option A or D), or TCP listen if Option B is explicitly chosen instead.
3. Manager pod reaches Ready under normal conditions; no new crash loops from postStart races.
4. Exception documentation matches reality (postStart present; preStop still excepted).
5. Compliance / best-practice check for postStart on this container passes (or is no longer filed as a finding).

---

## 11. Open Decision

**Default for implementation planning:** Option A.

If product prefers zero new packages, switch to **Option B** (TCP only) or **Option D** (manager binary waiter) before coding.

---

## 12. Next Step

After approval of this spec (and choice of A / B / D if not A), produce an implementation plan via the writing-plans workflow, then implement.

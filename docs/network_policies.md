# OADP NetworkPolicies

OADP creates NetworkPolicies when it reconciles a DataProtectionApplication (DPA). The policies restrict connections to and from selected OADP components in the DPA's namespace. They protect the operator, Velero, node-agent, data movers, download servers, and enabled optional controllers.

OADP does not create a namespace-wide default deny. This allows another product, such as ACM, to run its own controller in the same namespace without OADP isolating that controller.

## How the rules work

A NetworkPolicy selects pods by their labels. Once a policy selects a pod for ingress, new inbound connections must match an ingress allow rule. Once a policy selects a pod for egress, new outbound connections must match an egress allow rule. Declaring a direction with no allow rules blocks new connections in that direction. Replies to permitted connections are allowed automatically.

For example, a mover can start an outbound connection to object storage and receive its replies while new connections initiated by other pods toward the mover are blocked.

Policies are additive: the allowed traffic for each direction is the union of the applicable policies' allow rules. When both endpoints are isolated, the source's egress rules and the destination's ingress rules must both allow the connection. OADP's policies do not override other NetworkPolicies or cluster-level network controls. A pod that does not match an OADP policy is not isolated by that policy, but its connections to selected OADP pods still have to satisfy those pods' ingress rules.

See the [Kubernetes NetworkPolicy documentation](https://kubernetes.io/docs/concepts/services-networking/network-policies/) for the underlying behavior. Enforcement requires a network plugin that supports NetworkPolicy.

## Pod selection

Selectors use labels on the actual pod, rather than labels that appear only on a Deployment or DaemonSet object. Multiple labels in a selector must all match. The selectors below apply only within the DPA's namespace.

| Component | NetworkPolicy name | Required pod labels |
| --- | --- | --- |
| OADP operator | `oadp-operator-network-policy` | `app.kubernetes.io/name=oadp-operator`, `control-plane=controller-manager` |
| Velero and node-agent | `velero-network-policy` | `app.kubernetes.io/managed-by=oadp-operator`, `component=velero` |
| Velero mover and repository-maintenance pods | `velero-mover-network-policy` | `oadp.openshift.io/network-policy=velero` |
| CLI download server | `oadp-cli-server-network-policy` | `app=oadp-cli` |
| VMDP download server | `oadp-vmdp-server-network-policy` | `app=oadp-vmdp` |
| Non-admin controller | `non-admin-controller-network-policy` | `control-plane=non-admin-controller` |
| VM file restore controller | `vm-file-restore-controller-network-policy` | `control-plane=oadp-vm-file-restore-controller` |
| KubeVirt DataMover controller | `kubevirt-datamover-controller-network-policy` | `control-plane=oadp-kubevirt-datamover-controller` |
| KubeVirt DataMover transfer pods | `kubevirt-datamover-pods-network-policy` | The `kubevirt-datamover.io/pod-type` label is present |

The operator and Velero selectors include OADP-specific labels so another controller with only `control-plane=controller-manager`, or another Velero installation with only `component=velero`, does not match. OADP ensures node-agent pod templates carry the same managed-by label that Velero already carries. The DaemonSet's immutable selector is unchanged.

OADP injects the Velero mover label into the node-agent and global repository-maintenance ConfigMaps. Velero uses those settings when creating its temporary pods. The KubeVirt DataMover controller applies its own pod-type label when it creates upload and download pods. New pods are subject to matching policies when the network plugin applies them; they do not need a separate policy for each pod name.

## Allowed traffic

All policies declare both ingress and egress. All listed ports are TCP unless UDP is explicitly included.

| Component | Allowed new inbound connections | Allowed new outbound connections |
| --- | --- | --- |
| OADP operator | Metrics on 8443 from monitoring namespaces; health probes on 8081 without a source restriction | All destinations, including the Kubernetes API and configured storage endpoints used by DataProtectionTest |
| Velero and node-agent | Metrics on 8085 from monitoring namespaces | All destinations required by configurable backup and restore endpoints |
| Velero mover and repository-maintenance pods | None | All destinations required for data transfer and repository maintenance |
| CLI and VMDP download servers | Downloads on 8080 without a source restriction | None |
| Non-admin controller | Ports 8080 and 8081 from monitoring namespaces | DNS to the openshift-dns namespace on TCP/UDP 53 and 5353; TCP 6443 without a destination restriction for the Kubernetes API |
| VM file restore controller | Ports 8081 and 8443 from monitoring namespaces | All destinations, including configured backup storage |
| KubeVirt DataMover controller | Ports 8081 and 8443 from monitoring namespaces | All destinations, including configurable cloud and storage endpoints |
| KubeVirt DataMover transfer pods | None | All destinations required for data transfer |

Monitoring namespaces are selected by `network.openshift.io/policy-group=monitoring`.

Several components require unrestricted egress because administrators can configure arbitrary storage endpoints and ports. These policies restrict ingress but do not provide destination allowlists for those components.

OpenShift's DNS Service exposes port 53, while its DNS pods listen on 5353. The non-admin policy allows both ports over TCP and UDP because OVN-Kubernetes evaluates the destination pod port after Service translation. Both ports remain restricted to the openshift-dns namespace. The API rule uses port 6443 without a destination selector because API endpoints are not consistently selectable by pod labels across cluster topologies.

## Lifecycle and existing installations

The DPA owns these NetworkPolicy objects. OADP reconciles them, restores deleted policies, and removes optional-controller policies when their corresponding features are disabled. Kubernetes garbage collection removes the policies when their owning DPA is deleted.

Earlier candidate builds created a namespace-wide policy named `default-deny`. OADP first reconciles the component policies, including mover ingress isolation, then removes that old policy only if the current DPA is its controller owner. Policies without that ownership are preserved. Deletion preconditions prevent a stale read from deleting a replacement or a policy that changed after the read.

The managed-by label added to the node-agent pod template causes its DaemonSet to roll out updated pods. An older pod without that label does not match the tightened Velero policy until it is replaced. This is a transition that needs live-cluster validation, especially where additional network policies exist.

## Scope and validation

The policies cover the components listed above in the DPA's namespace. VM file restore policies in separate restore namespaces and a uniform label rollout across every component are outside this change. Restored application pods retain their own networking configuration.

The controller tests check generated selectors, excluded ACM and generic workload labels, allowed DNS ports, mover ingress isolation, and ownership-checked cleanup. They do not enforce network traffic. Live validation must exercise ACM startup and backup/restore, DNS over UDP and TCP, OADP backup and restore paths, dynamically created movers, monitoring, and the node-agent rollout on a cluster that enforces NetworkPolicy.

The E2E suite's `network-policy` cases check traffic while OADP's own policies are active. They do not create additional NetworkPolicies:

- Pods with ACM, generic controller-manager, generic Velero, other-operator, other-Velero, and unrelated labels can resolve the API Service, read their test ConfigMap using a restricted service account, and receive HTTP requests from another pod. The inbound check catches an accidentally broad selector even when that policy allows unrestricted egress.
- A partner service account submits a ConfigMap-only Velero backup and restore through the API from its own pod. The test deletes the original ConfigMap and verifies its restored contents.
- A monitoring-labelled namespace can reach Velero metrics, while an ordinary namespace cannot. Ordinary pods can reach both download servers, while those servers cannot initiate API connections. Positive checks establish that the destination is reachable before expecting a timeout from a restricted source.
- The enabled non-admin controller resolves DNS over UDP and TCP through both Service port 53 and endpoint port 5353, and accesses the API. Its other egress is blocked. Monitoring can reach its metrics, while an ordinary namespace cannot. A restricted diagnostic container shares the actual controller pod's network namespace because its image has no shell. The test restores the DPA configuration afterward.

The existing Kopia and CSI data mover backup/restore tests also check that actual node-agent pods match their policy. CSI data mover tests watch upload and download pods from before each operation starts, then check the observed pods against the mover policy, including pods already removed after transfer completion.

These checks run alongside the backup/restore and DPA deployment tests in the existing provider E2E jobs through `make test-e2e`; no separate Prow job is needed. Test pods, service accounts, ConfigMaps, RBAC resources, namespaces, and backup/restore artifacts are cleaned up afterward. Traffic checks require a cluster that enforces NetworkPolicy. Full ACM and individual partner-product integration remain separate validation.

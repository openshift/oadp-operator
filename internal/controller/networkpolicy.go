package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openshift/oadp-operator/pkg/common"
)

const (
	defaultDenyNetworkPolicyName           = "default-deny"
	operatorNetworkPolicyName              = "oadp-operator-network-policy"
	veleroNetworkPolicyName                = "velero-network-policy"
	veleroMoverNetworkPolicyName           = "velero-mover-network-policy"
	cliServerNetworkPolicyName             = "oadp-cli-server-network-policy"
	vmdpServerNetworkPolicyName            = "oadp-vmdp-server-network-policy"
	nonAdminNetworkPolicyName              = "non-admin-controller-network-policy"
	vmFileRestoreNetworkPolicyName         = "vm-file-restore-controller-network-policy"
	kubevirtDatamoverNetworkPolicyName     = "kubevirt-datamover-controller-network-policy"
	kubevirtDatamoverPodsNetworkPolicyName = "kubevirt-datamover-pods-network-policy"

	// dnsPort is the standard port used for DNS resolution (TCP and UDP).
	dnsPort = 53
	// apiServerPort is the standard port used for the Kubernetes API server.
	apiServerPort = 6443
)

// networkPolicyMoverLabel is applied to Velero's dynamically-spawned mover/maintenance
// pods (CSI DataUpload/DataDownload, PodVolumeBackup/Restore, repository-maintenance jobs)
// via NodeAgentConfigMapSettings.PodLabels / RepositoryMaintenanceConfig.PodLabels.
// It is intentionally distinct from "component=velero" (used by the main Velero
// Deployment/node-agent DaemonSet) to avoid unintentionally widening the scope of any
// existing selector (PodDisruptionBudget, ServiceMonitor, affinity rules, etc.) that
// already keys off "component=velero" today.
const networkPolicyMoverLabel = "oadp.openshift.io/network-policy"
const networkPolicyMoverLabelValue = "velero"

// kubevirtDatamoverPodTypeLabel is set by the kubevirt-datamover-controller (a
// separate repo/image) on the upload/download pods it dynamically spawns (e.g.
// "kubevirt-dm-du-*") to perform CSI DataUpload/DataDownload transfers for VM disks.
// These pods do not carry "control-plane" (used by the controller Deployment) or
// "component=velero"/networkPolicyMoverLabel (used by Velero's own mover pods), so
// they need their own allow-policy; see reconcileKubevirtDatamoverPodsNetworkPolicy.
const kubevirtDatamoverPodTypeLabel = "kubevirt-datamover.io/pod-type"

// scopedEgressRules returns the standard "least privilege" egress rules shared by
// operands that only need to reach DNS and the Kubernetes API server (no direct
// cloud/object-storage access). The API server rule intentionally omits a peer
// selector because the API server is not reliably selectable via podSelector/
// namespaceSelector across all cluster topologies (see NTO's 55-network-policy.yaml
// for the same, established pattern).
func scopedEgressRules() []networkingv1.NetworkPolicyEgressRule {
	tcp := corev1.ProtocolTCP
	udp := corev1.ProtocolUDP
	return []networkingv1.NetworkPolicyEgressRule{
		{
			// Allow DNS resolution via the cluster DNS service.
			To: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": "openshift-dns",
						},
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcp, Port: &intstr.IntOrString{Type: intstr.Int, IntVal: dnsPort}},
				{Protocol: &udp, Port: &intstr.IntOrString{Type: intstr.Int, IntVal: dnsPort}},
			},
		},
		{
			// Allow access to the Kubernetes API server. No peer restriction:
			// the API server endpoint(s) cannot be reliably scoped via
			// podSelector/namespaceSelector across all cluster topologies.
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcp, Port: &intstr.IntOrString{Type: intstr.Int, IntVal: apiServerPort}},
			},
		},
	}
}

// unrestrictedEgressRule returns an egress rule allowing all destinations. Used only by
// operands that must reach arbitrary, admin-configured endpoints (e.g. S3-compatible BSLs)
// that cannot be enumerated ahead of time.
func unrestrictedEgressRule() []networkingv1.NetworkPolicyEgressRule {
	return []networkingv1.NetworkPolicyEgressRule{{}}
}

// createOrUpdateNetworkPolicy wraps controllerutil.CreateOrUpdate with a short retry on
// AlreadyExists errors. Each reconcile below reads via the manager's cached client, which
// can briefly lag behind the API server (e.g. right after a previous DPA's NetworkPolicy
// of the same fixed name was deleted but not yet reflected in the informer cache). That
// causes CreateOrUpdate's Get to see NotFound and attempt a Create that the API server
// rejects as AlreadyExists. Left alone, this self-heals on the next 5s reconcile, but
// during rapid DPA delete/recreate cycles (as in e2e tests) it can take several cycles,
// stalling DPA reconciliation long enough to cascade into unrelated test timeouts. Retrying
// immediately, within the same call, gives the cache a brief chance to catch up so the
// retry's Get correctly reflects the deletion and Create succeeds right away.
func createOrUpdateNetworkPolicy(ctx context.Context, c client.Client, np *networkingv1.NetworkPolicy, mutateFn controllerutil.MutateFn) (controllerutil.OperationResult, error) {
	var op controllerutil.OperationResult
	err := retry.OnError(retry.DefaultBackoff, k8serrors.IsAlreadyExists, func() error {
		var err error
		op, err = controllerutil.CreateOrUpdate(ctx, c, np, mutateFn)
		return err
	})
	return op, err
}

// ReconcileNetworkPolicies creates NetworkPolicies for OADP operands.
// OCPSTRAT-819: Operator creates NPs at runtime for its workloads (operands).
// Design: Default-deny all ingress/egress traffic in the namespace, then allow only
// the specific ingress (metrics/health) and egress (DNS/API-server, or unrestricted
// for operands that talk directly to admin-configured, arbitrary S3-compatible
// endpoints) each operand actually needs.
//
// Ordering: all per-operand "allow" NetworkPolicies are reconciled BEFORE the
// default-deny policy (which is reconciled last). This is intentional: if a
// transient error (e.g. an "already exists" race during rapid DPA
// delete/recreate cycles, as happens in e2e tests) aborts this batch partway
// through, we want the failure mode to be "some allow-NPs may be missing and
// default-deny hasn't been applied yet" (briefly more permissive, self-heals
// on the next reconcile) rather than "default-deny is already active but an
// operand's allow-NP was never created" (a hard outage for that operand,
// since it would be fully blocked with no egress/ingress until the next
// successful reconcile).
func (r *DataProtectionApplicationReconciler) ReconcileNetworkPolicies(log logr.Logger) (bool, error) {
	// Reconcile OADP operator NetworkPolicy
	if err := r.reconcileOperatorNetworkPolicy(log); err != nil {
		return false, err
	}

	// Reconcile Velero/node-agent NetworkPolicy
	if err := r.reconcileVeleroNetworkPolicy(log); err != nil {
		return false, err
	}

	// Reconcile NetworkPolicy for Velero's dynamically-spawned mover/maintenance pods
	// (CSI DataUpload/DataDownload, PodVolumeBackup/Restore, repository-maintenance jobs)
	if err := r.reconcileVeleroMoverNetworkPolicy(log); err != nil {
		return false, err
	}

	// Reconcile CLI server NetworkPolicy
	if err := r.reconcileCLIServerNetworkPolicy(log); err != nil {
		return false, err
	}

	// Reconcile VMDP server NetworkPolicy
	if err := r.reconcileVMDPServerNetworkPolicy(log); err != nil {
		return false, err
	}

	// Reconcile Non-Admin Controller NetworkPolicy (conditional)
	if err := r.reconcileNonAdminNetworkPolicy(log); err != nil {
		return false, err
	}

	// Reconcile VM File Restore Controller NetworkPolicy (conditional)
	if err := r.reconcileVMFileRestoreNetworkPolicy(log); err != nil {
		return false, err
	}

	// Reconcile KubeVirt DataMover Controller NetworkPolicy (conditional)
	if err := r.reconcileKubevirtDatamoverNetworkPolicy(log); err != nil {
		return false, err
	}

	// Reconcile KubeVirt DataMover dynamically-spawned upload/download pods
	// NetworkPolicy (conditional)
	if err := r.reconcileKubevirtDatamoverPodsNetworkPolicy(log); err != nil {
		return false, err
	}

	// Reconcile default-deny policy last (baseline security), only after all
	// allow-NPs above have been successfully created/updated.
	if err := r.reconcileDefaultDenyNetworkPolicy(log); err != nil {
		return false, err
	}

	return true, nil
}

// reconcileDefaultDenyNetworkPolicy creates a default-deny NetworkPolicy for the entire namespace.
// This provides baseline security: all pods are denied ingress/egress by default,
// then specific allow rules are added by other NetworkPolicies.
func (r *DataProtectionApplicationReconciler) reconcileDefaultDenyNetworkPolicy(log logr.Logger) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultDenyNetworkPolicyName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := createOrUpdateNetworkPolicy(r.Context, r.Client, np, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(r.dpa, np, r.Scheme); err != nil {
			return err
		}

		// Apply labels
		np.Labels = getDpaAppLabels(r.dpa)
		np.Labels, np.Annotations = applyResourceLabels(r.dpa, np.Labels, np.Annotations)

		// Empty podSelector = select ALL pods in namespace
		// No ingress/egress rules = deny all traffic by default; other NetworkPolicies
		// in this namespace add back only the specific ingress/egress each operand needs.
		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile default-deny NetworkPolicy: %w", err)
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		log.Info(fmt.Sprintf("Default-deny NetworkPolicy %s: %s", np.Name, op))
		r.EventRecorder.Event(np,
			corev1.EventTypeNormal,
			"DefaultDenyNetworkPolicyReconciled",
			fmt.Sprintf("performed %s on default-deny NetworkPolicy %s/%s", op, np.Namespace, np.Name),
		)
	}
	return nil
}

// reconcileOperatorNetworkPolicy creates a NetworkPolicy for the OADP operator pod itself.
// Allows metrics and health endpoints.
func (r *DataProtectionApplicationReconciler) reconcileOperatorNetworkPolicy(log logr.Logger) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorNetworkPolicyName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := createOrUpdateNetworkPolicy(r.Context, r.Client, np, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(r.dpa, np, r.Scheme); err != nil {
			return err
		}

		// Apply labels
		np.Labels = getDpaAppLabels(r.dpa)
		np.Labels, np.Annotations = applyResourceLabels(r.dpa, np.Labels, np.Annotations)

		// Pod selector: control-plane=controller-manager (matches operator pod)
		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"control-plane": "controller-manager",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// Allow metrics scrape from the monitoring namespace only (matches the
					// scoping used by the non-admin/vm-file-restore/kubevirt-datamover policies).
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"network.openshift.io/policy-group": "monitoring",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8443},
						},
					},
				},
				{
					// Allow the kubelet to reach the manager's health-probe port. Liveness/
					// readiness/startup probes hit :8081/healthz from the node's kubelet (not
					// from a pod), so this cannot be scoped to a namespaceSelector and must stay
					// open, or the operator pod gets marked unhealthy and restarts.
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8081},
						},
					},
				},
			},
			// Egress: unrestricted. Although the operator mainly reconciles CRs via the
			// Kubernetes API, the DataProtectionTest controller runs directly inside this
			// pod and connects to admin-configured BSL endpoints (S3, GCS, etc.) to measure
			// upload speed, so DNS+API-server-only egress is insufficient.
			Egress: unrestrictedEgressRule(),
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile OADP operator NetworkPolicy: %w", err)
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		log.Info(fmt.Sprintf("OADP operator NetworkPolicy %s: %s", np.Name, op))
		r.EventRecorder.Event(np,
			corev1.EventTypeNormal,
			"OperatorNetworkPolicyReconciled",
			fmt.Sprintf("performed %s on OADP operator NetworkPolicy %s/%s", op, np.Namespace, np.Name),
		)
	}
	return nil
}

// reconcileVeleroNetworkPolicy creates a NetworkPolicy for the Velero deployment and node-agent daemonset.
// Note: this selector (component=velero) does NOT cover Velero's dynamically-spawned
// mover/maintenance pods (CSI DataUpload/DataDownload, PodVolumeBackup/Restore,
// repository-maintenance jobs) - those are covered by reconcileVeleroMoverNetworkPolicy.
func (r *DataProtectionApplicationReconciler) reconcileVeleroNetworkPolicy(log logr.Logger) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      veleroNetworkPolicyName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := createOrUpdateNetworkPolicy(r.Context, r.Client, np, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(r.dpa, np, r.Scheme); err != nil {
			return err
		}

		// Apply labels
		np.Labels = getDpaAppLabels(r.dpa)
		np.Labels, np.Annotations = applyResourceLabels(r.dpa, np.Labels, np.Annotations)

		// Pod selector: component=velero (matches velero deployment and node-agent daemonset)
		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"component": common.Velero,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// Allow metrics scrape from the monitoring namespace only (matches the
					// scoping used by the non-admin/vm-file-restore/kubevirt-datamover policies).
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"network.openshift.io/policy-group": "monitoring",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8085},
						},
					},
				},
			},
			// Egress: unrestricted, since Velero/node-agent must reach admin-configured,
			// arbitrary S3-compatible BackupStorageLocation endpoints.
			Egress: unrestrictedEgressRule(),
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile Velero NetworkPolicy: %w", err)
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		log.Info(fmt.Sprintf("Velero NetworkPolicy %s: %s", np.Name, op))
		r.EventRecorder.Event(np,
			corev1.EventTypeNormal,
			"VeleroNetworkPolicyReconciled",
			fmt.Sprintf("performed %s on Velero NetworkPolicy %s/%s", op, np.Namespace, np.Name),
		)
	}
	return nil
}

// reconcileVeleroMoverNetworkPolicy creates a NetworkPolicy covering Velero's
// dynamically-spawned mover/maintenance pods: CSI snapshot DataUpload/DataDownload
// exposer pods, PodVolumeBackup/PodVolumeRestore pods, and repository-maintenance
// job pods. These pods are created by Velero/node-agent at runtime (not from a pod
// template OADP owns) and do not carry the "component=velero" label used by
// reconcileVeleroNetworkPolicy, so they need their own NetworkPolicy. They are
// labeled via NodeAgentConfigMapSettings.PodLabels / RepositoryMaintenanceConfig.PodLabels
// (see updateNodeAgentCM / updateRepositoryMaintenanceCM), which OADP defaults to include
// networkPolicyMoverLabel.
func (r *DataProtectionApplicationReconciler) reconcileVeleroMoverNetworkPolicy(log logr.Logger) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      veleroMoverNetworkPolicyName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := createOrUpdateNetworkPolicy(r.Context, r.Client, np, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(r.dpa, np, r.Scheme); err != nil {
			return err
		}

		// Apply labels
		np.Labels = getDpaAppLabels(r.dpa)
		np.Labels, np.Annotations = applyResourceLabels(r.dpa, np.Labels, np.Annotations)

		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					networkPolicyMoverLabel: networkPolicyMoverLabelValue,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
				// No ingress rule: these are ephemeral data-transfer/maintenance pods,
				// not servers, so no inbound traffic is expected.
			},
			// Egress: unrestricted, same justification as the main Velero NetworkPolicy -
			// these pods move data directly to/from admin-configured, arbitrary
			// S3-compatible BackupStorageLocation endpoints.
			Egress: unrestrictedEgressRule(),
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile Velero mover NetworkPolicy: %w", err)
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		log.Info(fmt.Sprintf("Velero mover NetworkPolicy %s: %s", np.Name, op))
		r.EventRecorder.Event(np,
			corev1.EventTypeNormal,
			"VeleroMoverNetworkPolicyReconciled",
			fmt.Sprintf("performed %s on Velero mover NetworkPolicy %s/%s", op, np.Namespace, np.Name),
		)
	}
	return nil
}

// reconcileCLIServerNetworkPolicy creates a NetworkPolicy for the OADP CLI server.
// This server provides ConsoleCLIDownload endpoints for users to download the OADP CLI.
func (r *DataProtectionApplicationReconciler) reconcileCLIServerNetworkPolicy(log logr.Logger) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cliServerNetworkPolicyName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := createOrUpdateNetworkPolicy(r.Context, r.Client, np, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(r.dpa, np, r.Scheme); err != nil {
			return err
		}

		// Apply labels
		np.Labels = getDpaAppLabels(r.dpa)
		np.Labels, np.Annotations = applyResourceLabels(r.dpa, np.Labels, np.Annotations)

		// Pod selector: app=oadp-cli
		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "oadp-cli",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// Allow HTTP access from anywhere (CLI downloads)
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8080},
						},
					},
				},
			},
			// Egress: no rules - this is a static file server with no outbound calls,
			// so it relies entirely on the namespace default-deny baseline.
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile CLI server NetworkPolicy: %w", err)
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		log.Info(fmt.Sprintf("CLI server NetworkPolicy %s: %s", np.Name, op))
		r.EventRecorder.Event(np,
			corev1.EventTypeNormal,
			"CLIServerNetworkPolicyReconciled",
			fmt.Sprintf("performed %s on CLI server NetworkPolicy %s/%s", op, np.Namespace, np.Name),
		)
	}
	return nil
}

// reconcileVMDPServerNetworkPolicy creates a NetworkPolicy for the OADP VMDP (VM Data Protection) server.
// This server provides ConsoleCLIDownload endpoints for users to download the VMDP CLI.
func (r *DataProtectionApplicationReconciler) reconcileVMDPServerNetworkPolicy(log logr.Logger) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmdpServerNetworkPolicyName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := createOrUpdateNetworkPolicy(r.Context, r.Client, np, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(r.dpa, np, r.Scheme); err != nil {
			return err
		}

		// Apply labels
		np.Labels = getDpaAppLabels(r.dpa)
		np.Labels, np.Annotations = applyResourceLabels(r.dpa, np.Labels, np.Annotations)

		// Pod selector: app=oadp-vmdp
		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "oadp-vmdp",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// Allow HTTP access from anywhere (CLI downloads)
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8080},
						},
					},
				},
			},
			// Egress: no rules - this is a static file server with no outbound calls,
			// so it relies entirely on the namespace default-deny baseline.
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile VMDP server NetworkPolicy: %w", err)
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		log.Info(fmt.Sprintf("VMDP server NetworkPolicy %s: %s", np.Name, op))
		r.EventRecorder.Event(np,
			corev1.EventTypeNormal,
			"VMDPServerNetworkPolicyReconciled",
			fmt.Sprintf("performed %s on VMDP server NetworkPolicy %s/%s", op, np.Namespace, np.Name),
		)
	}
	return nil
}

// reconcileNonAdminNetworkPolicy creates a NetworkPolicy for the non-admin controller.
// Only created when spec.configuration.nonAdmin.enable is true.
func (r *DataProtectionApplicationReconciler) reconcileNonAdminNetworkPolicy(log logr.Logger) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nonAdminNetworkPolicyName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Delete NetworkPolicy if non-admin is disabled
	if !r.checkNonAdminEnabled() {
		if err := r.Get(r.Context, types.NamespacedName{
			Name:      np.Name,
			Namespace: np.Namespace,
		}, np); err != nil {
			if k8serrors.IsNotFound(err) {
				return nil // Already deleted
			}
			return fmt.Errorf("failed to check Non-Admin NetworkPolicy: %w", err)
		}

		if err := r.Delete(r.Context, np, &client.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete Non-Admin NetworkPolicy: %w", err)
		}

		log.Info(fmt.Sprintf("Non-Admin NetworkPolicy %s deleted (non-admin disabled)", np.Name))
		return nil
	}

	// Create/update NetworkPolicy if non-admin is enabled
	op, err := createOrUpdateNetworkPolicy(r.Context, r.Client, np, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(r.dpa, np, r.Scheme); err != nil {
			return err
		}

		// Apply labels
		np.Labels = getDpaAppLabels(r.dpa)
		np.Labels, np.Annotations = applyResourceLabels(r.dpa, np.Labels, np.Annotations)

		// Pod selector: app.kubernetes.io/component=manager (matches non-admin controller)
		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/component": "manager",
					"control-plane":               "non-admin-controller",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// Allow metrics and health probe access from monitoring namespace
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"network.openshift.io/policy-group": "monitoring",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							// Health probe port (8081) and metrics port (8080)
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8081},
						},
						{
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8080},
						},
					},
				},
			},
			// Egress: the non-admin controller only orchestrates via CRs (no cloud/BSL
			// credentials referenced in its code) and needs no direct cloud storage access.
			Egress: scopedEgressRules(),
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile Non-Admin NetworkPolicy: %w", err)
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		log.Info(fmt.Sprintf("Non-Admin NetworkPolicy %s: %s", np.Name, op))
		r.EventRecorder.Event(np,
			corev1.EventTypeNormal,
			"NonAdminNetworkPolicyReconciled",
			fmt.Sprintf("performed %s on Non-Admin NetworkPolicy %s/%s", op, np.Namespace, np.Name),
		)
	}
	return nil
}

// reconcileVMFileRestoreNetworkPolicy creates a NetworkPolicy for the VM file restore controller.
// Only created when VM file restore feature is enabled.
func (r *DataProtectionApplicationReconciler) reconcileVMFileRestoreNetworkPolicy(log logr.Logger) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmFileRestoreNetworkPolicyName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Delete NetworkPolicy if VM file restore is not enabled
	if !r.checkVMFileRestoreEnabled() {
		if err := r.Get(r.Context, types.NamespacedName{
			Name:      np.Name,
			Namespace: np.Namespace,
		}, np); err != nil {
			if k8serrors.IsNotFound(err) {
				return nil // Already deleted
			}
			return fmt.Errorf("failed to check VM File Restore NetworkPolicy: %w", err)
		}

		if err := r.Delete(r.Context, np, &client.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete VM File Restore NetworkPolicy: %w", err)
		}

		log.Info(fmt.Sprintf("VM File Restore NetworkPolicy %s deleted (feature disabled)", np.Name))
		return nil
	}

	// Create/update NetworkPolicy if VM file restore is enabled
	op, err := createOrUpdateNetworkPolicy(r.Context, r.Client, np, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(r.dpa, np, r.Scheme); err != nil {
			return err
		}

		// Apply labels
		np.Labels = getDpaAppLabels(r.dpa)
		np.Labels, np.Annotations = applyResourceLabels(r.dpa, np.Labels, np.Annotations)

		// Pod selector: control-plane=oadp-vm-file-restore-controller
		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"control-plane": "oadp-vm-file-restore-controller",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// Allow metrics and health probe access from monitoring namespace
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"network.openshift.io/policy-group": "monitoring",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							// Health probe port
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8081},
						},
						{
							// Metrics port (secure)
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8443},
						},
					},
				},
			},
			// Egress: VMFR downloads backup contents directly from the configured
			// BackupStorageLocation, which may be an arbitrary S3-compatible endpoint.
			Egress: unrestrictedEgressRule(),
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile VM File Restore NetworkPolicy: %w", err)
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		log.Info(fmt.Sprintf("VM File Restore NetworkPolicy %s: %s", np.Name, op))
		r.EventRecorder.Event(np,
			corev1.EventTypeNormal,
			"VMFileRestoreNetworkPolicyReconciled",
			fmt.Sprintf("performed %s on VM File Restore NetworkPolicy %s/%s", op, np.Namespace, np.Name),
		)
	}
	return nil
}

// reconcileKubevirtDatamoverNetworkPolicy creates a NetworkPolicy for the KubeVirt datamover controller.
// Only created when KubeVirt datamover feature is enabled.
func (r *DataProtectionApplicationReconciler) reconcileKubevirtDatamoverNetworkPolicy(log logr.Logger) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubevirtDatamoverNetworkPolicyName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Delete NetworkPolicy if KubeVirt datamover is not enabled
	if !r.checkKubevirtDatamoverEnabled() {
		if err := r.Get(r.Context, types.NamespacedName{
			Name:      np.Name,
			Namespace: np.Namespace,
		}, np); err != nil {
			if k8serrors.IsNotFound(err) {
				return nil // Already deleted
			}
			return fmt.Errorf("failed to check KubeVirt DataMover NetworkPolicy: %w", err)
		}

		if err := r.Delete(r.Context, np, &client.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete KubeVirt DataMover NetworkPolicy: %w", err)
		}

		log.Info(fmt.Sprintf("KubeVirt DataMover NetworkPolicy %s deleted (feature disabled)", np.Name))
		return nil
	}

	// Create/update NetworkPolicy if KubeVirt datamover is enabled
	op, err := createOrUpdateNetworkPolicy(r.Context, r.Client, np, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(r.dpa, np, r.Scheme); err != nil {
			return err
		}

		// Apply labels
		np.Labels = getDpaAppLabels(r.dpa)
		np.Labels, np.Annotations = applyResourceLabels(r.dpa, np.Labels, np.Annotations)

		// Pod selector: control-plane=oadp-kubevirt-datamover-controller
		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"control-plane": "oadp-kubevirt-datamover-controller",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// Allow metrics and health probe access from monitoring namespace
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"network.openshift.io/policy-group": "monitoring",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							// Health probe port
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8081},
						},
						{
							// Metrics port (secure)
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8443},
						},
					},
				},
			},
			// Egress: unrestricted. This controller authenticates directly to cloud
			// storage (e.g. Azure Workload Identity via pkg/credentials/stsflow) and
			// must reach admin-configured, arbitrary S3-compatible endpoints.
			Egress: unrestrictedEgressRule(),
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile KubeVirt DataMover NetworkPolicy: %w", err)
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		log.Info(fmt.Sprintf("KubeVirt DataMover NetworkPolicy %s: %s", np.Name, op))
		r.EventRecorder.Event(np,
			corev1.EventTypeNormal,
			"KubevirtDatamoverNetworkPolicyReconciled",
			fmt.Sprintf("performed %s on KubeVirt DataMover NetworkPolicy %s/%s", op, np.Namespace, np.Name),
		)
	}
	return nil
}

// reconcileKubevirtDatamoverPodsNetworkPolicy creates a NetworkPolicy for the
// upload/download pods dynamically spawned by the KubeVirt datamover controller
// (e.g. "kubevirt-dm-du-*") to perform CSI DataUpload/DataDownload transfers.
// These pods are distinct from the controller Deployment covered by
// reconcileKubevirtDatamoverNetworkPolicy: they carry kubevirtDatamoverPodTypeLabel
// instead of "control-plane", so a separate PodSelector/policy is required (mirrors
// how reconcileVeleroMoverNetworkPolicy covers Velero's own dynamically-spawned
// mover pods separately from reconcileVeleroNetworkPolicy).
// Only created when KubeVirt datamover feature is enabled.
func (r *DataProtectionApplicationReconciler) reconcileKubevirtDatamoverPodsNetworkPolicy(log logr.Logger) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubevirtDatamoverPodsNetworkPolicyName,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Delete NetworkPolicy if KubeVirt datamover is not enabled
	if !r.checkKubevirtDatamoverEnabled() {
		if err := r.Get(r.Context, types.NamespacedName{
			Name:      np.Name,
			Namespace: np.Namespace,
		}, np); err != nil {
			if k8serrors.IsNotFound(err) {
				return nil // Already deleted
			}
			return fmt.Errorf("failed to check KubeVirt DataMover pods NetworkPolicy: %w", err)
		}

		if err := r.Delete(r.Context, np, &client.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete KubeVirt DataMover pods NetworkPolicy: %w", err)
		}

		log.Info(fmt.Sprintf("KubeVirt DataMover pods NetworkPolicy %s deleted (feature disabled)", np.Name))
		return nil
	}

	op, err := createOrUpdateNetworkPolicy(r.Context, r.Client, np, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(r.dpa, np, r.Scheme); err != nil {
			return err
		}

		// Apply labels
		np.Labels = getDpaAppLabels(r.dpa)
		np.Labels, np.Annotations = applyResourceLabels(r.dpa, np.Labels, np.Annotations)

		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      kubevirtDatamoverPodTypeLabel,
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
				// No ingress rule: these are ephemeral data-transfer pods, not
				// servers, so no inbound traffic is expected.
			},
			// Egress: unrestricted. These pods move VM disk data directly
			// to/from admin-configured, arbitrary S3-compatible endpoints,
			// same justification as the Velero mover NetworkPolicy.
			Egress: unrestrictedEgressRule(),
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile KubeVirt DataMover pods NetworkPolicy: %w", err)
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		log.Info(fmt.Sprintf("KubeVirt DataMover pods NetworkPolicy %s: %s", np.Name, op))
		r.EventRecorder.Event(np,
			corev1.EventTypeNormal,
			"KubevirtDatamoverPodsNetworkPolicyReconciled",
			fmt.Sprintf("performed %s on KubeVirt DataMover pods NetworkPolicy %s/%s", op, np.Namespace, np.Name),
		)
	}
	return nil
}

package controller

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openshift/oadp-operator/pkg/common"
)

const (
	defaultDenyNetworkPolicyName       = "default-deny"
	operatorNetworkPolicyName          = "oadp-operator-network-policy"
	veleroNetworkPolicyName            = "velero-network-policy"
	veleroMoverNetworkPolicyName       = "velero-mover-network-policy"
	cliServerNetworkPolicyName         = "oadp-cli-server-network-policy"
	vmdpServerNetworkPolicyName        = "oadp-vmdp-server-network-policy"
	nonAdminNetworkPolicyName          = "non-admin-controller-network-policy"
	vmFileRestoreNetworkPolicyName     = "vm-file-restore-controller-network-policy"
	kubevirtDatamoverNetworkPolicyName = "kubevirt-datamover-controller-network-policy"

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

// ReconcileNetworkPolicies creates NetworkPolicies for OADP operands.
// OCPSTRAT-819: Operator creates NPs at runtime for its workloads (operands).
// Design: Default-deny all ingress/egress traffic in the namespace, then allow only
// the specific ingress (metrics/health) and egress (DNS/API-server, or unrestricted
// for operands that talk directly to admin-configured, arbitrary S3-compatible
// endpoints) each operand actually needs.
func (r *DataProtectionApplicationReconciler) ReconcileNetworkPolicies(log logr.Logger) (bool, error) {
	// Reconcile default-deny policy first (baseline security)
	if err := r.reconcileDefaultDenyNetworkPolicy(log); err != nil {
		return false, err
	}

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

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, np, func() error {
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

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, np, func() error {
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
					// Allow metrics from anywhere (standard pattern per OpenShift NP guide), and
					// allow the kubelet to reach the manager's health-probe port (liveness/readiness/
					// startup probes hit :8081/healthz from the node, not from a pod, so this must
					// stay open or the operator pod gets marked unhealthy and restarts).
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8443},
						},
						{
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8081},
						},
					},
				},
			},
			// Egress: the operator only reconciles CRs via the Kubernetes API and needs
			// no direct cloud/object-storage access.
			Egress: scopedEgressRules(),
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

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, np, func() error {
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
					// Allow metrics scrape from anywhere (standard pattern per OpenShift NP guide)
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

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, np, func() error {
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

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, np, func() error {
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

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, np, func() error {
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
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, np, func() error {
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
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, np, func() error {
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
			// Egress: this controller only orchestrates via CRs (no cloud/BSL credentials
			// referenced in its code) and needs no direct cloud storage access.
			Egress: scopedEgressRules(),
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
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, np, func() error {
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

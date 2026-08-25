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
	cliServerNetworkPolicyName         = "oadp-cli-server-network-policy"
	vmdpServerNetworkPolicyName        = "oadp-vmdp-server-network-policy"
	nonAdminNetworkPolicyName          = "non-admin-controller-network-policy"
	vmFileRestoreNetworkPolicyName     = "vm-file-restore-controller-network-policy"
	kubevirtDatamoverNetworkPolicyName = "kubevirt-datamover-controller-network-policy"
)

// ReconcileNetworkPolicies creates NetworkPolicies for OADP operands.
// OCPSTRAT-819: Operator creates NPs at runtime for its workloads (operands).
// Design: Default-deny all traffic, then allow specific ingress (metrics/health).
// Egress is unrestricted because BSLs can point to arbitrary S3-compatible endpoints.
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
		// No ingress/egress rules = deny all traffic
		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				// Egress intentionally omitted - not restricting egress
			},
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile default-deny NetworkPolicy: %w", err)
	}

	log.Info(fmt.Sprintf("Default-deny NetworkPolicy %s: %s", np.Name, op))
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
				// Egress intentionally omitted to leave egress unrestricted
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// Allow metrics from anywhere (standard pattern per OpenShift NP guide)
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: func() *corev1.Protocol { p := corev1.ProtocolTCP; return &p }(),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8443},
						},
					},
				},
			},
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile OADP operator NetworkPolicy: %w", err)
	}

	log.Info(fmt.Sprintf("OADP operator NetworkPolicy %s: %s", np.Name, op))
	return nil
}

// reconcileVeleroNetworkPolicy creates a NetworkPolicy for Velero deployment and node-agent daemonset.
// Selector: component=velero also covers datamover/repo-maintenance pods spawned by Velero/node-agent.
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

		// Pod selector: component=velero (matches velero deployment, node-agent, and their spawned pods)
		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"component": common.Velero,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				// Egress intentionally omitted to leave egress unrestricted
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
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile Velero NetworkPolicy: %w", err)
	}

	log.Info(fmt.Sprintf("Velero NetworkPolicy %s: %s", np.Name, op))
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
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile CLI server NetworkPolicy: %w", err)
	}

	log.Info(fmt.Sprintf("CLI server NetworkPolicy %s: %s", np.Name, op))
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
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile VMDP server NetworkPolicy: %w", err)
	}

	log.Info(fmt.Sprintf("VMDP server NetworkPolicy %s: %s", np.Name, op))
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
				// Egress intentionally omitted to leave egress unrestricted
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
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile Non-Admin NetworkPolicy: %w", err)
	}

	log.Info(fmt.Sprintf("Non-Admin NetworkPolicy %s: %s", np.Name, op))
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
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile VM File Restore NetworkPolicy: %w", err)
	}

	log.Info(fmt.Sprintf("VM File Restore NetworkPolicy %s: %s", np.Name, op))
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
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile KubeVirt DataMover NetworkPolicy: %w", err)
	}

	log.Info(fmt.Sprintf("KubeVirt DataMover NetworkPolicy %s: %s", np.Name, op))
	return nil
}

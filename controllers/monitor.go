package controllers

import (
	"fmt"
	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	monitor "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *DPAReconciler) ReconcileServiceMonitor(log logr.Logger) (bool, error) {

	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	serviceMonitor := &monitor.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "oadp-operator",
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, serviceMonitor, func() error {

		if serviceMonitor.ObjectMeta.CreationTimestamp.IsZero() {
			serviceMonitor.Spec.Selector = metav1.LabelSelector{
				MatchLabels: map[string]string{
					oadpv1alpha1.OadpOperatorLabel: "true",
					"app":                          "oadp-operator",
				},
			}
		}

		// update service monitor
		return r.buildServiceMonitor(serviceMonitor, &dpa)
	})

	if err != nil {
		return false, err
	}

	//TODO: Review service monitor status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate service monitor was created or updated
		r.EventRecorder.Event(serviceMonitor,
			corev1.EventTypeNormal,
			"ServiceMonitorReconciled",
			fmt.Sprintf("performed %s on service monitor %s/%s", op, serviceMonitor.Namespace, serviceMonitor.Name),
		)
	}
	return true, nil
}

func (r *DPAReconciler) buildServiceMonitor(serviceMonitor *monitor.ServiceMonitor, dpa *oadpv1alpha1.DataProtectionApplication) error {

	if dpa == nil {
		return fmt.Errorf("dpa CR cannot be nil")
	}

	if serviceMonitor == nil {
		return fmt.Errorf("service monitor cannot be nil")
	}

	// Setting controller owner reference on the service monitor
	err := controllerutil.SetControllerReference(dpa, serviceMonitor, r.Scheme)
	if err != nil {
		return err
	}

	serviceMonitor.Spec.Selector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			oadpv1alpha1.OadpOperatorLabel: "true",
			"app":                          "oadp-operator",
		},
	}

	serviceMonitor.Labels = map[string]string{
		oadpv1alpha1.OadpOperatorLabel: "true",
		"app":                          "oadp-operator",
	}

	serviceMonitor.Spec.Endpoints = []monitor.Endpoint{
		{
			Interval: "30s",
			Port:     "metrics",
		},
	}

	serviceMonitor.Spec.JobLabel = "app"

	serviceMonitor.Spec.NamespaceSelector = monitor.NamespaceSelector{
		MatchNames: []string{
			dpa.Namespace,
		},
	}

	return nil
}

func (r *DPAReconciler) ReconcileVeleroServiceMonitor(log logr.Logger) (bool, error) {

	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	serviceMonitor := &monitor.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openshift-adp-dpa-monitor",
			Namespace: r.NamespacedName.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, serviceMonitor, func() error {

		if serviceMonitor.ObjectMeta.CreationTimestamp.IsZero() {
			serviceMonitor.Spec.Selector = metav1.LabelSelector{
				MatchLabels: map[string]string{
					"k8s-app":   "openshift-adp",
					"component": "velero",
					"deploy":    "velero",
				},
			}
		}

		// update service monitor
		return r.buildVeleroServiceMonitor(serviceMonitor, &dpa)
	})

	if err != nil {
		return false, err
	}

	//TODO: Review service monitor status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate service monitor was created or updated
		r.EventRecorder.Event(serviceMonitor,
			corev1.EventTypeNormal,
			"VeleroServiceMonitorReconciled",
			fmt.Sprintf("performed %s on dpa service monitor %s/%s", op, serviceMonitor.Namespace, serviceMonitor.Name),
		)
	}
	return true, nil
}

func (r *DPAReconciler) buildVeleroServiceMonitor(serviceMonitor *monitor.ServiceMonitor, dpa *oadpv1alpha1.DataProtectionApplication) error {

	if dpa == nil {
		return fmt.Errorf("dpa CR cannot be nil")
	}

	if serviceMonitor == nil {
		return fmt.Errorf("service monitor cannot be nil")
	}

	// Setting controller owner reference on the service monitor
	err := controllerutil.SetControllerReference(dpa, serviceMonitor, r.Scheme)
	if err != nil {
		return err
	}

	serviceMonitor.Spec.Selector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			"k8s-app":   "openshift-adp",
			"component": "velero",
			"deploy":    "velero",
		},
	}

	serviceMonitor.Labels = map[string]string{
		"k8s-app": "openshift-adp-dpa-monitor",
	}

	serviceMonitor.Spec.Endpoints = []monitor.Endpoint{
		{
			Interval: "30s",
			Port:     "monitoring",
		},
	}

	//serviceMonitor.Spec.JobLabel = "app"

	serviceMonitor.Spec.NamespaceSelector = monitor.NamespaceSelector{
		MatchNames: []string{
			dpa.Namespace,
		},
	}

	return nil
}

func (r *DPAReconciler) ReconcileMetricsSVC(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "oadp-operator-metrics",
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Create SVC
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &svc, func() error {
		// TODO: check for svc status condition errors and respond here
		err := r.updateMetricsSVC(&svc, &dpa)

		return err
	})
	if err != nil {
		return false, err
	}
	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate SVC was created or updated
		r.EventRecorder.Event(&svc,
			corev1.EventTypeNormal,
			"MetricsServiceReconciled",
			fmt.Sprintf("performed %s on service %s/%s", op, svc.Namespace, svc.Name),
		)
	}

	return true, nil
}

func (r *DPAReconciler) updateMetricsSVC(svc *corev1.Service, dpa *oadpv1alpha1.DataProtectionApplication) error {
	// Setting controller owner reference on the metrics svc
	err := controllerutil.SetControllerReference(dpa, svc, r.Scheme)
	if err != nil {
		return err
	}

	// when updating the spec fields we update each field individually
	// to get around the immutable fields
	svc.Spec.Selector = map[string]string{
		oadpv1alpha1.OadpOperatorLabel: "true",
		"control-plane":                "controller-manager",
	}

	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:     "metrics",
			Port:     int32(2112),
			Protocol: corev1.ProtocolTCP,
			TargetPort: intstr.IntOrString{
				IntVal: int32(2112),
			},
		},
	}

	svc.Spec.ClusterIP = "None"

	svc.Spec.SessionAffinity = "None"

	svc.Labels = map[string]string{
		oadpv1alpha1.OadpOperatorLabel: "true",
		"app":                          "oadp-operator",
	}
	return nil
}

func (r *DPAReconciler) ReconcileVeleroMetricsSVC(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.NamespacedName.Name,
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Create SVC
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &svc, func() error {
		// TODO: check for svc status condition errors and respond here
		err := r.updateVeleroMetricsSVC(&svc, &dpa)

		return err
	})
	if err != nil {
		return false, err
	}
	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate SVC was created or updated
		r.EventRecorder.Event(&svc,
			corev1.EventTypeNormal,
			"VeleroMetricsServiceReconciled",
			fmt.Sprintf("performed %s on dpa metrics service %s/%s", op, svc.Namespace, svc.Name),
		)
	}

	return true, nil
}

func (r *DPAReconciler) updateVeleroMetricsSVC(svc *corev1.Service, dpa *oadpv1alpha1.DataProtectionApplication) error {
	// Setting controller owner reference on the metrics svc
	err := controllerutil.SetControllerReference(dpa, svc, r.Scheme)
	if err != nil {
		return err
	}

	// when updating the spec fields we update each field individually
	// to get around the immutable fields
	svc.Spec.Selector = map[string]string{
		"k8s-app":   "openshift-adp",
		"component": "velero",
		"deploy":    "velero",
	}

	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name: "monitoring",
			Port: int32(8085),
			TargetPort: intstr.IntOrString{
				IntVal: int32(8085),
			},
		},
	}

	svc.Labels = map[string]string{
		"k8s-app":   "openshift-adp",
		"component": "velero",
		"deploy":    "velero",
	}
	return nil
}

func (r *DPAReconciler) ReconcileMetricsRole(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	role := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus-k8s",
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Create Role
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &role, func() error {
		// TODO: check for role status condition errors and respond here
		err := r.updateMetricsRole(&role, &dpa)

		return err
	})
	if err != nil {
		return false, err
	}
	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate role was created or updated
		r.EventRecorder.Event(&role,
			corev1.EventTypeNormal,
			"MetricsRoleReconciled",
			fmt.Sprintf("performed %s on role %s/%s", op, role.Namespace, role.Name),
		)
	}

	return true, nil
}

func (r *DPAReconciler) updateMetricsRole(role *rbacv1.Role, dpa *oadpv1alpha1.DataProtectionApplication) error {
	// Setting controller owner reference on the metrics role
	err := controllerutil.SetControllerReference(dpa, role, r.Scheme)
	if err != nil {
		return err
	}

	role.Rules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"services",
				"endpoints",
				"pods",
			},
			Verbs: []string{
				"get",
				"list",
				"watch",
			},
		},
	}

	return nil
}

func (r *DPAReconciler) ReconcileMetricsRoleBinding(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus-k8s",
			Namespace: r.NamespacedName.Namespace,
		},
	}

	// Create RoleBinding
	op, err := controllerutil.CreateOrUpdate(r.Context, r.Client, &roleBinding, func() error {
		// TODO: check for roleBinding status condition errors and respond here
		err := r.updateMetricsRoleBinding(&roleBinding, &dpa)

		return err
	})
	if err != nil {
		return false, err
	}
	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate roleBinding was created or updated
		r.EventRecorder.Event(&roleBinding,
			corev1.EventTypeNormal,
			"MetricsRoleBindingReconciled",
			fmt.Sprintf("performed %s on roleBinding %s/%s", op, roleBinding.Namespace, roleBinding.Name),
		)
	}

	return true, nil
}

func (r *DPAReconciler) updateMetricsRoleBinding(roleBinding *rbacv1.RoleBinding, dpa *oadpv1alpha1.DataProtectionApplication) error {
	// Setting controller owner reference on the metrics roleBinding
	err := controllerutil.SetControllerReference(dpa, roleBinding, r.Scheme)
	if err != nil {
		return err
	}

	roleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     "prometheus-k8s",
	}

	roleBinding.Subjects = []rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      "prometheus-k8s",
			Namespace: "openshift-monitoring",
		},
	}

	return nil
}
